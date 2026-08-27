package report

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"
)

// CompareReport is a cross-machine, cross-LR pivot built from full tide /api/report archives.
type CompareReport struct {
	Generated time.Time      `json:"generated"`
	DataRev   uint64         `json:"data_rev"`
	Title     string         `json:"title"`
	Machines  []string       `json:"machines"`
	Peers     []ComparePeerMeta `json:"peers,omitempty"`
	CamPairs  []CompareCamPair  `json:"cam_pairs,omitempty"`
	LRs       []float64      `json:"lrs"`
	LRLabels  []string       `json:"lr_labels"`
	Summary   []CompareLRRow `json:"summary"`
	ModeLR    []CompareModeLRGrid `json:"mode_lr"`
	Matched   []CompareMatchRow   `json:"matched_top"`
	Wins      []CompareWinRow     `json:"wins_by_lr"`
	LPDTop    []CompareLPDEntry   `json:"lpd_top"`
	LPDPerMachine []CompareLPDGroup `json:"lpd_per_machine"`
	TrapTop   []CompareTrapEntry  `json:"trap_top"`
	TrapRate  []CompareTrapRateRow `json:"trap_rate"`
	SoftGap   []CompareSoftGapRow  `json:"soft_gap"`
	Scatter   []CompareScatterPoint `json:"scatter"`
	ModeBars      []CompareModeBar          `json:"mode_bars"`
	ModeSeries    []CompareModeSeries       `json:"mode_series"`
	ModeCross     []CompareModeCross        `json:"mode_cross"`
	VsBaseline    []CompareVsBaselineGrid   `json:"vs_baseline"`
	StepFamilies  []CompareStepFamilyGrid   `json:"step_families"`
	StepCross     []CompareStepCross        `json:"step_cross"`
	StepVerdicts  []CompareStepMachineVerdict `json:"step_verdicts"`
	StepByLR      []CompareStepLRRow        `json:"step_by_lr"`
	OverlapLRs    []string                  `json:"overlap_lrs,omitempty"`
	OverlapNote   string                    `json:"overlap_note,omitempty"`
	LRByMachine   map[string][]string       `json:"lr_by_machine,omitempty"`
}

// CompareLRRow is mean metrics for one machine at one LR.
type CompareLRRow struct {
	Machine   string  `json:"machine"`
	LR        float64 `json:"lr"`
	LRLabel   string  `json:"lr_label"`
	N         int     `json:"n"`
	MeanAcc   float64 `json:"mean_acc"`
	MeanSoft  float64 `json:"mean_soft"`
	MeanScore float64 `json:"mean_score"`
	MeanAvail float64 `json:"mean_avail"`
	BestMode  string  `json:"best_mode,omitempty"`
	BestAcc   float64 `json:"best_acc"`
}

// CompareModeLRGrid is mode × LR mean metrics for one machine (heatmap rows).
type CompareModeLRGrid struct {
	Machine  string      `json:"machine"`
	Modes    []string    `json:"modes"`
	LRLabels []string    `json:"lr_labels"`
	Acc      [][]float64 `json:"acc"`
	Score    [][]float64 `json:"score"`
	Avail    [][]float64 `json:"avail"`
	Soft     [][]float64 `json:"soft"`
}

// CompareModeSeriesPoint is one LR step for a train mode.
type CompareModeSeriesPoint struct {
	LR      float64 `json:"lr"`
	LRLabel string  `json:"lr_label"`
	N       int     `json:"n"`
	Acc     float64 `json:"acc"`
	Soft    float64 `json:"soft_acc"`
	Score   float64 `json:"score"`
	Avail   float64 `json:"avail"`
}

// CompareModeSeries is mean metrics for one mode across LR on one machine.
type CompareModeSeries struct {
	Machine string                   `json:"machine"`
	Mode    string                   `json:"mode"`
	Points  []CompareModeSeriesPoint `json:"points"`
}

// CompareModeCrossPoint is one machine at one LR for a shared mode comparison.
type CompareModeCrossPoint struct {
	Machine string  `json:"machine"`
	LR      float64 `json:"lr"`
	LRLabel string  `json:"lr_label"`
	N       int     `json:"n"`
	Acc     float64 `json:"acc"`
	Soft    float64 `json:"soft_acc"`
	Score   float64 `json:"score"`
	Avail   float64 `json:"avail"`
}

// CompareModeCross compares one train mode across machines as LR changes.
type CompareModeCross struct {
	Mode   string                  `json:"mode"`
	Points []CompareModeCrossPoint `json:"points"`
}

// CompareVsBaselineGrid is matched Acc/Score/Avail deltas vs baseline (sgd) per mode × LR.
type CompareVsBaselineGrid struct {
	Machine       string      `json:"machine"`
	Baseline      string      `json:"baseline"`
	LRLabels      []string    `json:"lr_labels"`
	LRs           []float64   `json:"lrs"`
	Modes         []string    `json:"modes"`
	AccDelta      [][]float64 `json:"acc_delta"`
	ScoreDelta    [][]float64 `json:"score_delta"`
	AvailDelta    [][]float64 `json:"avail_delta"`
	BaselineAcc   []float64   `json:"baseline_acc"`
	BaselineAvail []float64   `json:"baseline_avail"`
}

