package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// CompareStepFamilyGrid is Step* vs matched plain-mode deltas on one machine across LR.
type CompareStepFamilyGrid struct {
	Machine  string                  `json:"machine"`
	LRLabels []string                `json:"lr_labels"`
	LRs      []float64               `json:"lrs"`
	Pairs    []CompareStepFamilyPair `json:"pairs"`
}

// CompareStepFamilyPair is one Step* vs non-Step family at each LR (step − plain).
type CompareStepFamilyPair struct {
	Step            string    `json:"step"`
	Plain           string    `json:"plain"`
	AccDelta        []float64 `json:"acc_delta"`
	ScoreDelta      []float64 `json:"score_delta"`
	AvailDelta      []float64 `json:"avail_delta"`
	N               []int     `json:"n"`
	MeanAbsAcc      float64   `json:"mean_abs_acc"`
	PooledAccDelta  float64   `json:"pooled_acc_delta"`
	PooledScoreDelta float64  `json:"pooled_score_delta"`
	PooledAvailDelta float64  `json:"pooled_avail_delta"`
	AccWinPct       float64   `json:"acc_win_pct"`
	ScoreWinPct     float64   `json:"score_win_pct"`
	AvailWinPct     float64   `json:"avail_win_pct"`
	StepWins        int       `json:"step_wins"`
	PlainWins       int       `json:"plain_wins"`
	Ties            int       `json:"ties"`
	MatchN          int       `json:"match_n"`
	AccVerdict      string    `json:"acc_verdict"`
	ScoreVerdict    string    `json:"score_verdict"`
	AvailVerdict    string    `json:"avail_verdict"`
	OverallVerdict  string    `json:"overall_verdict"`
	StepBetterLRs   []string  `json:"step_better_lrs,omitempty"`
	PlainBetterLRs  []string  `json:"plain_better_lrs,omitempty"`
	Collapse        bool      `json:"collapse"`
}

// CompareStepMachineVerdict is a plain-English summary for one machine.
type CompareStepMachineVerdict struct {
	Machine          string   `json:"machine"`
	MatchN           int      `json:"match_n"`
	PooledAccDelta   float64  `json:"pooled_acc_delta"`
	PooledScoreDelta float64  `json:"pooled_score_delta"`
	PooledAvailDelta float64  `json:"pooled_avail_delta"`
	AccWinner        string   `json:"acc_winner"`
	ScoreWinner      string   `json:"score_winner"`
	AvailWinner      string   `json:"avail_winner"`
	CollapseFamilies int      `json:"collapse_families"`
	StepWinsFamilies int      `json:"step_wins_families"`
	PlainWinsFamilies int     `json:"plain_wins_families"`
	MixedFamilies    int      `json:"mixed_families"`
	Headline         string   `json:"headline"`
	Bullets          []string `json:"bullets"`
	StepWinsAtLR     []string `json:"step_wins_at_lr,omitempty"`
	PlainWinsAtLR    []string `json:"plain_wins_at_lr,omitempty"`
	BestStepFamilies []string `json:"best_step_families,omitempty"`
	WorstStepFamilies []string `json:"worst_step_families,omitempty"`
}

// CompareStepLRRow is aggregate step−plain delta at one LR across all families.
type CompareStepLRRow struct {
	Machine          string  `json:"machine"`
	LR               float64 `json:"lr"`
	LRLabel          string  `json:"lr_label"`
	N                int     `json:"n"`
	MeanAccDelta     float64 `json:"mean_acc_delta"`
	MeanScoreDelta   float64 `json:"mean_score_delta"`
	MeanAvailDelta   float64 `json:"mean_avail_delta"`
	AccWinner        string  `json:"acc_winner"`
	FamiliesStepWin  int     `json:"families_step_win"`
	FamiliesPlainWin int     `json:"families_plain_win"`
}

// CompareStepCross compares step−plain Acc delta for one family across machines × LR.
type CompareStepCross struct {
	Step   string                `json:"step"`
	Plain  string                `json:"plain"`
	Points []CompareStepCrossPt  `json:"points"`
}

// CompareStepCrossPt is step−plain delta on one machine at one LR.
type CompareStepCrossPt struct {
	Machine    string  `json:"machine"`
	LR         float64 `json:"lr"`
	LRLabel    string  `json:"lr_label"`
	N          int     `json:"n"`
	AccDelta   float64 `json:"acc_delta"`
	ScoreDelta float64 `json:"score_delta"`
	AvailDelta float64 `json:"avail_delta"`
}

