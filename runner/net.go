package runner

import (
	"github.com/openfluke/tide/chain"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/core"
)

// Net is one permutation's trainable model.
// live_mnist uses chain.Model (the default when Config.Build is nil).
// Other hosts (quick_sprint) supply a stack via Config.Build — additive, so
// existing tide hosts keep the MNIST CNN path.
type Net interface {
	TrainStep(x, target *core.Tensor[float32], lr float64, mode permute.TrainMode) (loss float64, err error)
	ServeEval(x, target *core.Tensor[float32]) (preds []int, softAcc float64, err error)
	WeightBytes() int64
}

// BuildFunc constructs a Net for one permute cell.
type BuildFunc func(cell permute.Cell) (Net, error)

var _ Net = (*chain.Model)(nil)

func (cfg Config) buildNet(cell permute.Cell) (Net, error) {
	if cfg.Build != nil {
		return cfg.Build(cell)
	}
	return chain.Build(cfg.Spec, cell)
}

func asChain(n Net) *chain.Model {
	m, _ := n.(*chain.Model)
	return m
}
