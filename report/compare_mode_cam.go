package report

import (
	"fmt"
	"sort"
)

// CompareModeCamSeries is one (machine × step|plain) line on a family overlay chart.
type CompareModeCamSeries struct {
	Machine string                   `json:"machine"`
	Kind    string                   `json:"kind"` // "step" | "plain"
	Mode    string                   `json:"mode"`
	Label   string                   `json:"label"`
	Points  []CompareModeSeriesPoint `json:"points"`
}

// CompareModeCamFamily overlays cams and Step*/plain for one train-mode family vs LR.
type CompareModeCamFamily struct {
	Family   string                 `json:"family"`
	Step     string                 `json:"step,omitempty"`
	Plain    string                 `json:"plain"`
	N        int                    `json:"n"`
	Series   []CompareModeCamSeries `json:"series"`
	Headline string                 `json:"headline,omitempty"`
}

type modeCamAgg struct {
	n                            int
	acc, soft, score, avail      float64
	lbl                          string
}

// compareModeCamFamilies builds Acc/Avail overlays: every cam × (plain + Step*) on one chart per family.
func compareModeCamFamilies(pts []taggedPoint, machines []string, topN int) []CompareModeCamFamily {
	if topN <= 0 {
		topN = 24
	}
	have := map[string]string{} // modeKey → display name
	count := map[string]int{}
	for _, p := range pts {
		k := modeKey(p.Mode)
		if k == "" {
			continue
		}
		if _, ok := have[k]; !ok {
			have[k] = PrettyMode(p.Mode)
		}
		count[k]++
	}

	type famSpec struct {
		family, step, plain string
		n                   int
	}
	var specs []famSpec
	used := map[string]bool{}
	for k, stepName := range have {
		plainName, ok := stepMate(k, have)
		if !ok {
			continue
		}
		pk := modeKey(plainName)
		used[k] = true
		used[pk] = true
		specs = append(specs, famSpec{
			family: PrettyMode(plainName),
			step:   stepName,
			plain:  plainName,
			n:      count[k] + count[pk],
		})
	}
	for k, name := range have {
		if used[k] {
			continue
		}
		specs = append(specs, famSpec{
			family: PrettyMode(name),
			plain:  name,
			n:      count[k],
		})
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].n != specs[j].n {
			return specs[i].n > specs[j].n
		}
		return specs[i].family < specs[j].family
	})
	if len(specs) > topN {
		specs = specs[:topN]
	}

	// machine → modeKey → lr → agg
	by := map[string]map[string]map[float64]*modeCamAgg{}
	for _, p := range pts {
		mk := modeKey(p.Mode)
		if mk == "" {
			continue
		}
		mm := by[p.Machine]
		if mm == nil {
			mm = map[string]map[float64]*modeCamAgg{}
			by[p.Machine] = mm
		}
		lm := mm[mk]
		if lm == nil {
			lm = map[float64]*modeCamAgg{}
			mm[mk] = lm
		}
		c := lm[p.LR]
		if c == nil {
			c = &modeCamAgg{lbl: p.LRLabel}
			lm[p.LR] = c
		}
		c.n++
		c.acc += p.Acc
		c.soft += p.Soft
		c.score += p.Score
		c.avail += p.Avail
		if c.lbl == "" {
			c.lbl = p.LRLabel
		}
	}

	out := make([]CompareModeCamFamily, 0, len(specs))
	for _, sp := range specs {
		fam := CompareModeCamFamily{
			Family: sp.family,
			Step:   sp.step,
			Plain:  sp.plain,
			N:      sp.n,
		}
		kinds := []struct{ kind, mode string }{{"plain", sp.plain}}
		if sp.step != "" {
			kinds = append(kinds, struct{ kind, mode string }{"step", sp.step})
		}
		for _, m := range machines {
			for _, md := range kinds {
				mk := modeKey(md.mode)
				lm := by[m][mk]
				if len(lm) == 0 {
					continue
				}
				lrs := make([]float64, 0, len(lm))
				for lr := range lm {
					lrs = append(lrs, lr)
				}
				sort.Float64s(lrs)
				ptsOut := make([]CompareModeSeriesPoint, 0, len(lrs))
				for _, lr := range lrs {
					c := lm[lr]
					if c == nil || c.n == 0 {
						continue
					}
					n := float64(c.n)
					lbl := c.lbl
					if lbl == "" {
						lbl = FormatLR(lr)
					}
					ptsOut = append(ptsOut, CompareModeSeriesPoint{
						LR: lr, LRLabel: lbl, N: c.n,
						Acc: c.acc / n, Soft: c.soft / n,
						Score: c.score / n, Avail: c.avail / n,
					})
				}
				if len(ptsOut) == 0 {
					continue
				}
				kindLbl := "plain"
				if md.kind == "step" {
					kindLbl = "Step*"
				}
				fam.Series = append(fam.Series, CompareModeCamSeries{
					Machine: m,
					Kind:    md.kind,
					Mode:    PrettyMode(md.mode),
					Label:   m + " · " + kindLbl,
					Points:  ptsOut,
				})
			}
		}
		if len(fam.Series) == 0 {
			continue
		}
		fam.Headline = modeCamFamilyHeadline(fam)
		out = append(out, fam)
	}
	return out
}

func modeCamFamilyHeadline(f CompareModeCamFamily) string {
	if f.Step == "" {
		return fmt.Sprintf("%s — cams overlaid (no Step* mate in this farm).", f.Family)
	}
	type avg struct {
		label string
		acc   float64
	}
	var avgs []avg
	for _, s := range f.Series {
		var sum float64
		n := 0
		for _, p := range s.Points {
			sum += p.Acc
			n++
		}
		if n == 0 {
			continue
		}
		avgs = append(avgs, avg{label: s.Label, acc: sum / float64(n)})
	}
	if len(avgs) == 0 {
		return f.Family
	}
	sort.Slice(avgs, func(i, j int) bool { return avgs[i].acc > avgs[j].acc })
	best := avgs[0]
	return fmt.Sprintf("%s — best mean Acc: %s (%.1f%%). Solid = plain, dashed = Step*.",
		f.Family, best.label, best.acc)
}
