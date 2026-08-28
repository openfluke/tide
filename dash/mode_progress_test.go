package dash

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestModeProgressCountsUniquePlanCells(t *testing.T) {
	cells := []permute.Cell{
		{ID: "float32|none|sgd|single|simd|lr=0.6", Mode: permute.ModeSGD},
		{ID: "int8|none|sgd|single|simd|lr=0.6", Mode: permute.ModeSGD},
		{ID: "float32|none|tween|single|simd|lr=0.6", Mode: permute.ModeTween},
	}
	tr := pulse.New()
	s := &Server{Tracker: tr, Cells: cells, Epoch: 1}
	tr.Restore([]pulse.Result{
		{Status: "ok", Epoch: 1, Cell: cells[0]},
		{Status: "ok", Epoch: 1, Cell: cells[0]},
		{Status: "ok", Epoch: 1, Cell: cells[0]},
	}, pulse.Best{}, pulse.BestMobile{}, pulse.BestLearn{}, pulse.BestLearnMobile{}, nil, 1, 3, "test")
	live := tr.SnapshotLive()
	mp := s.modeProgress(live)
	if len(mp) != 2 {
		t.Fatalf("modes %d want 2", len(mp))
	}
	var sgd ModeProgress
	for _, m := range mp {
		if m.Mode == string(permute.ModeSGD) {
			sgd = m
		}
	}
	if sgd.Done != 1 || sgd.Left != 1 || sgd.Total != 2 {
		t.Fatalf("sgd progress %+v want done=1 left=1 total=2", sgd)
	}
}

func TestBoardProgressUsesSeededDoneIDs(t *testing.T) {
	cells := make([]permute.Cell, 10)
	for i := range cells {
		cells[i] = permute.Cell{ID: "id" + string(rune('a'+i)), Mode: permute.ModeSGD}
	}
	tr := pulse.New()
	// Simulate checkpoint DoneIDs for 7 cells never present in trimmed Completed.
	ids := make([]string, 7)
	for i := 0; i < 7; i++ {
		ids[i] = cells[i].ID
	}
	tr.SeedDoneIDs(ids)
	s := &Server{Tracker: tr, Cells: cells, Epoch: 1}
	s.SignalStart()
	b := s.Board()
	if b.EpochDone != 7 {
		t.Fatalf("epoch_done %d want 7", b.EpochDone)
	}
	if b.ProgressPct < 69.9 || b.ProgressPct > 70.1 {
		t.Fatalf("progress_pct %v want ~70", b.ProgressPct)
	}
	if b.Status == "running" {
		t.Fatalf("status %q want idle/queued (not running)", b.Status)
	}
}

func TestBoardStatusDoneBeatsStuckRunning(t *testing.T) {
	cell := permute.Cell{ID: "only", Mode: permute.ModeSGD}
	tr := pulse.New()
	tr.Restore([]pulse.Result{
		{Status: "ok", Epoch: 1, Cell: cell},
	}, pulse.Best{}, pulse.BestMobile{}, pulse.BestLearn{}, pulse.BestLearnMobile{}, nil, 1, 1, "done")
	tr.BeginEpoch(cell, 1, "A") // stuck running flag
	s := &Server{Tracker: tr, Cells: []permute.Cell{cell}, Epoch: 1}
	s.SignalStart()
	b := s.Board()
	if b.Status != "done" {
		t.Fatalf("status %q want done", b.Status)
	}
	if b.Running {
		t.Fatal("board.Running should be false when plan complete")
	}
}

func TestBoardProgressPctIgnoresDuplicateCompletions(t *testing.T) {
	cells := make([]permute.Cell, 10)
	for i := range cells {
		cells[i] = permute.Cell{ID: "id" + string(rune('a'+i)), Mode: permute.ModeSGD}
	}
	tr := pulse.New()
	s := &Server{Tracker: tr, Cells: cells, Epoch: 1}
	var done []pulse.Result
	for i := 0; i < 3; i++ {
		done = append(done, pulse.Result{Status: "ok", Epoch: 1, Cell: cells[0]})
	}
	tr.Restore(done, pulse.Best{}, pulse.BestMobile{}, pulse.BestLearn{}, pulse.BestLearnMobile{}, nil, 0, 10, "test")
	b := s.Board()
	if b.EpochDone != 1 {
		t.Fatalf("epoch_done %d want 1", b.EpochDone)
	}
	if b.ProgressPct > 10.1 || b.ProgressPct < 9.9 {
		t.Fatalf("progress_pct %v want ~10", b.ProgressPct)
	}
}
