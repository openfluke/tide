package river

import (
	"sort"

	"github.com/openfluke/welvet/lucy"
)

// ThruPayload ranks finished cells by Tide throughput (samples/sec).
type ThruPayload struct {
	NTotal    int             `json:"n_total"`
	NRows     int             `json:"n_rows"`
	BestThru  float64         `json:"best_thru"`
	BestID    string          `json:"best_id"`
	BestAcc   float64         `json:"best_acc"` // Acc champion (for % of best Acc)
	Formula   string          `json:"formula"`
	Rows      []NearRow       `json:"rows"`
	Credits   map[string]int  `json:"credits"`
	Bands     map[string]int  `json:"bands"`
	BestNonBP *NearHighlight  `json:"best_non_bp,omitempty"`
	BestChain *NearHighlight  `json:"best_chain,omitempty"`
	ChainNote string          `json:"chain_note"`
}

func buildThru(f File) ThruPayload {
	out := ThruPayload{
		Formula:   lucy.DensityFormula(),
		Credits:   map[string]int{},
		Bands:     map[string]int{},
		ChainNote: chainCreditNote,
	}
	rows := make([]Row, len(f.Rows))
	copy(rows, f.Rows)
	for i := range rows {
		enrichRow(&rows[i])
	}
	out.NTotal = len(rows)
	if len(rows) == 0 {
		return out
	}

	byID := make(map[string]Row, len(rows))
	samples := make([]lucy.Sample, 0, len(rows))
	for _, r := range rows {
		if r.Status != "" && r.Status != "ok" && r.Status != "gap" {
			continue
		}
		byID[r.ID] = r
		samples = append(samples, rowToLucySample(r))
	}
	lpd := lucy.BuildLPD(samples)
	out.BestAcc = lpd.PeakAcc
	lucyByID := map[string]lucy.LPDRow{}
	for _, r := range lpd.Top {
		lucyByID[r.ID] = r
	}
	for _, r := range lpd.Pool {
		if _, ok := lucyByID[r.ID]; !ok {
			lucyByID[r.ID] = r
		}
	}

	band := make([]NearRow, 0, len(byID))
	for id, src := range byID {
		cred := creditOfRow(src)
		modeLabel := src.Mode
		lr := lucyByID[id]
		pct := 0.0
		if out.BestAcc > 0 {
			pct = 100 * src.Acc / out.BestAcc
		}
		band = append(band, NearRow{
			ID:           id,
			Mode:         modeLabel,
			BranchModes:  append([]string(nil), src.BranchModes...),
			MixPattern:   src.MixPattern,
			DType:        src.DType,
			Format:       firstNonEmpty(lr.Format, src.Format),
			Arch:         src.Arch,
			LRLabel:      src.LRLabel,
			Credit:       cred,
			Acc:          src.Acc,
			SoftAcc:      src.SoftAcc,
			PctOfBest:    pct,
			Throughput:   src.Throughput,
			Availability: src.Availability,
			Score:        src.Throughput * src.Availability * src.Acc / 10000,
			LPD:          lr.LPD,
			Q:            lr.Q,
			Shrink:       lr.Shrink,
			RAMKiB:       firstPositive(lr.RAMKiB, src.WeightKiB),
			RAMFrac:      lr.RAMFrac,
			DurationSec:  src.DurationSec,
			AccPerSec:    src.AccPerSec,
			DenseScore:   src.DenseScore,
			TimeTo50Sec:  src.TimeTo50Sec,
			Band:         lr.Band,
			Pillars:      lr.Pillars,
			RelThru:      lr.RelThru,
			RelAvail:     lr.RelAvail,
			Status:       src.Status,
		})
		out.Credits[cred]++
		if lr.Band != "" {
			out.Bands[lr.Band]++
		}
		if src.Throughput > out.BestThru {
			out.BestThru = src.Throughput
			out.BestID = id
		}
	}

	sort.SliceStable(band, func(i, j int) bool {
		a, b := band[i], band[j]
		if a.Throughput != b.Throughput {
			return a.Throughput > b.Throughput
		}
		if a.Acc != b.Acc {
			return a.Acc > b.Acc
		}
		if a.RAMKiB != b.RAMKiB {
			return a.RAMKiB < b.RAMKiB
		}
		return a.ID < b.ID
	})
	for i := range band {
		band[i].Rank = i + 1
	}

	out.BestNonBP = pickBestThruCredit(rows, out.BestAcc, creditNonBP, "")
	out.BestChain = pickBestThruCredit(rows, out.BestAcc, creditChain, chainCreditNote)
	if out.BestNonBP != nil {
		for i := range band {
			if band[i].ID == out.BestNonBP.ID {
				band[i].BestNonBP = true
				break
			}
		}
	}

	out.NRows = len(band)
	out.Rows = band
	return out
}

func pickBestThruCredit(rows []Row, bestAcc float64, want, note string) *NearHighlight {
	var best *NearHighlight
	for _, r := range rows {
		if r.Status != "" && r.Status != "ok" && r.Status != "gap" {
			continue
		}
		if creditOfRow(r) != want {
			continue
		}
		pct := 0.0
		if bestAcc > 0 {
			pct = 100 * r.Acc / bestAcc
		}
		h := &NearHighlight{
			ID: r.ID, Mode: r.Mode, DType: r.DType, Arch: r.Arch, LRLabel: r.LRLabel,
			Acc: r.Acc, PctOfBest: pct, Credit: want, InBand: true, Note: note,
		}
		// Prefer higher throughput; Acc as tiebreak.
		if best == nil ||
			r.Throughput > thrOf(best, rows) ||
			(r.Throughput == thrOf(best, rows) && h.Acc > best.Acc) ||
			(r.Throughput == thrOf(best, rows) && h.Acc == best.Acc && h.ID < best.ID) {
			best = h
		}
	}
	return best
}

func thrOf(h *NearHighlight, rows []Row) float64 {
	if h == nil {
		return 0
	}
	for _, r := range rows {
		if r.ID == h.ID {
			return r.Throughput
		}
	}
	return 0
}

func firstPositive(a, b float64) float64 {
	if a > 0 {
		return a
	}
	return b
}
