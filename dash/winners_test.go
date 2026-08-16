package dash

import (
	"testing"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
)

func TestComputeWinnersBestSettingsPerMode(t *testing.T) {
	ok := func(mode permute.TrainMode, arch permute.ArchKind, dt core.DType, fmt quant.Format, score, acc float64) pulse.Result {
		c := permute.Cell{
			DType:  dt,
			Format: fmt,
			Mode:   mode,
			Arch:   arch,
			Cams:   permute.CamsOf(arch),
		}
		c.ID = c.String()
		return pulse.Result{
			Cell:   c,
			Status: "ok",
			Snapshot: metrics.Snapshot{
				Score:       score,
				SoftAcc:     acc,
				AvgAccuracy: acc,
			},
		}
	}
	live := pulse.Live{Completed: []pulse.Result{
		ok(permute.ModeSGD, permute.ArchCNN, core.DTypeFloat32, quant.FormatNone, 30, 69),
		ok(permute.ModeSGD, permute.ArchCNN, core.DTypeInt8, quant.FormatNone, 10, 40),
		ok(permute.ModeSGD, permute.ArchBicameral, core.DTypeFloat32, quant.FormatNone, 22, 60),
		ok(permute.ModeTween, permute.ArchCNN, core.DTypeUint2, quant.FormatNone, 8, 12),
		{Status: "fail", Cell: permute.Cell{Mode: permute.ModeTween, ID: "x"}},
	}}
	w := computeWinners(live)
	if len(w.BestSettingsPerMode) != 2 {
		t.Fatalf("settings per mode: got %d want 2", len(w.BestSettingsPerMode))
	}
	sgd := w.BestSettingsPerMode[0]
	if sgd.Mode != "sgd" || sgd.DType != "float32" || sgd.Arch != "single×1" {
		t.Fatalf("sgd recipe: %+v", sgd)
	}
	if sgd.Score != 30 {
		t.Fatalf("sgd should pick highest score, got %v", sgd.Score)
	}
	if len(w.BestCellPerMode) != 3 { // sgd×cnn, sgd×bi, tween×cnn
		t.Fatalf("cell per mode×arch: got %d want 3", len(w.BestCellPerMode))
	}
}
