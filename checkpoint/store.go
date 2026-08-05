// Package checkpoint persists sweep progress, scores, and model weights.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openfluke/tide/chain"
	"github.com/openfluke/tide/metrics"
	"github.com/openfluke/tide/permute"
	"github.com/openfluke/tide/pulse"
)

const version = 2

// Inflight is a partially finished cell (resume mid-epoch).
type Inflight struct {
	Cell        permute.Cell     `json:"cell"`
	CellIndex   int              `json:"cell_index"`
	Epoch       int              `json:"epoch"`
	TrainOffset int              `json:"train_offset"` // examples consumed in this epoch
	ElapsedNS   int64            `json:"elapsed_ns"`
	Phase       string           `json:"phase"`
	Snapshot    metrics.Snapshot `json:"snapshot"`
}

// Progress is the on-disk resume state.
type Progress struct {
	Version       int                  `json:"version"`
	Mode          string               `json:"mode"`
	Epoch         int                  `json:"epoch"` // 1-based; re-run after full sweep bumps this
	UpdatedAt     time.Time            `json:"updated_at"`
	CellTotal     int                  `json:"cell_total"`
	NextCellIndex int                  `json:"next_cell_index"`
	DoneIDs       []string             `json:"done_ids"`
	Completed     []pulse.Result       `json:"completed"`
	Best          pulse.Best           `json:"best"`
	BestMobile    pulse.BestMobile     `json:"best_mobile"`
	BestLearn     pulse.BestLearn      `json:"best_learn"`
	BestLearnMobile pulse.BestLearnMobile `json:"best_learn_mobile"`
	History       []pulse.HistoryPoint `json:"history,omitempty"`
	Inflight      *Inflight            `json:"inflight,omitempty"`
}

// Store writes progress.json + model weight dirs under Root.
type Store struct {
	mu   sync.Mutex
	Root string
	Mode string
}

func New(root, mode string) *Store {
	if root == "" {
		root = "checkpoint"
	}
	return &Store{Root: root, Mode: mode}
}

func (s *Store) progressPath() string { return filepath.Join(s.Root, "progress.json") }
func (s *Store) historyPath() string  { return filepath.Join(s.Root, "history.json") }
func (s *Store) inflightDir() string  { return filepath.Join(s.Root, "models", "inflight") }
func (s *Store) bestDir(kind string) string {
	return filepath.Join(s.Root, "models", "best_"+kind)
}

// Load reads progress.json (+ history.json) if present.
func (s *Store) Load() (*Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.progressPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var p Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Epoch < 1 {
		p.Epoch = 1
	}
	// v1 was wall-clock cells — keep bests/history/models, but re-run full epochs.
	if p.Version < 2 {
		p.DoneIDs = nil
		p.Completed = nil
		p.Inflight = nil
		p.NextCellIndex = 0
		p.Epoch = 1
		p.Version = 2
	}
	if hb, err := os.ReadFile(s.historyPath()); err == nil {
		var hist []pulse.HistoryPoint
		if json.Unmarshal(hb, &hist) == nil {
			p.History = hist
		}
	}
	return &p, nil
}

// SaveAtomic writes progress.json + history.json.
func (s *Store) SaveAtomic(p *Progress) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	cp := *p
	cp.Version = version
	cp.Mode = s.Mode
	cp.UpdatedAt = time.Now()
	if cp.Epoch < 1 {
		cp.Epoch = 1
	}
	cp.Completed = slimResults(p.Completed)
	cp.Best = slimBest(p.Best)
	cp.BestMobile = slimBestMobile(p.BestMobile)
	cp.BestLearn = slimBestLearn(p.BestLearn)
	cp.BestLearnMobile = slimBestLearnMobile(p.BestLearnMobile)
	hist := append([]pulse.HistoryPoint(nil), p.History...)
	cp.History = nil
	if cp.Inflight != nil {
		inf := *cp.Inflight
		inf.Snapshot.Windows = nil
		cp.Inflight = &inf
	}
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.progressPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.progressPath()); err != nil {
		return err
	}
	hb, err := json.Marshal(hist)
	if err != nil {
		return err
	}
	htmp := s.historyPath() + ".tmp"
	if err := os.WriteFile(htmp, hb, 0o644); err != nil {
		return err
	}
	return os.Rename(htmp, s.historyPath())
}

func slimResults(in []pulse.Result) []pulse.Result {
	out := make([]pulse.Result, len(in))
	for i, r := range in {
		out[i] = r
		out[i].Snapshot.Windows = nil
	}
	return out
}

