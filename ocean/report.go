package ocean

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/openfluke/tide/report"
)

func (s *Server) handleReportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.BuildReport())
}

func (s *Server) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	rep := s.BuildReport()
	pdf, err := report.PDFOcean(rep)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if s.OutDir != "" {
		_ = os.MkdirAll(s.OutDir, 0o755)
		_ = os.WriteFile(filepath.Join(s.OutDir, "ocean-report.pdf"), pdf, 0o644)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="ocean-lucy-report.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func (s *Server) BuildReport() report.OceanReport {
	s.ensure()
	snap := s.Snapshot()
	out := report.OceanReport{
		Generated: time.Now(),
		Kind:      "ocean",
		Title:     snap.Title,
		Holistic: report.HolisticView{
			BestMode:   snap.Holistic.BestMode,
			BestDType:  snap.Holistic.BestDType,
			TidesUp:    snap.Holistic.TidesUp,
			TidesTotal: snap.Holistic.TidesTotal,
			CellsDone:  snap.Holistic.CellsDone,
			CellsTotal: snap.Holistic.CellsTotal,
		},
	}
	for _, v := range snap.Holistic.ModeVotes {
		out.Holistic.ModeVotes = append(out.Holistic.ModeVotes, report.Vote{Key: v.Key, Count: v.Count, Mean: v.Mean})
	}
	for _, v := range snap.Holistic.DTypeVotes {
		out.Holistic.DTypeVotes = append(out.Holistic.DTypeVotes, report.Vote{Key: v.Key, Count: v.Count, Mean: v.Mean})
	}
	for _, l := range snap.Holistic.Layers {
		out.Holistic.Layers = append(out.Holistic.Layers, report.LayerRow{
			Tide: l.Tide, Mode: l.Mode, DType: l.DType, CellID: l.CellID,
			Score: l.Score, SoftAcc: l.SoftAcc, Done: l.Done, Total: l.Total, Recorded: l.Recorded,
		})
	}
	for _, t := range snap.Holistic.CombinedTop {
		out.Holistic.CombinedTop = append(out.Holistic.CombinedTop, report.TopRow{
			Tide: t.Tide, CellID: t.Result.Cell.ID,
			Score: t.Result.Snapshot.Score, SoftAcc: t.Result.Snapshot.SoftAcc,
			Avail: t.Result.Snapshot.Availability,
		})
	}
	var (
		mu    sync.Mutex
		tides []report.TideReport
	)
	var wg sync.WaitGroup
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
			}
			if tr.Task == "" {
				tr.Task = p.Name
			}
			if tr.ID == "" {
				tr.ID = p.Name
			}
			mu.Lock()
			tides = append(tides, tr)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.SliceStable(tides, func(i, j int) bool {
		a, b := tides[i].Task, tides[j].Task
		if a == b {
			return tides[i].ID < tides[j].ID
		}
		return a < b
	})
	out.Tides = tides
	return out
}

func (s *Server) fetchTideReport(origin string) (report.TideReport, error) {
	var zero report.TideReport
	cli := &http.Client{Timeout: 12 * time.Second}
	resp, err := cli.Get(origin + "/api/report")
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var tr report.TideReport
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return zero, err
	}
	return tr, nil
}
