package report

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/phpdave11/gofpdf"

	"github.com/openfluke/tide/pulse"
)

func PDFTide(r TideReport) ([]byte, error) {
	p := newDoc("tide — " + nz(r.Task, r.ID))
	p.coverTide(r)
	p.sweepProgress(r)
	p.bests(r)
	p.axisTable("Lucy axis champions (this tide)", r.Axes)
	p.modelsRanked(r.LPD)
	p.winnersAll(r.Winners)
	p.leaderboard("Leaderboard (Lucy Score)", r.Leaderboard)
	p.leaderboardBy("Leaderboard (SoftAcc)", r.Leaderboard, func(x pulse.Result) float64 { return x.Snapshot.SoftAcc })
	p.leaderboardBy("Leaderboard (hard Acc)", r.Leaderboard, func(x pulse.Result) float64 { return x.Snapshot.AvgAccuracy })
	p.boardLeaderboards(r)
	p.learnSpeed(r)
	p.bars("Top scores", scoresOf(r.Leaderboard))
	p.bars("Best settings per mode", winnerScores(r.Winners.BestSettingsPerMode, func(w WinnerRow) string { return w.Mode }))
	p.lucyCharts(r.Cells, r.Heat)
	p.lpdBoard(r.LPD)
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
	p.lucyCharts(nil, o.Holistic.Heat)
	p.lpdBoard(o.Holistic.LPD)
	for _, t := range o.Tides {
		p.pdf.AddPage()
		p.coverTide(t)
		p.bests(t)
		p.axisTable("Lucy axes — "+nz(t.Task, t.ID), t.Axes)
		p.modelsRanked(t.LPD)
		p.winnersAll(t.Winners)
		p.leaderboard("Leaderboard — "+nz(t.Task, t.ID), t.Leaderboard)
		p.bars("Top scores — "+nz(t.Task, t.ID), scoresOf(t.Leaderboard))
		p.lucyCharts(t.Cells, t.Heat)
		p.lpdBoard(t.LPD)
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
	title = latin(title)
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetTitle(title, false)
	pdf.SetAuthor("tide / ocean Lucy report", false)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(120, 130, 140)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s  -  page %d", title, pdf.PageNo()), "", 0, "C", false, 0, "")
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
	d.muted(ModeLegend())
	d.muted(fmt.Sprintf("%s  ·  epoch %s  ·  lr %s  ·  %s  ·  %s", r.Status, formatEpoch(r), FormatLR(r.LR), r.Generated.Format("2006-01-02 15:04:05"), r.Addr))
	d.body(fmt.Sprintf("This epoch %d / %d cells. Recorded %d results. Learning rate %s.",
		r.EpochDone, r.Plan, r.Recorded, FormatLR(r.LR)))
	d.body("Score 0 usually means the cell finished before a Lucy pulse (Hard Acc never sampled). Score is live-fit: Throughput x Availability x Hard Acc. Acc keep % = Hard Acc / Acc-champ Hard Acc. SoftAcc is serve-confidence only. Lean = Acc keep >=95%, then smallest RAM.")
	if r.Subtitle != "" {
		d.body(r.Subtitle)
	}
	d.gap(3)
}

func (d *doc) coverOcean(o OceanReport) {
	d.h1("ocean  " + nz(o.Title, "linked tides"))
	d.muted("Lucy Score = Throughput x Availability x Acc / 10,000. Acc is learning. Availability is the live loop. LPD condenses that into Acc-champ RAM.")
	d.muted(ModeLegend())
	h := o.Holistic
	d.body(fmt.Sprintf("Generated %s. Tides up %d / %d. This-epoch cells %d / %d.",
		o.Generated.Format("2006-01-02 15:04:05"), h.TidesUp, h.TidesTotal, h.CellsDone, h.CellsTotal))
	d.body("Learning rate  " + oceanLRs(o.Tides))
	d.body(fmt.Sprintf("Holistic best train mode: %s     best dtype: %s     best arch: %s", nz(PrettyMode(h.BestMode), "-"), nz(h.BestDType, "-"), nz(PrettyArch(h.BestArch), "-")))
	d.body(fmt.Sprintf("Suggested default (most Lucy-axis wins): %s | %s | %s  (%d axes)",
		nz(PrettyMode(h.DefaultMode), "-"), nz(h.DefaultDType, "-"), nz(PrettyArch(h.DefaultArch), "-"), h.DefaultWins))
	if h.LPD.Champ.ID != "" {
		gold := "none yet"
		if len(h.LPD.Gold) > 0 {
			gold = fmt.Sprintf("%d  e.g. %s", len(h.LPD.Gold), PrettyCell(h.LPD.Gold[0].ID))
		}
		d.body(fmt.Sprintf("Acc champ %s (%.1f Acc, %.1f KiB). Gold cells (trifecta >=80%% at <=20%% Acc-champ RAM): %s",
			PrettyCell(h.LPD.AccChamp.ID), h.LPD.AccChamp.Acc, h.LPD.AccChamp.RAMKiB, gold))
	}
	d.gap(3)
}

func (d *doc) bests(r TideReport) {
	d.h2("Lucy champions (this tide)")
	type row struct {
		axis, id                      string
		score, soft, acc, thru, avail float64
	}
	var rows []row
	add := func(axis string, res *pulse.Result) {
		if res == nil {
			return
		}
		rows = append(rows, row{axis, res.Cell.ID, res.Snapshot.Score, res.Snapshot.SoftAcc, res.Snapshot.AvgAccuracy, res.Snapshot.Throughput, res.Snapshot.Availability})
	}
	add("hard acc", r.Best.Accuracy)
	add("throughput", r.Best.Throughput)
	add("availability", r.Best.Availability)
	add("lucy score", r.Best.Score)
	if r.BestMobile.Score != nil {
		add("mobile score (trap)", r.BestMobile.Score)
	}
	if r.BestLearn.To50 != nil {
		add("t->50%", r.BestLearn.To50)
	}
	if r.BestLearn.AccPerSec != nil {
		add("acc/sec", r.BestLearn.AccPerSec)
	}
	if r.BestLearnMobile.AccPerSec != nil {
		add("acc/sec/MiB", r.BestLearnMobile.AccPerSec)
	}
	d.table([]string{"Axis", "Cell", "Score", "Soft", "Acc", "Thru", "Avail"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{r.axis, PrettyCell(r.id), fmt.Sprintf("%.2f", r.score), fmt.Sprintf("%.1f", r.soft), fmt.Sprintf("%.1f", r.acc), fmt.Sprintf("%.1f", r.thru), fmt.Sprintf("%.1f", r.avail)}
	})
}

func (d *doc) leaderboard(title string, rows []pulse.Result) {
	d.leaderboardBy(title, rows, func(r pulse.Result) float64 { return r.Snapshot.Score })
}

func (d *doc) leaderboardBy(title string, rows []pulse.Result, key func(pulse.Result) float64) {
	cp := append([]pulse.Result(nil), rows...)
	sort.SliceStable(cp, func(i, j int) bool { return key(cp[i]) > key(cp[j]) })
	d.h2(title)
	d.table([]string{"#", "Cell", "Score", "Soft", "Acc", "Thru", "Avail"}, func(k int) []string {
		if k >= len(cp) || k >= 20 {
			return nil
		}
		r := cp[k]
		s := r.Snapshot
		return []string{
			fmt.Sprintf("%d", k+1), PrettyCell(r.Cell.ID),
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
		return []string{PrettyMode(m.Mode), itoa(m.Done), itoa(m.Running), itoa(m.Left), itoa(m.Total)}
	})
}

func (d *doc) votes(title string, vs []Vote) {
	d.h2(title)
	d.table([]string{"Key", "Layers", "Mean Score"}, func(k int) []string {
		if k >= len(vs) {
			return nil
		}
		v := vs[k]
		return []string{PrettyMode(PrettyArch(v.Key)), itoa(v.Count), fmt.Sprintf("%.2f", v.Mean)}
	})
}

func (d *doc) axisTable(title string, rows []AxisView) {
	if len(rows) == 0 {
		return
	}
	tides := map[string]bool{}
	for _, r := range rows {
		if t := strings.TrimSpace(r.Tide); t != "" {
			tides[t] = true
		}
	}
	showTide := len(tides) > 1
	d.h2(title)
	if showTide {
		d.table([]string{"Axis", "Tide", "Mode", "DType", "Arch", "Value"}, func(k int) []string {
			if k >= len(rows) {
				return nil
			}
			r := rows[k]
			return []string{clip(r.Name, 14), clip(r.Tide, 12), PrettyMode(r.Mode), r.DType, PrettyArch(r.Arch), fmt.Sprintf("%.2f", r.Value)}
		})
		return
	}
	d.table([]string{"Axis", "Mode", "DType", "Arch", "Value"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{clip(r.Name, 14), PrettyMode(r.Mode), r.DType, PrettyArch(r.Arch), fmt.Sprintf("%.2f", r.Value)}
	})
}

func oceanLRs(tides []TideReport) string {
	if len(tides) == 0 {
		return "-"
	}
	seen := map[string]bool{}
	var parts []string
	for _, t := range tides {
		line := nz(t.Task, t.ID) + "=" + FormatLR(t.LR)
		if seen[line] {
			continue
		}
		seen[line] = true
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "  ")
}

func (d *doc) layerTable(rows []LayerRow) {
	d.h2("Per-layer best (mode x dtype x arch)")
	d.table([]string{"Tide", "LR", "Mode", "DType", "Arch", "Score", "Soft", "Acc"}, func(k int) []string {
		if k >= len(rows) {
			return nil
		}
		r := rows[k]
		return []string{r.Tide, FormatLR(r.LR), PrettyMode(r.Mode), r.DType, PrettyArch(r.Arch), fmt.Sprintf("%.1f", r.Score), fmt.Sprintf("%.1f", r.SoftAcc), fmt.Sprintf("%.1f", r.Acc)}
	})
}

func (d *doc) topTable(rows []TopRow) {
	d.h2("Combined leaderboard")
	d.table([]string{"Tide", "Cell", "Score", "Soft", "Acc", "Avail"}, func(k int) []string {
		if k >= len(rows) || k >= 20 {
			return nil
		}
		r := rows[k]
		return []string{r.Tide, PrettyCell(r.CellID), fmt.Sprintf("%.1f", r.Score), fmt.Sprintf("%.1f", r.SoftAcc), fmt.Sprintf("%.1f", r.Acc), fmt.Sprintf("%.1f", r.Avail)}
	})
}

type kv struct {
	Label string
	Val   float64
}

func scoresOf(rows []pulse.Result) []kv {
	out := make([]kv, 0, len(rows))
	for _, r := range rows {
		out = append(out, kv{PrettyCell(r.Cell.ID), r.Snapshot.Score})
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
	for _, r := range rows {
		out = append(out, kv{PrettyMode(label(r)), r.Score})
	}
	return out
}

func voteBars(vs []Vote) []kv {
	out := make([]kv, 0, len(vs))
	for _, v := range vs {
		out = append(out, kv{PrettyMode(PrettyArch(v.Key)), v.Mean})
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
	d.pdf.SetFont("Helvetica", "", 6)
	for _, it := range items {
		if d.pdf.GetY()+barH > 278 {
			d.pdf.AddPage()
			d.h2(title)
			d.pdf.SetFont("Helvetica", "", 6)
		}
		y := d.pdf.GetY()
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(92, barH, d.fitText(it.Label, 92), "", 0, "L", false, 0, "")
		w := 0.0
		if max > 0 {
			w = (colW - 110) * it.Val / max
		}
		d.pdf.SetFillColor(61, 214, 198)
		d.pdf.Rect(left+94, y+0.7, w, barH-1.4, "F")
		d.pdf.SetXY(left+96+w, y)
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
	d.pdf.CellFormat(0, 9, latin(s), "", 1, "L", false, 0, "")
}

func (d *doc) h2(s string) {
	if d.pdf.GetY() > 265 {
		d.pdf.AddPage()
	}
	d.pdf.SetFont("Helvetica", "B", 11)
	d.pdf.SetTextColor(30, 90, 95)
	d.pdf.CellFormat(0, 7, latin(s), "", 1, "L", false, 0, "")
}

func (d *doc) muted(s string) {
	d.pdf.SetFont("Helvetica", "", 8)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.MultiCell(0, 4, latin(s), "", "L", false)
}

func (d *doc) body(s string) {
	d.pdf.SetFont("Helvetica", "", 9)
	d.pdf.SetTextColor(40, 50, 55)
	d.pdf.MultiCell(0, 4.5, latin(s), "", "L", false)
}

func (d *doc) gap(mm float64) { d.pdf.Ln(mm) }

func (d *doc) table(headers []string, row func(int) []string) {
	cols := len(headers)
	widths := tableColWidths(headers, 174)
	d.pdf.SetFont("Helvetica", "B", 6)
	d.pdf.SetFillColor(22, 40, 48)
	d.pdf.SetTextColor(230, 240, 242)
	for i, h := range headers {
		d.pdf.CellFormat(widths[i], 6, d.fitText(h, widths[i]), "1", 0, "L", true, 0, "")
	}
	d.pdf.Ln(-1)
	d.pdf.SetFont("Helvetica", "", 6)
	d.pdf.SetTextColor(30, 40, 45)
	for i := 0; ; i++ {
		r := row(i)
		if r == nil {
			break
		}
		if d.pdf.GetY() > 275 {
			d.pdf.AddPage()
			d.pdf.SetFont("Helvetica", "B", 6)
			d.pdf.SetFillColor(22, 40, 48)
			d.pdf.SetTextColor(230, 240, 242)
			for j, h := range headers {
				d.pdf.CellFormat(widths[j], 6, d.fitText(h, widths[j]), "1", 0, "L", true, 0, "")
			}
			d.pdf.Ln(-1)
			d.pdf.SetFont("Helvetica", "", 6)
			d.pdf.SetTextColor(30, 40, 45)
		}
		fill := i%2 == 1
		if fill {
			d.pdf.SetFillColor(236, 242, 244)
		}
		for j := 0; j < cols && j < len(r); j++ {
			d.pdf.CellFormat(widths[j], 5, d.fitText(r[j], widths[j]), "1", 0, "L", fill, 0, "")
		}
		d.pdf.Ln(-1)
	}
	d.gap(3)
}

func tableColWidths(headers []string, left float64) []float64 {
	n := len(headers)
	w := make([]float64, n)
	if n == 0 {
		return w
	}
	wt := make([]float64, n)
	sum := 0.0
	for i, h := range headers {
		wt[i] = tableColWeight(h)
		sum += wt[i]
	}
	if sum <= 0 {
		sum = float64(n)
		for i := range wt {
			wt[i] = 1
		}
	}
	for i := range w {
		w[i] = left * wt[i] / sum
	}
	return w
}

func tableColWeight(h string) float64 {
	low := strings.ToLower(strings.TrimSpace(h))
	if strings.Contains(low, "cell") {
		return 4.5
	}
	switch low {
	case "mode", "winner mode", "winner dtype", "dtype", "tide", "key", "axis", "arch", "format":
		return 3.2
	}
	return 1
}

func (d *doc) fitText(s string, mm float64) string {
	s = latin(s)
	max := mm - 1.4
	if max < 3 {
		max = 3
	}
	if d.pdf.GetStringWidth(s) <= max {
		return s
	}
	const ell = "~"
	for len(s) > 1 {
		s = s[:len(s)-1]
		if d.pdf.GetStringWidth(s+ell) <= max {
			return s + ell
		}
	}
	return ell
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func clip(s string, n int) string {
	s = latin(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
}

// latin maps punctuation Helvetica cannot encode (em dash, middle dot, times)
// so PDFs do not print mojibake like "â€”" or "Ã—".
func latin(s string) string {
	if s == "" {
		return s
	}
	r := strings.NewReplacer(
		"\u2014", "-",
		"\u2013", "-",
		"\u2212", "-",
		"\u00b7", " - ",
		"\u2022", "*",
		"\u00d7", "x",
		"\u0394", "D",
		"\u2206", "D",
		"\u2192", "->",
		"\u2265", ">=",
		"\u2264", "<=",
		"\u2018", "'",
		"\u2019", "'",
		"\u201c", "\"",
		"\u201d", "\"",
		"\u2026", "...",
		"\u00a0", " ",
	)
	return r.Replace(s)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
