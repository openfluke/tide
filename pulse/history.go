package pulse

import "time"

// HistoryPoint is one dashboard pulse retained server-side (survives refresh).
type HistoryPoint struct {
	At        time.Time `json:"at"`
	CellID    string    `json:"cell_id"`
	Phase     string    `json:"phase"`
	Score     float64   `json:"score"`
	Accuracy  float64   `json:"accuracy"`
	Throughput float64  `json:"throughput"`
	Availability float64 `json:"availability"`
	CellIndex int       `json:"cell_index"`
	Outputs   int64     `json:"outputs"`
	Status    string    `json:"status,omitempty"` // set on cell finish markers
}

const defaultHistoryCap = 7200 // ~2h at 1s pulses

func (t *Tracker) appendHistoryLocked(p HistoryPoint) {
	t.live.History = append(t.live.History, p)
	if len(t.live.History) > t.historyCap {
		// drop oldest quarter
		drop := t.historyCap / 4
		if drop < 1 {
			drop = 1
		}
		t.live.History = append([]HistoryPoint(nil), t.live.History[drop:]...)
	}
}
