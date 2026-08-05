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
	Completed         []Result   `json:"completed"`
	Leaderboard       []Result   `json:"leaderboard"`        // ranked by raw Lucy Score
	LeaderboardMobile []Result   `json:"leaderboard_mobile"` // ranked by Score/MiB
	Best              Best       `json:"best"`
	BestMobile        BestMobile `json:"best_mobile"` // axis winners by metric/MiB
	History     []HistoryPoint `json:"history"`     // server cache — refresh-safe
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
	cp.LeaderboardMobile = append([]Result(nil), t.live.LeaderboardMobile...)
	cp.Best = copyBest(t.live.Best)
	cp.BestMobile = copyBestMobile(t.live.BestMobile)
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

func copyBestMobile(b BestMobile) BestMobile {
	return BestMobile{
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
func (t *Tracker) Restore(completed []Result, best Best, mobile BestMobile, history []HistoryPoint, cellIdx, cellTotal int, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live.Completed = append([]Result(nil), completed...)
	t.live.Leaderboard = rankByScore(append([]Result(nil), completed...))
	t.live.LeaderboardMobile = rankByMobile(append([]Result(nil), completed...))
	t.live.Best = copyBest(best)
	t.live.BestMobile = copyBestMobile(mobile)
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

func (t *Tracker) BestMobile() BestMobile {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return copyBestMobile(t.live.BestMobile)
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

// SetCellProgress updates cell counter + message without clobbering batch totals.
func (t *Tracker) SetCellProgress(cellIdx, cellTotal int, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	// Live boards: completed + in-flight cell, refreshed every pulse.
	t.refreshBoardsLocked()
	t.refreshProvisionalBestsLocked()
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
	t.live.Current.Snapshot.Windows = nil // don't retain sparkline on every completed cell
	t.live.Current.Ended = time.Now()
	done := *t.live.Current
	t.live.Completed = append(t.live.Completed, done)
	t.live.Current = nil
	t.live.Running = false
	t.refreshBoardsLocked()
	// Committed winners: ok cells only (rebuild from completed).
	t.live.Best = Best{}
	t.live.BestMobile = BestMobile{}
	for _, r := range t.live.Completed {
		UpdateBest(&t.live.Best, r)
		UpdateBestMobile(&t.live.BestMobile, r)
	}
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
	t.live.UpdatedAt = time.Now()
	return done
}

// refreshBoardsLocked ranks completed + current in-flight row.
func (t *Tracker) refreshBoardsLocked() {
	pool := append([]Result(nil), t.live.Completed...)
	if t.live.Current != nil {
		pool = append(pool, *t.live.Current)
	}
	t.live.Leaderboard = rankByScore(pool)
	t.live.LeaderboardMobile = rankByMobile(pool)
}

// refreshProvisionalBestsLocked updates Best cards including the running cell.
func (t *Tracker) refreshProvisionalBestsLocked() {
	t.live.Best = Best{}
	t.live.BestMobile = BestMobile{}
	for _, r := range t.live.Completed {
		UpdateBest(&t.live.Best, r)
		UpdateBestMobile(&t.live.BestMobile, r)
	}
	if t.live.Current != nil {
		cur := *t.live.Current
		// Allow running into provisional winners for live UI.
		if cur.Status == "running" {
			cur.Status = "ok"
		}
		UpdateBest(&t.live.Best, cur)
		UpdateBestMobile(&t.live.BestMobile, cur)
	}
}

func rankByScore(in []Result) []Result {
	return rankAll(in, func(a, b Result) bool {
		return a.Snapshot.Score > b.Snapshot.Score
	})
}

func rankByMobile(in []Result) []Result {
	return rankAll(in, func(a, b Result) bool {
		if a.Snapshot.MobileScore != b.Snapshot.MobileScore {
			return a.Snapshot.MobileScore > b.Snapshot.MobileScore
		}
		if a.Snapshot.WeightBytes != b.Snapshot.WeightBytes && a.Snapshot.WeightBytes > 0 && b.Snapshot.WeightBytes > 0 {
			return a.Snapshot.WeightBytes < b.Snapshot.WeightBytes
		}
		return a.Snapshot.Score > b.Snapshot.Score
	})
}

func rankAll(in []Result, better func(a, b Result) bool) []Result {
	out := append([]Result(nil), in...)
	for i := 0; i < len(out); i++ {
		best := i
		for j := i + 1; j < len(out); j++ {
			if better(out[j], out[best]) {
				best = j
			}
		}
		out[i], out[best] = out[best], out[i]
	}
	return out
}
