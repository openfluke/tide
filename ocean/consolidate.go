package ocean

import (
	"sort"
	"strings"

	"github.com/openfluke/tide/dash"
	"github.com/openfluke/tide/pulse"
)

// LayerWinner is the best Score cell on one tide (usually one layer).
type LayerWinner struct {
	Tide     string  `json:"tide"`
	URL      string  `json:"url"`
	Mode     string  `json:"mode"`
	DType    string  `json:"dtype"`
	Format   string  `json:"format"`
	Arch     string  `json:"arch,omitempty"`
	CellID   string  `json:"cell_id"`
	Score    float64 `json:"score"`
	SoftAcc  float64 `json:"soft_acc"`
	Accuracy float64 `json:"avg_accuracy"`
	Thru     float64 `json:"throughput"`
	Avail    float64 `json:"availability"`
	Adapt    float64 `json:"adapt_pct"`
	AccPerSec float64 `json:"acc_per_sec"`
	Keep     float64 `json:"keep_learn"`
	Axes     []AxisChamp `json:"axes,omitempty"`
	Ok       int     `json:"ok"`
	Gap      int     `json:"gap"`
	Fail     int     `json:"fail"`
	Done     int     `json:"done"`
	Total    int     `json:"total"`
	Recorded int     `json:"recorded,omitempty"`
	Plan     int     `json:"plan,omitempty"`
}

// Vote is a plurality count with mean Score of the layers that picked this key.
type Vote struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Mean  float64 `json:"mean_score"`
}

// Tagged is a leaderboard row tagged with which tide it came from.
type Tagged struct {
	Tide   string       `json:"tide"`
	URL    string       `json:"url"`
	Result pulse.Result `json:"result"`
}

// AxisChamp is the ocean-wide winner on one Lucy axis.
type AxisChamp struct {
	Name    string  `json:"name"`
	Hint    string  `json:"hint"`
	Tide    string  `json:"tide"`
	URL     string  `json:"url"`
	CellID  string  `json:"cell_id"`
	Mode    string  `json:"mode"`
	DType   string  `json:"dtype"`
	Format  string  `json:"format"`
	Arch    string  `json:"arch"`
	Value   float64 `json:"value"`
	LowerBetter bool `json:"lower_better,omitempty"`
	Score   float64 `json:"score"`
	SoftAcc float64 `json:"soft_acc"`
	Thru    float64 `json:"throughput"`
	Avail   float64 `json:"availability"`
	Adapt   float64 `json:"adapt_pct"`
}

// Holistic is the master consolidation across all linked tides.
type Holistic struct {
	BestMode      string        `json:"best_mode"`
	BestDType     string        `json:"best_dtype"`
	BestArch      string        `json:"best_arch"`
	ModeVotes     []Vote        `json:"mode_votes"`
	DTypeVotes    []Vote        `json:"dtype_votes"`
	ArchVotes     []Vote        `json:"arch_votes"`
	Layers        []LayerWinner `json:"layers"`
	CombinedTop   []Tagged      `json:"combined_top"`
	Axes          []AxisChamp   `json:"axes"`
	DefaultMode   string        `json:"default_mode"`
	DefaultDType  string        `json:"default_dtype"`
	DefaultArch   string        `json:"default_arch"`
	DefaultWins   int           `json:"default_wins"`
	TidesUp       int           `json:"tides_up"`
	TidesTotal    int           `json:"tides_total"`
	CellsDone     int           `json:"cells_done"`
	CellsTotal    int           `json:"cells_total"`
}

func consolidate(peers []PeerState) Holistic {
	h := Holistic{TidesTotal: len(peers)}
	var layers []LayerWinner
	var top []Tagged
	for _, p := range peers {
		if !p.OK {
			continue
		}
		h.TidesUp++
		b := p.Board
		h.CellsDone += b.EpochDone
		if b.Plan > 0 {
			h.CellsTotal += b.Plan
		} else {
			h.CellsTotal += b.CellTotal
		}
		w := LayerWinner{
			Tide:     p.Name,
			URL:      p.URL,
			Ok:       b.Ok,
			Gap:      b.Gap,
			Fail:     b.Fail,
			Done:     b.EpochDone,
			Total:    b.Plan,
			Recorded: b.Recorded,
			Plan:     b.Plan,
		}
		if b.Best.Score != nil {
			fillLayer(&w, b.Best.Score)
		} else if len(b.Winners.BestSettingsPerMode) > 0 {
			best := b.Winners.BestSettingsPerMode[0]
			w.Mode, w.DType, w.Format = best.Mode, best.DType, best.Format
			w.Arch, w.CellID = best.Arch, best.CellID
			w.Score, w.SoftAcc, w.Accuracy = best.Score, best.SoftAcc, best.Accuracy
			w.Thru, w.Avail = best.Throughput, best.Avail
		}
		w.Axes = champsFromBoard(p)
		layers = append(layers, w)
		for _, r := range b.Leaderboard {
			if r.Status != "ok" {
				continue
			}
			top = append(top, Tagged{Tide: p.Name, URL: p.URL, Result: r})
		}
	}
	sort.SliceStable(layers, func(i, j int) bool {
		if layers[i].Score != layers[j].Score {
			return layers[i].Score > layers[j].Score
		}
		return layers[i].Tide < layers[j].Tide
	})
	sort.SliceStable(top, func(i, j int) bool {
		return top[i].Result.Snapshot.Score > top[j].Result.Snapshot.Score
	})
	if len(top) > 20 {
		top = top[:20]
	}
	h.Layers = layers
	h.CombinedTop = top
	h.ModeVotes = voteOf(layers, func(l LayerWinner) string { return l.Mode })
	h.DTypeVotes = voteOf(layers, func(l LayerWinner) string { return l.DType })
	h.ArchVotes = voteOf(layers, func(l LayerWinner) string { return l.Arch })
	if len(h.ModeVotes) > 0 {
		h.BestMode = h.ModeVotes[0].Key
	}
	if len(h.DTypeVotes) > 0 {
		h.BestDType = h.DTypeVotes[0].Key
	}
	if len(h.ArchVotes) > 0 {
		h.BestArch = h.ArchVotes[0].Key
	}
	h.Axes = oceanAxes(peers)
	h.DefaultMode, h.DefaultDType, h.DefaultArch, h.DefaultWins = defaultRecipe(h.Axes)
	return h
}

