package dash

import (
	"github.com/openfluke/tide/pulse"
)

// SlimResult strips heavy per-pulse arrays from API payloads.
// Full detail is available via GET /api/cell?id=….
func SlimResult(r pulse.Result) pulse.Result {
	cp := r
	cp.Snapshot.Windows = nil
	cp.Snapshot.SoftAccBlocks = nil
	cp.Snapshot.PhaseBlocks = nil
	cp.Snapshot.SwitchBlocks = nil
	return cp
}

func slimPtr(r *pulse.Result) *pulse.Result {
	if r == nil {
		return nil
	}
	cp := SlimResult(*r)
	return &cp
}

// SlimResults maps a slice through SlimResult.
func SlimResults(in []pulse.Result) []pulse.Result {
	if len(in) == 0 {
		return nil
	}
	out := make([]pulse.Result, len(in))
	for i, r := range in {
		out[i] = SlimResult(r)
	}
	return out
}

func slimBest(b pulse.Best) pulse.Best {
	return pulse.Best{
		Score:        slimPtr(b.Score),
		Throughput:   slimPtr(b.Throughput),
		Availability: slimPtr(b.Availability),
		Accuracy:     slimPtr(b.Accuracy),
	}
}

func slimBestMobile(b pulse.BestMobile) pulse.BestMobile {
	return pulse.BestMobile{
		Score:        slimPtr(b.Score),
		Throughput:   slimPtr(b.Throughput),
		Availability: slimPtr(b.Availability),
		Accuracy:     slimPtr(b.Accuracy),
	}
}

func slimBestLearn(b pulse.BestLearn) pulse.BestLearn {
	return pulse.BestLearn{
		To25:      slimPtr(b.To25),
		To50:      slimPtr(b.To50),
		AccPerSec: slimPtr(b.AccPerSec),
	}
}

func slimBestLearnMobile(b pulse.BestLearnMobile) pulse.BestLearnMobile {
	return pulse.BestLearnMobile{
		AccPerSec: slimPtr(b.AccPerSec),
		To50:      slimPtr(b.To50),
	}
}
