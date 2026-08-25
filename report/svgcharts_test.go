package report

import "testing"

func TestScatterSVGEmpty(t *testing.T) {
	b := ScatterSVG(nil, "availability", "avg_accuracy", "Avail vs Acc")
	if len(b) < 100 {
		t.Fatalf("short svg %d", len(b))
	}
}

func TestRadarFromLPD(t *testing.T) {
	l := LPD{
		AccChamp: LPDChamp{ID: "a"},
		Champ:    LPDChamp{ID: "b"},
		Top:      []LPDRow{{ID: "a", RelAcc: 0.8, RelThru: 0.7, RelAvail: 0.6, LPD: 1}},
	}
	live, dens := RadarFromLPD(l)
	if len(live) == 0 {
		t.Fatal("expected live radar series")
	}
	svg := RadarSVG("radar", live)
	if len(svg) < 200 {
		t.Fatalf("short radar svg %d", len(svg))
	}
	_ = dens
}