func fillLayer(w *LayerWinner, r *pulse.Result) {
	if w == nil || r == nil {
		return
	}
	w.Mode = string(r.Cell.Mode)
	w.DType = r.Cell.DType.String()
	w.Format = r.Cell.Format.String()
	w.Arch = r.Cell.ArchTag()
	w.CellID = r.Cell.ID
	s := r.Snapshot
	w.Score = s.Score
	w.SoftAcc = s.SoftAcc
	w.Accuracy = s.AvgAccuracy
	w.Thru = s.Throughput
	w.Avail = s.Availability
	w.Adapt = s.AdaptPct
	w.AccPerSec = s.AccPerSec
	w.Keep = dash.KeepLearn(s)
}

func champsFromBoard(p PeerState) []AxisChamp {
	src := p.Board.Axes
	if len(src) == 0 {
		src = dash.LucyAxes(p.Board)
	}
	out := make([]AxisChamp, 0, len(src))
	for _, a := range src {
		out = append(out, AxisChamp{
			Name: a.Name, Hint: a.Hint, Tide: p.Name, URL: p.URL,
			CellID: a.CellID, Mode: a.Mode, DType: a.DType, Format: a.Format, Arch: a.Arch,
			Value: a.Value, LowerBetter: a.LowerBetter,
			Score: a.Score, SoftAcc: a.SoftAcc, Thru: a.Thru, Avail: a.Avail, Adapt: a.Adapt,
		})
	}
	return out
}

func oceanAxes(peers []PeerState) []AxisChamp {
	byName := map[string]AxisChamp{}
	lower := map[string]bool{}
	var order []string
	for _, p := range peers {
		if !p.OK {
			continue
		}
		for _, a := range champsFromBoard(p) {
			if a.Value <= 0 {
				continue
			}
			if a.LowerBetter {
				lower[a.Name] = true
			}
			prev, ok := byName[a.Name]
			win := !ok
			if ok && lower[a.Name] {
				win = a.Value < prev.Value
			} else if ok {
				win = a.Value > prev.Value
			}
			if win {
				if !ok {
					order = append(order, a.Name)
				}
				byName[a.Name] = a
			}
		}
	}
	out := make([]AxisChamp, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func defaultRecipe(axes []AxisChamp) (mode, dtype, arch string, wins int) {
	type acc struct{ n int }
	modes, dtypes, arches := map[string]*acc{}, map[string]*acc{}, map[string]*acc{}
	bump := func(m map[string]*acc, k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		a := m[k]
		if a == nil {
			a = &acc{}
			m[k] = a
		}
		a.n++
	}
	for _, ax := range axes {
		bump(modes, ax.Mode)
		bump(dtypes, ax.DType)
		bump(arches, ax.Arch)
	}
	top := func(m map[string]*acc) (string, int) {
		bestK, bestN := "", 0
		for k, a := range m {
			if a.n > bestN || (a.n == bestN && k < bestK) {
				bestK, bestN = k, a.n
			}
		}
		return bestK, bestN
	}
	mode, mw := top(modes)
	dtype, _ = top(dtypes)
	arch, _ = top(arches)
	return mode, dtype, arch, mw
}

func voteOf(layers []LayerWinner, keyFn func(LayerWinner) string) []Vote {
	type acc struct {
		n int
		s float64
	}
	order := make([]string, 0)
	seen := map[string]bool{}
	by := map[string]*acc{}
	for _, l := range layers {
		k := strings.TrimSpace(keyFn(l))
		if k == "" || l.Score <= 0 {
			continue
		}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
		a := by[k]
		if a == nil {
			a = &acc{}
			by[k] = a
		}
		a.n++
		a.s += l.Score
	}
	out := make([]Vote, 0, len(order))
	for _, k := range order {
		a := by[k]
		mean := 0.0
		if a.n > 0 {
			mean = a.s / float64(a.n)
		}
		out = append(out, Vote{Key: k, Count: a.n, Mean: mean})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Mean > out[j].Mean
	})
	return out
}