// CompareMatchRow is one recipe at one LR with per-machine Acc and delta (A−B).
type CompareMatchRow struct {
	Recipe   string             `json:"recipe"`
	LRLabel  string             `json:"lr_label"`
	LR       float64            `json:"lr"`
	ByMachine map[string]float64 `json:"by_machine"`
	Delta    float64            `json:"delta,omitempty"`
	Winner   string             `json:"winner,omitempty"`
}

// CompareWinRow counts recipe wins per machine at each LR.
type CompareWinRow struct {
	LRLabel string         `json:"lr_label"`
	LR      float64        `json:"lr"`
	Wins    map[string]int `json:"wins"`
}

// CompareLPDEntry is one ranked LPD row tagged with machine + LR.
type CompareLPDEntry struct {
	Machine  string  `json:"machine"`
	LRLabel  string  `json:"lr_label"`
	Rank     int     `json:"rank"`
	ID       string  `json:"id"`
	Mode     string  `json:"mode"`
	DType    string  `json:"dtype"`
	LPD      float64 `json:"lpd"`
	Acc      float64 `json:"acc"`
	Score    float64 `json:"score"`
	Avail    float64 `json:"avail"`
	RAMKiB   float64 `json:"ram_kib"`
	Band     string  `json:"band"`
}

// CompareLPDGroup is top LPD rows for one machine.
type CompareLPDGroup struct {
	Machine string            `json:"machine"`
	Rows    []CompareLPDEntry `json:"rows"`
}

// CompareTrapEntry is a trap or soft-gap false positive (high serve confidence, low Acc keep).
type CompareTrapEntry struct {
	Machine  string  `json:"machine"`
	LRLabel  string  `json:"lr_label"`
	Rank     int     `json:"rank"`
	ID       string  `json:"id"`
	Mode     string  `json:"mode"`
	DType    string  `json:"dtype"`
	Band     string  `json:"band"`
	Acc      float64 `json:"acc"`
	Soft     float64 `json:"soft_acc"`
	SoftGap  float64 `json:"soft_gap"`
	Score    float64 `json:"score"`
	Thru     float64 `json:"throughput"`
	Avail    float64 `json:"availability"`
	RAMKiB   float64 `json:"ram_kib"`
	RelAcc   float64 `json:"rel_acc"`
	RelFast  float64 `json:"rel_fast"`
	RelDuty  float64 `json:"rel_duty"`
}

// CompareTrapRateRow is trap fraction at one machine × LR.
type CompareTrapRateRow struct {
	Machine  string  `json:"machine"`
	LR       float64 `json:"lr"`
	LRLabel  string  `json:"lr_label"`
	N        int     `json:"n"`
	TrapN    int     `json:"trap_n"`
	TrapPct  float64 `json:"trap_pct"`
	SoftGapN int     `json:"soft_gap_n"`
}

// CompareSoftGapRow is mean Soft−Acc per machine × LR (serve overconfidence).
type CompareSoftGapRow struct {
	Machine   string  `json:"machine"`
	LR        float64 `json:"lr"`
	LRLabel   string  `json:"lr_label"`
	MeanGap   float64 `json:"mean_gap"`
	MaxGap    float64 `json:"max_gap"`
	N         int     `json:"n"`
}

// CompareScatterPoint is one cell for Acc×Avail / Soft×Acc scatter overlays.
type CompareScatterPoint struct {
	Machine  string  `json:"machine"`
	LRLabel  string  `json:"lr_label"`
	ID       string  `json:"id,omitempty"`
	Acc      float64 `json:"acc"`
	Soft     float64 `json:"soft_acc"`
	Avail    float64 `json:"availability"`
	Score    float64 `json:"score"`
	Band     string  `json:"band,omitempty"`
}

// CompareModeBar is mean Acc for one mode on one machine (all LRs pooled).
type CompareModeBar struct {
	Mode    string             `json:"mode"`
	ByMachine map[string]float64 `json:"by_machine"`
}

type taggedPoint struct {
	Machine string
	LR      float64
	LRLabel string
	CellPoint
}

// NamedTideReport pairs a peer label with its full /api/report archive.
type NamedTideReport struct {
	Name   string
	Report TideReport
}

