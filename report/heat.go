package report

import (
	"math"
	"sort"
	"strings"

	"github.com/openfluke/tide/pulse"
)

const maxHeatPoints = 3000

// PointsFromResults keeps ok cells for charts. IDs are display-canonical.
func PointsFromResults(rs []pulse.Result, tide string) []CellPoint {
	out := make([]CellPoint, 0, len(rs))
	for _, r := range rs {
		if r.Status != "ok" {
			continue
		}
		s := r.Snapshot
		arch := PrettyArch(r.Cell.ArchTag())
		if arch == "" {
			arch = PrettyArch(string(r.Cell.Arch))
		}
		out = append(out, CellPoint{
			Tide:   tide,
			ID:     PrettyCell(r.Cell.ID),
			Mode:   string(r.Cell.Mode),
			DType:  r.Cell.DType.String(),
			Format: r.Cell.Format.String(),
			Arch:   arch,
			Score:  s.Throughput * s.Availability * s.AvgAccuracy / 10000,
			Soft:   s.SoftAcc,
			Acc:    s.AvgAccuracy,
			Avail:  s.Availability,
			Thru:   s.Throughput,
			Adapt:  s.AdaptPct,
			RAMKiB: float64(s.WeightBytes) / 1024,
		})
	}
	return out
}

// BuildHeat aggregates mean Score / Soft / Acc on the Lucy grids.
func BuildHeat(pts []CellPoint) Heat {
	h := Heat{Points: pts}
	if len(pts) > maxHeatPoints {
		h.Points = pts[:maxHeatPoints]
	}
	if len(pts) == 0 {
		return h
	}
	h.Modes = uniq(pts, func(p CellPoint) string { return p.Mode })
	h.DTypes = uniq(pts, func(p CellPoint) string { return p.DType })
	h.Arches = uniq(pts, func(p CellPoint) string { return p.Arch })
	h.Layers = uniq(pts, func(p CellPoint) string { return p.Tide })

	h.ModeDTypeScore, h.ModeDTypeSoft, h.ModeDTypeAcc = grid3(pts, h.Modes, h.DTypes,
		func(p CellPoint) string { return p.Mode },
		func(p CellPoint) string { return p.DType },
	)
	h.ModeArchScore, _, h.ModeArchAcc = grid3(pts, h.Modes, h.Arches,
		func(p CellPoint) string { return p.Mode },
		func(p CellPoint) string { return p.Arch },
	)
	if len(h.Layers) > 0 && h.Layers[0] != "" {
		h.LayerModeScore, _, h.LayerModeAcc = grid3(pts, h.Layers, h.Modes,
			func(p CellPoint) string { return p.Tide },
			func(p CellPoint) string { return p.Mode },
		)
	}
	h.ModeMeanScore, h.ModeMeanSoft, h.ModeMeanAcc = means(pts, h.Modes, func(p CellPoint) string { return p.Mode })
	h.ModeMeanAvail = meanOf(pts, h.Modes, func(p CellPoint) string { return p.Mode }, func(p CellPoint) float64 { return p.Avail })
	h.ModeMeanThru = meanOf(pts, h.Modes, func(p CellPoint) string { return p.Mode }, func(p CellPoint) float64 { return p.Thru })
	h.DTypeMeanScore, _, h.DTypeMeanAcc = means(pts, h.DTypes, func(p CellPoint) string { return p.DType })
	h.ArchMeanScore, _, h.ArchMeanAcc = means(pts, h.Arches, func(p CellPoint) string { return p.Arch })
	h.ModeDTypeAvail = grid1(pts, h.Modes, h.DTypes,
		func(p CellPoint) string { return p.Mode },
		func(p CellPoint) string { return p.DType },
		func(p CellPoint) float64 {
			if p.Avail <= 0 {
				return math.NaN()
			}
			return p.Avail
		},
	)
	h.ModeArchAvail = grid1(pts, h.Modes, h.Arches,
		func(p CellPoint) string { return p.Mode },
		func(p CellPoint) string { return p.Arch },
		func(p CellPoint) float64 { return p.Avail },
	)
	fillVs(&h, pts)
	return h
}

