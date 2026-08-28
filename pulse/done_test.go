package pulse

import (
	"testing"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

func TestDoneSetSurvivesCompletedTrim(t *testing.T) {
	tr := New()
	tr.completedCap = 3
	now := tr.live.UpdatedAt
	snap := metrics.Snapshot{Score: 1}
	for i := 0; i < 5; i++ {
		id := "cell" + string(rune('a'+i))
		tr.Commit(permute.Cell{ID: id, Mode: permute.ModeSGD}, 1, "ok", "", snap, now, now)
	}
	if got := len(tr.SnapshotLive().Completed); got != 3 {
		t.Fatalf("live completed %d want 3", got)
	}
	done := tr.DoneSet()
	for i := 0; i < 5; i++ {
		id := "cell" + string(rune('a'+i))
		if !permute.IDDone(done, id) {
			t.Fatalf("missing done id %s after trim", id)
		}
	}
}

func TestSeedDoneIDsWithoutCommit(t *testing.T) {
	tr := New()
	tr.SeedDoneIDs([]string{"a|lr=0.6", "b|lr=0.6"})
	done := tr.DoneSet()
	if !permute.IDDone(done, "a|lr=0.6") || !permute.IDDone(done, "b|lr=0.6") {
		t.Fatalf("seed missing: %+v", done)
	}
}

func TestParkClearsRunning(t *testing.T) {
	tr := New()
	tr.BeginEpoch(permute.Cell{ID: "x", Mode: permute.ModeSGD}, 1, "A")
	if !tr.SnapshotLive().Running {
		t.Fatal("expected running")
	}
	tr.Park("epoch done")
	live := tr.SnapshotLive()
	if live.Running || live.Current != nil || live.Message != "epoch done" {
		t.Fatalf("park live %+v", live)
	}
}
