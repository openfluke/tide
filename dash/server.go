package dash

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

//go:embed index.html
var static embed.FS

// Server serves the live HTML dashboard + JSON pulse.
// Tide is dataset-agnostic: the host (e.g. live_mnist) supplies Cells + optional Task.
type Server struct {
	Tracker *pulse.Tracker
	Cells   []permute.Cell // full sweep plan (for remaining-by-mode)
	Addr    string
	Epoch   int // 1-based; shown on Resume button

	// Task is the host workload name shown in the header (e.g. "MNIST").
	// Empty keeps the generic "live adaptation" title.
	Task     string
	Subtitle string
	// ID is the ocean peer name. Empty → Task, then Addr.
	ID string

	startMu sync.Mutex
	started bool
	startCh chan struct{} // closed when Start is pressed
}

// ModeProgress is done/left counts for one train mode in the plan.
type ModeProgress struct {
	Mode    string `json:"mode"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Running int    `json:"running"`
	Left    int    `json:"left"`
}

func (s *Server) modeProgress(live pulse.Live) []ModeProgress {
	if len(s.Cells) == 0 {
		return nil
	}
	type agg struct {
		total, done, running int
	}
	order := make([]string, 0)
	seen := map[string]bool{}
	by := map[string]*agg{}
	for _, c := range s.Cells {
		m := string(c.Mode)
		if !seen[m] {
			seen[m] = true
			order = append(order, m)
		}
		a := by[m]
		if a == nil {
			a = &agg{}
			by[m] = a
		}
		a.total++
	}
	for _, r := range live.Completed {
		if r.Status == "ok" || r.Status == "gap" || r.Status == "fail" {
			m := string(r.Cell.Mode)
			if a := by[m]; a != nil {
				a.done++
			}
		}
	}
	if live.Current != nil && live.Running {
		m := string(live.Current.Cell.Mode)
		if a := by[m]; a != nil {
			a.running++
		}
	}
	out := make([]ModeProgress, 0, len(order))
	for _, m := range order {
		a := by[m]
		left := a.total - a.done - a.running
		if left < 0 {
			left = 0
		}
		out = append(out, ModeProgress{
			Mode:    m,
			Total:   a.total,
			Done:    a.done,
			Running: a.running,
			Left:    left,
		})
	}
	return out
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
		live := s.Tracker.SnapshotLive()
		live.UpdatedAt = time.Now()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		payload := s.livePayload()
		payload["updated_at"] = live.UpdatedAt
		payload["live"] = live
		_ = json.NewEncoder(w).Encode(payload)
	})
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.Meta())
	})
	mux.HandleFunc("/api/board", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.Board())
	})
	mux.HandleFunc("/api/winners", func(w http.ResponseWriter, r *http.Request) {
		live := s.Tracker.SnapshotLive()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(computeWinners(live))
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "POST /api/start", http.StatusMethodNotAllowed)
			return
		}
		already := s.Started()
		s.SignalStart()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"started": true,
			"already": already,
			"message": "training start signaled",
		})
	})
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
	return withCORS(mux)
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	if s.Addr == "" {
		s.Addr = "0.0.0.0:8080"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}
