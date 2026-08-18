package report

import (
	"math"
	"sort"
)

// LPD is the Lucy Pareto / goldilocks board: stay close to live Score
// and to the Acc champion, then shrink RAM. Raw Score/MiB is the binary
// trap — a 1-bit net with chance Acc still "wins" MobileScore.
//
// Score and Thru peaks are the Score champion (binary must not set Thru).
// Soft and Acc peaks are their own board champs, so a fast cell with
// chance Acc cannot look like 94% Q. LPD = 0 unless Q ≥ 70% and Acc
// keep ≥ 70% of the Acc champ. Gold also needs RAM ≤ 20% of Score champ.
const (
	LPDKeepFloor = 0.70
	LPDGoldKeep  = 0.80
	LPDGoldRAM   = 0.20
	LPDNearRAM   = 0.50
	LPDShrinkCap = 32.0
)

// LPDChamp is the quality reference (highest Lucy Score).
type LPDChamp struct {
	ID     string  `json:"id"`
	Tide   string  `json:"tide,omitempty"`
	Mode   string  `json:"mode"`
	DType  string  `json:"dtype"`
	Arch   string  `json:"arch"`
	Score  float64 `json:"score"`
	Soft   float64 `json:"soft_acc"`
	Acc    float64 `json:"avg_accuracy"`
	Thru   float64 `json:"throughput"`
	Avail  float64 `json:"availability"`
	RAMKiB float64 `json:"ram_kib"`
}

// LPDRow is one cell against the champion.
type LPDRow struct {
	Tide     string  `json:"tide,omitempty"`
	ID       string  `json:"id"`
	Mode     string  `json:"mode"`
	DType    string  `json:"dtype"`
	Format   string  `json:"format"`
	Arch     string  `json:"arch"`
	Score    float64 `json:"score"`
	Soft     float64 `json:"soft_acc"`
	Acc      float64 `json:"avg_accuracy"`
	Thru     float64 `json:"throughput"`
	Avail    float64 `json:"availability"`
	RAMKiB   float64 `json:"ram_kib"`
	RelScore float64 `json:"rel_score"`
	RelSoft  float64 `json:"rel_soft"`
	RelAcc   float64 `json:"rel_acc"`
	RelThru  float64 `json:"rel_thru"`
	Q        float64 `json:"q"`        // 0–1 keep of the peaks
	RAMFrac  float64 `json:"ram_frac"` // this / champ RAM
	Shrink   float64 `json:"shrink"`   // champ / this (× smaller)
	LPD      float64 `json:"lpd"`      // Q × min(Shrink, 32); 0 if Q < 70%
	RelFast  float64 `json:"rel_fast"` // Thru / fastest on the board (capped 1)
	RelDuty  float64 `json:"rel_duty"` // Avail / best Availability on the board (capped 1)
	MSpeed   float64 `json:"mspeed"`   // RelFast if Q ≥ 70%, else 0
	MAvail   float64 `json:"mavail"`   // RelDuty if Q ≥ 70%, else 0
	Mix      float64 `json:"mix"`      // geomean(Q, RelFast, RelDuty); 0 if Q < 70%
	Band     string  `json:"band"`     // gold | near | keep | trap | —
	Gold     bool    `json:"gold"`
}

// LPDMode is one train mode among cells that kept Acc-champ quality.
type LPDMode struct {
	Mode     string  `json:"mode"`
	N        int     `json:"n"`
	BestAcc  float64 `json:"best_acc"`
	MinRAM   float64 `json:"min_ram_kib"`
	MaxThru  float64 `json:"max_thru"`
	BestQ    float64 `json:"best_q"`
	Smallest string  `json:"smallest"`
	Fastest  string  `json:"fastest"`
}

