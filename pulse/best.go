package pulse

// Best holds the top Result for each Lucy axis + composite Score (raw performance).
type Best struct {
	Score        *Result `json:"score,omitempty"`
	Throughput   *Result `json:"throughput,omitempty"`
	Availability *Result `json:"availability,omitempty"`
	Accuracy     *Result `json:"accuracy,omitempty"`
}

// BestMobile holds the best performance-per-RAM winners (metric / MiB).
// Tie-break: higher efficiency, then smaller WeightBytes, then higher raw metric.
type BestMobile struct {
	Score        *Result `json:"score,omitempty"`
	Throughput   *Result `json:"throughput,omitempty"`
	Availability *Result `json:"availability,omitempty"`
	Accuracy     *Result `json:"accuracy,omitempty"`
}

// UpdateBest revises Best axes from a finished ok result.
func UpdateBest(b *Best, r Result) {
	if b == nil || r.Status != "ok" {
		return
	}
	s := r.Snapshot
	if b.Score == nil || s.Score > b.Score.Snapshot.Score {
		cp := r
		b.Score = &cp
	}
	if b.Throughput == nil || s.Throughput > b.Throughput.Snapshot.Throughput {
		cp := r
		b.Throughput = &cp
	}
	if b.Availability == nil || s.Availability > b.Availability.Snapshot.Availability {
		cp := r
		b.Availability = &cp
	}
	if b.Accuracy == nil || s.AvgAccuracy > b.Accuracy.Snapshot.AvgAccuracy {
		cp := r
		b.Accuracy = &cp
	}
}

// UpdateBestMobile revises mobile (perf/RAM) winners.
func UpdateBestMobile(b *BestMobile, r Result) {
	if b == nil || r.Status != "ok" {
		return
	}
	s := r.Snapshot
	if betterMobile(b.Score, s.MobileScore, s.Score, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileScore, x.Snapshot.Score, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.Score = &cp
	}
	if betterMobile(b.Throughput, s.MobileThroughput, s.Throughput, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileThroughput, x.Snapshot.Throughput, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.Throughput = &cp
	}
	if betterMobile(b.Availability, s.MobileAvailability, s.Availability, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileAvailability, x.Snapshot.Availability, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.Availability = &cp
	}
	if betterMobile(b.Accuracy, s.MobileAccuracy, s.AvgAccuracy, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileAccuracy, x.Snapshot.AvgAccuracy, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.Accuracy = &cp
	}
}

func betterMobile(cur *Result, newEff, newRaw float64, newBytes int64, get func(*Result) (eff, raw float64, bytes int64)) bool {
	if cur == nil {
		return true
	}
	curEff, curRaw, curBytes := get(cur)
	if newEff > curEff {
		return true
	}
	if newEff < curEff {
		return false
	}
	if newBytes > 0 && (curBytes == 0 || newBytes < curBytes) {
		return true
	}
	if newBytes == curBytes && newRaw > curRaw {
		return true
	}
	return false
}
