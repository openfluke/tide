package dash

import (
	"github.com/openfluke/tide/pulse"
)

// APIPaths is the HTTP catalog every tide dashboard exposes.
// Ocean (and any other client) can discover these from /api/meta.
func APIPaths() map[string]string {
	return map[string]string{
		"live":    "/api/live",
		"board":   "/api/board",
		"meta":    "/api/meta",
		"start":   "/api/start",
		"history": "/api/history",
		"winners": "/api/winners",
	}
}

// Meta is the lightweight identity payload (no leaderboards).
type Meta struct {
	ID             string            `json:"id"`
	Task           string            `json:"task"`
	Subtitle       string            `json:"subtitle"`
	Addr           string            `json:"addr"`
	Epoch          int               `json:"epoch"`
	Started        bool              `json:"started"`
	AwaitingStart  bool              `json:"awaiting_start"`
	CellTotal      int               `json:"cell_total"`
	Ocean          bool              `json:"ocean"`
	APIs           map[string]string `json:"apis"`
}

// Board is the compact live snapshot ocean polls (~1s).
// Same Lucy boards as /api/live, without the full completed[] dump.
type Board struct {
	ID              string            `json:"id"`
	Task            string            `json:"task"`
	Subtitle        string            `json:"subtitle"`
	Addr            string            `json:"addr"`
	Epoch           int               `json:"epoch"`
	Started         bool              `json:"started"`
	AwaitingStart   bool              `json:"awaiting_start"`
	Running         bool              `json:"running"`
	Phase           string            `json:"phase"`
	Message         string            `json:"message"`
	CellIndex       int               `json:"cell_index"`
	CellTotal       int               `json:"cell_total"`
	Ok              int               `json:"ok"`
	Gap             int               `json:"gap"`
	Fail            int               `json:"fail"`
	Current         *pulse.Result     `json:"current,omitempty"`
	Best            pulse.Best        `json:"best"`
	BestMobile      pulse.BestMobile  `json:"best_mobile"`
	BestLearn       pulse.BestLearn   `json:"best_learn"`
	Winners         Winners           `json:"winners"`
	ModeProgress    []ModeProgress    `json:"mode_progress"`
	Leaderboard     []pulse.Result    `json:"leaderboard"`
	LeaderboardLearn []pulse.Result   `json:"leaderboard_learn"`
	APIs            map[string]string `json:"apis"`
}

func (s *Server) identityID() string {
	if s == nil {
		return ""
	}
	if s.ID != "" {
		return s.ID
	}
	if s.Task != "" {
		return s.Task
	}
	return s.Addr
}

func (s *Server) Meta() Meta {
	total := 0
	if s != nil && s.Tracker != nil {
		total = s.Tracker.SnapshotLive().CellTotal
	}
	if total == 0 && s != nil {
		total = len(s.Cells)
	}
	started := s != nil && s.Started()
	return Meta{
		ID:            s.identityID(),
		Task:          s.Task,
		Subtitle:      s.Subtitle,
		Addr:          s.Addr,
		Epoch:         s.Epoch,
		Started:       started,
		AwaitingStart: !started,
		CellTotal:     total,
		Ocean:         false,
		APIs:          APIPaths(),
	}
}

func (s *Server) Board() Board {
	var live pulse.Live
	if s != nil && s.Tracker != nil {
		live = s.Tracker.SnapshotLive()
	}
	ok, gap, fail := 0, 0, 0
	for _, r := range live.Completed {
		switch r.Status {
		case "ok":
			ok++
		case "gap":
			gap++
		case "fail":
			fail++
		}
	}
	lb := live.Leaderboard
	if len(lb) > 12 {
		lb = lb[:12]
	}
	learn := live.LeaderboardLearn
	if len(learn) > 8 {
		learn = learn[:8]
	}
	started := s != nil && s.Started()
	return Board{
		ID:               s.identityID(),
		Task:             s.Task,
		Subtitle:         s.Subtitle,
		Addr:             s.Addr,
		Epoch:            s.Epoch,
		Started:          started,
		AwaitingStart:    !started,
		Running:          live.Running,
		Phase:            live.Phase,
		Message:          live.Message,
		CellIndex:        live.CellIndex,
		CellTotal:        live.CellTotal,
		Ok:               ok,
		Gap:              gap,
		Fail:             fail,
		Current:          live.Current,
		Best:             live.Best,
		BestMobile:       live.BestMobile,
		BestLearn:        live.BestLearn,
		Winners:          computeWinners(live),
		ModeProgress:     s.modeProgress(live),
		Leaderboard:      append([]pulse.Result(nil), lb...),
		LeaderboardLearn: append([]pulse.Result(nil), learn...),
		APIs:             APIPaths(),
	}
}

func (s *Server) livePayload() map[string]any {
	live := s.Tracker.SnapshotLive()
	meta := s.Meta()
	return map[string]any{
		"live": live,
		// Flat fields kept for older dashboard JS that reads top-level keys.
		"updated_at":               live.UpdatedAt,
		"running":                  live.Running,
		"current":                  live.Current,
		"completed":                live.Completed,
		"leaderboard":              live.Leaderboard,
		"leaderboard_mobile":       live.LeaderboardMobile,
		"leaderboard_learn":        live.LeaderboardLearn,
		"leaderboard_learn_mobile": live.LeaderboardLearnMobile,
		"best":                     live.Best,
		"best_mobile":              live.BestMobile,
		"best_learn":               live.BestLearn,
		"best_learn_mobile":        live.BestLearnMobile,
		"history":                  live.History,
		"history_len":              live.HistoryLen,
		"phase":                    live.Phase,
		"batch_index":              live.BatchIndex,
		"batch_total":              live.BatchTotal,
		"cell_index":               live.CellIndex,
		"cell_total":               live.CellTotal,
		"message":                  live.Message,
		"mode_progress":            s.modeProgress(live),
		"winners":                  computeWinners(live),
		"awaiting_start":           meta.AwaitingStart,
		"started":                  meta.Started,
		"epoch":                    s.Epoch,
		"task":                     s.Task,
		"subtitle":                 s.Subtitle,
		"id":                       meta.ID,
		"addr":                     s.Addr,
		"apis":                     meta.APIs,
	}
}
