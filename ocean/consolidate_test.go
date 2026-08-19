package ocean

import (
	"testing"

	"github.com/openfluke/tide/dash"
	"github.com/openfluke/tide/report"
)

func TestCollectOceanPointsDedup(t *testing.T) {
	pt := report.CellPoint{Tide: "dense", ID: "int8", Score: 85, Soft: 72, Acc: 82, Thru: 180, RAMKiB: 180}
	row := report.LPDRow{Tide: "dense", ID: "int8", Score: 85, Soft: 72, Acc: 82, Thru: 180, RAMKiB: 180}
	peers := []PeerState{{
		OK: true, Name: "dense",
		Board: dash.Board{
			LR:   0.03,
			Heat: report.Heat{Points: []report.CellPoint{pt, pt}},
			LPD:  report.LPD{Top: []report.LPDRow{row}, Gold: []report.LPDRow{row}},
		},
	}}
	got := collectOceanPoints(peers, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 unique cell, got %d", len(got))
	}
	lpd := report.BuildLPD(got)
	if lpd.N != 1 || lpd.Champ.ID != "int8" {
		t.Fatalf("rebuild %+v", lpd)
	}
}

func TestLayerWinnerLR(t *testing.T) {
	h := consolidate([]PeerState{{OK: true, Name: "dense", Board: dash.Board{LR: 0.03}}})
	if len(h.Layers) != 1 || h.Layers[0].LR != 0.03 {
		t.Fatalf("layers %+v", h.Layers)
	}
}
