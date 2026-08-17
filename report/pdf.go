package report

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/phpdave11/gofpdf"

	"github.com/openfluke/tide/pulse"
)

func PDFTide(r TideReport) ([]byte, error) {
	p := newDoc("tide — " + nz(r.Task, r.ID))
	p.coverTide(r)
	p.bests(r)
	p.axisTable("Lucy axis champions (this tide)", r.Axes)
	p.winners(r.Winners)
	p.leaderboard("Leaderboard (Lucy Score)", r.Leaderboard)
	p.bars("Top scores", scoresOf(r.Leaderboard))
	p.bars("Best settings per mode", winnerScores(r.Winners.BestSettingsPerMode, func(w WinnerRow) string { return w.Mode }))
	p.spark("Live pulse (Score)", r.History, func(h pulse.HistoryPoint) float64 { return h.Score })
	p.spark("Live pulse (SoftAcc)", r.History, func(h pulse.HistoryPoint) float64 { return h.Accuracy })
	p.spark("Live pulse (Availability)", r.History, func(h pulse.HistoryPoint) float64 { return h.Availability })
	p.spark("Live pulse (Throughput)", r.History, func(h pulse.HistoryPoint) float64 { return h.Throughput })
	p.modeQueue(r.ModeProgress)
	return p.bytes()
}

func PDFOcean(o OceanReport) ([]byte, error) {
	p := newDoc("ocean — " + nz(o.Title, "linked tides"))
	p.coverOcean(o)
	p.votes("Train mode votes", o.Holistic.ModeVotes)
	p.votes("DType votes", o.Holistic.DTypeVotes)
	p.votes("Arch votes", o.Holistic.ArchVotes)
	p.axisTable("Lucy axis champions (ocean-wide)", o.Holistic.Axes)
	p.bars("Train mode mean Score", voteBars(o.Holistic.ModeVotes))
	p.bars("DType mean Score", voteBars(o.Holistic.DTypeVotes))
	p.layerTable(o.Holistic.Layers)
	for _, l := range o.Holistic.Layers {
		if len(l.Axes) == 0 {
			continue
		}
		p.axisTable("Lucy axes — "+l.Tide, l.Axes)
	}
	p.topTable(o.Holistic.CombinedTop)
	p.bars("Score by layer", layerScores(o.Holistic.Layers))
	for _, t := range o.Tides {
		p.pdf.AddPage()
		p.coverTide(t)
		p.bests(t)
		p.axisTable("Lucy axes — "+nz(t.Task, t.ID), t.Axes)
		p.winners(t.Winners)
		p.leaderboard("Leaderboard — "+nz(t.Task, t.ID), t.Leaderboard)
		p.bars("Top scores — "+nz(t.Task, t.ID), scoresOf(t.Leaderboard))
		p.spark("Pulse Score — "+nz(t.Task, t.ID), t.History, func(h pulse.HistoryPoint) float64 { return h.Score })
		p.spark("Pulse SoftAcc — "+nz(t.Task, t.ID), t.History, func(h pulse.HistoryPoint) float64 { return h.Accuracy })
		p.spark("Pulse Availability — "+nz(t.Task, t.ID), t.History, func(h pulse.HistoryPoint) float64 { return h.Availability })
		p.spark("Pulse Throughput — "+nz(t.Task, t.ID), t.History, func(h pulse.HistoryPoint) float64 { return h.Throughput })
		p.modeQueue(t.ModeProgress)
	}
	return p.bytes()
}

type doc struct {
	pdf *gofpdf.Fpdf
}

func newDoc(title string) *doc {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, false)
	pdf.SetAuthor("tide / ocean Lucy report", false)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(120, 130, 140)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s  ·  page %d", title, pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	return &doc{pdf: pdf}
}

func (d *doc) bytes() ([]byte, error) {
	var buf bytes.Buffer
	err := d.pdf.Output(&buf)
	return buf.Bytes(), err
}

func (d *doc) coverTide(r TideReport) {
	d.h1("tide  " + nz(r.Task, r.ID))
	d.muted(r.Formula)
	d.muted(fmt.Sprintf("%s  ·  epoch %d  ·  %s  ·  %s", r.Status, r.Epoch, r.Generated.Format("2006-01-02 15:04:05"), r.Addr))
	d.body(fmt.Sprintf("This epoch %d / %d cells. Recorded %d results.",
		r.EpochDone, r.Plan, r.Recorded))
	d.body("Score 0 usually means the cell finished before a Lucy pulse (SoftAcc never sampled) or true-class mass was 0. Those rows do not vote holistically.")
	if r.Subtitle != "" {
		d.body(r.Subtitle)
	}
	d.gap(3)
}

