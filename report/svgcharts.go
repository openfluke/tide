package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/openfluke/tide/pulse"
)

// RadarSeries is one polygon on a 3-axis radar (Acc / Thru / Avail).
type RadarSeries struct {
	Label string
	Color string
	Vals  [3]float64
}

const (
	svgWScatter = 960
	svgHScatter = 440
	svgWRadar   = 960
	svgHRadar   = 480
	svgWPulse   = 960
	svgHPulse   = 520
)

// ScatterSVG renders a scatter plot with optional Pareto front.
func ScatterSVG(pts []CellPoint, xk, yk, title string) []byte {
	xfn, yfn := cellAccessor(xk), cellAccessor(yk)
	w, h := svgWScatter, svgHScatter
	if len(pts) == 0 {
		return emptySVG(w, h, title, "no finished cells yet")
	}
	xs, ys := make([]float64, len(pts)), make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = xfn(p), yfn(p)
	}
	xmin, xmax := minMax(xs)
	ymin, ymax := minMax(ys)
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}
	const padL, padR, padT, padB = 48.0, 16.0, 12.0, 36.0
	X := func(v float64) float64 { return padL + (float64(w)-padL-padR)*(v-xmin)/(xmax-xmin) }
	Y := func(v float64) float64 {
		return float64(h) - padB - (float64(h)-padT-padB)*(v-ymin)/(ymax-ymin)
	}
	var b strings.Builder
	writeSVGHead(&b, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, w, h)
	if title != "" {
		fmt.Fprintf(&b, `<text x="12" y="16" fill="#8aa0ad" font-family="sans-serif" font-size="13">%s</text>`, escSVG(title))
	}
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="#1d3342"/>`, padL, padT, float64(w)-padL-padR, float64(h)-padT-padB)
	for i, p := range pts {
		col := svgArchColor(p.Arch)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="6" height="6" fill="%s"/>`, X(xs[i])-3, Y(ys[i])-3, col)
		_ = p
	}
	front := svgParetoFront(pts, xfn, yfn)
	if len(front) > 1 {
		b.WriteString(`<polyline fill="none" stroke="#b7791f" stroke-width="1.6" points="`)
		for i, p := range front {
			if i > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, "%.1f,%.1f", X(xfn(p)), Y(yfn(p)))
		}
		b.WriteString(`"/>`)
	}
	axisLabel(&b, w, h, padL, padB, axisLabelName(xk), axisLabelName(yk))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// RadarSVG renders a 3-axis radar chart with a right-side legend (role + cell).
