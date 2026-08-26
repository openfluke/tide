package report

import (
	"fmt"
	"math"
	"sort"
)

type compareLineSeries struct {
	Label  string
	Values map[float64]float64
}

var compareChartColors = [][3]int{
	{61, 214, 198},
	{78, 163, 255},
	{230, 179, 90},
	{198, 120, 221},
	{152, 195, 121},
	{224, 108, 117},
	{86, 182, 194},
	{209, 154, 102},
}

func (d *doc) compareCharts(c CompareReport) {
	lrs, labels := compareUnionLRs(c)
	if len(lrs) == 0 {
		return
	}

	d.compareLRLineChart("Mean Acc vs learning rate", "Acc %", lrs, labels, summarySeries(c, func(r CompareLRRow) float64 { return r.MeanAcc }), false)
	d.compareLRLineChart("Mean Score vs learning rate", "Score", lrs, labels, summarySeries(c, func(r CompareLRRow) float64 { return r.MeanScore }), false)
	d.compareLRLineChart("Mean Availability vs learning rate", "Avail %", lrs, labels, summarySeries(c, func(r CompareLRRow) float64 { return r.MeanAvail }), false)

	if len(c.SoftGap) > 0 {
		d.compareLRLineChart("Soft−Acc gap vs LR (false confidence)", "Soft−Acc", lrs, labels, softGapSeries(c), false)
	}
	if len(c.TrapRate) > 0 {
		d.compareLRLineChart("Trap rate vs LR", "trap %", lrs, labels, trapRateSeries(c), false)
	}

	if len(c.Scatter) > 0 {
		d.compareScatterChart("Acc × Availability", "Acc %", "Avail %", c.Scatter,
			func(p CompareScatterPoint) float64 { return p.Acc },
			func(p CompareScatterPoint) float64 { return p.Avail },
		)
		d.compareScatterChart("SoftAcc × hard Acc", "SoftAcc %", "hard Acc %", c.Scatter,
			func(p CompareScatterPoint) float64 { return p.Soft },
			func(p CompareScatterPoint) float64 { return p.Acc },
		)
	}

	if len(c.ModeBars) > 0 {
		var bars []kv
		for _, b := range c.ModeBars {
			for _, m := range c.Machines {
				if v, ok := b.ByMachine[m]; ok && v > 0 {
					bars = append(bars, kv{fmt.Sprintf("%s · %s", b.Mode, m), v})
				}
			}
		}
		sort.Slice(bars, func(i, j int) bool { return bars[i].Val > bars[j].Val })
		if len(bars) > 20 {
			bars = bars[:20]
		}
		d.bars("Mean Acc by train mode (top pairs)", bars)
	}

	for _, g := range c.ModeLR {
		d.heatmap("Mode × LR Acc — "+g.Machine, g.Modes, g.LRLabels, g.Acc)
		d.heatmap("Mode × LR Score — "+g.Machine, g.Modes, g.LRLabels, g.Score)
		d.heatmap("Mode × LR Avail — "+g.Machine, g.Modes, g.LRLabels, g.Avail)
		d.heatmap("Mode × LR SoftAcc — "+g.Machine, g.Modes, g.LRLabels, g.Soft)
	}

	seenMachine := map[string]bool{}
	for _, s := range c.ModeSeries {
		if seenMachine[s.Machine] {
			continue
		}
		seenMachine[s.Machine] = true
		lrs, labels := machineSeriesLRs(c.ModeSeries, s.Machine)
		d.compareLRLineChart("Train modes vs LR Acc — "+s.Machine, "Acc %", lrs, labels,
			groupModeSeriesByMachine(c.ModeSeries, s.Machine, func(p CompareModeSeriesPoint) float64 { return p.Acc }), false)
		d.compareLRLineChart("Train modes vs LR Avail — "+s.Machine, "Avail %", lrs, labels,
			groupModeSeriesByMachine(c.ModeSeries, s.Machine, func(p CompareModeSeriesPoint) float64 { return p.Avail }), false)
	}

	for _, cross := range c.ModeCross {
		lrs, labels := crossLRs(cross.Points)
		d.compareLRLineChart("Mode "+cross.Mode+" — machines vs LR Acc", "Acc %", lrs, labels,
			crossSeries(cross, func(p CompareModeCrossPoint) float64 { return p.Acc }), false)
		d.compareLRLineChart("Mode "+cross.Mode+" — machines vs LR Avail", "Avail %", lrs, labels,
			crossSeries(cross, func(p CompareModeCrossPoint) float64 { return p.Avail }), false)
	}

	for _, g := range c.VsBaseline {
		baseSeries := []compareLineSeries{{
			Label:  g.Baseline,
			Values: baselineAccValues(g),
		}}
		d.compareLRLineChart(g.Baseline+" baseline Acc vs LR — "+g.Machine, "Acc %", g.LRs, g.LRLabels, baseSeries, false)

		deltaSeries := vsBaselineDeltaSeries(g, g.AccDelta)
		d.compareLRLineChart("Δ Acc vs "+g.Baseline+" — "+g.Machine, "Δ Acc", g.LRs, g.LRLabels, deltaSeries, true)
		d.compareLRLineChart("Δ Score vs "+g.Baseline+" — "+g.Machine, "Δ Score", g.LRs, g.LRLabels, vsBaselineDeltaSeries(g, g.ScoreDelta), true)
		d.compareLRLineChart("Δ Avail vs "+g.Baseline+" — "+g.Machine, "Δ Avail", g.LRs, g.LRLabels, vsBaselineDeltaSeries(g, g.AvailDelta), true)

		hit := make([][]bool, len(g.Modes))
		for i := range g.Modes {
			hit[i] = make([]bool, len(g.LRLabels))
			for j := range g.LRLabels {
				if i < len(g.AccDelta) && j < len(g.AccDelta[i]) && g.AccDelta[i][j] != 0 {
					hit[i][j] = true
				}
			}
		}
		d.heatmapSigned("Δ Acc vs "+g.Baseline+" — "+g.Machine, g.Modes, g.LRLabels, g.AccDelta, hit)
	}
}

