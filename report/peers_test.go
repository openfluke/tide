package report

import "testing"

func TestTidePortForCam(t *testing.T) {
	cases := []struct {
		cam  int
		band string
		want int
	}{
		{1, "lo", 8080},
		{3, "lo", 8100},
		{3, "hi", 8102},
	}
	for _, c := range cases {
		got := TidePortForCam(c.cam, c.band)
		if got != c.want {
			t.Errorf("cam=%d band=%s: got %d want %d", c.cam, c.band, got, c.want)
		}
	}
}

func TestParsePeerCamBand(t *testing.T) {
	if ParsePeerCam("cam3") != 3 {
		t.Fatal("cam3")
	}
	if ParsePeerBand("cam3-lo") != "lo" {
		t.Fatal("band lo")
	}
	if ParsePeerBand("cam1-hi") != "hi" {
		t.Fatal("band hi")
	}
}
