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
}

type Vote struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Mean  float64 `json:"mean_score"`
}

type LayerRow struct {
	Tide     string  `json:"tide"`
	Mode     string  `json:"mode"`
	DType    string  `json:"dtype"`
	Arch     string  `json:"arch"`
	CellID   string  `json:"cell_id"`
	Score    float64 `json:"score"`
	SoftAcc  float64 `json:"soft_acc"`
	Avail    float64 `json:"availability"`
	Thru     float64 `json:"throughput"`
	Adapt    float64 `json:"adapt_pct"`
	Done     int     `json:"done"`
	Total    int     `json:"total"`
	Recorded int     `json:"recorded"`
	Status   string  `json:"status"`
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
	Avail   float64 `json:"availability"`
	N       int     `json:"n"`
}

type ModeRow struct {
	Mode    string `json:"mode"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Running int    `json:"running"`
	Left    int    `json:"left"`
}
