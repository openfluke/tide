package dash

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

//go:embed index.html
var static embed.FS

// Server serves the live HTML dashboard + JSON pulse.
type Server struct {
	Tracker *pulse.Tracker
	Cells   []permute.Cell // full sweep plan (for remaining-by-mode)
	Addr    string
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"live":          live,
			// Flat fields kept for older dashboard JS that reads top-level keys.
			"updated_at":    live.UpdatedAt,
			"running":       live.Running,
			"current":       live.Current,
			"completed":     live.Completed,
			"leaderboard":   live.Leaderboard,
			"leaderboard_mobile": live.LeaderboardMobile,
			"leaderboard_learn": live.LeaderboardLearn,
			"leaderboard_learn_mobile": live.LeaderboardLearnMobile,
			"best":          live.Best,
			"best_mobile":   live.BestMobile,
			"best_learn":    live.BestLearn,
			"best_learn_mobile": live.BestLearnMobile,
			"history":       live.History,
			"history_len":   live.HistoryLen,
			"phase":         live.Phase,
			"batch_index":   live.BatchIndex,
			"batch_total":   live.BatchTotal,
			"cell_index":    live.CellIndex,
			"cell_total":    live.CellTotal,
			"message":       live.Message,
			"mode_progress": s.modeProgress(live),
			"winners":       computeWinners(live),
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
	return mux
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	if s.Addr == "" {
		s.Addr = "0.0.0.0:8080"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}
