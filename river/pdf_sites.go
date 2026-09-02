package river

import (
	"fmt"

	"github.com/phpdave11/gofpdf"
)

// StorePDF builds the complete River PDF (compare + near + LPD + throughput).
func StorePDF(st *Store, title string) ([]byte, error) {
	f := st.Snapshot()
	return ComparePDF(
		buildCompare(f),
		buildNear(f, 0),
		buildLPDSearch(f),
		buildThru(f),
		st.Progress(),
		title,
	)
}

func nearPDF(pdf *gofpdf.Fpdf, p NearPayload) {
	if len(p.Rows) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, fmt.Sprintf("Acc keep band — %.0f%% to 100%% of champ", p.MinKeep*100))
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(70, 80, 90)
	pdf.MultiCell(0, 5, fmt.Sprintf("Champ %.1f%% (%s). Threshold %.1f%%. %d / %d cells in band.",
		p.BestAcc, clip(p.BestID, 40), p.Threshold, p.NBand, p.NTotal), "", "", false)
	nearHighlights(pdf, p.BestNonBP, p.BestChain, p.ChainNote)
	nearRowTable(pdf, "Acc keep ranking", p.Rows, 40)
}

func lpdSearchPDF(pdf *gofpdf.Fpdf, p LPDSearchPayload) {
	if len(p.Rows) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, "LPD search — Q x shrink vs Acc-champ RAM")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(70, 80, 90)
	pdf.MultiCell(0, 5, fmt.Sprintf("Acc champ %.1f%% (%s). keep>=%d trap=%d total=%d.",
		p.BestAcc, clip(p.BestID, 40), p.NKeep, p.NTrap, p.NTotal), "", "", false)
	lpdHighlight(pdf, "Acc champ", p.AccChamp)
	lpdHighlight(pdf, "Score champ", p.ScoreChamp)
	lpdHighlight(pdf, "Live champ", p.LiveChamp)
	lpdHighlight(pdf, "Lean champ", p.LeanChamp)
	lpdHighlight(pdf, "Gold-std", p.GoldStd)
	lpdHighlight(pdf, "Top LPD", p.TopLPD)
	nearHighlights(pdf, p.BestNonBP, p.BestChain, p.ChainNote)
	nearRowTable(pdf, "LPD ranking", p.Rows, 50)
}

func thruPDF(pdf *gofpdf.Fpdf, p ThruPayload) {
	if len(p.Rows) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, "Throughput ranking")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(70, 80, 90)
	pdf.MultiCell(0, 5, fmt.Sprintf("Best %.0f/s (%s). %d cells.", p.BestThru, clip(p.BestID, 40), p.NRows), "", "", false)
	nearHighlights(pdf, p.BestNonBP, p.BestChain, p.ChainNote)
	items := make([]barItem, 0, 20)
	for i, r := range p.Rows {
		if i >= 20 {
			break
		}
		items = append(items, barItem{
			Label: fmt.Sprintf("#%d %s", r.Rank, clip(r.Mode, 18)),
			Val:   r.Throughput,
			Note:  fmt.Sprintf("%.0f/s · Acc %.1f", r.Throughput, r.Acc),
		})
	}
	hbars(pdf, items, "samples/s", 91, 159, 212, true)
	nearRowTable(pdf, "Throughput table", p.Rows, 40)
}

func modeDtypeGridsPDF(pdf *gofpdf.Fpdf, grids []ModeDtypeGrid) {
	for _, g := range grids {
		if len(g.Cells) == 0 {
			continue
		}
		pdf.AddPage()
		section(pdf, "Mode x dtype x arch @ "+g.LRLabel)
		headers := []string{"mode", "dtype", "arch", "n", "mean Acc", "best Acc", "mean thru"}
		widths := []float64{36, 22, 28, 10, 22, 22, 22}
		tableHeader(pdf, headers, widths)
		pdf.SetFont("Helvetica", "", 7)
		limit := len(g.Cells)
		if limit > 60 {
			limit = 60
		}
		for i := 0; i < limit; i++ {
			c := g.Cells[i]
			if pdf.GetY() > 270 {
				pdf.AddPage()
				tableHeader(pdf, headers, widths)
				pdf.SetFont("Helvetica", "", 7)
			}
			tableRow(pdf, []string{
				clip(c.Mode, 20), clip(c.DType, 12), clip(c.Arch, 16),
				fmt.Sprintf("%d", c.N), fmt.Sprintf("%.1f", c.MeanAcc), fmt.Sprintf("%.1f", c.BestAcc),
				fmt.Sprintf("%.0f", c.MeanThru),
			}, widths)
		}
	}
}

