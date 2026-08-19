package permute

import (
	"fmt"
	"strconv"
	"strings"
)

const maxCams = 256

// CamsRange is lo..hi inclusive (Welvet Parallel branch count).
func CamsRange(lo, hi int) []int {
	if lo < 1 {
		lo = 1
	}
	if hi < 1 {
		hi = 1
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi > maxCams {
		hi = maxCams
	}
	out := make([]int, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		out = append(out, n)
	}
	return out
}

// ArchForCams is the cell-ID arch token for a Welvet Parallel branch count.
// 1/2/3 keep the legacy names so live_mnist checkpoints still match.
func ArchForCams(n int) ArchKind {
	if n < 1 {
		n = 1
	}
	switch n {
	case 1:
		return ArchSingle
	case 2:
		return ArchBicameral
	case 3:
		return ArchTricameral
	default:
		return ArchKind(fmt.Sprintf("cameral×%d", n))
	}
}

// ParseCams reads a host cameral spec: "4-15", "8", "1,2,3", "single,tri",
// "cameral×12". Empty / "all" means no override (caller keeps Config.Cams / Arches).
func ParseCams(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "all") {
		return nil, nil
	}
	var out []int
	seen := map[int]bool{}
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		lo, hi, ok := parseCamRange(tok)
		if !ok {
			n := parseCamCount(tok)
			if n < 1 {
				return nil, fmt.Errorf("unknown cameral %q (use 4-15, 8, or cameral×12)", tok)
			}
			lo, hi = n, n
		}
		if lo < 1 || hi < 1 || hi > maxCams || lo > maxCams {
			return nil, fmt.Errorf("cameral out of range in %q (1–%d)", tok, maxCams)
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		for n := lo; n <= hi; n++ {
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no camerals in %q", spec)
	}
	return out, nil
}

func parseCamRange(tok string) (lo, hi int, ok bool) {
	tok = strings.ReplaceAll(tok, "×", "x")
	a, b, found := strings.Cut(tok, "-")
	if !found {
		return 0, 0, false
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(a))
	hi, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lo, hi, true
}

func parseCamCount(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "×", "x")
	switch s {
	case "", "cnn", "single", "1", "cnnx1":
		return 1
	case "bicameral", "bi", "2":
		return 2
	case "tricameral", "tri", "3":
		return 3
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= maxCams {
		return n
	}
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil || n < 1 || n > maxCams {
		return 0
	}
	prefix := strings.Trim(s[:i], "x-_")
	switch prefix {
	case "", "cameral", "cam", "cams", "cnn":
		return n
	default:
		return 0
	}
}

func expandArchList(cfg Config) []ArchKind {
	if len(cfg.Cams) > 0 {
		out := make([]ArchKind, 0, len(cfg.Cams))
		seen := map[int]bool{}
		for _, n := range cfg.Cams {
			if n < 1 || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, ArchForCams(n))
		}
		return out
	}
	arches := cfg.Arches
	if len(arches) == 0 {
		arches = AllArches()
	}
	return arches
}
