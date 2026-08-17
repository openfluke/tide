package ocean

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// viewerHost is the hostname the browser used to reach ocean (LAN IP, mDNS, …).
func viewerHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		h = strings.TrimSpace(strings.Split(h, ",")[0])
		return hostOnly(h)
	}
	return hostOnly(r.Host)
}

func hostOnly(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return ""
	}
	h, _, err := net.SplitHostPort(hostport)
	if err == nil {
		return h
	}
	return hostport
}

func isLoopbackHost(h string) bool {
	h = strings.Trim(strings.ToLower(h), "[]")
	if h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "0.0.0.0" || h == "::" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// rewriteLoopbackURL swaps a 127.0.0.1 / localhost origin for the host the
// viewer actually typed, keeping the tide's port. Ocean still polls loopback.
func rewriteLoopbackURL(poll, viewer string) string {
	poll = strings.TrimRight(poll, "/")
	if poll == "" || viewer == "" || isLoopbackHost(viewer) {
		return poll
	}
	u, err := url.Parse(poll)
	if err != nil || u.Host == "" {
		return poll
	}
	if !isLoopbackHost(u.Hostname()) {
		return poll
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	u.Host = net.JoinHostPort(viewer, port)
	return strings.TrimRight(u.String(), "/")
}

func publicizeSnapshot(s Snapshot, viewer string) Snapshot {
	if viewer == "" || isLoopbackHost(viewer) {
		return s
	}
	for i := range s.Peers {
		s.Peers[i].URL = rewriteLoopbackURL(s.Peers[i].URL, viewer)
		if a := s.Peers[i].Board.Addr; a != "" {
			if strings.Contains(a, "://") {
				s.Peers[i].Board.Addr = rewriteLoopbackURL(a, viewer)
			} else {
				s.Peers[i].Board.Addr = rewriteLoopbackURL("http://"+a, viewer)
			}
		}
	}
	h := s.Holistic
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
	s.Holistic = h
	return s
}
