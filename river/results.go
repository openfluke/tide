package river

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
)

// Row is one finished cell for the compare site (acc + how fast it got there).
type Row struct {
	ID               string   `json:"id"`
	Mode             string   `json:"mode"` // display mode (mix label when BranchModes set)
	ParentMode       string   `json:"parent_mode,omitempty"`
	BranchModes      []string `json:"branch_modes,omitempty"`
	MixPattern       string   `json:"mix_pattern,omitempty"` // alt | block | roundrobin
	MixLabel         string   `json:"mix_label,omitempty"`
	DType            string   `json:"dtype"`
	Format           string   `json:"format"`
	Arch             string   `json:"arch"`
	LR               float64  `json:"lr"`
	LRLabel          string   `json:"lr_label"`
	Acc              float64  `json:"acc"`
	SoftAcc          float64  `json:"soft_acc"`
	Throughput       float64  `json:"throughput"`
	Availability     float64  `json:"availability"`
	DurationSec      float64  `json:"duration_sec"`
	AccPerSec        float64  `json:"acc_per_sec"`
	AccPerSecPerMiB  float64 `json:"acc_per_sec_per_mib"` // raw Acc/sec ÷ weight MiB
	DenseScore       float64 `json:"dense_score"`         // AccPerSecPerMiB × (Acc/100) — must actually learn
	TimeTo50Sec      float64 `json:"time_to_50_sec"`
	TimeTo25Sec      float64 `json:"time_to_25_sec"`
	WeightBytes      int64   `json:"weight_bytes"`
	WeightKiB        float64 `json:"weight_kib"`
	Status           string  `json:"status"`
	FinishedAt       string  `json:"finished_at,omitempty"`
}

// File is the on-disk results artifact the website reads.
type File struct {
	Generated  time.Time `json:"generated"`
	Machine    string    `json:"machine"`
	TrainN     int       `json:"train_n"`
	SampleSeed uint64    `json:"sample_seed"`
	LRs        []float64 `json:"lrs"`
	LRLabels   []string  `json:"lr_labels"`
	Matrix     string    `json:"matrix"`
	Rows       []Row     `json:"rows"`
}

type Store struct {
	mu      sync.Mutex
	path    string
	meta    File
	byID    map[string]Row
	planIDs []string // full sweep IDs (with |lr=…) for progress truth
}

func NewStore(path, machine, matrix string, trainN int, seed uint64, lrs []float64) *Store {
	labels := make([]string, len(lrs))
	for i, lr := range lrs {
		labels[i] = FormatLR(lr)
	}
	s := &Store{
		path: path,
		byID: map[string]Row{},
		meta: File{
			Machine:    machine,
			TrainN:     trainN,
			SampleSeed: seed,
			LRs:        append([]float64(nil), lrs...),
			LRLabels:   labels,
			Matrix:     matrix,
		},
	}
	_ = s.load()
	return s
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	repaired := false
	for _, r := range f.Rows {
		if r.ID == "" {
			continue
		}
		before := r.Mode
		enrichRow(&r)
		if r.Mode != before || len(r.BranchModes) > 0 {
			repaired = repaired || r.Mode != before
		}
		s.byID[r.ID] = r
	}
	if repaired {
		_ = s.flushLocked()
	}
	return nil
}

func (s *Store) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	return ok
}

func (s *Store) SyncFromTracker(tr *pulse.Tracker, cellLR map[string]float64) error {
	if tr == nil {
		return nil
	}
	// Full report archive — live.Completed is a trimmed ring (~2000) and would
	// drop older cells from results.json on every flush.
	rows := tr.ReportResults()
	if len(rows) == 0 {
		rows = tr.Snapshot().Completed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		if r.Status != "ok" && r.Status != "gap" && r.Status != "fail" {
			continue
		}
		id := r.Cell.ID
		if id == "" {
			continue
		}
		lr := cellLR[id]
		if lr == 0 {
			lr = ParseLRFromID(id)
		}
		row := Row{
			ID:          id,
			Mode:        string(r.Cell.Mode),
			ParentMode:  string(r.Cell.Mode),
			DType:       r.Cell.DType.String(),
			Format:      r.Cell.Format.String(),
			Arch:        string(permute.CanonicalArch(r.Cell.Arch)),
			LR:          lr,
			LRLabel:     FormatLR(lr),
			Acc:         r.Snapshot.AvgAccuracy,
			SoftAcc:     r.Snapshot.SoftAcc,
			Throughput:  r.Snapshot.Throughput,
			Availability: r.Snapshot.Availability,
			DurationSec: r.Snapshot.Duration.Seconds(),
			AccPerSec:   r.Snapshot.AccPerSec,
			TimeTo50Sec: r.Snapshot.TimeToAcc50Sec,
			TimeTo25Sec: r.Snapshot.TimeToAcc25Sec,
			WeightBytes: r.Snapshot.WeightBytes,
			Status:      r.Status,
			FinishedAt:  r.Ended.UTC().Format(time.RFC3339),
		}
		enrichRow(&row)
		// Prefer keeping a successful row over a later fail overwrite.
		if prev, ok := s.byID[id]; ok && (prev.Status == "ok" || prev.Status == "gap") && r.Status == "fail" {
			continue
		}
		s.byID[id] = row
	}
	return s.flushLocked()
}

