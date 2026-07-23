package chain

import (
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/layers/cnn2"
	"github.com/openfluke/welvet/layers/dense"
	"github.com/openfluke/welvet/runtime/training"
)

const stepTicks = 3

// TrainStep runs one training step for the given Lucy-style mode.
func (m *Model) TrainStep(x, target *core.Tensor[float32], lr float64, mode permute.TrainMode) (loss float64, err error) {
	ticks := 1
	switch mode {
	case permute.ModeStepSGD, permute.ModeStepSGDSimd,
		permute.ModeStepTween, permute.ModeStepTweenSimd,
		permute.ModeStepTweenChain, permute.ModeStepTweenChainSimd:
		ticks = stepTicks
	}

	var tp *tape
	var out *core.Tensor[float32]
	for i := 0; i < ticks; i++ {
		out, tp, err = m.Forward(x)
		if err != nil {
			return 0, err
		}
	}
	loss, err = training.MSE(out, target)
	if err != nil {
		return 0, err
	}

	switch mode {
	case permute.ModeTweenHead, permute.ModeTweenHeadSimd:
		return loss, m.tweenHead(tp, target, lr)
	case permute.ModeTween, permute.ModeTweenSimd,
		permute.ModeStepTween, permute.ModeStepTweenSimd:
		return loss, m.tweenLayerwise(tp, target, lr)
	case permute.ModeTweenChain, permute.ModeTweenChainSimd,
		permute.ModeStepTweenChain, permute.ModeStepTweenChainSimd:
		return loss, m.tweenChain(tp, target, lr)
	default:
		// sgd / sgd_simd / step_sgd / step_sgd_simd
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
	return dense.ApplyGradSGD(m.Head, dW, 1)
}

// tweenLayerwise: Lucy Tween — local gaps per layer (no chain-rule through lower layers).
// Head fits target; each CNN fits its own post as a soft identity nudge from upstream gap clone.
func (m *Model) tweenLayerwise(tp *tape, target *core.Tensor[float32], lr float64) error {
	if err := m.tweenHead(tp, target, lr); err != nil {
		return err
	}
	// Local CNN gaps: pull pre toward post (stabilize) + small head-error broadcast.
	batch := tp.out.Shape[0]
	classes := tp.out.Shape[1]
	var headGap float32
	for b := 0; b < batch; b++ {
		for c := 0; c < classes; c++ {
			headGap += tp.out.Data[b*classes+c] - target.Data[b*classes+c]
		}
	}
	headGap /= float32(batch * classes)
	return m.applyLocalCNNGaps(tp, headGap, lr)
}

// tweenChain: Lucy TweenChain — propagate output gap backward through the stack.
func (m *Model) tweenChain(tp *tape, target *core.Tensor[float32], lr float64) error {
	gy, err := training.MSEGrad(tp.out, target)
	if err != nil {
		return err
	}
	// Head gap update (target-prop style using grad as gap).
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

func (m *Model) applyLocalCNNGaps(tp *tape, headGap float32, lr float64) error {
	// Nudge CNN2 weights using (y2 - y1_broadcast) proxy via a synthetic grad on y2.
	g2 := core.NewTensor[float32](tp.y2.Shape...)
	for i := range g2.Data {
		g2.Data[i] = headGap * 0.1
	}
	_, dW2, err := cnn2.Backward(m.CNN2, g2, tp.y1, tp.pre2)
	if err != nil {
		return err
	}
	if err := cnn2.ApplyGradSGD(m.CNN2, dW2, lr*0.5); err != nil {
		return err
	}
	g1 := core.NewTensor[float32](tp.y1.Shape...)
	for i := range g1.Data {
		g1.Data[i] = headGap * 0.05
	}
	_, dW1, err := cnn2.Backward(m.CNN1, g1, tp.x, tp.pre1)
	if err != nil {
		return err
	}
	return cnn2.ApplyGradSGD(m.CNN1, dW1, lr*0.5)
}
