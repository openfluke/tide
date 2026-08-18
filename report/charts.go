package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/phpdave11/gofpdf"
)

func (d *doc) lucyCharts(cells []CellPoint, heat Heat) {
	if len(heat.Modes) == 0 && len(cells) > 0 {
		heat = BuildHeat(cells)
	}
	if len(heat.Modes) == 0 {
		return
	}
	d.bars("Mean Score by train mode", zipKV(heat.Modes, heat.ModeMeanScore))
	d.bars("Mean hard Acc by train mode", zipKV(heat.Modes, heat.ModeMeanAcc))
	d.bars("Mean SoftAcc by train mode", zipKV(heat.Modes, heat.ModeMeanSoft))
	d.bars("Mean Score by dtype", zipKV(heat.DTypes, heat.DTypeMeanScore))
	d.bars("Mean hard Acc by dtype", zipKV(heat.DTypes, heat.DTypeMeanAcc))
	d.bars("Mean Score by arch", zipKV(heat.Arches, heat.ArchMeanScore))
	d.heatmap("Honesty — mean Score, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeScore)
	d.heatmap("Honesty — mean hard Acc, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeAcc)
	d.heatmap("Honesty — mean SoftAcc, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeSoft)
	d.heatmap("Mean Score, every mode × arch", heat.Modes, heat.Arches, heat.ModeArchScore)
	d.heatmap("Mean hard Acc, every mode × arch", heat.Modes, heat.Arches, heat.ModeArchAcc)
	if len(heat.Layers) > 1 {
		d.heatmap("Mean Score, every layer × mode", heat.Layers, heat.Modes, heat.LayerModeScore)
		d.heatmap("Mean hard Acc, every layer × mode", heat.Layers, heat.Modes, heat.LayerModeAcc)
	}
	pts := heat.Points
	if len(pts) == 0 {
		pts = cells
	}
	d.scatter("Pillars — Availability vs hard Acc", "Availability %", "Hard Acc %", pts,
		func(p CellPoint) float64 { return p.Avail },
		func(p CellPoint) float64 { return p.Acc })
	d.scatter("Pillars — Throughput vs hard Acc", "Throughput /s", "Hard Acc %", pts,
		func(p CellPoint) float64 { return p.Thru },
		func(p CellPoint) float64 { return p.Acc })
	d.scatter("Pareto — hard Acc vs Lucy Score", "Hard Acc %", "Lucy Score (Welvet)", pts,
		func(p CellPoint) float64 { return p.Acc },
		func(p CellPoint) float64 { return p.Score })
	d.scatter("SoftAcc (serve-confidence) vs hard Acc", "SoftAcc %", "Hard Acc %", pts,
		func(p CellPoint) float64 { return p.Soft },
		func(p CellPoint) float64 { return p.Acc })
	d.scatter("Avail vs Lucy Score (duty clock)", "Availability %", "Lucy Score", pts,
		func(p CellPoint) float64 { return p.Avail },
		func(p CellPoint) float64 { return p.Score })
}

func (d *doc) lpdBoard(l LPD) {
	if l.N == 0 || l.Champ.ID == "" {
		return
	}
	d.h2("Consciousness — Acc, Throughput, Availability")
	d.body(l.Formula)
	d.body(fmt.Sprintf("Acc champ %s   Acc %.1f  Thru %.0f  Avail %.1f%%  %.1f KiB  (RAM reference)",
		PrettyCell(l.AccChamp.ID), l.AccChamp.Acc, l.AccChamp.Thru, l.AccChamp.Avail, l.AccChamp.RAMKiB))
	d.body(fmt.Sprintf("Lucy Score champ (T x Avail x Acc) %s   Score %.1f  Acc %.1f  Soft %.0f  %.1f KiB",
		PrettyCell(l.Champ.ID), l.Champ.Score, l.Champ.Acc, l.Champ.Soft, l.Champ.RAMKiB))
	if l.LiveChamp.ID != "" {
		d.body(fmt.Sprintf("Live-fit champ (best Q) %s   Acc %.1f  Thru %.0f  Avail %.1f%%  %.1f KiB",
			PrettyCell(l.LiveChamp.ID), l.LiveChamp.Acc, l.LiveChamp.Thru, l.LiveChamp.Avail, l.LiveChamp.RAMKiB))
	}
	if l.GoldStd.ID != "" {
		d.body(fmt.Sprintf("Gold-std (2+ pillars, Acc keep >= 80%%, then smallest then fastest): %s  mode %s  Acc %.1f  Thru %.0f  %.1f KiB",
			PrettyCell(l.GoldStd.ID), l.GoldStd.Mode, l.GoldStd.Acc, l.GoldStd.Thru, l.GoldStd.RAMKiB))
	}
	if l.FastID != "" {
		d.body(fmt.Sprintf("Board fastest %s  Thru %.0f (traps may own this)     Best availability %s  %.1f%%",
			PrettyCell(l.FastID), l.FastThru, PrettyCell(l.AvailID), l.BestAvail))
		d.body(fmt.Sprintf("Learner Thru peak %.0f   learner Avail peak %.1f%%", l.PeakThru, l.PeakAvail))
	}
	d.lpdRadars(l)
	if len(l.Gold) > 0 {
		d.h2("Gold — trifecta >=80% at <=20% of Acc-champ RAM")
		d.lpdTable(l.Gold, 16)
	} else {
		d.body("No gold cell yet. Need all three pillars at 80% of learner peaks in one fifth of Acc-champ RAM.")
	}
	if len(l.GoldModes) > 0 {
		d.h2("Gold-standard train modes — 2+ pillars, Acc keep >= 80%, smallest then fastest")
		d.lpdModeTable(l.GoldModes)
	}
	if len(l.Near) > 0 {
		d.h2("Near-gold — 2+ pillars, RAM <= 50% of Acc champ")
		d.lpdTable(l.Near, 12)
	}
	d.h2("Lucy density (LPD) — Q x shrink vs Acc-champ RAM; 0 if Acc keep < 70%")
	d.lpdTable(l.Top, 20)
	if len(l.TopMix) > 0 && l.TopMix[0].Mix > 0 {
		d.h2("Consciousness rank — Q = geomean Acc/Thru/Avail keep")
		d.lpdMobileTable(l.TopMix, 10)
	}
	if len(l.Trap) > 0 {
		d.h2("Trap — tiny RAM, Acc keep < 70% (Score/MiB and SoftAcc Score lie here)")
		d.lpdTable(l.Trap, 10)
	}
	d.scatterLPD("Acc keep % vs RAM KiB (gold = high Acc, low RAM vs Acc champ)", "RAM KiB", "Acc keep % vs Acc champ", l.Top,
		func(r LPDRow) float64 { return r.RAMKiB },
		func(r LPDRow) float64 { return r.RelAcc * 100 })
	d.scatterLPD("Q% vs RAM KiB (gold = high consciousness, low RAM)", "RAM KiB", "Q %", l.Top,
		func(r LPDRow) float64 { return r.RAMKiB },
		func(r LPDRow) float64 { return r.Q * 100 })
	d.scatterLPD("Q% vs shrink vs Acc champ (gold = upper right)", "times smaller than Acc champ", "Q %", l.Top,
		func(r LPDRow) float64 { return r.Shrink },
		func(r LPDRow) float64 { return r.Q * 100 })
	var bars []kv
	for i, r := range l.Top {
		if i >= 16 || r.LPD <= 0 {
			continue
		}
		bars = append(bars, kv{CompactCell(r.ID), r.LPD})
	}
	d.bars("Top Lucy density (LPD)", bars)
}

type radarSeries struct {
	Label   string
	R, G, B int
	Vals    [3]float64
}

func (d *doc) lpdRadars(l LPD) {
	find := func(id string) LPDRow {
		id = PrettyCell(id)
		for _, r := range l.Top {
			if r.ID == id {
				return r
			}
		}
		for _, r := range append(append(append([]LPDRow{}, l.Gold...), l.Near...), l.Trap...) {
			if r.ID == id {
				return r
			}
		}
		return LPDRow{}
	}
	seen := map[string]bool{}
	add := func(dst *[]radarSeries, label string, r, g, b int, row LPDRow, dens bool) {
		if row.ID == "" || seen[label] {
			return
		}
		seen[label] = true
		var v [3]float64
		if dens {
			v = [3]float64{densNorm(row.DensAcc, l.PeakDensAcc), densNorm(row.DensThru, l.PeakDensThru), densNorm(row.DensAvail, l.PeakDensAvail)}
		} else {
			v = [3]float64{row.RelAcc, row.RelThru, row.RelAvail}
		}
		*dst = append(*dst, radarSeries{Label: label + "  " + CompactCell(row.ID), R: r, G: g, B: b, Vals: v})
	}
	var live, dens []radarSeries
	seen = map[string]bool{}
	add(&live, "Acc champ", 230, 179, 90, find(l.AccChamp.ID), false)
	add(&live, "Gold-std", 183, 121, 31, l.GoldStd, false)
	add(&live, "Live-fit", 61, 214, 198, find(l.LiveChamp.ID), false)
	add(&live, "Lucy Score", 224, 108, 117, find(l.Champ.ID), false)
	seen = map[string]bool{}
	add(&dens, "Gold-std", 183, 121, 31, l.GoldStd, true)
	if len(l.Top) > 0 && l.Top[0].LPD > 0 {
		add(&dens, "LPD lead", 61, 214, 198, l.Top[0], true)
	}
	add(&dens, "Acc champ", 230, 179, 90, find(l.AccChamp.ID), true)
	add(&dens, "Lucy Score", 224, 108, 117, find(l.Champ.ID), true)
	d.radar("Consciousness radar — Acc / Thru / Avail vs learner peaks", live)
	d.radar("Memory density radar — same pillars x shrink vs Acc champ (traps sit at origin)", dens)
}

func densNorm(v, peak float64) float64 {
	if peak <= 0 {
		return 0
	}
	x := v / peak
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func (d *doc) radar(title string, series []radarSeries) {
	if len(series) == 0 {
		return
	}
	d.h2(title)
	if d.pdf.GetY() > 205 {
		d.pdf.AddPage()
		d.h2(title)
	}
	cx, cy, radius := 68.0, d.pdf.GetY()+38, 30.0
	labels := []string{"Acc", "Thru", "Avail"}
	d.pdf.SetDrawColor(190, 198, 204)
	d.pdf.SetLineWidth(0.2)
	for ring := 1; ring <= 4; ring++ {
		rr := radius * float64(ring) / 4
		pts := make([]gofpdf.PointType, 3)
		for i := 0; i < 3; i++ {
			ang := -math.Pi/2 + float64(i)*2*math.Pi/3
			pts[i] = gofpdf.PointType{X: cx + rr*math.Cos(ang), Y: cy + rr*math.Sin(ang)}
		}
		d.pdf.Polygon(pts, "D")
	}
	d.pdf.SetFont("Helvetica", "B", 7)
	d.pdf.SetTextColor(40, 50, 55)
	for i, lab := range labels {
		ang := -math.Pi/2 + float64(i)*2*math.Pi/3
		x := cx + (radius+7)*math.Cos(ang)
		y := cy + (radius+7)*math.Sin(ang)
		d.pdf.Line(cx, cy, cx+radius*math.Cos(ang), cy+radius*math.Sin(ang))
		d.pdf.SetXY(x-10, y-3)
		d.pdf.CellFormat(20, 5, lab, "", 0, "C", false, 0, "")
	}
	d.pdf.SetLineWidth(0.7)
	for _, s := range series {
		pts := make([]gofpdf.PointType, 3)
		for i := 0; i < 3; i++ {
			ang := -math.Pi/2 + float64(i)*2*math.Pi/3
			rr := radius * s.Vals[i]
			if rr < 0.4 {
				rr = 0.4
			}
			pts[i] = gofpdf.PointType{X: cx + rr*math.Cos(ang), Y: cy + rr*math.Sin(ang)}
		}
		d.pdf.SetDrawColor(s.R, s.G, s.B)
		d.pdf.Polygon(pts, "D")
	}
	d.pdf.SetLineWidth(0.2)
	lx, ly := 118.0, cy-26
	for i, s := range series {
		d.pdf.SetFillColor(s.R, s.G, s.B)
		d.pdf.Rect(lx, ly+float64(i)*6, 3.2, 3.2, "F")
		d.pdf.SetFont("Helvetica", "", 7)
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(lx+5, ly+float64(i)*6-1.2)
		d.pdf.CellFormat(72, 5, latin(clipHead(s.Label, 44)), "", 0, "L", false, 0, "")
	}
	d.pdf.SetY(cy + radius + 12)
	d.gap(2)
}

func (d *doc) lpdTable(rows []LPDRow, max int) {
	d.table([]string{"Band", "Cell", "Acc%", "Thru%", "Avail%", "Q%", "RAM%", "xSmall", "LPD", "Acc", "Thru", "KiB"}, func(k int) []string {
		if k >= len(rows) || k >= max {
			return nil
		}
		r := rows[k]
		return []string{
			r.Band, CompactCell(r.ID),
			fmt.Sprintf("%.0f", r.RelAcc*100),
			fmt.Sprintf("%.0f", r.RelThru*100),
			fmt.Sprintf("%.0f", r.RelAvail*100),
			fmt.Sprintf("%.0f", r.Q*100),
			fmt.Sprintf("%.0f", r.RAMFrac*100),
			fmt.Sprintf("%.1f", r.Shrink),
			fmt.Sprintf("%.2f", r.LPD),
			fmt.Sprintf("%.1f", r.Acc),
			fmt.Sprintf("%.0f", r.Thru),
			fmt.Sprintf("%.1f", r.RAMKiB),
		}
	})
}

func (d *doc) lpdModeTable(modes []LPDMode) {
	d.table([]string{"Mode", "n", "Acc", "KiB", "Thru", "Cell"}, func(k int) []string {
		if k >= len(modes) {
			return nil
		}
		m := modes[k]
		cell := CompactCell(m.Smallest)
		fast := CompactCell(m.Fastest)
		if fast != "" && fast != cell {
			cell += " / " + fast
		}
		return []string{
			m.Mode,
			fmt.Sprintf("%d", m.N),
			fmt.Sprintf("%.1f", m.BestAcc),
			fmt.Sprintf("%.1f", m.MinRAM),
			fmt.Sprintf("%.0f", m.MaxThru),
			cell,
		}
	})
}

func (d *doc) lpdMobileTable(rows []LPDRow, max int) {
	d.table([]string{"Band", "Cell", "Acc%", "Thru%", "Avail%", "Q%", "LPD", "Thru", "Avail"}, func(k int) []string {
		if k >= len(rows) || k >= max {
			return nil
		}
		r := rows[k]
		return []string{
			r.Band, CompactCell(r.ID),
			fmt.Sprintf("%.0f", r.RelAcc*100),
			fmt.Sprintf("%.0f", r.RelThru*100),
			fmt.Sprintf("%.0f", r.RelAvail*100),
			fmt.Sprintf("%.0f", r.Q*100),
			fmt.Sprintf("%.2f", r.LPD),
			fmt.Sprintf("%.0f", r.Thru),
			fmt.Sprintf("%.1f", r.Avail),
		}
	})
}

func (d *doc) scatterLPD(title, xlab, ylab string, rows []LPDRow, xof, yof func(LPDRow) float64) {
	pts := make([]CellPoint, 0, len(rows))
	for _, r := range rows {
		pts = append(pts, CellPoint{Arch: r.Band, Avail: xof(r), Acc: yof(r)})
	}
	if len(pts) < 2 {
		return
	}
	d.scatter(title, xlab, ylab, pts,
		func(p CellPoint) float64 { return p.Avail },
		func(p CellPoint) float64 { return p.Acc })
}

func zipKV(keys []string, vals []float64) []kv {
	n := len(keys)
	if len(vals) < n {
		n = len(vals)
	}
	out := make([]kv, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, kv{keys[i], vals[i]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Val > out[j].Val })
	if len(out) > 28 {
		out = out[:28]
	}
	return out
}

func (d *doc) heatmap(title string, rows, cols []string, grid [][]float64) {
	if len(rows) == 0 || len(cols) == 0 || len(grid) == 0 {
		return
	}
	const maxCols = 16
	for start := 0; start < len(cols); start += maxCols {
		end := start + maxCols
		if end > len(cols) {
			end = len(cols)
		}
		subCols := cols[start:end]
		sub := make([][]float64, len(rows))
		for i := range rows {
			if i >= len(grid) {
				break
			}
			row := make([]float64, len(subCols))
			for j := start; j < end && j < len(grid[i]); j++ {
				row[j-start] = grid[i][j]
			}
			sub[i] = row
		}
		label := title
		if len(cols) > maxCols {
			label = fmt.Sprintf("%s  (cols %d-%d)", title, start+1, end)
		}
		d.heatmapPage(label, rows, subCols, sub)
	}
}

func (d *doc) heatmapPage(title string, rows, cols []string, grid [][]float64) {
	d.h2(title)
	need := 12.0 + float64(len(rows))*5.2
	if d.pdf.GetY()+need > 270 {
		d.pdf.AddPage()
		d.h2(title)
	}
	left := 18.0
	labelW := 32.0
	avail := 174.0 - labelW
	cw := avail / float64(len(cols))
	if cw > 12 {
		cw = 12
	}
	rh := 5.0
	min, max := heatRange(grid)
	d.pdf.SetFont("Helvetica", "", 5)
	d.pdf.SetTextColor(90, 100, 110)
	d.pdf.SetXY(left+labelW, d.pdf.GetY())
	for _, c := range cols {
		d.pdf.CellFormat(cw, 6, latin(clipHead(c, 8)), "", 0, "C", false, 0, "")
	}
	d.pdf.Ln(-1)
	for i, r := range rows {
		y := d.pdf.GetY()
		if y > 278 {
			d.pdf.AddPage()
			y = d.pdf.GetY()
		}
		d.pdf.SetFont("Helvetica", "", 6)
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(labelW, rh, latin(clipHead(r, 16)), "", 0, "L", false, 0, "")
		for j := range cols {
			v := 0.0
			if i < len(grid) && j < len(grid[i]) {
				v = grid[i][j]
			}
			cr, cg, cb := 36, 48, 56
			if v != 0 || (i < len(grid) && j < len(grid[i]) && hasVal(grid, i, j)) {
				t := 0.0
				if max > min {
					t = (v - min) / (max - min)
				}
				cr, cg, cb = heatRGB(t)
			}
			x := left + labelW + float64(j)*cw
			d.pdf.SetFillColor(cr, cg, cb)
			d.pdf.Rect(x, y, cw-0.3, rh-0.3, "F")
			d.pdf.SetFont("Helvetica", "", 5)
			if tlight(cr, cg, cb) {
				d.pdf.SetTextColor(20, 24, 28)
			} else {
				d.pdf.SetTextColor(240, 244, 246)
			}
			d.pdf.SetXY(x, y)
			if v == 0 && !hasVal(grid, i, j) {
				d.pdf.CellFormat(cw-0.3, rh-0.3, "", "", 0, "C", false, 0, "")
			} else {
				d.pdf.CellFormat(cw-0.3, rh-0.3, fmt.Sprintf("%.0f", v), "", 0, "C", false, 0, "")
			}
		}
		d.pdf.SetY(y + rh)
	}
	d.gap(3)
}

func hasVal(grid [][]float64, i, j int) bool {
	return i < len(grid) && j < len(grid[i]) && !math.IsNaN(grid[i][j]) && grid[i][j] != 0
}

func heatRange(grid [][]float64) (min, max float64) {
	min, max = math.Inf(1), math.Inf(-1)
	any := false
	for _, row := range grid {
		for _, v := range row {
			if v == 0 {
				continue
			}
			any = true
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}
	if !any {
		return 0, 1
	}
	if max <= min {
		max = min + 1
	}
	return
}

func heatRGB(t float64) (int, int, int) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// RdYlGn-ish: red → gold → teal
	if t < 0.5 {
		u := t * 2
		return int(197 + (231-197)*u), int(48 + (179-48)*u), int(48 + (90-48)*u)
	}
	u := (t - 0.5) * 2
	return int(231 + (45-231)*u), int(179 + (166-179)*u), int(90 + (150-90)*u)
}

func tlight(r, g, b int) bool { return r*3+g*5+b*2 > 1400 }

func clipHead(s string, n int) string {
	s = latin(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (d *doc) scatter(title, xlab, ylab string, pts []CellPoint, xof, yof func(CellPoint) float64) {
	if len(pts) < 2 {
		return
	}
	d.h2(title)
	if d.pdf.GetY() > 210 {
		d.pdf.AddPage()
		d.h2(title)
	}
	x0, y0, w, h := 28.0, d.pdf.GetY()+4, 160.0, 72.0
	xmin, xmax, ymin, ymax := xof(pts[0]), xof(pts[0]), yof(pts[0]), yof(pts[0])
	for _, p := range pts {
		x, y := xof(p), yof(p)
		if x < xmin {
			xmin = x
		}
		if x > xmax {
			xmax = x
		}
		if y < ymin {
			ymin = y
		}
		if y > ymax {
			ymax = y
		}
	}
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}
	d.pdf.SetDrawColor(40, 55, 65)
	d.pdf.Rect(x0, y0, w, h, "D")
	n := len(pts)
	if n > 800 {
		n = 800
	}
	for i := 0; i < n; i++ {
		p := pts[i]
		px := x0 + w*(xof(p)-xmin)/(xmax-xmin)
		py := y0 + h - h*(yof(p)-ymin)/(ymax-ymin)
		r, g, b := archColor(p.Arch)
		d.pdf.SetFillColor(r, g, b)
		d.pdf.Rect(px-0.5, py-0.5, 1.1, 1.1, "F")
	}
	front := paretoFront(pts, xof, yof)
	d.pdf.SetDrawColor(183, 121, 31)
	d.pdf.SetLineWidth(0.35)
	for i := 1; i < len(front); i++ {
		x1 := x0 + w*(xof(front[i-1])-xmin)/(xmax-xmin)
		y1 := y0 + h - h*(yof(front[i-1])-ymin)/(ymax-ymin)
		x2 := x0 + w*(xof(front[i])-xmin)/(xmax-xmin)
		y2 := y0 + h - h*(yof(front[i])-ymin)/(ymax-ymin)
		d.pdf.Line(x1, y1, x2, y2)
	}
	d.pdf.SetLineWidth(0.2)
	d.pdf.SetFont("Helvetica", "", 7)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.SetXY(x0, y0+h+1)
	d.pdf.CellFormat(w, 4, latin(fmt.Sprintf("%s  →    n=%d   gold = Pareto    teal=single  blue=bi  purple=tri", xlab, len(pts))), "", 1, "L", false, 0, "")
	d.pdf.SetXY(8, y0+h/2)
	d.pdf.SetFont("Helvetica", "", 6)
	d.pdf.TransformBegin()
	d.pdf.TransformRotate(90, 12, y0+h/2)
	d.pdf.CellFormat(h, 4, latin(ylab), "", 0, "C", false, 0, "")
	d.pdf.TransformEnd()
	d.pdf.SetY(y0 + h + 8)
	d.gap(2)
}

func archColor(a string) (int, int, int) {
	s := strings.ToLower(PrettyArch(a))
	switch {
	case strings.Contains(s, "gold"):
		return 183, 121, 31
	case strings.Contains(s, "trap"):
		return 197, 48, 48
	case strings.Contains(s, "near"):
		return 230, 179, 90
	case strings.Contains(s, "tri"):
		return 168, 130, 220
	case strings.Contains(s, "bi"):
		return 78, 140, 230
	default:
		return 61, 214, 198
	}
}

func paretoFront(pts []CellPoint, xof, yof func(CellPoint) float64) []CellPoint {
	type pair struct {
		p    CellPoint
		x, y float64
	}
	all := make([]pair, 0, len(pts))
	for _, p := range pts {
		all = append(all, pair{p, xof(p), yof(p)})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].x != all[j].x {
			return all[i].x < all[j].x
		}
		return all[i].y > all[j].y
	})
	var out []CellPoint
	bestY := math.Inf(-1)
	for _, a := range all {
		if a.y >= bestY {
			bestY = a.y
			out = append(out, a.p)
		}
	}
	return out
}
