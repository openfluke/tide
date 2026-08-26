package report

import (
	"strconv"
	"strings"
)

// ParseLRFromCellID reads the trailing |lr=… token from test53-style cell IDs.
func ParseLRFromCellID(id string) (value float64, label string, ok bool) {
	id = strings.TrimSpace(id)
	lower := strings.ToLower(id)
	i := strings.LastIndex(lower, "|lr=")
	if i < 0 {
		return 0, "", false
	}
	label = strings.TrimSpace(id[i+4:])
	value, ok = ParseLRLabel(label)
	return value, label, ok
}

// ParseLRLabel parses funny-LR tokens: 0.02, 2, 200, 2k, 1m, 100m, -0.02, etc.
func ParseLRLabel(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = strings.TrimPrefix(s, "-")
	}
	mul := 1.0
	switch {
	case strings.HasSuffix(s, "m"):
		mul = 1e6
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mul = 1e3
		s = strings.TrimSuffix(s, "k")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		f = -f
	}
	return f * mul, true
}

// RecipeKey is the cell identity without the lr= suffix (layer|dtype|mode).
func RecipeKey(id string) string {
	id = PrettyCell(id)
	lower := strings.ToLower(id)
	if i := strings.LastIndex(lower, "|lr="); i >= 0 {
		return id[:i]
	}
	return id
}

// ParseMachineFromPeer takes the machine prefix from peer names like m4-lo, m5_hi.
func ParseMachineFromPeer(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "?"
	}
	for _, sep := range []string{"-", "_"} {
		if i := strings.Index(name, sep); i > 0 {
			return name[:i]
		}
	}
	return name
}
