package dash

import (
	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/pulse"
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

// AccThru is SoftAcc × Throughput / 100 — accurate *and* fast.
func AccThru(s metrics.Snapshot) float64 {
	return s.SoftAcc * s.Throughput / 100
}

// Realtime is Throughput × Availability / 100 — serve+train duty cycle speed.
func Realtime(s metrics.Snapshot) float64 {
	return s.Throughput * s.Availability / 100
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
