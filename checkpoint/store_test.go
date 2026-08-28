package checkpoint

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestNormalizeDedupesDoneIDsAndCompleted(t *testing.T) {
	p := &Progress{
		DoneIDs: []string{"a", "a", "b", "b", "b"},
		Completed: []pulse.Result{
			{Status: "ok", Epoch: 1, Cell: permute.Cell{ID: "x|lr=0.6"}},
			{Status: "gap", Epoch: 1, Cell: permute.Cell{ID: "x|lr=0.6"}},
			{Status: "ok", Epoch: 1, Cell: permute.Cell{ID: "y|lr=0.6"}},
		},
	}
	Normalize(p)
	if len(p.DoneIDs) != 2 {
		t.Fatalf("done_ids len %d want 2", len(p.DoneIDs))
	}
	if len(p.Completed) != 2 {
		t.Fatalf("completed len %d want 2", len(p.Completed))
	}
	if p.Completed[0].Status != "gap" {
		t.Fatalf("kept last result for duplicate cell, got %q", p.Completed[0].Status)
	}
}

func TestPlanDoneCountUniquePlanCells(t *testing.T) {
	cells := []permute.Cell{
		{ID: "float32|none|sgd|single|simd|lr=0.6", Mode: permute.ModeSGD},
		{ID: "int8|none|sgd|single|simd|lr=0.6", Mode: permute.ModeSGD},
	}
	completed := []pulse.Result{
		{Status: "ok", Epoch: 1, Cell: cells[0]},
		{Status: "ok", Epoch: 1, Cell: cells[0]},
		{Status: "ok", Epoch: 1, Cell: cells[1]},
	}
	done := DoneSetFromCompleted(completed, 1)
	if got := PlanDoneCount(cells, done); got != 2 {
		t.Fatalf("plan done %d want 2", got)
	}
}
