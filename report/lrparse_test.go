package report

import "testing"

func TestParseLRFromCellID(t *testing.T) {
	tests := []struct {
		id    string
		want  float64
		label string
	}{
		{"dense|int16|MeshBP|lr=0.02", 0.02, "0.02"},
		{"dense|f32|sgd|lr=2", 2, "2"},
		{"dense|f32|sgd|lr=200", 200, "200"},
		{"dense|f32|sgd|lr=2k", 2000, "2k"},
		{"dense|f32|sgd|lr=1m", 1e6, "1m"},
		{"dense|f32|sgd|lr=20000", 20000, "20000"},
	}
	for _, tc := range tests {
		v, lbl, ok := ParseLRFromCellID(tc.id)
		if !ok || v != tc.want || lbl != tc.label {
			t.Fatalf("%q → got %v %q ok=%v want %v %q", tc.id, v, lbl, ok, tc.want, tc.label)
		}
	}
	if _, _, ok := ParseLRFromCellID("dense|f32|sgd"); ok {
		t.Fatal("expected no lr")
	}
}

func TestRecipeKey(t *testing.T) {
	if got := RecipeKey("dense|int16|MeshBP|lr=0.02"); got != "dense|int16|MeshBP" {
		t.Fatalf("recipe key: %q", got)
	}
}

func TestParseMachineFromPeer(t *testing.T) {
	if ParseMachineFromPeer("m4-lo") != "m4" {
		t.Fatal("m4-lo")
	}
	if ParseMachineFromPeer("m5_hi") != "m5" {
		t.Fatal("m5_hi")
	}
}

func TestBuildCompare(t *testing.T) {
	mk := func(name, id, mode string, acc float64) NamedTideReport {
		return NamedTideReport{
			Name: name,
			Report: TideReport{
				Cells: []CellPoint{{
					ID: id, Mode: mode, DType: "int16", Arch: "dense", Layer: "dense",
					Acc: acc, Score: acc * 10, Avail: 80, Soft: acc + 5,
				}},
			},
		}
	}
	c := BuildCompare("test", []NamedTideReport{
		mk("m4-lo", "dense|int16|MeshBP|lr=0.02", "MeshBP", 40),
		mk("m5-lo", "dense|int16|MeshBP|lr=0.02", "MeshBP", 50),
		mk("m4-lo", "dense|int16|sgd|lr=0.02", "sgd", 35),
		mk("m5-lo", "dense|int16|sgd|lr=0.02", "sgd", 38),
		mk("m4-lo", "dense|int16|MeshBP|lr=2", "MeshBP", 45),
		mk("m5-lo", "dense|int16|MeshBP|lr=2", "MeshBP", 44),
		mk("m4-lo", "dense|int16|sgd|lr=2", "sgd", 42),
		mk("m5-lo", "dense|int16|sgd|lr=2", "sgd", 40),
	})
	if len(c.Machines) != 2 || len(c.LRs) != 2 {
		t.Fatalf("machines=%d lrs=%d", len(c.Machines), len(c.LRs))
	}
	if len(c.Matched) == 0 {
		t.Fatal("expected matched rows")
	}
	if len(c.ModeSeries) == 0 {
		t.Fatal("expected mode series")
	}
	if len(c.VsBaseline) != 2 {
		t.Fatalf("vs baseline grids=%d want 2", len(c.VsBaseline))
	}
	if c.VsBaseline[0].Baseline == "" || len(c.VsBaseline[0].Modes) == 0 {
		t.Fatalf("vs baseline: %+v", c.VsBaseline[0])
	}
	pdf, err := PDFCompare(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 500 || string(pdf[:4]) != "%PDF" {
		t.Fatalf("compare pdf too short %d", len(pdf))
	}
}
