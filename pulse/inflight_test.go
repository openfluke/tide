package pulse

import (
	"testing"
	"time"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

func TestMultiInflightParallelSafe(t *testing.T) {
	tr := New()
	a := permute.Cell{ID: "a|none|sgd|single|simd", Mode: "sgd"}
	b := permute.Cell{ID: "b|none|tween|single|simd", Mode: "tween"}
	tr.BeginEpoch(a, 1, "A")
	tr.BeginEpoch(b, 1, "A")
	live := tr.SnapshotLive()
	if live.RunningN != 2 {
		t.Fatalf("running_n=%d want 2", live.RunningN)
	}
	if len(live.Inflight) != 2 {
		t.Fatalf("inflight=%d want 2", len(live.Inflight))
	}
	tr.PulseID(a.ID, metrics.Window{}, metrics.Snapshot{AvgAccuracy: 10, SoftAcc: 8, Throughput: 100, Score: 1}, "A")
	tr.PulseID(b.ID, metrics.Window{}, metrics.Snapshot{AvgAccuracy: 20, SoftAcc: 15, Throughput: 200, Score: 2}, "A")
	live = tr.SnapshotLive()
	if live.RunningN != 2 {
		t.Fatalf("after pulse running_n=%d", live.RunningN)
	}
	var gotA, gotB float64
	for _, r := range live.Inflight {
		switch r.Cell.ID {
		case a.ID:
			gotA = r.Snapshot.AvgAccuracy
		case b.ID:
			gotB = r.Snapshot.AvgAccuracy
		}
	}
	if gotA != 10 || gotB != 20 {
		t.Fatalf("snaps a=%.0f b=%.0f (clobbered?)", gotA, gotB)
	}
	done := tr.FinishID(a.ID, "ok", "", metrics.Snapshot{AvgAccuracy: 11, SoftAcc: 9, Throughput: 110, Score: 1.1, Availability: 40})
	if done.Cell.ID != a.ID || done.Status != "ok" {
		t.Fatalf("finish a %+v", done)
	}
	if done.Started.IsZero() || done.Ended.Before(done.Started) {
		t.Fatalf("timestamps %+v %+v", done.Started, done.Ended)
	}
	live = tr.SnapshotLive()
	if live.RunningN != 1 || len(live.Inflight) != 1 || live.Inflight[0].Cell.ID != b.ID {
		t.Fatalf("after finish a: n=%d inflight=%+v", live.RunningN, live.Inflight)
	}
	tr.FinishID(b.ID, "ok", "", metrics.Snapshot{AvgAccuracy: 22, SoftAcc: 18, Throughput: 220, Score: 2.2, Availability: 50})
	live = tr.SnapshotLive()
	if live.RunningN != 0 || live.Running || live.Current != nil {
		t.Fatalf("idle after both: n=%d running=%v current=%v", live.RunningN, live.Running, live.Current)
	}
	time.Sleep(time.Millisecond) // ensure wall clock moves for Commit path
}
