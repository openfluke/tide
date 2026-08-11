package chain

import (
	"github.com/openfluke/welvet/layers/dense"
	"github.com/openfluke/welvet/weights"
)

// WeightBytes is the on-disk / resident weight payload for the full stack.
func (m *Model) WeightBytes() int64 {
	if m == nil {
		return 0
	}
	var n int64
	if m.CNN1 != nil && m.CNN1.Proj != nil {
		n += storeBytes(m.CNN1.Proj.Weights)
	}
	if m.CNN2 != nil && m.CNN2.Proj != nil {
		n += storeBytes(m.CNN2.Proj.Weights)
	}
	if m.Head != nil {
		n += storeBytes(m.Head.Weights)
	}
	for _, dl := range []*dense.Layer{m.DenseIn, m.BranchR, m.BranchL, m.DenseOut} {
		if dl != nil {
			n += storeBytes(dl.Weights)
		}
	}
	return n
}

func storeBytes(s *weights.Store) int64 {
	if s == nil {
		return 0
	}
	n := int64(len(s.Bias) * 8)
	if s.Packed != nil {
		n += int64(len(s.Packed.Raw))
		n += int64(len(s.Packed.Scales) * 4)
		n += int64(len(s.Packed.Mins) * 4)
		n += int64(len(s.Packed.Meta))
		return n
	}
	if len(s.Native) > 0 {
		return n + int64(len(s.Native))
	}
	bits := s.DType.Bits()
	if bits <= 0 {
		bits = 32
	}
	return n + int64((s.Rows*s.Cols*bits+7)/8)
}
