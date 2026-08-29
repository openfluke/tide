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

// WriteJSON encodes v as JSON with optional ETag short-circuit.
// Marshals before writing so encode failures become 500s instead of empty 200s.
func WriteJSON(w http.ResponseWriter, r *http.Request, etag string, v any) {
	if etag != "" {
		if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "json encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
	_, _ = w.Write([]byte{'\n'})
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