func uniq(pts []CellPoint, key func(CellPoint) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pts {
		k := strings.TrimSpace(key(p))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func grid3(pts []CellPoint, rows, cols []string, rowOf, colOf func(CellPoint) string) (score, soft, acc [][]float64) {
	type accu struct {
		n, ns    int
		s, so, a float64
	}
	idxR := indexOf(rows)
	idxC := indexOf(cols)
	bucket := make([][]accu, len(rows))
	for i := range bucket {
		bucket[i] = make([]accu, len(cols))
	}
	for _, p := range pts {
		i, ok := idxR[rowOf(p)]
		if !ok {
			continue
		}
		j, ok := idxC[colOf(p)]
		if !ok {
			continue
		}
		b := &bucket[i][j]
		b.n++
		b.so += p.Soft
		b.a += p.Acc
		// Score needs Availability; rows restored without Avail must not paint as Score=0.
		if p.Avail > 0 {
			b.ns++
			b.s += p.Score
		}
	}
	score = make([][]float64, len(rows))
	soft = make([][]float64, len(rows))
	acc = make([][]float64, len(rows))
	for i := range rows {
		score[i] = make([]float64, len(cols))
		soft[i] = make([]float64, len(cols))
		acc[i] = make([]float64, len(cols))
		for j := range cols {
			if bucket[i][j].n == 0 {
				score[i][j] = math.NaN()
				soft[i][j] = math.NaN()
				acc[i][j] = math.NaN()
				continue
			}
			n := float64(bucket[i][j].n)
			soft[i][j] = bucket[i][j].so / n
			acc[i][j] = bucket[i][j].a / n
			if bucket[i][j].ns == 0 {
				score[i][j] = math.NaN()
			} else {
				score[i][j] = bucket[i][j].s / float64(bucket[i][j].ns)
			}
		}
	}
	return
}

func means(pts []CellPoint, keys []string, keyOf func(CellPoint) string) (score, soft, acc []float64) {
	type accu struct {
		n        int
		s, so, a float64
	}
	idx := indexOf(keys)
	bucket := make([]accu, len(keys))
	for _, p := range pts {
		i, ok := idx[keyOf(p)]
		if !ok {
			continue
		}
		bucket[i].n++
		bucket[i].s += p.Score
		bucket[i].so += p.Soft
		bucket[i].a += p.Acc
	}
	score = make([]float64, len(keys))
	soft = make([]float64, len(keys))
	acc = make([]float64, len(keys))
	for i := range keys {
		if bucket[i].n == 0 {
			continue
		}
		n := float64(bucket[i].n)
		score[i] = bucket[i].s / n
		soft[i] = bucket[i].so / n
		acc[i] = bucket[i].a / n
	}
	return
}

func grid1(pts []CellPoint, rows, cols []string, rowOf, colOf func(CellPoint) string, val func(CellPoint) float64) [][]float64 {
	type accu struct {
		n int
		s float64
	}
	idxR := indexOf(rows)
	idxC := indexOf(cols)
	bucket := make([][]accu, len(rows))
	for i := range bucket {
		bucket[i] = make([]accu, len(cols))
	}
	for _, p := range pts {
		i, ok := idxR[rowOf(p)]
		if !ok {
			continue
		}
		j, ok := idxC[colOf(p)]
		if !ok {
			continue
		}
		v := val(p)
		// Skip missing Availability so Avail heatmaps don't treat "unknown" as 0%.
		if math.IsNaN(v) {
			continue
		}
		b := &bucket[i][j]
		b.n++
		b.s += v
	}
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = make([]float64, len(cols))
		for j := range cols {
			if bucket[i][j].n == 0 {
				out[i][j] = math.NaN()
				continue
			}
			out[i][j] = bucket[i][j].s / float64(bucket[i][j].n)
		}
	}
	return out
}

func meanOf(pts []CellPoint, keys []string, keyOf func(CellPoint) string, val func(CellPoint) float64) []float64 {
	type accu struct {
		n int
		s float64
	}
	idx := indexOf(keys)
	bucket := make([]accu, len(keys))
	for _, p := range pts {
		i, ok := idx[keyOf(p)]
		if !ok {
			continue
		}
		bucket[i].n++
		bucket[i].s += val(p)
	}
	out := make([]float64, len(keys))
	for i := range keys {
		if bucket[i].n == 0 {
			continue
		}
		out[i] = bucket[i].s / float64(bucket[i].n)
	}
	return out
}

func indexOf(xs []string) map[string]int {
	m := make(map[string]int, len(xs))
	for i, x := range xs {
		m[x] = i
	}
	return m
}