func (d *doc) coverOcean(o OceanReport) {
	d.h1("ocean  " + nz(o.Title, "linked tides"))
	d.muted("Score = Throughput x Availability% x SoftAcc% / 10,000   ·   Lucy / test41 harness")
	h := o.Holistic
	d.body(fmt.Sprintf("Generated %s. Tides up %d / %d. This-epoch cells %d / %d.",
		o.Generated.Format("2006-01-02 15:04:05"), h.TidesUp, h.TidesTotal, h.CellsDone, h.CellsTotal))
	d.body(fmt.Sprintf("Holistic best train mode: %s     best dtype: %s     best arch: %s", nz(h.BestMode, "—"), nz(h.BestDType, "—"), nz(h.BestArch, "—")))
	d.body(fmt.Sprintf("Suggested default (most Lucy-axis wins): %s | %s | %s  (%d axes)",
		nz(h.DefaultMode, "—"), nz(h.DefaultDType, "—"), nz(h.DefaultArch, "—"), h.DefaultWins))
	d.gap(3)
}

func (d *doc) bests(r TideReport) {
	d.h2("Lucy champions (this tide)")
	type row struct{ axis, id string; score, soft, thru, avail float64 }
	var rows []row
	add := func(axis string, res *pulse.Result) {
		if res == nil {
			return
		}
		rows = append(rows, row{axis, res.Cell.ID, res.Snapshot.Score, res.Snapshot.SoftAcc, res.Snapshot.Throughput, res.Snapshot.Availability})
	}
	add("score", r.Best.Score)
	add("throughput", r.Best.Throughput)
	add("availability", r.Best.Availability)
	add("accuracy", r.Best.Accuracy)
	if r.BestMobile.Score != nil {
		add("mobile score", r.BestMobile.Score)
	}
	if r.BestLearn.To50 != nil {
		add("t->50%", r.BestLearn.To50)
	}
	if r.BestLearn.AccPerSec != nil {
		add("acc/sec", r.BestLearn.AccPerSec)
	}
	d.table([]string{"Axis", "Cell", "Score", "Soft", "Thru", "Avail"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{r.axis, clip(r.id, 42), fmt.Sprintf("%.2f", r.score), fmt.Sprintf("%.1f", r.soft), fmt.Sprintf("%.1f", r.thru), fmt.Sprintf("%.1f", r.avail)}
	})
}

func (d *doc) winners(w WinnersView) {
	d.h2("Best settings per train mode")
	d.table([]string{"Mode", "DType", "Format", "Score", "Soft", "Avail", "n"}, func(k int) []string {
		if k >= len(w.BestSettingsPerMode) || k >= 23 {
			return nil
		}
		a := w.BestSettingsPerMode[k]
		return []string{clip(a.Mode, 22), a.DType, a.Format, fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc), fmt.Sprintf("%.1f", a.Avail), fmt.Sprintf("%d", a.N)}
	})
	d.h2("Best dtype per mode")
	d.table([]string{"Mode", "Winner dtype", "Score", "Soft"}, func(k int) []string {
		if k >= len(w.BestDTypePerMode) || k >= 23 {
			return nil
		}
		a := w.BestDTypePerMode[k]
		return []string{clip(a.Mode, 22), a.Winner, fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc)}
	})
	d.h2("Best mode per dtype")
	d.table([]string{"DType", "Winner mode", "Score", "Soft"}, func(k int) []string {
		if k >= len(w.BestModePerDType) || k >= 34 {
			return nil
		}
		a := w.BestModePerDType[k]
		return []string{a.Group, clip(a.Winner, 22), fmt.Sprintf("%.1f", a.Score), fmt.Sprintf("%.1f", a.SoftAcc)}
	})
}

func (d *doc) leaderboard(title string, rows []pulse.Result) {
	d.h2(title)
	d.table([]string{"#", "Cell", "Score", "Soft", "Hard", "Thru", "Avail"}, func(k int) []string {
		if k >= len(rows) || k >= 15 {
			return nil
		}
		r := rows[k]
		s := r.Snapshot
		return []string{
			fmt.Sprintf("%d", k+1), clip(r.Cell.ID, 40),
			fmt.Sprintf("%.1f", s.Score), fmt.Sprintf("%.1f", s.SoftAcc),
			fmt.Sprintf("%.1f", s.AvgAccuracy), fmt.Sprintf("%.1f", s.Throughput),
			fmt.Sprintf("%.1f", s.Availability),
		}
	})
}

