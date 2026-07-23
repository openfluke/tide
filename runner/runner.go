// Package runner drives concurrent serve + train pulses with mid-stream flips.
package runner

import (
	"context"
	"fmt"
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
	Labels []int                 // class ids (pre-flip)
}

// Dataset supplies train/serve batches.
type Dataset interface {
	NextServe(phase string) Sample
	NextTrain(phase string) Sample
}

// Config controls a multi-permutation adaptation run.
type Config struct {
	Spec            chain.Spec
	Cells           []permute.Cell
	BatchSize       int
	CellDuration    time.Duration
	PulseEvery      time.Duration
	TrainEvery      time.Duration
	CheckpointEvery time.Duration
	LR              float64
	FlipAt          float64
	FlipBack        float64
	Store           *checkpoint.Store
	Resume          *checkpoint.Progress
}

// DefaultConfig returns Lucy-like timings scaled for MNIST.
func DefaultConfig(cells []permute.Cell) Config {
	return Config{
		Spec:            chain.DefaultMNIST(),
		Cells:           cells,
		BatchSize:       4,
		CellDuration:    12 * time.Second,
		PulseEvery:      time.Second,
		TrainEvery:      50 * time.Millisecond,
		CheckpointEvery: time.Minute,
		LR:              0.02,
		FlipAt:          1.0 / 3.0,
		FlipBack:        2.0 / 3.0,
	}
}

// Run executes all cells, updating tracker live. Skips completed cells when Resume is set.
func Run(ctx context.Context, cfg Config, ds Dataset, tr *pulse.Tracker) error {
	if ds == nil || tr == nil {
		return fmt.Errorf("runner: nil dataset/tracker")
	}
	if cfg.CheckpointEvery <= 0 {
		cfg.CheckpointEvery = time.Minute
	}

	done := checkpoint.DoneSet(cfg.Resume)
	if cfg.Resume != nil {
		tr.Restore(cfg.Resume.Completed, cfg.Resume.Best, cfg.Resume.NextCellIndex, len(cfg.Cells),
			fmt.Sprintf("resumed (%d done)", len(done)))
	}

	batches := permute.Batch(cfg.Cells, cfg.BatchSize)
	cellTotal := len(cfg.Cells)
	cellIdx := 0

	if cfg.Resume != nil && cfg.Resume.Inflight != nil {
		inf := cfg.Resume.Inflight
		remain := cfg.CellDuration - time.Duration(inf.ElapsedNS)
		if remain < time.Second {
			remain = time.Second
		}
		cellIdx = inf.CellIndex
		tr.SetMeta(0, len(batches), cellIdx, cellTotal, fmt.Sprintf("resume inflight %s", inf.Cell.ID))
		err := runCell(ctx, cfg, ds, tr, inf.Cell, cellIdx, remain, true, inf)
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
			tr.SetMeta(bi, len(batches), cellIdx, cellTotal, fmt.Sprintf("running %s", cell.ID))
			err := runCell(ctx, cfg, ds, tr, cell, cellIdx, cfg.CellDuration, false, nil)
			if err != nil && ctx.Err() != nil {
				return err
			}
			cellIdx++
			_ = persistProgress(cfg, tr, cellIdx, nil, nil)
		}
	}
	tr.SetMeta(len(batches), len(batches), cellTotal, cellTotal, "done")
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
		if r.Status == "ok" || r.Status == "gap" {
			doneIDs = append(doneIDs, r.Cell.ID)
		}
	}
	if inf != nil {
		inf.CellIndex = nextIdx
	}
	p := &checkpoint.Progress{
		CellTotal:     len(cfg.Cells),
		NextCellIndex: nextIdx,
		DoneIDs:       doneIDs,
		Completed:     live.Completed,
		Best:          live.Best,
		Inflight:      inf,
	}
	if m != nil {
		if err := cfg.Store.SaveInflightModel(m); err != nil {
			return err
		}
	}
	return cfg.Store.SaveAtomic(p)
}

func phaseAt(elapsed, total time.Duration, flipAt, flipBack float64) string {
	f := float64(elapsed) / float64(total)
	if f >= flipAt && f < flipBack {
		return "B"
	}
	if f >= flipBack {
		return "A2"
	}
	return "A"
}

