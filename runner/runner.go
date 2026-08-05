// Package runner drives concurrent serve + train with mid-stream flips.
// Default: one full epoch over the train split per permutation cell.
package runner

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openfluke/tide/chain"
	"github.com/openfluke/tide/checkpoint"
	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/welvet/core"
)

// Sample is one labeled example batch.
type Sample struct {
	X      *core.Tensor[float32]
	Target *core.Tensor[float32] // one-hot
	Labels []int
}

// Config controls a multi-permutation adaptation run.
type Config struct {
	Spec            chain.Spec
	Cells           []permute.Cell
	BatchSize       int // permutation dashboard batch size
	Epoch           int // 1-based epoch being run (set by Run / PrepareEpoch)
	PulseEvery      time.Duration
	CheckpointEvery time.Duration
	LR              float64
	FlipAt          float64 // fraction of epoch → phase B
	FlipBack        float64 // fraction of epoch → phase A2
	Store           *checkpoint.Store
	Resume          *checkpoint.Progress
}

// DefaultConfig: 1 epoch per cell over the dataset train split.
func DefaultConfig(cells []permute.Cell) Config {
	return Config{
		Spec:            chain.DefaultMNIST(),
		Cells:           cells,
		BatchSize:       4,
		Epoch:           1,
		PulseEvery:      time.Second,
		CheckpointEvery: time.Minute,
		LR:              0.02,
		FlipAt:          1.0 / 3.0,
		FlipBack:        2.0 / 3.0,
	}
}

// Run executes all cells for one epoch (full pass over train data each).
// If the previous epoch is fully done, Resume is prepared for epoch+1.
func Run(ctx context.Context, cfg Config, ds Dataset, tr *pulse.Tracker) error {
	if ds == nil || tr == nil {
		return fmt.Errorf("runner: nil dataset/tracker")
	}
	if cfg.CheckpointEvery <= 0 {
		cfg.CheckpointEvery = time.Minute
	}
	if cfg.PulseEvery <= 0 {
		cfg.PulseEvery = time.Second
	}
	if cfg.Epoch < 1 {
		cfg.Epoch = 1
	}

	done := checkpoint.DoneSet(cfg.Resume)
	if cfg.Resume != nil {
		tr.Restore(cfg.Resume.Completed, cfg.Resume.Best, cfg.Resume.BestMobile,
			cfg.Resume.BestLearn, cfg.Resume.BestLearnMobile, cfg.Resume.History,
			cfg.Resume.NextCellIndex, len(cfg.Cells),
			fmt.Sprintf("epoch %d — %d done", cfg.Epoch, len(done)))
	}

	batches := permute.Batch(cfg.Cells, cfg.BatchSize)
	cellTotal := len(cfg.Cells)
	cellIdx := 0

	if cfg.Resume != nil && cfg.Resume.Inflight != nil {
		inf := cfg.Resume.Inflight
		cellIdx = inf.CellIndex
		tr.SetMeta(0, len(batches), cellIdx, cellTotal,
			fmt.Sprintf("epoch %d resume %s @%d/%d", cfg.Epoch, inf.Cell.ID, inf.TrainOffset, ds.TrainLen()))
		err := runCellEpoch(ctx, cfg, ds, tr, inf.Cell, cellIdx, true, inf)
		if err != nil && ctx.Err() != nil {
			return err
		}
		done[inf.Cell.ID] = true
		cellIdx = inf.CellIndex + 1
		_ = persistProgress(cfg, tr, cellIdx, nil, nil)
	}

	for bi, batch := range batches {
		for _, cell := range batch {
			if done[cell.ID] {
				cellIdx++
				continue
			}
			select {
			case <-ctx.Done():
				_ = persistProgress(cfg, tr, cellIdx, nil, nil)
				return ctx.Err()
			default:
			}
			tr.SetMeta(bi, len(batches), cellIdx, cellTotal,
				fmt.Sprintf("epoch %d · %s", cfg.Epoch, cell.ID))
			err := runCellEpoch(ctx, cfg, ds, tr, cell, cellIdx, false, nil)
			if err != nil && ctx.Err() != nil {
				return err
			}
			cellIdx++
			_ = persistProgress(cfg, tr, cellIdx, nil, nil)
		}
	}
	tr.SetMeta(len(batches), len(batches), cellTotal, cellTotal,
		fmt.Sprintf("epoch %d done", cfg.Epoch))
	_ = persistProgress(cfg, tr, cellTotal, nil, nil)
	return nil
}

