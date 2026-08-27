package report

import (
	"fmt"
	"strings"
)

// PDFCompare renders a machine × LR comparison report.
func PDFCompare(c CompareReport) ([]byte, error) {
	p := newDoc("compare — " + nz(c.Title, "machines"))
	p.h1("compare  " + nz(c.Title, "machines × LR"))
	p.muted("Matched cells share the same layer|dtype|mode recipe; LR comes from the |lr=… suffix in test53 cell IDs.")
	p.body(fmt.Sprintf("Generated %s. Machines: %s. LR steps: %d.",
		c.Generated.Format("2006-01-02 15:04:05"), strings.Join(c.Machines, ", "), len(c.LRs)))
	p.gap(2)

	p.h2("Charts")
	p.compareCharts(c)
	p.gap(2)

	if len(c.CamPairs) > 0 {
		p.h2("Cam compare")
		for _, pair := range c.CamPairs {
			p.compareCamPairSection(pair, c)
			p.gap(2)
		}
	}

	p.h2("Summary tables")
	p.body("Mean Acc vs LR (by machine)")
	p.compareSummaryTable(c)
	p.gap(2)

	p.h2("Recipe wins per LR")
	p.compareWinsTable(c)
	p.gap(2)

	for _, g := range c.VsBaseline {
		p.h2("vs " + g.Baseline + " tables — " + g.Machine)
		p.body(g.Baseline + " baseline mean Acc vs LR:")
		p.compareBaselineLRTable(g)
		p.body("Δ Acc vs " + g.Baseline + ":")
		p.compareVsDeltaHeatTable(g.Modes, g.LRLabels, g.AccDelta)
		p.gap(2)
	}

	for _, g := range c.StepFamilies {
		for _, v := range c.StepVerdicts {
			if v.Machine == g.Machine {
				p.h2("Step vs plain verdict — " + g.Machine)
				p.body(v.Headline)
				for _, b := range v.Bullets {
					p.body("• " + b)
				}
				p.gap(1)
				break
			}
		}
		p.h2("Step vs plain families — " + g.Machine)
		p.muted("Verdict per row: Acc / Score / Avail. step-better LR = use Step* at those learning rates.")
		p.compareStepFamilyTable(g)
		p.gap(2)
	}

	if len(c.Matched) > 0 {
		p.h2("Top matched recipe deltas (Acc A−B)")
		p.compareMatchTable(c)
		p.gap(2)
	} else if c.OverlapNote != "" {
		p.h2("Matched recipes")
		p.body(c.OverlapNote)
		p.gap(2)
	}

	if len(c.LPDTop) > 0 {
		p.h2("Top Lucy LPD (keepers — all machines)")
		p.compareLPDTable(c.LPDTop)
		p.gap(2)
	}
	for _, g := range c.LPDPerMachine {
		if len(g.Rows) == 0 {
			continue
		}
		p.h2("Top LPD — " + g.Machine)
		p.compareLPDTable(g.Rows)
		p.gap(2)
	}
	if len(c.TrapTop) > 0 {
		p.h2("Top traps & soft-gap false negatives")
		p.compareTrapTable(c.TrapTop)
		p.gap(2)
	}
	if len(c.TrapRate) > 0 {
		p.h2("Trap rate by machine × LR")
		p.compareTrapRateTable(c.TrapRate)
		p.gap(2)
	}
	if len(c.ModeBars) > 0 {
		p.h2("Mean Acc by train mode (all LRs pooled)")
		p.compareModeBarTable(c)
	}
	return p.bytes()
}

func (d *doc) compareSummaryTable(c CompareReport) {
	if len(c.Summary) == 0 {
		d.body("No LR-tagged cells yet.")
		return
	}
	type row struct {
		lbl string
		lr  float64
		val map[string]CompareLRRow
	}
	byLR := map[float64]*row{}
	var order []float64
	for _, s := range c.Summary {
		r := byLR[s.LR]
		if r == nil {
			r = &row{lbl: s.LRLabel, lr: s.LR, val: map[string]CompareLRRow{}}
			byLR[s.LR] = r
			order = append(order, s.LR)
		}
		r.val[s.Machine] = s
	}
	sortFloat64s(order)
	headers := []string{"LR", "n"}
	for _, m := range c.Machines {
		headers = append(headers, m+" Acc", m+" Score")
	}
	d.table(headers, func(i int) []string {
		if i >= len(order) {
			return nil
		}
		r := byLR[order[i]]
		n := 0
		for _, m := range c.Machines {
			if s, ok := r.val[m]; ok && s.N > n {
				n = s.N
			}
		}
		cols := []string{r.lbl, fmt.Sprintf("%d", n)}
		for _, m := range c.Machines {
			if s, ok := r.val[m]; ok {
				cols = append(cols, fmt.Sprintf("%.1f", s.MeanAcc), fmt.Sprintf("%.0f", s.MeanScore))
			} else {
				cols = append(cols, "—", "—")
			}
		}
		return cols
	})
}