func RadarSVG(title string, series []RadarSeries) []byte {
	w, h := svgWRadar, svgHRadar
	if len(series) == 0 {
		return emptySVG(w, h, title, "no champ cells yet — finish ok runs first")
	}
	legX := float64(w) - 300
	cx, cy := float64(w)*0.36, float64(h)*0.54
	radius := math.Min(cx-24, cy-40)
	labels := []string{"Acc", "Thru", "Avail"}
	n := 3
	ang := func(i int) float64 { return -math.Pi/2 + float64(i)*2*math.Pi/float64(n) }
	var b strings.Builder
	writeSVGHead(&b, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, w, h)
	if title != "" {
		fmt.Fprintf(&b, `<text x="12" y="18" fill="#8aa0ad" font-family="sans-serif" font-size="13">%s</text>`, escSVG(title))
	}
	for ring := 1; ring <= 4; ring++ {
		b.WriteString(`<polygon fill="none" stroke="#1d3342" points="`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			r := radius * float64(ring) / 4
			fmt.Fprintf(&b, "%.1f,%.1f", cx+r*math.Cos(ang(i)), cy+r*math.Sin(ang(i)))
		}
		b.WriteString(`"/>`)
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#1d3342"/>`, cx, cy, cx+radius*math.Cos(ang(i)), cy+radius*math.Sin(ang(i)))
		lx := cx + (radius+22)*math.Cos(ang(i))
		ly := cy + (radius+22)*math.Sin(ang(i))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="#8aa0ad" font-family="sans-serif" font-size="13" font-weight="600" text-anchor="middle">%s</text>`, lx, ly+4, labels[i])
	}
	for _, s := range series {
		col := s.Color
		if col == "" {
			col = "#3dd6c6"
		}
		b.WriteString(`<polygon fill="none" stroke="` + col + `" stroke-width="2" opacity="0.9" points="`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			v := s.Vals[i]
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			r := radius * v
			fmt.Fprintf(&b, "%.1f,%.1f", cx+r*math.Cos(ang(i)), cy+r*math.Sin(ang(i)))
		}
		b.WriteString(`"/>`)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" fill="#8aa0ad" font-family="sans-serif" font-size="11">each triangle = one champ cell</text>`, legX)
	ly := 42.0
	for _, s := range series {
		col := s.Color
		if col == "" {
			col = "#3dd6c6"
		}
		role, cell := splitRadarLabel(s.Label)
		fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="12" height="12" fill="%s"/>`, legX, ly-10, col)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="#e6eef2" font-family="sans-serif" font-size="12" font-weight="600">%s</text>`, legX+18, ly, escSVG(role))
		if cell != "" {
			ly += 14
			fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="#8aa0ad" font-family="monospace" font-size="10">%s</text>`, legX+18, ly, escSVG(clipStr(cell, 36)))
		}
		ly += 22
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func splitRadarLabel(s string) (role, cell string) {
	if i := strings.Index(s, " · "); i >= 0 {
		return s[:i], s[i+3:]
	}
	return s, ""
}

func clipStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// PulseSVG renders multi-panel pulse history (last window points).
func PulseSVG(history []pulse.HistoryPoint, best pulse.Best, end int) []byte {
	w, h := svgWPulse, svgHPulse
	if len(history) < 2 {
		return emptySVG(w, h, "Live pulse", "waiting for pulses…")
	}
	if end <= 0 || end >= len(history) {
		end = len(history) - 1
	}
	start := end - 240
	if start < 0 {
		start = 0
	}
	slice := history[start : end+1]
	type metric struct {
		key, snap, bestKey, label, color string
		fixed                            *float64
		digits                           int
	}
	fixed100 := 100.0
	metrics := []metric{
		{"accuracy", "avg_accuracy", "accuracy", "Acc %", "#e6b35a", &fixed100, 1},
		{"throughput", "throughput", "throughput", "throughput /s", "#7aa2f7", nil, 1},
		{"availability", "availability", "availability", "availability %", "#c3a6ff", &fixed100, 1},
		{"score", "score", "score", "lucy score", "#3dd6c6", nil, 3},
	}
	scoreC := best.Score
	var b strings.Builder
	writeSVGHead(&b, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, w, h)
	const padL, padR, padTop, gap = 52.0, 12.0, 4.0, 6.0
	panelH := (float64(h) - padTop - gap*float64(len(metrics)-1)) / float64(len(metrics))
	for mi, m := range metrics {
		y0 := padTop + float64(mi)*(panelH+gap)
		plotT := y0 + 22
		plotB := y0 + panelH - 2
		fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="rgba(0,0,0,0.2)"/>`, padL, y0, float64(w)-padL-padR, panelH)
		vals := make([]float64, len(slice))
		for i, p := range slice {
			switch m.key {
			case "accuracy":
				vals[i] = p.Accuracy
			case "throughput":
				vals[i] = p.Throughput
			case "availability":
				vals[i] = p.Availability
			case "score":
				vals[i] = p.Score
			}
		}
		maxV := 1e-9
		if m.fixed != nil {
			maxV = *m.fixed
		} else {
			for _, v := range vals {
				if v > maxV {
					maxV = v
				}
			}
		}
		metricC := bestForKey(best, m.bestKey)
		metricV, metricID := champMetricValue(metricC, m.snap)
		scoreV, scoreID := champMetricValue(scoreC, m.snap)
		if m.fixed == nil {
			if metricV > maxV {
				maxV = metricV
			}
			if scoreV > maxV {
				maxV = scoreV
			}
		}
		yAt := func(v float64) float64 { return plotB - (v/maxV)*(plotB-plotT) }
		x0, x1 := padL, float64(w)-padR
		// Metric champ — dashed white (under live line).
		if metricC != nil && metricV > 0 {
			y := yAt(metricV)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="rgba(255,255,255,0.75)" stroke-width="1.5" stroke-dasharray="7 5"/>`,
				x0, y, x1, y)
		}
		// Score champ — dotted teal when a different cell.
		if scoreC != nil && scoreV > 0 && (metricID == "" || scoreID != metricID) {
			y := yAt(scoreV)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="rgba(61,214,198,0.9)" stroke-width="1.5" stroke-dasharray="2 4"/>`,
				x0, y, x1, y)
		}
		b.WriteString(`<polyline fill="none" stroke="` + m.color + `" stroke-width="2" points="`)
		for i, v := range vals {
			if i > 0 {
				b.WriteByte(' ')
			}
			x := padL + float64(i)*((float64(w)-padL-padR)/math.Max(float64(len(vals)-1), 1))
			fmt.Fprintf(&b, "%.1f,%.1f", x, yAt(v))
		}
		b.WriteString(`"/>`)
		tip := vals[len(vals)-1]
		label := fmt.Sprintf("%s · %."+fmtDigit(m.digits), m.label, tip)
		if metricC != nil {
			label += fmt.Sprintf("  │ champ %."+fmtDigit(m.digits), metricV)
		}
		if scoreC != nil && (metricID == "" || scoreID != metricID) {
			label += fmt.Sprintf("  │ score-champ %."+fmtDigit(m.digits), scoreV)
		}
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="%s" font-family="sans-serif" font-size="10">%s</text>`, padL+4, y0+11, m.color, escSVG(label))
	}
	if tip := slice[len(slice)-1]; tip.CellID != "" {
		fmt.Fprintf(&b, `<text x="%.0f" y="12" fill="#e6eef2" font-family="monospace" font-size="11" text-anchor="end">%s</text>`, float64(w)-padR, escSVG(PrettyCell(tip.CellID)))
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func fmtDigit(d int) string {
	return fmt.Sprintf("%df", d)
}

func bestForKey(best pulse.Best, key string) *pulse.Result {
	switch key {
	case "accuracy":
		return best.Accuracy
	case "throughput":
		return best.Throughput
	case "availability":
		return best.Availability
	case "score":
		return best.Score
	default:
		return nil
	}
}

func champMetricValue(r *pulse.Result, snap string) (float64, string) {
	if r == nil {
		return 0, ""
	}
	id := r.Cell.ID
	switch snap {
	case "avg_accuracy":
		return r.Snapshot.AvgAccuracy, id
	case "soft_acc":
		return r.Snapshot.SoftAcc, id
	case "throughput":
		return r.Snapshot.Throughput, id
	case "availability":
		return r.Snapshot.Availability, id
	case "score":
		return r.Snapshot.Score, id
	default:
		return r.Snapshot.Score, id
	}
}

// HeatmapSVG renders a colored grid table as SVG.
func HeatmapSVG(rows, cols []string, grid [][]float64, title string, signed bool) []byte {
	w, h := 960, 320
	if len(rows) == 0 || len(cols) == 0 {
		return emptySVG(w, h, title, "no finished cells yet")
	}
	min, max, maxAbs := math.Inf(1), math.Inf(-1), 0.0
	for i := range rows {
		for j := range cols {
			if grid == nil || i >= len(grid) || j >= len(grid[i]) {
				continue
			}
			v := grid[i][j]
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}
	}
	if !isFinite(min) {
		min, max = 0, 1
	}
	if max <= min {
		max = min + 1
	}
	if maxAbs < 1e-9 {
		maxAbs = 1
	}
	cellW, cellH := 72.0, 24.0
	rowLab := 88.0
	headerH := 18.0
	top := 8.0
	totalW := int(rowLab + cellW*float64(len(cols)) + 16)
	totalH := int(top + headerH + cellH*float64(len(rows)+1) + 12)
	var b strings.Builder
	writeSVGHead(&b, totalW, totalH)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, totalW, totalH)
	gridTop := top + headerH
	for j, c := range cols {
		x := rowLab + float64(j)*cellW
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="#8aa0ad" font-family="sans-serif" font-size="10" text-anchor="middle">%s</text>`, x+cellW/2, top+13, escSVG(PrettyMode(c)))
	}
	for i, r := range rows {
		y := gridTop + float64(i+1)*cellH
		fmt.Fprintf(&b, `<text x="8" y="%.0f" fill="#8aa0ad" font-family="sans-serif" font-size="10">%s</text>`, y+16, escSVG(PrettyMode(r)))
		for j := range cols {
			x := rowLab + float64(j)*cellW
			v := 0.0
			has := false
			if grid != nil && i < len(grid) && j < len(grid[i]) {
				v = grid[i][j]
				has = true
			}
			if !has {
				fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="#121a20" stroke="#1d3342"/>`, x, y, cellW-2, cellH-2)
				continue
			}
			var t float64
			if signed {
				t = 0.5 + 0.5*(v/maxAbs)
			} else {
				t = (v - min) / (max - min)
			}
			cr, cg, cb := svgHeatRGB(t)
			ink := "#f4f7f8"
			if cr*3+cg*5+cb*2 <= 1400 {
				ink = "#1a2024"
			}
			lab := fmt.Sprintf("%.0f", v)
			if signed {
				lab = fmt.Sprintf("%+.1f", v)
			}
			fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="rgb(%d,%d,%d)" stroke="#1d3342"/>`, x, y, cellW-2, cellH-2, cr, cg, cb)
			fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="%s" font-family="sans-serif" font-size="10" text-anchor="middle">%s</text>`, x+(cellW-2)/2, y+16, ink, lab)
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// LPDScatterSVG renders an LPD scatter (RAM vs Q%, etc.).
// Points are colored by train mode so mode differences are visible; band is secondary (ring).
func LPDScatterSVG(pts []lpdScatterPt, xk, yk string, xMax, yMax float64, invertX bool) []byte {
	w, h := svgWScatter, svgHScatter
	if len(pts) == 0 {
		return emptySVG(w, h, "", "no LPD points yet")
	}
	xfn, yfn := lpdAccessor(xk), lpdAccessor(yk)
	xs, ys := make([]float64, len(pts)), make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = xfn(p), yfn(p)
	}
	xmin, xmax := minMax(xs)
	ymin, ymax := minMax(ys)
	if xMax > 0 {
		xmax = xMax
	}
	if yMax > 0 {
		ymax = yMax
	}
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}
	const padL, padR, padT, padB = 48.0, 16.0, 28.0, 36.0
	X := func(v float64) float64 {
		if invertX {
			return padL + (float64(w)-padL-padR)*(1-(v-xmin)/(xmax-xmin))
		}
		return padL + (float64(w)-padL-padR)*(v-xmin)/(xmax-xmin)
	}
	Y := func(v float64) float64 {
		return float64(h) - padB - (float64(h)-padT-padB)*(v-ymin)/(ymax-ymin)
	}
	modes := uniqModes(pts)
	var b strings.Builder
	writeSVGHead(&b, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="#1d3342"/>`, padL, padT, float64(w)-padL-padR, float64(h)-padT-padB)
	// mode legend along top
	lx := padL
	for _, m := range modes {
		col := svgModeColor(m)
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="14" r="4" fill="%s"/>`, lx+4, col)
		lab := PrettyMode(m)
		fmt.Fprintf(&b, `<text x="%.0f" y="17" fill="#8aa0ad" font-family="sans-serif" font-size="10">%s</text>`, lx+12, escSVG(lab))
		lx += float64(8 + len(lab)*6 + 14)
		if lx > float64(w)-80 {
			break
		}
	}
	for i, p := range pts {
		col := svgModeColor(p.Mode)
		r := 4.0
		if p.Gold {
			r = 6
		}
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s" opacity="0.9"/>`, X(xs[i]), Y(ys[i]), r, col)
		if p.Gold {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="8" fill="none" stroke="#b7791f" stroke-width="1.4"/>`, X(xs[i]), Y(ys[i]))
		} else if p.Lean {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="none" stroke="#9ece6a" stroke-width="1.2"/>`, X(xs[i]), Y(ys[i]))
		}
	}
	// Gold / lean guides when axes are RAM × keep%.
	if xk == "ram" && !invertX && xmax > xmin {
		goldRAM := xmax * LPDGoldRAM // xmax is Acc-champ RAM → 20% line
		if goldRAM >= xmin && goldRAM <= xmax {
			gx := X(goldRAM)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#b7791f" stroke-width="1" stroke-dasharray="5 4" opacity="0.85"/>`,
				gx, padT, gx, float64(h)-padB)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="#b7791f" font-family="sans-serif" font-size="10">20%% Acc-champ RAM</text>`,
				gx+4, padT+12)
		}
	}
	if yk == "accpct" || yk == "qpct" {
		drawH := func(pct float64, col, lab string) {
			if pct < ymin || pct > ymax {
				return
			}
			gy := Y(pct)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" stroke-dasharray="5 4" opacity="0.85"/>`,
				padL, gy, float64(w)-padR, gy, col)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-family="sans-serif" font-size="10">%s</text>`,
				padL+4, gy-4, col, escSVG(lab))
		}
		drawH(LPDGoldKeep*100, "#b7791f", "80% keep")
		if yk == "accpct" {
			drawH(LPDLeanKeep*100, "#9ece6a", "lean 95% Acc keep")
		}
	}
	axisLabel(&b, w, h, padL, padB, axisLabelName(xk), axisLabelName(yk))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

type lpdScatterPt struct {
	Band   string
	Mode   string
	Gold   bool
	Lean   bool
	RAM    float64
	Shrink float64
	QPct   float64
	AccPct float64
}

// LPDScatterPoints builds scatter inputs from an LPD board.
func LPDScatterPoints(l LPD) []lpdScatterPt {
	seen := map[string]bool{}
	var out []lpdScatterPt
	add := func(r LPDRow) {
		k := r.ID
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, lpdScatterPt{
			Band: r.Band, Mode: r.Mode, Gold: r.Gold,
			Lean: r.RelAcc >= LPDLeanKeep,
			RAM: r.RAMKiB, Shrink: r.Shrink,
			QPct: (r.Q) * 100, AccPct: (r.RelAcc) * 100,
		})
	}
	// Prefer full Pool so Acc-champ / gold cells aren't missing after Top[:40].
	for _, r := range l.Pool {
		add(r)
	}
	for _, r := range l.Gold {
		add(r)
	}
	for _, r := range l.Near {
		add(r)
	}
	for _, r := range l.Lean {
		add(r)
	}
	for _, r := range l.Trap {
		add(r)
	}
	for _, r := range l.Top {
		add(r)
	}
	return out
}

func uniqModes(pts []lpdScatterPt) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pts {
		m := p.Mode
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func svgModeColor(mode string) string {
	palette := []string{
		"#3dd6c6", "#e6b35a", "#7aa2f7", "#c3a6ff", "#e06c75",
		"#5ecf8a", "#f0a070", "#6ec6ff", "#d4a5ff", "#ffa0b0",
	}
	if mode == "" {
		return "#8aa0ad"
	}
	h := uint32(2166136261)
	for i := 0; i < len(mode); i++ {
		h ^= uint32(mode[i])
		h *= 16777619
	}
	return palette[int(h)%len(palette)]
}

func RadarFromLPD(l LPD) (live, dens []RadarSeries) {
	n := func(v, peak float64) float64 {
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
	// Synth a row from a champ when Top truncation / JSON omit of Pool would hide it.
	synth := func(c LPDChamp) LPDRow {
		ra, rt, rv := 0.0, 0.0, 0.0
		if l.PeakAcc > 0 {
			ra = c.Acc / l.PeakAcc
		}
		if l.PeakThru > 0 {
			rt = c.Thru / l.PeakThru
		}
		if l.PeakAvail > 0 {
			rv = c.Avail / l.PeakAvail
		}
		ref := l.AccChamp.RAMKiB
		if ref <= 0 {
			ref = 1e-6
		}
		ram := c.RAMKiB
		if ram <= 0 {
			ram = 1e-6
		}
		shrink := ref / ram
		if shrink > LPDShrinkCap {
			shrink = LPDShrinkCap
		}
		da, dt, dv := 0.0, 0.0, 0.0
		if ra >= LPDKeepFloor {
			da, dt, dv = ra*shrink, rt*shrink, rv*shrink
		}
		return LPDRow{
			ID: c.ID, Mode: c.Mode, DType: c.DType, Arch: c.Arch,
			Score: c.Score, Soft: c.Soft, Acc: c.Acc, Thru: c.Thru, Avail: c.Avail, RAMKiB: c.RAMKiB,
			RelAcc: ra, RelThru: rt, RelAvail: rv,
			DensAcc: da, DensThru: dt, DensAvail: dv, Shrink: shrink, LPD: 0,
		}
	}
	rowBy := func(id string) *LPDRow {
		if id == "" {
			return nil
		}
		for _, src := range [][]LPDRow{l.Pool, l.Top, l.Gold, l.Near, l.Lean, l.Trap} {
			for i := range src {
				if src[i].ID == id {
					return &src[i]
				}
			}
		}
		if l.GoldStd.ID == id {
			r := l.GoldStd
			return &r
		}
		if l.LeanChamp.ID == id {
			r := l.LeanChamp
			return &r
		}
		return nil
	}
	rowOrChamp := func(id string, c LPDChamp) *LPDRow {
		if r := rowBy(id); r != nil {
			return r
		}
		if c.ID == "" {
			return nil
		}
		r := synth(c)
		return &r
	}
	cellLabel := func(row *LPDRow) string {
		if row == nil {
			return ""
		}
		mode := PrettyMode(row.Mode)
		if mode == "" {
			mode = "—"
		}
		dtype := row.DType
		if dtype == "" {
			dtype = "—"
		}
		arch := PrettyArch(row.Arch)
		if arch == "" {
			arch = "—"
		}
		return mode + " · " + dtype + " · " + arch
	}
	push := func(arr *[]RadarSeries, role, color string, row *LPDRow, density bool) {
		if row == nil {
			return
		}
		var vals [3]float64
		if density {
			vals = [3]float64{
				n(row.DensAcc, l.PeakDensAcc),
				n(row.DensThru, l.PeakDensThru),
				n(row.DensAvail, l.PeakDensAvail),
			}
		} else {
			vals = [3]float64{row.RelAcc, row.RelThru, row.RelAvail}
		}
		*arr = append(*arr, RadarSeries{
			Label: role + " · " + cellLabel(row),
			Color: color,
			Vals:  vals,
		})
	}
	if l.AccChamp.ID != "" {
		push(&live, "Acc champ", "#e6b35a", rowOrChamp(l.AccChamp.ID, l.AccChamp), false)
	}
	if l.LeanChamp.ID != "" {
		push(&live, "Lean ≥95%", "#9ece6a", &l.LeanChamp, false)
	}
	if l.GoldStd.ID != "" {
		push(&live, "Gold-std", "#b7791f", rowOrChamp(l.GoldStd.ID, LPDChamp{
			ID: l.GoldStd.ID, Mode: l.GoldStd.Mode, DType: l.GoldStd.DType, Arch: l.GoldStd.Arch,
			Score: l.GoldStd.Score, Soft: l.GoldStd.Soft, Acc: l.GoldStd.Acc, Thru: l.GoldStd.Thru,
			Avail: l.GoldStd.Avail, RAMKiB: l.GoldStd.RAMKiB,
		}), false)
	}
	if l.LiveChamp.ID != "" {
		push(&live, "Live-fit", "#3dd6c6", rowOrChamp(l.LiveChamp.ID, l.LiveChamp), false)
	}
	if l.Champ.ID != "" {
		push(&live, "Lucy Score", "#e06c75", rowOrChamp(l.Champ.ID, l.Champ), false)
	}
	if l.LeanChamp.ID != "" {
		push(&dens, "Lean ≥95%", "#9ece6a", &l.LeanChamp, true)
	}
	if l.GoldStd.ID != "" {
		gs := l.GoldStd
		push(&dens, "Gold-std", "#b7791f", &gs, true)
	}
	if len(l.Top) > 0 && l.Top[0].LPD > 0 {
		push(&dens, "LPD lead", "#3dd6c6", &l.Top[0], true)
	}
	if l.AccChamp.ID != "" {
		push(&dens, "Acc champ", "#e6b35a", rowOrChamp(l.AccChamp.ID, l.AccChamp), true)
	}
	if l.Champ.ID != "" {
		push(&dens, "Lucy Score", "#e06c75", rowOrChamp(l.Champ.ID, l.Champ), true)
	}
	return live, dens
}

func cellAccessor(k string) func(CellPoint) float64 {
	switch k {
	case "availability":
		return func(p CellPoint) float64 { return p.Avail }
	case "avg_accuracy":
		return func(p CellPoint) float64 { return p.Acc }
	case "soft_acc":
		return func(p CellPoint) float64 { return p.Soft }
	case "score":
		return func(p CellPoint) float64 { return p.Score }
	case "throughput":
		return func(p CellPoint) float64 { return p.Thru }
	case "adapt_pct":
		return func(p CellPoint) float64 { return p.Adapt }
	default:
		return func(p CellPoint) float64 { return 0 }
	}
}

func lpdAccessor(k string) func(lpdScatterPt) float64 {
	switch k {
	case "ram":
		return func(p lpdScatterPt) float64 { return p.RAM }
	case "shrink":
		return func(p lpdScatterPt) float64 { return p.Shrink }
	case "qpct":
		return func(p lpdScatterPt) float64 { return p.QPct }
	case "accpct":
		return func(p lpdScatterPt) float64 { return p.AccPct }
	default:
		return func(p lpdScatterPt) float64 { return 0 }
	}
}

func svgParetoFront(pts []CellPoint, xfn, yfn func(CellPoint) float64) []CellPoint {
	cp := append([]CellPoint(nil), pts...)
	sortCellByX(cp, xfn)
	var out []CellPoint
	best := -math.Inf(1)
	for _, p := range cp {
		y := yfn(p)
		if y >= best {
			best = y
			out = append(out, p)
		}
	}
	return out
}

func sortCellByX(pts []CellPoint, xfn func(CellPoint) float64) {
	sortSlice(pts, func(i, j int) bool { return xfn(pts[i]) < xfn(pts[j]) })
}

func sortSlice[T any](s []T, less func(i, j int) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func minMax(v []float64) (min, max float64) {
	if len(v) == 0 {
		return 0, 1
	}
	min, max = v[0], v[0]
	for _, x := range v[1:] {
		if x < min {
			min = x
		}
		if x > max {
			max = x
		}
	}
	return min, max
}

func svgArchColor(arch string) string {
	a := strings.ToLower(arch)
	if strings.Contains(a, "tri") {
		return "#a882dc"
	}
	if strings.Contains(a, "bi") {
		return "#4ea3ff"
	}
	return "#3dd6c6"
}

func lpdBandColor(band string) string {
	switch band {
	case "gold":
		return "#b7791f"
	case "near":
		return "#e6b35a"
	case "trap":
		return "#e06c75"
	case "keep":
		return "#3dd6c6"
	case "acc":
		return "#7aa2f7"
	default:
		return "#8aa0ad"
	}
}

func svgHeatRGB(t float64) (r, g, b int) {
	if t < 0.5 {
		u := t * 2
		return int(197 + (231-197)*u), int(48 + (179-48)*u), int(48 + (90-48)*u)
	}
	u := (t - 0.5) * 2
	return int(231 + (45-231)*u), int(179 + (166-179)*u), int(90 + (150-90)*u)
}

func axisLabelName(k string) string {
	switch k {
	case "availability":
		return "Avail %"
	case "avg_accuracy":
		return "Hard Acc %"
	case "soft_acc":
		return "SoftAcc"
	case "score":
		return "Score"
	case "ram":
		return "RAM KiB"
	case "qpct":
		return "Q %"
	case "accpct":
		return "Acc keep %"
	case "shrink":
		return "shrink ×"
	case "throughput":
		return "Thru /s"
	case "adapt_pct":
		return "AdaptPct %"
	default:
		return k
	}
}

func writeSVGHead(b *strings.Builder, w, h int) {
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="100%%" height="100%%" viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet">`, w, h)
}