func (d *doc) modeQueue(ms []ModeRow) {
	d.h2("Queue by train mode (this epoch)")
	d.table([]string{"Mode", "Done", "Run", "Left", "Total"}, func(k int) []string {
		if k >= len(ms) {
			return nil
		}
		m := ms[k]
		return []string{clip(m.Mode, 24), itoa(m.Done), itoa(m.Running), itoa(m.Left), itoa(m.Total)}
	})
}

func (d *doc) votes(title string, vs []Vote) {
	d.h2(title)
	d.table([]string{"Key", "Layers", "Mean Score"}, func(k int) []string {
		if k >= len(vs) || k >= 16 {
			return nil
		}
		v := vs[k]
		return []string{clip(v.Key, 28), itoa(v.Count), fmt.Sprintf("%.2f", v.Mean)}
	})
}

func (d *doc) axisTable(title string, rows []AxisView) {
	if len(rows) == 0 {
		return
	}
	d.h2(title)
	d.table([]string{"Axis", "Tide", "Mode", "DType", "Arch", "Value"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{clip(r.Name, 14), clip(nz(r.Tide, "—"), 12), clip(r.Mode, 16), r.DType, clip(r.Arch, 14), fmt.Sprintf("%.2f", r.Value)}
	})
}

func (d *doc) layerTable(rows []LayerRow) {
	d.h2("Per-layer best (mode x dtype x arch)")
	d.table([]string{"Tide", "Mode", "DType", "Arch", "Score", "Soft"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{r.Tide, clip(r.Mode, 16), r.DType, clip(r.Arch, 12), fmt.Sprintf("%.1f", r.Score), fmt.Sprintf("%.1f", r.SoftAcc)}
	})
}

func (d *doc) topTable(rows []TopRow) {
	d.h2("Combined leaderboard")
	d.table([]string{"Tide", "Cell", "Score", "Soft", "Avail"}, func(k int) []string {
		if k >= len(rows) || k >= 20 {
			return nil
		}
		r := rows[k]
		return []string{r.Tide, clip(r.CellID, 40), fmt.Sprintf("%.1f", r.Score), fmt.Sprintf("%.1f", r.SoftAcc), fmt.Sprintf("%.1f", r.Avail)}
	})
}

type kv struct {
	Label string
	Val   float64
}

func scoresOf(rows []pulse.Result) []kv {
	out := make([]kv, 0, len(rows))
	for i, r := range rows {
		if i >= 16 {
			break
		}
		out = append(out, kv{clip(r.Cell.ID, 36), r.Snapshot.Score})
	}
	return out
}

func layerScores(rows []LayerRow) []kv {
	out := make([]kv, 0, len(rows))
	for _, r := range rows {
		out = append(out, kv{r.Tide, r.Score})
	}
	return out
}

func winnerScores(rows []WinnerRow, label func(WinnerRow) string) []kv {
	out := make([]kv, 0, len(rows))
	for i, r := range rows {
		if i >= 16 {
			break
		}
		out = append(out, kv{clip(label(r), 36), r.Score})
	}
	return out
}

func voteBars(vs []Vote) []kv {
	out := make([]kv, 0, len(vs))
	for _, v := range vs {
		out = append(out, kv{clip(v.Key, 28), v.Mean})
	}
	return out
}

func (d *doc) bars(title string, items []kv) {
	if len(items) == 0 {
		return
	}
	d.h2(title)
	max := 1.0
	for _, it := range items {
		if it.Val > max {
			max = it.Val
		}
	}
	left, colW := 18.0, 170.0
	barH := 4.2
	need := float64(len(items))*barH + 8
	if d.pdf.GetY()+need > 275 {
		d.pdf.AddPage()
	}
	for _, it := range items {
		y := d.pdf.GetY()
		d.pdf.SetFont("Helvetica", "", 7)
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(52, barH, it.Label, "", 0, "L", false, 0, "")
		w := 0.0
		if max > 0 {
			w = (colW - 70) * it.Val / max
		}
		d.pdf.SetFillColor(61, 214, 198)
		d.pdf.Rect(left+54, y+0.7, w, barH-1.4, "F")
		d.pdf.SetXY(left+56+w, y)
		d.pdf.CellFormat(30, barH, fmt.Sprintf("%.1f", it.Val), "", 1, "L", false, 0, "")
	}
	d.gap(3)
}

