package river

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
)

type barItem struct {
	Label string
	Val   float64
	Note  string // optional trailing note (e.g. Acc%)
}

// ComparePDF builds a multi-page PDF of the full River site (compare, near, LPD, thru).
func ComparePDF(compare ComparePayload, near NearPayload, lpd LPDSearchPayload, thru ThruPayload, prog PlanProgress, title string) ([]byte, error) {
	pdf := newRiverPDF(title)
	writeComparePDF(pdf, compare, near, lpd, thru, prog, title, true)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderComparePDF appends River pages onto an existing gofpdf document (e.g. Tide combined report).
func RenderComparePDF(pdf *gofpdf.Fpdf, compare ComparePayload, near NearPayload, lpd LPDSearchPayload, thru ThruPayload, prog PlanProgress, title string) {
	writeComparePDF(pdf, compare, near, lpd, thru, prog, title, false)
}

func newRiverPDF(title string) *gofpdf.Fpdf {
	if title == "" {
		title = "River compare"
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetTitle(title, false)
	pdf.SetAuthor("openfluke/river", false)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(120, 130, 140)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s  ·  page %d", title, pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	return pdf
}

func writeComparePDF(pdf *gofpdf.Fpdf, compare ComparePayload, near NearPayload, lpd LPDSearchPayload, thru ThruPayload, prog PlanProgress, title string, withCover bool) {
	if title == "" {
		title = "River compare"
	}
	if withCover {
		coverCompare(pdf, compare, prog, title)
	}

	leanPDF(pdf, "Lean champs — >=95% best Acc · smallest KiB · fastest", compare.LeanBest, compare.LeanBars)
	leanPDF(pdf, "Lean champs by dtype — one winner each · KiB small->big", compare.LeanBest, compare.LeanByDtype)

	aggPDF(pdf, "By train mode (mean Acc)", compare.ByMode, 62, 207, 142)
	aggPDF(pdf, "By dtype (mean Acc)", compare.ByDType, 91, 159, 212)
	aggPDF(pdf, "By arch (mean Acc)", compare.ByArch, 199, 179, 90)

	rowsChartPDF(pdf, "Top Acc", compare.TopAcc, 20, func(r Row) float64 { return r.Acc }, "Acc %", 62, 207, 142)
	rowsChartPDF(pdf, "Top throughput", compare.TopThru, 15, func(r Row) float64 { return r.Throughput }, "thru", 91, 159, 212)
	rowsChartPDF(pdf, "Top Acc/sec", compare.TopSpeed, 15, func(r Row) float64 { return r.AccPerSec }, "acc/s", 199, 179, 90)
	denseChartPDF(pdf, compare.TopDense)
	modeDtypeRangeChartsPDF(pdf, compare.ModeDtypeGrids)
	modeDtypeGridsPDF(pdf, compare.ModeDtypeGrids)
	bestModeByDtypePDF(pdf, compare.BestModeByDType)
	overlapPDF(pdf, compare.Overlap)
	progressArchPDF(pdf, prog)

	nearPDF(pdf, near)
	lpdSearchPDF(pdf, lpd)
	thruPDF(pdf, thru)
}

func coverCompare(pdf *gofpdf.Fpdf, p ComparePayload, prog PlanProgress, title string) {
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(20, 28, 36)
	pdf.Cell(0, 10, title)
	pdf.Ln(12)
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(60, 70, 80)
	lines := []string{
		fmt.Sprintf("Machine: %s", nz(p.Machine, "?")),
		fmt.Sprintf("Matrix: %s   LRs: %s   train-n: %d", nz(p.Matrix, "?"), strings.Join(p.LRLabels, ", "), p.TrainN),
		fmt.Sprintf("Rows: %d   generated: %s", len(p.Rows), p.Generated.UTC().Format(time.RFC3339)),
		fmt.Sprintf("Seed: 0x%X", p.SampleSeed),
	}
	if prog.Plan > 0 {
		lines = append(lines, fmt.Sprintf("Plan progress: %d/%d (%.1f%%) · left %d · ETA %s",
			prog.Done, prog.Plan, prog.Pct, prog.Left, nz(prog.ETAHuman, "-")))
		if prog.Window != "" {
			lines = append(lines, fmt.Sprintf("Pace: %.0f cells/hr (%s)", prog.RatePerHr, prog.Window))
		}
	}
	if p.LeanBest != nil && p.LeanBest.BestAcc > 0 {
		lines = append(lines,
			fmt.Sprintf("Champion Acc: %.1f%%  (threshold %.1f%% = 95%%)", p.LeanBest.BestAcc, p.LeanBest.Threshold),
		)
		if p.LeanBest.WinnerID != "" {
			lines = append(lines, fmt.Sprintf("Lean winner: %.1f KiB · %.1fs · Acc %.1f%%",
				p.LeanBest.WinnerKiB, p.LeanBest.WinnerSec, p.LeanBest.WinnerAcc))
			lines = append(lines, "  "+clip(p.LeanBest.WinnerID, 90))
		}
	}
	for _, ln := range lines {
		pdf.MultiCell(0, 6, latin(ln), "", "", false)
	}
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(100, 110, 120)
	pdf.MultiCell(0, 5, "Lean = Acc >= 95% of global best, then smallest weight KiB, then fastest train duration.", "", "", false)

	// Cover spark: plan progress bar
	if prog.Plan > 0 {
		pdf.Ln(6)
		section(pdf, "Plan completion")
		hbars(pdf, []barItem{{
			Label: fmt.Sprintf("%d / %d done", prog.Done, prog.Plan),
			Val:   prog.Pct,
			Note:  fmt.Sprintf("%.1f%%", prog.Pct),
		}}, "%", 62, 207, 142, false)
	}
}

func leanPDF(pdf *gofpdf.Fpdf, heading string, sum *LeanSummary, bars []LeanBar) {
	pdf.AddPage()
	section(pdf, heading)
	if sum != nil && sum.BestAcc > 0 {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(70, 80, 90)
		pdf.MultiCell(0, 5, fmt.Sprintf("Best Acc %.1f%% · threshold %.1f%% · eligible %d",
			sum.BestAcc, sum.Threshold, sum.Eligible), "", "", false)
		pdf.Ln(2)
	}
	if len(bars) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.Cell(0, 6, "No eligible cells yet.")
		return
	}

	kib := make([]barItem, 0, len(bars))
	secs := make([]barItem, 0, len(bars))
	for _, b := range bars {
		lab := fmt.Sprintf("#%d %s", b.Rank, b.Label)
		kib = append(kib, barItem{Label: lab, Val: b.WeightKiB, Note: fmt.Sprintf("%.1f KiB", b.WeightKiB)})
		secs = append(secs, barItem{Label: lab, Val: b.DurationSec, Note: fmt.Sprintf("%.1fs · Acc %.1f", b.DurationSec, b.Acc)})
	}
	subhead(pdf, "Weight KiB (smaller better)")
	hbars(pdf, kib, "KiB", 62, 207, 142, true)
	subhead(pdf, "Train duration sec (faster better)")
	hbars(pdf, secs, "sec", 230, 179, 90, true)

	pdf.Ln(2)
	subhead(pdf, "Detail table")
	headers := []string{"#", "KiB", "train s", "Acc %", "%best", "mode", "dtype", "arch"}
	widths := []float64{8, 14, 16, 14, 14, 36, 24, 40}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 8)
	for _, b := range bars {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 8)
		}
		tableRow(pdf, []string{
			fmt.Sprintf("%d", b.Rank),
			fmt.Sprintf("%.1f", b.WeightKiB),
			fmt.Sprintf("%.1f", b.DurationSec),
			fmt.Sprintf("%.1f", b.Acc),
			fmt.Sprintf("%.1f", b.PctOfBest),
			clip(b.Mode, 22),
			clip(b.DType, 14),
			clip(b.Arch, 24),
		}, widths)
	}
}

