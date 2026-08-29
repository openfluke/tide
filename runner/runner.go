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
	BatchSize       int // permutation dashboard batch size (grouping)
	Workers         int // concurrent cells (1 = sequential). Needs NewDataset when >1.
	NewDataset      func() Dataset // fresh Dataset per parallel cell (own train cursor)
	Epoch           int // 1-based epoch being run (set by Run / PrepareEpoch)
	PulseEvery      time.Duration
	CheckpointEvery time.Duration
	LR              float64
	FlipAt          float64       // fraction of epoch (or CellMin) → phase B
	FlipBack        float64       // fraction of epoch (or CellMin) → phase A2
	CellMin         time.Duration // 0 = one train epoch then stop (live_mnist). >0 loops train until this wall time.
	Store           *checkpoint.Store
	Resume          *checkpoint.Progress
	Build           BuildFunc // nil → chain.Build(Spec); live_mnist leaves this unset
	// CellLR optional per-cell learning rate (ok multi-LR sweeps). Falls back to LR.
	CellLR map[string]float64

	skipInflight bool       // set by Run when Workers>1 (shared inflight slot unsafe)
	storeMu      *sync.Mutex // serializes checkpoint writes when parallel
}

func (cfg Config) lrFor(cell permute.Cell) float64 {
	if cfg.CellLR != nil {
		if v, ok := cfg.CellLR[cell.ID]; ok && v > 0 {
			return v
		}
	}
	return cfg.LR
}

