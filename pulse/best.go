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

// BestLearn holds learning-speed winners (raw wall-clock / rate).
type BestLearn struct {
	To25      *Result `json:"to25,omitempty"`        // fastest time to 25% window acc
	To50      *Result `json:"to50,omitempty"`        // fastest time to 50% window acc
	AccPerSec *Result `json:"acc_per_sec,omitempty"` // highest AvgAccuracy / second
}

// BestLearnMobile holds learning-speed per MiB winners.
type BestLearnMobile struct {
	AccPerSec *Result `json:"acc_per_sec,omitempty"` // highest AccPerSec / MiB
	To50      *Result `json:"to50,omitempty"`        // best (1/time_to_50)/MiB
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
	if b.Accuracy == nil || s.SoftAcc > b.Accuracy.Snapshot.SoftAcc {
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
	if betterMobile(b.Accuracy, s.MobileAccuracy, s.SoftAcc, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileAccuracy, x.Snapshot.SoftAcc, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.Accuracy = &cp
	}
}

// UpdateBestLearn revises learning-speed winners.
func UpdateBestLearn(b *BestLearn, r Result) {
	if b == nil || r.Status != "ok" {
		return
	}
	s := r.Snapshot
	if s.TimeToAcc25Sec > 0 && (b.To25 == nil || s.TimeToAcc25Sec < b.To25.Snapshot.TimeToAcc25Sec) {
		cp := r
		b.To25 = &cp
	}
	if s.TimeToAcc50Sec > 0 && (b.To50 == nil || s.TimeToAcc50Sec < b.To50.Snapshot.TimeToAcc50Sec) {
		cp := r
		b.To50 = &cp
	}
	if s.AccPerSec > 0 && (b.AccPerSec == nil || s.AccPerSec > b.AccPerSec.Snapshot.AccPerSec) {
		cp := r
		b.AccPerSec = &cp
	}
}

// UpdateBestLearnMobile revises learning-speed / MiB winners.
func UpdateBestLearnMobile(b *BestLearnMobile, r Result) {
	if b == nil || r.Status != "ok" {
		return
	}
	s := r.Snapshot
	if betterMobile(b.AccPerSec, s.MobileAccPerSec, s.AccPerSec, s.WeightBytes, func(x *Result) (float64, float64, int64) {
		return x.Snapshot.MobileAccPerSec, x.Snapshot.AccPerSec, x.Snapshot.WeightBytes
	}) {
		cp := r
		b.AccPerSec = &cp
	}
	// Mobile time-to-50: higher (1/sec)/MiB = reached 50% faster per byte.
	if s.TimeToAcc50Sec > 0 {
		mb := s.WeightMiB
		if mb < 1e-9 {
			mb = 1e-9
		}
		eff := (1.0 / s.TimeToAcc50Sec) / mb
		curEff := 0.0
		if b.To50 != nil && b.To50.Snapshot.TimeToAcc50Sec > 0 {
			cmb := b.To50.Snapshot.WeightMiB
			if cmb < 1e-9 {
				cmb = 1e-9
			}
			curEff = (1.0 / b.To50.Snapshot.TimeToAcc50Sec) / cmb
		}
		if b.To50 == nil || eff > curEff {
			cp := r
			b.To50 = &cp
		}
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
