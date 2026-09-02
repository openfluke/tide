package river

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openfluke/tide/permute"
)

// DefaultLR is the host default learning rate when a cell has none.
const DefaultLR = 0.6

// ParseLRs expands a CSV of rates (0.6,1.2,20k). Empty → DefaultLR.
func ParseLRs(spec string) ([]float64, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" {
		return []float64{DefaultLR}, nil
	}
	parts := strings.Split(spec, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := parseLRToken(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no learning rates in %q", spec)
	}
	return out, nil
}

func parseLRToken(tok string) (float64, error) {
	tok = strings.TrimSpace(strings.ToLower(tok))
	mult := 1.0
	switch {
	case strings.HasSuffix(tok, "m"):
		mult = 1e6
		tok = strings.TrimSuffix(tok, "m")
	case strings.HasSuffix(tok, "k"):
		mult = 1e3
		tok = strings.TrimSuffix(tok, "k")
	}
	v, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, fmt.Errorf("lr %q: %w", tok, err)
	}
	return v * mult, nil
}

// FormatLR is a short label for cell IDs / charts (0.6, 20k, 1m).
func FormatLR(lr float64) string {
	neg := lr < 0
	x := lr
	if neg {
		x = -lr
	}
	var s string
	switch {
	case x >= 1e6 && x == float64(int64(x/1e6))*1e6:
		s = fmt.Sprintf("%gm", x/1e6)
	case x >= 1e3 && x == float64(int64(x/1e3))*1e3:
		s = fmt.Sprintf("%gk", x/1e3)
	default:
		s = strconv.FormatFloat(x, 'g', -1, 64)
	}
	if neg {
		return "-" + s
	}
	return s
}

// LRsCSV joins learning-rate labels for logs / Tide subtitles.
func LRsCSV(lrs []float64) string {
	parts := make([]string, len(lrs))
	for i, lr := range lrs {
		parts[i] = FormatLR(lr)
	}
	return strings.Join(parts, ",")
}

// ExpandWithLRs duplicates cells across learning rates.
// IDs always get "|lr=<label>" so later LR runs skip finished baselines cleanly.
func ExpandWithLRs(cells []permute.Cell, lrs []float64) ([]permute.Cell, map[string]float64) {
	if len(lrs) == 0 {
		lrs = []float64{DefaultLR}
	}
	cellLR := make(map[string]float64, len(cells)*len(lrs))
	out := make([]permute.Cell, 0, len(cells)*len(lrs))
	for _, lr := range lrs {
		tag := FormatLR(lr)
		for _, c := range cells {
			nc := c
			nc.ID = c.ID + "|lr=" + tag
			out = append(out, nc)
			cellLR[nc.ID] = lr
		}
	}
	return out, cellLR
}
