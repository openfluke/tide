// Package chain builds Welvet image-class stacks (default 28×28×10 MNIST).
package chain

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/layers/cnn2"
	"github.com/openfluke/welvet/layers/dense"
	"github.com/openfluke/welvet/layers/parallel"
	"github.com/openfluke/welvet/lucy"
	"github.com/openfluke/welvet/quant"
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
	Hidden     int // Cameral mid width (Bi/Tri)
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
		Hidden:     64,
		Seed:       0x71DE0001,
	}
}

// Model is CNN2 → CNN2 → flatten → (Dense | Bicameral Dense∥Dense → Dense).
type Model struct {
	Spec Spec
	Arch permute.ArchKind
	CNN1 *cnn2.Layer
	CNN2 *cnn2.Layer
	Head *dense.Layer // CNN arch only
	// Cameral: flatten → DenseIn → Parallel(n×Dense, add) → DenseOut
	DenseIn  *dense.Layer
	BranchR  *dense.Layer // hemi 0 (checkpoint name branch_r)
	BranchL  *dense.Layer // hemi 1 (checkpoint name branch_l)
	Para     *parallel.Layer
	DenseOut *dense.Layer
	stack    *parallel.Stack // Dense sandwich for TrainStackMSE (not CNN stem)
	FlatIn   int
	OutH1    int
	OutW1    int
	OutH2    int
	OutW2    int
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
	hidden := spec.Hidden
	if hidden <= 0 {
		hidden = 64
	}

	rng := rand.New(rand.NewPCG(spec.Seed, spec.Seed^0x9e3779b97f4a7c15))
	init1 := randWeights(cfg1.Filters*cfg1.PatchDim(), rng)
	init2 := randWeights(cfg2.Filters*cfg2.PatchDim(), rng)

	c1, err := cnn2.NewConfigured(cfg1, core.DTypeFloat32, quant.FormatNone, init1)
	if err != nil {
		return nil, fmt.Errorf("cnn1: %w", err)
	}
	c2, err := cnn2.NewConfigured(cfg2, core.DTypeFloat32, quant.FormatNone, init2)
	if err != nil {
		return nil, fmt.Errorf("cnn2: %w", err)
	}

	m := &Model{
		Spec: spec, Arch: cell.Arch,
		CNN1: c1, CNN2: c2,
		FlatIn: flat, OutH1: outH1, OutW1: outW1, OutH2: outH2, OutW2: outW2,
	}
	if m.Arch == "" {
		m.Arch = permute.ArchCNN
	}

	switch m.Arch {
	case permute.ArchBicameral, permute.ArchTricameral:
		n := permute.CamsOf(m.Arch)
		if n < 2 {
			n = 2
		}
		initIn := randWeights(hidden*flat, rng)
		initOut := randWeights(spec.Classes*hidden, rng)
		din, err := dense.NewConfigured(flat, hidden, core.ActivationLeakyReLU, core.DTypeFloat32, quant.FormatNone, initIn)
		if err != nil {
			return nil, fmt.Errorf("dense_in: %w", err)
		}
		branches := make([]any, n)
		var br, bl *dense.Layer
		for i := 0; i < n; i++ {
			initB := randWeights(hidden*hidden, rng)
			b, err := dense.NewConfigured(hidden, hidden, core.ActivationLeakyReLU, core.DTypeFloat32, quant.FormatNone, initB)
			if err != nil {
				return nil, fmt.Errorf("hemi %d: %w", i, err)
			}
			branches[i] = b
			if i == 0 {
				br = b
			}
			if i == 1 {
				bl = b
			}
		}
		para, err := parallel.NewFromBranches(parallel.Config{
			Dim: hidden, OutFeat: hidden, Branches: n, Combine: parallel.CombineAdd,
		}, branches, nil)
		if err != nil {
			return nil, fmt.Errorf("parallel: %w", err)
		}
		dout, err := dense.NewConfigured(hidden, spec.Classes, core.ActivationLinear, core.DTypeFloat32, quant.FormatNone, initOut)
		if err != nil {
			return nil, fmt.Errorf("dense_out: %w", err)
		}
		m.DenseIn, m.BranchR, m.BranchL, m.Para, m.DenseOut = din, br, bl, para, dout
	default:
		initH := randWeights(spec.Classes*flat, rng)
		head, err := dense.NewConfigured(flat, spec.Classes, core.ActivationLinear, core.DTypeFloat32, quant.FormatNone, initH)
		if err != nil {
			return nil, fmt.Errorf("dense: %w", err)
		}
		m.Head = head
		m.Arch = permute.ArchCNN
	}

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
	be := core.BackendSIMD
	if cell.Backend != 0 {
		be = cell.Backend
	}
	type layerExec struct {
		setD func(core.DType) error
		pack func(quant.Format) error
		exec *core.ExecConfig
	}
	var layers []layerExec
	layers = append(layers,
		layerExec{m.CNN1.SetDType, m.CNN1.Pack, &m.CNN1.Exec},
		layerExec{m.CNN2.SetDType, m.CNN2.Pack, &m.CNN2.Exec},
	)
	if m.Head != nil {
		layers = append(layers, layerExec{m.Head.SetDType, m.Head.Pack, &m.Head.Exec})
	}
	if m.DenseIn != nil {
		layers = append(layers, layerExec{m.DenseIn.SetDType, m.DenseIn.Pack, &m.DenseIn.Exec})
	}
	if m.DenseOut != nil {
		layers = append(layers, layerExec{m.DenseOut.SetDType, m.DenseOut.Pack, &m.DenseOut.Exec})
	}
	for _, dl := range m.hemiDenses() {
		layers = append(layers, layerExec{dl.SetDType, dl.Pack, &dl.Exec})
	}
	for _, set := range layers {
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
	if m.Para != nil {
		m.Para.Exec.Backend = be
		m.Para.Exec.MultiCore = true
		m.Para.SyncBranchExec()
	}
	return nil
}

type tape struct {
	x, pre1, y1, pre2, y2, flat *core.Tensor[float32]
	// CNN head
	preH, out *core.Tensor[float32]
	// Bicameral
	preIn, mid, preR, yR, preL, yL, prePara, yPara, preOut *core.Tensor[float32]
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
	tp := &tape{x: x, pre1: pre1, y1: y1, pre2: pre2, y2: y2, flat: flat}

	if m.Arch == permute.ArchBicameral || m.Arch == permute.ArchTricameral {
		preIn, mid, err := dense.Forward(m.DenseIn, flat)
		if err != nil {
			return nil, nil, err
		}
		prePara, yPara, err := parallel.Forward(m.Para, mid)
		if err != nil {
			return nil, nil, err
		}
		preOut, out, err := dense.Forward(m.DenseOut, yPara)
		if err != nil {
			return nil, nil, err
		}
		tp.preIn, tp.mid, tp.prePara, tp.yPara, tp.preOut, tp.out = preIn, mid, prePara, yPara, preOut, out
		return out, tp, nil
	}

	preH, out, err := dense.Forward(m.Head, flat)
	if err != nil {
		return nil, nil, err
	}
	tp.preH, tp.out = preH, out
	return out, tp, nil
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

// ServeEval runs Forward once and returns argmax preds + SoftAcc vs one-hot target.
//
// SoftAcc keeps the test41 formula 100×(1−|pred−target|/scale), applied to the
// true-class softmax probability vs 1.0 (both in [0,1], like sine targets).
// Classification uses SoftAccScaleClass=1.0 so SoftAcc ≈ 100×p(true) — the sine
// scale 0.10 would zero SoftAcc until p≥0.9 and collapse Score while Hard Acc
// was already high (raw logits vs 1.0 had the same failure mode).
func (m *Model) ServeEval(x, target *core.Tensor[float32]) (preds []int, softAcc float64, err error) {
	out, _, err := m.Forward(x)
	if err != nil {
		return nil, 0, err
	}
	batch := out.Shape[0]
	classes := out.Shape[1]
	preds = make([]int, batch)
	sumSoft := 0.0
	probs := make([]float32, classes)
	for b := 0; b < batch; b++ {
		off := b * classes
		best := 0
		bv := out.Data[off]
		for c := 1; c < classes; c++ {
			v := out.Data[off+c]
			if v > bv {
				bv, best = v, c
			}
		}
		preds[b] = best
		if target != nil && len(target.Data) >= off+classes {
			lab := 0
			for c := 1; c < classes; c++ {
				if target.Data[off+c] > target.Data[off+lab] {
					lab = c
				}
			}
			softmaxInto(out.Data[off:off+classes], probs)
			sumSoft += SoftAccProb(probs[lab], 1.0)
		}
	}
	if batch > 0 {
		softAcc = sumSoft / float64(batch)
	}
	return preds, softAcc, nil
}

func (m *Model) isCameral() bool {
	return m != nil && m.Para != nil
}

func (m *Model) hemiDenses() []*dense.Layer {
	if m == nil || m.Para == nil {
		return nil
	}
	out := make([]*dense.Layer, 0, len(m.Para.Branches))
	for _, ch := range m.Para.Branches {
		if d, ok := ch.(*dense.Layer); ok {
			out = append(out, d)
		}
	}
	return out
}

// denseSandwich is stem→hemispheres→head (no CNN). Credit TrainMode runs here.
func (m *Model) denseSandwich() (*parallel.Stack, error) {
	if m == nil {
		return nil, fmt.Errorf("chain: nil")
	}
	if m.stack != nil {
		return m.stack, nil
	}
	var s *parallel.Stack
	var err error
	if m.isCameral() {
		s, err = parallel.NewStack(m.DenseIn, m.Para, m.DenseOut)
	} else if m.Head != nil {
		s, err = parallel.NewStack(m.Head)
	} else {
		return nil, fmt.Errorf("chain: no dense sandwich")
	}
	if err != nil {
		return nil, err
	}
	m.stack = s
	return s, nil
}

// SoftAccScaleClass: SoftAcc on probabilities (MNIST). Sine test41 uses 0.10.
const SoftAccScaleClass = lucy.SoftAccScaleClass

// SoftAccProb is SoftAcc for a probability in [0,1] vs target (usually 1.0).
func SoftAccProb(pred, target float32) float64 {
	return lucy.SoftAccProb(pred, target)
}

// SoftAccOne is SoftAcc for a single pred/target pair (test41 sine formula, scale 0.10).
func SoftAccOne(pred, target float32) float64 {
	return lucy.SoftAccOne(pred, target)
}

func softmaxInto(logits, out []float32) {
	n := len(logits)
	if n == 0 || len(out) < n {
		return
	}
	max := logits[0]
	for i := 1; i < n; i++ {
		if logits[i] > max {
			max = logits[i]
		}
	}
	var sum float64
	for i := 0; i < n; i++ {
		out[i] = float32(math.Exp(float64(logits[i] - max)))
		sum += float64(out[i])
	}
	if sum <= 0 {
		for i := 0; i < n; i++ {
			out[i] = 1 / float32(n)
		}
		return
	}
	inv := float32(1 / sum)
	for i := 0; i < n; i++ {
		out[i] *= inv
	}
}
