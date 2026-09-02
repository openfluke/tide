package river

import (
	"github.com/phpdave11/gofpdf"
)

var (
	a4Portrait  = gofpdf.SizeType{Wd: 210, Ht: 297}
	a4Landscape = gofpdf.SizeType{Wd: 297, Ht: 210}
)

const portraitTableMaxW = 185.0

func sumWidths(widths []float64) float64 {
	s := 0.0
	for _, w := range widths {
		s += w
	}
	return s
}

func scaleWidths(widths []float64, maxTotal float64) []float64 {
	s := sumWidths(widths)
	if s <= maxTotal || s <= 0 {
		return widths
	}
	out := make([]float64, len(widths))
	for i, w := range widths {
		out[i] = w * maxTotal / s
	}
	return out
}

func isLandscapePage(pdf *gofpdf.Fpdf) bool {
	w, h := pdf.GetPageSize()
	return w > h
}

func usablePageWidth(pdf *gofpdf.Fpdf) float64 {
	w, _ := pdf.GetPageSize()
	l, _, r, _ := pdf.GetMargins()
	return w - l - r
}

func pageBreakY(pdf *gofpdf.Fpdf) float64 {
	_, h := pdf.GetPageSize()
	_, _, _, b := pdf.GetMargins()
	return h - b - 14 // footer band
}

func addLandscapePage(pdf *gofpdf.Fpdf) {
	pdf.AddPageFormat("L", a4Landscape)
}

func addPortraitPage(pdf *gofpdf.Fpdf) {
	pdf.AddPageFormat("P", a4Portrait)
}

func tableNeedsLandscape(headers []string, widths []float64, force bool) bool {
	if force {
		return true
	}
	return len(headers) >= 10 || sumWidths(widths) > portraitTableMaxW
}

// paginatedTable renders a full table, rotating to landscape when columns are wide.
// Returns to portrait afterward if this call opened landscape.
func paginatedTable(pdf *gofpdf.Fpdf, headers []string, widths []float64, fontSize float64, forceLandscape bool, rowAt func(i int) ([]string, bool)) {
	if len(headers) == 0 {
		return
	}
	landscape := tableNeedsLandscape(headers, widths, forceLandscape)
	wasPortrait := !isLandscapePage(pdf)
	if landscape {
		addLandscapePage(pdf)
	}
	widths = scaleWidths(widths, usablePageWidth(pdf)-1.0)
	yMax := pageBreakY(pdf)

	writeHeader := func() {
		tableHeader(pdf, headers, widths)
		pdf.SetFont("Helvetica", "", fontSize)
	}
	writeHeader()

	for i := 0; ; i++ {
		cols, ok := rowAt(i)
		if !ok {
			break
		}
		if pdf.GetY()+6 > yMax {
			if landscape {
				addLandscapePage(pdf)
			} else {
				pdf.AddPage()
			}
			yMax = pageBreakY(pdf)
			writeHeader()
		}
		tableRowFit(pdf, cols, widths)
	}
	if landscape && wasPortrait {
		addPortraitPage(pdf)
	}
}

func tableRowFit(pdf *gofpdf.Fpdf, cols []string, widths []float64) {
	pdf.SetTextColor(30, 40, 50)
	for i, c := range cols {
		w := widths[i]
		if i >= len(widths) {
			break
		}
		pdf.CellFormat(w, 5.5, cellText(pdf, c, w), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

func cellText(pdf *gofpdf.Fpdf, s string, width float64) string {
	s = latin(s)
	max := width - 1.2
	if max < 2 {
		max = 2
	}
	if pdf.GetStringWidth(s) <= max {
		return s
	}
	for len(s) > 1 {
		s = s[:len(s)-1]
		if pdf.GetStringWidth(s+"~") <= max {
			return s + "~"
		}
	}
	return "~"
}
