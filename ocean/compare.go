package ocean

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/openfluke/tide/report"
)

//go:embed compare.html
var compareHTML embed.FS

func (s *Server) BuildCompare() report.CompareReport {
	s.ensure()
	snap := s.Snapshot()
	var (
		tides []report.NamedTideReport
		mu    sync.Mutex
		wg    sync.WaitGroup
	)
	for _, p := range snap.Peers {
		if !p.OK {
			continue
		}
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr, err := s.fetchTideReport(p.URL)
			if err != nil {
				tr = p.Board.ToReport(nil)
				tr.Cells = report.PointsFromResults(p.Board.Leaderboard, p.Name)
			}
			name := p.Name
			if name == "" {
				name = tr.ID
			}
			mu.Lock()
			tides = append(tides, report.NamedTideReport{Name: name, Report: tr})
			mu.Unlock()
		}()
	}
	wg.Wait()
	return report.BuildCompare(snap.Title, tides)
}

func (s *Server) handleCompareJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.BuildCompare())
}

func (s *Server) handleComparePDF(w http.ResponseWriter, r *http.Request) {
	rep := s.BuildCompare()
	pdf, err := report.PDFCompare(rep)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if s.OutDir != "" {
		_ = os.MkdirAll(s.OutDir, 0o755)
		_ = os.WriteFile(filepath.Join(s.OutDir, "ocean-compare.pdf"), pdf, 0o644)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="ocean-compare-lr.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func (s *Server) handleComparePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/compare" {
		http.NotFound(w, r)
		return
	}
	b, err := compareHTML.ReadFile("compare.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
