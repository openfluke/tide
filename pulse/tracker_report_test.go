package pulse

import (
	"testing"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

func TestReportLogKeepsFullRunWhileCompletedTrims(t *testing.T) {
	tr := New()
	tr.completedCap = 3
	cell := func(id string) permute.Cell {
		return permute.Cell{ID: id, Mode: permute.ModeSGD}
	}
	snap := metrics.Snapshot{Score: 1}
	now := tr.live.UpdatedAt
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		tr.Commit(cell(id), 1, "ok", "", snap, now, now)
	}
	if got := len(tr.SnapshotLive().Completed); got != 3 {
		t.Fatalf("live completed cap: got %d want 3", got)
	}
	if got := len(tr.ReportResults()); got != 5 {
		t.Fatalf("report log: got %d want 5", got)
	}
	// upsert same id
	tr.Commit(cell("a"), 1, "ok", "", metrics.Snapshot{Score: 99}, now, now)
	if got := len(tr.ReportResults()); got != 5 {
		t.Fatalf("report log after upsert: got %d want 5", got)
	}
	if tr.ReportResults()[0].Snapshot.Score != 99 {
		t.Fatalf("upsert did not replace cell a")
	}
}
