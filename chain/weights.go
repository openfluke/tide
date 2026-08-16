package chain

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	if m.isCameral() {
		din, err := m.DenseIn.Weights.FlattenF32()
		if err != nil {
			return nil, fmt.Errorf("dense_in: %w", err)
		}
		parts["dense_in"] = din
		for i, d := range m.hemiDenses() {
			v, err := d.Weights.FlattenF32()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", hemiPartName(i), err)
			}
			parts[hemiPartName(i)] = v
		}
		dout, err := m.DenseOut.Weights.FlattenF32()
		if err != nil {
			return nil, fmt.Errorf("dense_out: %w", err)
		}
		parts["dense_out"] = dout
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

func hemiPartName(i int) string {
	switch i {
	case 0:
		return "branch_r"
	case 1:
		return "branch_l"
	default:
		return fmt.Sprintf("branch_%d", i)
	}
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
	if m.isCameral() {
		if err := m.DenseIn.Weights.SetFromF32(parts["dense_in"]); err != nil {
			return fmt.Errorf("dense_in: %w", err)
		}
		for i, d := range m.hemiDenses() {
			name := hemiPartName(i)
			if err := d.Weights.SetFromF32(parts[name]); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if err := m.DenseOut.Weights.SetFromF32(parts["dense_out"]); err != nil {
			return fmt.Errorf("dense_out: %w", err)
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
	if m.isCameral() {
		names = []string{"cnn1", "cnn2", "dense_in", "dense_out"}
		for i := range m.hemiDenses() {
			names = append(names, hemiPartName(i))
		}
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
