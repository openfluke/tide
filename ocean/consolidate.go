package ocean

import (
	"sort"
	"strings"

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
	Ok       int     `json:"ok"`
	Gap      int     `json:"gap"`
	Fail     int     `json:"fail"`
	Done     int     `json:"done"`
	Total    int     `json:"total"`
}

// Vote is a plurality count with mean Score of the layers that picked this key.
type Vote struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Mean  float64 `json:"mean_score"`
}

// Tagged is a leaderboard row tagged with which tide it came from.
type Tagged struct {
	Tide     string       `json:"tide"`
	URL      string       `json:"url"`
	Result   pulse.Result `json:"result"`
}

// Holistic is the master consolidation across all linked tides.
type Holistic struct {
	BestMode      string        `json:"best_mode"`
	BestDType     string        `json:"best_dtype"`
	ModeVotes     []Vote        `json:"mode_votes"`
	DTypeVotes    []Vote        `json:"dtype_votes"`
	Layers        []LayerWinner `json:"layers"`
	CombinedTop   []Tagged      `json:"combined_top"`
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
		h.CellsDone += b.Ok + b.Gap + b.Fail
		h.CellsTotal += b.CellTotal
		w := LayerWinner{
			Tide:  p.Name,
			URL:   p.URL,
			Ok:    b.Ok,
			Gap:   b.Gap,
			Fail:  b.Fail,
			Done:  b.Ok + b.Gap + b.Fail,
			Total: b.CellTotal,
		}
		if b.Best.Score != nil {
			r := b.Best.Score
			w.Mode = string(r.Cell.Mode)
			w.DType = r.Cell.DType.String()
			w.Format = r.Cell.Format.String()
			w.Arch = r.Cell.ArchTag()
			w.CellID = r.Cell.ID
			w.Score = r.Snapshot.Score
			w.SoftAcc = r.Snapshot.SoftAcc
			w.Accuracy = r.Snapshot.AvgAccuracy
		} else if len(b.Winners.BestSettingsPerMode) > 0 {
			best := b.Winners.BestSettingsPerMode[0]
			w.Mode, w.DType, w.Format = best.Mode, best.DType, best.Format
			w.Arch, w.CellID = best.Arch, best.CellID
			w.Score, w.SoftAcc, w.Accuracy = best.Score, best.SoftAcc, best.Accuracy
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
	if len(h.ModeVotes) > 0 {
		h.BestMode = h.ModeVotes[0].Key
	}
	if len(h.DTypeVotes) > 0 {
		h.BestDType = h.DTypeVotes[0].Key
	}
	return h
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
		if k == "" {
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
