package report

import "testing"

func TestPickBaselineDynamic(t *testing.T) {
	if g := PickBaseline([]string{"Sparse", "FastProxy", "sgd", "step_sgd"}); g != "sgd" {
		t.Fatalf("lucy board: %q", g)
	}
	if g := PickBaseline([]string{"StepBP", "NormalBP", "TweenSplitSparse"}); g != "NormalBP" {
		t.Fatalf("welvet board: %q", g)
	}
	if g := PickBaseline([]string{"Sparse", "FastProxy"}); g != "" {
		t.Fatalf("no bp analog: %q", g)
	}
}

func TestVsMatchedDeltas(t *testing.T) {
	pts := []CellPoint{
		{Mode: "sgd", DType: "float32", Arch: "cameral×4", Acc: 50, Soft: 20, Avail: 12, Thru: 100, Score: 6},
		{Mode: "sgd", DType: "int8", Arch: "cameral×4", Acc: 40, Soft: 10, Avail: 12, Thru: 90, Score: 4},
		{Mode: "Sparse", DType: "float32", Arch: "cameral×4", Acc: 48, Soft: 22, Avail: 45, Thru: 200, Score: 40},
		{Mode: "Sparse", DType: "int8", Arch: "cameral×4", Acc: 30, Soft: 8, Avail: 44, Thru: 180, Score: 30},
		{Mode: "tween", DType: "float32", Arch: "cameral×4", Acc: 20, Soft: 5, Avail: 15, Thru: 110, Score: 3},
		{Mode: "step_sgd", DType: "float32", Arch: "cameral×4", Acc: 50.2, Soft: 20, Avail: 12, Thru: 100, Score: 6},
		{Mode: "step_sgd", DType: "int8", Arch: "cameral×4", Acc: 40, Soft: 10, Avail: 12, Thru: 90, Score: 4},
	}
	h := BuildHeat(pts)
	if h.Vs == nil || h.Vs.Baseline != "sgd" {
		t.Fatalf("vs %+v", h.Vs)
	}
	if len(h.Vs.Modes) != 3 {
		t.Fatalf("modes %v", h.Vs.Modes)
	}
	by := map[string]VsMode{}
	for _, m := range h.Vs.Modes {
		by[m.Mode] = m
	}
	sp := by["Sparse"]
	if sp.N != 2 {
		t.Fatalf("sparse n %d", sp.N)
	}
	// float32 AccΔ = 48-50 = -2; int8 AccΔ = 30-40 = -10; mean = -6
	if sp.AccDelta < -6.1 || sp.AccDelta > -5.9 {
		t.Fatalf("sparse accΔ %.3f", sp.AccDelta)
	}
	if sp.AvailDelta < 30 {
		t.Fatalf("sparse should win avail, got %.1f", sp.AvailDelta)
	}
	tw := by["tween"]
	if tw.N != 1 || tw.AccDelta > -29 {
		t.Fatalf("tween %+v", tw)
	}
	if len(h.Vs.ByDType) == 0 || len(h.Vs.ByArch) == 0 {
		t.Fatal("expected sparse bins")
	}
	if len(h.Vs.Families) != 1 || h.Vs.Families[0].Plain != "sgd" || h.Vs.Families[0].Step != "step_sgd" {
		t.Fatalf("family %+v", h.Vs.Families)
	}
	if h.Vs.Families[0].MeanAbsAcc > 0.3 {
		t.Fatalf("step vs sgd should collapse, |AccΔ|=%.3f", h.Vs.Families[0].MeanAbsAcc)
	}
}

func TestVsNoMatchDifferentArch(t *testing.T) {
	pts := []CellPoint{
		{Mode: "sgd", DType: "float32", Arch: "cameral×4", Acc: 50, Score: 6},
		{Mode: "Sparse", DType: "float32", Arch: "cameral×15", Acc: 90, Score: 40},
	}
	h := BuildHeat(pts)
	if h.Vs != nil {
		t.Fatalf("unmatched arch should not produce vs: %+v", h.Vs)
	}
}
