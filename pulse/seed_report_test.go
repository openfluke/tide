package pulse

import (
	"testing"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

func TestSeedReportLogMergesAndGrows(t *testing.T) {
	tr := New()
	tr.completedCap = 2
	cell := func(id string) permute.Cell {
		return permute.Cell{ID: id, Mode: permute.ModeSGD}
	}
	now := tr.live.UpdatedAt
	tr.Commit(cell("a"), 1, "ok", "", metrics.Snapshot{AvgAccuracy: 10, Availability: 30}, now, now)
	tr.Commit(cell("b"), 1, "ok", "", metrics.Snapshot{AvgAccuracy: 11, Availability: 31}, now, now)
	tr.SeedReportLog([]Result{
		{Cell: cell("b"), Status: "ok", Snapshot: metrics.Snapshot{AvgAccuracy: 99, Availability: 40}},
		{Cell: cell("c"), Status: "ok", Snapshot: metrics.Snapshot{AvgAccuracy: 12, Availability: 32}},
	})
	got := tr.ReportResults()
	if len(got) != 3 {
		t.Fatalf("report log len %d want 3", len(got))
	}
	by := map[string]Result{}
	for _, r := range got {
		by[r.Cell.ID] = r
	}
	if by["b"].Snapshot.AvgAccuracy != 99 {
		t.Fatalf("b Acc not upserted: %+v", by["b"].Snapshot)
	}
	// Incoming c has no Avail — should still be present.
	if _, ok := by["c"]; !ok {
		t.Fatal("missing c")
	}
	// Seed a with Avail=0 should keep prior Avail.
	tr.SeedReportLog([]Result{
		{Cell: cell("a"), Status: "ok", Snapshot: metrics.Snapshot{AvgAccuracy: 50, Availability: 0}},
	})
	a := tr.ReportResults()
	for _, r := range a {
		if r.Cell.ID == "a" {
			if r.Snapshot.Availability != 30 {
				t.Fatalf("a Avail overwritten: got %v want 30", r.Snapshot.Availability)
			}
			if r.Snapshot.AvgAccuracy != 50 {
				t.Fatalf("a Acc not updated: %v", r.Snapshot.AvgAccuracy)
			}
		}
	}
}
