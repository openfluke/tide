package dash

import (
	"github.com/openfluke/tide/checkpoint"
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
		"cell":    "/api/cell",
		"charts":  "/api/charts/",
		"report":  "/api/report.pdf",
	}
}

// Meta is the lightweight identity payload (no leaderboards).
type Meta struct {
	ID            string            `json:"id"`
	Task          string            `json:"task"`
	Subtitle      string            `json:"subtitle"`
	LR            float64           `json:"lr"`
	Addr          string            `json:"addr"`
	Epoch         int               `json:"epoch"`
	EpochMax      int               `json:"epoch_max"`
	EpochsLeft    int               `json:"epochs_left"`
	EpochOverallPct float64         `json:"epoch_overall_pct"`
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
	LR               float64           `json:"lr"`
	Addr             string            `json:"addr"`
	Epoch            int               `json:"epoch"`
	EpochMax         int               `json:"epoch_max"`
	EpochsLeft       int               `json:"epochs_left"`
	EpochOverallPct  float64           `json:"epoch_overall_pct"`
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
	epoch, emax, left, overall := s.epochProgress(0)
	return Meta{
		ID:              s.identityID(),
		Task:            s.Task,
		Subtitle:        s.Subtitle,
		LR:              s.LR,
		Addr:            s.Addr,
		Epoch:           epoch,
		EpochMax:        emax,
		EpochsLeft:      left,
		EpochOverallPct: overall,
		Started:         started,
		AwaitingStart:   !started,
		CellTotal:       total,
		Ocean:           false,
		APIs:            APIPaths(),
	}
}

