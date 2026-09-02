package river

import (
	"fmt"
	"sort"
	"strings"

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

// RenderStorePDF appends the full River site onto an existing PDF document.
func RenderStorePDF(pdf *gofpdf.Fpdf, st *Store, title string) {
	f := st.Snapshot()
	RenderComparePDF(
		pdf,
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
	nearRowTable(pdf, "Acc keep ranking", p.Rows, 0)
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
	nearRowTable(pdf, "LPD ranking", p.Rows, 0)
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
	nearRowTable(pdf, "Throughput table", p.Rows, 0)
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
		for i := 0; i < len(g.Cells); i++ {
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
		arches := uniqueArchesBestMode(g.Rows)
		for _, arch := range arches {
			var archRows []BestModeByDTypeRow
			for _, r := range g.Rows {
				if r.Arch == arch {
					archRows = append(archRows, r)
				}
			}
			if len(archRows) == 0 {
				continue
			}
			sort.Slice(archRows, func(i, j int) bool { return archRows[i].Acc > archRows[j].Acc })
			pdf.AddPage()
			section(pdf, fmt.Sprintf("Best train mode per dtype @ %s · %s", g.LRLabel, arch))
			items := make([]barItem, 0, len(archRows))
			for _, r := range archRows {
				items = append(items, barItem{
					Label: fmt.Sprintf("%s · %s", r.DType, r.Mode),
					Val:   r.Acc,
					Note:  fmt.Sprintf("%.1f Acc · thru %.0f · acc/s %.2f", r.Acc, r.Throughput, r.AccPerSec),
				})
			}
			subhead(pdf, "Best Acc by dtype (bar chart)")
			hbars(pdf, items, "Acc %", 62, 207, 142, true)
			subhead(pdf, "Detail table")
			headers := []string{"dtype", "mode", "Acc", "thru", "acc/s", "id"}
			widths := []float64{24, 40, 16, 18, 18, 58}
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 8)
			for _, r := range archRows {
				if pdf.GetY() > 270 {
					pdf.AddPage()
					tableHeader(pdf, headers, widths)
					pdf.SetFont("Helvetica", "", 8)
				}
				tableRow(pdf, []string{
					clip(r.DType, 12), clip(r.Mode, 22),
					fmt.Sprintf("%.1f", r.Acc), fmt.Sprintf("%.0f", r.Throughput), fmt.Sprintf("%.2f", r.AccPerSec),
					clip(r.ID, 40),
				}, widths)
			}
		}
	}
}

func modeDtypeRangeChartsPDF(pdf *gofpdf.Fpdf, grids []ModeDtypeGrid) {
	for _, g := range grids {
		if len(g.Cells) == 0 {
			continue
		}
		arches := g.Arches
		if len(arches) == 0 {
			arches = uniqueArchesModeDtype(g.Cells)
		}
		for _, arch := range arches {
			stats := modeDtypeStats(g.Cells, arch)
			if len(stats) == 0 {
				continue
			}
			sort.Slice(stats, func(i, j int) bool { return stats[i].mean < stats[j].mean })
			pdf.AddPage()
			section(pdf, fmt.Sprintf("Mode x dtype range (mean Acc) @ %s · %s", g.LRLabel, arch))
			items := make([]barItem, 0, len(stats))
			for _, s := range stats {
				items = append(items, barItem{
					Label: s.mode,
					Val:   s.max,
					Note:  fmt.Sprintf("%.1f-%.1f mean %.1f (%d dtypes)", s.min, s.max, s.mean, s.n),
				})
			}
			subhead(pdf, "Worst to best train mode — bar = max mean Acc, note shows min-max spread")
			hbars(pdf, items, "mean Acc %", 91, 159, 212, true)
		}
	}
}

type modeDtypeStat struct {
	mode       string
	min, max   float64
	mean       float64
	n          int
}

func modeDtypeStats(cells []ModeDtypeCell, arch string) []modeDtypeStat {
	byMode := map[string][]float64{}
	for _, c := range cells {
		if c.Arch != arch {
			continue
		}
		byMode[c.Mode] = append(byMode[c.Mode], c.MeanAcc)
	}
	out := make([]modeDtypeStat, 0, len(byMode))
	for mode, vals := range byMode {
		if len(vals) == 0 {
			continue
		}
		min, max := vals[0], vals[0]
		sum := 0.0
		for _, v := range vals {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
			sum += v
		}
		out = append(out, modeDtypeStat{
			mode: mode, min: min, max: max, mean: sum / float64(len(vals)), n: len(vals),
		})
	}
	return out
}

func uniqueArchesModeDtype(cells []ModeDtypeCell) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cells {
		if c.Arch == "" || seen[c.Arch] {
			continue
		}
		seen[c.Arch] = true
		out = append(out, c.Arch)
	}
	sort.Strings(out)
	return out
}

