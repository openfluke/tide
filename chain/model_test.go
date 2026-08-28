package chain

import (
	"testing"

	"github.com/openfluke/tide/permute"
	"github.com/openfluke/welvet/core"
	"github.com/openfluke/welvet/quant"
)

func TestBuildCameralBranchCounts(t *testing.T) {
	spec := DefaultMNIST()
	for _, arch := range []permute.ArchKind{
		permute.ArchSingle,
		permute.ArchBicameral,
		permute.ArchTricameral,
		permute.ArchForCams(4),
		permute.ArchForCams(5),
		permute.ArchForCams(6),
	} {
		want := permute.CamsOf(arch)
		cell := permute.Cell{
			DType: core.DTypeFloat32, Format: quant.FormatNone,
			Mode: permute.ModeSGD, Arch: arch, Backend: core.BackendSIMD,
		}
		cell.ID = cell.String()
		m, err := Build(spec, cell)
		if err != nil {
			t.Fatalf("%s: %v", arch, err)
		}
		if want >= 2 {
			if m.Head != nil {
				t.Fatalf("%s: expected cameral stack, got single head", arch)
			}
			if m.Para == nil || len(m.Para.Branches) != want {
				t.Fatalf("%s: want %d branches got %d", arch, want, len(m.Para.Branches))
			}
			if m.Arch != arch {
				t.Fatalf("%s: arch rewritten to %q", arch, m.Arch)
			}
			continue
		}
		if m.Head == nil || m.Para != nil {
			t.Fatalf("%s: want single head", arch)
		}
	}
}

func TestForwardCameralX4(t *testing.T) {
	spec := DefaultMNIST()
	cell := permute.Cell{
		DType: core.DTypeFloat32, Format: quant.FormatNone,
		Mode: permute.ModeSGD, Arch: permute.ArchForCams(4), Backend: core.BackendSIMD,
	}
	m, err := Build(spec, cell)
	if err != nil {
		t.Fatal(err)
	}
	n := 2 * spec.InChannels * spec.Height * spec.Width
	x := &core.Tensor[float32]{
		Shape: []int{2, spec.InChannels, spec.Height, spec.Width},
		Data:  make([]float32, n),
	}
	out, _, err := m.Forward(x)
	if err != nil {
		t.Fatal(err)
	}
	if out.Shape[0] != 2 || out.Shape[1] != spec.Classes {
		t.Fatalf("out shape %v", out.Shape)
	}
}