func (d *doc) compareWinsTable(c CompareReport) {
	if len(c.Wins) == 0 {
		d.body("No wins yet.")
		return
	}
	headers := append([]string{"LR"}, c.Machines...)
	d.table(headers, func(i int) []string {
		if i >= len(c.Wins) {
			return nil
		}
		w := c.Wins[i]
		cols := []string{w.LRLabel}
		for _, m := range c.Machines {
			cols = append(cols, fmt.Sprintf("%d", w.Wins[m]))
		}
		return cols
	})
}

func (d *doc) compareHeatTable(rows, cols []string, grid [][]float64) {
	if len(rows) == 0 || len(cols) == 0 {
		d.body("No data.")
		return
	}
	headers := append([]string{"mode"}, cols...)
	d.table(headers, func(i int) []string {
		if i >= len(rows) {
			return nil
		}
		colsOut := []string{PrettyMode(rows[i])}
		for j := range cols {
			v := 0.0
			if i < len(grid) && j < len(grid[i]) {
				v = grid[i][j]
			}
			if v == 0 {
				colsOut = append(colsOut, "")
			} else {
				colsOut = append(colsOut, fmt.Sprintf("%.1f", v))
			}
		}
		return colsOut
	})
}

func (d *doc) compareMatchTable(c CompareReport) {
	headers := []string{"LR", "cell"}
	for _, m := range c.Machines {
		headers = append(headers, m+" Acc")
	}
	if len(c.Machines) >= 2 {
		headers = append(headers, "delta", "winner")
	}
	d.table(headers, func(i int) []string {
		if i >= len(c.Matched) {
			return nil
		}
		r := c.Matched[i]
		cols := []string{r.LRLabel, clipStr(r.Recipe, 42)}
		for _, m := range c.Machines {
			if v, ok := r.ByMachine[m]; ok {
				cols = append(cols, fmt.Sprintf("%.1f", v))
			} else {
				cols = append(cols, "—")
			}
		}
		if len(c.Machines) >= 2 {
			cols = append(cols, fmt.Sprintf("%+.1f", r.Delta), r.Winner)
		}
		return cols
	})
}

