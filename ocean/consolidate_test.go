package ocean

import (
	"testing"

	"github.com/openfluke/tide/dash"
	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
)

func okResult(mode permute.TrainMode, dt core.DType, score float64) *pulse.Result {
	c := permute.Cell{DType: dt, Format: quant.FormatNone, Mode: mode, Arch: permute.ArchCNN, Cams: 1}
	c.ID = c.String()
	r := pulse.Result{
		Cell:   c,
		Status: "ok",
		Snapshot: metrics.Snapshot{
			Score:       score,
			SoftAcc:     score,
			AvgAccuracy: score,
		},
	}
	return &r
}

func TestConsolidateVotesModeAndDType(t *testing.T) {
	peers := []PeerState{
		{Name: "dense", OK: true, URL: "http://d", Board: dash.Board{
			CellTotal: 10, Ok: 8,
			Best: pulse.Best{Score: okResult(permute.ModeSGD, core.DTypeFloat32, 40)},
			Leaderboard: []pulse.Result{*okResult(permute.ModeSGD, core.DTypeFloat32, 40)},
		}},
		{Name: "cnn2", OK: true, URL: "http://c", Board: dash.Board{
			CellTotal: 10, Ok: 7,
			Best: pulse.Best{Score: okResult(permute.ModeSGD, core.DTypeInt8, 22)},
			Leaderboard: []pulse.Result{*okResult(permute.ModeSGD, core.DTypeInt8, 22)},
		}},
		{Name: "mha", OK: true, URL: "http://m", Board: dash.Board{
			CellTotal: 10, Ok: 3,
			Best: pulse.Best{Score: okResult(permute.ModeTween, core.DTypeFloat32, 18)},
			Leaderboard: []pulse.Result{*okResult(permute.ModeTween, core.DTypeFloat32, 18)},
		}},
		{Name: "down", OK: false, Error: "dial"},
		{Name: "layernorm", OK: true, URL: "http://ln", Board: dash.Board{
			CellTotal: 10, Ok: 10,
			Best: pulse.Best{Score: okResult(permute.ModeSGD, core.DTypeFloat64, 0)},
		}},
	}
	h := consolidate(peers)
	if h.TidesUp != 4 || h.TidesTotal != 5 {
		t.Fatalf("up %d/%d", h.TidesUp, h.TidesTotal)
	}
	if h.BestMode != "sgd" {
		t.Fatalf("best mode %q want sgd", h.BestMode)
	}
	if h.BestDType != "float32" {
		t.Fatalf("best dtype %q want float32", h.BestDType)
	}
	if len(h.Layers) != 4 || h.Layers[0].Tide != "dense" {
		t.Fatalf("layers %+v", h.Layers)
	}
	if len(h.CombinedTop) != 3 {
		t.Fatalf("combined %d", len(h.CombinedTop))
	}
}

func TestConsolidateUsesEpochPlan(t *testing.T) {
	peers := []PeerState{
		{Name: "cnn2", OK: true, Board: dash.Board{Plan: 782, EpochDone: 291, Recorded: 1073, Ok: 291}},
		{Name: "dense", OK: true, Board: dash.Board{Plan: 782, EpochDone: 782, Recorded: 782, Ok: 782}},
	}
	h := consolidate(peers)
	if h.CellsDone != 291+782 || h.CellsTotal != 782+782 {
		t.Fatalf("cells %d/%d", h.CellsDone, h.CellsTotal)
	}
	by := map[string]LayerWinner{}
	for _, l := range h.Layers {
		by[l.Tide] = l
	}
	c := by["cnn2"]
	if c.Done != 291 || c.Total != 782 || c.Recorded != 1073 {
		t.Fatalf("cnn2 %+v", c)
	}
}
