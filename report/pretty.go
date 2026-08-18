package report

import "strings"

// PrettyCell rewrites legacy |cnn| cameral tokens to |single|.
func PrettyCell(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	return strings.ReplaceAll(id, "|cnn|", "|single|")
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
