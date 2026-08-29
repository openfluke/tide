package report

import (
	"strings"
	"testing"
)

func TestScatterSVGEmpty(t *testing.T) {
	b := ScatterSVG(nil, "availability", "avg_accuracy", "Avail vs Acc")
	if len(b) < 100 {
		t.Fatalf("short svg %d", len(b))
	}
}

func TestRadarFromLPD(t *testing.T) {
	l := LPD{
		PeakAcc:  100,
		PeakThru: 50,
		PeakAvail: 80,
		PeakDensAcc: 1, PeakDensThru: 1, PeakDensAvail: 1,
		AccChamp: LPDChamp{ID: "acc-only", Mode: "sgd", DType: "f32", Arch: "cam×1", Acc: 100, Thru: 10, Avail: 40, RAMKiB: 500},
		Champ:    LPDChamp{ID: "score-win", Mode: "fp", DType: "f32", Arch: "cam×2", Acc: 96, Thru: 50, Avail: 80, RAMKiB: 100, Score: 384},
		LeanChamp: LPDRow{ID: "lean", Mode: "fp", DType: "f32", Arch: "cam×3", Acc: 96, Thru: 40, Avail: 70, RAMKiB: 80,
			RelAcc: 0.96, RelThru: 0.8, RelAvail: 0.875, DensAcc: 6, DensThru: 5, DensAvail: 5.5},
		// Acc champ deliberately missing from Top — used to empty the radar before the fix.
		Top: []LPDRow{{ID: "other", RelAcc: 0.8, RelThru: 0.7, RelAvail: 0.6, LPD: 1,
			DensAcc: 2, DensThru: 1.5, DensAvail: 1.2}},
	}
	live, dens := RadarFromLPD(l)
	if len(live) < 2 {
		t.Fatalf("live radar series %d want ≥2 (Acc champ synth + lean)", len(live))
	}
	foundAcc := false
	for _, s := range live {
		if strings.Contains(s.Label, "Acc champ") {
			foundAcc = true
			if s.Vals[0] < 0.99 {
				t.Fatalf("Acc champ RelAcc = %v want ~1", s.Vals)
			}
		}
	}
	if !foundAcc {
		t.Fatal("Acc champ missing from live radar")
	}
	if len(dens) == 0 {
		t.Fatal("expected density radar series")
	}
	svg := RadarSVG("radar", live)
	if len(svg) < 200 {
		t.Fatalf("short radar svg %d", len(svg))
	}
	_ = dens
}
