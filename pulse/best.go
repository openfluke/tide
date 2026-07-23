package pulse

// Best holds the top Result for each Lucy axis + composite Score.
type Best struct {
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
