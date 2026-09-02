package river

import (
	"sort"
	"strconv"
	"strings"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/layers/parallel"
	"github.com/openfluke/welvet/lucy"
)

// Credit kinds for the Acc-keep band. Chain modes use the real chain-rule
// update (Backward + SGD) — same path as BP — so they are not "non-BP".
const (
	creditBP    = "bp"     // NormalBP / StepBP / MeshBP (sgd, step_sgd, …)
	creditChain = "chain"  // TweenChain family — named Tween, credit = BP/SGD
	creditNonBP = "non_bp" // Tween / Split / Alt / proxies / Sparse / Linear / …
)

const chainCreditNote = "Chain (tween_chain / MeshTweenChain / …) uses chain-rule Backward+SGD — same update path as BP, not non-BP credit."

// NearPayload is the Acc-keep band page: everything from 100% of best Acc
// down to min_keep (default 70% — same floor as Lucy LPD).
type NearPayload struct {
	MinKeep      float64            `json:"min_keep"`   // Acc / Acc-champ floor (0.70)
	BestAcc      float64            `json:"best_acc"`
	BestID       string             `json:"best_id"`
	Threshold    float64            `json:"threshold"` // min_keep × best_acc
	NTotal       int                `json:"n_total"`
	NBand        int                `json:"n_band"`
	Formula      string             `json:"formula"`
	Rows         []NearRow          `json:"rows"`
	Bands        map[string]int     `json:"bands"` // gold/near/keep/… counts in the band
	Credits      map[string]int     `json:"credits"` // bp/chain/non_bp counts in the band
	BestNonBP    *NearHighlight     `json:"best_non_bp,omitempty"`
	BestChain    *NearHighlight     `json:"best_chain,omitempty"`
	ChainNote    string             `json:"chain_note"`
}

// NearHighlight is a callout champ (Acc / non-BP / Chain).
type NearHighlight struct {
	ID        string  `json:"id"`
	Mode      string  `json:"mode"`
	DType     string  `json:"dtype"`
	Arch      string  `json:"arch"`
	LRLabel   string  `json:"lr_label"`
	Acc       float64 `json:"acc"`
	PctOfBest float64 `json:"pct_of_best"`
	Credit    string  `json:"credit"`
	InBand    bool    `json:"in_band"`
	Note      string  `json:"note,omitempty"`
}

// NearRow is one cell inside the Acc-keep band with Lucy + train stats.
type NearRow struct {
	Rank         int      `json:"rank"`
	ID           string   `json:"id"`
	Mode         string   `json:"mode"` // mix label when BranchModes set
	BranchModes  []string `json:"branch_modes,omitempty"`
	MixPattern   string   `json:"mix_pattern,omitempty"`
	DType        string   `json:"dtype"`
	Format       string   `json:"format"`
	Arch         string   `json:"arch"`
	LRLabel      string   `json:"lr_label"`
	Credit       string   `json:"credit"` // bp | chain | non_bp | mix
	Acc          float64  `json:"acc"`
	SoftAcc      float64  `json:"soft_acc"`
	PctOfBest    float64  `json:"pct_of_best"` // Acc keep % (100 = matches champ)
	Throughput   float64  `json:"throughput"`
	Availability float64  `json:"availability"`
	Score        float64  `json:"score"` // Thru × Avail × Acc / 10_000
	LPD          float64  `json:"lpd"`
	Q            float64  `json:"q"`
	Shrink       float64  `json:"shrink"`
	RAMKiB       float64  `json:"ram_kib"`
	RAMFrac      float64  `json:"ram_frac"`
	DurationSec  float64  `json:"duration_sec"`
	AccPerSec    float64  `json:"acc_per_sec"`
	DenseScore   float64  `json:"dense_score"`
	TimeTo50Sec  float64  `json:"time_to_50_sec"`
	Band         string   `json:"band"`
	Pillars      int      `json:"pillars"`
	RelThru      float64  `json:"rel_thru"`
	RelAvail     float64  `json:"rel_avail"`
	Status       string   `json:"status,omitempty"`
	BestNonBP    bool     `json:"best_non_bp,omitempty"`
}

