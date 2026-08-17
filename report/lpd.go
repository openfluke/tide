package report

import (
	"math"
	"sort"
)

// LPD is the Lucy Pareto / goldilocks board: stay close to the quality
// champion, then shrink RAM. Raw Score/MiB is the binary trap — a 1-bit
// net with chance Acc still "wins" MobileScore.
//
// Q is the geometric mean of Score/Soft/Acc/Thru vs the Score champion
// (each ratio capped at 1). Binary's high Thru must not set the Thru peak.
// LPD = 0 unless Q ≥ 70%. Gold = Q ≥ 80% and RAM ≤ 20% of the champion.
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

// LPD is the goldilocks snapshot for a tide or the ocean.
type LPD struct {
	Formula   string   `json:"formula"`
	Champ     LPDChamp `json:"champ"`
	PeakScore float64  `json:"peak_score"`
	PeakSoft  float64  `json:"peak_soft"`
	PeakAcc   float64  `json:"peak_acc"`
	PeakThru  float64  `json:"peak_thru"`
	FastThru  float64  `json:"fast_thru"` // fastest Throughput on the board
	FastID    string   `json:"fast_id,omitempty"`
	BestAvail float64  `json:"best_avail"` // highest Availability on the board
	AvailID   string   `json:"avail_id,omitempty"`
	N         int      `json:"n"`
	Gold      []LPDRow `json:"gold,omitempty"`
	Near      []LPDRow `json:"near,omitempty"`
	Top       []LPDRow `json:"top,omitempty"`
	Trap      []LPDRow `json:"trap,omitempty"`
	TopSpeed  []LPDRow `json:"top_speed,omitempty"`
	TopAvail  []LPDRow `json:"top_avail,omitempty"`
	TopMix    []LPDRow `json:"top_mix,omitempty"`
}

func lpdFormula() string {
	return "Q = geomean of Score/Soft/Acc/Thru vs the Lucy Score champion. LPD = 0 if Q<70%, else Q × shrink vs champ RAM (capped 32×). Gold = Q≥80% and RAM≤20% of champ. MSpeed = Thru vs the board's fastest (0 if Q<70%). MAvail = Availability vs the board's best duty cycle (0 if Q<70%). Mix = geomean(Q, MSpeed, MAvail). Raw Score/MiB is the binary trap."
}

// BuildLPD ranks cells for the goldilocks: good enough quality, then smaller RAM.
func BuildLPD(pts []CellPoint) LPD {
	out := LPD{Formula: lpdFormula()}
	if len(pts) == 0 {
		return out
	}
	var champ CellPoint
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
		if i == 0 || p.Thru > fastThru {
			fastThru, fastID = p.Thru, p.ID
		}
		if i == 0 || p.Avail > bestAvail {
			bestAvail, availID = p.Avail, p.ID
		}
	}
	out.PeakScore, out.PeakSoft, out.PeakAcc, out.PeakThru = champ.Score, champ.Soft, champ.Acc, champ.Thru
	out.FastThru, out.FastID = fastThru, PrettyCell(fastID)
	out.BestAvail, out.AvailID = bestAvail, PrettyCell(availID)
	if champ.RAMKiB <= 0 {
		champ.RAMKiB = 1e-6
	}
	out.Champ = LPDChamp{
		ID: champ.ID, Tide: champ.Tide, Mode: champ.Mode, DType: champ.DType, Arch: champ.Arch,
		Score: champ.Score, Soft: champ.Soft, Acc: champ.Acc, Thru: champ.Thru, Avail: champ.Avail,
		RAMKiB: champ.RAMKiB,
	}
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
	return out
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
	if q >= LPDKeepFloor {
		lpd = q * shrink
		mspeed, mavail = relFast, relDuty
		mix = geomean3(q, relFast, relDuty)
	}
	band := "—"
	gold := q >= LPDGoldKeep && frac <= LPDGoldRAM
	switch {
	case gold:
		band = "gold"
	case q >= LPDKeepFloor && frac <= LPDNearRAM:
		band = "near"
	case q >= LPDGoldKeep:
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
