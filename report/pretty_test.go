package report

import "testing"

func TestPrettyCellAndArch(t *testing.T) {
	if PrettyCell("fp6|none|sgd|cnn|simd") != "fp6|none|sgd|single|simd" {
		t.Fatal(PrettyCell("fp6|none|sgd|cnn|simd"))
	}
	if PrettyArch("cnn") != "single" {
		t.Fatal(PrettyArch("cnn"))
	}
	if PrettyArch("single×1") != "single×1" {
		t.Fatal(PrettyArch("single×1"))
	}
	if PrettyArch("bicameral×2") != "bicameral×2" {
		t.Fatal(PrettyArch("bicameral×2"))
	}
}

func TestBuildHeat(t *testing.T) {
	pts := []CellPoint{
		{Tide: "mha", Mode: "sgd", DType: "float32", Arch: "single", Score: 10, Soft: 40, Acc: 50},
		{Tide: "mha", Mode: "sgd", DType: "int8", Arch: "single", Score: 6, Soft: 20, Acc: 30},
		{Tide: "dense", Mode: "tween", DType: "float32", Arch: "bicameral", Score: 4, Soft: 10, Acc: 12},
	}
	h := BuildHeat(pts)
	if len(h.Modes) != 2 || len(h.DTypes) != 2 {
		t.Fatalf("axes %+v %+v", h.Modes, h.DTypes)
	}
	if h.ModeMeanScore[0] == 0 && h.ModeMeanScore[1] == 0 {
		t.Fatal("means empty")
	}
}