func persistProgress(cfg Config, tr *pulse.Tracker, nextIdx int, inf *checkpoint.Inflight, m *chain.Model) error {
	if cfg.Store == nil {
		return nil
	}
	live := tr.Snapshot()
	doneIDs := make([]string, 0, len(live.Completed))
	for _, r := range live.Completed {
		re := r.Epoch
		if re < 1 {
			re = 1
		}
		if re == cfg.Epoch && (r.Status == "ok" || r.Status == "gap") {
			doneIDs = append(doneIDs, r.Cell.ID)
		}
	}
	if inf != nil {
		inf.CellIndex = nextIdx
		inf.Epoch = cfg.Epoch
	}
	p := &checkpoint.Progress{
		Epoch:         cfg.Epoch,
		CellTotal:     len(cfg.Cells),
		NextCellIndex: nextIdx,
		DoneIDs:       doneIDs,
		Completed:     live.Completed,
		Best:          live.Best,
		BestMobile:    live.BestMobile,
		BestLearn:     live.BestLearn,
		BestLearnMobile: live.BestLearnMobile,
		History:       live.History,
		Inflight:      inf,
	}
	if m != nil {
		if err := cfg.Store.SaveInflightModel(m); err != nil {
			return err
		}
	}
	return cfg.Store.SaveAtomic(p)
}

func phaseAtFrac(frac, flipAt, flipBack float64) string {
	if frac >= flipAt && frac < flipBack {
		return "B"
	}
	if frac >= flipBack {
		return "A2"
	}
	return "A"
}

