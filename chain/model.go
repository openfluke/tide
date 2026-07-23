// Package chain builds input → CNN2 → CNN2 → Dense (with NCHW flatten).
package chain

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/layers/cnn2"
	"github.com/openfluke/welvet/layers/dense"
	"github.com/openfluke/welvet/quant"
	"github.com/openfluke/welvet/runtime/training"
)

// Spec is the MNIST-style CNN stack.
type Spec struct {
	InChannels int
	Height     int
	Width      int
	Filters1   int
	Filters2   int
	Kernel     int
	Classes    int
	Seed       uint64
}

// DefaultMNIST is 28×28 → cnn2 → cnn2 → dense → 10.
func DefaultMNIST() Spec {
	return Spec{
		InChannels: 1,
		Height:     28,
		Width:      28,
		Filters1:   8,
		Filters2:   16,
		Kernel:     3,
		Classes:    10,
		Seed:       0x71DE0001,
	}
}

// Model is CNN2 → CNN2 → flatten → Dense.
type Model struct {
	Spec   Spec
	CNN1   *cnn2.Layer
	CNN2   *cnn2.Layer
	Head   *dense.Layer
	FlatIn int
	OutH1  int
	OutW1  int
	OutH2  int
	OutW2  int
}

// Build constructs a fresh model for one permutation cell.
func Build(spec Spec, cell permute.Cell) (*Model, error) {
	cfg1 := cnn2.Config{
		InChannels: spec.InChannels,
		Filters:    spec.Filters1,
		Height:     spec.Height,
		Width:      spec.Width,
		Kernel:     spec.Kernel,
		Stride:     1,
		Padding:    1,
		Activation: core.ActivationReLU,
	}
	if err := cfg1.Validate(); err != nil {
		return nil, err
	}
	outH1, outW1 := cfg1.OutH(), cfg1.OutW()
	cfg2 := cnn2.Config{
		InChannels: spec.Filters1,
		Filters:    spec.Filters2,
		Height:     outH1,
		Width:      outW1,
		Kernel:     spec.Kernel,
		Stride:     2,
		Padding:    1,
		Activation: core.ActivationReLU,
	}
	if err := cfg2.Validate(); err != nil {
		return nil, err
	}
	outH2, outW2 := cfg2.OutH(), cfg2.OutW()
	flat := spec.Filters2 * outH2 * outW2

	rng := rand.New(rand.NewPCG(spec.Seed, spec.Seed^0x9e3779b97f4a7c15))
	init1 := randWeights(cfg1.Filters*cfg1.PatchDim(), rng)
	init2 := randWeights(cfg2.Filters*cfg2.PatchDim(), rng)
	initH := randWeights(spec.Classes*flat, rng)

	c1, err := cnn2.NewConfigured(cfg1, core.DTypeFloat32, quant.FormatNone, init1)
	if err != nil {
		return nil, fmt.Errorf("cnn1: %w", err)
	}
	c2, err := cnn2.NewConfigured(cfg2, core.DTypeFloat32, quant.FormatNone, init2)
	if err != nil {
		return nil, fmt.Errorf("cnn2: %w", err)
	}
	head, err := dense.NewConfigured(flat, spec.Classes, core.ActivationLinear, core.DTypeFloat32, quant.FormatNone, initH)
	if err != nil {
		return nil, fmt.Errorf("dense: %w", err)
	}

	m := &Model{Spec: spec, CNN1: c1, CNN2: c2, Head: head, FlatIn: flat, OutH1: outH1, OutW1: outW1, OutH2: outH2, OutW2: outW2}
	if err := m.applyCell(cell); err != nil {
		return nil, err
	}
	return m, nil
}

func randWeights(n int, rng *rand.Rand) []float32 {
	w := make([]float32, n)
	scale := float32(1 / math.Sqrt(float64(n)))
	if scale > 0.1 {
		scale = 0.1
	}
	for i := range w {
		w[i] = (rng.Float32()*2 - 1) * scale
	}
	return w
}

func (m *Model) applyCell(cell permute.Cell) error {
	be := cell.Backend
	if cell.UseSIMD {
		be = core.BackendSIMD
	}
	for _, set := range []struct {
		setD func(core.DType) error
		pack func(quant.Format) error
		exec *core.ExecConfig
	}{
		{m.CNN1.SetDType, m.CNN1.Pack, &m.CNN1.Exec},
		{m.CNN2.SetDType, m.CNN2.Pack, &m.CNN2.Exec},
		{m.Head.SetDType, m.Head.Pack, &m.Head.Exec},
	} {
		set.exec.Backend = be
		set.exec.MultiCore = true
		if cell.Format != quant.FormatNone {
			if err := set.pack(cell.Format); err != nil {
				return err
			}
		} else if cell.DType != core.DTypeFloat32 {
			if err := set.setD(cell.DType); err != nil {
				return err
			}
		}
	}
	return nil
}

