package dash

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestBoardCountsAndIdentity(t *testing.T) {
	tr := pulse.New()
	tr.SetMeta(0, 0, 3, 10, "hi")
	s := &Server{Tracker: tr, Task: "dense", ID: "dense", Addr: "127.0.0.1:8101", Epoch: 1, Cells: make([]permute.Cell, 10)}
	s.SignalStart()
	b := s.Board()
	if b.ID != "dense" || b.Task != "dense" || !b.Started {
		t.Fatalf("identity %+v", b)
	}
	if b.APIs["board"] != "/api/board" || b.APIs["live"] != "/api/live" {
		t.Fatalf("apis %+v", b.APIs)
	}
	m := s.Meta()
	if m.Ocean || m.ID != "dense" {
		t.Fatalf("meta %+v", m)
	}
}