// SetTrainN updates the train subset size shown on the compare site.
func (s *Store) SetTrainN(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta.TrainN = n
}

// SetMatrix updates the matrix label (near80, mixcam, …).
func (s *Store) SetMatrix(matrix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta.Matrix = matrix
}

func (s *Store) Snapshot() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

// PulseResults rebuilds tide pulse rows from results.json so PDF/LPD can use
// the full archive after checkpoint Completed was capped at ~2000.
func (s *Store) PulseResults() []pulse.Result {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]pulse.Result, 0, len(s.byID))
	for _, r := range s.byID {
		if r.Status != "ok" && r.Status != "gap" {
			continue
		}
		id := permute.NormalizeCellID(r.ID)
		arch := permute.CanonicalArch(permute.ArchKind(r.Arch))
		snap := metrics.Snapshot{
			AvgAccuracy:  r.Acc,
			SoftAcc:      r.SoftAcc,
			Throughput:   r.Throughput,
			Availability: r.Availability,
			WeightBytes:  r.WeightBytes,
			AccPerSec:    r.AccPerSec,
			TimeToAcc50Sec: r.TimeTo50Sec,
			TimeToAcc25Sec: r.TimeTo25Sec,
			Duration:     time.Duration(r.DurationSec * float64(time.Second)),
		}
		if snap.WeightBytes > 0 {
			snap.WeightMiB = float64(snap.WeightBytes) / (1024 * 1024)
		}
		if snap.Availability > 0 || snap.Throughput > 0 {
			snap.Score = snap.Throughput * snap.Availability * snap.AvgAccuracy / 10000
		}
		out = append(out, pulse.Result{
			Cell: permute.Cell{
				ID:     id,
				Mode:   permute.TrainMode(pulseTrainMode(r)),
				DType:  core.ParseDType(r.DType),
				Format: quant.ParseFormatName(r.Format),
				Arch:   arch,
			},
			Status:   r.Status,
			Snapshot: snap,
		})
	}
	return out
}

// pulseTrainMode is the Tide/Welvet train token for archive re-seed.
// Mix cells keep the NormalBP parent; uniform cells must NOT fall back to
// DefaultParentMode — that poisoned results.json Mode to NormalBP for every row.
func pulseTrainMode(r Row) string {
	if len(r.BranchModes) > 0 || r.MixPattern != "" {
		return firstNonEmpty(r.ParentMode, DefaultParentMode)
	}
	if m := modeTokenFromID(r.ID); m != "" {
		return m
	}
	if r.Mode != "" && r.Mode != DefaultParentMode {
		return r.Mode
	}
	return firstNonEmpty(r.ParentMode, "sgd")
}

func (s *Store) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f := s.snapshotLocked()
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) snapshotLocked() File {
	rows := make([]Row, 0, len(s.byID))
	for _, r := range s.byID {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Acc != rows[j].Acc {
			return rows[i].Acc > rows[j].Acc
		}
		if rows[i].Throughput != rows[j].Throughput {
			return rows[i].Throughput > rows[j].Throughput
		}
		return rows[i].ID < rows[j].ID
	})
	out := s.meta
	out.Generated = time.Now().UTC()
	out.Rows = rows

	seen := map[string]bool{}
	var labels []string
	var lrs []float64
	for i, lab := range out.LRLabels {
		if seen[lab] {
			continue
		}
		seen[lab] = true
		labels = append(labels, lab)
		if i < len(out.LRs) {
			lrs = append(lrs, out.LRs[i])
		}
	}
	for _, r := range rows {
		if r.LRLabel == "" || seen[r.LRLabel] {
			continue
		}
		seen[r.LRLabel] = true
		labels = append(labels, r.LRLabel)
		lrs = append(lrs, r.LR)
	}
	out.LRLabels = labels
	out.LRs = lrs
	return out
}

