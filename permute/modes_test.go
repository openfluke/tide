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
