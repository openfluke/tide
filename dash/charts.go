package dash

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/openfluke/tide/pulse"
	"github.com/openfluke/tide/report"
)

type ChartSource struct {
	Rev     int64
	Points  []report.CellPoint
	Heat    report.Heat
	LPD     report.LPD
	History []pulse.HistoryPoint
	Best    pulse.Best
}

func (s *Server) chartSource() ChartSource {
	live := s.Tracker.SnapshotLive()
	rev := live.UpdatedAt.UnixNano()
	task := ""
	if s != nil {
		task = s.Task
	}
	// Full archive — live.Completed is trimmed and starves LPD/radars/scatters.
	completed := s.reportCompleted(live)
	pts := report.PointsFromResults(completed, task)
	heat := report.BuildHeat(pts)
	lpd := report.BuildLPD(pts)
	return ChartSource{
		Rev:     rev,
		Points:  pts,
		Heat:    heat,
		LPD:     lpd,
		History: s.Tracker.History(),
		Best:    live.Best,
	}
}

func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/charts/")
	name = strings.TrimSuffix(name, ".svg")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	src := s.chartSource()
	end := len(src.History) - 1
	if v := r.URL.Query().Get("end"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			end = n
		}
	}
	svg, ok := RenderChart(name, src, end)
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := `"chart-` + name + `-` + strconv.FormatInt(src.Rev, 10) + `"`
	WriteSVG(w, r, etag, svg)
}

// RenderChart produces server-side SVG for a named live chart.
func RenderChart(name string, src ChartSource, end int) ([]byte, bool) {
	champRAM := src.LPD.AccChamp.RAMKiB * 0.2
	switch name {
	case "pulse":
		return report.PulseSVG(src.History, src.Best, end), true
	case "radar-live":
		live, _ := report.RadarFromLPD(src.LPD)
		return report.RadarSVG("", live), true
	case "radar-dens":
		_, dens := report.RadarFromLPD(src.LPD)
		return report.RadarSVG("", dens), true
	case "scatter-avail-acc":
		return report.ScatterSVG(src.Points, "availability", "avg_accuracy", ""), true
	case "scatter-soft-score":
		return report.ScatterSVG(src.Points, "soft_acc", "score", ""), true
	case "scatter-acc-score":
		return report.ScatterSVG(src.Points, "avg_accuracy", "score", ""), true
	case "scatter-soft-acc":
		return report.ScatterSVG(src.Points, "soft_acc", "avg_accuracy", ""), true
	case "scatter-thru-acc":
		return report.ScatterSVG(src.Points, "throughput", "avg_accuracy", ""), true
	case "scatter-thru-avail":
		return report.ScatterSVG(src.Points, "throughput", "availability", ""), true
	case "scatter-adapt-acc":
		if !anyAdapt(src.Points) {
			return report.ScatterSVG(nil, "adapt_pct", "avg_accuracy", ""), true
		}
		return report.ScatterSVG(src.Points, "adapt_pct", "avg_accuracy", ""), true
	case "lpd-ram":
		return report.LPDScatterSVG(report.LPDScatterPoints(src.LPD), "ram", "qpct", champRAM*5, 80, false), true
	case "lpd-acc":
		return report.LPDScatterSVG(report.LPDScatterPoints(src.LPD), "ram", "accpct", champRAM*5, 80, false), true
	case "lpd-shrink":
		return report.LPDScatterSVG(report.LPDScatterPoints(src.LPD), "shrink", "qpct", 5, 80, true), true
	case "heat-score":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.DTypes, src.Heat.ModeDTypeScore, "", false), true
	case "heat-acc":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.DTypes, src.Heat.ModeDTypeAcc, "", false), true
	case "heat-soft":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.DTypes, src.Heat.ModeDTypeSoft, "", false), true
	case "heat-avail":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.DTypes, src.Heat.ModeDTypeAvail, "", false), true
	case "heat-arch":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.Arches, src.Heat.ModeArchScore, "", false), true
	case "heat-arch-avail":
		return report.HeatmapSVG(src.Heat.Modes, src.Heat.Arches, src.Heat.ModeArchAvail, "", false), true
	case "heat-layer":
		return report.HeatmapSVG(src.Heat.Layers, src.Heat.Modes, src.Heat.LayerModeScore, "", false), true
	default:
		return nil, false
	}
}

func anyAdapt(pts []report.CellPoint) bool {
	for _, p := range pts {
		if p.Adapt > 0.05 {
			return true
		}
	}
	return false
}

// ChartNames lists server-rendered chart endpoints for the dashboard.
func ChartNames() []string {
	return []string{
		"pulse", "radar-live", "radar-dens",
		"scatter-avail-acc", "scatter-soft-score", "scatter-acc-score",
		"scatter-soft-acc", "scatter-thru-acc", "scatter-thru-avail", "scatter-adapt-acc",
		"lpd-ram", "lpd-acc", "lpd-shrink",
		"heat-score", "heat-acc", "heat-soft", "heat-avail", "heat-arch", "heat-arch-avail", "heat-layer",
	}
}
