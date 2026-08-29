package dash

import (
	"encoding/json"
	"testing"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

func TestLiveViewJSONWithInflight(t *testing.T) {
	tr := pulse.New()
	a := permute.Cell{ID: "bin|none|sgd|tricameral|simd|lr=0.6", Mode: "sgd", Arch: "tricameral"}
	b := permute.Cell{ID: "f32|none|tween|single|simd|lr=0.6", Mode: "tween", Arch: "single"}
	tr.BeginEpoch(a, 1, "A")
	tr.BeginEpoch(b, 1, "A")
	tr.PulseID(a.ID, metrics.Window{}, metrics.Snapshot{AvgAccuracy: 90, SoftAcc: 20, Throughput: 100, Availability: 40, Score: 360}, "A")
	s := &Server{Tracker: tr, Cells: []permute.Cell{a, b}, Task: "t", ID: "id"}
	s.SignalStart()
	v := s.LiveView()
	if v.RunningN != 2 || v.Plan != 2 {
		t.Fatalf("running_n=%d plan=%d inflight=%d", v.RunningN, v.Plan, len(v.Inflight))
	}
	if _, err := json.Marshal(v); err != nil {
		t.Fatal(err)
	}
}
