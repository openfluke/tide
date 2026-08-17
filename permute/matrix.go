// Package permute builds numerical-type × quant × train-mode × arch matrices.
// Backend is always SIMD (no CPU-tiled twin modes).
package permute

import (
	"fmt"
	"strings"

	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
	"github.com/openfluke/welvet/simd"
)

// TrainMode is a serve+train path (Lucy / test41-w suite @ SIMD).
type TrainMode string

const (
	ModeSGD            TrainMode = "sgd"              // NormalBP — keep this token; checkpoints key on it
	ModeStepSGD        TrainMode = "step_sgd"         // StepBP
	ModeTween          TrainMode = "tween"            // Tween
	ModeTweenChain     TrainMode = "tween_chain"      // TweenChain
	ModeStepTween      TrainMode = "step_tween"       // StepTween
	ModeStepTweenChain TrainMode = "step_tween_chain" // StepTweenChain
)

// ArchKind is cameral topology (1 / 2 / 3 heads). It is not a layer type —
// live_mnist uses a CNN stem, live_gpt uses MHA; both are "single" at cams=1.
type ArchKind string

const (
	ArchSingle     ArchKind = "single"     // 1 cam
	ArchCNN        ArchKind = ArchSingle   // frozen alias; old cell IDs used "cnn"
	ArchBicameral  ArchKind = "bicameral"  // Parallel 2×Dense, add (cams=2)
	ArchTricameral ArchKind = "tricameral" // Parallel 3×Dense, add (cams=3)
)

// LucyModes is the original test41 sequential suite. Token strings are frozen
// so epoch-1 checkpoints keep matching after new Welvet modes are appended.
func LucyModes() []TrainMode {
	return []TrainMode{
		ModeSGD, ModeStepSGD,
		ModeTween, ModeTweenChain,
		ModeStepTween, ModeStepTweenChain,
	}
}

// AllArches is single + Bi + Tri cameral.
func AllArches() []ArchKind {
	return []ArchKind{ArchSingle, ArchBicameral, ArchTricameral}
}

// Cell is one permutation to benchmark.
type Cell struct {
	ID      string       `json:"id"`
	DType   core.DType   `json:"dtype"`
	Format  quant.Format `json:"format"`
	Mode    TrainMode    `json:"mode"`
	Arch    ArchKind     `json:"arch"`
	Cams    int          `json:"cams,omitempty"` // 1=single, 2=bi, 3=tri
	Backend core.Backend `json:"backend"`
	UseSIMD bool         `json:"use_simd"`
}

func (c Cell) String() string {
	return fmt.Sprintf("%s|%s|%s|%s|simd", c.DType, c.Format, c.Mode, CanonicalArch(c.Arch))
}

// CanonicalArch maps legacy "cnn" (and empty) onto single.
func CanonicalArch(a ArchKind) ArchKind {
	switch strings.ToLower(strings.TrimSpace(string(a))) {
	case "", "cnn", "single", "1":
		return ArchSingle
	case "bicameral", "bi", "2":
		return ArchBicameral
	case "tricameral", "tri", "3":
		return ArchTricameral
	default:
		return a
	}
}

// NormalizeCellID rewrites legacy |cnn| tokens so checkpoints resume after the
// arch axis was renamed to single.
func NormalizeCellID(id string) string {
	return strings.ReplaceAll(id, "|cnn|", "|single|")
}

// IDAliases is the current ID plus the pre-rename form (cnn ↔ single).
func IDAliases(id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	n := NormalizeCellID(id)
	legacy := strings.ReplaceAll(n, "|single|", "|cnn|")
	out := []string{n}
	if legacy != n {
		out = append(out, legacy)
	}
	if id != n && id != legacy {
		out = append(out, id)
	}
	return out
}

// IDDone reports whether any alias of id is in done.
func IDDone(done map[string]bool, id string) bool {
	if done == nil {
		return false
	}
	for _, a := range IDAliases(id) {
		if done[a] {
			return true
		}
	}
	return false
}

