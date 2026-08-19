package permute

import (
	"strings"
	"testing"

	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/layers/parallel"
	"github.com/openfluke/welvet/quant"
)

func TestAllModesKeepsLucyIDs(t *testing.T) {
	lucy := LucyModes()
	all := AllModes()
	if len(all) != len(parallel.AllNamedTrainModes()) {
		t.Fatalf("AllModes=%d want %d named Welvet modes", len(all), len(parallel.AllNamedTrainModes()))
	}
	for i, m := range lucy {
		if all[i] != m {
			t.Fatalf("Lucy token %d: got %s want %s (checkpoint IDs must stay frozen)", i, all[i], m)
		}
	}
	seen := map[TrainMode]bool{}
	for _, m := range all {
		if seen[m] {
			t.Fatalf("duplicate mode %s", m)
		}
		seen[m] = true
		if _, err := m.Welvet(); err != nil {
			t.Fatalf("Welvet(%s): %v", m, err)
		}
	}
}

func TestScreenIsCNNLucy(t *testing.T) {
	cells := Expand(Screen())
	full := Expand(Full())
	if len(cells) == 0 || len(cells) >= len(full) {
		t.Fatalf("screen=%d full=%d", len(cells), len(full))
	}
	for _, c := range cells {
		if c.Arch != ArchCNN {
			t.Fatalf("screen arch %s", c.Arch)
		}
	}
}

func TestArchTag(t *testing.T) {
	if g := (Cell{Arch: ArchCNN}).ArchTag(); g != "single×1" {
		t.Fatalf("single: %s", g)
	}
	if g := (Cell{Arch: ArchBicameral}).ArchTag(); g != "bicameral×2" {
		t.Fatalf("bi: %s", g)
	}
	if g := (Cell{Arch: ArchTricameral}).ArchTag(); g != "tricameral×3" {
		t.Fatalf("tri: %s", g)
	}
	wide := Cell{Arch: ArchForCams(12), Cams: 12}
	if g := wide.ArchTag(); g != "cameral×12" {
		t.Fatalf("cam12: %s", g)
	}
}

func TestParseCamsAndExpand(t *testing.T) {
	got, err := ParseCams("4-15")
	if err != nil {
		t.Fatal(err)
	}
	want := CamsRange(4, 15)
	if len(got) != len(want) {
		t.Fatalf("4-15 len %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("4-15[%d]=%d", i, got[i])
		}
	}
	named, err := ParseCams("single,tri,cameral×8")
	if err != nil || len(named) != 3 || named[0] != 1 || named[1] != 3 || named[2] != 8 {
		t.Fatalf("named %v %v", named, err)
	}
	if _, err := ParseCams("nope"); err == nil {
		t.Fatal("expected unknown cameral")
	}
	cells := Expand(Config{
		DTypes:  []core.DType{core.DTypeFloat32},
		Formats: []quant.Format{quant.FormatNone},
		Modes:   []TrainMode{ModeSGD},
		Cams:    []int{4, 15},
	})
	if len(cells) != 2 {
		t.Fatalf("expand cams %d", len(cells))
	}
	if cells[0].Cams != 4 || cells[0].Arch != ArchForCams(4) || !strings.Contains(cells[0].ID, "|cameral×4|") {
		t.Fatalf("cam4 %+v", cells[0])
	}
	if cells[1].Cams != 15 || cells[1].ArchTag() != "cameral×15" {
		t.Fatalf("cam15 %+v", cells[1])
	}
	// Named 1–3 still win when Cams is empty (live_mnist / Sprint).
	legacy := Expand(Config{
		DTypes:  []core.DType{core.DTypeFloat32},
		Formats: []quant.Format{quant.FormatNone},
		Modes:   []TrainMode{ModeSGD},
		Arches:  AllArches(),
	})
	if len(legacy) != 3 || legacy[0].Arch != ArchSingle || legacy[2].Arch != ArchTricameral {
		t.Fatalf("legacy %+v", legacy)
	}
}

func TestSprintIsNoneAllArches(t *testing.T) {
	cells := Expand(Sprint())
	if len(cells) == 0 {
		t.Fatal("sprint empty")
	}
	full := Expand(Full())
	if len(cells) >= len(full) {
		t.Fatalf("sprint=%d should be < full=%d", len(cells), len(full))
	}
	seen := map[ArchKind]int{}
	for _, c := range cells {
		seen[c.Arch]++
		if c.Format != 0 { // FormatNone
			t.Fatalf("sprint format %v", c.Format)
		}
		if c.Cams != CamsOf(c.Arch) {
			t.Fatalf("cams %d for %s", c.Cams, c.Arch)
		}
	}
	for _, a := range AllArches() {
		if seen[a] == 0 {
			t.Fatalf("missing arch %s", a)
		}
	}
	if seen[ArchCNN] != seen[ArchBicameral] || seen[ArchCNN] != seen[ArchTricameral] {
		t.Fatalf("uneven arches %+v", seen)
	}
}

func TestCanonicalArchAndIDs(t *testing.T) {
	if CanonicalArch("cnn") != ArchSingle || CanonicalArch(ArchCNN) != ArchSingle {
		t.Fatalf("cnn → %s", CanonicalArch("cnn"))
	}
	c := Cell{Mode: ModeSGD, Arch: "cnn"}
	if !strings.Contains(c.String(), "|single|") || strings.Contains(c.String(), "|cnn|") {
		t.Fatalf("id %s", c.String())
	}
	old := "fp6|none|sgd|cnn|simd"
	if NormalizeCellID(old) != "fp6|none|sgd|single|simd" {
		t.Fatalf("norm %s", NormalizeCellID(old))
	}
	done := map[string]bool{old: true}
	if !IDDone(done, "fp6|none|sgd|single|simd") {
		t.Fatal("alias miss")
	}
}

func TestParseModes(t *testing.T) {
	ms, err := ParseModes("sgd,step_sgd,tween")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 || ms[0] != ModeSGD || ms[1] != ModeStepSGD || ms[2] != ModeTween {
		t.Fatalf("%v", ms)
	}
	alias, err := ParseModes("NormalBP")
	if err != nil || len(alias) != 1 || alias[0] != ModeSGD {
		t.Fatalf("alias %v %v", alias, err)
	}
	if _, err := ParseModes("not-a-mode"); err == nil {
		t.Fatal("expected unknown mode error")
	}
	all, err := ParseModes("all")
	if err != nil || all != nil {
		t.Fatalf("all: %v %v", all, err)
	}
}