// epochProgress returns current epoch, planned max, epochs still to finish
// (including the current one), and overall % across epochs for this LR.
// withinPct is ProgressPct of the current epoch (0–100).
// When EpochMax is unset (0), emax/left/overall stay 0 so the UI hides multi-epoch chrome.
func (s *Server) epochProgress(withinPct float64) (epoch, emax, left int, overall float64) {
	epoch = 1
	if s != nil && s.Epoch > 0 {
		epoch = s.Epoch
	}
	if s == nil || s.EpochMax < 1 {
		return epoch, 0, 0, 0
	}
	emax = s.EpochMax
	if emax < epoch {
		emax = epoch
	}
	left = emax - epoch + 1
	if left < 0 {
		left = 0
	}
	frac := float64(epoch-1) + withinPct/100
	if frac < 0 {
		frac = 0
	}
	if frac > float64(emax) {
		frac = float64(emax)
	}
	overall = 100 * frac / float64(emax)
	return epoch, emax, left, overall
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
	if live.Recorded > rec {
		rec = live.Recorded
	}
	// Hosts like test53 drive queue via SetMeta(CellIndex); trust that for progress.
	if live.CellIndex > 0 && live.CellTotal > 0 {
		if live.CellIndex > rec {
			rec = live.CellIndex
		}
	}
	plan := 0
	if s != nil {
		plan = len(s.Cells)
	}
	if plan == 0 {
		plan = live.CellTotal
	}
	doneSet := checkpoint.DoneSetFromCompleted(live.Completed, epoch)
	if s != nil && s.Tracker != nil {
		// Durable done set survives Completed trim + includes seeded skip IDs.
		doneSet = s.Tracker.DoneSet()
	}
	planDone := checkpoint.PlanDoneCount(s.Cells, doneSet)
	epochDone := planDone
	if epochDone > plan {
		epochDone = plan
	}
	pct := 0.0
	if plan > 0 {
		pct = 100 * float64(planDone) / float64(plan)
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
	completed := s.reportCompleted(live)
	pts := report.PointsFromResults(completed, s.Task)
	lpd := report.BuildLPD(pts)
	heat := report.BuildHeat(pts)
	_, emax, eleft, eoverall := s.epochProgress(pct)
	running := live.Running
	// Prefer plan completion over a stuck Running flag after Park/finish.
	if plan > 0 && planDone >= plan {
		running = false
	}
	out := Board{
		ID:               s.identityID(),
		Task:             s.Task,
		Subtitle:         s.Subtitle,
		LR:               s.LR,
		Addr:             s.Addr,
		Epoch:            epoch,
		EpochMax:         emax,
		EpochsLeft:       eleft,
		EpochOverallPct:  eoverall,
		Started:          started,
		AwaitingStart:    !started,
		Running:          running,
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
		Status:           boardStatus(started, running, epochDone, plan),
	}
	out.Axes = boardLPDAxes(out)
	return out
}

// enrichBoardFromCompleted rebuilds chart/winner tables from the full run archive (PDF).
func enrichBoardFromCompleted(b *Board, s *Server, live pulse.Live, completed []pulse.Result, task string) {
	if len(completed) == 0 {
		return
	}
	liveFull := live
	liveFull.Completed = completed
	pts := report.PointsFromResults(completed, task)
	b.Heat = report.BuildHeat(pts)
	b.LPD = report.BuildLPD(pts)
	b.Winners = computeWinners(liveFull)
	if s != nil {
		b.ModeProgress = s.modeProgress(liveFull)
	}
	adapt, soft, hard, cons, stab, accThru, realtime, keep := extraBests(completed)
	b.BestAdapt = adapt
	b.BestSoft = soft
	b.BestHard = hard
	b.BestConsistency = cons
	b.BestStability = stab
	b.BestAccThru = accThru
	b.BestRealtime = realtime
	b.BestKeep = keep
	b.Axes = boardLPDAxes(*b)
}

func boardLPDAxes(out Board) []LucyAxis {
	out.Axes = LucyAxes(out)
	if ax := lpdAxis(out.LPD); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mspeed", "Thru keep vs learner-fast peak; 0 unless Acc keep ≥70%", func(r report.LPDRow) float64 { return r.MSpeed }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mavail", "Avail keep vs learner-best duty; 0 unless Acc keep ≥70%", func(r report.LPDRow) float64 { return r.MAvail }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if ax := lpdPickAxis(out.LPD, "mix", "consciousness Q = geomean Acc/Thru/Avail keep", func(r report.LPDRow) float64 { return r.Mix }); ax.Name != "" {
		out.Axes = append(out.Axes, ax)
	}
	if s := out.LPD.GoldStd; s.ID != "" {
		out.Axes = append(out.Axes, LucyAxis{
			Name: "gold_std", Hint: "2+ pillars with Acc keep ≥80%, then smallest RAM then fastest",
			Value: s.RAMKiB, CellID: s.ID, Mode: s.Mode, DType: s.DType, Arch: s.Arch,
			Score: s.Score, SoftAcc: s.Soft, Thru: s.Thru, Avail: s.Avail,
		})
	}
	if s := out.LPD.LeanChamp; s.ID != "" {
		out.Axes = append(out.Axes, LucyAxis{
			Name: "lean_95", Hint: "Acc keep ≥95% of Acc champ, then smallest RAM / fastest Thru / Avail",
			Value: s.RAMKiB, CellID: s.ID, Mode: s.Mode, DType: s.DType, Arch: s.Arch,
			Score: s.Score, SoftAcc: s.Soft, Thru: s.Thru, Avail: s.Avail,
		})
	}
	return out.Axes
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
		Name: "lpd", Hint: "Lucy density: Q × shrink vs Acc-champ RAM; 0 unless Acc keep ≥70%",
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
	okSeen := map[string]bool{}
	gapSeen := map[string]bool{}
	failSeen := map[string]bool{}
	for _, r := range completed {
		recorded++
		id := r.Cell.ID
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
			if !okSeen[id] {
				okSeen[id] = true
				okE++
			}
		case "gap":
			if !gapSeen[id] {
				gapSeen[id] = true
				gapE++
			}
		case "fail":
			if !failSeen[id] {
				failSeen[id] = true
				failE++
			}
		}
	}
	return
}

func boardStatus(started, running bool, done, total int) string {
	if total > 0 && done >= total {
		return "done"
	}
	if running {
		return "running"
	}
	if !started {
		return "paused"
	}
	if done == 0 {
		return "queued"
	}
	return "idle"
}
