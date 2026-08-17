package ocean

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openfluke/tide/dash"
)

//go:embed ocean.html
var static embed.FS

// Peer is one tide dashboard to poll.
type Peer struct {
	Name  string   `json:"name"`
	URL   string   `json:"url"` // origin ocean should poll, e.g. http://192.168.1.21:8101
	Layer string   `json:"layer,omitempty"`
	Modes []string `json:"modes,omitempty"`
}

// PeerState is the last successful (or failed) poll of one tide.
type PeerState struct {
	Name    string     `json:"name"`
	URL     string     `json:"url"`
	OK      bool       `json:"ok"`
	Error   string     `json:"error,omitempty"`
	AgeMS   int64      `json:"age_ms"`
	Fetched time.Time  `json:"fetched"`
	Board   dash.Board `json:"board"`
}

// Snapshot is the ocean API payload.
type Snapshot struct {
	UpdatedAt time.Time   `json:"updated_at"`
	Title     string      `json:"title"`
	Holistic  Holistic    `json:"holistic"`
	Peers     []PeerState `json:"peers"`
}

// Server is a tide-of-tides: it does not train; it polls child dashboards.
type Server struct {
	Addr   string
	Title  string
	Peers  []Peer
	OutDir string // optional; /api/report.pdf also writes ocean-report.pdf here

	client *http.Client
	mu     sync.Mutex
	cache  []PeerState
}

func (s *Server) ensure() {
	if s.client == nil {
		s.client = &http.Client{Timeout: 2 * time.Second}
	}
	if s.Title == "" {
		s.Title = "ocean"
	}
}

func (s *Server) Handler() http.Handler {
	s.ensure()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := static.ReadFile("ocean.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/ocean", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(publicizeSnapshot(s.Snapshot(), viewerHost(r)))
	})
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "ocean",
			"task":  s.Title,
			"ocean": true,
			"peers": len(s.peerList()),
			"addr":  s.Addr,
			"apis":  map[string]string{"ocean": "/api/ocean", "start_all": "/api/start-all", "start": "/api/start", "register": "/api/register", "peers": "/api/peers", "report": "/api/report.pdf"},
		})
	})
	mux.HandleFunc("/api/report.pdf", s.handleReportPDF)
	mux.HandleFunc("/api/report", s.handleReportJSON)
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/start-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "POST /api/start-all", http.StatusMethodNotAllowed)
			return
		}
		results := s.startAll(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"started": results})
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "POST /api/start?peer=name", http.StatusMethodNotAllowed)
			return
		}
		name := r.URL.Query().Get("peer")
		if name == "" {
			http.Error(w, "peer query required", 400)
			return
		}
		err := s.startOne(r.Context(), name)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "peer": name})
	})
	return corsWrap(mux)
}

func corsWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe blocks on Addr.
func (s *Server) ListenAndServe() error {
	s.ensure()
	if s.Addr == "" {
		s.Addr = "0.0.0.0:8090"
	}
	return http.ListenAndServe(s.Addr, s.Handler())
}

// Snapshot polls every peer (in parallel) and consolidates winners.
func (s *Server) Snapshot() Snapshot {
	s.ensure()
	peers := s.peerList()
	states := make([]PeerState, len(peers))
	var wg sync.WaitGroup
	for i, p := range peers {
		wg.Add(1)
		go func(i int, p Peer) {
			defer wg.Done()
			states[i] = s.fetchPeer(p)
		}(i, p)
	}
	wg.Wait()
	s.mu.Lock()
	s.cache = append([]PeerState(nil), states...)
	s.mu.Unlock()
	return Snapshot{
		UpdatedAt: time.Now(),
		Title:     s.Title,
		Holistic:  consolidate(states),
		Peers:     states,
	}
}

func (s *Server) fetchPeer(p Peer) PeerState {
	st := PeerState{Name: p.Name, URL: strings.TrimRight(p.URL, "/")}
	if st.Name == "" {
		st.Name = st.URL
	}
	t0 := time.Now()
	board, err := s.getBoard(st.URL)
	st.AgeMS = time.Since(t0).Milliseconds()
	st.Fetched = time.Now()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.OK = true
	st.Board = board
	if board.ID != "" && p.Name == "" {
		st.Name = board.ID
	}
	return st
}

func (s *Server) getBoard(origin string) (dash.Board, error) {
	var zero dash.Board
	resp, err := s.client.Get(origin + "/api/board")
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return zero, fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var board dash.Board
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		return zero, err
	}
	return board, nil
}

func (s *Server) startAll(ctx context.Context) map[string]any {
	peers := s.peerList()
	out := make(map[string]any, len(peers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(p Peer) {
			defer wg.Done()
			err := s.postStart(ctx, strings.TrimRight(p.URL, "/"))
			name := p.Name
			if name == "" {
				name = p.URL
			}
			mu.Lock()
			if err != nil {
				out[name] = err.Error()
			} else {
				out[name] = "ok"
			}
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return out
}

func (s *Server) startOne(ctx context.Context, name string) error {
	for _, p := range s.peerList() {
		if p.Name == name || p.URL == name {
			return s.postStart(ctx, strings.TrimRight(p.URL, "/"))
		}
	}
	return fmt.Errorf("unknown peer %q", name)
}

func (s *Server) postStart(ctx context.Context, origin string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/start", nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
