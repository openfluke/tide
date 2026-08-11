// Package metrics implements the Lucy / test41-w mid-stream adaptation Score.
//
//	SoftAcc     = 100 × (1 − |pred−target| / 0.10)   (clamped; mean over dims/batch)
//	Availability = InferMs / (InferMs + TrainMs) × 100
//	Score        = Throughput × Availability × SoftAcc / 10_000
package metrics

import (
	"math"
	"time"
)

// SoftAccScale matches test41_w_sine_ada_perm / legacy all_sine_wave.go.
const SoftAccScale = 0.10

// ConsThreshold — window SoftAcc ≥ this counts toward Consistency (%).
const ConsThreshold = 10.0

// AdaptWindows — number of pulse windows after a phase switch folded into AdaptPct.
const AdaptWindows = 4

// Acc thresholds for time-to-accuracy tracking (hard Acc).
const (
	AccThreshold25 = 25.0
	AccThreshold50 = 50.0
)

// MaxRetainedWindows caps in-memory sparkline history.
const MaxRetainedWindows = 120

// Window is one pulse sample (typically 1s).
type Window struct {
	At             time.Time     `json:"at"`
	Outputs        int64         `json:"outputs"`
	Correct        int64         `json:"correct"`
	TrainSteps     int64         `json:"train_steps"`
	InferMs        float64       `json:"infer_ms"`
	TrainMs        float64       `json:"train_ms"`
	BlockedTrain   time.Duration `json:"blocked_train"` // legacy alias of train busy
	Phase          string        `json:"phase"`
	PhaseSwitches  int           `json:"phase_switches"`
	Accuracy       float64       `json:"accuracy"`  // hard Acc 0–100
	SoftAcc        float64       `json:"soft_acc"`  // SoftAcc 0–100
	Throughput     float64       `json:"throughput"`
}

// Snapshot is the Lucy-style aggregate for one permutation run.
type Snapshot struct {
	TotalOutputs int64         `json:"total_outputs"`
	TotalCorrect int64         `json:"total_correct"`
	TotalTrain   int64         `json:"total_train"`
	InferMs      float64       `json:"infer_ms"`
	TrainMs      float64       `json:"train_ms"`
	BlockedTrain time.Duration `json:"blocked_train"` // = TrainMs as duration (compat)
	Duration     time.Duration `json:"duration"`

	AvgAccuracy  float64 `json:"avg_accuracy"` // hard Acc 0–100 (argmax)
	SoftAcc      float64 `json:"soft_acc"`     // SoftAcc 0–100 — Acc term in Score
	AdaptPct     float64 `json:"adapt_pct"`    // mean SoftAcc in AdaptWindows after switches
	Stability    float64 `json:"stability"`    // 100 − σ(SoftAcc windows)
	Consistency  float64 `json:"consistency"`  // % windows with SoftAcc ≥ ConsThreshold

	Throughput   float64 `json:"throughput"`   // outputs/s
	Availability float64 `json:"availability"` // Infer/(Infer+Train) × 100
	Score        float64 `json:"score"`
	ZeroDowntime float64 `json:"zero_downtime"` // SoftAcc × Avail / 100

	WeightBytes int64   `json:"weight_bytes"`
	WeightMiB   float64 `json:"weight_mib"`
	HeapBytes   int64   `json:"heap_bytes"`
	HeapMiB     float64 `json:"heap_mib"`

	MobileScore        float64 `json:"mobile_score"`
	MobileThroughput   float64 `json:"mobile_throughput"`
	MobileAvailability float64 `json:"mobile_availability"`
	MobileAccuracy     float64 `json:"mobile_accuracy"` // SoftAcc / MiB
	Windows            []Window `json:"windows,omitempty"`

	// SoftAccBlocks is the 1s SoftAcc strip kept after Windows are stripped (dash detail).
	SoftAccBlocks []float64 `json:"soft_acc_blocks,omitempty"`
	PhaseBlocks   []string  `json:"phase_blocks,omitempty"`
	SwitchBlocks  []bool    `json:"switch_blocks,omitempty"`

	// AccuracyPulses folds SoftAcc into SoftAcc running mean when >0.
	AccuracyPulses int64 `json:"-"`

	TimeToAcc25Sec  float64 `json:"time_to_acc25_sec"`
	TimeToAcc50Sec  float64 `json:"time_to_acc50_sec"`
	AccPerSec       float64 `json:"acc_per_sec"`
	MobileAccPerSec float64 `json:"mobile_acc_per_sec"`
}

// SoftAccOne is SoftAcc for a single pred/target pair (test41 formula).
func SoftAccOne(pred, target float32) float64 {
	if math.IsNaN(float64(pred)) || math.IsInf(float64(pred), 0) {
		return 0
	}
	a := 100 * (1 - math.Abs(float64(pred-target))/SoftAccScale)
	if a < 0 {
		return 0
	}
	if a > 100 {
		return 100
	}
	return a
}

// SoftAccBatch means SoftAcc across all elements of pred vs one-hot/target.
func SoftAccBatch(pred, target []float32) float64 {
	n := len(pred)
	if n == 0 || len(target) < n {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += SoftAccOne(pred[i], target[i])
	}
	return sum / float64(n)
}

