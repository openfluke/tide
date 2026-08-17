package report

import "testing"

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
