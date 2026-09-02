package dash

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/openfluke/tide/checkpoint"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/river"
)

//go:embed web/*
var webFS embed.FS

// Server serves the live HTML dashboard + JSON pulse.
// Tide is dataset-agnostic: the host (e.g. live_mnist) supplies Cells + optional Task.
type Server struct {
	Tracker *pulse.Tracker
	Cells   []permute.Cell // full sweep plan (for remaining-by-mode)
	Addr    string
	Epoch    int // 1-based; shown on Resume button
	EpochMax int // total epochs planned for this LR (0 = unknown / single)

	// Task is the host workload name shown in the header (e.g. "MNIST").
	// Empty keeps the generic "live adaptation" title.
	Task     string
	Subtitle string
	// LR is the host learning rate for this sweep (shown on dash, ocean, PDF).
	LR float64
	// ID is the ocean peer name. Empty → Task, then Addr.
	ID string
	// Workers is parallel cell trainers (for sweep-active UI when between checkpoints).
	Workers int

	// River is an optional results store for compare / near / LPD / throughput pages.
	River     *river.Store
	RiverOpts river.Options

	startMu sync.Mutex
	started bool
	startCh chan struct{} // closed when Start is pressed

	// Pace anchor: wall-clock ETA from cells finished after Start.
	paceMu       sync.Mutex
	paceAt       time.Time
	paceDoneBase int
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
		m := permute.QueueMode(c)
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
	epoch := 1
	if s != nil && s.Epoch > 0 {
		epoch = s.Epoch
	}
	doneSet := checkpoint.DoneSetFromCompleted(live.Completed, epoch)
	if s != nil && s.Tracker != nil {
		doneSet = s.Tracker.DoneSet()
	}
	for _, c := range s.Cells {
		m := permute.QueueMode(c)
		a := by[m]
		if a == nil {
			continue
		}
		if permute.IDDone(doneSet, c.ID) {
			a.done++
		}
	}
	if len(live.Inflight) > 0 {
		for _, r := range live.Inflight {
			m := permute.QueueMode(r.Cell)
			if a := by[m]; a != nil {
				a.running++
			}
		}
	} else if live.Current != nil && live.Running {
		m := permute.QueueMode(live.Current.Cell)
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
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	servePage := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := webFS.ReadFile("web/" + name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
		}
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(webRoot))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		servePage("live.html")(w, r)
	})
	mux.HandleFunc("/lucy", servePage("lucy.html"))
	mux.HandleFunc("/honesty", servePage("honesty.html"))
	mux.HandleFunc("/winners", servePage("winners.html"))
	mux.HandleFunc("/boards", servePage("boards.html"))
	if s.River != nil {
		opts := s.RiverOpts
		opts.Integrated = true
		opts.TideListen = s.Addr
		if opts.Title == "" && s.Task != "" {
			opts.Title = s.Task + " compare"
		}
		river.Mount(mux, s.River, opts)
	}
	mux.HandleFunc("/api/live", s.handleLive)
	mux.HandleFunc("/api/cell", s.handleCell)
	mux.HandleFunc("/api/charts/", s.handleChart)
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.Meta())
	})
	mux.HandleFunc("/api/board", func(w http.ResponseWriter, r *http.Request) {
		b := s.Board()
		b.Leaderboard = SlimResults(b.Leaderboard)
		b.LeaderboardLearn = SlimResults(b.LeaderboardLearn)
		b.Heat.Points = nil
		etag := `"board-` + strconv.FormatInt(s.Tracker.SnapshotLive().UpdatedAt.UnixNano(), 10) + `"`
		WriteJSON(w, r, etag, b)
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
	mux.HandleFunc("/api/report.pdf", s.handleReportPDF)
	mux.HandleFunc("/api/report", s.handleReportJSON)
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
	return WithGzip(withCORS(mux))
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	if s.Addr == "" {
		s.Addr = "0.0.0.0:8080"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}
