package report

import "testing"

func TestCompareModeCamFamilies(t *testing.T) {
	pts := []taggedPoint{
		{Machine: "cam1", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Mode: "sgd", Acc: 20, Soft: 22, Score: 100, Avail: 40}},
		{Machine: "cam1", LR: 2, LRLabel: "2", CellPoint: CellPoint{Mode: "sgd", Acc: 30, Soft: 32, Score: 200, Avail: 45}},
		{Machine: "cam1", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Mode: "step_sgd", Acc: 22, Soft: 24, Score: 110, Avail: 38}},
		{Machine: "cam1", LR: 2, LRLabel: "2", CellPoint: CellPoint{Mode: "step_sgd", Acc: 28, Soft: 30, Score: 190, Avail: 42}},
		{Machine: "cam3", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Mode: "sgd", Acc: 25, Soft: 26, Score: 120, Avail: 50}},
		{Machine: "cam3", LR: 2, LRLabel: "2", CellPoint: CellPoint{Mode: "sgd", Acc: 35, Soft: 36, Score: 250, Avail: 55}},
		{Machine: "cam3", LR: 0.02, LRLabel: "0.02", CellPoint: CellPoint{Mode: "step_sgd", Acc: 27, Soft: 28, Score: 130, Avail: 48}},
		{Machine: "cam3", LR: 2, LRLabel: "2", CellPoint: CellPoint{Mode: "step_sgd", Acc: 33, Soft: 34, Score: 240, Avail: 52}},
	}
	fams := compareModeCamFamilies(pts, []string{"cam1", "cam3"}, 10)
	if len(fams) == 0 {
		t.Fatal("expected families")
	}
	var sgd *CompareModeCamFamily
	for i := range fams {
		if modeKey(fams[i].Plain) == "sgd" || modeKey(fams[i].Family) == "sgd" {
			sgd = &fams[i]
			break
		}
	}
	if sgd == nil {
		t.Fatalf("no sgd family in %+v", fams)
	}
	if sgd.Step == "" {
		t.Fatal("expected step mate")
	}
	if len(sgd.Series) != 4 {
		t.Fatalf("want 4 series (2 cams × plain/step), got %d: %+v", len(sgd.Series), sgd.Series)
	}
}
