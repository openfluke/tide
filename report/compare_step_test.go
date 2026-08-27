package report

import "testing"

func TestCompareStepFamilies(t *testing.T) {
	pts := []taggedPoint{
		{Machine: "m4", LR: 2, LRLabel: "2", CellPoint: CellPoint{
			ID: "dense|int16|StepTweenSplit|lr=2", Mode: "StepTweenSplit", DType: "int16", Arch: "dense", Layer: "dense",
			Acc: 45, Score: 100, Avail: 30,
		}},
		{Machine: "m4", LR: 2, LRLabel: "2", CellPoint: CellPoint{
			ID: "dense|int16|TweenSplit|lr=2", Mode: "TweenSplit", DType: "int16", Arch: "dense", Layer: "dense",
			Acc: 40, Score: 90, Avail: 28,
		}},
		{Machine: "m4", LR: 2, LRLabel: "2", CellPoint: CellPoint{
			ID: "dense|int16|step_sgd|lr=2", Mode: "step_sgd", DType: "int16", Arch: "dense", Layer: "dense",
			Acc: 42, Score: 95, Avail: 25,
		}},
		{Machine: "m4", LR: 2, LRLabel: "2", CellPoint: CellPoint{
			ID: "dense|int16|sgd|lr=2", Mode: "sgd", DType: "int16", Arch: "dense", Layer: "dense",
			Acc: 38, Score: 80, Avail: 22,
		}},
	}
	fams := compareStepFamilies(pts, []string{"m4"})
	if len(fams) != 1 || len(fams[0].Pairs) < 2 {
		t.Fatalf("families=%+v", fams)
	}
	verdicts, byLR := compareStepVerdicts(fams)
	if len(verdicts) != 1 || verdicts[0].Headline == "" {
		t.Fatalf("verdicts=%+v", verdicts)
	}
	if len(byLR) == 0 {
		t.Fatal("expected step by LR rows")
	}
	var found bool
	for _, p := range fams[0].Pairs {
		if p.Step == "step_sgd" && p.Plain == "sgd" {
			found = true
			if p.PooledAccDelta < 3 || p.MatchN != 1 {
				t.Fatalf("step_sgd vs sgd: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("missing step_sgd/sgd pair")
	}
	cross := compareStepCross(fams)
	if len(cross) == 0 {
		t.Fatal("expected step cross")
	}
}
