package report

import "github.com/openfluke/welvet/lucy"

// Consciousness / synthetic-organism metric lives in welvet/lucy.
// This package pretty-prints cell IDs then calls lucy.BuildLPD.
const (
	LPDKeepFloor = lucy.LPDKeepFloor
	LPDGoldKeep  = lucy.LPDGoldKeep
	LPDGoldRAM   = lucy.LPDGoldRAM
	LPDNearRAM   = lucy.LPDNearRAM
	LPDShrinkCap = lucy.LPDShrinkCap
)

type (
	LPD      = lucy.LPD
	LPDRow   = lucy.LPDRow
	LPDChamp = lucy.LPDChamp
	LPDMode  = lucy.LPDMode
)

// BuildLPD ranks cells for consciousness then memory density.
func BuildLPD(pts []CellPoint) LPD {
	return lucy.BuildLPD(samplesOf(pts))
}

// CellPointOf maps a density row back onto a heatmap point.
func CellPointOf(r LPDRow) CellPoint {
	s := r.Sample()
	return CellPoint{
		Tide: s.Tide, ID: s.ID, Mode: s.Mode, DType: s.DType, Format: s.Format, Arch: s.Arch,
		Score: s.Score, Soft: s.Soft, Acc: s.Acc, Thru: s.Thru, Avail: s.Avail, RAMKiB: s.RAMKiB,
	}
}

func samplesOf(pts []CellPoint) []lucy.Sample {
	out := make([]lucy.Sample, len(pts))
	for i, p := range pts {
		out[i] = lucy.Sample{
			Tide: p.Tide, ID: PrettyCell(p.ID), Mode: p.Mode, DType: p.DType, Format: p.Format,
			Arch: PrettyArch(p.Arch), Score: p.Score, Soft: p.Soft, Acc: p.Acc,
			Thru: p.Thru, Avail: p.Avail, RAMKiB: p.RAMKiB,
		}
	}
	return out
}
