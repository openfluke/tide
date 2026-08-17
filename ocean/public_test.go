package ocean

import "testing"

func TestRewriteLoopbackURL(t *testing.T) {
	got := rewriteLoopbackURL("http://127.0.0.1:8101", "192.168.1.50")
	want := "http://192.168.1.50:8101"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if rewriteLoopbackURL("http://127.0.0.1:8101", "127.0.0.1") != "http://127.0.0.1:8101" {
		t.Fatal("local viewer should keep loopback")
	}
	if rewriteLoopbackURL("http://10.0.0.8:8101", "192.168.1.50") != "http://10.0.0.8:8101" {
		t.Fatal("non-loopback poll URL should stay")
	}
}

func TestViewerHost(t *testing.T) {
	if hostOnly("192.168.1.50:8090") != "192.168.1.50" {
		t.Fatal(hostOnly("192.168.1.50:8090"))
	}
	if hostOnly("ocean.local") != "ocean.local" {
		t.Fatal(hostOnly("ocean.local"))
	}
}

func TestPublicizeSnapshot(t *testing.T) {
	s := Snapshot{
		Peers: []PeerState{{Name: "dense", URL: "http://127.0.0.1:8101"}},
		Holistic: Holistic{
			Layers: []LayerWinner{{Tide: "dense", URL: "http://127.0.0.1:8101"}},
		},
	}
	out := publicizeSnapshot(s, "10.0.0.4")
	if out.Peers[0].URL != "http://10.0.0.4:8101" {
		t.Fatalf("peer %q", out.Peers[0].URL)
	}
	if out.Holistic.Layers[0].URL != "http://10.0.0.4:8101" {
		t.Fatalf("layer %q", out.Holistic.Layers[0].URL)
	}
}