func aggPDF(pdf *gofpdf.Fpdf, heading string, bars []AggBar, r, g, b int) {
	if len(bars) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, heading)
	cp := append([]AggBar(nil), bars...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].MeanAcc > cp[j].MeanAcc })
	if len(cp) > 30 {
		cp = cp[:30]
	}
	items := make([]barItem, 0, len(cp))
	for _, a := range cp {
		items = append(items, barItem{
			Label: a.Key,
			Val:   a.MeanAcc,
			Note:  fmt.Sprintf("%.1f (best %.1f, n=%d)", a.MeanAcc, a.BestAcc, a.N),
		})
	}
	hbars(pdf, items, "mean Acc %", r, g, b, true)

	pdf.Ln(2)
	subhead(pdf, "Detail table")
	headers := []string{"key", "n", "mean Acc", "best Acc", "mean thru"}
	widths := []float64{70, 14, 24, 24, 28}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 8)
	for _, a := range cp {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 8)
		}
		tableRow(pdf, []string{
			clip(a.Key, 40),
			fmt.Sprintf("%d", a.N),
			fmt.Sprintf("%.1f", a.MeanAcc),
			fmt.Sprintf("%.1f", a.BestAcc),
			fmt.Sprintf("%.0f", a.MeanThru),
		}, widths)
	}
}

