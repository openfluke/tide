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

const version = 1

// Inflight is a partially finished cell (resume mid-run).
type Inflight struct {
	Cell      permute.Cell     `json:"cell"`
	CellIndex int              `json:"cell_index"`
	ElapsedNS int64            `json:"elapsed_ns"`
	Phase     string           `json:"phase"`
	Snapshot  metrics.Snapshot `json:"snapshot"`
}

// Progress is the on-disk resume state.
type Progress struct {
	Version       int            `json:"version"`
	Mode          string         `json:"mode"`
	UpdatedAt     time.Time      `json:"updated_at"`
	CellTotal     int            `json:"cell_total"`
	NextCellIndex int            `json:"next_cell_index"`
	DoneIDs       []string       `json:"done_ids"`
	Completed     []pulse.Result `json:"completed"`
	Best          pulse.Best     `json:"best"`
	Inflight      *Inflight      `json:"inflight,omitempty"`
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
func (s *Store) inflightDir() string  { return filepath.Join(s.Root, "models", "inflight") }
func (s *Store) bestDir(kind string) string {
	return filepath.Join(s.Root, "models", "best_"+kind)
}

// Load reads progress.json if present.
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
	return &p, nil
}

// SaveAtomic writes progress.json (strips per-window history to keep file lean).
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
	cp.Completed = slimResults(p.Completed)
	cp.Best = slimBest(p.Best)
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
	return os.Rename(tmp, s.progressPath())
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

func slimPtr(r *pulse.Result) *pulse.Result {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Snapshot.Windows = nil
	return &cp
}

// SaveModel writes weights under models/<slot>/.
func (s *Store) SaveModel(slot string, m *chain.Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.Root, "models", sanitize(slot))
	return m.SaveWeightsDir(dir)
}

// LoadModel loads weights from models/<slot>/.
func (s *Store) LoadModel(slot string, m *chain.Model) error {
	dir := filepath.Join(s.Root, "models", sanitize(slot))
	return m.LoadWeightsDir(dir)
}

// SaveInflightModel saves the current cell's weights for resume.
func (s *Store) SaveInflightModel(m *chain.Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return m.SaveWeightsDir(s.inflightDir())
}

// LoadInflightModel restores inflight weights if present.
func (s *Store) LoadInflightModel(m *chain.Model) error {
	return m.LoadWeightsDir(s.inflightDir())
}

// SaveBestModels writes weights into best_* dirs when r is the current best on an axis.
func (s *Store) SaveBestModels(m *chain.Model, best pulse.Best, r pulse.Result) error {
	id := r.Cell.ID
	checks := []struct {
		kind string
		cur  *pulse.Result
	}{
		{"score", best.Score},
		{"throughput", best.Throughput},
		{"availability", best.Availability},
		{"accuracy", best.Accuracy},
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

// DoneSet returns cell IDs that should not be re-run (ok/gap only).
func DoneSet(p *Progress) map[string]bool {
	out := map[string]bool{}
	if p == nil {
		return out
	}
	for _, id := range p.DoneIDs {
		out[id] = true
	}
	for _, r := range p.Completed {
		if r.Status == "ok" || r.Status == "gap" {
			out[r.Cell.ID] = true
		}
	}
	// Inflight is still in progress — not done.
	if p.Inflight != nil {
		delete(out, p.Inflight.Cell.ID)
	}
	return out
}