func bestModeByDtypePDF(pdf *gofpdf.Fpdf, grids []BestModeByDTypeGrid) {
	for _, g := range grids {
		if len(g.Rows) == 0 {
			continue
		}
		pdf.AddPage()
		section(pdf, "Best train mode per dtype @ "+g.LRLabel)
		headers := []string{"dtype", "arch", "mode", "Acc", "thru", "acc/s"}
		widths := []float64{24, 28, 40, 16, 18, 18}
		tableHeader(pdf, headers, widths)
		pdf.SetFont("Helvetica", "", 8)
		for _, r := range g.Rows {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				tableHeader(pdf, headers, widths)
				pdf.SetFont("Helvetica", "", 8)
			}
			tableRow(pdf, []string{
				clip(r.DType, 12), clip(r.Arch, 16), clip(r.Mode, 22),
				fmt.Sprintf("%.1f", r.Acc), fmt.Sprintf("%.0f", r.Throughput), fmt.Sprintf("%.2f", r.AccPerSec),
			}, widths)
		}
	}
}

func overlapPDF(pdf *gofpdf.Fpdf, series []OverlapSeries) {
	if len(series) == 0 {
		return
	}
	pdf.AddPage()
	section(pdf, "Best Acc per mode x LR (overlap)")
	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		subhead(pdf, clip(s.Mode, 48))
		items := make([]barItem, 0, len(s.Points))
		for _, p := range s.Points {
			items = append(items, barItem{
				Label: p.LRLabel,
				Val:   p.Acc,
				Note:  fmt.Sprintf("%.1f%% · %s", p.Acc, clip(p.ID, 24)),
			})
		}
		hbars(pdf, items, "Acc %", 62, 207, 142, true)
	}
}

func nearHighlights(pdf *gofpdf.Fpdf, nonBP, chain *NearHighlight, chainNote string) {
	if nonBP != nil {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.Cell(0, 5, "Best non-BP: "+highlightLine(nonBP))
		pdf.Ln(6)
	}
	if chain != nil {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.Cell(0, 5, "Best chain: "+highlightLine(chain))
		pdf.Ln(6)
	}
	if chainNote != "" {
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(100, 110, 120)
		pdf.MultiCell(0, 4, latin(chainNote), "", "", false)
		pdf.Ln(2)
	}
}

func highlightLine(h *NearHighlight) string {
	if h == nil {
		return "—"
	}
	return fmt.Sprintf("%s · %s · Acc %.1f%% (%.0f%% keep) · %s",
		clip(h.Mode, 20), clip(h.DType, 10), h.Acc, h.PctOfBest, clip(h.ID, 36))
}

func lpdHighlight(pdf *gofpdf.Fpdf, label string, h *NearHighlight) {
	if h == nil || h.ID == "" {
		return
	}
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(50, 60, 70)
	pdf.Cell(0, 4, label+": "+highlightLine(h))
	pdf.Ln(5)
}

func nearRowTable(pdf *gofpdf.Fpdf, heading string, rows []NearRow, limit int) {
	if len(rows) == 0 {
		return
	}
	pdf.Ln(2)
	subhead(pdf, heading)
	headers := []string{"#", "Acc", "keep%", "LPD", "Q%", "KiB", "thru", "mode", "dtype", "arch"}
	widths := []float64{8, 12, 12, 12, 10, 12, 14, 30, 18, 24}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 7)
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		r := rows[i]
		if pdf.GetY() > 270 {
			pdf.AddPage()
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 7)
		}
		tableRow(pdf, []string{
			fmt.Sprintf("%d", r.Rank),
			fmt.Sprintf("%.1f", r.Acc),
			fmt.Sprintf("%.0f", r.PctOfBest),
			fmt.Sprintf("%.2f", r.LPD),
			fmt.Sprintf("%.0f", r.Q*100),
			fmt.Sprintf("%.1f", r.RAMKiB),
			fmt.Sprintf("%.0f", r.Throughput),
			clip(r.Mode, 18),
			clip(r.DType, 10),
			clip(r.Arch, 14),
		}, widths)
	}
}
