package dash

import (
	"net/http"
	"time"

	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/report"
)

// LiveView is the slim 1 Hz dashboard poll payload.
// Charts render server-side at /api/charts/*.svg — not embedded here.
type LiveView struct {
	UpdatedAt   time.Time `json:"updated_at"`
	ChartRev    int64     `json:"chart_rev"`
	HistoryLen  int       `json:"history_len"`
	HistoryTip  []pulse.HistoryPoint `json:"history_tip,omitempty"`
	AwaitingStart bool    `json:"awaiting_start"`
	Started     bool      `json:"started"`

	ID              string  `json:"id"`
	Task            string  `json:"task"`
	Subtitle        string  `json:"subtitle"`
	LR              float64 `json:"lr"`
	Addr            string  `json:"addr"`
	Epoch           int     `json:"epoch"`
	EpochMax        int     `json:"epoch_max"`
	EpochsLeft      int     `json:"epochs_left"`
	EpochOverallPct float64 `json:"epoch_overall_pct"`

	Running     bool   `json:"running"`
	RunningN    int    `json:"running_n"`
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	CellIndex   int    `json:"cell_index"`
	CellTotal   int    `json:"cell_total"`
	Plan        int    `json:"plan"`
	ProgressPct float64 `json:"progress_pct"`

	EpochDone  int `json:"epoch_done"`
	EpochOk    int `json:"epoch_ok"`
	EpochGap   int `json:"epoch_gap"`
	EpochFail  int `json:"epoch_fail"`
	OkAll      int `json:"ok_all"`
	Recorded   int `json:"recorded"`

	Current              *pulse.Result `json:"current,omitempty"`
	Inflight             []pulse.Result `json:"inflight,omitempty"`
	Leaderboard          []pulse.Result `json:"leaderboard"`
	LeaderboardMobile    []pulse.Result `json:"leaderboard_mobile"`
	LeaderboardLearn     []pulse.Result `json:"leaderboard_learn"`
	LeaderboardLearnMobile []pulse.Result `json:"leaderboard_learn_mobile"`

	Best             pulse.Best           `json:"best"`
	BestMobile       pulse.BestMobile     `json:"best_mobile"`
	BestLearn        pulse.BestLearn      `json:"best_learn"`
	BestLearnMobile  pulse.BestLearnMobile `json:"best_learn_mobile"`

	Winners      Winners        `json:"winners"`
	ModeProgress []ModeProgress `json:"mode_progress"`
	Axes         []LucyAxis     `json:"axes,omitempty"`
	LPD          report.LPD     `json:"lpd,omitempty"`
	Heat         report.Heat    `json:"heat,omitempty"`
	APIs         map[string]string `json:"apis"`
	Pace         SweepPace      `json:"pace"`
}

const liveBoardCap = 250

func (s *Server) LiveView() LiveView {
	b := s.Board()
	live := s.Tracker.SnapshotLive()
	// Host already Begin'd without pressing Start (e.g. test53) — release the gate.
	if !b.Started && b.Running {
		s.SignalStart()
		b.Started = true
		b.AwaitingStart = false
	}
	rev := live.UpdatedAt.UnixNano()
	if rev == 0 {
		rev = time.Now().UnixNano()
	}
	heat := b.Heat
	heat.Points = nil
	left := b.Plan - b.EpochDone
	runN := b.RunningN
	if runN == 0 && b.Running {
		runN = 1
	}
	if runN > 0 && left > 0 {
		left -= runN
	}
	if left < 0 {
		left = 0
	}
	wallAt, wallBase := s.paceAnchor()
	wallNew := len(live.Completed) - wallBase
	if wallNew < 0 {
		wallNew = 0
	}
	pace := computeSweepPaceWall(live.Completed, left, b.EpochsLeft, wallAt, wallBase, wallNew)
	return LiveView{
		UpdatedAt:            live.UpdatedAt,
		ChartRev:             rev,
		HistoryLen:           live.HistoryLen,
		HistoryTip:           live.History,
		AwaitingStart:        !b.Started,
		Started:              b.Started,
		ID:                   b.ID,
		Task:                 b.Task,
		Subtitle:             b.Subtitle,
		LR:                   b.LR,
		Addr:                 b.Addr,
		Epoch:                b.Epoch,
		EpochMax:             b.EpochMax,
		EpochsLeft:           b.EpochsLeft,
		EpochOverallPct:      b.EpochOverallPct,
		Running:              b.Running,
		RunningN:             b.RunningN,
		Phase:                b.Phase,
		Message:              b.Message,
		CellIndex:            b.CellIndex,
		CellTotal:            b.CellTotal,
		Plan:                 b.Plan,
		ProgressPct:          b.ProgressPct,
		EpochDone:            b.EpochDone,
		EpochOk:              b.Ok,
		EpochGap:             b.Gap,
		EpochFail:            b.Fail,
		OkAll:                b.OkAll,
		Recorded:             b.Recorded,
		Current:              slimPtr(b.Current),
		Inflight:             SlimResults(b.Inflight),
		Leaderboard:          SlimResults(trimLeaderboard(live.Leaderboard, liveBoardCap)),
		LeaderboardMobile:    SlimResults(trimLeaderboard(live.LeaderboardMobile, liveBoardCap)),
		LeaderboardLearn:     SlimResults(trimLeaderboard(live.LeaderboardLearn, liveBoardCap)),
		LeaderboardLearnMobile: SlimResults(trimLeaderboard(live.LeaderboardLearnMobile, liveBoardCap)),
		Best:                 slimBest(b.Best),
		BestMobile:           slimBestMobile(b.BestMobile),
		BestLearn:            slimBestLearn(b.BestLearn),
		BestLearnMobile:      slimBestLearnMobile(live.BestLearnMobile),
		Winners:              b.Winners,
		ModeProgress:         b.ModeProgress,
		Axes:                 b.Axes,
		LPD:                  b.LPD,
		Heat:                 heat,
		APIs:                 APIPaths(),
		Pace:                 pace,
	}
}

func (s *Server) cellByID(id string) (pulse.Result, bool) {
	if s == nil || s.Tracker == nil || id == "" {
		return pulse.Result{}, false
	}
	live := s.Tracker.SnapshotLive()
	if live.Current != nil && live.Current.Cell.ID == id {
		return *live.Current, true
	}
	for _, r := range live.Inflight {
		if r.Cell.ID == id {
			return r, true
		}
	}
	for _, r := range live.Completed {
		if r.Cell.ID == id {
			return r, true
		}
	}
	for _, r := range live.Leaderboard {
		if r.Cell.ID == id {
			return r, true
		}
	}
	return pulse.Result{}, false
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	v := s.LiveView()
	// No ETag — live poll must always refresh (304 was freezing progress UI).
	WriteJSON(w, r, "", v)
}

func (s *Server) handleCell(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query required", http.StatusBadRequest)
		return
	}
	cell, ok := s.cellByID(id)
	if !ok {
		http.Error(w, "cell not found", http.StatusNotFound)
		return
	}
	WriteJSON(w, r, "", cell)
}

func trimLeaderboard(in []pulse.Result, n int) []pulse.Result {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