// AppendWindow adds w and drops the oldest when over MaxRetainedWindows.
func AppendWindow(dst []Window, w Window) []Window {
	dst = append(dst, w)
	if len(dst) > MaxRetainedWindows {
		dst = append([]Window(nil), dst[len(dst)-MaxRetainedWindows:]...)
	}
	return dst
}

// Finalize computes Lucy / test41 aggregates from windows + totals.
func Finalize(s *Snapshot) {
	if s == nil {
		return
	}
	if s.Duration > 0 {
		s.Throughput = float64(s.TotalOutputs) / s.Duration.Seconds()
	}
	// Duty-cycle Availability (test41): Infer / (Infer + Train).
	busy := s.InferMs + s.TrainMs
	if busy > 0 {
		s.Availability = 100 * s.InferMs / busy
	} else if s.Duration > 0 {
		// Fallback if timers missing: wall − blocked.
		avail := s.Duration - s.BlockedTrain
		if avail < 0 {
			avail = 0
		}
		s.Availability = float64(avail) / float64(s.Duration) * 100
	}

	if s.AccuracyPulses == 0 {
		if len(s.Windows) > 0 {
			var hard, soft float64
			for _, w := range s.Windows {
				hard += w.Accuracy
				soft += w.SoftAcc
			}
			n := float64(len(s.Windows))
			s.AvgAccuracy = hard / n
			s.SoftAcc = soft / n
		} else if s.TotalOutputs > 0 {
			s.AvgAccuracy = 100 * float64(s.TotalCorrect) / float64(s.TotalOutputs)
		}
	}

	// Stability / Consistency / AdaptPct from SoftAcc windows or retained SoftAccBlocks.
	nWin := len(s.Windows)
	blocks := s.SoftAccBlocks
	switches := s.SwitchBlocks
	if len(blocks) == 0 && nWin > 0 {
		blocks = make([]float64, nWin)
		switches = make([]bool, nWin)
		for i, w := range s.Windows {
			blocks[i] = w.SoftAcc
			switches[i] = w.PhaseSwitches > 0
		}
	}
	nBlk := len(blocks)
	if nBlk > 0 {
		mean := s.SoftAcc
		vari := 0.0
		valid := 0
		above := 0
		for _, a := range blocks {
			if math.IsNaN(a) {
				continue
			}
			d := a - mean
			vari += d * d
			valid++
			if a >= ConsThreshold {
				above++
			}
		}
		if valid > 0 {
			vari /= float64(valid)
			s.Stability = math.Max(0, 100-math.Sqrt(vari))
		}
		s.Consistency = float64(above) / float64(nBlk) * 100

		adaptSum, adaptN := 0.0, 0
		for i := range blocks {
			sw := false
			if i < len(switches) {
				sw = switches[i]
			} else if i < nWin {
				sw = s.Windows[i].PhaseSwitches > 0
			}
			if !sw {
				continue
			}
			for k := 0; k < AdaptWindows && i+k < nBlk; k++ {
				adaptSum += blocks[i+k]
				adaptN++
			}
		}
		if adaptN > 0 {
			s.AdaptPct = adaptSum / float64(adaptN)
		}
	}

	// SoftAcc 1s strip for dash detail — prefer runner-accumulated blocks; else copy Windows.
	if len(s.SoftAccBlocks) == 0 && nWin > 0 {
		s.SoftAccBlocks = make([]float64, nWin)
		s.PhaseBlocks = make([]string, nWin)
		s.SwitchBlocks = make([]bool, nWin)
		for i, w := range s.Windows {
			s.SoftAccBlocks[i] = w.SoftAcc
			s.PhaseBlocks[i] = w.Phase
			s.SwitchBlocks[i] = w.PhaseSwitches > 0
		}
	}

	// Score Acc term = SoftAcc (test41).
	s.Score = s.Throughput * s.Availability * s.SoftAcc / 10000
	s.ZeroDowntime = s.SoftAcc * s.Availability / 100
	s.WeightMiB = float64(s.WeightBytes) / (1024 * 1024)
	s.HeapMiB = float64(s.HeapBytes) / (1024 * 1024)
	mb := s.WeightMiB
	if mb < 1e-9 {
		mb = 1e-9
	}
	s.MobileScore = s.Score / mb
	s.MobileThroughput = s.Throughput / mb
	s.MobileAvailability = s.Availability / mb
	s.MobileAccuracy = s.SoftAcc / mb
	if s.Duration > 0 {
		s.AccPerSec = s.SoftAcc / s.Duration.Seconds()
		s.MobileAccPerSec = s.AccPerSec / mb
	}
	if math.IsNaN(s.Score) || math.IsInf(s.Score, 0) {
		s.Score = 0
	}
}

// WindowAccuracy returns 0–100 hard accuracy for a window.
func WindowAccuracy(correct, outputs int64) float64 {
	if outputs <= 0 {
		return 0
	}
	return 100 * float64(correct) / float64(outputs)
}
