package report

import (
	"math"
	"sort"
	"strings"
)

const (
	accWinThresh   = 0.5
	scoreWinThresh = 1.0
)

type vsAgg struct {
	n                             int
	acc, soft, avail, thru, score float64
}

// PickBaseline chooses a backprop analog from whatever modes are on the board.
// Lucy hosts use sgd; Welvet-native hosts use StepBP / NormalBP. No mode list is frozen.
func PickBaseline(modes []string) string {
	rank := map[string]int{
		"sgd": 1, "normalbp": 2, "normal_bp": 2,
		"stepbp": 3, "step_bp": 3, "step_sgd": 4,
	}
	best, bestR := "", 0
	for _, m := range modes {
		r, ok := rank[modeKey(m)]
		if !ok {
			continue
		}
		if bestR == 0 || r < bestR {
			best, bestR = m, r
		}
	}
	return best
}

func fillVs(h *Heat, pts []CellPoint) {
	base := PickBaseline(h.Modes)
	if base == "" {
		return
	}
	vs := buildVs(pts, base)
	if vs == nil || len(vs.Modes) == 0 {
		return
	}
	h.Vs = vs
}

func buildVs(pts []CellPoint, baseline string) *VsBoard {
	baseKey := modeKey(baseline)
	bp := map[string]vsAgg{}
	for _, p := range pts {
		if modeKey(p.Mode) != baseKey {
			continue
		}
		k := matchKey(p)
		a := bp[k]
		a.n++
		a.acc += p.Acc
		a.soft += p.Soft
		a.avail += p.Avail
		a.thru += p.Thru
		a.score += p.Score
		bp[k] = a
	}
	if len(bp) == 0 {
		return &VsBoard{Baseline: baseline}
	}
	type modeAcc struct {
		n, accW, scoreW               int
		acc, soft, avail, thru, score float64
	}
	byMode := map[string]*modeAcc{}
	dtypeB, archB, layerB := map[string]*vsAgg{}, map[string]*vsAgg{}, map[string]*vsAgg{}
	binAdd := func(dst map[string]*vsAgg, mode, key string, dAcc, dSoft, dAvail, dThru, dScore float64) {
		if strings.TrimSpace(key) == "" {
			return
		}
		k := mode + "\x1f" + key
		a := dst[k]
		if a == nil {
			a = &vsAgg{}
			dst[k] = a
		}
		a.n++
		a.acc += dAcc
		a.soft += dSoft
		a.avail += dAvail
		a.thru += dThru
		a.score += dScore
	}
	for _, p := range pts {
		if modeKey(p.Mode) == baseKey {
			continue
		}
		b, ok := bp[matchKey(p)]
		if !ok || b.n == 0 {
			continue
		}
		n := float64(b.n)
		dAcc := p.Acc - b.acc/n
		dSoft := p.Soft - b.soft/n
		dAvail := p.Avail - b.avail/n
		dThru := p.Thru - b.thru/n
		dScore := p.Score - b.score/n
		m := byMode[p.Mode]
		if m == nil {
			m = &modeAcc{}
			byMode[p.Mode] = m
		}
		m.n++
		m.acc += dAcc
		m.soft += dSoft
		m.avail += dAvail
		m.thru += dThru
		m.score += dScore
		if dAcc > accWinThresh {
			m.accW++
		}
		if dScore > scoreWinThresh {
			m.scoreW++
		}
		binAdd(dtypeB, p.Mode, p.DType, dAcc, dSoft, dAvail, dThru, dScore)
		binAdd(archB, p.Mode, p.Arch, dAcc, dSoft, dAvail, dThru, dScore)
		layer := p.Layer
		if layer == "" {
			layer = p.Tide
		}
		binAdd(layerB, p.Mode, layer, dAcc, dSoft, dAvail, dThru, dScore)
	}
	out := &VsBoard{Baseline: baseline}
	for mode, m := range byMode {
		if m.n == 0 {
			continue
		}
		n := float64(m.n)
		out.Modes = append(out.Modes, VsMode{
			Mode:       mode,
			N:          m.n,
			AccDelta:   m.acc / n,
			AccWin:     100 * float64(m.accW) / n,
			SoftDelta:  m.soft / n,
			AvailDelta: m.avail / n,
			ThruDelta:  m.thru / n,
			ScoreDelta: m.score / n,
			ScoreWin:   100 * float64(m.scoreW) / n,
		})
	}
	sort.Slice(out.Modes, func(i, j int) bool {
		if out.Modes[i].AccDelta != out.Modes[j].AccDelta {
			return out.Modes[i].AccDelta > out.Modes[j].AccDelta
		}
		return out.Modes[i].Mode < out.Modes[j].Mode
	})
	out.ByDType = flattenBins(dtypeB)
	out.ByArch = flattenBins(archB)
	out.ByLayer = flattenBins(layerB)
	if uniqueKeys(out.ByLayer) < 2 {
		out.ByLayer = nil
	}
	out.Families = familyPairs(pts)
	return out
}