// DefaultConfig: 1 epoch per cell over the dataset train split.
func DefaultConfig(cells []permute.Cell) Config {
	return Config{
		Spec:            chain.DefaultMNIST(), // host may override; any Dataset + Spec works
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

// Hydrate loads checkpoint boards into the tracker so the dashboard can show
// metrics before training starts. msg overrides the status line (empty → default).
func Hydrate(tr *pulse.Tracker, cfg Config, msg string) {
	if tr == nil {
		return
	}
	done := checkpoint.DoneSet(cfg.Resume)
	planDone := checkpoint.PlanDoneCount(cfg.Cells, done)
	if msg == "" {
		if cfg.Resume != nil {
			msg = fmt.Sprintf("epoch %d — %d/%d done", cfg.Epoch, planDone, len(cfg.Cells))
		} else {
			msg = fmt.Sprintf("epoch %d — ready", cfg.Epoch)
		}
	}
	if cfg.Resume != nil {
		tr.Restore(cfg.Resume.Completed, cfg.Resume.Best, cfg.Resume.BestMobile,
			cfg.Resume.BestLearn, cfg.Resume.BestLearnMobile, cfg.Resume.History,
			planDone, len(cfg.Cells), msg)
		// Seed skip IDs that may not be in the trimmed Completed window.
		tr.SeedDoneIDs(cfg.Resume.DoneIDs)
		tr.Park(msg)
		return
	}
	tr.SetMeta(0, 0, 0, len(cfg.Cells), msg)
	tr.Park(msg)
}

// Run executes all cells for one epoch (full pass over train data each).
// If the previous epoch is fully done, Resume is prepared for epoch+1.
// Set Workers>1 and NewDataset to train multiple permute cells at once.
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
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > 1 && cfg.NewDataset == nil {
		workers = 1
	}
	if workers > 1 {
		cfg.skipInflight = true
		cfg.storeMu = &sync.Mutex{}
	}

	done := checkpoint.DoneSet(cfg.Resume)
	planDone := checkpoint.PlanDoneCount(cfg.Cells, done)
	Hydrate(tr, cfg, fmt.Sprintf("epoch %d — %d/%d done · workers=%d", cfg.Epoch, planDone, len(cfg.Cells), workers))

	batches := permute.Batch(cfg.Cells, cfg.BatchSize)
	cellTotal := len(cfg.Cells)
	cellIdx := 0

	if cfg.Resume != nil && cfg.Resume.Inflight != nil {
		inf := cfg.Resume.Inflight
		cellIdx = inf.CellIndex
		inf.Cell.Arch = permute.CanonicalArch(inf.Cell.Arch)
		inf.Cell.ID = permute.NormalizeCellID(inf.Cell.ID)
		tr.SetMeta(0, len(batches), cellIdx, cellTotal,
			fmt.Sprintf("epoch %d resume %s @%d/%d", cfg.Epoch, inf.Cell.ID, inf.TrainOffset, ds.TrainLen()))
		err := runCellEpoch(ctx, cfg, ds, tr, inf.Cell, cellIdx, true, inf)
		if err != nil && ctx.Err() != nil {
			return err
		}
		for _, a := range permute.IDAliases(inf.Cell.ID) {
			done[a] = true
		}
		cellIdx = inf.CellIndex + 1
		_ = persistProgress(cfg, tr, cellIdx, nil, nil)
	}

	if workers <= 1 {
		for bi, batch := range batches {
			for _, cell := range batch {
				if permute.IDDone(done, cell.ID) {
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
	} else {
		type job struct {
			cell permute.Cell
			idx  int
			bi   int
		}
		var jobs []job
		idx := cellIdx
		for bi, batch := range batches {
			for _, cell := range batch {
				if permute.IDDone(done, cell.ID) {
					idx++
					continue
				}
				jobs = append(jobs, job{cell: cell, idx: idx, bi: bi})
				idx++
			}
		}
		cellIdx = idx
		var (
			wg      sync.WaitGroup
			sem     = make(chan struct{}, workers)
			errOnce sync.Once
			runErr  error
			doneN   atomic.Int64
		)
		pending := int64(len(jobs))
		tr.SetMeta(0, len(batches), int(doneN.Load()), cellTotal,
			fmt.Sprintf("epoch %d · parallel×%d · %d cells queued", cfg.Epoch, workers, pending))
		for _, j := range jobs {
			j := j
			select {
			case <-ctx.Done():
				wg.Wait()
				_ = persistProgress(cfg, tr, cellIdx, nil, nil)
				return ctx.Err()
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if ctx.Err() != nil {
					return
				}
				cellDS := cfg.NewDataset()
				tr.SetMeta(j.bi, len(batches), int(doneN.Load()), cellTotal,
					fmt.Sprintf("epoch %d · ×%d · %s", cfg.Epoch, workers, j.cell.ID))
				err := runCellEpoch(ctx, cfg, cellDS, tr, j.cell, j.idx, false, nil)
				if err != nil && ctx.Err() != nil {
					errOnce.Do(func() { runErr = err })
					return
				}
				n := doneN.Add(1)
				_ = persistProgress(cfg, tr, cellIdx, nil, nil)
				tr.SetMeta(j.bi, len(batches), int(n), cellTotal,
					fmt.Sprintf("epoch %d · ×%d · finished %d/%d", cfg.Epoch, workers, n, pending))
			}()
		}
		wg.Wait()
		if runErr != nil {
			return runErr
		}
		if ctx.Err() != nil {
			_ = persistProgress(cfg, tr, cellIdx, nil, nil)
			return ctx.Err()
		}
	}

	tr.SetMeta(len(batches), len(batches), cellTotal, cellTotal,
		fmt.Sprintf("epoch %d done", cfg.Epoch))
	tr.Park(fmt.Sprintf("epoch %d done", cfg.Epoch))
	_ = persistProgress(cfg, tr, cellTotal, nil, nil)
	return nil
}

func persistProgress(cfg Config, tr *pulse.Tracker, nextIdx int, inf *checkpoint.Inflight, m Net) error {
	if cfg.Store == nil {
		return nil
	}
	if cfg.storeMu != nil {
		cfg.storeMu.Lock()
		defer cfg.storeMu.Unlock()
	}
	live := tr.Snapshot()
	// Durable done set: seeded skip IDs ∪ every Commit (not the trimmed Completed ring).
	doneSet := tr.DoneSet()
	doneIDs := make([]string, 0, len(cfg.Cells))
	for _, c := range cfg.Cells {
		if permute.IDDone(doneSet, c.ID) {
			doneIDs = append(doneIDs, c.ID)
		}
	}
	// Prefer full report archive for Completed on disk. DoneIDs already covers the
	// whole plan; trimming Completed here starved PDF/LPD (Acc champ / lean wrong).
	archive := tr.ReportResults()
	completed := checkpoint.DedupeCompleted(archive)
	if cfg.Resume != nil {
		cfg.Resume.DoneIDs = doneIDs
	}
	if inf != nil {
		inf.CellIndex = nextIdx
		inf.Epoch = cfg.Epoch
	}
	p := &checkpoint.Progress{
		Epoch:           cfg.Epoch,
		LR:              cfg.LR,
		CellTotal:       len(cfg.Cells),
		NextCellIndex:   nextIdx,
		DoneIDs:         doneIDs,
		Completed:       completed,
		Best:            live.Best,
		BestMobile:      live.BestMobile,
		BestLearn:       live.BestLearn,
		BestLearnMobile: live.BestLearnMobile,
		History:         live.History,
		Inflight:        inf,
	}
	if !cfg.skipInflight {
		if cm := asChain(m); cm != nil {
			if err := cfg.Store.SaveInflightModel(cm); err != nil {
				return err
			}
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
	m, err := cfg.buildNet(cell)
	if err != nil {
		tr.Finish("gap", err.Error(), metrics.Snapshot{})
		_ = persistProgress(cfg, tr, cellIdx+1, nil, nil)
		return err
	}

	offset := 0
	if resume && inf != nil {
		offset = inf.TrainOffset
		if cfg.Store != nil {
			if cm := asChain(m); cm != nil {
				_ = cfg.Store.LoadInflightModel(cm)
			}
		}
	} else if cfg.Epoch > 1 && cfg.Store != nil {
		// Continue weights from previous epoch of this cell (chain models only).
		if cm := asChain(m); cm != nil {
			_ = cfg.Store.LoadModel(cell.ID, cm)
		}
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
		inferNS      atomic.Int64
		trainNS      atomic.Int64
		winOut       atomic.Int64
		winCorrect   atomic.Int64
		winTrain     atomic.Int64
		winInferNS   atomic.Int64
		winTrainNS   atomic.Int64
		winSoftSum   atomic.Uint64 // float64 bits via math? use int64 micro-soft×1000
		winSoftN     atomic.Int64
		phaseSwitch  atomic.Int64
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
		inferNS.Store(int64(inf.Snapshot.InferMs * 1e6))
		trainNS.Store(int64(inf.Snapshot.TrainMs * 1e6))
		lastSnap.Store(inf.Snapshot)
	}

	trainLen := ds.TrainLen()
	weightBytes := m.WeightBytes()
	heapBytes := int64(0)
	{
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		heapBytes = int64(ms.HeapAlloc)
	}
	var wg sync.WaitGroup

	// Serve loop while the epoch trains.
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
			t0 := time.Now()
			preds, soft, err := m.ServeEval(s.X, s.Target)
			infDur := time.Since(t0)
			mu.Unlock()
			if err != nil {
				failNote.Store(err.Error())
				cancel()
				return
			}
			inferNS.Add(infDur.Nanoseconds())
			winInferNS.Add(infDur.Nanoseconds())
			winSoftN.Add(1)
			winSoftSum.Add(uint64(soft * 1e6))
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
		prevPhase := "A"
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			off := ds.EpochOffset()
			frac := 0.0
			if cfg.CellMin > 0 {
				frac = time.Since(start).Seconds() / cfg.CellMin.Seconds()
				if frac > 1 {
					frac = 1
				}
			} else if trainLen > 0 {
				frac = float64(off) / float64(trainLen)
			}
			p := phaseAtFrac(frac, cfg.FlipAt, cfg.FlipBack)
			if p != prevPhase {
				phaseSwitch.Add(1)
				prevPhase = p
			}
			phase.Store(p)
			if ps, ok := ds.(interface{ SetPhase(string) }); ok {
				ps.SetPhase(p)
			}

			s, ok := ds.NextTrain()
			if !ok {
				if cfg.CellMin > 0 && time.Since(start) < cfg.CellMin {
					ds.ResetEpoch(0)
					continue
				}
				return
			}
			if cfg.CellMin > 0 && time.Since(start) >= cfg.CellMin {
				return
			}
			s = remap(s, p)
			t0 := time.Now()
			mu.Lock()
			_, err := m.TrainStep(s.X, s.Target, cfg.lrFor(cell), cell.Mode)
			mu.Unlock()
			trDur := time.Since(t0)
			trainNS.Add(trDur.Nanoseconds())
			winTrainNS.Add(trDur.Nanoseconds())
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
		var pulseHardSum, pulseSoftSum float64
		var pulseAccN int64
		if snap.AccuracyPulses > 0 {
			pulseAccN = snap.AccuracyPulses
			pulseHardSum = snap.AvgAccuracy * float64(pulseAccN)
			pulseSoftSum = snap.SoftAcc * float64(pulseAccN)
		}
		emitPulse := func(now time.Time) {
			p := phase.Load().(string)
			softN := winSoftN.Swap(0)
			softMicro := winSoftSum.Swap(0)
			softWin := 0.0
			if softN > 0 {
				softWin = float64(softMicro) / 1e6 / float64(softN)
			}
			switches := int(phaseSwitch.Swap(0))
			w := metrics.Window{
				At:            now,
				Outputs:       winOut.Swap(0),
				Correct:       winCorrect.Swap(0),
				TrainSteps:    winTrain.Swap(0),
				InferMs:       float64(winInferNS.Swap(0)) / 1e6,
				TrainMs:       float64(winTrainNS.Swap(0)) / 1e6,
				Phase:         p,
				PhaseSwitches: switches,
				SoftAcc:       softWin,
			}
			w.BlockedTrain = time.Duration(w.TrainMs * float64(time.Millisecond))
			w.Accuracy = metrics.WindowAccuracy(w.Correct, w.Outputs)
			sec := cfg.PulseEvery.Seconds()
			if sec <= 0 {
				sec = 0.05
			}
			w.Throughput = float64(w.Outputs) / sec
			pulseHardSum += w.Accuracy
			pulseSoftSum += w.SoftAcc
			pulseAccN++
			snap.Windows = metrics.AppendWindow(snap.Windows, w)
			snap.SoftAccBlocks = append(snap.SoftAccBlocks, softWin)
			snap.PhaseBlocks = append(snap.PhaseBlocks, p)
			snap.SwitchBlocks = append(snap.SwitchBlocks, switches > 0)
			snap.TotalOutputs = totalOut.Load()
			snap.TotalCorrect = totalCorrect.Load()
			snap.TotalTrain = totalTrain.Load()
			snap.InferMs = float64(inferNS.Load()) / 1e6
			snap.TrainMs = float64(trainNS.Load()) / 1e6
			snap.BlockedTrain = time.Duration(trainNS.Load())
			snap.Duration = time.Since(start)
			snap.WeightBytes = weightBytes
			snap.HeapBytes = heapBytes
			snap.AvgAccuracy = pulseHardSum / float64(pulseAccN)
			snap.SoftAcc = pulseSoftSum / float64(pulseAccN)
			snap.AccuracyPulses = pulseAccN
			durSec := snap.Duration.Seconds()
			if snap.TimeToAcc25Sec == 0 && w.Accuracy >= metrics.AccThreshold25 {
				snap.TimeToAcc25Sec = durSec
			}
			if snap.TimeToAcc50Sec == 0 && w.Accuracy >= metrics.AccThreshold50 {
				snap.TimeToAcc50Sec = durSec
			}
			metrics.Finalize(&snap)
			lastSnap.Store(snap)
			tr.Pulse(w, snap, p)
			msg := fmt.Sprintf("epoch %d · %s · %d/%d", cfg.Epoch, cell.ID, ds.EpochOffset(), trainLen)
			if cfg.CellMin > 0 {
				msg = fmt.Sprintf("%s · %s/%s", msg, time.Since(start).Truncate(10*time.Millisecond), cfg.CellMin)
			}
			tr.SetCellProgress(cellIdx, len(cfg.Cells), msg)
		}
		for {
			select {
			case <-cellCtx.Done():
				if winSoftN.Load() > 0 || winOut.Load() > 0 || pulseAccN == 0 {
					emitPulse(time.Now())
				}
				return
			case now := <-tick.C:
				emitPulse(now)
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
		InferMs:      float64(inferNS.Load()) / 1e6,
		TrainMs:      float64(trainNS.Load()) / 1e6,
		BlockedTrain: time.Duration(trainNS.Load()),
		Duration:     time.Since(start),
		WeightBytes:  m.WeightBytes(),
		HeapBytes:    heapBytes,
	}
	if v := lastSnap.Load(); v != nil {
		prev := v.(metrics.Snapshot)
		snap.AvgAccuracy = prev.AvgAccuracy
		snap.SoftAcc = prev.SoftAcc
		snap.AccuracyPulses = prev.AccuracyPulses
		snap.Windows = prev.Windows
		snap.TimeToAcc25Sec = prev.TimeToAcc25Sec
		snap.TimeToAcc50Sec = prev.TimeToAcc50Sec
		snap.AdaptPct = prev.AdaptPct
		snap.Stability = prev.Stability
		snap.Consistency = prev.Consistency
		snap.SoftAccBlocks = prev.SoftAccBlocks
		snap.PhaseBlocks = prev.PhaseBlocks
		snap.SwitchBlocks = prev.SwitchBlocks
	} else if live.Current != nil {
		snap.Windows = live.Current.Snapshot.Windows
		snap.AvgAccuracy = live.Current.Snapshot.AvgAccuracy
		snap.SoftAcc = live.Current.Snapshot.SoftAcc
		snap.AccuracyPulses = live.Current.Snapshot.AccuracyPulses
		snap.TimeToAcc25Sec = live.Current.Snapshot.TimeToAcc25Sec
		snap.TimeToAcc50Sec = live.Current.Snapshot.TimeToAcc50Sec
	}
	// Fast cells can finish before a Lucy pulse, which left Acc=0 and Score=0.
	if snap.SoftAcc == 0 && failNote.Load() == nil {
		p := "A"
		if v := phase.Load(); v != nil {
			p, _ = v.(string)
		}
		s := remap(ds.NextServe(p), p)
		mu.Lock()
		preds, soft, err := m.ServeEval(s.X, s.Target)
		mu.Unlock()
		if err == nil {
			snap.SoftAcc = soft
			if snap.AvgAccuracy == 0 {
				snap.AvgAccuracy = evalHardAcc(preds, s.Labels)
			}
		}
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
		if cm := asChain(m); cm != nil {
			_ = cfg.Store.SaveBestModels(cm, best, mobile, done)
			_ = cfg.Store.SaveModel(cell.ID, cm)
		}
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

func evalHardAcc(preds []int, labels []int) float64 {
	n := len(preds)
	if len(labels) < n {
		n = len(labels)
	}
	if n == 0 {
		return 0
	}
	ok := 0
	for i := 0; i < n; i++ {
		if preds[i] == labels[i] {
			ok++
		}
	}
	return 100 * float64(ok) / float64(n)
}
