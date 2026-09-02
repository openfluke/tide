package river

import (
	"bytes"
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web/*
var webFS embed.FS

// DefaultParentMode is used when PulseResults rebuilds a mix cell without ParentMode set.
const DefaultParentMode = "NormalBP"

// Options customize the River compare companion site for a host (near80, mixcam, …).
type Options struct {
	TideListen  string // e.g. 0.0.0.0:8204 — port used for tide_url (standalone only)
	Title       string // HTML <title> / <h1>
	Subtitle    string // header blurb
	PDFFilename string // Content-Disposition filename
	PDFTitle    string // PDF cover title
	// Integrated mounts River under the Tide dash (same origin, /compare + /api/river/*).
	Integrated bool
}

func (o Options) withDefaults() Options {
	if o.Title == "" {
		o.Title = "River compare"
	}
	if o.Subtitle == "" {
		o.Subtitle = "Tide companion — Acc, throughput, Acc-keep, LPD, and mix BranchModes."
	}
	if o.PDFFilename == "" {
		o.PDFFilename = "river_compare.pdf"
	}
	if o.PDFTitle == "" {
		o.PDFTitle = o.Title
	}
	return o
}

func (o Options) apiPrefix() string {
	if o.Integrated {
		return "/api/river"
	}
	return "/api"
}

func (o Options) compareHome() string {
	if o.Integrated {
		return "/compare"
	}
	return "/"
}

func (o Options) tideHome(r *http.Request, tidePort string) string {
	if o.Integrated {
		return "/"
	}
	return PublicURLForRequest(r, tidePort)
}

// Mount registers River routes on an existing mux (Tide dash integration).
func Mount(mux *http.ServeMux, st *Store, opts Options) {
	registerRoutes(mux, st, opts.withDefaults())
}

// Start serves the River compare site on addr (goroutine ListenAndServe).
func Start(addr string, st *Store, opts Options) *http.Server {
	opts = opts.withDefaults()
	mux := http.NewServeMux()
	registerRoutes(mux, st, opts)
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	return srv
}

func registerRoutes(mux *http.ServeMux, st *Store, opts Options) {
	_, tidePort, _ := net.SplitHostPort(opts.TideListen)
	if tidePort == "" {
		tidePort = "8203"
	}
	serveHTML := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := webFS.ReadFile(name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			b = brandHTML(b, opts)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
		}
	}
	comparePath := opts.compareHome()
	if comparePath == "/" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			serveHTML("web/index.html")(w, r)
		})
	} else {
		mux.HandleFunc(comparePath, serveHTML("web/index.html"))
	}
	mux.HandleFunc("/near", serveHTML("web/near.html"))
	mux.HandleFunc("/lpd", serveHTML("web/lpd.html"))
	mux.HandleFunc("/thru", serveHTML("web/thru.html"))

	api := opts.apiPrefix()
	mux.HandleFunc(api+"/results", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, buildCompare(st.Snapshot()))
	})
	mux.HandleFunc(api+"/near", func(w http.ResponseWriter, r *http.Request) {
		minKeep := parseMinKeep(r.URL.Query().Get("min"))
		writeJSON(w, buildNear(st.Snapshot(), minKeep))
	})
	mux.HandleFunc(api+"/lpd", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, buildLPDSearch(st.Snapshot()))
	})
	mux.HandleFunc(api+"/thru", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, buildThru(st.Snapshot()))
	})
	mux.HandleFunc(api+"/progress", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, st.Progress())
	})
	mux.HandleFunc(api+"/meta", func(w http.ResponseWriter, r *http.Request) {
		f := st.Snapshot()
		prog := st.Progress()
		writeJSON(w, map[string]any{
			"machine":     f.Machine,
			"train_n":     f.TrainN,
			"sample_seed": f.SampleSeed,
			"lrs":         f.LRs,
			"lr_labels":   f.LRLabels,
			"matrix":      f.Matrix,
			"n_rows":      len(f.Rows),
			"tide_url":    opts.tideHome(r, tidePort),
			"title":       opts.Title,
			"subtitle":    opts.Subtitle,
			"generated":   f.Generated,
			"progress":    prog,
		})
	})
	mux.HandleFunc(api+"/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		pdf, err := StorePDF(st, opts.PDFTitle)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+opts.PDFFilename+`"`)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(pdf)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func brandHTML(b []byte, opts Options) []byte {
	b = bytes.ReplaceAll(b, []byte("__RIVER_TITLE__"), []byte(opts.Title))
	b = bytes.ReplaceAll(b, []byte("__RIVER_SUBTITLE__"), []byte(opts.Subtitle))
	b = bytes.ReplaceAll(b, []byte("__API__"), []byte(opts.apiPrefix()))
	b = bytes.ReplaceAll(b, []byte("__COMPARE_HOME__"), []byte(opts.compareHome()))
	return b
}

// PublicURLForRequest builds a Tide URL from the host the browser used
// (LAN clients get 192.168.x.x:port, not 127.0.0.1).
func PublicURLForRequest(r *http.Request, port string) string {
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8203"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// DashURLs prints a clickable URL; prefer a real LAN IP over 127 when bound to all interfaces.
func DashURLs(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			port = strings.TrimPrefix(addr, ":")
			host = ""
		} else {
			return "http://" + addr
		}
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if lan := OutboundIP(); lan != "" {
			host = lan
		} else {
			host = "127.0.0.1"
		}
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "http://" + addr
	}
	return "http://" + net.JoinHostPort(host, port)
}

// OutboundIP is a best-effort LAN address for console URLs.
func OutboundIP() string {
	c, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer c.Close()
	a, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || a == nil || a.IP == nil {
		return ""
	}
	ip := a.IP
	if ip.IsLoopback() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}
