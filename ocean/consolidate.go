package ocean

import (
	"sort"
	"strings"

	"github.com/openfluke/tide/dash"
	"github.com/openfluke/tide/metrics"
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

func oceanAxes(peers []PeerState) []AxisChamp {
	type spec struct {
		name, hint string
		get        func(dash.Board) *pulse.Result
		val        func(metrics.Snapshot) float64
		higher     bool
	}
	specs := []spec{
		{"score", "T x Avail x SoftAcc / 10,000", func(b dash.Board) *pulse.Result { return b.Best.Score }, func(s metrics.Snapshot) float64 { return s.Score }, true},
		{"soft_acc", "class-mass SoftAcc (adaptation quality)", func(b dash.Board) *pulse.Result { return b.BestSoft }, func(s metrics.Snapshot) float64 { return s.SoftAcc }, true},
		{"hard_acc", "argmax accuracy", func(b dash.Board) *pulse.Result { return b.BestHard }, func(s metrics.Snapshot) float64 { return s.AvgAccuracy }, true},
		{"throughput", "outputs / second (fast realtime)", func(b dash.Board) *pulse.Result { return b.Best.Throughput }, func(s metrics.Snapshot) float64 { return s.Throughput }, true},
		{"availability", "infer / (infer+train) duty cycle", func(b dash.Board) *pulse.Result { return b.Best.Availability }, func(s metrics.Snapshot) float64 { return s.Availability }, true},
		{"acc_thru", "SoftAcc x Throughput / 100", func(b dash.Board) *pulse.Result { return b.BestAccThru }, dash.AccThru, true},
		{"realtime", "Throughput x Availability / 100", func(b dash.Board) *pulse.Result { return b.BestRealtime }, dash.Realtime, true},
		{"adapt", "AdaptPct after phase switches", func(b dash.Board) *pulse.Result { return b.BestAdapt }, func(s metrics.Snapshot) float64 { return s.AdaptPct }, true},
		{"keep_learn", "late SoftAcc still rising (not plateau)", func(b dash.Board) *pulse.Result { return b.BestKeep }, dash.KeepLearn, true},
		{"acc_per_sec", "SoftAcc gained per wall second", func(b dash.Board) *pulse.Result {
			if b.BestLearn.AccPerSec != nil {
				return b.BestLearn.AccPerSec
			}
			return nil
		}, func(s metrics.Snapshot) float64 { return s.AccPerSec }, true},
		{"time_to_50", "seconds to 50% window acc (lower better)", func(b dash.Board) *pulse.Result {
			if b.BestLearn.To50 != nil {
				return b.BestLearn.To50
			}
			return nil
		}, func(s metrics.Snapshot) float64 { return s.TimeToAcc50Sec }, false},
		{"consistency", "share of windows above SoftAcc threshold", func(b dash.Board) *pulse.Result { return b.BestConsistency }, func(s metrics.Snapshot) float64 { return s.Consistency }, true},
		{"stability", "low SoftAcc variance after switches", func(b dash.Board) *pulse.Result { return b.BestStability }, func(s metrics.Snapshot) float64 { return s.Stability }, true},
		{"mobile_score", "Score per MiB", func(b dash.Board) *pulse.Result { return b.BestMobile.Score }, func(s metrics.Snapshot) float64 { return s.MobileScore }, true},
	}
	out := make([]AxisChamp, 0, len(specs))
	for _, sp := range specs {
		if c, ok := pickAxis(peers, sp.name, sp.hint, sp.get, sp.val, sp.higher); ok {
			out = append(out, c)
		}
	}
	return out
}

func pickAxis(peers []PeerState, name, hint string, get func(dash.Board) *pulse.Result, val func(metrics.Snapshot) float64, higher bool) (AxisChamp, bool) {
	var best AxisChamp
	found := false
	for _, p := range peers {
		if !p.OK {
			continue
		}
		r := get(p.Board)
		if r == nil {
			continue
		}
		v := val(r.Snapshot)
		if v <= 0 {
			continue
		}
		if !found || (higher && v > best.Value) || (!higher && v < best.Value) {
			found = true
			best = champOf(name, hint, p, r, v)
		}
	}
	return best, found
}

func champOf(name, hint string, p PeerState, r *pulse.Result, value float64) AxisChamp {
	s := r.Snapshot
	return AxisChamp{
		Name: name, Hint: hint,
		Tide: p.Name, URL: p.URL, CellID: r.Cell.ID,
		Mode: string(r.Cell.Mode), DType: r.Cell.DType.String(),
		Format: r.Cell.Format.String(), Arch: r.Cell.ArchTag(),
		Value: value, Score: s.Score, SoftAcc: s.SoftAcc,
		Thru: s.Throughput, Avail: s.Availability, Adapt: s.AdaptPct,
	}
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
