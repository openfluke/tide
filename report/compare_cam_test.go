package report

import "testing"

func TestCompareCamPairs(t *testing.T) {
	pts := []taggedPoint{
		{Machine: "cam1", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Tide: "cam1-lo", ID: "dense|binary|sgd|lr=0.02", Acc: 20, Score: 100}},
		{Machine: "cam3", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Tide: "cam3-lo", ID: "dense|binary|sgd|lr=0.02", Acc: 25, Score: 120}},
		{Machine: "cam1", LR: 2, LRLabel: "2", CellPoint: CellPoint{Tide: "cam1-lo", ID: "dense|binary|sgd|lr=2", Acc: 30, Score: 200}},
		{Machine: "cam3", LR: 2, LRLabel: "2", CellPoint: CellPoint{Tide: "cam3-lo", ID: "dense|binary|sgd|lr=2", Acc: 28, Score: 190}},
	}
	lrMap := map[float64]string{0.02: "0.02", 2: "2"}
	pairs := compareCamPairs(pts, nil, []string{"cam1", "cam3"}, lrMap, 10)
	if len(pairs) != 1 {
		t.Fatalf("want 1 pair got %d", len(pairs))
	}
	p := pairs[0]
	if p.Base != "cam1" || p.Other != "cam3" || p.Band != "lo" {
		t.Fatalf("pair meta: %+v", p)
	}
	if len(p.ByLR) != 2 {
		t.Fatalf("by_lr: %+v", p.ByLR)
	}
	if p.ByLR[0].MeanAccDelta != 5 {
		t.Fatalf("delta 0.02: %v", p.ByLR[0].MeanAccDelta)
	}
}