// Config controls which axes expand.
type Config struct {
	DTypes  []core.DType
	Formats []quant.Format
	Modes   []TrainMode
	Arches  []ArchKind
}

// Smoke is a fast dashboard-friendly subset.
func Smoke() Config {
	return Config{
		DTypes: []core.DType{
			core.DTypeFloat32, core.DTypeFloat16, core.DTypeInt8,
		},
		Formats: []quant.Format{
			quant.FormatNone, quant.FormatQ8_0, quant.FormatQ4_K,
		},
		Modes:  AllModes(),
		Arches: AllArches(),
	}
}

// Full expands Welvet's FormatNone dtypes + packed quants + all train modes × arches.
func Full() Config {
	fmts := make([]quant.Format, 0, len(quant.AllFormats))
	for _, f := range quant.AllFormats {
		fmts = append(fmts, f)
	}
	return Config{
		DTypes:  append([]core.DType(nil), core.AllDTypes...),
		Formats: fmts,
		Modes:   AllModes(),
		Arches:  AllArches(),
	}
}

// Sprint is the quick_sprint matrix: every native dtype × FormatNone × every
// train mode × single/bi/tri cameral. Packed quants stay opt-in so a layer
// sprint finishes in minutes, not a week. One epoch is the whole point.
func Sprint() Config {
	return Config{
		DTypes:  append([]core.DType(nil), core.AllDTypes...),
		Formats: []quant.Format{quant.FormatNone},
		Modes:   AllModes(),
		Arches:  AllArches(),
	}
}

// Screen is the cheap first pass: Lucy 6 × single × full numeric axis.
// Promote winners onto bi/tri and extra Welvet modes after this.
func Screen() Config {
	f := Full()
	return Config{
		DTypes:  f.DTypes,
		Formats: f.Formats,
		Modes:   LucyModes(),
		Arches:  []ArchKind{ArchSingle},
	}
}

// KQuant focuses on k-quant packs × Lucy train modes × arches.
func KQuant() Config {
	return Config{
		DTypes: []core.DType{core.DTypeFloat32},
		Formats: []quant.Format{
			quant.FormatQ2_K, quant.FormatQ3_K, quant.FormatQ4_K,
			quant.FormatQ5_K, quant.FormatQ6_K,
		},
		Modes:  LucyModes(),
		Arches: AllArches(),
	}
}

// Expand builds the cartesian product. Always BackendSIMD.
// FormatNone cells use DType demotion; packed-format cells keep float32 source then Pack.
func Expand(cfg Config) []Cell {
	arches := cfg.Arches
	if len(arches) == 0 {
		arches = AllArches()
	}
	var out []Cell
	simdOK := simd.Enabled()
	for _, mode := range cfg.Modes {
		for _, arch := range arches {
			arch = CanonicalArch(arch)
			for _, dt := range cfg.DTypes {
				for _, f := range cfg.Formats {
					cellDT := dt
					if f != quant.FormatNone {
						cellDT = core.DTypeFloat32
					}
					if f != quant.FormatNone && dt != core.DTypeFloat32 {
						continue
					}
					c := Cell{
						DType:   cellDT,
						Format:  f,
						Mode:    mode,
						Arch:    arch,
						Cams:    CamsOf(arch),
						Backend: core.BackendSIMD,
						UseSIMD: simdOK,
					}
					c.ID = c.String()
					out = append(out, c)
				}
			}
		}
	}
	return out
}

// Batch splits cells into chunks for paced dashboard runs.
func Batch(cells []Cell, size int) [][]Cell {
	if size < 1 {
		size = 1
	}
	var batches [][]Cell
	for i := 0; i < len(cells); i += size {
		end := i + size
		if end > len(cells) {
			end = len(cells)
		}
		batches = append(batches, cells[i:end])
	}
	return batches
}
