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
}
