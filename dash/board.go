package dash

import (
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/report"
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
		"report":  "/api/report.pdf",
	}
}

// Meta is the lightweight identity payload (no leaderboards).
type Meta struct {
	ID            string            `json:"id"`
	Task          string            `json:"task"`
	Subtitle      string            `json:"subtitle"`
	Addr          string            `json:"addr"`
	Epoch         int               `json:"epoch"`
	Started       bool              `json:"started"`
	AwaitingStart bool              `json:"awaiting_start"`
	CellTotal     int               `json:"cell_total"`
	Ocean         bool              `json:"ocean"`
	APIs          map[string]string `json:"apis"`
}

// Board is the compact live snapshot ocean polls (~1s).
// Same Lucy boards as /api/live, without the full completed[] dump.
type Board struct {
	ID               string            `json:"id"`
	Task             string            `json:"task"`
	Subtitle         string            `json:"subtitle"`
	Addr             string            `json:"addr"`
	Epoch            int               `json:"epoch"`
	Started          bool              `json:"started"`
	AwaitingStart    bool              `json:"awaiting_start"`
	Running          bool              `json:"running"`
	Phase            string            `json:"phase"`
	Message          string            `json:"message"`
	CellIndex        int               `json:"cell_index"`
	CellTotal        int               `json:"cell_total"`
	Ok               int               `json:"ok"`
	Gap              int               `json:"gap"`
	Fail             int               `json:"fail"`
	OkAll            int               `json:"ok_all"`
	Recorded         int               `json:"recorded"`
	Plan             int               `json:"plan"`
	EpochDone        int               `json:"epoch_done"`
	ProgressPct      float64           `json:"progress_pct"`
	Current          *pulse.Result     `json:"current,omitempty"`
	Best             pulse.Best        `json:"best"`
	BestMobile       pulse.BestMobile  `json:"best_mobile"`
	BestLearn        pulse.BestLearn   `json:"best_learn"`
	Winners          Winners           `json:"winners"`
	ModeProgress     []ModeProgress    `json:"mode_progress"`
	Leaderboard      []pulse.Result    `json:"leaderboard"`
	LeaderboardLearn []pulse.Result    `json:"leaderboard_learn"`
	BestAdapt        *pulse.Result     `json:"best_adapt,omitempty"`
	BestSoft         *pulse.Result     `json:"best_soft,omitempty"`
	BestHard         *pulse.Result     `json:"best_hard,omitempty"`
	BestConsistency  *pulse.Result     `json:"best_consistency,omitempty"`
	BestStability    *pulse.Result     `json:"best_stability,omitempty"`
	BestAccThru      *pulse.Result     `json:"best_acc_thru,omitempty"`
	BestRealtime     *pulse.Result     `json:"best_realtime,omitempty"`
	BestKeep         *pulse.Result     `json:"best_keep,omitempty"`
	Axes             []LucyAxis        `json:"axes,omitempty"`
	Heat             report.Heat       `json:"heat,omitempty"`
	LPD              report.LPD        `json:"lpd,omitempty"`
	APIs             map[string]string `json:"apis"`
	// Status is paused | queued | running | done | idle — ocean uses this so
	// a finished epoch (dashboard kept up) is not shown as "still running".
	Status string `json:"status"`
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
	epoch := 1
	if s != nil && s.Epoch > 0 {
		epoch = s.Epoch
	}
	okE, gapE, failE, okAll, rec := countResults(live.Completed, epoch)
	plan := 0
	if s != nil {
		plan = len(s.Cells)
	}
	if plan == 0 {
		plan = live.CellTotal
	}
	epochDone := okE + gapE + failE
	pct := 0.0
	if plan > 0 {
		pct = 100 * float64(epochDone) / float64(plan)
		if pct > 100 {
			pct = 100
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
	adapt, soft, hard, cons, stab, accThru, realtime, keep := extraBests(live.Completed)
	pts := report.PointsFromResults(live.Completed, s.Task)
	lpd := report.BuildLPD(pts)
	heat := report.BuildHeat(pts)
	out := Board{
		ID:               s.identityID(),
		Task:             s.Task,
		Subtitle:         s.Subtitle,
		Addr:             s.Addr,
		Epoch:            epoch,
		Started:          started,
		AwaitingStart:    !started,
		Running:          live.Running,
		Phase:            live.Phase,
		Message:          live.Message,
		CellIndex:        live.CellIndex,
		CellTotal:        plan,
		Ok:               okE,
		Gap:              gapE,
		Fail:             failE,
		OkAll:            okAll,
		Recorded:         rec,
		Plan:             plan,
		EpochDone:        epochDone,
		ProgressPct:      pct,
		Current:          live.Current,
		Best:             live.Best,
		BestMobile:       live.BestMobile,
		BestLearn:        live.BestLearn,
		Winners:          computeWinners(live),
		ModeProgress:     s.modeProgress(live),
		Leaderboard:      append([]pulse.Result(nil), lb...),
		LeaderboardLearn: append([]pulse.Result(nil), learn...),
		BestAdapt:        adapt,
		BestSoft:         soft,
		BestHard:         hard,
		BestConsistency:  cons,
		BestStability:    stab,
		BestAccThru:      accThru,
		BestRealtime:     realtime,
		BestKeep:         keep,
		Heat:             heat,
		LPD:              lpd,
		APIs:             APIPaths(),
		Status:           boardStatus(started, live.Running, epochDone, plan),
	}
	out.Axes = LucyAxes(out)
	if ax := lpdAxis(out.LPD); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mspeed", "Thru vs board fastest; 0 unless Q≥70%", func(r report.LPDRow) float64 { return r.MSpeed }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mavail", "Avail vs board best duty cycle; 0 unless Q≥70%", func(r report.LPDRow) float64 { return r.MAvail }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mix", "geomean of Q, MSpeed, MAvail — live mobile blend", func(r report.LPDRow) float64 { return r.Mix }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if s := out.LPD.GoldStd; s.ID != "" {
		out.Axes = append(out.Axes, LucyAxis{
			Name: "gold_std", Hint: "smallest then fastest cell with Acc keep ≥80% of Acc champ",
			Value: s.RAMKiB, CellID: s.ID, Mode: s.Mode, DType: s.DType, Arch: s.Arch,
			Score: s.Score, SoftAcc: s.Soft, Thru: s.Thru, Avail: s.Avail,
		})
	}
	return out
}

func lpdAxis(l report.LPD) LucyAxis {
	pick := report.LPDRow{}
	if len(l.Gold) > 0 {
		pick = l.Gold[0]
	} else if len(l.Top) > 0 && l.Top[0].LPD > 0 {
		pick = l.Top[0]
	} else {
		return LucyAxis{}
	}
	return LucyAxis{
		Name: "lpd", Hint: "goldilocks: ≥80% Q and ≥80% of Acc champ at ≤20% RAM",
		Value: pick.LPD, CellID: pick.ID, Mode: pick.Mode, DType: pick.DType, Arch: pick.Arch,
		Score: pick.Score, SoftAcc: pick.Soft, Thru: pick.Thru, Avail: pick.Avail,
	}
}

func lpdPickAxis(l report.LPD, name, hint string, val func(report.LPDRow) float64) LucyAxis {
	src := l.Top
	switch name {
	case "mspeed":
		src = l.TopSpeed
	case "mavail":
		src = l.TopAvail
	case "mix":
		src = l.TopMix
	}
	if len(src) == 0 || val(src[0]) <= 0 {
		return LucyAxis{}
	}
	pick := src[0]
	return LucyAxis{
		Name: name, Hint: hint, Value: val(pick),
		CellID: pick.ID, Mode: pick.Mode, DType: pick.DType, Arch: pick.Arch,
		Score: pick.Score, SoftAcc: pick.Soft, Thru: pick.Thru, Avail: pick.Avail,
	}
}

func countResults(completed []pulse.Result, epoch int) (okE, gapE, failE, okAll, recorded int) {
	if epoch < 1 {
		epoch = 1
	}
	for _, r := range completed {
		recorded++
		switch r.Status {
		case "ok":
			okAll++
		}
		re := r.Epoch
		if re < 1 {
			re = 1
		}
		if re != epoch {
			continue
		}
		switch r.Status {
		case "ok":
			okE++
		case "gap":
			gapE++
		case "fail":
			failE++
		}
	}
	return
}

func boardStatus(started, running bool, done, total int) string {
	if running {
		return "running"
	}
	if total > 0 && done >= total {
		return "done"
	}
	if !started {
		return "paused"
	}
	if done == 0 {
		return "queued"
	}
	return "idle"
}

func (s *Server) livePayload() map[string]any {
	live := s.Tracker.SnapshotLive()
	meta := s.Meta()
	b := s.Board()
	cellTotal := b.Plan
	if cellTotal == 0 {
		cellTotal = live.CellTotal
	}
	heatLive := b.Heat
	if n := len(heatLive.Points); n > 240 {
		step := (n + 239) / 240
		slim := make([]report.CellPoint, 0, 240)
		for i := 0; i < n; i += step {
			slim = append(slim, heatLive.Points[i])
		}
		heatLive.Points = slim
	}
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
		"cell_total":               cellTotal,
		"message":                  live.Message,
		"mode_progress":            s.modeProgress(live),
		"winners":                  computeWinners(live),
		"awaiting_start":           meta.AwaitingStart,
		"started":                  meta.Started,
		"epoch":                    b.Epoch,
		"task":                     s.Task,
		"subtitle":                 s.Subtitle,
		"id":                       meta.ID,
		"addr":                     s.Addr,
		"apis":                     meta.APIs,
		"plan":                     b.Plan,
		"epoch_ok":                 b.Ok,
		"epoch_fail":               b.Fail,
		"epoch_done":               b.EpochDone,
		"ok_all":                   b.OkAll,
		"recorded":                 b.Recorded,
		"progress_pct":             b.ProgressPct,
		"axes":                     b.Axes,
		"heat":                     heatLive,
		"lpd":                      b.LPD,
	}
}
