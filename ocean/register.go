package ocean

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// RegisterRequest is a worker checking in so ocean can poll it.
type RegisterRequest struct {
	Name  string   `json:"name"`
	URL   string   `json:"url"`
	Port  int      `json:"port,omitempty"`
	Layer string   `json:"layer,omitempty"`
	Modes []string `json:"modes,omitempty"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "POST /api/register", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	origin, err := resolveAdvertise(req.URL, req.Port, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Layer)
	}
	if name == "" {
		http.Error(w, "name or layer required", 400)
		return
	}
	p := Peer{Name: name, URL: origin, Layer: req.Layer, Modes: append([]string(nil), req.Modes...)}
	s.upsertPeer(p)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "peer": p})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"peers": s.peerList()})
}

func (s *Server) peerList() []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Peer(nil), s.Peers...)
}

func (s *Server) upsertPeer(p Peer) {
	p.URL = strings.TrimRight(p.URL, "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, old := range s.Peers {
		if old.Name == p.Name {
			s.Peers[i] = p
			return
		}
	}
	s.Peers = append(s.Peers, p)
}

func resolveAdvertise(posted string, port int, remoteAddr string) (string, error) {
	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		remoteHost = strings.TrimSpace(remoteAddr)
	}
	posted = strings.TrimSpace(posted)
	if posted != "" {
		u, err := url.Parse(posted)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("bad url %q", posted)
		}
		if u.Port() == "" && port > 0 {
			u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
		}
		if isLoopbackHost(u.Hostname()) || u.Hostname() == "0.0.0.0" || u.Hostname() == "::" {
			portStr := u.Port()
			if portStr == "" {
				portStr = strconv.Itoa(port)
			}
			if portStr == "" || portStr == "0" {
				return "", fmt.Errorf("url missing port")
			}
			host := remoteHost
			if host == "" || isLoopbackHost(host) {
				host = "127.0.0.1"
			}
			u.Host = net.JoinHostPort(host, portStr)
		}
		return strings.TrimRight(u.String(), "/"), nil
	}
	if port < 1 {
		return "", fmt.Errorf("url or port required")
	}
	host := remoteHost
	if host == "" || isLoopbackHost(host) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// RegisterWith posts this tide onto a remote ocean master.
func RegisterWith(ctx context.Context, oceanURL string, req RegisterRequest) error {
	oceanURL = strings.TrimRight(oceanURL, "/")
	if oceanURL == "" {
		return fmt.Errorf("ocean url required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, oceanURL+"/api/register", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("register HTTP %d", resp.StatusCode)
	}
	return nil
}

// KeepRegistered heartbeats /api/register until ctx is done.
func KeepRegistered(ctx context.Context, oceanURL string, req RegisterRequest) {
	try := func() {
		c, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if err := RegisterWith(c, oceanURL, req); err != nil {
			fmt.Fprintf(os.Stderr, "ocean register: %v\n", err)
		}
	}
	try()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			try()
		}
	}
}
