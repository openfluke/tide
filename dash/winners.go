package dash

import (
	"sort"

	"github.com/openfluke/tide/pulse"
)

// AxisWinner is the best completed cell within a grouping key.
type AxisWinner struct {
	Group      string  `json:"group"`
	Winner     string  `json:"winner"`
	CellID     string  `json:"cell_id"`
	Score      float64 `json:"score"`
	SoftAcc    float64 `json:"soft_acc"`
	Accuracy   float64 `json:"avg_accuracy"`
	Throughput float64 `json:"throughput"`
	Avail      float64 `json:"availability"`
	AccPerSec  float64 `json:"acc_per_sec"`
	TimeTo50   float64 `json:"time_to_acc50_sec"`
	WeightKiB  float64 `json:"weight_kib"`
	Arch       string  `json:"arch,omitempty"`
	DType      string  `json:"dtype,omitempty"`
	Format     string  `json:"format,omitempty"`
	Mode       string  `json:"mode,omitempty"`
	N          int     `json:"n"` // ok cells considered in this group
}

// Winners is cross-axis champion tables for the dashboard.
type Winners struct {
	// BestSettingsPerMode is the single highest-Score cell per train mode
	// (full recipe: arch × dtype × format).
	BestSettingsPerMode []AxisWinner `json:"best_settings_per_mode"`
	BestCellPerMode     []AxisWinner `json:"best_cell_per_mode"`
	BestDTypePerMode    []AxisWinner `json:"best_dtype_per_mode"`
	BestFormatPerMode   []AxisWinner `json:"best_format_per_mode"`
	BestModePerDType    []AxisWinner `json:"best_mode_per_dtype"`
	BestModePerFormat   []AxisWinner `json:"best_mode_per_format"`
	BestFormatPerDType  []AxisWinner `json:"best_format_per_dtype"`
}

func betterScore(a, b pulse.Result) bool {
	if a.Snapshot.Score != b.Snapshot.Score {
		return a.Snapshot.Score > b.Snapshot.Score
	}
	if a.Snapshot.AvgAccuracy != b.Snapshot.AvgAccuracy {
		return a.Snapshot.AvgAccuracy > b.Snapshot.AvgAccuracy
	}
	return a.Snapshot.AccPerSec > b.Snapshot.AccPerSec
}

func toWinner(group, winner string, r pulse.Result, n int) AxisWinner {
	return AxisWinner{
		Group:      group,
		Winner:     winner,
		CellID:     r.Cell.ID,
		Score:      r.Snapshot.Score,
		SoftAcc:    r.Snapshot.SoftAcc,
		Accuracy:   r.Snapshot.AvgAccuracy,
		Throughput: r.Snapshot.Throughput,
		Avail:      r.Snapshot.Availability,
		AccPerSec:  r.Snapshot.AccPerSec,
		TimeTo50:   r.Snapshot.TimeToAcc50Sec,
		WeightKiB:  float64(r.Snapshot.WeightBytes) / 1024,
		Arch:       r.Cell.ArchTag(),
		DType:      r.Cell.DType.String(),
		Format:     r.Cell.Format.String(),
		Mode:       string(r.Cell.Mode),
		N:          n,
	}
}

// pickBest groups ok results by keyFn(groupKey) and keeps the best score cell;
// winner label comes from winFn(result).
func pickBest(ok []pulse.Result, groupFn, winFn func(pulse.Result) string) []AxisWinner {
	type bucket struct {
		best pulse.Result
		n    int
		set  bool
	}
	order := make([]string, 0)
	seen := map[string]bool{}
	by := map[string]*bucket{}
	for _, r := range ok {
		g := groupFn(r)
		if g == "" {
			continue
		}
		if !seen[g] {
			seen[g] = true
			order = append(order, g)
		}
		b := by[g]
		if b == nil {
			b = &bucket{}
			by[g] = b
		}
		b.n++
		if !b.set || betterScore(r, b.best) {
			b.best = r
			b.set = true
		}
	}
	out := make([]AxisWinner, 0, len(order))
	for _, g := range order {
		b := by[g]
		if b == nil || !b.set {
			continue
		}
		out = append(out, toWinner(g, winFn(b.best), b.best, b.n))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

func computeWinners(live pulse.Live) Winners {
	ok := make([]pulse.Result, 0, len(live.Completed))
	for _, r := range live.Completed {
		if r.Status == "ok" {
			ok = append(ok, r)
		}
	}
	dtypeOf := func(r pulse.Result) string { return r.Cell.DType.String() }
	formatOf := func(r pulse.Result) string { return r.Cell.Format.String() }
	modeOf := func(r pulse.Result) string { return string(r.Cell.Mode) }
	modeArchOf := func(r pulse.Result) string {
		return string(r.Cell.Mode) + " · " + r.Cell.ArchTag()
	}
	idOf := func(r pulse.Result) string {
		if r.Cell.ID != "" {
			return r.Cell.ID
		}
		return r.Cell.String()
	}

	return Winners{
		BestSettingsPerMode: pickBest(ok, modeOf, idOf),
		BestCellPerMode:     pickBest(ok, modeArchOf, idOf),
		BestDTypePerMode:    pickBest(ok, modeOf, dtypeOf),
		BestFormatPerMode:   pickBest(ok, modeOf, formatOf),
		BestModePerDType:    pickBest(ok, dtypeOf, modeOf),
		BestModePerFormat:   pickBest(ok, formatOf, modeOf),
		BestFormatPerDType:  pickBest(ok, dtypeOf, formatOf),
	}
}