// BuildCompare merges full cell archives from named tide reports.
func BuildCompare(title string, tides []NamedTideReport) CompareReport {
	out := CompareReport{
		Generated: time.Now(),
		Title:     title,
	}
	var pts []taggedPoint
	machineSet := map[string]bool{}
	lrMap := map[float64]string{}

	for _, tr := range tides {
		machine := ParseMachineFromPeer(tr.Name)
		if tr.Report.ID != "" && machine == tr.Name {
			if m := ParseMachineFromPeer(tr.Report.ID); m != "?" {
				machine = m
			}
		}
		machineSet[machine] = true
		for _, c := range tr.Report.Cells {
			if c.Score <= 0 && c.Acc <= 0 {
				continue
			}
			lr, lbl, ok := ParseLRFromCellID(c.ID)
			if !ok {
				continue
			}
			lrMap[lr] = lbl
			pt := c
			pt.Tide = tr.Name
			if pt.Layer == "" {
				parts := strings.SplitN(RecipeKey(c.ID), "|", 2)
				if len(parts) > 0 {
					pt.Layer = parts[0]
				}
			}
			pts = append(pts, taggedPoint{Machine: machine, LR: lr, LRLabel: lbl, CellPoint: pt})
		}
	}
	if len(pts) == 0 {
		return out
	}
	out.Machines = sortedKeys(machineSet)
	out.Machines = SortCamMachines(out.Machines)
	out.Peers = peerMetas(tides)
	out.LRs, out.LRLabels = sortedLRs(lrMap)
	out.LRByMachine = lrBandsByMachine(pts, out.Machines)
	out.OverlapLRs, out.OverlapNote = compareOverlap(pts, out.Machines, lrMap)
	out.Summary = compareSummaries(pts, out.Machines, out.LRs, lrMap)
	out.ModeLR = compareModeLR(pts, out.Machines)
	out.Matched = compareMatchedTop(pts, out.Machines, lrMap, 50)
	out.CamPairs = compareCamPairs(pts, tides, out.Machines, lrMap, 40)
	out.Wins = compareWins(pts, out.Machines, out.LRs, lrMap)
	out.LPDTop = compareLPDTop(pts, 50)
	out.LPDPerMachine = compareLPDPerMachine(pts, 20)
	out.TrapTop = compareTrapsTop(pts, 50)
	out.TrapRate = compareTrapRate(pts, out.Machines, lrMap)
	out.SoftGap = compareSoftGap(pts, out.Machines, lrMap)
	out.Scatter = compareScatter(pts, 1200)
	out.ModeBars = compareModeBars(pts, out.Machines, 24)
	out.ModeSeries = compareModeSeries(pts, out.Machines, 10)
	out.ModeCross = compareModeCross(pts, out.Machines, 8)
	out.VsBaseline = compareVsBaselineLR(pts, out.Machines, 12)
	out.StepFamilies = compareStepFamilies(pts, out.Machines)
	out.StepCross = compareStepCross(out.StepFamilies)
	out.StepVerdicts, out.StepByLR = compareStepVerdicts(out.StepFamilies)
	out.DataRev = compareDataRev(out)
	return out
}