func runCellEpoch(ctx context.Context, cfg Config, ds Dataset, tr *pulse.Tracker, cell permute.Cell, cellIdx int, resume bool, inf *checkpoint.Inflight) error {
	tr.BeginEpoch(cell, cfg.Epoch, "A")
	m, err := chain.Build(cfg.Spec, cell)
	if err != nil {
		tr.Finish("gap", err.Error(), metrics.Snapshot{})
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
		return err
	}

	offset := 0
	if resume && inf != nil {
		offset = inf.TrainOffset
		if cfg.Store != nil {
			_ = cfg.Store.LoadInflightModel(m)
		}
	} else if cfg.Epoch > 1 && cfg.Store != nil {
		// Continue weights from previous epoch of this cell.
		_ = cfg.Store.LoadModel(cell.ID, m)
	}
	ds.ResetEpoch(offset)

	start := time.Now()
	cellCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu           sync.Mutex
		phase        atomic.Value
		totalOut     atomic.Int64
		totalCorrect atomic.Int64
		totalTrain   atomic.Int64
		blockedNS    atomic.Int64
		winOut       atomic.Int64
		winCorrect   atomic.Int64
		winTrain     atomic.Int64
		winBlocked   atomic.Int64
		failNote     atomic.Value
		lastSnap     atomic.Value
		trainDone    atomic.Bool
	)
	phase.Store("A")
	if resume && inf != nil {
		if inf.Phase != "" {
			phase.Store(inf.Phase)
		}
		totalOut.Store(inf.Snapshot.TotalOutputs)
		totalCorrect.Store(inf.Snapshot.TotalCorrect)
		totalTrain.Store(inf.Snapshot.TotalTrain)
		blockedNS.Store(inf.Snapshot.BlockedTrain.Nanoseconds())
		lastSnap.Store(inf.Snapshot)
	}

	trainLen := ds.TrainLen()
	weightBytes := m.WeightBytes()
	var wg sync.WaitGroup

	// Serve loop while the epoch trains.
	// TryLock + allocate-only-when-free: the old busy-spin allocated a new MNIST
	// batch then blocked on mu while train held it — huge alloc churn → Go RSS balloon.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-cellCtx.Done():
				return
			default:
			}
			if trainDone.Load() {
				return
			}
			if !mu.TryLock() {
				runtime.Gosched()
				continue
			}
			if trainDone.Load() {
				mu.Unlock()
				return
			}
			p := phase.Load().(string)
			s := remap(ds.NextServe(p), p)
			preds, err := m.PredictArgmax(s.X)
			mu.Unlock()
			if err != nil {
				failNote.Store(err.Error())
				cancel()
				return
			}
			for i, pred := range preds {
				totalOut.Add(1)
				winOut.Add(1)
				if i < len(s.Labels) && pred == s.Labels[i] {
					totalCorrect.Add(1)
					winCorrect.Add(1)
				}
			}
		}
	}()

	// Train loop — sequential full epoch over 80% train.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			trainDone.Store(true)
			cancel()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			off := ds.EpochOffset()
			frac := 0.0
			if trainLen > 0 {
				frac = float64(off) / float64(trainLen)
			}
			p := phaseAtFrac(frac, cfg.FlipAt, cfg.FlipBack)
			phase.Store(p)

			s, ok := ds.NextTrain()
			if !ok {
				return
			}
			s = remap(s, p)
			t0 := time.Now()
			mu.Lock()
			_, err := m.TrainStep(s.X, s.Target, cfg.LR, cell.Mode)
			mu.Unlock()
			blocked := time.Since(t0)
			blockedNS.Add(blocked.Nanoseconds())
			winBlocked.Add(blocked.Nanoseconds())
			if err != nil {
				failNote.Store(err.Error())
				return
			}
			totalTrain.Add(1)
			winTrain.Add(1)
		}
	}()

	// Pulse loop.
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(cfg.PulseEvery)
		defer tick.Stop()
		var snap metrics.Snapshot
		if v := lastSnap.Load(); v != nil {
			snap = v.(metrics.Snapshot)
		}
		var pulseAccSum float64
		var pulseAccN int64
		if snap.AccuracyPulses > 0 {
			pulseAccN = snap.AccuracyPulses
			pulseAccSum = snap.AvgAccuracy * float64(pulseAccN)
		}
		for {
			select {
			case <-cellCtx.Done():
				return
			case now := <-tick.C:
				if trainDone.Load() {
					// one final pulse then exit
				}
				p := phase.Load().(string)
				w := metrics.Window{
					At:           now,
					Outputs:      winOut.Swap(0),
					Correct:      winCorrect.Swap(0),
					TrainSteps:   winTrain.Swap(0),
					BlockedTrain: time.Duration(winBlocked.Swap(0)),
					Phase:        p,
				}
				w.Accuracy = metrics.WindowAccuracy(w.Correct, w.Outputs)
				w.Throughput = float64(w.Outputs) / cfg.PulseEvery.Seconds()
				pulseAccSum += w.Accuracy
				pulseAccN++
				snap.Windows = metrics.AppendWindow(snap.Windows, w)
				snap.TotalOutputs = totalOut.Load()
				snap.TotalCorrect = totalCorrect.Load()
				snap.TotalTrain = totalTrain.Load()
				snap.BlockedTrain = time.Duration(blockedNS.Load())
				snap.Duration = time.Since(start)
				snap.WeightBytes = weightBytes
				snap.AvgAccuracy = pulseAccSum / float64(pulseAccN)
				snap.AccuracyPulses = pulseAccN
				sec := snap.Duration.Seconds()
				if snap.TimeToAcc25Sec == 0 && w.Accuracy >= metrics.AccThreshold25 {
					snap.TimeToAcc25Sec = sec
				}
				if snap.TimeToAcc50Sec == 0 && w.Accuracy >= metrics.AccThreshold50 {
					snap.TimeToAcc50Sec = sec
				}
				metrics.Finalize(&snap)
				lastSnap.Store(snap)
				tr.Pulse(w, snap, p)
				tr.SetCellProgress(cellIdx, len(cfg.Cells),
					fmt.Sprintf("epoch %d · %s · %d/%d", cfg.Epoch, cell.ID, ds.EpochOffset(), trainLen))
				if trainDone.Load() {
					return
				}
			}
		}
	}()

	// Checkpoint every minute.
	if cfg.Store != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(cfg.CheckpointEvery)
			defer tick.Stop()
			for {
				select {
				case <-cellCtx.Done():
					return
				case <-tick.C:
					var snap metrics.Snapshot
					if v := lastSnap.Load(); v != nil {
						snap = v.(metrics.Snapshot)
					}
					infight := &checkpoint.Inflight{
						Cell:        cell,
						CellIndex:   cellIdx,
						Epoch:       cfg.Epoch,
						TrainOffset: ds.EpochOffset(),
						ElapsedNS:   time.Since(start).Nanoseconds(),
						Phase:       phase.Load().(string),
						Snapshot:    snap,
					}
					mu.Lock()
					_ = persistProgress(cfg, tr, cellIdx, infight, m)
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	live := tr.Snapshot()
	snap := metrics.Snapshot{
		TotalOutputs: totalOut.Load(),
		TotalCorrect: totalCorrect.Load(),
		TotalTrain:   totalTrain.Load(),
		BlockedTrain: time.Duration(blockedNS.Load()),
		Duration:     time.Since(start),
		WeightBytes:  m.WeightBytes(),
	}
	if v := lastSnap.Load(); v != nil {
		prev := v.(metrics.Snapshot)
		snap.AvgAccuracy = prev.AvgAccuracy
		snap.AccuracyPulses = prev.AccuracyPulses
		snap.Windows = prev.Windows
		snap.TimeToAcc25Sec = prev.TimeToAcc25Sec
		snap.TimeToAcc50Sec = prev.TimeToAcc50Sec
	} else if live.Current != nil {
		snap.Windows = live.Current.Snapshot.Windows
		snap.AvgAccuracy = live.Current.Snapshot.AvgAccuracy
		snap.AccuracyPulses = live.Current.Snapshot.AccuracyPulses
		snap.TimeToAcc25Sec = live.Current.Snapshot.TimeToAcc25Sec
		snap.TimeToAcc50Sec = live.Current.Snapshot.TimeToAcc50Sec
	}
	metrics.Finalize(&snap)

	if ctx.Err() != nil {
		infight := &checkpoint.Inflight{
			Cell:        cell,
			CellIndex:   cellIdx,
			Epoch:       cfg.Epoch,
			TrainOffset: ds.EpochOffset(),
			ElapsedNS:   time.Since(start).Nanoseconds(),
			Phase:       phase.Load().(string),
			Snapshot:    snap,
		}
		mu.Lock()
		_ = persistProgress(cfg, tr, cellIdx, infight, m)
		mu.Unlock()
		return ctx.Err()
	}

	if note, ok := failNote.Load().(string); ok && note != "" {
		tr.Finish("fail", note, snap)
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
		m = nil
		return fmt.Errorf("%s", note)
	}

	done := tr.Finish("ok", "", snap)
	if cfg.Store != nil {
		best := tr.Best()
		mobile := tr.BestMobile()
		mu.Lock()
		_ = cfg.Store.SaveBestModels(m, best, mobile, done)
		_ = cfg.Store.SaveModel(cell.ID, m)
		mu.Unlock()
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
	}
	// Drop model refs so GC can reclaim before the next of 756 cells.
	m = nil
	runtime.GC()
	return nil
}

func remap(s Sample, phase string) Sample {
	if phase != "B" || len(s.Labels) == 0 {
		return s
	}
	classes := 10
	if s.Target != nil && len(s.Target.Shape) == 2 {
		classes = s.Target.Shape[1]
	}
	out := Sample{
		X:      s.X,
		Labels: make([]int, len(s.Labels)),
		Target: core.NewTensor[float32](len(s.Labels), classes),
	}
	for i, lab := range s.Labels {
		n := (lab + 5) % classes
		out.Labels[i] = n
		out.Target.Data[i*classes+n] = 1
	}
	return out
}