func rowsChartPDF(pdf *gofpdf.Fpdf, heading string, rows []Row, n int, val func(Row) float64, unit string, cr, cg, cb int) {
	if len(rows) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, heading)
	if len(rows) > n {
		rows = rows[:n]
	}
	items := make([]barItem, 0, len(rows))
	for i, r := range rows {
		lab := fmt.Sprintf("#%d %s · %s · %s", i+1, r.Mode, r.DType, r.Arch)
		items = append(items, barItem{
			Label: lab,
			Val:   val(r),
			Note:  fmt.Sprintf("%.2f %s", val(r), unit),
		})
	}
	hbars(pdf, items, unit, cr, cg, cb, true)

	pdf.Ln(2)
	subhead(pdf, "Detail table")
	headers := []string{"Acc", "thru", "acc/s", "KiB", "mode", "dtype", "arch"}
	widths := []float64{14, 16, 16, 14, 36, 24, 40}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 8)
	for _, r := range rows {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 8)
		}
		tableRow(pdf, []string{
			fmt.Sprintf("%.1f", r.Acc),
			fmt.Sprintf("%.0f", r.Throughput),
			fmt.Sprintf("%.2f", r.AccPerSec),
			fmt.Sprintf("%.1f", r.WeightKiB),
			clip(r.Mode, 22),
			clip(r.DType, 14),
			clip(r.Arch, 24),
		}, widths)
	}
}

func denseChartPDF(pdf *gofpdf.Fpdf, rows []Row) {
	if len(rows) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, "Top Acc/sec/MiB x Acc^2 (Acc>=40%)")
	if len(rows) > 20 {
		rows = rows[:20]
	}
	items := make([]barItem, 0, len(rows))
	for i, r := range rows {
		lab := fmt.Sprintf("#%d %s · %s · %s", i+1, r.Mode, r.DType, r.Arch)
		items = append(items, barItem{
			Label: lab,
			Val:   r.DenseScore,
			Note:  fmt.Sprintf("%.1f · Acc %.1f · %.1fKiB", r.DenseScore, r.Acc, r.WeightKiB),
		})
	}
	hbars(pdf, items, "dense score", 199, 125, 255, true)

	pdf.Ln(2)
	subhead(pdf, "Detail table")
	headers := []string{"score", "Acc", "KiB", "acc/s", "mode", "dtype", "arch"}
	widths := []float64{16, 14, 14, 16, 36, 24, 40}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 8)
	for _, r := range rows {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 8)
		}
		tableRow(pdf, []string{
			fmt.Sprintf("%.1f", r.DenseScore),
			fmt.Sprintf("%.1f", r.Acc),
			fmt.Sprintf("%.1f", r.WeightKiB),
			fmt.Sprintf("%.2f", r.AccPerSec),
			clip(r.Mode, 22),
			clip(r.DType, 14),
			clip(r.Arch, 24),
		}, widths)
	}
}