// ParseLRFromID reads |lr=… from a cell id (0 if missing/unparseable).
func ParseLRFromID(id string) float64 {
	const tag = "|lr="
	i := strings.LastIndex(id, tag)
	if i < 0 {
		return 0
	}
	v, err := parseLRToken(id[i+len(tag):])
	if err != nil {
		return 0
	}
	return v
}

// ComparePayload is what /api/results serves (plus aggregates for charts).
type ComparePayload struct {
	File
	ByMode      []AggBar        `json:"by_mode"`
	ByDType     []AggBar        `json:"by_dtype"`
	ByArch      []AggBar        `json:"by_arch"`
	Overlap     []OverlapSeries `json:"overlap"`
	TopAcc      []Row           `json:"top_acc"`
	TopThru     []Row           `json:"top_throughput"`
	TopSpeed    []Row           `json:"top_acc_per_sec"`
	TopDense         []Row            `json:"top_acc_per_sec_per_mib"` // best Acc/s at smallest RAM
	DenseBars        []DenseBar       `json:"dense_bars"`             // same ranking for the bar chart
	LeanBest         *LeanSummary     `json:"lean_best,omitempty"`    // ≥95% of champ Acc · smallest RAM · fastest
	LeanBars         []LeanBar        `json:"lean_bars"`              // top-20 runner-ups for the lean chart
	LeanByDtype      []LeanBar        `json:"lean_by_dtype"`          // one lean champ per dtype, KiB small→big
	ModeDtypeGrids   []ModeDtypeGrid        `json:"mode_dtype_grids"`
	BestModeByDType  []BestModeByDTypeGrid  `json:"best_mode_by_dtype"`
}

// LeanSummary is the global Acc champion + Acc threshold for the lean ranking.
type LeanSummary struct {
	BestAcc     float64 `json:"best_acc"`
	BestID      string  `json:"best_id"`
	Threshold   float64 `json:"threshold"` // 0.95 × best_acc
	PctOfBest   float64 `json:"pct_of_best"` // always 95
	Eligible    int     `json:"eligible"`
	WinnerID    string  `json:"winner_id,omitempty"`
	WinnerKiB   float64 `json:"winner_kib,omitempty"`
	WinnerSec   float64 `json:"winner_duration_sec,omitempty"`
	WinnerAcc   float64 `json:"winner_acc,omitempty"`
}

// LeanBar is one cell that hit ≥95% of the global best Acc, ranked small RAM then fast.
type LeanBar struct {
	Rank        int     `json:"rank"`
	Label       string  `json:"label"`
	Mode        string  `json:"mode"`
	DType       string  `json:"dtype"`
	Arch        string  `json:"arch"`
	LRLabel     string  `json:"lr_label"`
	Acc         float64 `json:"acc"`
	PctOfBest   float64 `json:"pct_of_best"`
	WeightKiB   float64 `json:"weight_kib"`
	DurationSec float64 `json:"duration_sec"`
	Throughput  float64 `json:"throughput"`
	AccPerSec   float64 `json:"acc_per_sec"`
	ID          string  `json:"id"`
}

// ModeDtypeGrid is one LR slice: dtype cells per train mode × cameral arch.
type ModeDtypeGrid struct {
	LRLabel string          `json:"lr_label"`
	LR      float64         `json:"lr"`
	DTypes  []string        `json:"dtypes"`
	Modes   []string        `json:"modes"`
	Arches  []string        `json:"arches"`
	Cells   []ModeDtypeCell `json:"cells"`
}

// ModeDtypeCell is performance for one train mode × dtype × arch (not rolled up).
type ModeDtypeCell struct {
	Mode           string  `json:"mode"`
	DType          string  `json:"dtype"`
	Arch           string  `json:"arch"`
	N              int     `json:"n"`
	MeanAcc        float64 `json:"mean_acc"`
	MeanThru       float64 `json:"mean_throughput"`
	MeanAccPS      float64 `json:"mean_acc_per_sec"`
	BestAcc        float64 `json:"best_acc"`
	BestThroughput float64 `json:"best_throughput"`
}

// BestModeByDTypeRow is the highest-Acc train mode for one dtype (per LR, per arch).
type BestModeByDTypeRow struct {
	DType      string  `json:"dtype"`
	Mode       string  `json:"mode"`
	Arch       string  `json:"arch"`
	LRLabel    string  `json:"lr_label"`
	Acc        float64 `json:"acc"`
	Throughput float64 `json:"throughput"`
	AccPerSec  float64 `json:"acc_per_sec"`
	ID         string  `json:"id"`
}