// LPD is the goldilocks snapshot for a tide or the ocean.
type LPD struct {
	Formula   string    `json:"formula"`
	Champ     LPDChamp  `json:"champ"`
	AccChamp  LPDChamp  `json:"acc_champ"`
	SoftChamp LPDChamp  `json:"soft_champ"`
	PeakScore float64   `json:"peak_score"`
	PeakSoft  float64   `json:"peak_soft"`
	PeakAcc   float64   `json:"peak_acc"`
	PeakThru  float64   `json:"peak_thru"`
	FastThru  float64   `json:"fast_thru"` // fastest Throughput on the board
	FastID    string    `json:"fast_id,omitempty"`
	BestAvail float64   `json:"best_avail"` // highest Availability on the board
	AvailID   string    `json:"avail_id,omitempty"`
	N         int       `json:"n"`
	Gold      []LPDRow  `json:"gold,omitempty"`
	Near      []LPDRow  `json:"near,omitempty"`
	Top       []LPDRow  `json:"top,omitempty"`
	Trap      []LPDRow  `json:"trap,omitempty"`
	TopSpeed  []LPDRow  `json:"top_speed,omitempty"`
	TopAvail  []LPDRow  `json:"top_avail,omitempty"`
	TopMix    []LPDRow  `json:"top_mix,omitempty"`
	GoldStd   LPDRow    `json:"gold_std,omitempty"`
	GoldModes []LPDMode `json:"gold_modes,omitempty"`
}

func lpdFormula() string {
	return "Q = geomean of Score/Thru vs Score champ and Soft/Acc vs their own champs. Thru peak is the Score champ (not the board fastest). LPD = 0 unless Q≥70% and Acc keep ≥70% of the Acc champ, else Q × shrink vs Score-champ RAM (capped 32×). Gold = Q≥80% and Acc keep ≥80% and RAM≤20%. Gold-std mode = smallest then fastest among Acc-keep ≥80%. Mix = geomean(Q, MSpeed, MAvail). Raw Score/MiB is the binary trap."
}