func progressArchPDF(pdf *gofpdf.Fpdf, prog PlanProgress) {
	if prog.Plan == 0 || len(prog.ByArch) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, "Plan progress by arch")
	keys := make([]string, 0, len(prog.ByArch))
	for k := range prog.ByArch {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := make([]barItem, 0, len(keys))
	for _, k := range keys {
		a := prog.ByArch[k]
		pct := 0.0
		if a.Plan > 0 {
			pct = 100 * float64(a.Done) / float64(a.Plan)
		}
		items = append(items, barItem{
			Label: k,
			Val:   pct,
			Note:  fmt.Sprintf("%d/%d (%.0f%%) left %d", a.Done, a.Plan, pct, a.Left),
		})
	}
	hbars(pdf, items, "% done", 91, 159, 212, true)
}

// hbars draws horizontal bar charts. invertMax=false scales so larger Val = longer bar.
// For "smaller better" metrics (KiB, sec) pass invert=true so shorter bars win visually
// by showing relative size still (larger value = longer bar) — we keep normal scale
// (bigger value = longer bar) and label the axis; user reads the number.
func hbars(pdf *gofpdf.Fpdf, items []barItem, unit string, fr, fg, fb int, paginate bool) {
	if len(items) == 0 {
		return
	}
	max := 1.0
	for _, it := range items {
		if it.Val > max {
			max = it.Val
		}
	}
	left := 14.0
	labelW := 78.0
	barMax := 70.0
	barH := 4.4
	pdf.SetFont("Helvetica", "I", 7)
	pdf.SetTextColor(120, 130, 140)
	pdf.Cell(0, 4, "unit: "+unit+"  (bar length = value)")
	pdf.Ln(5)
	pdf.SetFont("Helvetica", "", 6.5)
	for _, it := range items {
		if paginate && pdf.GetY()+barH > 278 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 6.5)
		}
		y := pdf.GetY()
		pdf.SetTextColor(40, 50, 55)
		pdf.SetXY(left, y)
		pdf.CellFormat(labelW, barH, fitText(pdf, latin(it.Label), labelW), "", 0, "L", false, 0, "")
		w := 0.0
		if max > 0 {
			w = barMax * it.Val / max
		}
		if w < 0.4 && it.Val > 0 {
			w = 0.4
		}
		pdf.SetFillColor(fr, fg, fb)
		pdf.Rect(left+labelW+2, y+0.7, w, barH-1.4, "F")
		pdf.SetXY(left+labelW+4+w, y)
		note := it.Note
		if note == "" {
			note = fmt.Sprintf("%.2f", it.Val)
		}
		pdf.CellFormat(50, barH, latin(note), "", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
}

func fitText(pdf *gofpdf.Fpdf, s string, width float64) string {
	if pdf.GetStringWidth(s) <= width {
		return s
	}
	for n := len(s); n > 1; n-- {
		t := s[:n-1] + "..."
		if pdf.GetStringWidth(t) <= width {
			return t
		}
	}
	return s
}

func section(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(20, 28, 36)
	pdf.Cell(0, 8, latin(title))
	pdf.Ln(10)
}

func subhead(pdf *gofpdf.Fpdf, title string) {
	if pdf.GetY() > 265 {
		pdf.AddPage()
	}
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(40, 55, 65)
	pdf.Cell(0, 6, latin(title))
	pdf.Ln(7)
}

func tableHeader(pdf *gofpdf.Fpdf, headers []string, widths []float64) {
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(230, 236, 242)
	pdf.SetTextColor(50, 60, 70)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 6, latin(h), "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
}

func tableRow(pdf *gofpdf.Fpdf, cols []string, widths []float64) {
	pdf.SetTextColor(30, 40, 50)
	for i, c := range cols {
		pdf.CellFormat(widths[i], 5.5, latin(c), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

func latin(s string) string {
	repl := strings.NewReplacer(
		"×", "x", "·", "-", "—", "-", "–", "-", "≥", ">=", "≤", "<=",
		"→", "->", "←", "<-", "“", "\"", "”", "\"", "’", "'",
	)
	return repl.Replace(s)
}

func clip(s string, n int) string {
	s = latin(s)
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "..."
}

func nz(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
