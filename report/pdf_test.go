package report

import (
	"bytes"
	"testing"
	"time"
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
	// Helvetica WinAnsi: UTF-8 em dash/middot/times become â€” / Â· / Ã—.
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
