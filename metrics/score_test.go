package metrics

import (
	"testing"
	"time"
)

func TestLucyScoreEquation(t *testing.T) {
	s := Snapshot{
		TotalOutputs: 1000,
		TotalCorrect: 800,
		BlockedTrain: 200 * time.Millisecond,
		Duration:     time.Second,
		Windows: []Window{
			{Accuracy: 80},
			{Accuracy: 70},
		},
	}
	Finalize(&s)
	// thru=1000, avail=80, avgAcc=75 → score = 1000*80*75/10000 = 600
	if s.Throughput != 1000 {
		t.Fatalf("throughput %v", s.Throughput)
	}
	if s.Availability != 80 {
		t.Fatalf("availability %v", s.Availability)
	}
	if s.AvgAccuracy != 75 {
		t.Fatalf("avg accuracy %v", s.AvgAccuracy)
	}
	if s.Score != 600 {
		t.Fatalf("score %v want 600", s.Score)
	}
}