// BestModeByDTypeGrid is best train mode by Acc for every dtype at one LR.
type BestModeByDTypeGrid struct {
	LRLabel string               `json:"lr_label"`
	LR      float64              `json:"lr"`
	Rows    []BestModeByDTypeRow `json:"rows"`
}

// DenseBar is one cell for the Acc/sec÷MiB chart.
type DenseBar struct {
	Label      string  `json:"label"`
	Mode       string  `json:"mode"`
	DType      string  `json:"dtype"`
	Arch       string  `json:"arch"`
	LRLabel    string  `json:"lr_label"`
	AccPerSec  float64 `json:"acc_per_sec"`
	WeightKiB  float64 `json:"weight_kib"`
	Dense      float64 `json:"acc_per_sec_per_mib"`
	Score      float64 `json:"dense_score"` // Acc-weighted rank key
	Acc        float64 `json:"acc"`
	ID         string  `json:"id"`
}

type AggBar struct {
	Key       string  `json:"key"`
	LRLabel   string  `json:"lr_label"`
	N         int     `json:"n"`
	MeanAcc   float64 `json:"mean_acc"`
	MeanThru  float64 `json:"mean_throughput"`
	MeanAccPS float64 `json:"mean_acc_per_sec"`
	BestAcc   float64 `json:"best_acc"`
	BestThru  float64 `json:"best_throughput"`
}

type OverlapSeries struct {
	Mode   string         `json:"mode"`
	Points []OverlapPoint `json:"points"`
}

// OverlapPoint is the single best-Acc cell for that mode at that LR (not a mean).
type OverlapPoint struct {
	LR          float64 `json:"lr"`
	LRLabel     string  `json:"lr_label"`
	Acc         float64 `json:"acc"`
	Throughput  float64 `json:"throughput"`
	AccPerSec   float64 `json:"acc_per_sec"`
	DType       string  `json:"dtype"`
	Arch        string  `json:"arch"`
	ID          string  `json:"id"`
}

func buildCompare(f File) ComparePayload {
	rows := make([]Row, len(f.Rows))
	copy(rows, f.Rows)
	for i := range rows {
		enrichRow(&rows[i])
	}
	f.Rows = rows
	dense := topN(denseCandidates(rows), 25, denserThan)
	leanSum, lean := buildLeanBars(rows, 20)
	_, leanDtype := buildLeanByDtype(rows)
	return ComparePayload{
		File:      f,
		ByMode:    aggregate(rows, func(r Row) string { return r.Mode }),
		ByDType:   aggregate(rows, func(r Row) string { return r.DType }),
		ByArch:    aggregate(rows, func(r Row) string { return r.Arch }),
		Overlap:   overlapByMode(rows),
		TopAcc:    topN(rows, 25, func(a, b Row) bool { return a.Acc > b.Acc }),
		TopThru:   topN(rows, 25, func(a, b Row) bool { return a.Throughput > b.Throughput }),
		TopSpeed:  topN(rows, 25, func(a, b Row) bool { return a.AccPerSec > b.AccPerSec }),
		TopDense:        dense,
		DenseBars:       denseBars(dense),
		LeanBest:        leanSum,
		LeanBars:        lean,
		LeanByDtype:     leanDtype,
		ModeDtypeGrids:  buildModeDtypeGrids(rows),
		BestModeByDType: buildBestModeByDType(rows),
	}
}

func enrichRow(r *Row) {
	if r.WeightBytes > 0 {
		r.WeightKiB = float64(r.WeightBytes) / 1024
		mib := float64(r.WeightBytes) / (1024 * 1024)
		if mib > 0 && r.AccPerSec > 0 {
			r.AccPerSecPerMiB = r.AccPerSec / mib
		}
	}
	applyMixTags(r)
	// Non-mix: ID pipe mode is canonical. PulseResults used to re-seed Tide with
	// DefaultParentMode (NormalBP), then SyncFromTracker wrote that over every Mode.
	if len(r.BranchModes) == 0 {
		if m := modeTokenFromID(r.ID); m != "" {
			r.Mode = m
		}
	}
	r.DenseScore = denseScore(*r)
}