type stepPlainPair struct {
	step, plain string
}

func compareStepFamilies(pts []taggedPoint, machines []string) []CompareStepFamilyGrid {
	var out []CompareStepFamilyGrid
	for _, machine := range machines {
		var mpts []taggedPoint
		for _, p := range pts {
			if p.Machine == machine {
				mpts = append(mpts, p)
			}
		}
		if len(mpts) == 0 {
			continue
		}
		pairs := discoverStepPairs(mpts)
		if len(pairs) == 0 {
			continue
		}
		lrs, lrLabels := lrsForMachine(mpts, machine)
		grid := CompareStepFamilyGrid{Machine: machine, LRs: lrs, LRLabels: lrLabels}
		for _, pr := range pairs {
			grid.Pairs = append(grid.Pairs, buildStepPair(mpts, pr.step, pr.plain, lrs, lrLabels))
		}
		sort.Slice(grid.Pairs, func(i, j int) bool {
			if grid.Pairs[i].MatchN != grid.Pairs[j].MatchN {
				return grid.Pairs[i].MatchN > grid.Pairs[j].MatchN
			}
			return grid.Pairs[i].Step < grid.Pairs[j].Step
		})
		out = append(out, grid)
	}
	return out
}

func compareStepVerdicts(families []CompareStepFamilyGrid) ([]CompareStepMachineVerdict, []CompareStepLRRow) {
	var verdicts []CompareStepMachineVerdict
	var lrRows []CompareStepLRRow
	for _, g := range families {
		verdicts = append(verdicts, buildStepMachineVerdict(g))
		lrRows = append(lrRows, buildStepLRRows(g)...)
	}
	return verdicts, lrRows
}

func buildStepLRRows(g CompareStepFamilyGrid) []CompareStepLRRow {
	var out []CompareStepLRRow
	for i, lr := range g.LRs {
		lbl := g.LRLabels[i]
		if lbl == "" {
			lbl = FormatLR(lr)
		}
		var accN, scoreN, availN int
		var accD, scoreD, availD float64
		stepWin, plainWin := 0, 0
		for _, p := range g.Pairs {
			if i >= len(p.N) || p.N[i] == 0 {
				continue
			}
			n := p.N[i]
			if i < len(p.AccDelta) {
				accD += p.AccDelta[i] * float64(n)
				accN += n
				if p.AccDelta[i] > accWinThresh {
					stepWin++
				} else if p.AccDelta[i] < -accWinThresh {
					plainWin++
				}
			}
			if i < len(p.ScoreDelta) {
				scoreD += p.ScoreDelta[i] * float64(n)
				scoreN += n
			}
			if i < len(p.AvailDelta) {
				availD += p.AvailDelta[i] * float64(n)
				availN += n
			}
		}
		if accN == 0 {
			continue
		}
		row := CompareStepLRRow{
			Machine: g.Machine, LR: lr, LRLabel: lbl, N: accN,
			MeanAccDelta: accD / float64(accN), FamiliesStepWin: stepWin, FamiliesPlainWin: plainWin,
		}
		if scoreN > 0 {
			row.MeanScoreDelta = scoreD / float64(scoreN)
		}
		if availN > 0 {
			row.MeanAvailDelta = availD / float64(availN)
		}
		row.AccWinner = metricWinner(row.MeanAccDelta, accWinThresh)
		out = append(out, row)
	}
	return out
}

