// Package report builds printable Lucy reports from a tide (or ocean of tides).
package report

import (
	"time"

	"github.com/openfluke/tide/pulse"
)

// TideReport is one dashboard's Lucy snapshot for PDF/JSON.
type TideReport struct {
	Generated    time.Time            `json:"generated"`
	Kind         string               `json:"kind"` // tide | ocean
	ID           string               `json:"id"`
	Task         string               `json:"task"`
	Subtitle     string               `json:"subtitle"`
	Addr         string               `json:"addr"`
	Epoch        int                  `json:"epoch"`
	Plan         int                  `json:"plan"`
	EpochDone    int                  `json:"epoch_done"`
	Recorded     int                  `json:"recorded"`
	Status       string               `json:"status"`
	Formula      string               `json:"formula"`
	Best         pulse.Best           `json:"best"`
	BestMobile   pulse.BestMobile     `json:"best_mobile"`
	BestLearn    pulse.BestLearn      `json:"best_learn"`
	Winners      WinnersView          `json:"winners"`
	Leaderboard  []pulse.Result       `json:"leaderboard"`
	ModeProgress []ModeRow            `json:"mode_progress"`
	History      []pulse.HistoryPoint `json:"history,omitempty"`
	Axes         []AxisView           `json:"axes,omitempty"`
	Cells        []CellPoint          `json:"cells,omitempty"`
	Heat         Heat                 `json:"heat,omitempty"`
	LPD          LPD                  `json:"lpd,omitempty"`
}

// OceanReport is the master consolidation plus per-tide pages.
type OceanReport struct {
	Generated time.Time    `json:"generated"`
	Kind      string       `json:"kind"`
	Title     string       `json:"title"`
	Holistic  HolisticView `json:"holistic"`
	Tides     []TideReport `json:"tides"`
}

// HolisticView is a JSON-friendly copy of ocean.Holistic without importing ocean
// (report is used by both dash and ocean).
type HolisticView struct {
	BestMode     string     `json:"best_mode"`
	BestDType    string     `json:"best_dtype"`
	BestArch     string     `json:"best_arch"`
	ModeVotes    []Vote     `json:"mode_votes"`
	DTypeVotes   []Vote     `json:"dtype_votes"`
	ArchVotes    []Vote     `json:"arch_votes"`
	Layers       []LayerRow `json:"layers"`
	CombinedTop  []TopRow   `json:"combined_top"`
	Axes         []AxisView `json:"axes"`
	DefaultMode  string     `json:"default_mode"`
	DefaultDType string     `json:"default_dtype"`
	DefaultArch  string     `json:"default_arch"`
	DefaultWins  int        `json:"default_wins"`
	TidesUp      int        `json:"tides_up"`
	TidesTotal   int        `json:"tides_total"`
	CellsDone    int        `json:"cells_done"`
	CellsTotal   int        `json:"cells_total"`
	Heat         Heat       `json:"heat,omitempty"`
	LPD          LPD        `json:"lpd,omitempty"`
}

type Vote struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Mean  float64 `json:"mean_score"`
}

type LayerRow struct {
	Tide     string     `json:"tide"`
	Mode     string     `json:"mode"`
	DType    string     `json:"dtype"`
	Arch     string     `json:"arch"`
	CellID   string     `json:"cell_id"`
	Score    float64    `json:"score"`
	SoftAcc  float64    `json:"soft_acc"`
	Acc      float64    `json:"avg_accuracy"`
	Avail    float64    `json:"availability"`
	Thru     float64    `json:"throughput"`
	Adapt    float64    `json:"adapt_pct"`
	Done     int        `json:"done"`
	Total    int        `json:"total"`
	Recorded int        `json:"recorded"`
	Status   string     `json:"status"`
	Axes     []AxisView `json:"axes,omitempty"`
}

type AxisView struct {
	Name    string  `json:"name"`
	Hint    string  `json:"hint"`
	Tide    string  `json:"tide"`
	CellID  string  `json:"cell_id"`
	Mode    string  `json:"mode"`
	DType   string  `json:"dtype"`
	Arch    string  `json:"arch"`
	Value   float64 `json:"value"`
	SoftAcc float64 `json:"soft_acc"`
	Thru    float64 `json:"throughput"`
}

type TopRow struct {
	Tide    string  `json:"tide"`
	CellID  string  `json:"cell_id"`
	Score   float64 `json:"score"`
	SoftAcc float64 `json:"soft_acc"`
	Acc     float64 `json:"avg_accuracy"`
	Avail   float64 `json:"availability"`
}

type WinnersView struct {
	BestSettingsPerMode []WinnerRow `json:"best_settings_per_mode"`
	BestDTypePerMode    []WinnerRow `json:"best_dtype_per_mode"`
	BestModePerDType    []WinnerRow `json:"best_mode_per_dtype"`
}

type WinnerRow struct {
	Group   string  `json:"group"`
	Winner  string  `json:"winner"`
	CellID  string  `json:"cell_id"`
	Mode    string  `json:"mode"`
	DType   string  `json:"dtype"`
	Format  string  `json:"format"`
	Arch    string  `json:"arch"`
	Score   float64 `json:"score"`
	SoftAcc float64 `json:"soft_acc"`
	Acc     float64 `json:"avg_accuracy"`
	Avail   float64 `json:"availability"`
	N       int     `json:"n"`
}

