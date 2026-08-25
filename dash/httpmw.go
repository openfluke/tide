package dash

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func WithGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(gzipWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	io.Writer
}

func (w gzipWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// writeJSON encodes v as JSON with optional ETag short-circuit.
func WriteJSON(w http.ResponseWriter, r *http.Request, etag string, v any) {
	if etag != "" {
		w.Header().Set("ETag", etag)
		if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func WriteSVG(w http.ResponseWriter, r *http.Request, etag string, svg []byte) {
	if etag != "" {
		w.Header().Set("ETag", etag)
		if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1")
	_, _ = w.Write(svg)
}
