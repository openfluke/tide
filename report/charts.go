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
	d.bars("Mean Availability by train mode", zipKV(heat.Modes, heat.ModeMeanAvail))
	d.bars("Mean Throughput by train mode", zipKV(heat.Modes, heat.ModeMeanThru))
	d.bars("Mean Score by dtype", zipKV(heat.DTypes, heat.DTypeMeanScore))
	d.bars("Mean hard Acc by dtype", zipKV(heat.DTypes, heat.DTypeMeanAcc))
	d.bars("Mean Score by arch", zipKV(heat.Arches, heat.ArchMeanScore))
	d.heatmap("Honesty — mean Score, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeScore)
	d.heatmap("Honesty — mean hard Acc, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeAcc)
	d.heatmap("Honesty — mean SoftAcc, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeSoft)
	d.heatmap("Honesty — mean Avail, every mode × dtype", heat.Modes, heat.DTypes, heat.ModeDTypeAvail)
	d.heatmap("Mean Score, every mode × arch", heat.Modes, heat.Arches, heat.ModeArchScore)
	d.heatmap("Mean hard Acc, every mode × arch", heat.Modes, heat.Arches, heat.ModeArchAcc)
	d.heatmap("Mean Avail, every mode × arch", heat.Modes, heat.Arches, heat.ModeArchAvail)
	if len(heat.Layers) > 1 {
		d.heatmap("Mean Score, every layer × mode", heat.Layers, heat.Modes, heat.LayerModeScore)
		d.heatmap("Mean hard Acc, every layer × mode", heat.Layers, heat.Modes, heat.LayerModeAcc)
	}
	pts := cells
	if len(pts) == 0 {
		pts = heat.Points
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
	d.scatter("Pillars — Throughput vs Availability", "Throughput /s", "Availability %", pts,
		func(p CellPoint) float64 { return p.Thru },
		func(p CellPoint) float64 { return p.Avail })
	if anyAdapt(pts) {
		d.scatter("AdaptPct vs hard Acc", "AdaptPct %", "Hard Acc %", pts,
			func(p CellPoint) float64 { return p.Adapt },
			func(p CellPoint) float64 { return p.Acc })
	}
	d.vsBoard(heat)
}

func anyAdapt(pts []CellPoint) bool {
	for _, p := range pts {
		if p.Adapt > 0.05 {
			return true
		}
	}
	return false
}

func (d *doc) vsBoard(heat Heat) {
	vs := heat.Vs
	if vs == nil || vs.Baseline == "" || len(vs.Modes) == 0 {
		return
	}
	d.h2("vs " + PrettyMode(vs.Baseline) + " — matched dtype x format x arch")
	d.body("Hard AccΔ is percentage points vs baseline on the same recipe — not Acc-keep%. ScoreΔ is usually Availability (duty clock). Acc win = Hard AccΔ > 0.5. Score win = ScoreΔ > 1. Baseline is discovered from the board. For footprint vs Acc champ use Lean / Acc keep % in Consciousness below.")
	modes := vs.Modes
	d.table([]string{"mode", "n", "Hard AccΔ", "Acc win%", "SoftΔ", "AvailΔ", "ThruΔ", "ScoreΔ", "Score win%"}, func(i int) []string {
		if i >= len(modes) {
			return nil
		}
		m := modes[i]
		return []string{
			PrettyMode(m.Mode),
			fmt.Sprintf("%d", m.N),
			fmt.Sprintf("%+.1f", m.AccDelta),
			fmt.Sprintf("%.0f", m.AccWin),
			fmt.Sprintf("%+.1f", m.SoftDelta),
			fmt.Sprintf("%+.1f", m.AvailDelta),
			fmt.Sprintf("%+.0f", m.ThruDelta),
			fmt.Sprintf("%+.1f", m.ScoreDelta),
			fmt.Sprintf("%.0f", m.ScoreWin),
		}
	})
	d.signedBars("mean Hard AccΔ vs "+PrettyMode(vs.Baseline)+" (pp)", vsDeltaKV(vs.Modes, func(m VsMode) float64 { return m.AccDelta }))
	d.signedBars("mean ThruΔ vs "+PrettyMode(vs.Baseline), vsDeltaKV(vs.Modes, func(m VsMode) float64 { return m.ThruDelta }))
	d.signedBars("mean ScoreΔ vs "+PrettyMode(vs.Baseline)+" (duty clock)", vsDeltaKV(vs.Modes, func(m VsMode) float64 { return m.ScoreDelta }))
	d.signedBars("mean AvailΔ vs "+PrettyMode(vs.Baseline), vsDeltaKV(vs.Modes, func(m VsMode) float64 { return m.AvailDelta }))
	d.deltaHeat("Hard AccΔ vs "+PrettyMode(vs.Baseline)+", every mode x dtype", vs.ByDType, func(b DeltaBin) float64 { return b.Acc })
	d.deltaHeat("ScoreΔ vs "+PrettyMode(vs.Baseline)+", every mode x dtype", vs.ByDType, func(b DeltaBin) float64 { return b.Score })
	d.deltaHeat("AvailΔ vs "+PrettyMode(vs.Baseline)+", every mode x dtype", vs.ByDType, func(b DeltaBin) float64 { return b.Avail })
	d.deltaHeat("Hard AccΔ vs "+PrettyMode(vs.Baseline)+", every mode x arch / cam", vs.ByArch, func(b DeltaBin) float64 { return b.Acc })
	if len(vs.ByLayer) > 0 {
		d.deltaHeat("Hard AccΔ vs "+PrettyMode(vs.Baseline)+", every mode x layer", vs.ByLayer, func(b DeltaBin) float64 { return b.Acc })
	}
	if len(vs.Families) > 0 {
		fams := vs.Families
		d.h2("Family collapse - Step* vs non-Step")
		d.body("Step* is a 1D pipe (one sample enters layer 0 per tick; output is the sample that entered depth ticks ago). Non-Step of the same family is a full-chain update. AccD is a real scheduler split, not leftover warm-up forwards.")
		d.table([]string{"step", "plain", "n", "mean |Hard AccΔ|"}, func(i int) []string {
			if i >= len(fams) {
				return nil
			}
			f := fams[i]
			return []string{PrettyMode(f.Step), PrettyMode(f.Plain), fmt.Sprintf("%d", f.N), fmt.Sprintf("%.2f", f.MeanAbsAcc)}
		})
	}
}

func vsDeltaKV(modes []VsMode, val func(VsMode) float64) []kv {
	out := make([]kv, 0, len(modes))
	for _, m := range modes {
		out = append(out, kv{PrettyMode(m.Mode), val(m)})
	}
	return out
}

func (d *doc) deltaHeat(title string, bins []DeltaBin, val func(DeltaBin) float64) {
	rows, cols, grid, hit := pivotDelta(bins, val)
	d.heatmapSigned(title, rows, cols, grid, hit)
}

func (d *doc) signedBars(title string, items []kv) {
	if len(items) == 0 {
		return
	}
	d.h2(title)
	maxAbs := 1.0
	for _, it := range items {
		a := math.Abs(it.Val)
		if a > maxAbs {
			maxAbs = a
		}
	}
	left := 18.0
	labelW := 52.0
	barW := 110.0
	barH := 4.2
	mid := left + labelW + barW/2
	d.pdf.SetFont("Helvetica", "", 6)
	for _, it := range items {
		if d.pdf.GetY()+barH > d.pageBreakY() {
			d.pdf.AddPage()
			d.h2(title)
			d.pdf.SetFont("Helvetica", "", 6)
		}
		y := d.pdf.GetY()
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(labelW, barH, d.fitText(it.Label, labelW), "", 0, "L", false, 0, "")
		w := (barW / 2) * math.Abs(it.Val) / maxAbs
		if it.Val >= 0 {
			d.pdf.SetFillColor(45, 166, 150)
			d.pdf.Rect(mid, y+0.7, w, barH-1.4, "F")
		} else {
			d.pdf.SetFillColor(197, 48, 48)
			d.pdf.Rect(mid-w, y+0.7, w, barH-1.4, "F")
		}
		d.pdf.SetDrawColor(90, 100, 110)
		d.pdf.Line(mid, y+0.4, mid, y+barH-0.4)
		d.pdf.SetXY(left+labelW+barW+2, y)
		d.pdf.CellFormat(22, barH, fmt.Sprintf("%+.1f", it.Val), "", 1, "L", false, 0, "")
	}
	d.gap(3)
}

func (d *doc) lpdBoard(l LPD) {
	if l.N == 0 || l.Champ.ID == "" {
		return
	}
	d.h2("Consciousness — Acc keep, Throughput, Availability")
	d.body(l.Formula)
	d.body(fmt.Sprintf("Acc champ (Hard Acc peak) %s   Hard Acc %.1f  Acc keep 100%%  Thru %.0f  Avail %.1f%%  %.1f KiB  (RAM reference)",
		PrettyCell(l.AccChamp.ID), l.AccChamp.Acc, l.AccChamp.Thru, l.AccChamp.Avail, l.AccChamp.RAMKiB))
	d.body(fmt.Sprintf("Lucy Score champ (T x Avail x Hard Acc) %s   Score %.1f  Hard Acc %.1f  Soft %.0f  %.1f KiB",
		PrettyCell(l.Champ.ID), l.Champ.Score, l.Champ.Acc, l.Champ.Soft, l.Champ.RAMKiB))
	if l.LiveChamp.ID != "" {
		d.body(fmt.Sprintf("Live-fit champ (best Q) %s   Hard Acc %.1f  Thru %.0f  Avail %.1f%%  %.1f KiB",
			PrettyCell(l.LiveChamp.ID), l.LiveChamp.Acc, l.LiveChamp.Thru, l.LiveChamp.Avail, l.LiveChamp.RAMKiB))
	}
	if l.LeanChamp.ID != "" {
		d.body(fmt.Sprintf("Lean >=95%% Acc keep (smallest RAM, then fastest Thru): %s  mode %s  Hard Acc %.1f  Acc keep %.0f%%  Thru %.0f  Avail %.1f%%  %.1f KiB  (%d in band)",
			PrettyCell(l.LeanChamp.ID), PrettyMode(l.LeanChamp.Mode), l.LeanChamp.Acc, l.LeanChamp.RelAcc*100,
			l.LeanChamp.Thru, l.LeanChamp.Avail, l.LeanChamp.RAMKiB, len(l.Lean)))
	} else {
		d.body("No lean cell yet — need Hard Acc >=95% of Acc champ, then pick smallest RAM / fastest Thru.")
	}
	if l.GoldStd.ID != "" {
		d.body(fmt.Sprintf("Gold-std (2+ pillars, Acc keep >= 80%%, then smallest then fastest): %s  mode %s  Hard Acc %.1f  Acc keep %.0f%%  Thru %.0f  %.1f KiB",
			PrettyCell(l.GoldStd.ID), PrettyMode(l.GoldStd.Mode), l.GoldStd.Acc, l.GoldStd.RelAcc*100, l.GoldStd.Thru, l.GoldStd.RAMKiB))
	}
	if l.FastID != "" {
		d.body(fmt.Sprintf("Board fastest %s  Thru %.0f (traps may own this)     Best availability %s  %.1f%%",
			PrettyCell(l.FastID), l.FastThru, PrettyCell(l.AvailID), l.BestAvail))
		d.body(fmt.Sprintf("Learner Thru peak %.0f   learner Avail peak %.1f%%   LPD board n=%d", l.PeakThru, l.PeakAvail, l.N))
	}
	d.lpdRadars(l)
	if len(l.Gold) > 0 {
		d.h2("Gold — trifecta >=80% Acc/Thru/Avail keep at <=20% of Acc-champ RAM")
		d.lpdTable(l.Gold, 16)
	} else {
		d.body("No gold cell yet. Need all three pillars at 80% of learner peaks in one fifth of Acc-champ RAM.")
	}
	if len(l.GoldModes) > 0 {
		d.h2("Gold-standard train modes — 2+ pillars, Acc keep >= 80%, smallest then fastest")
		d.lpdModeTable(l.GoldModes)
	}
	if len(l.Lean) > 0 {
		d.h2("Lean >=95% Acc keep — sacrifice peak Acc for footprint (smallest RAM first)")
		d.lpdTable(l.Lean, 20)
	}
	if len(l.LeanByArch) > 0 {
		d.h2("Lean winners by arch / cam — Acc keep >=95%, then smallest KiB")
		d.lpdModeTable(l.LeanByArch)
	}
	if len(l.Near) > 0 {
		d.h2("Near-gold — 2+ pillars, RAM <= 50% of Acc champ")
		d.lpdTable(l.Near, 12)
	}
	d.h2("Lucy density (LPD) — Q x shrink vs Acc-champ RAM; 0 if Acc keep < 70%")
	d.lpdTable(l.Top, 20)
	if len(l.TopMix) > 0 && l.TopMix[0].Mix > 0 {
		d.h2("Consciousness rank — Q = geomean Acc-keep / Thru-keep / Avail-keep")
		d.lpdMobileTable(l.TopMix, 10)
	}
	if len(l.Trap) > 0 {
		d.h2("Trap — tiny RAM, Acc keep < 70% (Score/MiB and SoftAcc Score lie here)")
		d.lpdTable(l.Trap, 10)
	}
	scatterRows := l.Pool
	if len(scatterRows) == 0 {
		scatterRows = l.Top
	}
	d.scatterLPD("Acc keep % vs RAM KiB (gold = high Acc keep, low RAM; lean = Acc keep >=95%)", "RAM KiB", "Acc keep % vs Acc champ", scatterRows,
		func(r LPDRow) float64 { return r.RAMKiB },
		func(r LPDRow) float64 { return r.RelAcc * 100 },
		l.AccChamp.RAMKiB, true)
	d.scatterLPD("Q% vs RAM KiB (gold = high consciousness, low RAM)", "RAM KiB", "Q %", scatterRows,
		func(r LPDRow) float64 { return r.RAMKiB },
		func(r LPDRow) float64 { return r.Q * 100 },
		l.AccChamp.RAMKiB, false)
	d.scatterLPD("Q% vs shrink vs Acc champ (gold = upper right)", "times smaller than Acc champ", "Q %", scatterRows,
		func(r LPDRow) float64 { return r.Shrink },
		func(r LPDRow) float64 { return r.Q * 100 },
		0, false)
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
		for _, src := range [][]LPDRow{l.Pool, l.Top, l.Gold, l.Near, l.Lean, l.Trap} {
			for _, r := range src {
				if r.ID == id {
					return r
				}
			}
		}
		if l.GoldStd.ID == id {
			return l.GoldStd
		}
		if l.LeanChamp.ID == id {
			return l.LeanChamp
		}
		return LPDRow{}
	}
	synth := func(c LPDChamp) LPDRow {
		if c.ID == "" {
			return LPDRow{}
		}
		if r := find(c.ID); r.ID != "" {
			return r
		}
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
			Acc: c.Acc, Thru: c.Thru, Avail: c.Avail, RAMKiB: c.RAMKiB,
			RelAcc: ra, RelThru: rt, RelAvail: rv,
			DensAcc: da, DensThru: dt, DensAvail: dv, Shrink: shrink,
		}
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
	add(&live, "Acc champ", 230, 179, 90, synth(l.AccChamp), false)
	add(&live, "Lean >=95%", 158, 206, 106, l.LeanChamp, false)
	add(&live, "Gold-std", 183, 121, 31, l.GoldStd, false)
	add(&live, "Live-fit", 61, 214, 198, synth(l.LiveChamp), false)
	add(&live, "Lucy Score", 224, 108, 117, synth(l.Champ), false)
	seen = map[string]bool{}
	add(&dens, "Lean >=95%", 158, 206, 106, l.LeanChamp, true)
	add(&dens, "Gold-std", 183, 121, 31, l.GoldStd, true)
	if len(l.Top) > 0 && l.Top[0].LPD > 0 {
		add(&dens, "LPD lead", 61, 214, 198, l.Top[0], true)
	}
	add(&dens, "Acc champ", 230, 179, 90, synth(l.AccChamp), true)
	add(&dens, "Lucy Score", 224, 108, 117, synth(l.Champ), true)
	d.radar("Consciousness radar — Acc keep / Thru keep / Avail keep vs learner peaks", live)
	d.radar("Memory density radar — keep x shrink vs Acc champ (traps sit at origin)", dens)
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
	d.table([]string{"Band", "Cell", "Acc keep%", "Thru%", "Avail%", "Q%", "RAM%", "xSmall", "LPD", "Hard Acc", "Thru", "KiB"}, func(k int) []string {
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
	d.table([]string{"Mode / arch", "n", "Hard Acc", "KiB", "Thru", "Cell"}, func(k int) []string {
		if k >= len(modes) {
			return nil
		}
		m := modes[k]
		cell := CompactCell(m.Smallest)
		fast := CompactCell(m.Fastest)
		if fast != "" && fast != cell {
			cell += " / " + fast
		}
		label := PrettyMode(m.Mode)
		if label == m.Mode && strings.Contains(strings.ToLower(m.Mode), "cam") {
			label = PrettyArch(m.Mode)
		}
		return []string{
			label,
			fmt.Sprintf("%d", m.N),
			fmt.Sprintf("%.1f", m.BestAcc),
			fmt.Sprintf("%.1f", m.MinRAM),
			fmt.Sprintf("%.0f", m.MaxThru),
			cell,
		}
	})
}

func (d *doc) lpdMobileTable(rows []LPDRow, max int) {
	d.table([]string{"Band", "Cell", "Acc keep%", "Thru%", "Avail%", "Q%", "LPD", "Thru", "Avail"}, func(k int) []string {
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

func (d *doc) scatterLPD(title, xlab, ylab string, rows []LPDRow, xof, yof func(LPDRow) float64, accChampRAM float64, accKeepAxis bool) {
	pts := make([]CellPoint, 0, len(rows))
	for _, r := range rows {
		band := r.Band
		if r.Gold {
			band = "gold"
		} else if r.RelAcc >= LPDLeanKeep {
			band = "lean"
		}
		pts = append(pts, CellPoint{Arch: band, Mode: r.Mode, Avail: xof(r), Acc: yof(r)})
	}
	if len(pts) < 2 {
		return
	}
	d.scatterGuides(title, xlab, ylab, pts,
		func(p CellPoint) float64 { return p.Avail },
		func(p CellPoint) float64 { return p.Acc },
		accChampRAM, accKeepAxis)
}

func zipKV(keys []string, vals []float64) []kv {
	n := len(keys)
	if len(vals) < n {
		n = len(vals)
	}
	out := make([]kv, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, kv{PrettyMode(keys[i]), vals[i]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Val > out[j].Val })
	return out
}

func (d *doc) heatmap(title string, rows, cols []string, grid [][]float64) {
	rows, cols = PrettyModes(rows), PrettyModes(cols)
	if len(rows) == 0 || len(cols) == 0 || len(grid) == 0 {
		return
	}
	const maxCols = 12
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

func (d *doc) heatmapSigned(title string, rows, cols []string, grid [][]float64, hit [][]bool) {
	rows, cols = PrettyModes(rows), PrettyModes(cols)
	if len(rows) == 0 || len(cols) == 0 || len(grid) == 0 {
		return
	}
	const maxCols = 12
	for start := 0; start < len(cols); start += maxCols {
		end := start + maxCols
		if end > len(cols) {
			end = len(cols)
		}
		subCols := cols[start:end]
		sub := make([][]float64, len(rows))
		subHit := make([][]bool, len(rows))
		for i := range rows {
			row := make([]float64, len(subCols))
			hs := make([]bool, len(subCols))
			if i < len(grid) {
				for j := start; j < end && j < len(grid[i]); j++ {
					row[j-start] = grid[i][j]
					if i < len(hit) && j < len(hit[i]) {
						hs[j-start] = hit[i][j]
					}
				}
			}
			sub[i] = row
			subHit[i] = hs
		}
		label := title
		if len(cols) > maxCols {
			label = fmt.Sprintf("%s  (cols %d-%d)", title, start+1, end)
		}
		d.heatmapSignedPage(label, rows, subCols, sub, subHit)
	}
}

func (d *doc) heatColHeads(left, labelW, cw, headH float64, cols []string) {
	y0 := d.pdf.GetY()
	d.pdf.SetFont("Helvetica", "", 5)
	d.pdf.SetTextColor(90, 100, 110)
	for j, c := range cols {
		x := left + labelW + float64(j)*cw + 1.2
		d.pdf.TransformBegin()
		d.pdf.TransformRotate(90, x, y0+headH)
		d.pdf.SetXY(x, y0+headH)
		d.pdf.CellFormat(headH, 3.2, latin(c), "", 0, "L", false, 0, "")
		d.pdf.TransformEnd()
	}
	d.pdf.SetY(y0 + headH)
}

func (d *doc) heatmapSignedPage(title string, rows, cols []string, grid [][]float64, hit [][]bool) {
	d.h2(title)
	const headH = 34.0
	need := headH + 8.0 + float64(len(rows))*5.2
	if d.pdf.GetY()+need > d.pageBreakY()-8 {
		d.pdf.AddPage()
		d.h2(title)
	}
	left := 18.0
	labelW := 46.0
	avail := 174.0 - labelW
	cw := avail / float64(len(cols))
	if cw > 12 {
		cw = 12
	}
	rh := 5.0
	maxAbs := 1.0
	for i := range grid {
		for j, v := range grid[i] {
			if i < len(hit) && j < len(hit[i]) && !hit[i][j] {
				continue
			}
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}
	}
	d.heatColHeads(left, labelW, cw, headH, cols)
	for i, r := range rows {
		y := d.pdf.GetY()
		if y > d.pageBreakY() {
			d.pdf.AddPage()
			d.heatColHeads(left, labelW, cw, headH, cols)
			y = d.pdf.GetY()
		}
		d.pdf.SetFont("Helvetica", "", 5)
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(labelW, rh, d.fitText(r, labelW), "", 0, "L", false, 0, "")
		for j := range cols {
			present := i < len(hit) && j < len(hit[i]) && hit[i][j]
			v := 0.0
			if i < len(grid) && j < len(grid[i]) {
				v = grid[i][j]
			}
			x := left + labelW + float64(j)*cw
			if !present {
				d.pdf.SetFillColor(36, 48, 56)
				d.pdf.Rect(x, y, cw-0.3, rh-0.3, "F")
				continue
			}
			t := 0.5 + 0.5*(v/maxAbs)
			cr, cg, cb := heatRGB(t)
			d.pdf.SetFillColor(cr, cg, cb)
			d.pdf.Rect(x, y, cw-0.3, rh-0.3, "F")
			d.pdf.SetFont("Helvetica", "", 5)
			if tlight(cr, cg, cb) {
				d.pdf.SetTextColor(20, 24, 28)
			} else {
				d.pdf.SetTextColor(240, 244, 246)
			}
			d.pdf.SetXY(x, y)
			d.pdf.CellFormat(cw-0.3, rh-0.3, fmt.Sprintf("%+.0f", v), "", 0, "C", false, 0, "")
		}
		d.pdf.SetY(y + rh)
	}
	d.gap(3)
}

func (d *doc) heatmapPage(title string, rows, cols []string, grid [][]float64) {
	d.h2(title)
	const headH = 34.0
	need := headH + 8.0 + float64(len(rows))*5.2
	if d.pdf.GetY()+need > d.pageBreakY()-8 {
		d.pdf.AddPage()
		d.h2(title)
	}
	left := 18.0
	labelW := 46.0
	avail := 174.0 - labelW
	cw := avail / float64(len(cols))
	if cw > 12 {
		cw = 12
	}
	rh := 5.0
	min, max := heatRange(grid)
	d.heatColHeads(left, labelW, cw, headH, cols)
	for i, r := range rows {
		y := d.pdf.GetY()
		if y > d.pageBreakY() {
			d.pdf.AddPage()
			d.heatColHeads(left, labelW, cw, headH, cols)
			y = d.pdf.GetY()
		}
		d.pdf.SetFont("Helvetica", "", 5)
		d.pdf.SetTextColor(40, 50, 55)
		d.pdf.SetXY(left, y)
		d.pdf.CellFormat(labelW, rh, d.fitText(r, labelW), "", 0, "L", false, 0, "")
		for j := range cols {
			cr, cg, cb := 36, 48, 56 // missing = dark hole (not in sample / not in plan)
			present := i < len(grid) && j < len(grid[i]) && !math.IsNaN(grid[i][j])
			v := 0.0
			if present {
				v = grid[i][j]
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
			if !present {
				d.pdf.CellFormat(cw-0.3, rh-0.3, "·", "", 0, "C", false, 0, "")
			} else {
				d.pdf.CellFormat(cw-0.3, rh-0.3, fmt.Sprintf("%.0f", v), "", 0, "C", false, 0, "")
			}
		}
		d.pdf.SetY(y + rh)
	}
	d.gap(3)
}

func hasVal(grid [][]float64, i, j int) bool {
	return i < len(grid) && j < len(grid[i]) && !math.IsNaN(grid[i][j])
}

func heatRange(grid [][]float64) (min, max float64) {
	min, max = math.Inf(1), math.Inf(-1)
	any := false
	for _, row := range grid {
		for _, v := range row {
			if math.IsNaN(v) {
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
	d.scatterGuides(title, xlab, ylab, pts, xof, yof, 0, false)
}

func (d *doc) scatterGuides(title, xlab, ylab string, pts []CellPoint, xof, yof func(CellPoint) float64, accChampRAM float64, accKeepAxis bool) {
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
	// Acc-keep charts: pin Y to 0–100 so 80%/95% guides are visible.
	if accKeepAxis {
		if ymin > 0 {
			ymin = 0
		}
		if ymax < 100 {
			ymax = 100
		}
	}
	d.pdf.SetDrawColor(40, 55, 65)
	d.pdf.Rect(x0, y0, w, h, "D")
	X := func(v float64) float64 { return x0 + w*(v-xmin)/(xmax-xmin) }
	Y := func(v float64) float64 { return y0 + h - h*(v-ymin)/(ymax-ymin) }
	// Gold / lean guides for Acc-keep vs RAM.
	if accChampRAM > 0 && xlab == "RAM KiB" {
		goldRAM := accChampRAM * LPDGoldRAM
		if goldRAM >= xmin && goldRAM <= xmax {
			d.pdf.SetDrawColor(183, 121, 31)
			d.pdf.SetDashPattern([]float64{1.5, 1.2}, 0)
			d.pdf.Line(X(goldRAM), y0, X(goldRAM), y0+h)
			d.pdf.SetDashPattern([]float64{}, 0)
			d.pdf.SetFont("Helvetica", "", 5)
			d.pdf.SetTextColor(183, 121, 31)
			d.pdf.SetXY(X(goldRAM)+1, y0+1)
			d.pdf.CellFormat(40, 3, "20% Acc-champ RAM", "", 0, "L", false, 0, "")
		}
	}
	if accKeepAxis {
		drawHY := func(pct float64, r, g, b int, lab string) {
			if pct < ymin || pct > ymax {
				return
			}
			d.pdf.SetDrawColor(r, g, b)
			d.pdf.SetDashPattern([]float64{1.5, 1.2}, 0)
			d.pdf.Line(x0, Y(pct), x0+w, Y(pct))
			d.pdf.SetDashPattern([]float64{}, 0)
			d.pdf.SetFont("Helvetica", "", 5)
			d.pdf.SetTextColor(r, g, b)
			d.pdf.SetXY(x0+1, Y(pct)-3)
			d.pdf.CellFormat(40, 3, lab, "", 0, "L", false, 0, "")
		}
		drawHY(LPDGoldKeep*100, 183, 121, 31, "80% Acc keep")
		drawHY(LPDLeanKeep*100, 80, 160, 90, "lean 95% Acc keep")
	}
	const maxDots = 1500
	step := 1
	if len(pts) > maxDots {
		step = (len(pts) + maxDots - 1) / maxDots
	}
	for i := 0; i < len(pts); i += step {
		p := pts[i]
		px, py := X(xof(p)), Y(yof(p))
		r, g, b := archColor(p.Arch)
		d.pdf.SetFillColor(r, g, b)
		sz := 1.1
		if p.Arch == "gold" || p.Arch == "lean" {
			sz = 1.6
		}
		d.pdf.Rect(px-sz/2, py-sz/2, sz, sz, "F")
	}
	front := paretoFront(pts, xof, yof)
	d.pdf.SetDrawColor(183, 121, 31)
	d.pdf.SetLineWidth(0.35)
	for i := 1; i < len(front); i++ {
		d.pdf.Line(X(xof(front[i-1])), Y(yof(front[i-1])), X(xof(front[i])), Y(yof(front[i])))
	}
	d.pdf.SetLineWidth(0.2)
	d.pdf.SetFont("Helvetica", "", 7)
	d.pdf.SetTextColor(110, 120, 130)
	d.pdf.SetXY(x0, y0+h+1)
	note := "gold=Pareto  teal=single  blue=bi  purple=tri"
	if accKeepAxis {
		note = "gold ring band / lean>=95%  dashed=20% RAM + 80/95% Acc keep"
	}
	d.pdf.CellFormat(w, 4, latin(fmt.Sprintf("%s  →    n=%d   %s", xlab, len(pts), note)), "", 1, "L", false, 0, "")
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
	case strings.Contains(s, "lean"):
		return 80, 160, 90
	case strings.Contains(s, "trap"):
		return 197, 48, 48
	case strings.Contains(s, "near"):
		return 230, 179, 90
	case strings.Contains(s, "keep"):
		return 61, 214, 198
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
