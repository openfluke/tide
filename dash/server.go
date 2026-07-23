package dash

import (
	"embed"
	"encoding/json"
	"net/http"
	"time"

	"github.com/openfluke/tide/pulse"
)

//go:embed index.html
var static embed.FS

// Server serves the live HTML dashboard + JSON pulse.
type Server struct {
	Tracker *pulse.Tracker
	Addr    string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := static.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/live", func(w http.ResponseWriter, r *http.Request) {
		live := s.Tracker.Snapshot()
		live.UpdatedAt = time.Now()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(live)
	})
	return mux
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	if s.Addr == "" {
		s.Addr = ":8080"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}