func flattenBins(src map[string]*vsAgg) []DeltaBin {
	out := make([]DeltaBin, 0, len(src))
	for k, a := range src {
		if a == nil || a.n == 0 {
			continue
		}
		mode, key, _ := strings.Cut(k, "\x1f")
		n := float64(a.n)
		out = append(out, DeltaBin{
			Mode:  mode,
			Key:   key,
			N:     a.n,
			Acc:   a.acc / n,
			Soft:  a.soft / n,
			Avail: a.avail / n,
			Thru:  a.thru / n,
			Score: a.score / n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mode != out[j].Mode {
			return out[i].Mode < out[j].Mode
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func uniqueKeys(bins []DeltaBin) int {
	seen := map[string]bool{}
	for _, b := range bins {
		if b.Key != "" {
			seen[b.Key] = true
		}
	}
	return len(seen)
}

func familyPairs(pts []CellPoint) []FamilyRow {
	have := map[string]string{}
	for _, p := range pts {
		have[modeKey(p.Mode)] = p.Mode
	}
	type pair struct{ step, plain string }
	var pairs []pair
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
		pairs = append(pairs, pair{step: stepName, plain: plainName})
	}
	if len(pairs) == 0 {
		return nil
	}
	byKey := map[string]map[string]CellPoint{}
	for _, p := range pts {
		mk := modeKey(p.Mode)
		m := byKey[mk]
		if m == nil {
			m = map[string]CellPoint{}
			byKey[mk] = m
		}
		m[matchKey(p)] = p
	}
	var out []FamilyRow
	for _, pr := range pairs {
		a, b := byKey[modeKey(pr.step)], byKey[modeKey(pr.plain)]
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		n := 0
		sum := 0.0
		for k, pa := range a {
			pb, ok := b[k]
			if !ok {
				continue
			}
			n++
			sum += math.Abs(pa.Acc - pb.Acc)
		}
		if n == 0 {
			continue
		}
		out = append(out, FamilyRow{
			Step: pr.step, Plain: pr.plain, N: n, MeanAbsAcc: sum / float64(n),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Step < out[j].Step })
	return out
}

func stepMate(k string, have map[string]string) (string, bool) {
	if !strings.HasPrefix(k, "step") {
		return "", false
	}
	rest := strings.TrimPrefix(k, "step")
	if rest == "" {
		return "", false
	}
	if rest == "bp" || rest == "sgd" {
		for _, cand := range []string{"sgd", "normalbp", "normal_bp", "bp"} {
			if name, ok := have[cand]; ok && cand != k {
				return name, true
			}
		}
		return "", false
	}
	name, ok := have[rest]
	return name, ok
}

func matchKey(p CellPoint) string {
	return strings.Join([]string{p.Tide, p.Task, p.Layer, p.DType, p.Format, p.Arch}, "\x1f")
}

func modeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

// pivotDelta turns sparse bins into a dense grid. Empty cells stay unhit (not zero).
func pivotDelta(bins []DeltaBin, val func(DeltaBin) float64) (rows, cols []string, grid [][]float64, hit [][]bool) {
	if len(bins) == 0 {
		return nil, nil, nil, nil
	}
	rowSeen, colSeen := map[string]bool{}, map[string]bool{}
	for _, b := range bins {
		if b.N == 0 || b.Mode == "" || b.Key == "" {
			continue
		}
		if !rowSeen[b.Mode] {
			rowSeen[b.Mode] = true
			rows = append(rows, b.Mode)
		}
		if !colSeen[b.Key] {
			colSeen[b.Key] = true
			cols = append(cols, b.Key)
		}
	}
	sort.Strings(rows)
	sort.Strings(cols)
	if len(rows) == 0 || len(cols) == 0 {
		return nil, nil, nil, nil
	}
	ri, ci := indexOf(rows), indexOf(cols)
	grid = make([][]float64, len(rows))
	hit = make([][]bool, len(rows))
	for i := range rows {
		grid[i] = make([]float64, len(cols))
		hit[i] = make([]bool, len(cols))
	}
	for _, b := range bins {
		i, ok := ri[b.Mode]
		if !ok {
			continue
		}
		j, ok := ci[b.Key]
		if !ok {
			continue
		}
		grid[i][j] = val(b)
		hit[i][j] = true
	}
	return
}
