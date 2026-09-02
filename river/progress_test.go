package river

import (
	"strings"
	"testing"
	"time"

	"github.com/openfluke/tide/permute"
)

func TestComputeProgressDoneLeftETA(t *testing.T) {
	plan := []string{
		"a|none|sgd|single|simd|lr=0.6",
		"b|none|sgd|single|simd|lr=0.6",
		"c|none|sgd|bicameral|simd|lr=0.6",
		"d|none|sgd|bicameral|simd|lr=0.6",
	}
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	byID := map[string]Row{
		plan[0]: {ID: plan[0], Status: "ok", FinishedAt: now.Add(-10 * time.Minute).Format(time.RFC3339)},
		plan[1]: {ID: plan[1], Status: "ok", FinishedAt: now.Add(-5 * time.Minute).Format(time.RFC3339)},
		plan[2]: {ID: plan[2], Status: "ok", FinishedAt: now.Add(-2 * time.Minute).Format(time.RFC3339)},
	}
	p := computeProgress(plan, byID, now)
	if p.Plan != 4 || p.Done != 3 || p.Left != 1 {
		t.Fatalf("progress %+v", p)
	}
	if p.Pct < 74.9 || p.Pct > 75.1 {
		t.Fatalf("pct %v", p.Pct)
	}
	if p.Complete {
		t.Fatal("expected incomplete")
	}
	if p.ByArch["single"].Done != 2 || p.ByArch["bicameral"].Left != 1 {
		t.Fatalf("by_arch %+v", p.ByArch)
	}
	if p.RatePerHr <= 0 || p.ETAHuman == "" || p.ETAHuman == "—" {
		t.Fatalf("eta/rate %+v", p)
	}
	if !strings.Contains(p.ETAHuman, "left") {
		t.Fatalf("ETA should say it's for leftovers: %q", p.ETAHuman)
	}
}

func TestComputeProgressComplete(t *testing.T) {
	cells := []permute.Cell{
		{ID: "x|lr=0.6"},
		{ID: "y|lr=0.6"},
	}
	s := &Store{byID: map[string]Row{
		"x|lr=0.6": {ID: "x|lr=0.6", Status: "ok"},
		"y|lr=0.6": {ID: "y|lr=0.6", Status: "ok"},
	}}
	s.SetPlan(cells)
	p := s.Progress()
	if !p.Complete || p.Left != 0 || p.ETAHuman != "done" {
		t.Fatalf("%+v", p)
	}
}
