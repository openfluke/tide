package dash

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
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
	// Lightweight 1s poll — tip only, not the whole timeline.
	mux.HandleFunc("/api/live", func(w http.ResponseWriter, r *http.Request) {
		live := s.Tracker.SnapshotLive()
		live.UpdatedAt = time.Now()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(live)
	})
	// Full or incremental history: /api/history or /api/history?from=N
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		from := 0
		if v := r.URL.Query().Get("from"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				from = n
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"from":    from,
			"total":   s.Tracker.SnapshotLive().HistoryLen,
			"history": s.Tracker.HistoryFrom(from),
		})
	})
	return mux
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	if s.Addr == "" {
		s.Addr = "0.0.0.0:8080"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}
