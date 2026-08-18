package report

import (
	"bytes"
	"testing"
	"time"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/welvet/lucy"
)

func TestPDFTideEmpty(t *testing.T) {
	b, err := PDFTide(TideReport{Task: "dense", ID: "dense", Formula: "Score = T*A*S/10000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 200 || string(b[:4]) != "%PDF" {
		t.Fatalf("not a pdf (%d bytes)", len(b))
	}
}

func TestPDFOceanEmpty(t *testing.T) {
	b, err := PDFOcean(OceanReport{Title: "quick_sprint", Tides: []TideReport{{Task: "cnn2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 200 || string(b[:4]) != "%PDF" {
		t.Fatalf("not a pdf (%d bytes)", len(b))
	}
}

func TestPDFLatinNoMojibake(t *testing.T) {
	b, err := PDFTide(TideReport{
		Task:      "mha",
		ID:        "mha",
		Subtitle:  "token mixing on 8×8 patches · Lucy",
		Status:    "done",
		Generated: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Axes: []AxisView{
			{Name: "score", Mode: "sgd", DType: "int4", Arch: "single×1", Value: 1.2},
			{Name: "soft_acc", Tide: "", Mode: "adam", DType: "float32", Arch: "bi×2", Value: 40},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, seq := range map[string][]byte{
		"emdash": []byte{0xe2, 0x80, 0x94},
		"endash": []byte{0xe2, 0x80, 0x93},
		"middot": []byte{0xc2, 0xb7},
		"times":  []byte{0xc3, 0x97},
	} {
		if bytes.Contains(b, seq) {
			t.Fatalf("pdf still contains utf-8 %s", name)
		}
	}
	if !bytes.Contains(b, []byte("tide - mha")) {
		t.Fatal("expected ascii title in pdf info")
	}
}

func TestPDFPrettyCellNoClip(t *testing.T) {
	id := "float32|none|TweenSplitHeadProxy|cnn|simd"
	cell := permute.Cell{ID: id, Mode: "TweenSplitHeadProxy", Arch: "cnn"}
	b, err := PDFTide(TideReport{
		Task: "mha",
		Leaderboard: []pulse.Result{{
			Cell: cell,
			Snapshot: lucy.Snapshot{
				Score: 12, SoftAcc: 25, AvgAccuracy: 26, Throughput: 100, Availability: 40,
			},
		}},
		Cells: []CellPoint{{
			ID: PrettyCell(id), Mode: "TweenSplitHeadProxy", DType: "float32", Arch: "single",
			Score: 12, Soft: 25, Acc: 26, Avail: 40,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("|cnn|")) {
		t.Fatal("pdf still contains |cnn|")
	}
	if !bytes.Contains(b, []byte("TweenSplitHeadProxy")) {
		t.Fatal("cell id was clipped; missing TweenSplitHeadProxy")
	}
	if !bytes.Contains(b, []byte("single")) {
		t.Fatal("expected single in pdf")
	}
}

func TestLatin(t *testing.T) {
	got := latin("tide — mha · 8×8 → 50%")
	want := "tide - mha  -  8x8 -> 50%"
	if got != want {
		t.Fatalf("latin: got %q want %q", got, want)
	}
	if clip("single×1", 20) != "singlex1" {
		t.Fatalf("clip arch: %q", clip("single×1", 20))
	}
}

func TestTableColWidthsCellGetsRoom(t *testing.T) {
	w := tableColWidths([]string{"Mode", "n", "Acc", "KiB", "Thru", "Cell"}, 174)
	if len(w) != 6 {
		t.Fatalf("widths %v", w)
	}
	if w[5] <= w[1]*2 {
		t.Fatalf("cell col should dwarf n: n=%.1f cell=%.1f", w[1], w[5])
	}
}

func TestPDFGoldModeTableFits(t *testing.T) {
	id := "bfloat16|none|MeshTweenSplitSparse|single|simd"
	b, err := PDFTide(TideReport{
		Task: "GPT-char",
		LPD: LPD{
			N:       1,
			Formula: "Q = test",
			Champ:   LPDChamp{ID: id, Mode: "MeshTweenSplitSparse", Score: 4, Acc: 8, Thru: 300, RAMKiB: 40},
			GoldModes: []LPDMode{{
				Mode: "MeshTweenSplitSparse", N: 1, BestAcc: 8.6, MinRAM: 142.1, MaxThru: 395,
				Smallest: id, Fastest: id,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("smallest cell")) || bytes.Contains(b, []byte("fastest cell")) {
		t.Fatal("old overlapping gold-mode headers still present")
	}
	if !bytes.Contains(b, []byte("bfloat16 MeshTweenSplitSparse single")) {
		t.Fatal("expected compact cell in gold-mode table")
	}
}

func TestPDFConsciousnessRadar(t *testing.T) {
	pts := []CellPoint{
		{ID: "f32", Mode: "sgd", DType: "float32", Arch: "single", Score: 100, Soft: 80, Acc: 90, Thru: 200, Avail: 40, RAMKiB: 1000},
		{ID: "int8", Mode: "sgd", DType: "int8", Arch: "single", Score: 85, Soft: 72, Acc: 82, Thru: 180, Avail: 38, RAMKiB: 180},
		{ID: "bin", Mode: "sgd", DType: "binary", Arch: "single", Score: 40, Soft: 20, Acc: 12, Thru: 400, Avail: 50, RAMKiB: 40},
	}
	l := BuildLPD(pts)
	b, err := PDFTide(TideReport{Task: "GPT-char", LPD: l, Cells: pts})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("Consciousness radar")) {
		t.Fatal("missing consciousness radar")
	}
	if !bytes.Contains(b, []byte("Memory density radar")) {
		t.Fatal("missing density radar")
	}
}
