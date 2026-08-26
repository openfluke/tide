package dash

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/report"
)

func (s *Server) reportCompleted(live pulse.Live) []pulse.Result {
	if s != nil && s.Tracker != nil {
		if full := s.Tracker.ReportResults(); len(full) > 0 {
			return full
		}
	}
	return live.Completed
}

func reportLeaderboard(completed []pulse.Result, n int) []pulse.Result {
	out := append([]pulse.Result(nil), completed...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Snapshot.Score > out[j].Snapshot.Score
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (s *Server) Report() report.TideReport {
	var hist []pulse.HistoryPoint
	var live pulse.Live
	if s != nil && s.Tracker != nil {
		hist = s.Tracker.History()
		live = s.Tracker.SnapshotLive()
	}
	reportRows := s.reportCompleted(live)
	b := s.Board()
	enrichBoardFromCompleted(&b, s, live, reportRows, s.Task)
	pts := report.PointsFromResults(reportRows, s.Task)

	r := b.ToReport(hist)
	r.Cells = pts
	r.Heat = b.Heat
	r.LPD = b.LPD
	r.Winners = winnersView(b.Winners)
	r.ModeProgress = modeProgressRows(b.ModeProgress)
	r.Axes = axesView(b.Task, b.Axes)
	r.Leaderboard = reportLeaderboard(reportRows, 40)
	return r
}

func modeProgressRows(rows []ModeProgress) []report.ModeRow {
	out := make([]report.ModeRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, report.ModeRow{
			Mode: m.Mode, Total: m.Total, Done: m.Done, Running: m.Running, Left: m.Left,
		})
	}
	return out
}

func (s *Server) handleReportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.Report())
}

func (s *Server) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	pdf, err := report.PDFTide(s.Report())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	name := s.identityID()
	if name == "" {
		name = "tide"
	}
	if s.LR > 0 {
		name += "-lr" + report.FormatLR(s.LR)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`-lucy-report.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

// ToReport maps a live board into the printable Lucy snapshot.
func (b Board) ToReport(history []pulse.HistoryPoint) report.TideReport {
	if len(history) > 400 {
		history = history[len(history)-400:]
	}
	modes := make([]report.ModeRow, 0, len(b.ModeProgress))
	for _, m := range b.ModeProgress {
		modes = append(modes, report.ModeRow{
			Mode: m.Mode, Total: m.Total, Done: m.Done, Running: m.Running, Left: m.Left,
		})
	}
	return report.TideReport{
		Generated:    time.Now(),
		Kind:         "tide",
		ID:           b.ID,
		Task:         b.Task,
		Subtitle:     b.Subtitle,
		LR:           b.LR,
		Addr:         b.Addr,
		Epoch:           b.Epoch,
		EpochMax:        b.EpochMax,
		EpochsLeft:      b.EpochsLeft,
		EpochOverallPct: b.EpochOverallPct,
		Plan:            b.Plan,
		EpochDone:       b.EpochDone,
		ProgressPct:     b.ProgressPct,
		Recorded:        b.Recorded,
		Status:          b.Status,
		Formula:      "Lucy Score = Throughput x Availability x Acc / 10,000. LPD condenses live-fit vs Acc-champ RAM.",
		Best:         b.Best,
		BestMobile:   b.BestMobile,
		BestLearn:    b.BestLearn,
		Winners:      winnersView(b.Winners),
		Leaderboard:  append([]pulse.Result(nil), b.Leaderboard...),
		ModeProgress: modes,
		History:      append([]pulse.HistoryPoint(nil), history...),
		Axes:         axesView(b.Task, b.Axes),
		Heat:         b.Heat,
		LPD:          b.LPD,
	}
}

func axesView(tide string, xs []LucyAxis) []report.AxisView {
	out := make([]report.AxisView, 0, len(xs))
	for _, a := range xs {
		out = append(out, report.AxisView{
			Name: a.Name, Hint: a.Hint, Tide: tide, CellID: report.PrettyCell(a.CellID),
			Mode: a.Mode, DType: a.DType, Arch: report.PrettyArch(a.Arch), Value: a.Value,
			SoftAcc: a.SoftAcc, Thru: a.Thru,
		})
	}
	return out
}

func winnersView(w Winners) report.WinnersView {
	mapRows := func(xs []AxisWinner) []report.WinnerRow {
		out := make([]report.WinnerRow, 0, len(xs))
		for _, a := range xs {
			out = append(out, report.WinnerRow{
				Group: a.Group, Winner: a.Winner, CellID: report.PrettyCell(a.CellID),
				Mode: a.Mode, DType: a.DType, Format: a.Format, Arch: report.PrettyArch(a.Arch),
				Score: a.Score, SoftAcc: a.SoftAcc, Acc: a.Accuracy, Avail: a.Avail, N: a.N,
			})
		}
		return out
	}
	return report.WinnersView{
		BestSettingsPerMode: mapRows(w.BestSettingsPerMode),
		BestDTypePerMode:    mapRows(w.BestDTypePerMode),
		BestModePerDType:    mapRows(w.BestModePerDType),
	}
}
