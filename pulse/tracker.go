package pulse

import (
	"sync"
	"time"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

// Result is one finished (or in-flight) permutation.
type Result struct {
	Cell     permute.Cell     `json:"cell"`
	Epoch    int              `json:"epoch,omitempty"` // 1-based train epoch
	Status   string           `json:"status"` // running | ok | fail | gap
	Note     string           `json:"note,omitempty"`
	Snapshot metrics.Snapshot `json:"snapshot"`
	Started  time.Time        `json:"started"`
	Ended    time.Time        `json:"ended,omitempty"`
}

// Live is the dashboard payload (polled ~1s).
type Live struct {
	UpdatedAt   time.Time      `json:"updated_at"`
	Running     bool           `json:"running"`
	Current     *Result        `json:"current,omitempty"`
	Completed   []Result       `json:"completed"`
	Leaderboard []Result       `json:"leaderboard"`
	Best        Best           `json:"best"`
	History     []HistoryPoint `json:"history"` // server cache — refresh-safe
	Phase       string         `json:"phase"`
	BatchIndex  int            `json:"batch_index"`
	BatchTotal  int            `json:"batch_total"`
	CellIndex   int            `json:"cell_index"`
	CellTotal   int            `json:"cell_total"`
	Message     string         `json:"message"`
	HistoryLen  int            `json:"history_len"`
}

// Tracker is the concurrent pulse store.
type Tracker struct {
	mu         sync.RWMutex
	live       Live
	historyCap int
}

func New() *Tracker {
	return &Tracker{
		live:       Live{UpdatedAt: time.Now()},
		historyCap: defaultHistoryCap,
	}
}

func (t *Tracker) Snapshot() Live {
	return t.snapshot(true)
}

// SnapshotLive is the 1s poll payload: no full history (tip only).
func (t *Tracker) SnapshotLive() Live {
	return t.snapshot(false)
}

func (t *Tracker) snapshot(fullHistory bool) Live {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := t.live
	if t.live.Current != nil {
		c := *t.live.Current
		cp.Current = &c
	}
	cp.Completed = append([]Result(nil), t.live.Completed...)
	cp.Leaderboard = append([]Result(nil), t.live.Leaderboard...)
	cp.Best = copyBest(t.live.Best)
	n := len(t.live.History)
	cp.HistoryLen = n
	if fullHistory {
		cp.History = append([]HistoryPoint(nil), t.live.History...)
	} else if n > 0 {
		// Tip only — browser already holds the rest (or loads /api/history once).
		cp.History = []HistoryPoint{t.live.History[n-1]}
	} else {
		cp.History = nil
	}
	return cp
}

func copyBest(b Best) Best {
	return Best{
		Score:        cloneResult(b.Score),
		Throughput:   cloneResult(b.Throughput),
		Availability: cloneResult(b.Availability),
		Accuracy:     cloneResult(b.Accuracy),
	}
}

func cloneResult(r *Result) *Result {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

// Restore loads completed results + bests + history from a checkpoint (resume).
func (t *Tracker) Restore(completed []Result, best Best, history []HistoryPoint, cellIdx, cellTotal int, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live.Completed = append([]Result(nil), completed...)
	t.live.Leaderboard = rank(append([]Result(nil), completed...))
	t.live.Best = copyBest(best)
	t.live.History = append([]HistoryPoint(nil), history...)
	t.live.CellIndex = cellIdx
	t.live.CellTotal = cellTotal
	t.live.Message = msg
	t.live.UpdatedAt = time.Now()
}

func (t *Tracker) Best() Best {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return copyBest(t.live.Best)
}

func (t *Tracker) History() []HistoryPoint {
	return t.HistoryFrom(0)
}

// HistoryFrom returns points [from:] for incremental browser sync.
func (t *Tracker) HistoryFrom(from int) []HistoryPoint {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if from < 0 {
		from = 0
	}
	if from >= len(t.live.History) {
		return nil
	}
	return append([]HistoryPoint(nil), t.live.History[from:]...)
}

func (t *Tracker) SetMeta(batchIdx, batchTotal, cellIdx, cellTotal int, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live.BatchIndex = batchIdx
	t.live.BatchTotal = batchTotal
	t.live.CellIndex = cellIdx
	t.live.CellTotal = cellTotal
	t.live.Message = msg
	t.live.UpdatedAt = time.Now()
}

func (t *Tracker) Begin(cell permute.Cell, phase string) {
	t.BeginEpoch(cell, 1, phase)
}

func (t *Tracker) BeginEpoch(cell permute.Cell, epoch int, phase string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if epoch < 1 {
		epoch = 1
	}
	t.live.Running = true
	t.live.Phase = phase
	t.live.Current = &Result{
		Cell:    cell,
		Epoch:   epoch,
		Status:  "running",
		Started: time.Now(),
	}
	t.live.UpdatedAt = time.Now()
}

func (t *Tracker) Pulse(win metrics.Window, snap metrics.Snapshot, phase string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live.Phase = phase
	cellID := ""
	if t.live.Current != nil {
		_ = win
		t.live.Current.Snapshot = snap
		cellID = t.live.Current.Cell.ID
	}
	t.appendHistoryLocked(HistoryPoint{
		At:           time.Now(),
		CellID:       cellID,
		Phase:        phase,
		Score:        snap.Score,
		Accuracy:     snap.AvgAccuracy,
		Throughput:   snap.Throughput,
		Availability: snap.Availability,
		CellIndex:    t.live.CellIndex,
		Outputs:      snap.TotalOutputs,
		Status:       "running",
	})
	t.live.UpdatedAt = time.Now()
}

func (t *Tracker) Finish(status, note string, snap metrics.Snapshot) Result {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.live.Current == nil {
		return Result{}
	}
	t.live.Current.Status = status
	t.live.Current.Note = note
	t.live.Current.Snapshot = snap
	t.live.Current.Ended = time.Now()
	done := *t.live.Current
	t.live.Completed = append(t.live.Completed, done)
	t.live.Leaderboard = rank(append([]Result(nil), t.live.Completed...))
	UpdateBest(&t.live.Best, done)
	t.appendHistoryLocked(HistoryPoint{
		At:           time.Now(),
		CellID:       done.Cell.ID,
		Phase:        t.live.Phase,
		Score:        snap.Score,
		Accuracy:     snap.AvgAccuracy,
		Throughput:   snap.Throughput,
		Availability: snap.Availability,
		CellIndex:    t.live.CellIndex,
		Outputs:      snap.TotalOutputs,
		Status:       status,
	})
	t.live.Current = nil
	t.live.Running = false
	t.live.UpdatedAt = time.Now()
	return done
}

func rank(in []Result) []Result {
	out := append([]Result(nil), in...)
	for i := 0; i < len(out); i++ {
		best := i
		for j := i + 1; j < len(out); j++ {
			if out[j].Snapshot.Score > out[best].Snapshot.Score {
				best = j
			}
		}
		out[i], out[best] = out[best], out[i]
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
