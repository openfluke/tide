package report

import (
	"fmt"

	"github.com/openfluke/tide/pulse"
)

func (d *doc) sweepProgress(r TideReport) {
	d.h2("Sweep progress")
	left := r.Plan - r.EpochDone
	if left < 0 {
		left = 0
	}
	d.body(fmt.Sprintf("Epoch %d done %d / %d plan (%.0f%%). ok %d · gap %d · fail %d · running %d · recorded %d.",
		r.Epoch, r.EpochDone, r.Plan, r.ProgressPct, r.EpochOk, r.EpochGap, r.EpochFail, r.RunningN, r.Recorded))
	if r.Plan > 0 {
		d.bars("Epoch completion", []kv{{"done", float64(r.EpochDone)}, {"left", float64(left)}})
	}
}

func (d *doc) modelsRanked(l LPD) {
	if len(l.Top) == 0 {
		return
	}
	d.h2("Models ranked — live-fit + Lucy density")
	n := len(l.Top)
	if n > 20 {
		n = 20
	}
	bars := make([]kv, 0, n)
	for i := 0; i < n; i++ {
		r := l.Top[i]
		bars = append(bars, kv{CompactCell(r.ID), r.LPD})
	}
	d.bars("Top LPD (memory density)", bars)
	d.lpdTable(l.Top, n)
}

func (d *doc) winnersAll(w WinnersView) {
	d.h2("Best settings per train mode")
	d.winnerTable([]string{"Mode", "Arch", "DType", "Fmt", "Score", "Soft", "Acc", "Avail", "t50", "Acc/s", "KiB", "n"}, w.BestSettingsPerMode, true)
	d.h2("Best cell per mode x arch")
	d.winnerTable([]string{"Group", "Cell", "Score", "Soft", "Acc", "t50", "Acc/s", "n"}, w.BestCellPerMode, false)
	d.h2("Best dtype per train mode")
	d.winnerTable([]string{"Mode", "Dtype", "Score", "Soft", "Acc", "n"}, w.BestDTypePerMode, false)
	d.h2("Best quant format per train mode")
	d.winnerTable([]string{"Mode", "Format", "Score", "Soft", "Acc", "n"}, w.BestFormatPerMode, false)
	d.h2("Best train mode per dtype")
	d.winnerTable([]string{"Dtype", "Mode", "Score", "Soft", "Acc", "n"}, w.BestModePerDType, false)
	d.h2("Best train mode per quant format")
	d.winnerTable([]string{"Format", "Mode", "Score", "Soft", "Acc", "n"}, w.BestModePerFormat, false)
	d.h2("Best format per dtype")
	d.winnerTable([]string{"Dtype", "Format", "Score", "Soft", "Acc", "n"}, w.BestFormatPerDType, false)
}

func (d *doc) winnerTable(headers []string, rows []WinnerRow, recipe bool) {
	if len(rows) == 0 {
		d.body("—")
		return
	}
	d.table(headers, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		a := rows[k]
		if recipe {
			return []string{
				PrettyMode(a.Mode), PrettyArch(a.Arch), a.DType, a.Format,
				fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc), fmt.Sprintf("%.1f", a.Acc),
				fmt.Sprintf("%.1f", a.Avail), fmtSec(a.TimeTo50), fmt.Sprintf("%.2f", a.AccPerSec),
				fmt.Sprintf("%.1f", a.WeightKiB), fmt.Sprintf("%d", a.N),
			}
		}
		switch len(headers) {
		case 6:
			return []string{a.Group, PrettyMode(a.Winner), fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc), fmt.Sprintf("%.1f", a.Acc), fmt.Sprintf("%d", a.N)}
		case 8:
			return []string{a.Group, PrettyCell(a.CellID), fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc), fmt.Sprintf("%.1f", a.Acc), fmtSec(a.TimeTo50), fmt.Sprintf("%.2f", a.AccPerSec), fmt.Sprintf("%d", a.N)}
		default:
			return []string{a.Group, PrettyMode(a.Winner), fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc), fmt.Sprintf("%.1f", a.Acc), fmt.Sprintf("%d", a.N)}
		}
	})
}

