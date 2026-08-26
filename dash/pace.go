package dash

import (
	"sort"
	"time"

	"github.com/openfluke/tide/pulse"
)

// SweepPace holds ETA stats derived from finished cell durations + wall clock.
type SweepPace struct {
	AvgCellSec   float64 `json:"avg_cell_sec"`
	MedCellSec   float64 `json:"med_cell_sec"`
	CellsPerHour float64 `json:"cells_per_hour"`
	EtaEpochSec  float64 `json:"eta_epoch_sec"`
	EtaSweepSec  float64 `json:"eta_sweep_sec"`
	PaceSamples  int     `json:"pace_samples"`
	WallSecPer   float64 `json:"wall_sec_per_cell,omitempty"`
}

// minPaceSec drops instant Begin→Finish noise (multi-worker hosts that
// re-Begin after the job already finished report ~0s otherwise).
const minPaceSec = 0.05
const maxPaceSamples = 200

func computeSweepPace(completed []pulse.Result, left, epochsLeft int) SweepPace {
	return computeSweepPaceWall(completed, left, epochsLeft, time.Time{}, 0, 0)
}

// computeSweepPaceWall prefers real cell durations; if those are junk/missing,
// falls back to wall-clock throughput since the pace anchor (SignalStart).
// wallDoneBase is completed-count at anchor; wallNew is cells finished after.
func computeSweepPaceWall(completed []pulse.Result, left, epochsLeft int, wallStart time.Time, wallDoneBase, wallNew int) SweepPace {
	var secs []float64
	start := 0
	if len(completed) > maxPaceSamples {
		start = len(completed) - maxPaceSamples
	}
	for _, r := range completed[start:] {
		if r.Ended.IsZero() || r.Started.IsZero() {
			continue
		}
		d := r.Ended.Sub(r.Started).Seconds()
		if d >= minPaceSec {
			secs = append(secs, d)
		}
	}
	out := SweepPace{PaceSamples: len(secs)}
	use := 0.0
	if len(secs) > 0 {
		sort.Float64s(secs)
		sum := 0.0
		for _, s := range secs {
			sum += s
		}
		out.AvgCellSec = sum / float64(len(secs))
		out.MedCellSec = secs[len(secs)/2]
		use = out.MedCellSec
		if use <= 0 {
			use = out.AvgCellSec
		}
	}
	// Wall-clock farm rate: accounts for parallel workers + queue wait.
	if !wallStart.IsZero() && wallNew > 0 {
		elapsed := time.Since(wallStart).Seconds()
		if elapsed > 1 {
			wallSec := elapsed / float64(wallNew)
			out.WallSecPer = wallSec
			// Prefer wall rate when cell timestamps are missing/junk, or when
			// wall is slower (queue/wait) so ETA isn't wildly optimistic.
			if use < minPaceSec || wallSec > use {
				use = wallSec
			}
		}
	}
	_ = wallDoneBase
	if use > 0 {
		out.CellsPerHour = 3600 / use
		if left > 0 {
			out.EtaEpochSec = use * float64(left)
		}
		epLeft := epochsLeft
		if epLeft < 1 {
			epLeft = 1
		}
		out.EtaSweepSec = out.EtaEpochSec * float64(epLeft)
	}
	return out
}
