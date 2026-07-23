package chain

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// ExportF32 returns float32 copies of CNN1 / CNN2 / Dense weights (unpacked).
func (m *Model) ExportF32() (cnn1, cnn2, head []float32, err error) {
	if m == nil || m.CNN1 == nil || m.CNN2 == nil || m.Head == nil {
		return nil, nil, nil, fmt.Errorf("chain: nil model")
	}
	cnn1, err = m.CNN1.Proj.Weights.FlattenF32()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cnn1: %w", err)
	}
	cnn2, err = m.CNN2.Proj.Weights.FlattenF32()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cnn2: %w", err)
	}
	head, err = m.Head.Weights.FlattenF32()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("head: %w", err)
	}
	return cnn1, cnn2, head, nil
}

// ImportF32 reloads weights and re-encodes into the layers' current dtype/format.
func (m *Model) ImportF32(cnn1, cnn2, head []float32) error {
	if m == nil || m.CNN1 == nil || m.CNN2 == nil || m.Head == nil {
		return fmt.Errorf("chain: nil model")
	}
	if err := m.CNN1.Proj.Weights.SetFromF32(cnn1); err != nil {
		return fmt.Errorf("cnn1: %w", err)
	}
	if err := m.CNN2.Proj.Weights.SetFromF32(cnn2); err != nil {
		return fmt.Errorf("cnn2: %w", err)
	}
	if err := m.Head.Weights.SetFromF32(head); err != nil {
		return fmt.Errorf("head: %w", err)
	}
	return nil
}

// SaveWeightsDir writes cnn1.bin / cnn2.bin / head.bin under dir.
func (m *Model) SaveWeightsDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	c1, c2, h, err := m.ExportF32()
	if err != nil {
		return err
	}
	if err := writeF32(filepath.Join(dir, "cnn1.bin"), c1); err != nil {
		return err
	}
	if err := writeF32(filepath.Join(dir, "cnn2.bin"), c2); err != nil {
		return err
	}
	return writeF32(filepath.Join(dir, "head.bin"), h)
}

// LoadWeightsDir reads cnn1.bin / cnn2.bin / head.bin into the model.
func (m *Model) LoadWeightsDir(dir string) error {
	c1, err := readF32(filepath.Join(dir, "cnn1.bin"))
	if err != nil {
		return err
	}
	c2, err := readF32(filepath.Join(dir, "cnn2.bin"))
	if err != nil {
		return err
	}
	h, err := readF32(filepath.Join(dir, "head.bin"))
	if err != nil {
		return err
	}
	return m.ImportF32(c1, c2, h)
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
