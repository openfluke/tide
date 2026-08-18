package report

import "testing"

func TestBuildLPDGoldilocks(t *testing.T) {
	pts := []CellPoint{
		{ID: "f32", Mode: "sgd", DType: "float32", Arch: "single", Score: 100, Soft: 80, Acc: 90, Thru: 200, Avail: 40, RAMKiB: 1000},
		{ID: "int8", Mode: "sgd", DType: "int8", Arch: "single", Score: 85, Soft: 72, Acc: 82, Thru: 180, Avail: 38, RAMKiB: 180},
		{ID: "bin", Mode: "sgd", DType: "binary", Arch: "single", Score: 40, Soft: 20, Acc: 12, Thru: 400, Avail: 50, RAMKiB: 40},
		{ID: "fat", Mode: "tween", DType: "float32", Arch: "tricameral", Score: 95, Soft: 78, Acc: 88, Thru: 190, Avail: 35, RAMKiB: 3000},
	}
	l := BuildLPD(pts)
	if l.Champ.ID != "f32" {
		t.Fatalf("champ %s", l.Champ.ID)
	}
	var gold, trap, fat LPDRow
	for _, r := range l.Top {
		switch r.ID {
		case "int8":
			gold = r
		case "bin":
			trap = r
		case "fat":
			fat = r
		}
	}
	if !gold.Gold || gold.Band != "gold" {
		t.Fatalf("int8 should be gold: %+v", gold)
	}
	if gold.Q < 0.80 {
		t.Fatalf("int8 Q %v", gold.Q)
	}
	if trap.LPD != 0 || trap.Band != "trap" {
		t.Fatalf("binary should be trap with LPD 0: %+v", trap)
	}
	if fat.Band != "keep" {
		t.Fatalf("fat quality should be keep, got %s Q=%.2f ram=%.2f", fat.Band, fat.Q, fat.RAMFrac)
	}
	if l.Top[0].ID != "int8" {
		t.Fatalf("LPD rank want int8 first, got %s", l.Top[0].ID)
	}
	if l.FastID != "bin" || l.AvailID != "bin" {
		t.Fatalf("board fastest/avail want bin, got %s / %s", l.FastID, l.AvailID)
	}
	if trap.MSpeed != 0 || trap.MAvail != 0 || trap.Mix != 0 {
		t.Fatalf("trap must zero mobile speed/avail/mix: %+v", trap)
	}
	if gold.MSpeed <= 0 || gold.MAvail <= 0 || gold.Mix <= 0 {
		t.Fatalf("gold should keep mobile speed/avail/mix: %+v", gold)
	}
	if gold.RelFast > 0.5 {
		t.Fatalf("int8 RelFast vs binary 400 thru want ~0.45, got %v", gold.RelFast)
	}
	if l.TopSpeed[0].ID == "bin" && l.TopSpeed[0].MSpeed > 0 {
		t.Fatalf("binary must not win mspeed")
	}
	if l.AccChamp.ID != "f32" {
		t.Fatalf("acc champ %s", l.AccChamp.ID)
	}
	if l.GoldStd.ID != "int8" {
		t.Fatalf("gold-std smallest Acc-keep want int8, got %s", l.GoldStd.ID)
	}
}

func TestBuildLPDUsesAccChamp(t *testing.T) {
	pts := []CellPoint{
		{ID: "fast", Mode: "tween", DType: "float32", Arch: "single", Score: 100, Soft: 50, Acc: 20, Thru: 200, Avail: 40, RAMKiB: 1000},
		{ID: "learn", Mode: "sgd", DType: "bfloat16", Arch: "single", Score: 30, Soft: 70, Acc: 80, Thru: 80, Avail: 24, RAMKiB: 900},
		{ID: "tiny", Mode: "tween", DType: "binary", Arch: "single", Score: 90, Soft: 50, Acc: 18, Thru: 180, Avail: 38, RAMKiB: 150},
		{ID: "keep", Mode: "sgd", DType: "int8", Arch: "single", Score: 85, Soft: 65, Acc: 72, Thru: 170, Avail: 36, RAMKiB: 180},
	}
	l := BuildLPD(pts)
	if l.Champ.ID != "fast" {
		t.Fatalf("score champ %s", l.Champ.ID)
	}
	if l.AccChamp.ID != "learn" {
		t.Fatalf("acc champ %s", l.AccChamp.ID)
	}
	var tiny, keep LPDRow
	for _, r := range l.Top {
		switch r.ID {
		case "tiny":
			tiny = r
		case "keep":
			keep = r
		}
	}
	if tiny.Gold || tiny.LPD != 0 {
		t.Fatalf("fast tiny with chance Acc vs Acc champ must not be gold: %+v", tiny)
	}
	if !keep.Gold {
		t.Fatalf("int8 should be gold vs Acc champ: Q=%.2f relAcc=%.2f band=%s", keep.Q, keep.RelAcc, keep.Band)
	}
	if l.GoldStd.ID != "keep" {
		t.Fatalf("gold-std want keep, got %s", l.GoldStd.ID)
	}
	if len(l.GoldModes) == 0 || l.GoldModes[0].Mode != "sgd" {
		t.Fatalf("gold-std mode want sgd, got %+v", l.GoldModes)
	}
	if l.PeakThru != 170 {
		t.Fatalf("learner Thru peak must ignore chance-Acc fast cell, got %v", l.PeakThru)
	}
	if keep.RAMFrac <= 0 || keep.RAMFrac > 0.21 {
		t.Fatalf("shrink vs Acc-champ RAM want ~0.20, got %v", keep.RAMFrac)
	}
}

func TestBuildLPDTrapDoesNotSetThruPeak(t *testing.T) {
	pts := []CellPoint{
		{ID: "learn", Mode: "sgd", DType: "float32", Acc: 90, Thru: 100, Avail: 40, Score: 10, RAMKiB: 800},
		{ID: "bin", Mode: "tween", DType: "binary", Acc: 12, Thru: 900, Avail: 80, Score: 50, RAMKiB: 30},
	}
	l := BuildLPD(pts)
	if l.PeakThru != 100 {
		t.Fatalf("PeakThru want learner 100, got %v (board fast %v)", l.PeakThru, l.FastThru)
	}
	if l.FastThru != 900 {
		t.Fatalf("board FastThru still records trap speed, got %v", l.FastThru)
	}
	var bin LPDRow
	for _, r := range l.Top {
		if r.ID == "bin" {
			bin = r
		}
	}
	if bin.LPD != 0 || bin.Band != "trap" {
		t.Fatalf("binary trap: %+v", bin)
	}
	if bin.RelAcc >= LPDKeepFloor {
		t.Fatalf("trap RelAcc should miss Acc keep, got %v", bin.RelAcc)
	}
}
