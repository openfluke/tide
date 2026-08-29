package report

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/openfluke/tide/pulse"
)

// MarshalJSON encodes Heat for APIs. Empty grid cells are stored as NaN in
// memory (SVG heatmaps), but encoding/json cannot emit NaN — that used to
// make /api/live and /api/board return HTTP 200 with an empty body.
func (h Heat) MarshalJSON() ([]byte, error) {
	type dto struct {
		Modes  []string `json:"modes,omitempty"`
		DTypes []string `json:"dtypes,omitempty"`
		Arches []string `json:"arches,omitempty"`
		Layers []string `json:"layers,omitempty"`

		ModeDTypeScore [][]any `json:"mode_dtype_score,omitempty"`
		ModeDTypeSoft  [][]any `json:"mode_dtype_soft,omitempty"`
		ModeDTypeAcc   [][]any `json:"mode_dtype_acc,omitempty"`
		ModeArchScore  [][]any `json:"mode_arch_score,omitempty"`
		ModeArchAcc    [][]any `json:"mode_arch_acc,omitempty"`
		LayerModeScore [][]any `json:"layer_mode_score,omitempty"`
		LayerModeAcc   [][]any `json:"layer_mode_acc,omitempty"`
		ModeDTypeAvail [][]any `json:"mode_dtype_avail,omitempty"`
		ModeArchAvail  [][]any `json:"mode_arch_avail,omitempty"`

		ModeMeanScore  []any `json:"mode_mean_score,omitempty"`
		ModeMeanSoft   []any `json:"mode_mean_soft,omitempty"`
		ModeMeanAcc    []any `json:"mode_mean_acc,omitempty"`
		ModeMeanAvail  []any `json:"mode_mean_avail,omitempty"`
		ModeMeanThru   []any `json:"mode_mean_thru,omitempty"`
		DTypeMeanScore []any `json:"dtype_mean_score,omitempty"`
		DTypeMeanAcc   []any `json:"dtype_mean_acc,omitempty"`
		ArchMeanScore  []any `json:"arch_mean_score,omitempty"`
		ArchMeanAcc    []any `json:"arch_mean_acc,omitempty"`

		Vs     *VsBoard    `json:"vs,omitempty"`
		Points []CellPoint `json:"points,omitempty"`
	}
	return json.Marshal(dto{
		Modes:          h.Modes,
		DTypes:         h.DTypes,
		Arches:         h.Arches,
		Layers:         h.Layers,
		ModeDTypeScore: jsonSafeGrid(h.ModeDTypeScore),
		ModeDTypeSoft:  jsonSafeGrid(h.ModeDTypeSoft),
		ModeDTypeAcc:   jsonSafeGrid(h.ModeDTypeAcc),
		ModeArchScore:  jsonSafeGrid(h.ModeArchScore),
		ModeArchAcc:    jsonSafeGrid(h.ModeArchAcc),
		LayerModeScore: jsonSafeGrid(h.LayerModeScore),
		LayerModeAcc:   jsonSafeGrid(h.LayerModeAcc),
		ModeDTypeAvail: jsonSafeGrid(h.ModeDTypeAvail),
		ModeArchAvail:  jsonSafeGrid(h.ModeArchAvail),
		ModeMeanScore:  jsonSafeRow(h.ModeMeanScore),
		ModeMeanSoft:   jsonSafeRow(h.ModeMeanSoft),
		ModeMeanAcc:    jsonSafeRow(h.ModeMeanAcc),
		ModeMeanAvail:  jsonSafeRow(h.ModeMeanAvail),
		ModeMeanThru:   jsonSafeRow(h.ModeMeanThru),
		DTypeMeanScore: jsonSafeRow(h.DTypeMeanScore),
		DTypeMeanAcc:   jsonSafeRow(h.DTypeMeanAcc),
		ArchMeanScore:  jsonSafeRow(h.ArchMeanScore),
		ArchMeanAcc:    jsonSafeRow(h.ArchMeanAcc),
		Vs:             h.Vs,
		Points:         h.Points,
	})
}

func jsonSafeFloat(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

func jsonSafeRow(in []float64) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = jsonSafeFloat(v)
	}
	return out
}

func jsonSafeGrid(in [][]float64) [][]any {
	if len(in) == 0 {
		return nil
	}
	out := make([][]any, len(in))
	for i, row := range in {
		if row == nil {
			continue
		}
		out[i] = make([]any, len(row))
		for j, v := range row {
			out[i][j] = jsonSafeFloat(v)
		}
	}
	return out
}

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