func compareUnionLRs(c CompareReport) ([]float64, []string) {
	m := map[float64]string{}
	for _, s := range c.Summary {
		if s.LRLabel != "" {
			m[s.LR] = s.LRLabel
		} else {
			m[s.LR] = FormatLR(s.LR)
		}
	}
	return sortedLRs(m)
}

func summarySeries(c CompareReport, val func(CompareLRRow) float64) []compareLineSeries {
	byMachine := map[string]compareLineSeries{}
	for _, s := range c.Summary {
		ser := byMachine[s.Machine]
		if ser.Values == nil {
			ser.Label = s.Machine
			ser.Values = map[float64]float64{}
		}
		ser.Values[s.LR] = val(s)
		byMachine[s.Machine] = ser
	}
	out := make([]compareLineSeries, 0, len(byMachine))
	for _, m := range c.Machines {
		if ser, ok := byMachine[m]; ok {
			out = append(out, ser)
		}
	}
	return out
}

func softGapSeries(c CompareReport) []compareLineSeries {
	byMachine := map[string]compareLineSeries{}
	for _, s := range c.SoftGap {
		ser := byMachine[s.Machine]
		if ser.Values == nil {
			ser.Label = s.Machine
			ser.Values = map[float64]float64{}
		}
		ser.Values[s.LR] = s.MeanGap
		byMachine[s.Machine] = ser
	}
	var out []compareLineSeries
	for _, m := range c.Machines {
		if ser, ok := byMachine[m]; ok {
			out = append(out, ser)
		}
	}
	return out
}

func trapRateSeries(c CompareReport) []compareLineSeries {
	byMachine := map[string]compareLineSeries{}
	for _, s := range c.TrapRate {
		ser := byMachine[s.Machine]
		if ser.Values == nil {
			ser.Label = s.Machine
			ser.Values = map[float64]float64{}
		}
		ser.Values[s.LR] = s.TrapPct
		byMachine[s.Machine] = ser
	}
	var out []compareLineSeries
	for _, m := range c.Machines {
		if ser, ok := byMachine[m]; ok {
			out = append(out, ser)
		}
	}
	return out
}

