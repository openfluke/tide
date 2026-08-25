package dash

import (
	"testing"
	"time"

	"github.com/openfluke/tide/pulse"
)

func TestComputeSweepPace(t *testing.T) {
	start := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	completed := []pulse.Result{
		{Started: start, Ended: start.Add(10 * time.Second)},
		{Started: start, Ended: start.Add(20 * time.Second)},
		{Started: start, Ended: start.Add(30 * time.Second)},
	}
	p := computeSweepPace(completed, 100, 3)
	if p.PaceSamples != 3 {
		t.Fatalf("pace_samples = %d, want 3", p.PaceSamples)
	}
	if p.MedCellSec != 20 {
		t.Fatalf("med_cell_sec = %v, want 20", p.MedCellSec)
	}
	if p.AvgCellSec != 20 {
		t.Fatalf("avg_cell_sec = %v, want 20", p.AvgCellSec)
	}
	if p.CellsPerHour != 180 {
		t.Fatalf("cells_per_hour = %v, want 180", p.CellsPerHour)
	}
	if p.EtaEpochSec != 2000 {
		t.Fatalf("eta_epoch_sec = %v, want 2000", p.EtaEpochSec)
	}
	if p.EtaSweepSec != 6000 {
		t.Fatalf("eta_sweep_sec = %v, want 6000", p.EtaSweepSec)
	}
}

func TestComputeSweepPaceEmpty(t *testing.T) {
	p := computeSweepPace(nil, 50, 2)
	if p.PaceSamples != 0 || p.EtaEpochSec != 0 {
		t.Fatalf("expected zero pace, got %+v", p)
	}
}

func TestComputeSweepPaceIgnoresInstant(t *testing.T) {
	now := time.Now()
	completed := []pulse.Result{
		{Started: now, Ended: now},                         // seeded junk
		{Started: now, Ended: now.Add(2 * time.Millisecond)}, // re-Begin noise
	}
	p := computeSweepPace(completed, 100, 1)
	if p.PaceSamples != 0 {
		t.Fatalf("expected 0 samples, got %d", p.PaceSamples)
	}
}

func TestComputeSweepPaceWall(t *testing.T) {
	wallStart := time.Now().Add(-100 * time.Second)
	p := computeSweepPaceWall(nil, 50, 1, wallStart, 0, 10)
	// 100s / 10 cells = 10s/cell → ETA 500s
	if p.WallSecPer < 9 || p.WallSecPer > 11 {
		t.Fatalf("wall_sec_per = %v, want ~10", p.WallSecPer)
	}
	if p.EtaEpochSec < 450 || p.EtaEpochSec > 550 {
		t.Fatalf("eta_epoch_sec = %v, want ~500", p.EtaEpochSec)
	}
}