type tape struct {
	x, pre1, y1, pre2, y2, flat, preH, out *core.Tensor[float32]
}

// Forward runs the stack; returns logits [B, classes].
func (m *Model) Forward(x *core.Tensor[float32]) (*core.Tensor[float32], *tape, error) {
	if m == nil || x == nil {
		return nil, nil, fmt.Errorf("chain: nil")
	}
	pre1, y1, err := cnn2.Forward(m.CNN1, x)
	if err != nil {
		return nil, nil, err
	}
	pre2, y2, err := cnn2.Forward(m.CNN2, y1)
	if err != nil {
		return nil, nil, err
	}
	batch := y2.Shape[0]
	flat := &core.Tensor[float32]{Shape: []int{batch, m.FlatIn}, Data: y2.Data}
	preH, out, err := dense.Forward(m.Head, flat)
	if err != nil {
		return nil, nil, err
	}
	return out, &tape{x: x, pre1: pre1, y1: y1, pre2: pre2, y2: y2, flat: flat, preH: preH, out: out}, nil
}

// PredictArgmax returns predicted class indices.
func (m *Model) PredictArgmax(x *core.Tensor[float32]) ([]int, error) {
	out, _, err := m.Forward(x)
	if err != nil {
		return nil, err
	}
	batch := out.Shape[0]
	classes := out.Shape[1]
	preds := make([]int, batch)
	for b := 0; b < batch; b++ {
		best := 0
		bv := out.Data[b*classes]
		for c := 1; c < classes; c++ {
			v := out.Data[b*classes+c]
			if v > bv {
				bv, best = v, c
			}
		}
		preds[b] = best
	}
	return preds, nil
}

// TrainStep runs one training step for the given mode.
func (m *Model) TrainStep(x, target *core.Tensor[float32], lr float64, mode permute.TrainMode) (loss float64, err error) {
	out, tp, err := m.Forward(x)
	if err != nil {
		return 0, err
	}
	loss, err = training.MSE(out, target)
	if err != nil {
		return 0, err
	}
	switch mode {
	case permute.ModeTweenHead, permute.ModeTweenHeadSimd:
		return loss, m.tweenHead(tp, target, lr)
	default:
		return loss, m.sgdFull(tp, target, lr)
	}
}

func (m *Model) sgdFull(tp *tape, target *core.Tensor[float32], lr float64) error {
	gy, err := training.MSEGrad(tp.out, target)
	if err != nil {
		return err
	}
	gFlat, dWHead, err := dense.Backward(m.Head, gy, tp.flat, tp.preH)
	if err != nil {
		return err
	}
	if err := dense.ApplyGradSGD(m.Head, dWHead, lr); err != nil {
		return err
	}
	gY2 := &core.Tensor[float32]{Shape: append([]int(nil), tp.y2.Shape...), Data: gFlat.Data}
	gY1, dW2, err := cnn2.Backward(m.CNN2, gY2, tp.y1, tp.pre2)
	if err != nil {
		return err
	}
	if err := cnn2.ApplyGradSGD(m.CNN2, dW2, lr); err != nil {
		return err
	}
	_, dW1, err := cnn2.Backward(m.CNN1, gY1, tp.x, tp.pre1)
	if err != nil {
		return err
	}
	return cnn2.ApplyGradSGD(m.CNN1, dW1, lr)
}

// tweenHead updates only the Dense classifier toward the target (gap × input).
func (m *Model) tweenHead(tp *tape, target *core.Tensor[float32], lr float64) error {
	batch := tp.out.Shape[0]
	classes := tp.out.Shape[1]
	in := m.FlatIn
	dW := core.NewTensor[float32](classes, in)
	for b := 0; b < batch; b++ {
		for c := 0; c < classes; c++ {
			// Same sign as MSE: ∂L/∂pred ∝ (pred − target); ApplyGradSGD does w -= lr·dW.
			gap := tp.out.Data[b*classes+c] - target.Data[b*classes+c]
			base := c * in
			off := b * in
			for i := 0; i < in; i++ {
				dW.Data[base+i] += gap * tp.flat.Data[off+i]
			}
		}
	}
	scale := float32(lr / float64(batch))
	for i := range dW.Data {
		dW.Data[i] *= scale
	}
	// ApplyGradSGD does w -= lr * dW; we already scaled by lr, so pass lr=1.
	return dense.ApplyGradSGD(m.Head, dW, 1)
}
