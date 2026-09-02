package river

import (
	"sort"

	"github.com/openfluke/welvet/lucy"
)

// LPDSearchPayload is the Lucy density explorer: every finished cell ranked by
// LPD (Q × shrink), including traps (LPD = 0 below Acc-keep floor).
type LPDSearchPayload struct {
	Formula   string         `json:"formula"`
	BestAcc   float64        `json:"best_acc"`
	BestID    string         `json:"best_id"`
	PeakThru  float64        `json:"peak_thru"`
	PeakAvail float64        `json:"peak_avail"`
	NTotal    int            `json:"n_total"`
	NKeep     int            `json:"n_keep"` // Acc keep ≥ 70%
	NTrap     int            `json:"n_trap"`
	Rows      []NearRow      `json:"rows"`
	Bands     map[string]int `json:"bands"`
	Credits   map[string]int `json:"credits"`
	ChainNote string         `json:"chain_note"`

	AccChamp   *NearHighlight `json:"acc_champ,omitempty"`
	ScoreChamp *NearHighlight `json:"score_champ,omitempty"`
	LiveChamp  *NearHighlight `json:"live_champ,omitempty"`
	LeanChamp  *NearHighlight `json:"lean_champ,omitempty"`
	GoldStd    *NearHighlight `json:"gold_std,omitempty"`
	BestNonBP  *NearHighlight `json:"best_non_bp,omitempty"`
	BestChain  *NearHighlight `json:"best_chain,omitempty"`
	TopLPD     *NearHighlight `json:"top_lpd,omitempty"`
}

func buildLPDSearch(f File) LPDSearchPayload {
	out := LPDSearchPayload{
		Formula:   lucy.DensityFormula(),
		Bands:     map[string]int{},
		Credits:   map[string]int{},
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
	out.BestID = lpd.AccChamp.ID
	out.PeakThru = lpd.PeakThru
	out.PeakAvail = lpd.PeakAvail

	pool := lpd.Pool
	if len(pool) == 0 {
		pool = lpd.Top
	}
	band := make([]NearRow, 0, len(pool))
	for _, r := range pool {
		src := byID[r.ID]
		cred := creditOfRow(src)
		modeLabel := src.Mode
		if modeLabel == "" {
			modeLabel = r.Mode
		}
		nr := NearRow{
			ID:           r.ID,
			Mode:         modeLabel,
			BranchModes:  append([]string(nil), src.BranchModes...),
			MixPattern:   src.MixPattern,
			DType:        r.DType,
			Format:       firstNonEmpty(r.Format, src.Format),
			Arch:         r.Arch,
			LRLabel:      src.LRLabel,
			Credit:       cred,
			Acc:          r.Acc,
			SoftAcc:      r.Soft,
			PctOfBest:    r.RelAcc * 100,
			Throughput:   r.Thru,
			Availability: r.Avail,
			Score:        r.Score,
			LPD:          r.LPD,
			Q:            r.Q,
			Shrink:       r.Shrink,
			RAMKiB:       r.RAMKiB,
			RAMFrac:      r.RAMFrac,
			DurationSec:  src.DurationSec,
			AccPerSec:    src.AccPerSec,
			DenseScore:   src.DenseScore,
			TimeTo50Sec:  src.TimeTo50Sec,
			Band:         r.Band,
			Pillars:      r.Pillars,
			RelThru:      r.RelThru,
			RelAvail:     r.RelAvail,
			Status:       src.Status,
		}
		band = append(band, nr)
		out.Bands[r.Band]++
		out.Credits[cred]++
		if r.RelAcc >= lucy.LPDKeepFloor {
			out.NKeep++
		}
		if r.Band == "trap" {
			out.NTrap++
		}
	}
	// LPD high → low; then Q; then smaller RAM.
	sort.SliceStable(band, func(i, j int) bool {
		a, b := band[i], band[j]
		if a.LPD != b.LPD {
			return a.LPD > b.LPD
		}
		if a.Q != b.Q {
			return a.Q > b.Q
		}
		if a.RAMKiB != b.RAMKiB {
			return a.RAMKiB < b.RAMKiB
		}
		return a.ID < b.ID
	})
	for i := range band {
		band[i].Rank = i + 1
	}

	thr := out.BestAcc * lucy.LPDKeepFloor
	out.BestNonBP = pickBestCredit(rows, out.BestAcc, thr, creditNonBP, "")
	out.BestChain = pickBestCredit(rows, out.BestAcc, thr, creditChain, chainCreditNote)
	if out.BestNonBP != nil {
		for i := range band {
			if band[i].ID == out.BestNonBP.ID {
				band[i].BestNonBP = true
				break
			}
		}
	}
	out.AccChamp = champFromLPD(lpd.AccChamp, out.BestAcc, thr)
	out.ScoreChamp = champFromLPD(lpd.Champ, out.BestAcc, thr)
	out.LiveChamp = champFromLPD(lpd.LiveChamp, out.BestAcc, thr)
	if lpd.LeanChamp.ID != "" {
		out.LeanChamp = highlightFromRow(lpd.LeanChamp, byID[lpd.LeanChamp.ID], out.BestAcc, thr)
	}
	if lpd.GoldStd.ID != "" {
		out.GoldStd = highlightFromRow(lpd.GoldStd, byID[lpd.GoldStd.ID], out.BestAcc, thr)
	}
	if len(band) > 0 && band[0].LPD > 0 {
		top := band[0]
		out.TopLPD = &NearHighlight{
			ID: top.ID, Mode: top.Mode, DType: top.DType, Arch: top.Arch, LRLabel: top.LRLabel,
			Acc: top.Acc, PctOfBest: top.PctOfBest, Credit: top.Credit, InBand: top.PctOfBest >= 70,
		}
	}

	out.Rows = band
	return out
}

func champFromLPD(c lucy.LPDChamp, bestAcc, thr float64) *NearHighlight {
	if c.ID == "" {
		return nil
	}
	pct := 0.0
	if bestAcc > 0 {
		pct = 100 * c.Acc / bestAcc
	}
	return &NearHighlight{
		ID: c.ID, Mode: c.Mode, DType: c.DType, Arch: c.Arch,
		Acc: c.Acc, PctOfBest: pct, Credit: creditOfMode(c.Mode), InBand: thr > 0 && c.Acc >= thr,
	}
}

func highlightFromRow(r lucy.LPDRow, src Row, bestAcc, thr float64) *NearHighlight {
	pct := r.RelAcc * 100
	if pct == 0 && bestAcc > 0 {
		pct = 100 * r.Acc / bestAcc
	}
	return &NearHighlight{
		ID: r.ID, Mode: r.Mode, DType: r.DType, Arch: r.Arch, LRLabel: src.LRLabel,
		Acc: r.Acc, PctOfBest: pct, Credit: creditOfMode(r.Mode), InBand: thr > 0 && r.Acc >= thr,
	}
}
