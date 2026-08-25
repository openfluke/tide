package ocean

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openfluke/tide/dash"
	"github.com/openfluke/tide/report"
)

func (s *Server) handleOcean(w http.ResponseWriter, r *http.Request) {
	snap := s.cachedSnapshot()
	v := oceanView(snap)
	etag := `"ocean-` + strconv.FormatInt(v.ChartRev, 10) + `"`
	dash.WriteJSON(w, r, etag, publicizeOceanView(v, viewerHost(r)))
}

func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/charts/")
	name = strings.TrimSuffix(name, ".svg")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	snap := s.cachedSnapshot()
	src := s.chartSource(snap)
	svg, ok := dash.RenderChart(name, src, 0)
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := `"chart-` + name + `-` + strconv.FormatInt(src.Rev, 10) + `"`
	dash.WriteSVG(w, r, etag, svg)
}

func (s *Server) chartSource(snap Snapshot) dash.ChartSource {
	h := snap.Holistic
	pts := h.Heat.Points
	if len(pts) == 0 {
		pts = collectOceanPoints(snap.Peers, h.CombinedTop)
	}
	heat := h.Heat
	if len(heat.Modes) == 0 && len(pts) > 0 {
		heat = report.BuildHeat(pts)
	}
	rev := snap.UpdatedAt.UnixNano()
	if rev == 0 {
		rev = time.Now().UnixNano()
	}
	return dash.ChartSource{
		Rev:    rev,
		Points: pts,
		Heat:   heat,
		LPD:    h.LPD,
	}
}

func publicizeOceanView(v OceanView, viewer string) OceanView {
	if viewer == "" {
		return v
	}
	for i := range v.Peers {
		v.Peers[i].URL = rewriteLoopbackURL(v.Peers[i].URL, viewer)
	}
	h := v.Holistic
	for i := range h.Layers {
		h.Layers[i].URL = rewriteLoopbackURL(h.Layers[i].URL, viewer)
		for j := range h.Layers[i].Axes {
			h.Layers[i].Axes[j].URL = rewriteLoopbackURL(h.Layers[i].Axes[j].URL, viewer)
		}
	}
	for i := range h.Axes {
		h.Axes[i].URL = rewriteLoopbackURL(h.Axes[i].URL, viewer)
	}
	for i := range h.CombinedTop {
		h.CombinedTop[i].URL = rewriteLoopbackURL(h.CombinedTop[i].URL, viewer)
	}
	v.Holistic = h
	return v
}

func (s *Server) cachedSnapshot() Snapshot {
	s.cacheMu.RLock()
	if !s.cacheAt.IsZero() {
		snap := s.cacheSnap
		s.cacheMu.RUnlock()
		return snap
	}
	s.cacheMu.RUnlock()
	return s.refreshCache()
}

func (s *Server) refreshCache() Snapshot {
	states := s.pollAllPeers()
	snap := Snapshot{
		UpdatedAt: time.Now(),
		Title:     s.Title,
		Holistic:  consolidate(states),
		Peers:     states,
	}
	s.cacheMu.Lock()
	s.cache = append([]PeerState(nil), states...)
	s.cacheSnap = snap
	s.cacheAt = time.Now()
	s.cacheMu.Unlock()
	return snap
}

func (s *Server) pollAllPeers() []PeerState {
	if s.client == nil {
		s.client = &http.Client{Timeout: 2 * time.Second}
	}
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
	return states
}

func (s *Server) startPoller() {
	s.pollerOnce.Do(func() {
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for range t.C {
				s.refreshCache()
			}
		}()
	})
}