func groupModeSeriesByMachine(all []CompareModeSeries, machine string, val func(CompareModeSeriesPoint) float64) []compareLineSeries {
	var out []compareLineSeries
	for _, s := range all {
		if s.Machine != machine {
			continue
		}
		out = append(out, compareLineSeries{
			Label:  s.Mode,
			Values: modeSeriesValues(s, val),
		})
	}
	return out
}

func modeSeriesValues(s CompareModeSeries, val func(CompareModeSeriesPoint) float64) map[float64]float64 {
	m := map[float64]float64{}
	for _, p := range s.Points {
		m[p.LR] = val(p)
	}
	return m
}

func machineSeriesLRs(all []CompareModeSeries, machine string) ([]float64, []string) {
	lrMap := map[float64]string{}
	for _, s := range all {
		if s.Machine != machine {
			continue
		}
		for _, p := range s.Points {
			lbl := p.LRLabel
			if lbl == "" {
				lbl = FormatLR(p.LR)
			}
			lrMap[p.LR] = lbl
		}
	}
	return sortedLRs(lrMap)
}

func crossLRs(pts []CompareModeCrossPoint) ([]float64, []string) {
	m := map[float64]string{}
	for _, p := range pts {
		lbl := p.LRLabel
		if lbl == "" {
			lbl = FormatLR(p.LR)
		}
		m[p.LR] = lbl
	}
	return sortedLRs(m)
}