func uniqueArchesBestMode(rows []BestModeByDTypeRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		if r.Arch == "" || seen[r.Arch] {
			continue
		}
		seen[r.Arch] = true
		out = append(out, r.Arch)
	}
	sort.Strings(out)
	return out
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
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	pdf.Ln(2)
	subhead(pdf, heading+" (all "+fmt.Sprintf("%d", limit)+" rows · landscape)")
	addLandscapePage(pdf)
	headers := []string{
		"#", "credit", "pat", "cams", "keep%", "Acc", "Soft", "Thru", "Avail", "Score",
		"LPD", "Q", "KiB", "shrink", "RAM%", "train s", "Acc/s", "dense",
		"ThruK", "AvailK", "pillars", "band", "mode", "dtype", "arch", "lr", "id",
	}
	widths := []float64{
		6, 10, 8, 16, 9, 9, 9, 9, 9, 9,
		8, 7, 8, 8, 9, 9, 9, 8,
		9, 9, 8, 10, 14, 10, 12, 8, 28,
	}
	tableHeader(pdf, headers, widths)
	pdf.SetFont("Helvetica", "", 5.5)
	yMax := landscapeYMax(pdf)
	for i := 0; i < limit; i++ {
		r := rows[i]
		if pdf.GetY() > yMax {
			addLandscapePage(pdf)
			tableHeader(pdf, headers, widths)
			pdf.SetFont("Helvetica", "", 5.5)
		}
		cams := r.Mode
		if len(r.BranchModes) > 0 {
			cams = strings.Join(r.BranchModes, "+")
		}
		tableRow(pdf, []string{
			fmt.Sprintf("%d", r.Rank),
			creditLabel(r.Credit),
			nz(r.MixPattern, "-"),
			clip(cams, 14),
			fmt.Sprintf("%.1f", r.PctOfBest),
			fmt.Sprintf("%.1f", r.Acc),
			fmtSoft(r.SoftAcc),
			fmt.Sprintf("%.0f", r.Throughput),
			fmtSoft(r.Availability),
			fmtSoft(r.Score),
			fmtSoft(r.LPD),
			fmtQ(r.Q),
			fmt.Sprintf("%.1f", r.RAMKiB),
			fmtSoft(r.Shrink),
			fmtPct(r.RAMFrac),
			fmtSoft(r.DurationSec),
			fmtSoft(r.AccPerSec),
			fmtSoft(r.DenseScore),
			fmtPct(r.RelThru),
			fmtPct(r.RelAvail),
			fmt.Sprintf("%d", r.Pillars),
			nz(r.Band, "-"),
			clip(r.Mode, 12),
			clip(r.DType, 8),
			clip(r.Arch, 10),
			nz(r.LRLabel, "-"),
			clip(r.ID, 24),
		}, widths)
	}
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 210, Ht: 297})
}

func addLandscapePage(pdf *gofpdf.Fpdf) {
	pdf.AddPageFormat("L", gofpdf.SizeType{Wd: 297, Ht: 210})
}

func landscapeYMax(_ *gofpdf.Fpdf) float64 {
	return 188.0
}

func creditLabel(c string) string {
	switch c {
	case "non_bp":
		return "non-BP"
	case "chain":
		return "chain"
	case "mix":
		return "mix"
	default:
		return "BP"
	}
}

func fmtSoft(v float64) string {
	if v == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", v)
}

func fmtQ(v float64) string {
	if v == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func fmtPct(v float64) string {
	if v == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", v*100)
}