func sortFloat64s(xs []float64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func (d *doc) compareLPDTable(rows []CompareLPDEntry) {
	headers := []string{"#", "machine", "LR", "LPD", "Acc", "Score", "Avail", "RAM", "band", "cell"}
	d.table(headers, func(i int) []string {
		if i >= len(rows) {
			return nil
		}
		r := rows[i]
		return []string{
			fmt.Sprintf("%d", r.Rank), r.Machine, r.LRLabel,
			fmt.Sprintf("%.3f", r.LPD), fmt.Sprintf("%.1f", r.Acc), fmt.Sprintf("%.0f", r.Score),
			fmt.Sprintf("%.1f", r.Avail), fmt.Sprintf("%.0f", r.RAMKiB), r.Band, clipStr(r.ID, 36),
		}
	})
}

func (d *doc) compareTrapTable(rows []CompareTrapEntry) {
	headers := []string{"#", "machine", "LR", "band", "Score", "Acc", "Soft", "gap", "Thru", "Avail", "cell"}
	d.table(headers, func(i int) []string {
		if i >= len(rows) {
			return nil
		}
		r := rows[i]
		return []string{
			fmt.Sprintf("%d", r.Rank), r.Machine, r.LRLabel, r.Band,
			fmt.Sprintf("%.0f", r.Score), fmt.Sprintf("%.1f", r.Acc), fmt.Sprintf("%.1f", r.Soft),
			fmt.Sprintf("%.1f", r.SoftGap), fmt.Sprintf("%.0f", r.Thru), fmt.Sprintf("%.1f", r.Avail),
			clipStr(r.ID, 32),
		}
	})
}

func (d *doc) compareTrapRateTable(rows []CompareTrapRateRow) {
	headers := []string{"machine", "LR", "n", "traps", "trap%", "soft-gap n"}
	d.table(headers, func(i int) []string {
		if i >= len(rows) {
			return nil
		}
		r := rows[i]
		return []string{
			r.Machine, r.LRLabel, fmt.Sprintf("%d", r.N), fmt.Sprintf("%d", r.TrapN),
			fmt.Sprintf("%.1f", r.TrapPct), fmt.Sprintf("%d", r.SoftGapN),
		}
	})
}

func (d *doc) compareBaselineLRTable(g CompareVsBaselineGrid) {
	headers := append([]string{"LR"}, g.Baseline+" Acc", g.Baseline+" Avail")
	d.table(headers, func(i int) []string {
		if i >= len(g.LRs) {
			return nil
		}
		acc := g.BaselineAcc[i]
		avail := g.BaselineAvail[i]
		if acc == 0 && avail == 0 {
			return nil
		}
		lbl := g.LRLabels[i]
		if lbl == "" {
			lbl = FormatLR(g.LRs[i])
		}
		return []string{lbl, fmt.Sprintf("%.1f", acc), fmt.Sprintf("%.1f", avail)}
	})
}

func (d *doc) compareVsDeltaHeatTable(rows, cols []string, grid [][]float64) {
	if len(rows) == 0 || len(cols) == 0 {
		d.body("No matched baseline pairs yet.")
		return
	}
	headers := append([]string{"mode"}, cols...)
	d.table(headers, func(i int) []string {
		if i >= len(rows) {
			return nil
		}
		colsOut := []string{rows[i]}
		for j := range cols {
			v := 0.0
			if i < len(grid) && j < len(grid[i]) {
				v = grid[i][j]
			}
			if v == 0 {
				colsOut = append(colsOut, "")
			} else {
				colsOut = append(colsOut, fmt.Sprintf("%+.1f", v))
			}
		}
		return colsOut
	})
}

func (d *doc) compareStepFamilyTable(g CompareStepFamilyGrid) {
	if len(g.Pairs) == 0 {
		d.body("No Step* / plain pairs found on this machine.")
		return
	}
	headers := []string{"Step", "plain", "overall", "Acc", "Score", "Avail", "ΔAcc", "step LRs", "plain LRs", "n"}
	d.table(headers, func(i int) []string {
		if i >= len(g.Pairs) {
			return nil
		}
		p := g.Pairs[i]
		if p.MatchN == 0 {
			return nil
		}
		return []string{
			p.Step, p.Plain,
			stepVerdictLabel(p.OverallVerdict),
			stepVerdictLabel(p.AccVerdict),
			stepVerdictLabel(p.ScoreVerdict),
			stepVerdictLabel(p.AvailVerdict),
			fmt.Sprintf("%+.2f", p.PooledAccDelta),
			strings.Join(p.StepBetterLRs, ", "),
			strings.Join(p.PlainBetterLRs, ", "),
			fmt.Sprintf("%d", p.MatchN),
		}
	})
	d.body("Δ Acc by LR (step − plain):")
	rows := make([]string, 0, len(g.Pairs))
	grid := make([][]float64, 0, len(g.Pairs))
	for _, p := range g.Pairs {
		if p.MatchN == 0 {
			continue
		}
		rows = append(rows, p.Step+"−"+p.Plain)
		grid = append(grid, p.AccDelta)
	}
	d.compareVsDeltaHeatTable(rows, g.LRLabels, grid)
}

func (d *doc) compareModeBarTable(c CompareReport) {
	headers := []string{"mode"}
	for _, m := range c.Machines {
		headers = append(headers, m+" Acc")
	}
	d.table(headers, func(i int) []string {
		if i >= len(c.ModeBars) {
			return nil
		}
		b := c.ModeBars[i]
		cols := []string{PrettyMode(b.Mode)}
		for _, m := range c.Machines {
			if v, ok := b.ByMachine[m]; ok {
				cols = append(cols, fmt.Sprintf("%.1f", v))
			} else {
				cols = append(cols, "—")
			}
		}
		return cols
	})
}

func (d *doc) compareCamPairSection(p CompareCamPair, c CompareReport) {
	band := bandSuffix(p.Band)
	d.h2("Cam " + p.Other + " vs " + p.Base + band)
	d.body(p.Headline)
	if len(p.ByLR) > 0 {
		d.body("Pooled Δ on matched recipes (other − base):")
		headers := []string{"LR", "n", "Δ Acc", "Δ Score", "wins other", "wins base"}
		d.table(headers, func(i int) []string {
			if i >= len(p.ByLR) {
				return nil
			}
			r := p.ByLR[i]
			return []string{
				r.LRLabel,
				fmt.Sprintf("%d", r.N),
				fmt.Sprintf("%+.2f", r.MeanAccDelta),
				fmt.Sprintf("%+.0f", r.MeanScoreDelta),
				fmt.Sprintf("%d", r.WinsOther),
				fmt.Sprintf("%d", r.WinsBase),
			}
		})
	}
	if len(p.Matched) > 0 {
		d.gap(1)
		d.body("Top matched recipe deltas:")
		headers := []string{"LR", "recipe", p.Base, p.Other, "Δ", "winner"}
		d.table(headers, func(i int) []string {
			if i >= len(p.Matched) {
				return nil
			}
			r := p.Matched[i]
			return []string{
				r.LRLabel,
				CompactCell(r.Recipe),
				fmt.Sprintf("%.1f", r.ByMachine[p.Base]),
				fmt.Sprintf("%.1f", r.ByMachine[p.Other]),
				fmt.Sprintf("%+.1f", r.Delta),
				r.Winner,
			}
		})
	}
	_ = c
}