// BuildLPD ranks cells for the goldilocks: good enough quality, then smaller RAM.
func BuildLPD(pts []CellPoint) LPD {
	out := LPD{Formula: lpdFormula()}
	if len(pts) == 0 {
		return out
	}
	var champ, accChamp, softChamp CellPoint
	var fastThru, bestAvail float64
	var fastID, availID string
	for i, p := range pts {
		better := p.Score > champ.Score
		if p.Score == champ.Score && champ.ID != "" {
			if p.Acc > champ.Acc || (p.Acc == champ.Acc && p.Soft > champ.Soft) {
				better = true
			}
		}
		if i == 0 || better {
			champ = p
		}
		if i == 0 || p.Acc > accChamp.Acc || (p.Acc == accChamp.Acc && (p.Soft > accChamp.Soft || (p.Soft == accChamp.Soft && p.Score > accChamp.Score))) {
			accChamp = p
		}
		if i == 0 || p.Soft > softChamp.Soft || (p.Soft == softChamp.Soft && (p.Acc > softChamp.Acc || (p.Acc == softChamp.Acc && p.Score > softChamp.Score))) {
			softChamp = p
		}
		if i == 0 || p.Thru > fastThru {
			fastThru, fastID = p.Thru, p.ID
		}
		if i == 0 || p.Avail > bestAvail {
			bestAvail, availID = p.Avail, p.ID
		}
	}
	out.PeakScore, out.PeakThru = champ.Score, champ.Thru
	out.PeakSoft, out.PeakAcc = softChamp.Soft, accChamp.Acc
	out.FastThru, out.FastID = fastThru, PrettyCell(fastID)
	out.BestAvail, out.AvailID = bestAvail, PrettyCell(availID)
	if champ.RAMKiB <= 0 {
		champ.RAMKiB = 1e-6
	}
	out.Champ = lpdChampOf(champ)
	out.AccChamp = lpdChampOf(accChamp)
	out.SoftChamp = lpdChampOf(softChamp)
	out.N = len(pts)
	rows := make([]LPDRow, 0, len(pts))
	for _, p := range pts {
		r := lpdRow(p, out)
		rows = append(rows, r)
		switch r.Band {
		case "gold":
			out.Gold = append(out.Gold, r)
		case "near":
			out.Near = append(out.Near, r)
		case "trap":
			out.Trap = append(out.Trap, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LPD != rows[j].LPD {
			return rows[i].LPD > rows[j].LPD
		}
		if rows[i].Q != rows[j].Q {
			return rows[i].Q > rows[j].Q
		}
		return rows[i].RAMKiB < rows[j].RAMKiB
	})
	sort.SliceStable(out.Gold, func(i, j int) bool { return out.Gold[i].LPD > out.Gold[j].LPD })
	sort.SliceStable(out.Near, func(i, j int) bool { return out.Near[i].LPD > out.Near[j].LPD })
	sort.SliceStable(out.Trap, func(i, j int) bool { return out.Trap[i].RAMFrac < out.Trap[j].RAMFrac })
	if len(out.Gold) > 24 {
		out.Gold = out.Gold[:24]
	}
	if len(out.Near) > 16 {
		out.Near = out.Near[:16]
	}
	if len(out.Trap) > 12 {
		out.Trap = out.Trap[:12]
	}
	out.Top = rows
	if len(out.Top) > 40 {
		out.Top = out.Top[:40]
	}
	out.TopSpeed = rankLPD(rows, func(r LPDRow) float64 { return r.MSpeed }, 12)
	out.TopAvail = rankLPD(rows, func(r LPDRow) float64 { return r.MAvail }, 12)
	out.TopMix = rankLPD(rows, func(r LPDRow) float64 { return r.Mix }, 12)
	out.GoldStd, out.GoldModes = goldStandard(rows)
	return out
}

func lpdChampOf(p CellPoint) LPDChamp {
	return LPDChamp{
		ID: PrettyCell(p.ID), Tide: p.Tide, Mode: p.Mode, DType: p.DType, Arch: PrettyArch(p.Arch),
		Score: p.Score, Soft: p.Soft, Acc: p.Acc, Thru: p.Thru, Avail: p.Avail, RAMKiB: p.RAMKiB,
	}
}

func goldStandard(rows []LPDRow) (LPDRow, []LPDMode) {
	var keep []LPDRow
	for _, r := range rows {
		if r.RelAcc >= LPDGoldKeep {
			keep = append(keep, r)
		}
	}
	if len(keep) == 0 {
		return LPDRow{}, nil
	}
	sort.SliceStable(keep, func(i, j int) bool {
		if keep[i].RAMKiB != keep[j].RAMKiB {
			return keep[i].RAMKiB < keep[j].RAMKiB
		}
		if keep[i].Thru != keep[j].Thru {
			return keep[i].Thru > keep[j].Thru
		}
		return keep[i].Acc > keep[j].Acc
	})
	std := keep[0]
	type agg struct {
		n                               int
		bestAcc, maxThru, bestQ, minRAM float64
		smallest, fastest               string
	}
	by := map[string]*agg{}
	order := []string{}
	for _, r := range keep {
		a := by[r.Mode]
		if a == nil {
			a = &agg{minRAM: r.RAMKiB, smallest: r.ID, fastest: r.ID}
			by[r.Mode] = a
			order = append(order, r.Mode)
		}
		a.n++
		if r.Acc > a.bestAcc {
			a.bestAcc = r.Acc
		}
		if r.Q > a.bestQ {
			a.bestQ = r.Q
		}
		if r.RAMKiB < a.minRAM {
			a.minRAM, a.smallest = r.RAMKiB, r.ID
		}
		if r.Thru > a.maxThru {
			a.maxThru, a.fastest = r.Thru, r.ID
		}
	}
	modes := make([]LPDMode, 0, len(order))
	for _, m := range order {
		a := by[m]
		modes = append(modes, LPDMode{
			Mode: m, N: a.n, BestAcc: a.bestAcc, MinRAM: a.minRAM, MaxThru: a.maxThru,
			BestQ: a.bestQ, Smallest: a.smallest, Fastest: a.fastest,
		})
	}
	sort.SliceStable(modes, func(i, j int) bool {
		if modes[i].MinRAM != modes[j].MinRAM {
			return modes[i].MinRAM < modes[j].MinRAM
		}
		return modes[i].MaxThru > modes[j].MaxThru
	})
	if len(modes) > 12 {
		modes = modes[:12]
	}
	return std, modes
}

func rankLPD(rows []LPDRow, val func(LPDRow) float64, max int) []LPDRow {
	cp := append([]LPDRow(nil), rows...)
	sort.SliceStable(cp, func(i, j int) bool {
		if val(cp[i]) != val(cp[j]) {
			return val(cp[i]) > val(cp[j])
		}
		return cp[i].Q > cp[j].Q
	})
	if len(cp) > max {
		cp = cp[:max]
	}
	return cp
}

func lpdRow(p CellPoint, board LPD) LPDRow {
	rel := func(v, peak float64) float64 {
		if peak <= 0 {
			return 1
		}
		x := v / peak
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}
	rs, rso, ra, rt := rel(p.Score, board.PeakScore), rel(p.Soft, board.PeakSoft), rel(p.Acc, board.PeakAcc), rel(p.Thru, board.PeakThru)
	q := geomean4(rs, rso, ra, rt)
	relFast := rel(p.Thru, board.FastThru)
	relDuty := rel(p.Avail, board.BestAvail)
	ram := p.RAMKiB
	if ram <= 0 {
		ram = 1e-6
	}
	frac := ram / board.Champ.RAMKiB
	shrink := board.Champ.RAMKiB / ram
	if shrink > LPDShrinkCap {
		shrink = LPDShrinkCap
	}
	lpd, mspeed, mavail, mix := 0.0, 0.0, 0.0, 0.0
	accKeep := ra >= LPDKeepFloor
	if q >= LPDKeepFloor && accKeep {
		lpd = q * shrink
		mspeed, mavail = relFast, relDuty
		mix = geomean3(q, relFast, relDuty)
	}
	band := "—"
	gold := q >= LPDGoldKeep && ra >= LPDGoldKeep && frac <= LPDGoldRAM
	switch {
	case gold:
		band = "gold"
	case q >= LPDKeepFloor && ra >= LPDKeepFloor && frac <= LPDNearRAM:
		band = "near"
	case q >= LPDGoldKeep && ra >= LPDGoldKeep:
		band = "keep"
	case frac <= LPDGoldRAM:
		band = "trap"
	}
	return LPDRow{
		Tide: p.Tide, ID: PrettyCell(p.ID), Mode: p.Mode, DType: p.DType, Format: p.Format, Arch: PrettyArch(p.Arch),
		Score: p.Score, Soft: p.Soft, Acc: p.Acc, Thru: p.Thru, Avail: p.Avail, RAMKiB: p.RAMKiB,
		RelScore: rs, RelSoft: rso, RelAcc: ra, RelThru: rt,
		Q: q, RAMFrac: frac, Shrink: shrink, LPD: lpd, Band: band, Gold: gold,
		RelFast: relFast, RelDuty: relDuty, MSpeed: mspeed, MAvail: mavail, Mix: mix,
	}
}

func (r LPDRow) Point() CellPoint {
	return CellPoint{
		Tide: r.Tide, ID: r.ID, Mode: r.Mode, DType: r.DType, Format: r.Format, Arch: r.Arch,
		Score: r.Score, Soft: r.Soft, Acc: r.Acc, Avail: r.Avail, Thru: r.Thru, RAMKiB: r.RAMKiB,
	}
}

func geomean3(a, b, c float64) float64 {
	const eps = 1e-6
	if a < eps {
		a = eps
	}
	if b < eps {
		b = eps
	}
	if c < eps {
		c = eps
	}
	return math.Pow(a*b*c, 1.0/3.0)
}

func geomean4(a, b, c, d float64) float64 {
	const eps = 1e-6
	if a < eps {
		a = eps
	}
	if b < eps {
		b = eps
	}
	if c < eps {
		c = eps
	}
	if d < eps {
		d = eps
	}
	return math.Pow(a*b*c*d, 0.25)
}