func buildStepMachineVerdict(g CompareStepFamilyGrid) CompareStepMachineVerdict {
	v := CompareStepMachineVerdict{Machine: g.Machine}
	if len(g.Pairs) == 0 {
		v.Headline = "No Step* / plain pairs on this machine."
		return v
	}
	var sumAcc, sumScore, sumAvail float64
	var bestName string
	var bestDelta = -1e9
	var worstName string
	var worstDelta = 1e9
	for _, p := range g.Pairs {
		if p.MatchN == 0 {
			continue
		}
		v.MatchN += p.MatchN
		w := float64(p.MatchN)
		sumAcc += p.PooledAccDelta * w
		sumScore += p.PooledScoreDelta * w
		sumAvail += p.PooledAvailDelta * w
		switch p.OverallVerdict {
		case "collapse":
			v.CollapseFamilies++
		case "step":
			v.StepWinsFamilies++
		case "plain":
			v.PlainWinsFamilies++
		default:
			v.MixedFamilies++
		}
		if p.PooledAccDelta > bestDelta {
			bestDelta = p.PooledAccDelta
			bestName = p.Step + " vs " + p.Plain
		}
		if p.PooledAccDelta < worstDelta {
			worstDelta = p.PooledAccDelta
			worstName = p.Step + " vs " + p.Plain
		}
	}
	if v.MatchN == 0 {
		v.Headline = "No matched step/plain cells."
		return v
	}
	wt := float64(v.MatchN)
	v.PooledAccDelta = sumAcc / wt
	v.PooledScoreDelta = sumScore / wt
	v.PooledAvailDelta = sumAvail / wt
	v.AccWinner = metricWinner(v.PooledAccDelta, accWinThresh)
	v.ScoreWinner = metricWinner(v.PooledScoreDelta, scoreWinThresh)
	v.AvailWinner = metricWinner(v.PooledAvailDelta, 0.5)
	v.Headline = stepHeadline(v)
	v.Bullets = stepBullets(g, v, bestName, bestDelta, worstName, worstDelta)
	if bestDelta > accWinThresh {
		v.BestStepFamilies = []string{fmt.Sprintf("%s (+%.1f Acc pooled)", bestName, bestDelta)}
	}
	if worstDelta < -accWinThresh {
		v.WorstStepFamilies = []string{fmt.Sprintf("%s (%.1f Acc pooled)", worstName, worstDelta)}
	}
	for _, row := range buildStepLRRows(g) {
		if row.AccWinner == "step" {
			v.StepWinsAtLR = append(v.StepWinsAtLR, row.LRLabel)
		} else if row.AccWinner == "plain" {
			v.PlainWinsAtLR = append(v.PlainWinsAtLR, row.LRLabel)
		}
	}
	return v
}

func stepHeadline(v CompareStepMachineVerdict) string {
	totalFamilies := v.StepWinsFamilies + v.PlainWinsFamilies + v.MixedFamilies + v.CollapseFamilies
	if totalFamilies > 0 && v.CollapseFamilies == totalFamilies {
		return fmt.Sprintf("%s: stepping ≈ plain everywhere (family collapse — |ΔAcc| tiny).", v.Machine)
	}
	parts := []string{v.Machine + ":"}
	if v.AccWinner == "step" {
		parts = append(parts, fmt.Sprintf("stepping wins Acc (+%.1f pp pooled)", v.PooledAccDelta))
	} else if v.AccWinner == "plain" {
		parts = append(parts, fmt.Sprintf("plain wins Acc (%.1f pp pooled)", v.PooledAccDelta))
	} else {
		parts = append(parts, "Acc is a toss-up between step and plain")
	}
	if v.ScoreWinner == "step" {
		parts = append(parts, fmt.Sprintf("step wins Score (+%.0f)", v.PooledScoreDelta))
	} else if v.ScoreWinner == "plain" {
		parts = append(parts, fmt.Sprintf("plain wins Score (%.0f)", v.PooledScoreDelta))
	}
	if v.AvailWinner == "step" {
		parts = append(parts, fmt.Sprintf("step serves more (+%.1f pp Avail)", v.PooledAvailDelta))
	} else if v.AvailWinner == "plain" {
		parts = append(parts, fmt.Sprintf("plain serves more (%.1f pp Avail)", v.PooledAvailDelta))
	}
	return strings.Join(parts, " · ")
}

func stepBullets(g CompareStepFamilyGrid, v CompareStepMachineVerdict, bestName string, bestDelta float64, worstName string, worstDelta float64) []string {
	var b []string
	b = append(b, fmt.Sprintf("Matched %d cells across %d step/plain families.", v.MatchN, len(g.Pairs)))
	if v.StepWinsFamilies+ v.PlainWinsFamilies+ v.MixedFamilies+ v.CollapseFamilies > 0 {
		b = append(b, fmt.Sprintf("Families: %d step wins Acc · %d plain wins · %d mixed · %d collapsed (scheduler barely matters).",
			v.StepWinsFamilies, v.PlainWinsFamilies, v.MixedFamilies, v.CollapseFamilies))
	}
	if len(v.StepWinsAtLR) > 0 {
		b = append(b, "LR bands where stepping wins Acc (most families): "+strings.Join(v.StepWinsAtLR, ", "))
	}
	if len(v.PlainWinsAtLR) > 0 {
		b = append(b, "LR bands where plain wins Acc: "+strings.Join(v.PlainWinsAtLR, ", "))
	}
	if bestDelta > accWinThresh {
		b = append(b, fmt.Sprintf("Best step win: %s (+%.1f Acc pooled).", bestName, bestDelta))
	}
	if worstDelta < -accWinThresh {
		b = append(b, fmt.Sprintf("Worst step loss: %s (%.1f Acc pooled) — use plain here.", worstName, worstDelta))
	}
	if v.PooledScoreDelta > scoreWinThresh && v.ScoreWinner == "step" {
		b = append(b, fmt.Sprintf("Score/duty clock: stepping ahead (+%.0f pooled) — pipe keeps the cell serving while learning.", v.PooledScoreDelta))
	} else if v.PooledScoreDelta < -scoreWinThresh {
		b = append(b, fmt.Sprintf("Score/duty clock: plain ahead (%.0f) — full-chain updates may block serve less at this LR band.", v.PooledScoreDelta))
	}
	if v.PooledAvailDelta > 0.5 {
		b = append(b, fmt.Sprintf("Availability: stepping +%.1f pp — more time in serve mode.", v.PooledAvailDelta))
	} else if v.PooledAvailDelta < -0.5 {
		b = append(b, fmt.Sprintf("Availability: plain +%.1f pp vs step — pipe may block serve longer.", -v.PooledAvailDelta))
	}
	return b
}

