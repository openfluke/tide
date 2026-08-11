package chain

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/openfluke/tide/permute"
)

// ExportF32 returns float32 copies of all stack weights (unpacked).
func (m *Model) ExportF32() (parts map[string][]float32, err error) {
	if m == nil || m.CNN1 == nil || m.CNN2 == nil {
		return nil, fmt.Errorf("chain: nil model")
	}
	parts = make(map[string][]float32)
	c1, err := m.CNN1.Proj.Weights.FlattenF32()
	if err != nil {
		return nil, fmt.Errorf("cnn1: %w", err)
	}
	c2, err := m.CNN2.Proj.Weights.FlattenF32()
	if err != nil {
		return nil, fmt.Errorf("cnn2: %w", err)
	}
	parts["cnn1"] = c1
	parts["cnn2"] = c2
	if m.Arch == permute.ArchBicameral {
		for name, layer := range map[string]*struct {
			flat func() ([]float32, error)
		}{
			"dense_in":  {func() ([]float32, error) { return m.DenseIn.Weights.FlattenF32() }},
			"branch_r":  {func() ([]float32, error) { return m.BranchR.Weights.FlattenF32() }},
			"branch_l":  {func() ([]float32, error) { return m.BranchL.Weights.FlattenF32() }},
			"dense_out": {func() ([]float32, error) { return m.DenseOut.Weights.FlattenF32() }},
		} {
			v, e := layer.flat()
			if e != nil {
				return nil, fmt.Errorf("%s: %w", name, e)
			}
			parts[name] = v
		}
		return parts, nil
	}
	if m.Head == nil {
		return nil, fmt.Errorf("chain: nil head")
	}
	h, err := m.Head.Weights.FlattenF32()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}
	parts["head"] = h
	return parts, nil
}

// ImportF32 reloads weights and re-encodes into the layers' current dtype/format.
func (m *Model) ImportF32(parts map[string][]float32) error {
	if m == nil || m.CNN1 == nil || m.CNN2 == nil {
		return fmt.Errorf("chain: nil model")
	}
	if err := m.CNN1.Proj.Weights.SetFromF32(parts["cnn1"]); err != nil {
		return fmt.Errorf("cnn1: %w", err)
	}
	if err := m.CNN2.Proj.Weights.SetFromF32(parts["cnn2"]); err != nil {
		return fmt.Errorf("cnn2: %w", err)
	}
	if m.Arch == permute.ArchBicameral {
		for name, set := range map[string]func([]float32) error{
			"dense_in":  m.DenseIn.Weights.SetFromF32,
			"branch_r":  m.BranchR.Weights.SetFromF32,
			"branch_l":  m.BranchL.Weights.SetFromF32,
			"dense_out": m.DenseOut.Weights.SetFromF32,
		} {
			if err := set(parts[name]); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	}
	if m.Head == nil {
		return fmt.Errorf("chain: nil head")
	}
	return m.Head.Weights.SetFromF32(parts["head"])
}

// SaveWeightsDir writes weight bins under dir.
func (m *Model) SaveWeightsDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	parts, err := m.ExportF32()
	if err != nil {
		return err
	}
	for name, v := range parts {
		if err := writeF32(filepath.Join(dir, name+".bin"), v); err != nil {
			return err
		}
	}
	return nil
}

// LoadWeightsDir reads weight bins into the model.
func (m *Model) LoadWeightsDir(dir string) error {
	names := []string{"cnn1", "cnn2", "head"}
	if m.Arch == permute.ArchBicameral {
		names = []string{"cnn1", "cnn2", "dense_in", "branch_r", "branch_l", "dense_out"}
	}
	parts := make(map[string][]float32, len(names))
	for _, name := range names {
		v, err := readF32(filepath.Join(dir, name+".bin"))
		if err != nil {
			return err
		}
		parts[name] = v
	}
	return m.ImportF32(parts)
}

func writeF32(path string, v []float32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 4)
	for _, x := range v {
		binary.LittleEndian.PutUint32(buf, math.Float32bits(x))
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func readF32(path string) ([]float32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("bad f32 file %s len %d", path, len(b))
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out, nil
}