func buildNear(f File, minKeep float64) NearPayload {
	if minKeep <= 0 || minKeep > 1 {
		minKeep = lucy.LPDKeepFloor
	}
	out := NearPayload{
		MinKeep:   minKeep,
		Formula:   lucy.DensityFormula(),
		Bands:     map[string]int{},
		Credits:   map[string]int{},
		ChainNote: chainCreditNote,
	}
	rows := make([]Row, len(f.Rows))
	copy(rows, f.Rows)
	for i := range rows {
		enrichRow(&rows[i])
	}
	out.NTotal = len(rows)
	if len(rows) == 0 {
		return out
	}

	byID := make(map[string]Row, len(rows))
	samples := make([]lucy.Sample, 0, len(rows))
	for _, r := range rows {
		if r.Status != "" && r.Status != "ok" && r.Status != "gap" {
			continue
		}
		byID[r.ID] = r
		samples = append(samples, rowToLucySample(r))
	}
	lpd := lucy.BuildLPD(samples)
	out.BestAcc = lpd.PeakAcc
	out.BestID = lpd.AccChamp.ID
	out.Threshold = out.BestAcc * minKeep

	pool := lpd.Pool
	if len(pool) == 0 {
		pool = lpd.Top
	}
	band := make([]NearRow, 0, len(pool))
	for _, r := range pool {
		if r.RelAcc < minKeep {
			continue
		}
		src := byID[r.ID]
		cred := creditOfRow(src)
		modeLabel := src.Mode
		if modeLabel == "" {
			modeLabel = r.Mode
		}
		band = append(band, NearRow{
			ID:           r.ID,
			Mode:         modeLabel,
			BranchModes:  append([]string(nil), src.BranchModes...),
			MixPattern:   src.MixPattern,
			DType:        r.DType,
			Format:       firstNonEmpty(r.Format, src.Format),
			Arch:         r.Arch,
			LRLabel:      src.LRLabel,
			Credit:       cred,
			Acc:          r.Acc,
			SoftAcc:      r.Soft,
			PctOfBest:    r.RelAcc * 100,
			Throughput:   r.Thru,
			Availability: r.Avail,
			Score:        r.Score,
			LPD:          r.LPD,
			Q:            r.Q,
			Shrink:       r.Shrink,
			RAMKiB:       r.RAMKiB,
			RAMFrac:      r.RAMFrac,
			DurationSec:  src.DurationSec,
			AccPerSec:    src.AccPerSec,
			DenseScore:   src.DenseScore,
			TimeTo50Sec:  src.TimeTo50Sec,
			Band:         r.Band,
			Pillars:      r.Pillars,
			RelThru:      r.RelThru,
			RelAvail:     r.RelAvail,
			Status:       src.Status,
		})
		out.Bands[r.Band]++
		out.Credits[cred]++
	}
	// Acc keep high → low; within that, LPD high → low; then smaller RAM.
	sort.SliceStable(band, func(i, j int) bool {
		a, b := band[i], band[j]
		if a.PctOfBest != b.PctOfBest {
			return a.PctOfBest > b.PctOfBest
		}
		if a.LPD != b.LPD {
			return a.LPD > b.LPD
		}
		if a.RAMKiB != b.RAMKiB {
			return a.RAMKiB < b.RAMKiB
		}
		return a.ID < b.ID
	})
	for i := range band {
		band[i].Rank = i + 1
	}

	out.BestNonBP = pickBestCredit(rows, out.BestAcc, out.Threshold, creditNonBP, "")
	out.BestChain = pickBestCredit(rows, out.BestAcc, out.Threshold, creditChain, chainCreditNote)
	if out.BestNonBP != nil {
		for i := range band {
			if band[i].ID == out.BestNonBP.ID {
				band[i].BestNonBP = true
				break
			}
		}
	}

	out.NBand = len(band)
	out.Rows = band
	return out
}

// creditOfRow classifies mix cells from BranchModes; uniform cells use Mode.
func creditOfRow(r Row) string {
	if len(r.BranchModes) > 0 {
		return creditOfBranchModes(r.BranchModes)
	}
	return creditOfMode(r.Mode)
}

