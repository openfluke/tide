package river

import (
	"sort"
	"strconv"
	"time"

	"github.com/openfluke/tide/permute"
)

// PlanProgress is mnist_baseline's own truth check: results.json ∩ plan.
type PlanProgress struct {
	Plan       int                `json:"plan"`
	Done       int                `json:"done"`
	Left       int                `json:"left"`
	Pct        float64            `json:"pct"`
	RatePerHr  float64            `json:"rate_per_hr"`  // recent finish rate
	ETASeconds float64            `json:"eta_seconds"`  // 0 if done / unknown
	ETAHuman   string             `json:"eta_human"`
	Window     string             `json:"window"` // e.g. "last 15m · 85 cells"
	Complete   bool               `json:"complete"`
	ByArch     map[string]ArchProg `json:"by_arch,omitempty"`
	Missing    []string           `json:"missing,omitempty"` // capped sample
}

// ArchProg is done/left for one cameral arch in the plan.
type ArchProg struct {
	Plan int `json:"plan"`
	Done int `json:"done"`
	Left int `json:"left"`
}

func (s *Store) SetPlan(cells []permute.Cell) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(cells))
	for _, c := range cells {
		if c.ID != "" {
			ids = append(ids, c.ID)
		}
	}
	s.planIDs = ids
}

func (s *Store) PlanIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.planIDs...)
}

// ResultDoneIDs are ok/gap cell IDs currently in results.json (skip set truth).
func (s *Store) ResultDoneIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byID))
	for id, r := range s.byID {
		if r.Status == "ok" || r.Status == "gap" || r.Status == "" {
			out = append(out, id)
		}
	}
	return out
}

// Progress computes done/left/%/ETA from results vs the sweep plan.
func (s *Store) Progress() PlanProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return computeProgress(s.planIDs, s.byID, time.Now())
}

func computeProgress(planIDs []string, byID map[string]Row, now time.Time) PlanProgress {
	out := PlanProgress{
		Plan:   len(planIDs),
		ByArch: map[string]ArchProg{},
	}
	if len(planIDs) == 0 {
		// Fall back to unique results count only.
		out.Done = len(byID)
		out.Pct = 0
		out.ETAHuman = "—"
		return out
	}

	have := map[string]bool{}
	for id, r := range byID {
		if r.Status != "" && r.Status != "ok" && r.Status != "gap" {
			continue
		}
		for _, a := range permute.IDAliases(id) {
			have[a] = true
		}
	}

	var missing []string
	var finished []time.Time
	for _, id := range planIDs {
		arch := archFromID(id)
		ap := out.ByArch[arch]
		ap.Plan++
		if permute.IDDone(have, id) {
			out.Done++
			ap.Done++
			if r, ok := byID[id]; ok {
				if t, err := time.Parse(time.RFC3339, r.FinishedAt); err == nil {
					finished = append(finished, t.UTC())
				}
			}
		} else {
			out.Left++
			ap.Left++
			if len(missing) < 24 {
				missing = append(missing, id)
			}
		}
		out.ByArch[arch] = ap
	}
	out.Missing = missing
	if out.Plan > 0 {
		out.Pct = 100 * float64(out.Done) / float64(out.Plan)
	}
	out.Complete = out.Left == 0 && out.Plan > 0

	rate, window := finishRate(finished, now)
	out.RatePerHr = rate
	out.Window = window
	if out.Complete {
		out.ETAHuman = "done"
		out.ETASeconds = 0
		return out
	}
	if rate > 0 && out.Left > 0 {
		sec := float64(out.Left) / rate * 3600
		out.ETASeconds = sec
		// Explicit: ETA is for remaining cells only, at recent finish pace.
		out.ETAHuman = formatETA(sec) + " for " + strconv.Itoa(out.Left) + " left"
	} else {
		out.ETAHuman = "—"
	}
	return out
}

func archFromID(id string) string {
	// dtype|fmt|mode|arch|backend|lr=…
	parts := splitPipe(id)
	if len(parts) >= 4 {
		return string(permute.CanonicalArch(permute.ArchKind(parts[3])))
	}
	return "?"
}

func splitPipe(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// finishRate picks the shortest recent window with enough samples.
func finishRate(finished []time.Time, now time.Time) (ratePerHr float64, window string) {
	if len(finished) < 2 {
		return 0, ""
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].Before(finished[j]) })
	anchor := now.UTC()
	last := finished[len(finished)-1]
	if last.After(anchor) {
		anchor = last
	}
	type cand struct {
		label string
		dur   time.Duration
		minN  int
	}
	for _, c := range []cand{
		{"last 15m", 15 * time.Minute, 5},
		{"last 1h", time.Hour, 5},
		{"last 2h", 2 * time.Hour, 5},
	} {
		cut := anchor.Add(-c.dur)
		n := 0
		for _, t := range finished {
			if !t.Before(cut) {
				n++
			}
		}
		if n >= c.minN {
			return float64(n) / c.dur.Hours(), c.label + " · " + strconv.Itoa(n) + " cells"
		}
	}
	span := last.Sub(finished[0]).Hours()
	if span > 0.05 {
		n := len(finished) - 1
		return float64(n) / span, "overall · " + strconv.Itoa(n) + " cells"
	}
	return 0, ""
}

func formatETA(sec float64) string {
	if sec < 60 {
		return "<1m"
	}
	if sec < 3600 {
		m := int(sec/60 + 0.5)
		return strconv.Itoa(m) + "m"
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	if h >= 48 {
		d := h / 24
		return strconv.Itoa(d) + "d " + strconv.Itoa(h%24) + "h"
	}
	if m == 0 {
		return strconv.Itoa(h) + "h"
	}
	return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
}