func (d *doc) spark(title string, hist []pulse.HistoryPoint, val func(pulse.HistoryPoint) float64) {
	if len(hist) < 2 {
		return
	}
	d.h2(title)
	if d.pdf.GetY() > 240 {
		d.pdf.AddPage()
	}
	x0, y0, w, h := 18.0, d.pdf.GetY()+2, 174.0, 32.0
	d.pdf.SetDrawColor(40, 55, 65)
	d.pdf.Rect(x0, y0, w, h, "D")
	min, max := val(hist[0]), val(hist[0])
	for _, p := range hist {
		v := val(p)
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if max <= min {
		max = min + 1
	}
	d.pdf.SetDrawColor(61, 214, 198)
	d.pdf.SetLineWidth(0.25)
	for i := 1; i < len(hist); i++ {
		x1 := x0 + w*float64(i-1)/float64(len(hist)-1)
		x2 := x0 + w*float64(i)/float64(len(hist)-1)
		y1 := y0 + h - h*(val(hist[i-1])-min)/(max-min)
		y2 := y0 + h - h*(val(hist[i])-min)/(max-min)
		d.pdf.Line(x1, y1, x2, y2)
	}
	d.pdf.SetFont("Helvetica", "", 7)
	d.pdf.SetTextColor(120, 130, 140)
	d.pdf.SetXY(x0, y0+h+1)
	d.pdf.CellFormat(w, 5, fmt.Sprintf("n=%d   min=%.2f   max=%.2f", len(hist), min, max), "", 1, "L", false, 0, "")
	d.gap(4)
}

func (d *doc) h1(s string) {
	d.pdf.SetFont("Helvetica", "B", 16)
	d.pdf.SetTextColor(20, 40, 48)
	d.pdf.CellFormat(0, 9, s, "", 1, "L", false, 0, "")
}

func (d *doc) h2(s string) {
	if d.pdf.GetY() > 265 {
		d.pdf.AddPage()
	}
	d.pdf.SetFont("Helvetica", "B", 11)
	d.pdf.SetTextColor(30, 90, 95)
	d.pdf.CellFormat(0, 7, s, "", 1, "L", false, 0, "")
}

func (d *doc) muted(s string) {
	d.pdf.SetFont("Helvetica", "", 8)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.MultiCell(0, 4, s, "", "L", false)
}

func (d *doc) body(s string) {
	d.pdf.SetFont("Helvetica", "", 9)
	d.pdf.SetTextColor(40, 50, 55)
	d.pdf.MultiCell(0, 4.5, s, "", "L", false)
}

func (d *doc) gap(mm float64) { d.pdf.Ln(mm) }

func (d *doc) table(headers []string, row func(int) []string) {
	cols := len(headers)
	widths := make([]float64, cols)
	left := 174.0
	for i := range widths {
		if i == 1 && cols > 3 {
			widths[i] = left * 0.38
		} else if i == 0 {
			widths[i] = left * 0.18
		} else {
			widths[i] = left * (1 - 0.18 - 0.38) / float64(cols-2)
			if cols <= 3 {
				widths[i] = left / float64(cols)
			}
		}
	}
	if cols <= 3 {
		for i := range widths {
			widths[i] = 174 / float64(cols)
		}
	}
	d.pdf.SetFont("Helvetica", "B", 7)
	d.pdf.SetFillColor(22, 40, 48)
	d.pdf.SetTextColor(230, 240, 242)
	for i, h := range headers {
		d.pdf.CellFormat(widths[i], 6, h, "1", 0, "L", true, 0, "")
	}
	d.pdf.Ln(-1)
	d.pdf.SetFont("Helvetica", "", 7)
	d.pdf.SetTextColor(30, 40, 45)
	for i := 0; ; i++ {
		r := row(i)
		if r == nil {
			break
		}
		if d.pdf.GetY() > 275 {
			d.pdf.AddPage()
			d.pdf.SetFont("Helvetica", "B", 7)
			d.pdf.SetFillColor(22, 40, 48)
			d.pdf.SetTextColor(230, 240, 242)
			for j, h := range headers {
				d.pdf.CellFormat(widths[j], 6, h, "1", 0, "L", true, 0, "")
			}
			d.pdf.Ln(-1)
			d.pdf.SetFont("Helvetica", "", 7)
			d.pdf.SetTextColor(30, 40, 45)
		}
		fill := i%2 == 1
		if fill {
			d.pdf.SetFillColor(236, 242, 244)
		}
		for j := 0; j < cols && j < len(r); j++ {
			d.pdf.CellFormat(widths[j], 5, r[j], "1", 0, "L", fill, 0, "")
		}
		d.pdf.Ln(-1)
	}
	d.gap(3)
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "×", "x")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