func emptySVG(w, h int, title, msg string) []byte {
	var b strings.Builder
	writeSVGHead(&b, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0d1216"/>`, w, h)
	if title != "" {
		fmt.Fprintf(&b, `<text x="12" y="16" fill="#8aa0ad" font-family="sans-serif" font-size="13">%s</text>`, escSVG(title))
	}
	fmt.Fprintf(&b, `<text x="20" y="40" fill="#8aa0ad" font-family="sans-serif" font-size="13">%s</text>`, escSVG(msg))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func axisLabel(b *strings.Builder, w, h int, padL, padB int, xLab, yLab string) {
	fmt.Fprintf(b, `<text x="%.0f" y="%d" fill="#8aa0ad" font-family="sans-serif" font-size="12" text-anchor="middle">%s</text>`, float64(padL)+float64(w-padL-12)/2, h-8, escSVG(xLab))
	fmt.Fprintf(b, `<text x="14" y="%.0f" fill="#8aa0ad" font-family="sans-serif" font-size="12" text-anchor="middle" transform="rotate(-90 14 %.0f)">%s</text>`, float64(h)/2, float64(h)/2, escSVG(yLab))
}

func isFinite(v float64) bool {
	return !math.IsInf(v, 0) && !math.IsNaN(v)
}

func escSVG(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