// modeTokenFromID returns the train-mode segment of dtype|format|mode|arch|…
func modeTokenFromID(id string) string {
	id = permute.NormalizeCellID(id)
	if id == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(id, "|") {
		if p == "" || strings.HasPrefix(p, "lr=") || strings.HasPrefix(p, "bm=") ||
			strings.HasPrefix(p, "pat=") || strings.HasPrefix(p, "cs=") ||
			strings.HasPrefix(p, "ba=") || strings.HasPrefix(p, "rot=") ||
			strings.HasPrefix(p, "dna=") || strings.HasPrefix(p, "kit=") {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// applyMixTags lifts bm=/pat=/cs=/ba=/rot=/dna=/kit= from the cell ID so charts
// and tables show the per-cam training modes (and CamSync α) instead of the
// NormalBP parent token.
func applyMixTags(r *Row) {
	if r == nil || r.ID == "" {
		return
	}
	bm, pat, cs, extra := parseMixTags(r.ID)
	if len(bm) == 0 && extra == "" {
		return
	}
	if len(bm) == 0 {
		return
	}
	if r.ParentMode == "" {
		r.ParentMode = r.Mode
		if r.ParentMode == "" {
			r.ParentMode = DefaultParentMode
		}
	}
	r.BranchModes = bm
	r.MixPattern = pat
	label := strings.Join(bm, "+")
	if pat != "" {
		label = pat + " · " + label
	}
	if cs != "" {
		label = label + " · cs=" + cs
	}
	if extra != "" {
		label = label + " · " + extra
	}
	r.MixLabel = label
	r.Mode = label // by_mode / overlap / lean charts key on Mode
}

func parseMixTags(id string) (branch []string, pattern, camSync, extra string) {
	var extras []string
	for _, p := range strings.Split(id, "|") {
		switch {
		case strings.HasPrefix(p, "bm="):
			raw := strings.TrimPrefix(p, "bm=")
			for _, m := range strings.Split(raw, "+") {
				m = strings.TrimSpace(m)
				if m != "" {
					branch = append(branch, m)
				}
			}
		case strings.HasPrefix(p, "pat="):
			pattern = strings.TrimPrefix(p, "pat=")
		case strings.HasPrefix(p, "cs="):
			camSync = formatCSDisplay(strings.TrimPrefix(p, "cs="))
		case strings.HasPrefix(p, "ba="):
			extras = append(extras, "ba="+strings.TrimPrefix(p, "ba="))
		case strings.HasPrefix(p, "rot="):
			extras = append(extras, "rot="+strings.TrimPrefix(p, "rot="))
		case strings.HasPrefix(p, "dna="):
			extras = append(extras, "dna="+strings.TrimPrefix(p, "dna="))
		case strings.HasPrefix(p, "kit="):
			extras = append(extras, "kit="+strings.TrimPrefix(p, "kit="))
		}
	}
	extra = strings.Join(extras, " · ")
	return branch, pattern, camSync, extra
}

// formatCSDisplay turns "0.10" → "10%" for mix labels.
func formatCSDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var a float64
	if _, err := fmt.Sscanf(raw, "%f", &a); err != nil {
		return raw
	}
	pct := a * 100
	if pct == float64(int(pct)) {
		return fmt.Sprintf("%.0f%%", pct)
	}
	return fmt.Sprintf("%.1f%%", pct)
}

// denseScore ranks fast+small configs only if they actually learned (Acc weighted).
// AccPerSecPerMiB × (Acc/100)² — low-Acc flukes sink to the bottom.
func denseScore(r Row) float64 {
	if r.AccPerSecPerMiB <= 0 || r.Acc <= 0 {
		return 0
	}
	accW := r.Acc / 100.0
	return r.AccPerSecPerMiB * accW * accW
}

const minDenseAcc = 40.0 // ignore top-dense list below this hard Acc

func denseCandidates(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Acc < minDenseAcc || r.DenseScore <= 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

func denserThan(a, b Row) bool {
	if a.DenseScore != b.DenseScore {
		return a.DenseScore > b.DenseScore
	}
	if a.Acc != b.Acc {
		return a.Acc > b.Acc
	}
	if a.AccPerSecPerMiB != b.AccPerSecPerMiB {
		return a.AccPerSecPerMiB > b.AccPerSecPerMiB
	}
	if a.WeightBytes != b.WeightBytes && a.WeightBytes > 0 && b.WeightBytes > 0 {
		return a.WeightBytes < b.WeightBytes
	}
	return a.AccPerSec > b.AccPerSec
}

const leanPctOfBest = 0.95

func leanThreshold(rows []Row) (best Row, thr float64, ok bool) {
	if len(rows) == 0 {
		return Row{}, 0, false
	}
	best = rows[0]
	for _, r := range rows {
		if r.Acc > best.Acc {
			best = r
		}
	}
	if best.Acc <= 0 {
		return best, 0, false
	}
	return best, best.Acc * leanPctOfBest, true
}

func leanEligible(rows []Row, thr float64) []Row {
	cands := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Acc < thr || r.WeightBytes <= 0 || r.DurationSec <= 0 {
			continue
		}
		cands = append(cands, r)
	}
	return cands
}

func leanLess(a, b Row) bool {
	if a.WeightBytes != b.WeightBytes {
		return a.WeightBytes < b.WeightBytes
	}
	if a.DurationSec != b.DurationSec {
		return a.DurationSec < b.DurationSec
	}
	if a.Acc != b.Acc {
		return a.Acc > b.Acc
	}
	return a.ID < b.ID
}

func rowToLeanBar(rank int, r Row, bestAcc float64) LeanBar {
	label := r.Mode + " · " + r.DType
	if r.Arch != "" {
		label += " · " + r.Arch
	}
	pct := 0.0
	if bestAcc > 0 {
		pct = 100 * r.Acc / bestAcc
	}
	return LeanBar{
		Rank:        rank,
		Label:       label,
		Mode:        r.Mode,
		DType:       r.DType,
		Arch:        r.Arch,
		LRLabel:     r.LRLabel,
		Acc:         r.Acc,
		PctOfBest:   pct,
		WeightKiB:   r.WeightKiB,
		DurationSec: r.DurationSec,
		Throughput:  r.Throughput,
		AccPerSec:   r.AccPerSec,
		ID:          r.ID,
	}
}

// buildLeanBars: among cells with Acc ≥ 95% of the global best Acc, rank
// smallest weight RAM first, then fastest wall duration (time to that Acc).
func buildLeanBars(rows []Row, n int) (*LeanSummary, []LeanBar) {
	sum := &LeanSummary{PctOfBest: leanPctOfBest * 100}
	best, thr, ok := leanThreshold(rows)
	if !ok {
		return sum, nil
	}
	sum.BestAcc = best.Acc
	sum.BestID = best.ID
	sum.Threshold = thr

	cands := leanEligible(rows, thr)
	sum.Eligible = len(cands)
	sort.Slice(cands, func(i, j int) bool { return leanLess(cands[i], cands[j]) })
	if len(cands) > n {
		cands = cands[:n]
	}
	out := make([]LeanBar, 0, len(cands))
	for i, r := range cands {
		out = append(out, rowToLeanBar(i+1, r, sum.BestAcc))
	}
	if len(out) > 0 {
		w := out[0]
		sum.WinnerID = w.ID
		sum.WinnerKiB = w.WeightKiB
		sum.WinnerSec = w.DurationSec
		sum.WinnerAcc = w.Acc
	}
	return sum, out
}

// buildLeanByDtype: one lean champ per dtype (same ≥95% Acc rule), ordered
// by weight KiB ascending (smallest dtype footprint → largest).
func buildLeanByDtype(rows []Row) (*LeanSummary, []LeanBar) {
	sum := &LeanSummary{PctOfBest: leanPctOfBest * 100}
	best, thr, ok := leanThreshold(rows)
	if !ok {
		return sum, nil
	}
	sum.BestAcc = best.Acc
	sum.BestID = best.ID
	sum.Threshold = thr

	cands := leanEligible(rows, thr)
	sum.Eligible = len(cands)
	by := map[string]Row{}
	for _, r := range cands {
		prev, hit := by[r.DType]
		if !hit || leanLess(r, prev) {
			by[r.DType] = r
		}
	}
	outRows := make([]Row, 0, len(by))
	for _, r := range by {
		outRows = append(outRows, r)
	}
	sort.Slice(outRows, func(i, j int) bool {
		a, b := outRows[i], outRows[j]
		if a.WeightBytes != b.WeightBytes {
			return a.WeightBytes < b.WeightBytes
		}
		return a.DType < b.DType
	})
	out := make([]LeanBar, 0, len(outRows))
	for i, r := range outRows {
		bar := rowToLeanBar(i+1, r, sum.BestAcc)
		bar.Label = r.DType + " · " + r.Mode + " · " + r.Arch
		out = append(out, bar)
	}
	if len(out) > 0 {
		w := out[0]
		sum.WinnerID = w.ID
		sum.WinnerKiB = w.WeightKiB
		sum.WinnerSec = w.DurationSec
		sum.WinnerAcc = w.Acc
	}
	return sum, out
}

func denseBars(rows []Row) []DenseBar {
	out := make([]DenseBar, 0, len(rows))
	for _, r := range rows {
		label := r.Mode + " · " + r.DType
		if r.Arch != "" && r.Arch != "single" {
			label += " · " + r.Arch
		}
		if r.LRLabel != "" {
			label += " · lr=" + r.LRLabel
		}
		out = append(out, DenseBar{
			Label: label, Mode: r.Mode, DType: r.DType, Arch: r.Arch, LRLabel: r.LRLabel,
			AccPerSec: r.AccPerSec, WeightKiB: r.WeightKiB, Dense: r.AccPerSecPerMiB,
			Score: r.DenseScore, Acc: r.Acc, ID: r.ID,
		})
	}
	return out
}

func buildModeDtypeGrids(rows []Row) []ModeDtypeGrid {
	type cellKey struct{ lr, mode, dtype, arch string }
	type acc struct {
		lr                      float64
		n                       int
		sumAcc, sumThru, sumAPS float64
		bestAcc, bestThru       float64
	}
	byLR := map[string]map[cellKey]*acc{}
	lrOrder := []string{}
	lrSeen := map[string]bool{}
	for _, r := range rows {
		lr := r.LRLabel
		if lr == "" {
			lr = FormatLR(r.LR)
		}
		if !lrSeen[lr] {
			lrSeen[lr] = true
			lrOrder = append(lrOrder, lr)
		}
		if byLR[lr] == nil {
			byLR[lr] = map[cellKey]*acc{}
		}
		arch := r.Arch
		if arch == "" {
			arch = "single"
		}
		k := cellKey{lr, r.Mode, r.DType, arch}
		a := byLR[lr][k]
		if a == nil {
			a = &acc{lr: r.LR}
			byLR[lr][k] = a
		}
		a.n++
		a.sumAcc += r.Acc
		a.sumThru += r.Throughput
		a.sumAPS += r.AccPerSec
		if r.Acc > a.bestAcc {
			a.bestAcc = r.Acc
		}
		if r.Throughput > a.bestThru {
			a.bestThru = r.Throughput
		}
	}
	sort.Slice(lrOrder, func(i, j int) bool {
		ai, aj := parseLRFromLabel(lrOrder[i]), parseLRFromLabel(lrOrder[j])
		return ai < aj
	})
	out := make([]ModeDtypeGrid, 0, len(lrOrder))
	for _, lrLabel := range lrOrder {
		cells := byLR[lrLabel]
		dtypeSet := map[string]bool{}
		modeSet := map[string]bool{}
		archSet := map[string]bool{}
		var gridCells []ModeDtypeCell
		for k, a := range cells {
			dtypeSet[k.dtype] = true
			modeSet[k.mode] = true
			archSet[k.arch] = true
			n := float64(a.n)
			gridCells = append(gridCells, ModeDtypeCell{
				Mode: k.mode, DType: k.dtype, Arch: k.arch, N: a.n,
				MeanAcc: a.sumAcc / n, MeanThru: a.sumThru / n, MeanAccPS: a.sumAPS / n,
				BestAcc: a.bestAcc, BestThroughput: a.bestThru,
			})
		}
		dtypes := keysSorted(dtypeSet)
		modes := keysSorted(modeSet)
		arches := keysSorted(archSet)
		sort.Slice(gridCells, func(i, j int) bool {
			if gridCells[i].Arch != gridCells[j].Arch {
				return gridCells[i].Arch < gridCells[j].Arch
			}
			if gridCells[i].DType != gridCells[j].DType {
				return gridCells[i].DType < gridCells[j].DType
			}
			return gridCells[i].Mode < gridCells[j].Mode
		})
		lr := parseLRFromLabel(lrLabel)
		if lr == 0 && len(gridCells) > 0 {
			for _, c := range cells {
				if c.lr > 0 {
					lr = c.lr
					break
				}
			}
		}
		out = append(out, ModeDtypeGrid{
			LRLabel: lrLabel, LR: lr,
			DTypes: dtypes, Modes: modes, Arches: arches, Cells: gridCells,
		})
	}
	return out
}

func buildBestModeByDType(rows []Row) []BestModeByDTypeGrid {
	type key struct{ lr, dtype, arch string }
	best := map[key]Row{}
	lrOrder := []string{}
	lrSeen := map[string]bool{}
	for _, r := range rows {
		if r.Acc <= 0 {
			continue
		}
		lr := r.LRLabel
		if lr == "" {
			lr = FormatLR(r.LR)
		}
		if !lrSeen[lr] {
			lrSeen[lr] = true
			lrOrder = append(lrOrder, lr)
		}
		arch := r.Arch
		if arch == "" {
			arch = "single"
		}
		k := key{lr, r.DType, arch}
		cur, ok := best[k]
		if !ok || r.Acc > cur.Acc || (r.Acc == cur.Acc && r.Throughput > cur.Throughput) {
			best[k] = r
		}
	}
	sort.Slice(lrOrder, func(i, j int) bool {
		return parseLRFromLabel(lrOrder[i]) < parseLRFromLabel(lrOrder[j])
	})
	out := make([]BestModeByDTypeGrid, 0, len(lrOrder))
	for _, lrLabel := range lrOrder {
		var gridRows []BestModeByDTypeRow
		lrVal := parseLRFromLabel(lrLabel)
		for k, r := range best {
			if k.lr != lrLabel {
				continue
			}
			gridRows = append(gridRows, BestModeByDTypeRow{
				DType: k.dtype, Mode: r.Mode, Arch: k.arch, LRLabel: lrLabel,
				Acc: r.Acc, Throughput: r.Throughput, AccPerSec: r.AccPerSec, ID: r.ID,
			})
			if lrVal == 0 && r.LR > 0 {
				lrVal = r.LR
			}
		}
		sort.Slice(gridRows, func(i, j int) bool {
			if gridRows[i].DType != gridRows[j].DType {
				return gridRows[i].DType < gridRows[j].DType
			}
			return gridRows[i].Arch < gridRows[j].Arch
		})
		out = append(out, BestModeByDTypeGrid{LRLabel: lrLabel, LR: lrVal, Rows: gridRows})
	}
	return out
}

func keysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parseLRFromLabel(lab string) float64 {
	v, err := parseLRToken(lab)
	if err != nil {
		return 0
	}
	return v
}

func aggregate(rows []Row, key func(Row) string) []AggBar {
	type acc struct {
		n                       int
		sumAcc, sumThru, sumAPS float64
		bestAcc, bestThru       float64
	}
	type kk struct{ k, lr string }
	m := map[kk]*acc{}
	for _, r := range rows {
		k := kk{key(r), r.LRLabel}
		a := m[k]
		if a == nil {
			a = &acc{}
			m[k] = a
		}
		a.n++
		a.sumAcc += r.Acc
		a.sumThru += r.Throughput
		a.sumAPS += r.AccPerSec
		if r.Acc > a.bestAcc {
			a.bestAcc = r.Acc
		}
		if r.Throughput > a.bestThru {
			a.bestThru = r.Throughput
		}
	}
	out := make([]AggBar, 0, len(m))
	for k, a := range m {
		n := float64(a.n)
		out = append(out, AggBar{
			Key: k.k, LRLabel: k.lr, N: a.n,
			MeanAcc: a.sumAcc / n, MeanThru: a.sumThru / n, MeanAccPS: a.sumAPS / n,
			BestAcc: a.bestAcc, BestThru: a.bestThru,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MeanAcc != out[j].MeanAcc {
			return out[i].MeanAcc > out[j].MeanAcc
		}
		return out[i].Key+"|"+out[i].LRLabel < out[j].Key+"|"+out[j].LRLabel
	})
	return out
}

func overlapByMode(rows []Row) []OverlapSeries {
	// Per mode×LR keep only the highest-Acc cell (its dtype/arch/id).
	type ptKey struct{ mode, lr string }
	best := map[ptKey]Row{}
	modes := map[string]bool{}
	for _, r := range rows {
		modes[r.Mode] = true
		k := ptKey{r.Mode, r.LRLabel}
		cur, ok := best[k]
		if !ok || r.Acc > cur.Acc || (r.Acc == cur.Acc && r.Throughput > cur.Throughput) {
			best[k] = r
		}
	}
	var modeList []string
	for mode := range modes {
		modeList = append(modeList, mode)
	}
	sort.Strings(modeList)
	out := make([]OverlapSeries, 0, len(modeList))
	for _, mode := range modeList {
		var pts []OverlapPoint
		for k, r := range best {
			if k.mode != mode {
				continue
			}
			pts = append(pts, OverlapPoint{
				LR: r.LR, LRLabel: r.LRLabel,
				Acc: r.Acc, Throughput: r.Throughput, AccPerSec: r.AccPerSec,
				DType: r.DType, Arch: r.Arch, ID: r.ID,
			})
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].LR < pts[j].LR })
		out = append(out, OverlapSeries{Mode: mode, Points: pts})
	}
	return out
}

func topN(rows []Row, n int, less func(a, b Row) bool) []Row {
	cp := append([]Row(nil), rows...)
	sort.Slice(cp, func(i, j int) bool { return less(cp[i], cp[j]) })
	if len(cp) > n {
		cp = cp[:n]
	}
	return cp
}
