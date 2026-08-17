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
	}
	h := consolidate(peers)
	if h.TidesUp != 3 || h.TidesTotal != 4 {
		t.Fatalf("up %d/%d", h.TidesUp, h.TidesTotal)
	}
	if h.BestMode != "sgd" {
		t.Fatalf("best mode %q want sgd", h.BestMode)
	}
	if h.BestDType != "float32" {
		t.Fatalf("best dtype %q want float32", h.BestDType)
	}
	if len(h.Layers) != 3 || h.Layers[0].Tide != "dense" {
		t.Fatalf("layers %+v", h.Layers)
	}
	if len(h.CombinedTop) != 3 {
		t.Fatalf("combined %d", len(h.CombinedTop))
	}
}
