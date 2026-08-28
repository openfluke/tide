package dash

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestEpochProgressMulti(t *testing.T) {
	tr := pulse.New()
	s := &Server{Tracker: tr, Task: "yap", Epoch: 2, EpochMax: 5, Cells: make([]permute.Cell, 10)}
	b := s.Board()
	if b.EpochMax != 5 || b.EpochsLeft != 4 {
		t.Fatalf("epoch_max=%d left=%d want 5/4", b.EpochMax, b.EpochsLeft)
	}
	// 0 cells done → overall = (1 + 0)/5 * 100 = 20%
	if b.EpochOverallPct < 19.9 || b.EpochOverallPct > 20.1 {
		t.Fatalf("overall pct %v want ~20", b.EpochOverallPct)
	}
}

func TestCountResultsThisEpoch(t *testing.T) {
	tr := pulse.New()
	cell := permute.Cell{ID: "float32|none|sgd|single|simd", Mode: permute.ModeSGD}
	s := &Server{Tracker: tr, Task: "cnn2", ID: "cnn2", Epoch: 2, Cells: []permute.Cell{cell}}
	s.SignalStart()
	tr.Restore([]pulse.Result{
		{Status: "ok", Epoch: 1, Cell: cell},
		{Status: "ok", Epoch: 1, Cell: cell},
		{Status: "ok", Epoch: 2, Cell: cell},
	}, pulse.Best{}, pulse.BestMobile{}, pulse.BestLearn{}, pulse.BestLearnMobile{}, nil, 1, 1, "e2")
	b := s.Board()
	if b.Ok != 1 || b.OkAll != 3 || b.Recorded != 3 || b.Plan != 1 {
		t.Fatalf("board ok=%d okAll=%d rec=%d plan=%d", b.Ok, b.OkAll, b.Recorded, b.Plan)
	}
	if b.ProgressPct > 100 {
		t.Fatalf("pct %v", b.ProgressPct)
	}
	if b.EpochDone != 1 {
		t.Fatalf("epoch_done %d", b.EpochDone)
	}
}

func TestBoardCountsAndIdentity(t *testing.T) {
	tr := pulse.New()
	tr.SetMeta(0, 0, 3, 10, "hi")
	s := &Server{Tracker: tr, Task: "dense", ID: "dense", Addr: "127.0.0.1:8101", Epoch: 1, Cells: make([]permute.Cell, 10)}
	s.SignalStart()
	b := s.Board()
	if b.ID != "dense" || b.Task != "dense" || !b.Started {
		t.Fatalf("identity %+v", b)
	}
	if b.Status != "queued" {
		t.Fatalf("status %q want queued (started, not running, 0 done)", b.Status)
	}
	if b.APIs["board"] != "/api/board" || b.APIs["live"] != "/api/live" {
		t.Fatalf("apis %+v", b.APIs)
	}
	m := s.Meta()
	if m.Ocean || m.ID != "dense" {
		t.Fatalf("meta %+v", m)
	}
}

func TestBoardLR(t *testing.T) {
	tr := pulse.New()
	s := &Server{Tracker: tr, Task: "dense", ID: "dense", LR: 0.07, Cells: make([]permute.Cell, 1)}
	b := s.Board()
	if b.LR != 0.07 {
		t.Fatalf("board lr %v", b.LR)
	}
	if s.Meta().LR != 0.07 {
		t.Fatalf("meta lr %v", s.Meta().LR)
	}
	if r := b.ToReport(nil); r.LR != 0.07 {
		t.Fatalf("report lr %v", r.LR)
	}
}