func crossSeries(cross CompareModeCross, val func(CompareModeCrossPoint) float64) []compareLineSeries {
	byMachine := map[string]compareLineSeries{}
	for _, p := range cross.Points {
		ser := byMachine[p.Machine]
		if ser.Values == nil {
			ser.Label = p.Machine
			ser.Values = map[float64]float64{}
		}
		ser.Values[p.LR] = val(p)
		byMachine[p.Machine] = ser
	}
	out := make([]compareLineSeries, 0, len(byMachine))
	for m := range byMachine {
		out = append(out, byMachine[m])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func baselineAccValues(g CompareVsBaselineGrid) map[float64]float64 {
	m := map[float64]float64{}
	for i, lr := range g.LRs {
		if i < len(g.BaselineAcc) && g.BaselineAcc[i] > 0 {
			m[lr] = g.BaselineAcc[i]
		}
	}
	return m
}

func vsBaselineDeltaSeries(g CompareVsBaselineGrid, grid [][]float64) []compareLineSeries {
	var out []compareLineSeries
	for i, mode := range g.Modes {
		vals := map[float64]float64{}
		for j, lr := range g.LRs {
			if i < len(grid) && j < len(grid[i]) {
				v := grid[i][j]
				if v != 0 {
					vals[lr] = v
				}
			}
		}
		if len(vals) > 0 {
			out = append(out, compareLineSeries{Label: mode, Values: vals})
		}
	}
	return out
}

func (d *doc) compareLRLineChart(title, yLabel string, lrs []float64, lrLabels []string, series []compareLineSeries, zeroLine bool) {
	if len(lrs) == 0 || len(series) == 0 {
		return
	}
	d.h2(title)
	if d.pdf.GetY() > 210 {
		d.pdf.AddPage()
		d.h2(title)
	}
	x0, y0, w, h := 30.0, d.pdf.GetY()+4, 156.0, 52.0
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, s := range series {
		for _, lr := range lrs {
			v, ok := s.Values[lr]
			if !ok {
				continue
			}
			if v < ymin {
				ymin = v
			}
			if v > ymax {
				ymax = v
			}
		}
	}
	if zeroLine {
		ymin = math.Min(ymin, 0)
		ymax = math.Max(ymax, 0)
	}
	if !compareFinite(ymin) {
		ymin, ymax = 0, 100
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}

	d.pdf.SetDrawColor(40, 55, 65)
	d.pdf.Rect(x0, y0, w, h, "D")
	if zeroLine && ymin < 0 && ymax > 0 {
		y0z := y0 + h - h*(0-ymin)/(ymax-ymin)
		d.pdf.SetDrawColor(197, 48, 48)
		d.pdf.SetLineWidth(0.15)
		d.pdf.Line(x0, y0z, x0+w, y0z)
	}
	xAt := func(i int) float64 {
		if len(lrs) == 1 {
			return x0 + w/2
		}
		return x0 + w*float64(i)/float64(len(lrs)-1)
	}
	yAt := func(v float64) float64 { return y0 + h - h*(v-ymin)/(ymax-ymin) }

	for si, s := range series {
		col := compareChartColors[si%len(compareChartColors)]
		d.pdf.SetDrawColor(col[0], col[1], col[2])
		d.pdf.SetLineWidth(0.35)
		prevOK := false
		var px, py float64
		for i, lr := range lrs {
			v, ok := s.Values[lr]
			if !ok {
				prevOK = false
				continue
			}
			x, y := xAt(i), yAt(v)
			if prevOK {
				d.pdf.Line(px, py, x, y)
			}
			d.pdf.SetFillColor(col[0], col[1], col[2])
			d.pdf.Rect(x-0.8, y-0.8, 1.6, 1.6, "F")
			px, py, prevOK = x, y, true
		}
	}
	d.pdf.SetLineWidth(0.2)

	d.pdf.SetFont("Helvetica", "", 5)
	d.pdf.SetTextColor(110, 120, 130)
	for i, lr := range lrs {
		lbl := lrLabels[i]
		if lbl == "" {
			lbl = FormatLR(lr)
		}
		d.pdf.SetXY(xAt(i)-4, y0+h+1)
		d.pdf.CellFormat(8, 3, latin(clipHead(lbl, 6)), "", 0, "C", false, 0, "")
	}
	d.pdf.SetFont("Helvetica", "", 6)
	legY := y0 + 2
	for si, s := range series {
		if si > 7 {
			break
		}
		col := compareChartColors[si%len(compareChartColors)]
		d.pdf.SetFillColor(col[0], col[1], col[2])
		d.pdf.Rect(x0+w-42, legY+float64(si)*3.5, 2, 2, "F")
		d.pdf.SetTextColor(60, 70, 75)
		d.pdf.SetXY(x0+w-38, legY+float64(si)*3.5-0.5)
		d.pdf.CellFormat(36, 3, latin(d.fitText(s.Label, 36)), "", 0, "L", false, 0, "")
	}
	d.pdf.SetXY(x0, y0+h+5)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.CellFormat(w, 4, latin(yLabel), "", 1, "L", false, 0, "")
	d.gap(3)
}

func (d *doc) compareScatterChart(title, xlab, ylab string, pts []CompareScatterPoint, xfn, yfn func(CompareScatterPoint) float64) {
	if len(pts) < 2 {
		return
	}
	d.h2(title)
	if d.pdf.GetY() > 210 {
		d.pdf.AddPage()
		d.h2(title)
	}
	n := len(pts)
	if n > 600 {
		n = 600
	}
	x0, y0, w, h := 30.0, d.pdf.GetY()+4, 156.0, 52.0
	xmin, xmax := xfn(pts[0]), xfn(pts[0])
	ymin, ymax := yfn(pts[0]), yfn(pts[0])
	for i := 0; i < n; i++ {
		x, y := xfn(pts[i]), yfn(pts[i])
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
	if xlab == "SoftAcc %" && ylab == "hard Acc %" {
		d.pdf.SetDrawColor(197, 48, 48)
		d.pdf.SetLineWidth(0.15)
		d.pdf.Line(x0, y0+h, x0+w, y0)
	}
	machineIdx := map[string]int{}
	for i := 0; i < n; i++ {
		p := pts[i]
		mi, ok := machineIdx[p.Machine]
		if !ok {
			mi = len(machineIdx)
			machineIdx[p.Machine] = mi
		}
		col := compareChartColors[mi%len(compareChartColors)]
		px := x0 + w*(xfn(p)-xmin)/(xmax-xmin)
		py := y0 + h - h*(yfn(p)-ymin)/(ymax-ymin)
		d.pdf.SetFillColor(col[0], col[1], col[2])
		d.pdf.Rect(px-0.4, py-0.4, 0.9, 0.9, "F")
	}
	d.pdf.SetLineWidth(0.2)
	d.pdf.SetFont("Helvetica", "", 6)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.SetXY(x0, y0+h+1)
	d.pdf.CellFormat(w, 4, latin(xlab), "", 1, "L", false, 0, "")
	d.gap(3)
}

func compareFinite(v float64) bool {
	return !math.IsInf(v, 0) && !math.IsNaN(v)
}
