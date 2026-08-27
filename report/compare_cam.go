package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ComparePeerMeta describes one tide peer in a compare run.
type ComparePeerMeta struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Machine string `json:"machine"`
	Cam     int    `json:"cam,omitempty"`
	Band    string `json:"band,omitempty"`
}

// CompareCamPair is head-to-head cam1 vs camN within one LR band (lo/hi).
type CompareCamPair struct {
	Base     string              `json:"base"`
	Other    string              `json:"other"`
	Band     string              `json:"band"`
	Headline string              `json:"headline"`
	ByLR     []CompareCamDeltaLR `json:"by_lr"`
	Matched  []CompareMatchRow   `json:"matched_top"`
}

// CompareCamDeltaLR is pooled Δ(other−base) across matched recipes at one LR.
type CompareCamDeltaLR struct {
	LR             float64 `json:"lr"`
	LRLabel        string  `json:"lr_label"`
	N              int     `json:"n"`
	MeanAccDelta   float64 `json:"mean_acc_delta"`
	MeanScoreDelta float64 `json:"mean_score_delta"`
	WinsOther      int     `json:"wins_other"`
	WinsBase       int     `json:"wins_base"`
}

func peerMetas(tides []NamedTideReport) []ComparePeerMeta {
	out := make([]ComparePeerMeta, 0, len(tides))
	for _, tr := range tides {
		machine := ParseMachineFromPeer(tr.Name)
		if tr.Report.ID != "" && machine == tr.Name {
			if m := ParseMachineFromPeer(tr.Report.ID); m != "?" {
				machine = m
			}
		}
		out = append(out, ComparePeerMeta{
			Name:    tr.Name,
			Machine: machine,
			Cam:     ParsePeerCam(machine),
			Band:    ParsePeerBand(tr.Name),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cam != out[j].Cam {
			return out[i].Cam < out[j].Cam
		}
		return out[i].Band < out[j].Band
	})
	return out
}

func compareCamPairs(pts []taggedPoint, tides []NamedTideReport, machines []string, lrMap map[float64]string, topN int) []CompareCamPair {
	camMachines := camMachineList(machines)
	if len(camMachines) < 2 {
		return nil
	}
	bands := camBandsFromPoints(pts)
	if len(bands) == 0 {
		bands = []string{""}
	}
	base := camMachines[0]
	var pairs []CompareCamPair
	for _, band := range bands {
		for _, other := range camMachines[1:] {
			if other == base {
				continue
			}
			pair := buildCamPair(pts, base, other, band, lrMap, topN)
			if len(pair.ByLR) == 0 && len(pair.Matched) == 0 {
				continue
			}
			pairs = append(pairs, pair)
		}
	}
	return pairs
}

func camMachineList(machines []string) []string {
	var out []int
	seen := map[int]string{}
	for _, m := range machines {
		c := ParsePeerCam(m)
		if c < 1 {
			continue
		}
		if _, ok := seen[c]; !ok {
			seen[c] = m
			out = append(out, c)
		}
	}
	sort.Ints(out)
	names := make([]string, len(out))
	for i, c := range out {
		names[i] = seen[c]
	}
	return names
}

func camBandsFromPoints(pts []taggedPoint) []string {
	seen := map[string]bool{}
	for _, p := range pts {
		if ParsePeerCam(p.Machine) < 1 {
			continue
		}
		b := ParsePeerBand(p.Tide)
		if b != "" {
			seen[b] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func buildCamPair(pts []taggedPoint, base, other, band string, lrMap map[float64]string, topN int) CompareCamPair {
	filter := func(p taggedPoint) bool {
		if p.Machine != base && p.Machine != other {
			return false
		}
		if band == "" {
			return true
		}
		return ParsePeerBand(p.Tide) == band
	}
	var sub []taggedPoint
	for _, p := range pts {
		if filter(p) {
			sub = append(sub, p)
		}
	}
	machines := []string{base, other}
	matched := compareMatchedTop(sub, machines, lrMap, topN)
	byLR := camDeltaByLR(sub, base, other, lrMap)
	headline := camPairHeadline(base, other, band, byLR, matched)
	return CompareCamPair{
		Base: base, Other: other, Band: band,
		Headline: headline, ByLR: byLR, Matched: matched,
	}
}

func camDeltaByLR(pts []taggedPoint, base, other string, lrMap map[float64]string) []CompareCamDeltaLR {
	type key struct {
		recipe string
		lr     float64
	}
	by := map[key]map[string]cellAgg{}
	for _, p := range pts {
		if p.Machine != base && p.Machine != other {
			continue
		}
		k := key{recipe: RecipeKey(p.ID), lr: p.LR}
		m := by[k]
		if m == nil {
			m = map[string]cellAgg{}
			by[k] = m
		}
		a := m[p.Machine]
		if p.Acc > a.acc {
			a.acc = p.Acc
		}
		if p.Score > a.score {
			a.score = p.Score
		}
		m[p.Machine] = a
	}
	type lrAcc struct {
		lr    float64
		dAcc  float64
		dSc   float64
		winO  bool
	}
	perLR := map[float64][]lrAcc{}
	for k, vals := range by {
		ba, okB := vals[base]
		oa, okO := vals[other]
		if !okB || !okO || ba.acc <= 0 || oa.acc <= 0 {
			continue
		}
		da := oa.acc - ba.acc
		ds := oa.score - ba.score
		perLR[k.lr] = append(perLR[k.lr], lrAcc{lr: k.lr, dAcc: da, dSc: ds, winO: da > 0.05})
	}
	lrs := make([]float64, 0, len(perLR))
	for lr := range perLR {
		lrs = append(lrs, lr)
	}
	sort.Float64s(lrs)
	out := make([]CompareCamDeltaLR, 0, len(lrs))
	for _, lr := range lrs {
		vals := perLR[lr]
		if len(vals) == 0 {
			continue
		}
		var sumA, sumS float64
		wO, wB := 0, 0
		for _, v := range vals {
			sumA += v.dAcc
			sumS += v.dSc
			if v.winO {
				wO++
			} else if v.dAcc < -0.05 {
				wB++
			}
		}
		n := len(vals)
		lbl := lrMap[lr]
		if lbl == "" {
			lbl = FormatLR(lr)
		}
		out = append(out, CompareCamDeltaLR{
			LR: lr, LRLabel: lbl, N: n,
			MeanAccDelta: sumA / float64(n),
			MeanScoreDelta: sumS / float64(n),
			WinsOther: wO, WinsBase: wB,
		})
	}
	return out
}

type cellAgg struct {
	acc, score float64
}

func camPairHeadline(base, other, band string, byLR []CompareCamDeltaLR, matched []CompareMatchRow) string {
	bandTxt := band
	if bandTxt != "" {
		bandTxt = " (" + band + " band)"
	}
	if len(byLR) == 0 {
		return fmt.Sprintf("%s vs %s%s — no shared LR steps with matched recipes yet.", other, base, bandTxt)
	}
	var sum float64
	for _, r := range byLR {
		sum += r.MeanAccDelta
	}
	avg := sum / float64(len(byLR))
	winner := other
	if avg < -0.05 {
		winner = base
	} else if math.Abs(avg) <= 0.05 {
		winner = "tie"
	}
	switch winner {
	case other:
		return fmt.Sprintf("%s vs %s%s — %s ahead on mean Acc (Δ %+.1f pp pooled across %d LR steps, %d matched recipes).",
			other, base, bandTxt, other, avg, len(byLR), len(matched))
	case base:
		return fmt.Sprintf("%s vs %s%s — %s ahead on mean Acc (Δ %+.1f pp pooled).",
			other, base, bandTxt, base, avg)
	default:
		return fmt.Sprintf("%s vs %s%s — Acc tie (Δ %+.1f pp pooled across %d LR steps).",
			other, base, bandTxt, avg, len(byLR))
	}
}

// SortCamMachines orders cam1, cam3, … then other names.
func SortCamMachines(machines []string) []string {
	out := append([]string(nil), machines...)
	sort.Slice(out, func(i, j int) bool {
		ci, cj := ParsePeerCam(out[i]), ParsePeerCam(out[j])
		if ci > 0 && cj > 0 {
			return ci < cj
		}
		if ci > 0 {
			return true
		}
		if cj > 0 {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

func camPairLabel(base, other, band string) string {
	s := other + " − " + base
	if band != "" {
		s += " · " + strings.ToUpper(band)
	}
	return s
}