func runCell(ctx context.Context, cfg Config, ds Dataset, tr *pulse.Tracker, cell permute.Cell, cellIdx int, duration time.Duration, resume bool, inf *checkpoint.Inflight) error {
	tr.Begin(cell, "A")
	m, err := chain.Build(cfg.Spec, cell)
	if err != nil {
		tr.Finish("gap", err.Error(), metrics.Snapshot{})
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
		return err
	}
	priorElapsed := time.Duration(0)
	if resume && inf != nil && cfg.Store != nil {
		priorElapsed = time.Duration(inf.ElapsedNS)
		_ = cfg.Store.LoadInflightModel(m)
		if inf.Phase != "" {
			tr.Pulse(metrics.Window{}, inf.Snapshot, inf.Phase)
		}
	}

	start := time.Now()
	cellCtx, cancel := context.WithTimeout(ctx, duration)
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

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-cellCtx.Done():
				return
			default:
			}
			p := phase.Load().(string)
			s := remap(ds.NextServe(p), p)
			mu.Lock()
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(cfg.TrainEvery)
		defer tick.Stop()
		for {
			select {
			case <-cellCtx.Done():
				return
			case t := <-tick.C:
				elapsed := priorElapsed + t.Sub(start)
				p := phaseAt(elapsed, cfg.CellDuration, cfg.FlipAt, cfg.FlipBack)
				phase.Store(p)
				s := remap(ds.NextTrain(p), p)
				t0 := time.Now()
				mu.Lock()
				_, err := m.TrainStep(s.X, s.Target, cfg.LR, cell.Mode)
				mu.Unlock()
				blocked := time.Since(t0)
				blockedNS.Add(blocked.Nanoseconds())
				winBlocked.Add(blocked.Nanoseconds())
				if err != nil {
					failNote.Store(err.Error())
					cancel()
					return
				}
				totalTrain.Add(1)
				winTrain.Add(1)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(cfg.PulseEvery)
		defer tick.Stop()
		var snap metrics.Snapshot
		if v := lastSnap.Load(); v != nil {
			snap = v.(metrics.Snapshot)
		}
		for {
			select {
			case <-cellCtx.Done():
				return
			case now := <-tick.C:
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
				snap.Windows = append(snap.Windows, w)
				snap.TotalOutputs = totalOut.Load()
				snap.TotalCorrect = totalCorrect.Load()
				snap.TotalTrain = totalTrain.Load()
				snap.BlockedTrain = time.Duration(blockedNS.Load())
				snap.Duration = priorElapsed + now.Sub(start)
				metrics.Finalize(&snap)
				lastSnap.Store(snap)
				tr.Pulse(w, snap, p)
			}
		}
	}()

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
				case now := <-tick.C:
					elapsed := priorElapsed + now.Sub(start)
					p := phase.Load().(string)
					var snap metrics.Snapshot
					if v := lastSnap.Load(); v != nil {
						snap = v.(metrics.Snapshot)
					}
					infight := &checkpoint.Inflight{
						Cell:      cell,
						CellIndex: cellIdx,
						ElapsedNS: elapsed.Nanoseconds(),
						Phase:     p,
						Snapshot:  snap,
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
		Duration:     priorElapsed + duration,
	}
	if live.Current != nil {
		snap.Windows = live.Current.Snapshot.Windows
	}
	if snap.Duration > cfg.CellDuration {
		snap.Duration = cfg.CellDuration
	}
	metrics.Finalize(&snap)

	// Parent cancelled — keep inflight, do not mark cell done.
	if ctx.Err() != nil {
		elapsed := priorElapsed + time.Since(start)
		infight := &checkpoint.Inflight{
			Cell:      cell,
			CellIndex: cellIdx,
			ElapsedNS: elapsed.Nanoseconds(),
			Phase:     phase.Load().(string),
			Snapshot:  snap,
		}
		mu.Lock()
		_ = persistProgress(cfg, tr, cellIdx, infight, m)
		mu.Unlock()
		return ctx.Err()
	}

	if note, ok := failNote.Load().(string); ok && note != "" {
		tr.Finish("fail", note, snap)
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
		return fmt.Errorf("%s", note)
	}

	done := tr.Finish("ok", "", snap)
	if cfg.Store != nil {
		best := tr.Best()
		mu.Lock()
		_ = cfg.Store.SaveBestModels(m, best, done)
		_ = cfg.Store.SaveModel(cell.ID, m)
		mu.Unlock()
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
	}
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
