package dash

import (
	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/report"
)

func extraBests(completed []pulse.Result) (adapt, soft, hard, cons, stab, accThru, realtime, keep *pulse.Result) {
	for i := range completed {
		r := completed[i]
		if r.Status != "ok" {
			continue
		}
		adapt = betterMax(adapt, r, func(s metrics.Snapshot) float64 { return s.AdaptPct })
		soft = betterMax(soft, r, func(s metrics.Snapshot) float64 { return s.SoftAcc })
		hard = betterMax(hard, r, func(s metrics.Snapshot) float64 { return s.AvgAccuracy })
		cons = betterMax(cons, r, func(s metrics.Snapshot) float64 { return s.Consistency })
		stab = betterMax(stab, r, func(s metrics.Snapshot) float64 { return s.Stability })
		accThru = betterMax(accThru, r, AccThru)
		realtime = betterMax(realtime, r, Realtime)
		keep = betterMax(keep, r, KeepLearn)
	}
	return
}

func betterMax(dst *pulse.Result, r pulse.Result, val func(metrics.Snapshot) float64) *pulse.Result {
	v := val(r.Snapshot)
	if v <= 0 {
		return dst
	}
	if dst == nil || v > val(dst.Snapshot) {
		cp := r
		return &cp
	}
	return dst
}

// AccThru is hard Acc × Throughput / 100 — accurate and fast.
func AccThru(s metrics.Snapshot) float64 {
	return s.AvgAccuracy * s.Throughput / 100
}

// Realtime is Throughput × Availability / 100 — serve+train duty cycle speed.
func Realtime(s metrics.Snapshot) float64 {
	return metrics.Realtime(s.Throughput, s.Availability)
}

// KeepLearn prefers cells that keep rising in late SoftAcc windows instead of peaking then flattening.
func KeepLearn(s metrics.Snapshot) float64 {
	blocks := s.SoftAccBlocks
	n := len(blocks)
	if n >= 6 {
		k := n / 3
		first, last, peak := 0.0, 0.0, 0.0
		for i := 0; i < k; i++ {
			first += blocks[i]
			last += blocks[n-k+i]
		}
		first /= float64(k)
		last /= float64(k)
		for _, v := range blocks {
			if v > peak {
				peak = v
			}
		}
		if peak < 1e-9 {
			return 0
		}
		stay := last / peak
		rise := last - first
		if rise < 0 {
			rise = 0
		}
		return stay * (40 + rise) * (0.25 + s.AdaptPct/400)
	}
	return s.AccPerSec
}

// LucyAxis is this tide's champion on one Lucy metric.
type LucyAxis struct {
	Name        string  `json:"name"`
	Hint        string  `json:"hint"`
	Value       float64 `json:"value"`
	LowerBetter bool    `json:"lower_better,omitempty"`
	CellID      string  `json:"cell_id"`
	Mode        string  `json:"mode"`
	DType       string  `json:"dtype"`
	Format      string  `json:"format"`
	Arch        string  `json:"arch"`
	Score       float64 `json:"score"`
	SoftAcc     float64 `json:"soft_acc"`
	Thru        float64 `json:"throughput"`
	Avail       float64 `json:"availability"`
	Adapt       float64 `json:"adapt_pct"`
}

// LucyAxes lists this board's winner on every Lucy axis that has a sample.
func LucyAxes(b Board) []LucyAxis {
	type spec struct {
		name, hint string
		get        func(Board) *pulse.Result
		val        func(metrics.Snapshot) float64
		lower      bool
	}
	specs := []spec{
		{"hard_acc", "pillar: argmax accuracy (learning)", func(b Board) *pulse.Result { return b.BestHard }, func(s metrics.Snapshot) float64 { return s.AvgAccuracy }, false},
		{"throughput", "pillar: outputs / second", func(b Board) *pulse.Result { return b.Best.Throughput }, func(s metrics.Snapshot) float64 { return s.Throughput }, false},
		{"availability", "pillar: infer / (infer+train) duty cycle", func(b Board) *pulse.Result { return b.Best.Availability }, func(s metrics.Snapshot) float64 { return s.Availability }, false},
		{"score", "live-fit: T x Avail x Acc / 10,000 (SGD that blocks serve dies here)", func(b Board) *pulse.Result { return b.Best.Score }, func(s metrics.Snapshot) float64 { return s.Score }, false},
		{"soft_acc", "serve-confidence (softmax vs true class) — not Score", func(b Board) *pulse.Result { return b.BestSoft }, func(s metrics.Snapshot) float64 { return s.SoftAcc }, false},
		{"acc_thru", "hard Acc x Throughput / 100", func(b Board) *pulse.Result { return b.BestAccThru }, AccThru, false},
		{"realtime", "Throughput x Availability / 100", func(b Board) *pulse.Result { return b.BestRealtime }, Realtime, false},
		{"adapt", "AdaptPct after phase switches", func(b Board) *pulse.Result { return b.BestAdapt }, func(s metrics.Snapshot) float64 { return s.AdaptPct }, false},
		{"keep_learn", "late SoftAcc still rising (not plateau)", func(b Board) *pulse.Result { return b.BestKeep }, KeepLearn, false},
		{"acc_per_sec", "SoftAcc gained per wall second", func(b Board) *pulse.Result { return b.BestLearn.AccPerSec }, func(s metrics.Snapshot) float64 { return s.AccPerSec }, false},
		{"time_to_50", "seconds to 50% window acc (lower better)", func(b Board) *pulse.Result { return b.BestLearn.To50 }, func(s metrics.Snapshot) float64 { return s.TimeToAcc50Sec }, true},
		{"consistency", "share of windows above SoftAcc threshold", func(b Board) *pulse.Result { return b.BestConsistency }, func(s metrics.Snapshot) float64 { return s.Consistency }, false},
		{"stability", "low SoftAcc variance after switches", func(b Board) *pulse.Result { return b.BestStability }, func(s metrics.Snapshot) float64 { return s.Stability }, false},
		{"mobile_score", "raw Score/MiB (binary trap — use LPD density)", func(b Board) *pulse.Result { return b.BestMobile.Score }, func(s metrics.Snapshot) float64 { return s.MobileScore }, false},
	}
	out := make([]LucyAxis, 0, len(specs))
	for _, sp := range specs {
		r := sp.get(b)
		if r == nil {
			continue
		}
		v := sp.val(r.Snapshot)
		if v <= 0 {
			continue
		}
		s := r.Snapshot
		out = append(out, LucyAxis{
			Name: sp.name, Hint: sp.hint, Value: v, LowerBetter: sp.lower,
			CellID: report.PrettyCell(r.Cell.ID), Mode: string(r.Cell.Mode), DType: r.Cell.DType.String(),
			Format: r.Cell.Format.String(), Arch: report.PrettyArch(r.Cell.ArchTag()),
			Score: s.Score, SoftAcc: s.SoftAcc, Thru: s.Throughput, Avail: s.Availability, Adapt: s.AdaptPct,
		})
	}
	return out
}
