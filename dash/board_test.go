package dash

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestCountResultsThisEpoch(t *testing.T) {
	tr := pulse.New()
	s := &Server{Tracker: tr, Task: "cnn2", ID: "cnn2", Epoch: 2, Cells: make([]permute.Cell, 782)}
	s.SignalStart()
	tr.Restore([]pulse.Result{
		{Status: "ok", Epoch: 1},
		{Status: "ok", Epoch: 1},
		{Status: "ok", Epoch: 2},
	}, pulse.Best{}, pulse.BestMobile{}, pulse.BestLearn{}, pulse.BestLearnMobile{}, nil, 1, 782, "e2")
	b := s.Board()
	if b.Ok != 1 || b.OkAll != 3 || b.Recorded != 3 || b.Plan != 782 {
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
