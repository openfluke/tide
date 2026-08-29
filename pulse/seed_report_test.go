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
		t.Fatalf("b not upserted: %+v", by["b"].Snapshot)
	}
	if _, ok := by["c"]; !ok {
		t.Fatal("missing c")
	}
}