// CellPoint is one finished ok cell, slim enough for heatmaps / Pareto / scatter.
type CellPoint struct {
	Tide   string  `json:"tide,omitempty"`
	Task   string  `json:"task,omitempty"`
	Layer  string  `json:"layer,omitempty"`
	ID     string  `json:"id"`
	Mode   string  `json:"mode"`
	DType  string  `json:"dtype"`
	Format string  `json:"format"`
	Arch   string  `json:"arch"`
	Score  float64 `json:"score"`
	Soft   float64 `json:"soft_acc"`
	Acc    float64 `json:"avg_accuracy"`
	Avail  float64 `json:"availability"`
	Thru   float64 `json:"throughput"`
	Adapt  float64 `json:"adapt_pct"`
	RAMKiB float64 `json:"ram_kib,omitempty"`
}

// Heat is mean Lucy metrics on the mode × dtype / mode × arch / layer × mode grids.
type Heat struct {
	Modes  []string `json:"modes,omitempty"`
	DTypes []string `json:"dtypes,omitempty"`
	Arches []string `json:"arches,omitempty"`
	Layers []string `json:"layers,omitempty"`

	ModeDTypeScore [][]float64 `json:"mode_dtype_score,omitempty"`
	ModeDTypeSoft  [][]float64 `json:"mode_dtype_soft,omitempty"`
	ModeDTypeAcc   [][]float64 `json:"mode_dtype_acc,omitempty"`
	ModeArchScore  [][]float64 `json:"mode_arch_score,omitempty"`
	ModeArchAcc    [][]float64 `json:"mode_arch_acc,omitempty"`
	LayerModeScore [][]float64 `json:"layer_mode_score,omitempty"`
	LayerModeAcc   [][]float64 `json:"layer_mode_acc,omitempty"`
	ModeDTypeAvail [][]float64 `json:"mode_dtype_avail,omitempty"`
	ModeArchAvail  [][]float64 `json:"mode_arch_avail,omitempty"`

	ModeMeanScore  []float64 `json:"mode_mean_score,omitempty"`
	ModeMeanSoft   []float64 `json:"mode_mean_soft,omitempty"`
	ModeMeanAcc    []float64 `json:"mode_mean_acc,omitempty"`
	ModeMeanAvail  []float64 `json:"mode_mean_avail,omitempty"`
	ModeMeanThru   []float64 `json:"mode_mean_thru,omitempty"`
	DTypeMeanScore []float64 `json:"dtype_mean_score,omitempty"`
	DTypeMeanAcc   []float64 `json:"dtype_mean_acc,omitempty"`
	ArchMeanScore  []float64 `json:"arch_mean_score,omitempty"`
	ArchMeanAcc    []float64 `json:"arch_mean_acc,omitempty"`

	Vs     *VsBoard    `json:"vs,omitempty"`
	Points []CellPoint `json:"points,omitempty"`
}

// VsBoard is matched deltas vs a discovered backprop analog (sgd / StepBP / NormalBP).
// Axes and modes come from the cells; nothing is hardcoded to test48's 16 names.
type VsBoard struct {
	Baseline string      `json:"baseline"`
	Modes    []VsMode    `json:"modes,omitempty"`
	ByDType  []DeltaBin  `json:"by_dtype,omitempty"`
	ByArch   []DeltaBin  `json:"by_arch,omitempty"`
	ByLayer  []DeltaBin  `json:"by_layer,omitempty"`
	Families []FamilyRow `json:"families,omitempty"`
}

// VsMode is one train mode vs the baseline, pooled over matched cells.
type VsMode struct {
	Mode       string  `json:"mode"`
	N          int     `json:"n"`
	AccDelta   float64 `json:"acc_delta"`
	AccWin     float64 `json:"acc_win"` // % of matched cells with AccΔ > 0.5
	SoftDelta  float64 `json:"soft_delta"`
	AvailDelta float64 `json:"avail_delta"`
	ThruDelta  float64 `json:"thru_delta"`
	ScoreDelta float64 `json:"score_delta"`
	ScoreWin   float64 `json:"score_win"` // % with ScoreΔ > 1
}

// DeltaBin is mean delta for one (mode × axis-key) bucket. Missing buckets stay absent.
type DeltaBin struct {
	Mode  string  `json:"mode"`
	Key   string  `json:"key"`
	N     int     `json:"n"`
	Acc   float64 `json:"acc"`
	Soft  float64 `json:"soft"`
	Avail float64 `json:"avail"`
	Thru  float64 `json:"thru"`
	Score float64 `json:"score"`
}

// FamilyRow is a dynamically found Step* vs non-Step pair (same update family).
type FamilyRow struct {
	Step       string  `json:"step"`
	Plain      string  `json:"plain"`
	N          int     `json:"n"`
	MeanAbsAcc float64 `json:"mean_abs_acc"`
}

type ModeRow struct {
	Mode    string `json:"mode"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Running int    `json:"running"`
	Left    int    `json:"left"`
}