func creditOfBranchModes(modes []string) string {
	seen := map[string]bool{}
	for _, m := range modes {
		seen[creditOfMode(m)] = true
	}
	if len(seen) > 1 {
		return "mix"
	}
	for k := range seen {
		return k
	}
	return creditBP
}

// creditOfMode classifies a results.json mode token.
// Chain is called out separately because it falls back onto the BP/SGD path.
func creditOfMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return creditBP
	}
	wm, err := permute.TrainMode(mode).Welvet()
	if err != nil {
		low := strings.ToLower(strings.ReplaceAll(mode, "_", ""))
		if strings.Contains(low, "chain") {
			return creditChain
		}
		if low == "sgd" || low == "stepsgd" || strings.Contains(low, "bp") {
			return creditBP
		}
		return creditNonBP
	}
	return creditFromWelvet(wm)
}

func creditFromWelvet(wm parallel.TrainMode) string {
	if wm.UseChainRule() {
		return creditChain
	}
	switch wm {
	case parallel.ModeNormalBP, parallel.ModeStepBP, parallel.ModeMeshBP:
		return creditBP
	case parallel.ModeTween, parallel.ModeStepTween, parallel.ModeMeshTween,
		parallel.ModeTweenSplit, parallel.ModeStepTweenSplit, parallel.ModeMeshTweenSplit,
		parallel.ModeTweenAlt, parallel.ModeStepTweenAlt, parallel.ModeMeshTweenAlt,
		parallel.ModeTweenSplitHeadProxy, parallel.ModeTweenSplitLinear,
		parallel.ModeTweenSplitFastProxy, parallel.ModeTweenSplitLinearCache,
		parallel.ModeTweenSplitHeadProxyAsync, parallel.ModeTweenSplitSparse,
		parallel.ModeMeshTweenSplitFastProxy, parallel.ModeMeshTweenSplitSparse,
		parallel.ModeStepTweenSplitHeadProxy, parallel.ModeStepTweenSplitLinear,
		parallel.ModeStepTweenSplitFastProxy, parallel.ModeStepTweenSplitLinearCache,
		parallel.ModeStepTweenSplitHeadProxyAsync, parallel.ModeStepTweenSplitSparse:
		return creditNonBP
	case parallel.ModeTweenChain, parallel.ModeStepTweenChain, parallel.ModeMeshTweenChain:
		return creditChain
	default:
		return creditBP
	}
}

func pickBestCredit(rows []Row, bestAcc, thr float64, want, note string) *NearHighlight {
	var best *NearHighlight
	for _, r := range rows {
		if r.Status != "" && r.Status != "ok" && r.Status != "gap" {
			continue
		}
		if creditOfRow(r) != want {
			continue
		}
		pct := 0.0
		if bestAcc > 0 {
			pct = 100 * r.Acc / bestAcc
		}
		h := &NearHighlight{
			ID: r.ID, Mode: r.Mode, DType: r.DType, Arch: r.Arch, LRLabel: r.LRLabel,
			Acc: r.Acc, PctOfBest: pct, Credit: want, InBand: thr > 0 && r.Acc >= thr, Note: note,
		}
		if best == nil || h.Acc > best.Acc || (h.Acc == best.Acc && h.ID < best.ID) {
			best = h
		}
	}
	return best
}

func rowToLucySample(r Row) lucy.Sample {
	score := r.Throughput * r.Availability * r.Acc / 10000
	kib := r.WeightKiB
	if kib <= 0 && r.WeightBytes > 0 {
		kib = float64(r.WeightBytes) / 1024
	}
	return lucy.Sample{
		ID:     r.ID,
		Mode:   r.Mode,
		DType:  r.DType,
		Format: r.Format,
		Arch:   r.Arch,
		Score:  score,
		Soft:   r.SoftAcc,
		Acc:    r.Acc,
		Thru:   r.Throughput,
		Avail:  r.Availability,
		RAMKiB: kib,
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parseMinKeep(s string) float64 {
	if s == "" {
		return lucy.LPDKeepFloor
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return lucy.LPDKeepFloor
	}
	// Accept 70 or 0.70
	if v > 1 {
		v = v / 100
	}
	if v <= 0 || v > 1 {
		return lucy.LPDKeepFloor
	}
	return v
}
