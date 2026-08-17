package permute

import (
	"testing"

	"github.com/openfluke/welvet/layers/parallel"
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
		t.Fatalf("cnn: %s", g)
	}
	if g := (Cell{Arch: ArchBicameral}).ArchTag(); g != "bicameral×2" {
		t.Fatalf("bi: %s", g)
	}
	if g := (Cell{Arch: ArchTricameral}).ArchTag(); g != "tricameral×3" {
		t.Fatalf("tri: %s", g)
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