func fmtSec(s float64) string {
	if s <= 0 {
		return "—"
	}
	if s < 60 {
		return fmt.Sprintf("%.0fs", s)
	}
	return fmt.Sprintf("%.1fm", s/60)
}

func (d *doc) learnSpeed(r TideReport) {
	d.h2("Learn speed")
	to50 := make([]kv, 0, 16)
	aps := make([]kv, 0, 16)
	for i, res := range r.LeaderboardLearn {
		if i >= 16 || res.Snapshot.TimeToAcc50Sec <= 0 {
			continue
		}
		to50 = append(to50, kv{PrettyCell(res.Cell.ID), 1 / res.Snapshot.TimeToAcc50Sec})
	}
	for i, res := range r.LeaderboardLearn {
		if i >= 16 || res.Snapshot.AccPerSec <= 0 {
			continue
		}
		aps = append(aps, kv{PrettyCell(res.Cell.ID), res.Snapshot.AccPerSec})
	}
	if len(to50) > 0 {
		d.bars("Time to 50% acc (bar = 1/time)", to50)
	}
	if len(aps) > 0 {
		d.bars("Acc / second", aps)
	}
}

func (d *doc) boardLeaderboards(r TideReport) {
	d.leaderboardFull("All models — raw Score", r.Leaderboard, boardColsScore)
	d.leaderboardFull("All models — Score/MiB (trap)", r.LeaderboardMobile, boardColsMobile)
	d.leaderboardFull("Learn — fastest to 50%", r.LeaderboardLearn, boardColsLearn)
	d.leaderboardFull("Learn — Acc/sec/MiB (trap)", r.LeaderboardLearnMobile, boardColsLearnMobile)
}

type boardCol struct {
	head string
	val  func(pulse.Result) string
}

var (
	boardColsScore = []boardCol{
		{"Score", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.Score) }},
		{"Soft", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.SoftAcc) }},
		{"Acc", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.AvgAccuracy) }},
		{"Avail", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.Availability) }},
		{"Thru", func(r pulse.Result) string { return fmt.Sprintf("%.0f", r.Snapshot.Throughput) }},
		{"KiB", func(r pulse.Result) string { return fmt.Sprintf("%.1f", float64(r.Snapshot.WeightBytes)/1024) }},
	}
	boardColsMobile = []boardCol{
		{"S/MiB", func(r pulse.Result) string { return fmt.Sprintf("%.2f", r.Snapshot.MobileScore) }},
		{"Score", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.Score) }},
		{"Acc", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.AvgAccuracy) }},
		{"KiB", func(r pulse.Result) string { return fmt.Sprintf("%.1f", float64(r.Snapshot.WeightBytes)/1024) }},
	}
	boardColsLearn = []boardCol{
		{"t50", func(r pulse.Result) string { return fmtSec(r.Snapshot.TimeToAcc50Sec) }},
		{"Acc/s", func(r pulse.Result) string { return fmt.Sprintf("%.2f", r.Snapshot.AccPerSec) }},
		{"Acc", func(r pulse.Result) string { return fmt.Sprintf("%.1f", r.Snapshot.AvgAccuracy) }},
	}
	boardColsLearnMobile = []boardCol{
		{"A/s/MiB", func(r pulse.Result) string { return fmt.Sprintf("%.2f", r.Snapshot.MobileAccPerSec) }},
		{"Acc/s", func(r pulse.Result) string { return fmt.Sprintf("%.2f", r.Snapshot.AccPerSec) }},
		{"KiB", func(r pulse.Result) string { return fmt.Sprintf("%.1f", float64(r.Snapshot.WeightBytes)/1024) }},
	}
)

func (d *doc) leaderboardFull(title string, rows []pulse.Result, cols []boardCol) {
	if len(rows) == 0 {
		return
	}
	headers := make([]string, 0, 2+len(cols))
	headers = append(headers, "#", "Cell")
	for _, c := range cols {
		headers = append(headers, c.head)
	}
	d.h2(title)
	limit := len(rows)
	if limit > 40 {
		limit = 40
	}
	d.table(headers, func(k int) []string {
		if k >= limit {
			return nil
		}
		r := rows[k]
		out := []string{fmt.Sprintf("%d", k+1), PrettyCell(r.Cell.ID)}
		for _, c := range cols {
			out = append(out, c.val(r))
		}
		return out
	})
}