func slimBest(b pulse.Best) pulse.Best {
	return pulse.Best{
		Score:        slimPtr(b.Score),
		Throughput:   slimPtr(b.Throughput),
		Availability: slimPtr(b.Availability),
		Accuracy:     slimPtr(b.Accuracy),
	}
}

func slimBestMobile(b pulse.BestMobile) pulse.BestMobile {
	return pulse.BestMobile{
		Score:        slimPtr(b.Score),
		Throughput:   slimPtr(b.Throughput),
		Availability: slimPtr(b.Availability),
		Accuracy:     slimPtr(b.Accuracy),
	}
}

func slimBestLearn(b pulse.BestLearn) pulse.BestLearn {
	return pulse.BestLearn{
		To25:      slimPtr(b.To25),
		To50:      slimPtr(b.To50),
		AccPerSec: slimPtr(b.AccPerSec),
	}
}

func slimBestLearnMobile(b pulse.BestLearnMobile) pulse.BestLearnMobile {
	return pulse.BestLearnMobile{
		AccPerSec: slimPtr(b.AccPerSec),
		To50:      slimPtr(b.To50),
	}
}

func slimPtr(r *pulse.Result) *pulse.Result {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Snapshot.Windows = nil
	return &cp
}

func (s *Store) SaveModel(slot string, m *chain.Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.Root, "models", sanitize(slot))
	return m.SaveWeightsDir(dir)
}

func (s *Store) LoadModel(slot string, m *chain.Model) error {
	dir := filepath.Join(s.Root, "models", sanitize(slot))
	return m.LoadWeightsDir(dir)
}

func (s *Store) SaveInflightModel(m *chain.Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return m.SaveWeightsDir(s.inflightDir())
}

func (s *Store) LoadInflightModel(m *chain.Model) error {
	return m.LoadWeightsDir(s.inflightDir())
}

func (s *Store) SaveBestModels(m *chain.Model, best pulse.Best, mobile pulse.BestMobile, r pulse.Result) error {
	id := r.Cell.ID
	checks := []struct {
		kind string
		cur  *pulse.Result
	}{
		{"score", best.Score},
		{"throughput", best.Throughput},
		{"availability", best.Availability},
		{"accuracy", best.Accuracy},
		{"mobile_score", mobile.Score},
		{"mobile_throughput", mobile.Throughput},
		{"mobile_availability", mobile.Availability},
		{"mobile_accuracy", mobile.Accuracy},
	}
	for _, c := range checks {
		if c.cur != nil && c.cur.Cell.ID == id {
			if err := m.SaveWeightsDir(s.bestDir(c.kind)); err != nil {
				return fmt.Errorf("best_%s: %w", c.kind, err)
			}
		}
	}
	return nil
}

func sanitize(id string) string {
	r := strings.NewReplacer("|", "_", "/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(id)
}

// DoneSet returns cell IDs finished for the current epoch.
func DoneSet(p *Progress) map[string]bool {
	out := map[string]bool{}
	if p == nil {
		return out
	}
	epoch := p.Epoch
	if epoch < 1 {
		epoch = 1
	}
	for _, id := range p.DoneIDs {
		out[id] = true
	}
	for _, r := range p.Completed {
		re := r.Epoch
		if re < 1 {
			re = 1
		}
		if re == epoch && (r.Status == "ok" || r.Status == "gap") {
			out[r.Cell.ID] = true
		}
	}
	if p.Inflight != nil {
		delete(out, p.Inflight.Cell.ID)
	}
	return out
}

// AllDone reports whether every cell has an ok/gap result for this epoch.
func AllDone(p *Progress, cells []permute.Cell) bool {
	if p == nil || len(cells) == 0 {
		return false
	}
	done := DoneSet(p)
	if p.Inflight != nil {
		return false
	}
	for _, c := range cells {
		if !done[c.ID] {
			return false
		}
	}
	return true
}

// PrepareEpoch bumps to the next epoch when the previous sweep finished.
// Returns the epoch to run and a progress view with DoneIDs cleared for that epoch.
func PrepareEpoch(p *Progress, cells []permute.Cell) (epoch int, resume *Progress) {
	if p == nil {
		return 1, nil
	}
	cp := *p
	if cp.Epoch < 1 {
		cp.Epoch = 1
	}
	if AllDone(&cp, cells) {
		cp.Epoch++
		cp.DoneIDs = nil
		cp.NextCellIndex = 0
		cp.Inflight = nil
		// Keep Completed/Best/History — new epoch appends more results.
	}
	return cp.Epoch, &cp
}
