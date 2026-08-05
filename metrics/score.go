// Package metrics implements the Lucy dense mid-stream adaptation Score.
//
//	Score = Throughput × Availability% × AvgAccuracy% / 10_000
package metrics

import "time"

// Window is one pulse sample (typically 1s).
type Window struct {
	At           time.Time `json:"at"`
	Outputs      int64     `json:"outputs"`
	Correct      int64     `json:"correct"`
	TrainSteps   int64     `json:"train_steps"`
	BlockedTrain time.Duration `json:"blocked_train"`
	Phase        string    `json:"phase"`
	Accuracy     float64   `json:"accuracy"` // 0–100
	Throughput   float64   `json:"throughput"`
}

// Snapshot is the Lucy-style aggregate for one permutation run.
type Snapshot struct {
	TotalOutputs   int64         `json:"total_outputs"`
	TotalCorrect   int64         `json:"total_correct"`
	TotalTrain     int64         `json:"total_train"`
	BlockedTrain   time.Duration `json:"blocked_train"`
	Duration       time.Duration `json:"duration"`
	AvgAccuracy    float64       `json:"avg_accuracy"`  // 0–100
	Throughput     float64       `json:"throughput"`    // outputs/s
	Availability   float64       `json:"availability"`  // 0–100
	Score          float64       `json:"score"`
	ZeroDowntime   float64       `json:"zero_downtime"` // Acc × Avail / 100
	WeightBytes    int64         `json:"weight_bytes"`  // model payload (mobile footprint)
	WeightMiB      float64       `json:"weight_mib"`
	// Mobile* = metric / MiB — higher = better performance per RAM.
	MobileScore        float64 `json:"mobile_score"`
	MobileThroughput   float64 `json:"mobile_throughput"`
	MobileAvailability float64 `json:"mobile_availability"`
	MobileAccuracy     float64 `json:"mobile_accuracy"`
	Windows            []Window `json:"windows,omitempty"`
	// AccuracyPulses is the number of 1s windows folded into AvgAccuracy (running mean).
	// When >0, Finalize keeps AvgAccuracy instead of re-averaging (possibly capped) Windows.
	AccuracyPulses int64 `json:"-"`

	// Learning-speed (recorded live; 0 = threshold never hit).
	TimeToAcc25Sec float64 `json:"time_to_acc25_sec"` // wall seconds until 1s-window acc ≥ 25%
	TimeToAcc50Sec float64 `json:"time_to_acc50_sec"` // wall seconds until 1s-window acc ≥ 50%
	AccPerSec      float64 `json:"acc_per_sec"`       // AvgAccuracy / Duration.Seconds()
	MobileAccPerSec float64 `json:"mobile_acc_per_sec"` // AccPerSec / MiB
}

// Acc thresholds for time-to-accuracy tracking.
const (
	AccThreshold25 = 25.0
	AccThreshold50 = 50.0
)

// MaxRetainedWindows caps in-memory sparkline history (dashboard / Current only).
// Completed cells strip Windows entirely — unbounded growth was blowing RSS on long sweeps.
const MaxRetainedWindows = 120

// AppendWindow adds w and drops the oldest when over MaxRetainedWindows.
func AppendWindow(dst []Window, w Window) []Window {
	dst = append(dst, w)
	if len(dst) > MaxRetainedWindows {
		dst = append([]Window(nil), dst[len(dst)-MaxRetainedWindows:]...)
	}
	return dst
}

// Finalize computes Lucy aggregates from windows + totals.
func Finalize(s *Snapshot) {
	if s == nil {
		return
	}
	if s.Duration > 0 {
		s.Throughput = float64(s.TotalOutputs) / s.Duration.Seconds()
		avail := s.Duration - s.BlockedTrain
		if avail < 0 {
			avail = 0
		}
		s.Availability = float64(avail) / float64(s.Duration) * 100
	}
	if s.AccuracyPulses == 0 {
		if len(s.Windows) > 0 {
			var sum float64
			for _, w := range s.Windows {
				sum += w.Accuracy
			}
			s.AvgAccuracy = sum / float64(len(s.Windows))
		} else if s.TotalOutputs > 0 {
			s.AvgAccuracy = 100 * float64(s.TotalCorrect) / float64(s.TotalOutputs)
		}
	}
	s.Score = s.Throughput * s.Availability * s.AvgAccuracy / 10000
	s.ZeroDowntime = s.AvgAccuracy * s.Availability / 100
	s.WeightMiB = float64(s.WeightBytes) / (1024 * 1024)
	mb := s.WeightMiB
	if mb < 1e-9 {
		mb = 1e-9
	}
	s.MobileScore = s.Score / mb
	s.MobileThroughput = s.Throughput / mb
	s.MobileAvailability = s.Availability / mb
	s.MobileAccuracy = s.AvgAccuracy / mb
	if s.Duration > 0 {
		s.AccPerSec = s.AvgAccuracy / s.Duration.Seconds()
		s.MobileAccPerSec = s.AccPerSec / mb
	}
}

// WindowAccuracy returns 0–100 accuracy for a window.
func WindowAccuracy(correct, outputs int64) float64 {
	if outputs <= 0 {
		return 0
	}
	return 100 * float64(correct) / float64(outputs)
}
