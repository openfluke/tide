package permute

import "strings"

// MixTagsFromID parses bm= and pat= tags from a cell ID (cam-mix sweeps).
func MixTagsFromID(id string) (branch []string, pattern string, ok bool) {
	id = NormalizeCellID(id)
	for _, p := range strings.Split(id, "|") {
		switch {
		case strings.HasPrefix(p, "bm="):
			raw := strings.TrimPrefix(p, "bm=")
			for _, m := range strings.Split(raw, "+") {
				m = strings.TrimSpace(m)
				if m != "" {
					branch = append(branch, m)
				}
			}
		case strings.HasPrefix(p, "pat="):
			pattern = strings.TrimPrefix(p, "pat=")
		}
	}
	return branch, pattern, len(branch) > 0
}

// QueueMode is the dashboard bucket for mode-progress tables.
// Uniform cells group by train mode; cam-mix cells (bm=…) group under "cam-mix".
func QueueMode(c Cell) string {
	if bm, _, mix := MixTagsFromID(c.ID); mix {
		_ = bm
		return "cam-mix"
	}
	if c.Mode != "" {
		return string(c.Mode)
	}
	return modeTokenFromID(c.ID)
}

func modeTokenFromID(id string) string {
	id = NormalizeCellID(id)
	if id == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(id, "|") {
		if p == "" || strings.HasPrefix(p, "lr=") || strings.HasPrefix(p, "bm=") ||
			strings.HasPrefix(p, "pat=") || strings.HasPrefix(p, "cs=") {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}
