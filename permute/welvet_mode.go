package permute

import (
	"fmt"
	"strings"

	"github.com/openfluke/welvet/layers/parallel"
)

// AllModes is Lucy's original 6 tokens (checkpoint-stable) plus every other
// named Welvet TrainMode (Split / Alt / FastProxy / Sparse / Mesh*, …).
// Inherit is omitted. Old IDs keep sgd/step_sgd/tween… so resume skips epoch-1 work.
func AllModes() []TrainMode {
	out := append([]TrainMode(nil), LucyModes()...)
	legacy := map[string]bool{
		"NormalBP": true, "StepBP": true,
		"Tween": true, "TweenChain": true,
		"StepTween": true, "StepTweenChain": true,
	}
	for _, wm := range parallel.AllNamedTrainModes() {
		if legacy[wm.String()] {
			continue
		}
		out = append(out, TrainMode(wm.String()))
	}
	return out
}

// CamsOf is 1 for a single CNN head, 2 for bicameral, 3 for tricameral.
func CamsOf(arch ArchKind) int {
	switch arch {
	case ArchTricameral:
		return 3
	case ArchBicameral:
		return 2
	default:
		return 1
	}
}

// ArchTag is a short dashboard label: "single×1", "bicameral×2", "tricameral×3".
func (c Cell) ArchTag() string {
	n := c.Cams
	if n <= 0 {
		n = CamsOf(c.Arch)
	}
	if n <= 1 {
		return "single×1"
	}
	arch := string(c.Arch)
	if arch == "" {
		arch = "cameral"
	}
	return fmt.Sprintf("%s×%d", arch, n)
}

// Welvet maps a permute train token onto layers/parallel.TrainMode.
func (m TrainMode) Welvet() (parallel.TrainMode, error) {
	switch m {
	case ModeSGD:
		return parallel.ModeNormalBP, nil
	case ModeStepSGD:
		return parallel.ModeStepBP, nil
	case ModeTween:
		return parallel.ModeTween, nil
	case ModeTweenChain:
		return parallel.ModeTweenChain, nil
	case ModeStepTween:
		return parallel.ModeStepTween, nil
	case ModeStepTweenChain:
		return parallel.ModeStepTweenChain, nil
	default:
		return parallel.ParseTrainMode(string(m))
	}
}

// IsStepSched is the Lucy 3-forward / 1-update schedule (legacy step_* only).
func (m TrainMode) IsStepSched() bool {
	switch m {
	case ModeStepSGD, ModeStepTween, ModeStepTweenChain:
		return true
	default:
		s := strings.ToLower(string(m))
		return strings.HasPrefix(s, "step") && !strings.Contains(s, "mesh")
	}
}

// ParseModes splits a csv of train-mode tokens (sgd, TweenSplit, Sparse, …).
// Empty or "all" means no filter. Unknown tokens error.
func ParseModes(spec string) ([]TrainMode, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "all") {
		return nil, nil
	}
	by := map[string]TrainMode{
		"normalbp": ModeSGD,
		"stepbp":   ModeStepSGD,
	}
	for _, m := range AllModes() {
		by[strings.ToLower(string(m))] = m
	}
	var out []TrainMode
	seen := map[TrainMode]bool{}
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		m, ok := by[strings.ToLower(tok)]
		if !ok {
			return nil, fmt.Errorf("unknown train mode %q", tok)
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no train modes in %q", spec)
	}
	return out, nil
}
