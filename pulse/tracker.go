package pulse

import (
	"sort"
	"sync"
	"time"

	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
)

const defaultCompletedCap = 2000 // retained rows; Recorded tracks lifetime total

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
	LeaderboardLearn  []Result   `json:"leaderboard_learn"`  // ranked by time-to-50% (then AccPerSec)
	LeaderboardLearnMobile []Result `json:"leaderboard_learn_mobile"` // ranked by AccPerSec/MiB
	Best              Best       `json:"best"`
	BestMobile        BestMobile `json:"best_mobile"` // axis winners by metric/MiB
	BestLearn         BestLearn  `json:"best_learn"`
	BestLearnMobile   BestLearnMobile `json:"best_learn_mobile"`
	History     []HistoryPoint `json:"history"`     // server cache — refresh-safe
	Phase       string         `json:"phase"`
	BatchIndex  int            `json:"batch_index"`
	BatchTotal  int            `json:"batch_total"`
	CellIndex   int            `json:"cell_index"`
	CellTotal   int            `json:"cell_total"`
	Message     string         `json:"message"`
	HistoryLen  int            `json:"history_len"`
	Recorded    int            `json:"recorded"` // commits ever (Completed may be trimmed)
}

// Tracker is the concurrent pulse store.
type Tracker struct {
	mu           sync.RWMutex
	live         Live
	historyCap   int
	completedCap int
	reportLog    []Result // full run archive for PDF (/api/report); never trimmed
	// doneIDs is the durable skip/progress set (seeded from checkpoint DoneIDs +
	// every Commit). Survives Completed ring-buffer trim so dashboards and
	// persistProgress do not "forget" older cells.
	doneIDs map[string]bool
}

