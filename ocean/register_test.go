package ocean

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveAdvertiseInfersRemote(t *testing.T) {
	got, err := resolveAdvertise("", 8101, "192.168.1.21:5555")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://192.168.1.21:8101" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAdvertiseRewritesLoopback(t *testing.T) {
	got, err := resolveAdvertise("http://127.0.0.1:8101", 0, "10.0.0.8:9")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://10.0.0.8:8101" {
		t.Fatalf("got %q", got)
	}
}

func TestRegisterHTTP(t *testing.T) {
	s := &Server{Title: "test"}
	body := `{"name":"dense-sgd","port":8101,"layer":"dense","modes":["sgd"]}`
	req := httptest.NewRequest("POST", "/api/register", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.21:40000"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	peers := s.peerList()
	if len(peers) != 1 || peers[0].Name != "dense-sgd" || peers[0].URL != "http://192.168.1.21:8101" {
		t.Fatalf("%+v", peers)
	}
}