func metricWinner(delta, thresh float64) string {
	if delta > thresh {
		return "step"
	}
	if delta < -thresh {
		return "plain"
	}
	return "tie"
}

func compareStepCross(families []CompareStepFamilyGrid) []CompareStepCross {
	type key struct{ step, plain string }
	by := map[key][]CompareStepCrossPt{}
	for _, g := range families {
		for _, pair := range g.Pairs {
			k := key{step: pair.Step, plain: pair.Plain}
			for i, lr := range g.LRs {
				if i >= len(pair.N) || pair.N[i] == 0 {
					continue
				}
				lbl := g.LRLabels[i]
				if lbl == "" {
					lbl = FormatLR(lr)
				}
				by[k] = append(by[k], CompareStepCrossPt{
					Machine: g.Machine, LR: lr, LRLabel: lbl, N: pair.N[i],
					AccDelta: pair.AccDelta[i], ScoreDelta: pair.ScoreDelta[i], AvailDelta: pair.AvailDelta[i],
				})
			}
		}
	}
	var out []CompareStepCross
	for k, pts := range by {
		sort.Slice(pts, func(i, j int) bool {
			if pts[i].Machine != pts[j].Machine {
				return pts[i].Machine < pts[j].Machine
			}
			return pts[i].LR < pts[j].LR
		})
		out = append(out, CompareStepCross{Step: k.step, Plain: k.plain, Points: pts})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Step != out[j].Step {
			return out[i].Step < out[j].Step
		}
		return out[i].Plain < out[j].Plain
	})
	return out
}

func discoverStepPairs(mpts []taggedPoint) []stepPlainPair {
	have := map[string]string{}
	for _, p := range mpts {
		have[modeKey(p.Mode)] = p.Mode
	}
	var pairs []stepPlainPair
	seen := map[string]bool{}
	for k, stepName := range have {
		plainName, ok := stepMate(k, have)
		if !ok || modeKey(plainName) == k {
			continue
		}
		id := k + "|" + modeKey(plainName)
		if seen[id] {
			continue
		}
		seen[id] = true
		pairs = append(pairs, stepPlainPair{step: stepName, plain: plainName})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].step < pairs[j].step })
	return pairs
}

const stepCollapseAbsAcc = 1.0