func New() *Tracker {
	return &Tracker{
		live:         Live{UpdatedAt: time.Now()},
		historyCap:   defaultHistoryCap,
		completedCap: defaultCompletedCap,
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
	cp.LeaderboardLearn = append([]Result(nil), t.live.LeaderboardLearn...)
	cp.LeaderboardLearnMobile = append([]Result(nil), t.live.LeaderboardLearnMobile...)
	cp.Best = copyBest(t.live.Best)
	cp.BestMobile = copyBestMobile(t.live.BestMobile)
	cp.BestLearn = copyBestLearn(t.live.BestLearn)
	cp.BestLearnMobile = copyBestLearnMobile(t.live.BestLearnMobile)
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

func copyBestLearn(b BestLearn) BestLearn {
	return BestLearn{
		To25:      cloneResult(b.To25),
		To50:      cloneResult(b.To50),
		AccPerSec: cloneResult(b.AccPerSec),
	}
}

func copyBestLearnMobile(b BestLearnMobile) BestLearnMobile {
	return BestLearnMobile{
		AccPerSec: cloneResult(b.AccPerSec),
		To50:      cloneResult(b.To50),
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
func (t *Tracker) Restore(completed []Result, best Best, mobile BestMobile, learn BestLearn, learnMobile BestLearnMobile, history []HistoryPoint, cellIdx, cellTotal int, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reportLog = append([]Result(nil), completed...)
	for i := range t.reportLog {
		slimSnapshot(&t.reportLog[i].Snapshot)
	}
	t.live.Completed = append([]Result(nil), completed...)
	for i := range t.live.Completed {
		s := &t.live.Completed[i].Snapshot
		if s.AccPerSec == 0 && s.Duration > 0 {
			metrics.Finalize(s)
		}
	}
	t.live.Leaderboard = rankByScore(append([]Result(nil), t.live.Completed...))
	t.live.LeaderboardMobile = rankByMobile(append([]Result(nil), t.live.Completed...))
	t.live.LeaderboardLearn = rankByLearn(append([]Result(nil), t.live.Completed...))
	t.live.LeaderboardLearnMobile = rankByLearnMobile(append([]Result(nil), t.live.Completed...))
	t.live.Best = copyBest(best)
	t.live.BestMobile = copyBestMobile(mobile)
	t.live.BestLearn = copyBestLearn(learn)
	t.live.BestLearnMobile = copyBestLearnMobile(learnMobile)
	// Rebuild learn bests if checkpoint lacked them (older progress.json).
	if t.live.BestLearn.To50 == nil && t.live.BestLearn.AccPerSec == nil {
		for _, r := range t.live.Completed {
			UpdateBestLearn(&t.live.BestLearn, r)
			UpdateBestLearnMobile(&t.live.BestLearnMobile, r)
		}
	}
	t.live.History = append([]HistoryPoint(nil), history...)
	t.trimHistoryLocked()
	t.trimCompletedLocked()
	t.live.Recorded = len(completed)
	t.live.CellIndex = cellIdx
	t.live.CellTotal = cellTotal
	t.live.Message = msg
	t.live.Running = false
	t.live.Current = nil
	t.live.UpdatedAt = time.Now()
	// Seed done set from restored archive (DoneIDs-only seeds come via SeedDoneIDs).
	for _, r := range completed {
		switch r.Status {
		case "ok", "gap":
			t.markDoneLocked(r.Cell.ID)
		}
	}
}

// SeedDoneIDs merges checkpoint / host skip IDs into the durable done set so
// progress and persist survive Completed trim and skipped (never-Committed) cells.
func (t *Tracker) SeedDoneIDs(ids []string) {
	if t == nil || len(ids) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range ids {
		t.markDoneLocked(id)
	}
}

// DoneSet is the alias-expanded finished-cell set for dashboards and checkpointing.
func (t *Tracker) DoneSet() map[string]bool {
	if t == nil {
		return map[string]bool{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]bool, len(t.doneIDs))
	for id, v := range t.doneIDs {
		if v {
			out[id] = true
		}
	}
	return out
}

// ResetDoneIDs replaces the durable skip set (used when results.json is truth).
func (t *Tracker) ResetDoneIDs(ids []string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.doneIDs = map[string]bool{}
	for _, id := range ids {
		t.markDoneLocked(id)
	}
}

// Park clears the running flag (epoch finished / idle with dashboards still up).
func (t *Tracker) Park(msg string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live.Running = false
	t.live.Current = nil
	if msg != "" {
		t.live.Message = msg
	}
	t.live.UpdatedAt = time.Now()
}

func (t *Tracker) markDoneLocked(id string) {
	if id == "" {
		return
	}
	if t.doneIDs == nil {
		t.doneIDs = map[string]bool{}
	}
	for _, a := range permute.IDAliases(id) {
		t.doneIDs[a] = true
	}
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

func (t *Tracker) BestLearn() BestLearn {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return copyBestLearn(t.live.BestLearn)
}

func (t *Tracker) BestLearnMobile() BestLearnMobile {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return copyBestLearnMobile(t.live.BestLearnMobile)
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

// ReportResults is the full completed archive for PDF /api/report (not poll-trimmed).
func (t *Tracker) ReportResults() []Result {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]Result(nil), t.reportLog...)
}

// SeedReportLog merges rows into the PDF archive (keeps larger/newer set).
// Used when results.json / a full checkpoint must refill after Completed was capped.
func (t *Tracker) SeedReportLog(rows []Result) {
	if t == nil || len(rows) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	by := map[string]int{}
	for i, r := range t.reportLog {
		by[r.Cell.ID] = i
	}
	for _, r := range rows {
		id := r.Cell.ID
		if id == "" {
			continue
		}
		if i, ok := by[id]; ok {
			t.reportLog[i] = r
			continue
		}
		by[id] = len(t.reportLog)
		t.reportLog = append(t.reportLog, r)
	}
	t.live.Recorded = len(t.reportLog)
	if t.live.Recorded < t.live.CellIndex {
		// keep CellIndex from plan progress
	}
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
		Accuracy:     snap.SoftAcc,
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
	// Keep summary metrics; drop heavy sparkline payloads.
	slimSnapshot(&t.live.Current.Snapshot)
	t.live.Current.Ended = time.Now()
	done := *t.live.Current
	t.commitLocked(done)
	return done
}

// Commit records a finished cell with explicit wall-clock times.
// Use this for multi-worker hosts where Begin/Finish on a shared Current
// would clobber siblings and report ~0s durations.
func (t *Tracker) Commit(cell permute.Cell, epoch int, status, note string, snap metrics.Snapshot, started, ended time.Time) Result {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ended.IsZero() {
		ended = time.Now()
	}
	if started.IsZero() {
		started = ended
	}
	if epoch < 1 {
		epoch = 1
	}
	slimSnapshot(&snap)
	done := Result{
		Cell:     cell,
		Epoch:    epoch,
		Status:   status,
		Note:     note,
		Snapshot: snap,
		Started:  started,
		Ended:    ended,
	}
	// Drop matching in-flight row if host had Begin'd this cell.
	if t.live.Current != nil && t.live.Current.Cell.ID == cell.ID {
		t.live.Current = nil
		t.live.Running = false
	}
	t.commitLocked(done)
	return done
}

func (t *Tracker) commitLocked(done Result) {
	slimSnapshot(&done.Snapshot)
	t.live.Completed = append(t.live.Completed, done)
	t.live.Recorded++
	t.upsertReportLocked(done)
	t.trimCompletedLocked()
	switch done.Status {
	case "ok", "gap":
		// fail is NOT durable-done — retry next pass / restart (results.json is truth).
		t.markDoneLocked(done.Cell.ID)
	}
	if t.live.Current == nil {
		t.live.Running = false
	}
	t.refreshBoardsLocked()
	UpdateBest(&t.live.Best, done)
	UpdateBestMobile(&t.live.BestMobile, done)
	UpdateBestLearn(&t.live.BestLearn, done)
	UpdateBestLearnMobile(&t.live.BestLearnMobile, done)
	t.appendHistoryLocked(HistoryPoint{
		At:           time.Now(),
		CellID:       done.Cell.ID,
		Phase:        t.live.Phase,
		Score:        done.Snapshot.Score,
		Accuracy:     done.Snapshot.SoftAcc,
		Throughput:   done.Snapshot.Throughput,
		Availability: done.Snapshot.Availability,
		CellIndex:    t.live.CellIndex,
		Outputs:      done.Snapshot.TotalOutputs,
		Status:       done.Status,
	})
	t.live.UpdatedAt = time.Now()
}

// refreshBoardsLocked ranks completed + current in-flight row.
func (t *Tracker) refreshBoardsLocked() {
	pool := append([]Result(nil), t.live.Completed...)
	if t.live.Current != nil {
		pool = append(pool, *t.live.Current)
	}
	t.live.Leaderboard = rankByScore(pool)
	t.live.LeaderboardMobile = rankByMobile(pool)
	t.live.LeaderboardLearn = rankByLearn(pool)
	t.live.LeaderboardLearnMobile = rankByLearnMobile(pool)
}

// refreshProvisionalBestsLocked updates Best cards including the running cell.
func (t *Tracker) refreshProvisionalBestsLocked() {
	if t.live.Current == nil {
		return
	}
	cur := *t.live.Current
	// Allow running into provisional winners for live UI.
	if cur.Status == "running" {
		cur.Status = "ok"
	}
	UpdateBest(&t.live.Best, cur)
	UpdateBestMobile(&t.live.BestMobile, cur)
	UpdateBestLearn(&t.live.BestLearn, cur)
	UpdateBestLearnMobile(&t.live.BestLearnMobile, cur)
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

// rankByLearn: reached 50% fastest first; never-reached last; tie-break AccPerSec.
func rankByLearn(in []Result) []Result {
	return rankAll(in, func(a, b Result) bool {
		aHit, bHit := a.Snapshot.TimeToAcc50Sec > 0, b.Snapshot.TimeToAcc50Sec > 0
		if aHit != bHit {
			return aHit
		}
		if aHit && bHit && a.Snapshot.TimeToAcc50Sec != b.Snapshot.TimeToAcc50Sec {
			return a.Snapshot.TimeToAcc50Sec < b.Snapshot.TimeToAcc50Sec
		}
		return a.Snapshot.AccPerSec > b.Snapshot.AccPerSec
	})
}

func rankByLearnMobile(in []Result) []Result {
	return rankAll(in, func(a, b Result) bool {
		if a.Snapshot.MobileAccPerSec != b.Snapshot.MobileAccPerSec {
			return a.Snapshot.MobileAccPerSec > b.Snapshot.MobileAccPerSec
		}
		aHit, bHit := a.Snapshot.TimeToAcc50Sec > 0, b.Snapshot.TimeToAcc50Sec > 0
		if aHit != bHit {
			return aHit
		}
		if aHit && bHit {
			return a.Snapshot.TimeToAcc50Sec < b.Snapshot.TimeToAcc50Sec
		}
		return a.Snapshot.AccPerSec > b.Snapshot.AccPerSec
	})
}

func rankAll(in []Result, better func(a, b Result) bool) []Result {
	out := append([]Result(nil), in...)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

func slimSnapshot(s *metrics.Snapshot) {
	if s == nil {
		return
	}
	s.Windows = nil
	s.SoftAccBlocks = nil
	s.PhaseBlocks = nil
	s.SwitchBlocks = nil
}

func (t *Tracker) trimCompletedLocked() {
	cap := t.completedCap
	if cap <= 0 {
		cap = defaultCompletedCap
	}
	if len(t.live.Completed) <= cap {
		return
	}
	drop := len(t.live.Completed) - cap
	t.live.Completed = append([]Result(nil), t.live.Completed[drop:]...)
}

func (t *Tracker) upsertReportLocked(done Result) {
	id := done.Cell.ID
	for i := range t.reportLog {
		if t.reportLog[i].Cell.ID == id {
			t.reportLog[i] = done
			return
		}
	}
	t.reportLog = append(t.reportLog, done)
}

func (t *Tracker) trimHistoryLocked() {
	if len(t.live.History) <= t.historyCap {
		return
	}
	drop := t.historyCap / 4
	if drop < 1 {
		drop = 1
	}
	t.live.History = append([]HistoryPoint(nil), t.live.History[drop:]...)
}
