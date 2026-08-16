// Package metrics is a thin re-export of the Lucy measuring harness
// (github.com/openfluke/welvet/lucy) for tide hosts.
package metrics

import "github.com/openfluke/welvet/lucy"

// SoftAccScale matches test41_w / Lucy SoftAcc (0.10).
const SoftAccScale = lucy.SoftAccScaleSine

// ConsThreshold — window SoftAcc ≥ this counts toward Consistency (%).
const ConsThreshold = lucy.ConsThreshold

// AdaptWindows — number of pulse windows after a phase switch folded into AdaptPct.
const AdaptWindows = lucy.AdaptWindowsDefault

// Acc thresholds for time-to-accuracy tracking (hard Acc).
const (
	AccThreshold25 = lucy.AccThreshold25
	AccThreshold50 = lucy.AccThreshold50
)

// MaxRetainedWindows caps in-memory sparkline history.
const MaxRetainedWindows = lucy.MaxRetainedWindows

// SoftAccScaleClass is SoftAcc scale for classification probabilities.
const SoftAccScaleClass = lucy.SoftAccScaleClass

// Window is one pulse sample (typically 1s).
type Window = lucy.Window

// Snapshot is the Lucy-style aggregate for one permutation run.
type Snapshot = lucy.Snapshot

// SoftAccOne is SoftAcc for a single pred/target pair (test41 formula).
func SoftAccOne(pred, target float32) float64 {
	return lucy.SoftAccOne(pred, target)
}

// SoftAccBatch means SoftAcc across all elements of pred vs target.
func SoftAccBatch(pred, target []float32) float64 {
	return lucy.SoftAccBatch(pred, target)
}

// SoftAccProb is SoftAcc for a probability in [0,1] vs target (MNIST / class).
func SoftAccProb(pred, target float32) float64 {
	return lucy.SoftAccProb(pred, target)
}

// AppendWindow adds w and drops the oldest when over MaxRetainedWindows.
func AppendWindow(dst []Window, w Window) []Window {
	return lucy.AppendWindow(dst, w)
}

// Finalize computes Lucy / test41 aggregates from windows + totals.
func Finalize(s *Snapshot) {
	lucy.Finalize(s, lucy.Options{
		AdaptWindows:  AdaptWindows,
		ConsThreshold: ConsThreshold,
	})
}

// WindowAccuracy returns 0–100 hard accuracy for a window.
func WindowAccuracy(correct, outputs int64) float64 {
	return lucy.WindowAccuracy(correct, outputs)
}
