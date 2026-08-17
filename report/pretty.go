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