// compareDataRev fingerprints report payload (excluding Generated/DataRev) so clients can skip redraws.
func compareDataRev(c CompareReport) uint64 {
	c.Generated = time.Time{}
	c.DataRev = 0
	b, err := json.Marshal(c)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedLRs(lrMap map[float64]string) ([]float64, []string) {
	lrs := make([]float64, 0, len(lrMap))
	for v := range lrMap {
		lrs = append(lrs, v)
	}
	sort.Float64s(lrs)
	labels := make([]string, len(lrs))
	for i, v := range lrs {
		if lbl := lrMap[v]; lbl != "" {
			labels[i] = lbl
		} else {
			labels[i] = FormatLR(v)
		}
	}
	return lrs, labels
}

func compareSummaries(pts []taggedPoint, machines []string, lrs []float64, lrMap map[float64]string) []CompareLRRow {
	type acc struct{ n int; acc, soft, score, avail float64; bestMode string; bestAcc float64 }
	by := map[string]*acc{}
	key := func(m string, lr float64) string { return m + "\x00" + FormatLR(lr) }
	for _, p := range pts {
		k := key(p.Machine, p.LR)
		a := by[k]
		if a == nil {
			a = &acc{}
			by[k] = a
		}
		a.n++
		a.acc += p.Acc
		a.soft += p.Soft
		a.score += p.Score
		a.avail += p.Avail
		if p.Acc > a.bestAcc {
			a.bestAcc = p.Acc
			a.bestMode = p.Mode
		}
	}
	var out []CompareLRRow
	for _, m := range machines {
		for _, lr := range lrs {
			a := by[key(m, lr)]
			if a == nil || a.n == 0 {
				continue
			}
			lbl := lrMap[lr]
			if lbl == "" {
				lbl = FormatLR(lr)
			}
			n := float64(a.n)
			out = append(out, CompareLRRow{
				Machine: m, LR: lr, LRLabel: lbl, N: a.n,
				MeanAcc: a.acc / n, MeanSoft: a.soft / n, MeanScore: a.score / n, MeanAvail: a.avail / n,
				BestMode: PrettyMode(a.bestMode), BestAcc: a.bestAcc,
			})
		}
	}
	return out
}

func compareModeLR(pts []taggedPoint, machines []string) []CompareModeLRGrid {
	var out []CompareModeLRGrid
	for _, m := range machines {
		lrs, lrLabels := lrsForMachine(pts, m)
		if len(lrs) == 0 {
			continue
		}
		modeSet := map[string]bool{}
		type cell struct{ n int; acc, score, avail, soft float64 }
		by := map[string]map[float64]*cell{}
		for _, p := range pts {
			if p.Machine != m {
				continue
			}
			mode := PrettyMode(p.Mode)
			modeSet[mode] = true
			row := by[mode]
			if row == nil {
				row = map[float64]*cell{}
				by[mode] = row
			}
			c := row[p.LR]
			if c == nil {
				c = &cell{}
				row[p.LR] = c
			}
			c.n++
			c.acc += p.Acc
			c.score += p.Score
			c.avail += p.Avail
			c.soft += p.Soft
		}
		modes := sortedKeys(modeSet)
		accGrid := make([][]float64, len(modes))
		scoreGrid := make([][]float64, len(modes))
		availGrid := make([][]float64, len(modes))
		softGrid := make([][]float64, len(modes))
		for i, mode := range modes {
			accGrid[i] = make([]float64, len(lrs))
			scoreGrid[i] = make([]float64, len(lrs))
			availGrid[i] = make([]float64, len(lrs))
			softGrid[i] = make([]float64, len(lrs))
			for j, lr := range lrs {
				if c := by[mode][lr]; c != nil && c.n > 0 {
					n := float64(c.n)
					accGrid[i][j] = c.acc / n
					scoreGrid[i][j] = c.score / n
					availGrid[i][j] = c.avail / n
					softGrid[i][j] = c.soft / n
				}
			}
		}
		out = append(out, CompareModeLRGrid{
			Machine: m, Modes: modes, LRLabels: lrLabels,
			Acc: accGrid, Score: scoreGrid, Avail: availGrid, Soft: softGrid,
		})
	}
	return out
}

func lrsForMachine(pts []taggedPoint, machine string) ([]float64, []string) {
	lrMap := map[float64]string{}
	for _, p := range pts {
		if p.Machine == machine {
			if p.LRLabel != "" {
				lrMap[p.LR] = p.LRLabel
			} else {
				lrMap[p.LR] = FormatLR(p.LR)
			}
		}
	}
	return sortedLRs(lrMap)
}

func lrBandsByMachine(pts []taggedPoint, machines []string) map[string][]string {
	out := map[string][]string{}
	for _, m := range machines {
		_, labels := lrsForMachine(pts, m)
		if len(labels) > 0 {
			out[m] = labels
		}
	}
	return out
}

func compareOverlap(pts []taggedPoint, machines []string, lrMap map[float64]string) ([]string, string) {
	if len(machines) < 2 {
		return nil, ""
	}
	has := map[float64]map[string]bool{}
	for _, p := range pts {
		row := has[p.LR]
		if row == nil {
			row = map[string]bool{}
			has[p.LR] = row
		}
		row[p.Machine] = true
	}
	var overlap []string
	for _, lr := range sortedLRKeys(has) {
		ok := true
		for _, m := range machines {
			if !has[lr][m] {
				ok = false
				break
			}
		}
		if ok {
			lbl := lrMap[lr]
			if lbl == "" {
				lbl = FormatLR(lr)
			}
			overlap = append(overlap, lbl)
		}
	}
	note := ""
	if len(overlap) == 0 && len(machines) >= 2 {
		var bands []string
		for _, m := range machines {
			var lrs []string
			for lr := range lrMap {
				row := has[lr]
				if row != nil && row[m] {
					lbl := lrMap[lr]
					if lbl == "" {
						lbl = FormatLR(lr)
					}
					lrs = append(lrs, lbl)
				}
			}
			sort.Strings(lrs)
			if len(lrs) > 0 {
				bands = append(bands, m+": "+strings.Join(lrs, ", "))
			}
		}
		note = "No shared LR steps — matched recipe deltas need the exact same |lr=… on every peer (same farm: both lo or both hi)."
		if len(bands) > 0 {
			note += " You have: " + strings.Join(bands, " · ") + "."
		}
	}
	return overlap, note
}

func sortedLRKeys(has map[float64]map[string]bool) []float64 {
	out := make([]float64, 0, len(has))
	for v := range has {
		out = append(out, v)
	}
	sort.Float64s(out)
	return out
}

func compareMatchedTop(pts []taggedPoint, machines []string, lrMap map[float64]string, topN int) []CompareMatchRow {
	type key struct {
		recipe, lbl string
		lr          float64
	}
	by := map[key]map[string]float64{}
	for _, p := range pts {
		k := key{recipe: RecipeKey(p.ID), lr: p.LR, lbl: p.LRLabel}
		if k.lbl == "" {
			k.lbl = lrMap[p.LR]
		}
		m := by[k]
		if m == nil {
			m = map[string]float64{}
			by[k] = m
		}
		if p.Acc > m[p.Machine] {
			m[p.Machine] = p.Acc
		}
	}
	var rows []CompareMatchRow
	for k, vals := range by {
		var present []string
		for _, m := range machines {
			if vals[m] > 0 {
				present = append(present, m)
			}
		}
		if len(present) < 2 {
			continue
		}
		sort.Strings(present)
		row := CompareMatchRow{Recipe: k.recipe, LR: k.lr, LRLabel: k.lbl, ByMachine: vals}
		a, b := present[0], present[len(present)-1]
		va, vb := vals[a], vals[b]
		row.Delta = va - vb
		if math.Abs(va-vb) < 0.05 {
			row.Winner = "tie"
		} else if va > vb {
			row.Winner = a
		} else {
			row.Winner = b
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LR != rows[j].LR {
			return rows[i].LR < rows[j].LR
		}
		return math.Abs(rows[i].Delta) > math.Abs(rows[j].Delta)
	})
	if topN > 0 && len(rows) > topN {
		rows = rows[:topN]
	}
	return rows
}

func compareLPDPerMachine(pts []taggedPoint, topN int) []CompareLPDGroup {
	by := groupByMachine(pts)
	machines := make([]string, 0, len(by))
	for m := range by {
		machines = append(machines, m)
	}
	sort.Strings(machines)
	var out []CompareLPDGroup
	for _, m := range machines {
		rows := compareLPDTop(by[m], topN)
		for i := range rows {
			rows[i].Rank = i + 1
			rows[i].Machine = m
		}
		out = append(out, CompareLPDGroup{Machine: m, Rows: rows})
	}
	return out
}

func compareTrapsTop(pts []taggedPoint, topN int) []CompareTrapEntry {
	lrByID := lrLabelsOf(pts)
	seen := map[string]bool{}
	var all []CompareTrapEntry
	addRow := func(r LPDRow, machine, lbl, band string) {
		id := PrettyCell(r.ID)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		all = append(all, CompareTrapEntry{
			Machine: machine, LRLabel: lbl, ID: id, Mode: PrettyMode(r.Mode), DType: r.DType, Band: band,
			Acc: r.Acc, Soft: r.Soft, SoftGap: r.Soft - r.Acc, Score: r.Score, Thru: r.Thru, Avail: r.Avail,
			RAMKiB: r.RAMKiB, RelAcc: r.RelAcc, RelFast: r.RelFast, RelDuty: r.RelDuty,
		})
	}
	for machine, mpts := range groupByMachine(pts) {
		lpd := BuildLPD(taggedToCells(mpts))
		for _, r := range lpd.Trap {
			lbl := lrByID[r.ID]
			if lbl == "" {
				if _, l2, ok := ParseLRFromCellID(r.ID); ok {
					lbl = l2
				}
			}
			addRow(r, machine, lbl, "trap")
		}
		for _, p := range mpts {
			gap := p.Soft - p.Acc
			if gap < 10 || p.Acc >= 55 || p.Score <= 0 {
				continue
			}
			id := PrettyCell(p.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			all = append(all, CompareTrapEntry{
				Machine: machine, LRLabel: p.LRLabel, ID: id, Mode: PrettyMode(p.Mode), DType: p.DType, Band: "soft-gap",
				Acc: p.Acc, Soft: p.Soft, SoftGap: gap, Score: p.Score, Thru: p.Thru, Avail: p.Avail, RAMKiB: p.RAMKiB,
			})
		}
		for _, r := range lpd.TopSpeed {
			if r.RelAcc >= LPDKeepFloor {
				continue
			}
			lbl := lrByID[r.ID]
			if lbl == "" {
				if _, l2, ok := ParseLRFromCellID(r.ID); ok {
					lbl = l2
				}
			}
			addRow(r, machine, lbl, "fast-trap")
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		return all[i].SoftGap > all[j].SoftGap
	})
	for i := range all {
		all[i].Rank = i + 1
	}
	if topN > 0 && len(all) > topN {
		all = all[:topN]
	}
	return all
}

func compareTrapRate(pts []taggedPoint, machines []string, lrMap map[float64]string) []CompareTrapRateRow {
	type acc struct{ n, trap, softGap int }
	by := map[string]*acc{}
	key := func(m string, lr float64) string { return m + "\x00" + FormatLR(lr) }
	for _, m := range machines {
		for lr := range lrMap {
			by[key(m, lr)] = &acc{}
		}
	}
	for machine, mpts := range groupByMachine(pts) {
		lpd := BuildLPD(taggedToCells(mpts))
		trapID := map[string]bool{}
		for _, r := range lpd.Trap {
			trapID[PrettyCell(r.ID)] = true
		}
		for _, p := range mpts {
			k := key(machine, p.LR)
			a := by[k]
			if a == nil {
				a = &acc{}
				by[k] = a
			}
			a.n++
			if trapID[PrettyCell(p.ID)] {
				a.trap++
			}
			if p.Soft-p.Acc >= 10 && p.Acc < 55 {
				a.softGap++
			}
		}
	}
	var out []CompareTrapRateRow
	for _, m := range machines {
		for lr, lbl := range lrMap {
			a := by[key(m, lr)]
			if a == nil || a.n == 0 {
				continue
			}
			if lbl == "" {
				lbl = FormatLR(lr)
			}
			out = append(out, CompareTrapRateRow{
				Machine: m, LR: lr, LRLabel: lbl, N: a.n, TrapN: a.trap,
				TrapPct: 100 * float64(a.trap) / float64(a.n), SoftGapN: a.softGap,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Machine != out[j].Machine {
			return out[i].Machine < out[j].Machine
		}
		return out[i].LR < out[j].LR
	})
	return out
}

func compareSoftGap(pts []taggedPoint, _ []string, lrMap map[float64]string) []CompareSoftGapRow {
	type acc struct{ n int; sum, max float64 }
	by := map[string]*acc{}
	key := func(m string, lr float64) string { return m + "\x00" + FormatLR(lr) }
	for _, p := range pts {
		k := key(p.Machine, p.LR)
		a := by[k]
		if a == nil {
			a = &acc{}
			by[k] = a
		}
		gap := p.Soft - p.Acc
		a.n++
		a.sum += gap
		if gap > a.max {
			a.max = gap
		}
	}
	var out []CompareSoftGapRow
	for k, a := range by {
		if a.n == 0 {
			continue
		}
		parts := strings.Split(k, "\x00")
		if len(parts) != 2 {
			continue
		}
		m := parts[0]
		lr, _ := ParseLRLabel(parts[1])
		lbl := lrMap[lr]
		if lbl == "" {
			lbl = parts[1]
		}
		out = append(out, CompareSoftGapRow{
			Machine: m, LR: lr, LRLabel: lbl,
			MeanGap: a.sum / float64(a.n), MaxGap: a.max, N: a.n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Machine != out[j].Machine {
			return out[i].Machine < out[j].Machine
		}
		return out[i].LR < out[j].LR
	})
	return out
}

func compareScatter(pts []taggedPoint, maxN int) []CompareScatterPoint {
	if maxN <= 0 || len(pts) <= maxN {
		out := make([]CompareScatterPoint, 0, len(pts))
		for _, p := range pts {
			out = append(out, CompareScatterPoint{
				Machine: p.Machine, LRLabel: p.LRLabel, Acc: p.Acc, Soft: p.Soft,
				Avail: p.Avail, Score: p.Score,
			})
		}
		return out
	}
	step := len(pts) / maxN
	if step < 1 {
		step = 1
	}
	out := make([]CompareScatterPoint, 0, maxN)
	for i := 0; i < len(pts); i += step {
		p := pts[i]
		out = append(out, CompareScatterPoint{
			Machine: p.Machine, LRLabel: p.LRLabel, Acc: p.Acc, Soft: p.Soft,
			Avail: p.Avail, Score: p.Score,
		})
	}
	return out
}

func compareModeSeries(pts []taggedPoint, machines []string, topN int) []CompareModeSeries {
	var out []CompareModeSeries
	for _, m := range machines {
		lrs, _ := lrsForMachine(pts, m)
		if len(lrs) == 0 {
			continue
		}
		type cell struct{ n int; acc, soft, score, avail float64 }
		by := map[string]map[float64]*cell{}
		modeN := map[string]int{}
		for _, p := range pts {
			if p.Machine != m {
				continue
			}
			mode := PrettyMode(p.Mode)
			modeN[mode]++
			row := by[mode]
			if row == nil {
				row = map[float64]*cell{}
				by[mode] = row
			}
			c := row[p.LR]
			if c == nil {
				c = &cell{}
				row[p.LR] = c
			}
			c.n++
			c.acc += p.Acc
			c.soft += p.Soft
			c.score += p.Score
			c.avail += p.Avail
		}
		for _, mode := range topModesByCount(modeN, topN) {
			ptsOut := make([]CompareModeSeriesPoint, 0, len(lrs))
			for _, lr := range lrs {
				c := by[mode][lr]
				if c == nil || c.n == 0 {
					continue
				}
				n := float64(c.n)
				lbl := lrLabelOf(pts, m, lr)
				ptsOut = append(ptsOut, CompareModeSeriesPoint{
					LR: lr, LRLabel: lbl, N: c.n,
					Acc: c.acc / n, Soft: c.soft / n, Score: c.score / n, Avail: c.avail / n,
				})
			}
			if len(ptsOut) == 0 {
				continue
			}
			out = append(out, CompareModeSeries{Machine: m, Mode: mode, Points: ptsOut})
		}
	}
	return out
}

func compareModeCross(pts []taggedPoint, machines []string, topN int) []CompareModeCross {
	modeN := map[string]int{}
	for _, p := range pts {
		modeN[PrettyMode(p.Mode)]++
	}
	var out []CompareModeCross
	for _, mode := range topModesByCount(modeN, topN) {
		type cell struct{ n int; acc, soft, score, avail float64 }
		by := map[string]*cell{}
		key := func(m string, lr float64) string { return m + "\x00" + FormatLR(lr) }
		for _, p := range pts {
			if PrettyMode(p.Mode) != mode {
				continue
			}
			k := key(p.Machine, p.LR)
			c := by[k]
			if c == nil {
				c = &cell{}
				by[k] = c
			}
			c.n++
			c.acc += p.Acc
			c.soft += p.Soft
			c.score += p.Score
			c.avail += p.Avail
		}
		var ptsOut []CompareModeCrossPoint
		for k, c := range by {
			if c.n == 0 {
				continue
			}
			parts := strings.Split(k, "\x00")
			if len(parts) != 2 {
				continue
			}
			lr, _ := ParseLRLabel(parts[1])
			n := float64(c.n)
			ptsOut = append(ptsOut, CompareModeCrossPoint{
				Machine: parts[0], LR: lr, LRLabel: parts[1], N: c.n,
				Acc: c.acc / n, Soft: c.soft / n, Score: c.score / n, Avail: c.avail / n,
			})
		}
		sort.Slice(ptsOut, func(i, j int) bool {
			if ptsOut[i].Machine != ptsOut[j].Machine {
				return ptsOut[i].Machine < ptsOut[j].Machine
			}
			return ptsOut[i].LR < ptsOut[j].LR
		})
		if len(ptsOut) == 0 {
			continue
		}
		out = append(out, CompareModeCross{Mode: mode, Points: ptsOut})
	}
	return out
}

func compareVsBaselineLR(pts []taggedPoint, machines []string, topN int) []CompareVsBaselineGrid {
	var out []CompareVsBaselineGrid
	for _, machine := range machines {
		var mpts []taggedPoint
		rawModes := make([]string, 0, 32)
		for _, p := range pts {
			if p.Machine != machine {
				continue
			}
			mpts = append(mpts, p)
			rawModes = append(rawModes, p.Mode)
		}
		if len(mpts) == 0 {
			continue
		}
		baseline := PickBaseline(rawModes)
		if baseline == "" {
			continue
		}
		baseKey := modeKey(baseline)
		lrs, lrLabels := lrsForMachine(mpts, machine)
		baseByKey := map[string]CellPoint{}
		for _, p := range mpts {
			if modeKey(p.Mode) != baseKey {
				continue
			}
			k := compareMatchKey(p)
			prev, ok := baseByKey[k]
			if !ok || p.Acc > prev.Acc {
				baseByKey[k] = p.CellPoint
			}
		}
		if len(baseByKey) == 0 {
			continue
		}
		type cell struct{ n int; accD, scoreD, availD float64 }
		by := map[string]map[float64]*cell{}
		modeN := map[string]int{}
		for _, p := range mpts {
			if modeKey(p.Mode) == baseKey {
				continue
			}
			b, ok := baseByKey[compareMatchKey(p)]
			if !ok {
				continue
			}
			mode := PrettyMode(p.Mode)
			modeN[mode]++
			row := by[mode]
			if row == nil {
				row = map[float64]*cell{}
				by[mode] = row
			}
			c := row[p.LR]
			if c == nil {
				c = &cell{}
				row[p.LR] = c
			}
			c.n++
			c.accD += p.Acc - b.Acc
			c.scoreD += p.Score - b.Score
			c.availD += p.Avail - b.Avail
		}
		modes := topModesByCount(modeN, topN)
		accGrid := make([][]float64, len(modes))
		scoreGrid := make([][]float64, len(modes))
		availGrid := make([][]float64, len(modes))
		for i, mode := range modes {
			accGrid[i] = make([]float64, len(lrs))
			scoreGrid[i] = make([]float64, len(lrs))
			availGrid[i] = make([]float64, len(lrs))
			for j, lr := range lrs {
				if c := by[mode][lr]; c != nil && c.n > 0 {
					n := float64(c.n)
					accGrid[i][j] = c.accD / n
					scoreGrid[i][j] = c.scoreD / n
					availGrid[i][j] = c.availD / n
				}
			}
		}
		type lrAcc struct{ n int; acc, avail float64 }
		baseLR := map[float64]*lrAcc{}
		for _, p := range mpts {
			if modeKey(p.Mode) != baseKey {
				continue
			}
			a := baseLR[p.LR]
			if a == nil {
				a = &lrAcc{}
				baseLR[p.LR] = a
			}
			a.n++
			a.acc += p.Acc
			a.avail += p.Avail
		}
		baselineAcc := make([]float64, len(lrs))
		baselineAvail := make([]float64, len(lrs))
		for j, lr := range lrs {
			if a := baseLR[lr]; a != nil && a.n > 0 {
				n := float64(a.n)
				baselineAcc[j] = a.acc / n
				baselineAvail[j] = a.avail / n
			}
		}
		out = append(out, CompareVsBaselineGrid{
			Machine: machine, Baseline: PrettyMode(baseline),
			LRLabels: lrLabels, LRs: lrs, Modes: modes,
			AccDelta: accGrid, ScoreDelta: scoreGrid, AvailDelta: availGrid,
			BaselineAcc: baselineAcc, BaselineAvail: baselineAvail,
		})
	}
	return out
}

func compareMatchKey(p taggedPoint) string {
	cp := p.CellPoint
	layer := cp.Layer
	if layer == "" {
		parts := strings.SplitN(RecipeKey(cp.ID), "|", 2)
		if len(parts) > 0 {
			layer = parts[0]
		}
	}
	return strings.Join([]string{cp.Tide, cp.Task, layer, cp.DType, cp.Format, cp.Arch, FormatLR(p.LR)}, "\x1f")
}

func topModesByCount(modeN map[string]int, topN int) []string {
	type kv struct{ mode string; n int }
	order := make([]kv, 0, len(modeN))
	for m, n := range modeN {
		order = append(order, kv{m, n})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].n != order[j].n {
			return order[i].n > order[j].n
		}
		return order[i].mode < order[j].mode
	})
	if topN > 0 && len(order) > topN {
		order = order[:topN]
	}
	out := make([]string, len(order))
	for i, k := range order {
		out[i] = k.mode
	}
	return out
}

func lrLabelOf(pts []taggedPoint, machine string, lr float64) string {
	for _, p := range pts {
		if p.Machine == machine && p.LR == lr && p.LRLabel != "" {
			return p.LRLabel
		}
	}
	return FormatLR(lr)
}

func compareModeBars(pts []taggedPoint, machines []string, topModes int) []CompareModeBar {
	type acc struct{ n int; sum float64 }
	by := map[string]map[string]*acc{}
	modeN := map[string]int{}
	for _, p := range pts {
		mode := PrettyMode(p.Mode)
		modeN[mode]++
		row := by[mode]
		if row == nil {
			row = map[string]*acc{}
			by[mode] = row
		}
		a := row[p.Machine]
		if a == nil {
			a = &acc{}
			row[p.Machine] = a
		}
		a.n++
		a.sum += p.Acc
	}
	type kv struct{ mode string; n int }
	var order []kv
	for m, n := range modeN {
		order = append(order, kv{m, n})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].n != order[j].n {
			return order[i].n > order[j].n
		}
		return order[i].mode < order[j].mode
	})
	if topModes > 0 && len(order) > topModes {
		order = order[:topModes]
	}
	var out []CompareModeBar
	for _, k := range order {
		vals := map[string]float64{}
		for _, m := range machines {
			if a := by[k.mode][m]; a != nil && a.n > 0 {
				vals[m] = a.sum / float64(a.n)
			}
		}
		out = append(out, CompareModeBar{Mode: k.mode, ByMachine: vals})
	}
	return out
}

func lrLabelsOf(pts []taggedPoint) map[string]string {
	m := map[string]string{}
	for _, p := range pts {
		m[p.ID] = p.LRLabel
	}
	return m
}

func taggedToCells(mpts []taggedPoint) []CellPoint {
	cells := make([]CellPoint, 0, len(mpts))
	for _, p := range mpts {
		pt := p.CellPoint
		pt.Tide = p.Machine
		cells = append(cells, pt)
	}
	return cells
}

func groupByMachine(pts []taggedPoint) map[string][]taggedPoint {
	out := map[string][]taggedPoint{}
	for _, p := range pts {
		out[p.Machine] = append(out[p.Machine], p)
	}
	return out
}

func compareLPDTop(pts []taggedPoint, topN int) []CompareLPDEntry {
	cells := make([]CellPoint, 0, len(pts))
	lrByID := map[string]string{}
	for _, p := range pts {
		pt := p.CellPoint
		pt.Tide = p.Machine
		cells = append(cells, pt)
		lrByID[pt.ID] = p.LRLabel
	}
	lpd := BuildLPD(cells)
	var rows []LPDRow
	rows = append(rows, lpd.Top...)
	rows = append(rows, lpd.Gold...)
	rows = append(rows, lpd.Near...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LPD != rows[j].LPD {
			return rows[i].LPD > rows[j].LPD
		}
		return rows[i].Score > rows[j].Score
	})
	seen := map[string]bool{}
	var out []CompareLPDEntry
	for _, r := range rows {
		id := PrettyCell(r.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		lbl := lrByID[r.ID]
		if lbl == "" {
			if _, lbl2, ok := ParseLRFromCellID(r.ID); ok {
				lbl = lbl2
			}
		}
		out = append(out, CompareLPDEntry{
			Machine: r.Tide, LRLabel: lbl, Rank: len(out) + 1,
			ID: id, Mode: PrettyMode(r.Mode), DType: r.DType,
			LPD: r.LPD, Acc: r.Acc, Score: r.Score, Avail: r.Avail, RAMKiB: r.RAMKiB, Band: r.Band,
		})
		if topN > 0 && len(out) >= topN {
			break
		}
	}
	return out
}

func compareWins(pts []taggedPoint, machines []string, lrs []float64, lrMap map[float64]string) []CompareWinRow {
	type key struct{ recipe string; lr float64 }
	best := map[key]struct{ machine string; acc float64 }{}
	for _, p := range pts {
		k := key{recipe: RecipeKey(p.ID), lr: p.LR}
		prev := best[k]
		if p.Acc > prev.acc {
			best[k] = struct{ machine string; acc float64 }{p.Machine, p.Acc}
		}
	}
	out := make([]CompareWinRow, 0, len(lrs))
	for _, lr := range lrs {
		lbl := lrMap[lr]
		if lbl == "" {
			lbl = FormatLR(lr)
		}
		wins := map[string]int{}
		for _, m := range machines {
			wins[m] = 0
		}
		for k, w := range best {
			if k.lr != lr {
				continue
			}
			wins[w.machine]++
		}
		out = append(out, CompareWinRow{LRLabel: lbl, LR: lr, Wins: wins})
	}
	return out
}