func buildStepPair(mpts []taggedPoint, step, plain string, lrs []float64, lrLabels []string) CompareStepFamilyPair {
	stepK, plainK := modeKey(step), modeKey(plain)
	plainByKey := map[string]CellPoint{}
	for _, p := range mpts {
		if modeKey(p.Mode) != plainK {
			continue
		}
		k := compareMatchKey(p)
		prev, ok := plainByKey[k]
		if !ok || p.Acc > prev.Acc {
			plainByKey[k] = p.CellPoint
		}
	}
	type bucket struct {
		n                                        int
		accD, scoreD, availD                     float64
		stepWins, plainWins, tie                   int
		scoreStep, scorePlain                      int
		availStep, availPlain                      int
		absAcc                                     float64
	}
	byLR := map[float64]*bucket{}
	for _, p := range mpts {
		if modeKey(p.Mode) != stepK {
			continue
		}
		b, ok := plainByKey[compareMatchKey(p)]
		if !ok {
			continue
		}
		a := byLR[p.LR]
		if a == nil {
			a = &bucket{}
			byLR[p.LR] = a
		}
		dAcc := p.Acc - b.Acc
		dScore := p.Score - b.Score
		dAvail := p.Avail - b.Avail
		a.n++
		a.accD += dAcc
		a.scoreD += dScore
		a.availD += dAvail
		a.absAcc += math.Abs(dAcc)
		switch {
		case dAcc > accWinThresh:
			a.stepWins++
		case dAcc < -accWinThresh:
			a.plainWins++
		default:
			a.tie++
		}
		switch {
		case dScore > scoreWinThresh:
			a.scoreStep++
		case dScore < -scoreWinThresh:
			a.scorePlain++
		}
		switch {
		case dAvail > 0.5:
			a.availStep++
		case dAvail < -0.5:
			a.availPlain++
		}
	}
	out := CompareStepFamilyPair{Step: PrettyMode(step), Plain: PrettyMode(plain)}
	out.AccDelta = make([]float64, len(lrs))
	out.ScoreDelta = make([]float64, len(lrs))
	out.AvailDelta = make([]float64, len(lrs))
	out.N = make([]int, len(lrs))
	totalN := 0
	var sumAccD, sumScoreD, sumAvailD, sumAbsAcc float64
	var scoreStep, scorePlain, availStep, availPlain int
	for i, lr := range lrs {
		a := byLR[lr]
		if a == nil || a.n == 0 {
			continue
		}
		n := float64(a.n)
		out.N[i] = a.n
		out.AccDelta[i] = a.accD / n
		out.ScoreDelta[i] = a.scoreD / n
		out.AvailDelta[i] = a.availD / n
		totalN += a.n
		sumAccD += a.accD
		sumScoreD += a.scoreD
		sumAvailD += a.availD
		sumAbsAcc += a.absAcc
		out.StepWins += a.stepWins
		out.PlainWins += a.plainWins
		out.Ties += a.tie
		scoreStep += a.scoreStep
		scorePlain += a.scorePlain
		availStep += a.availStep
		availPlain += a.availPlain
	}
	out.StepBetterLRs, out.PlainBetterLRs = nil, nil
	for i, lr := range lrs {
		if i >= len(out.N) || out.N[i] == 0 {
			continue
		}
		lbl := FormatLR(lr)
		if i < len(lrLabels) && lrLabels[i] != "" {
			lbl = lrLabels[i]
		}
		if out.AccDelta[i] > accWinThresh {
			out.StepBetterLRs = append(out.StepBetterLRs, lbl)
		} else if out.AccDelta[i] < -accWinThresh {
			out.PlainBetterLRs = append(out.PlainBetterLRs, lbl)
		}
	}
	out.MatchN = totalN
	if totalN > 0 {
		out.PooledAccDelta = sumAccD / float64(totalN)
		out.PooledScoreDelta = sumScoreD / float64(totalN)
		out.PooledAvailDelta = sumAvailD / float64(totalN)
		out.MeanAbsAcc = sumAbsAcc / float64(totalN)
		out.AccWinPct = 100 * float64(out.StepWins) / float64(totalN)
		out.ScoreWinPct = 100 * float64(scoreStep) / float64(totalN)
		out.AvailWinPct = 100 * float64(availStep) / float64(totalN)
	}
	out.Collapse = out.MeanAbsAcc < stepCollapseAbsAcc && totalN > 0
	out.AccVerdict = stepMetricVerdict(out.PooledAccDelta, out.MeanAbsAcc, accWinThresh)
	out.ScoreVerdict = stepMetricVerdict(out.PooledScoreDelta, out.MeanAbsAcc, scoreWinThresh)
	out.AvailVerdict = stepMetricVerdict(out.PooledAvailDelta, out.MeanAbsAcc, 0.5)
	out.OverallVerdict = stepOverallVerdict(out)
	return out
}

func stepMetricVerdict(pooled, meanAbs, thresh float64) string {
	if meanAbs < stepCollapseAbsAcc {
		return "collapse"
	}
	return metricWinner(pooled, thresh)
}

func stepOverallVerdict(p CompareStepFamilyPair) string {
	if p.Collapse {
		return "collapse"
	}
	wins := 0
	losses := 0
	for _, v := range []string{p.AccVerdict, p.ScoreVerdict, p.AvailVerdict} {
		switch v {
		case "step":
			wins++
		case "plain":
			losses++
		}
	}
	if wins >= 2 && losses == 0 {
		return "step"
	}
	if losses >= 2 && wins == 0 {
		return "plain"
	}
	if p.AccVerdict == "step" {
		return "step"
	}
	if p.AccVerdict == "plain" {
		return "plain"
	}
	return "mixed"
}
