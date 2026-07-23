// Package permute builds numerical-type × quant × train-mode matrices.
package permute

import (
	"fmt"

	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
	"github.com/openfluke/welvet/simd"
)

// TrainMode is a serve+train path (Lucy-style).
type TrainMode string

const (
	ModeSGD            TrainMode = "sgd"
	ModeSGDSimd        TrainMode = "sgd_simd"
	ModeTweenHead      TrainMode = "tween_head"
	ModeTweenHeadSimd  TrainMode = "tween_head_simd"
)

// Cell is one permutation to benchmark.
type Cell struct {
	ID       string         `json:"id"`
	DType    core.DType     `json:"dtype"`
	Format   quant.Format   `json:"format"`
	Mode     TrainMode      `json:"mode"`
	Backend  core.Backend   `json:"backend"`
	UseSIMD  bool           `json:"use_simd"`
}

func (c Cell) String() string {
	return fmt.Sprintf("%s|%s|%s|%s", c.DType, c.Format, c.Mode, c.Backend)
}

// Config controls which axes expand.
type Config struct {
	DTypes  []core.DType
	Formats []quant.Format
	Modes   []TrainMode
}

// Smoke is a fast dashboard-friendly subset.
func Smoke() Config {
	return Config{
		DTypes: []core.DType{
			core.DTypeFloat32, core.DTypeFloat16, core.DTypeInt8, core.DTypeInt4,
		},
		Formats: []quant.Format{
			quant.FormatNone, quant.FormatQ8_0, quant.FormatQ4_0, quant.FormatQ4_K,
		},
		Modes: []TrainMode{ModeSGD, ModeSGDSimd, ModeTweenHead, ModeTweenHeadSimd},
	}
}

// Full expands Welvet's FormatNone dtypes + packed quants + all train modes.
func Full() Config {
	fmts := make([]quant.Format, 0, len(quant.AllFormats))
	for _, f := range quant.AllFormats {
		fmts = append(fmts, f)
	}
	return Config{
		DTypes:  append([]core.DType(nil), core.AllDTypes...),
		Formats: fmts,
		Modes:   []TrainMode{ModeSGD, ModeSGDSimd, ModeTweenHead, ModeTweenHeadSimd},
	}
}

// KQuant focuses on k-quant packs × a few host dtypes.
func KQuant() Config {
	return Config{
		DTypes: []core.DType{core.DTypeFloat32, core.DTypeInt8},
		Formats: []quant.Format{
			quant.FormatQ2_K, quant.FormatQ3_K, quant.FormatQ4_K,
			quant.FormatQ5_K, quant.FormatQ6_K,
		},
		Modes: []TrainMode{ModeSGD, ModeSGDSimd, ModeTweenHead, ModeTweenHeadSimd},
	}
}

// Expand builds the cartesian product. FormatNone cells use DType demotion;
// packed-format cells keep float32 source then Pack (down-the-dem style).
func Expand(cfg Config) []Cell {
	var out []Cell
	simdOK := simd.Enabled()
	for _, mode := range cfg.Modes {
		useSIMD := mode == ModeSGDSimd || mode == ModeTweenHeadSimd
		be := core.BackendCPUTiled
		if useSIMD {
			be = core.BackendSIMD
		}
		for _, dt := range cfg.DTypes {
			for _, f := range cfg.Formats {
				// Packed formats always start from float32 weights (Pack truth).
				cellDT := dt
				if f != quant.FormatNone {
					cellDT = core.DTypeFloat32
				}
				// Skip redundant dtype×format duplicates for packed (one dtype axis).
				if f != quant.FormatNone && dt != core.DTypeFloat32 {
					continue
				}
				c := Cell{
					DType:   cellDT,
					Format:  f,
					Mode:    mode,
					Backend: be,
					UseSIMD: useSIMD && simdOK,
				}
				c.ID = c.String()
				out = append(out, c)
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
