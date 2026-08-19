package report

import (
	"strconv"
	"strings"

	"github.com/openfluke/welvet/layers/parallel"
)

// FormatLR is the learning-rate label for dash / ocean / PDF (e.g. 0.02).
func FormatLR(lr float64) string {
	if lr <= 0 {
		return "unset"
	}
	return strconv.FormatFloat(lr, 'g', 6, 64)
}

// ModeLegend is the compact train-mode key printed on dash / ocean / PDF covers.
func ModeLegend() string {
	return parallel.ShortTrainModeLegend
}

// PrettyMode shortens train-mode titles for tables, heatmaps, and chips.
func PrettyMode(s string) string {
	return parallel.ShortTrainMode(s)
}

// PrettyModes maps PrettyMode over a slice (heat axis labels).
func PrettyModes(xs []string) []string {
	if len(xs) == 0 {
		return xs
	}
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = PrettyMode(x)
	}
	return out
}

// PrettyCell rewrites legacy |cnn| cameral tokens to |single|.
func PrettyCell(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	return shortenCellMode(strings.ReplaceAll(id, "|cnn|", "|single|"))
}

func shortenCellMode(id string) string {
	parts := strings.Split(id, "|")
	if len(parts) < 3 {
		return id
	}
	parts[2] = PrettyMode(parts[2])
	return strings.Join(parts, "|")
}

// CompactCell is PrettyCell with filler tokens dropped so IDs fit PDF tables.
// float32|none|sgd|single|simd → float32 sgd single
func CompactCell(id string) string {
	id = PrettyCell(id)
	if id == "" {
		return id
	}
	parts := strings.Split(id, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.EqualFold(p, "none") || strings.EqualFold(p, "simd") {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return id
	}
	return strings.Join(out, " ")
}

// PrettyArch maps the old cameral name onto single (display only).
func PrettyArch(a string) string {
	s := strings.TrimSpace(a)
	if s == "" {
		return s
	}
	low := strings.ToLower(s)
	switch low {
	case "cnn", "cnnx1", "1":
		return "single"
	}
	s = strings.ReplaceAll(s, "cnn×", "single×")
	s = strings.ReplaceAll(s, "cnnx", "singlex")
	s = strings.ReplaceAll(s, "CNN×", "single×")
	if strings.EqualFold(s, "cnn×1") || strings.EqualFold(s, "cnnx1") {
		return "single×1"
	}
	return s
}
