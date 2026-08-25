package ocean

import (
	"time"
)

// PeerSummary is a slim peer row for the ocean dashboard poll.
type PeerSummary struct {
	Name    string  `json:"name"`
	URL     string  `json:"url"`
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	AgeMS   int64   `json:"age_ms"`
	Task    string  `json:"task,omitempty"`
	ID      string  `json:"id,omitempty"`
	LR      float64 `json:"lr,omitempty"`
	Status  string  `json:"status,omitempty"`
	Running bool    `json:"running"`
	Plan    int     `json:"plan"`
	Done    int     `json:"done"`
	ProgressPct float64 `json:"progress_pct"`
}

// OceanView is the slim 1 Hz ocean dashboard payload.
type OceanView struct {
	UpdatedAt time.Time `json:"updated_at"`
	ChartRev  int64     `json:"chart_rev"`
	Title     string    `json:"title"`
	Holistic  Holistic  `json:"holistic"`
	Peers     []PeerSummary `json:"peers"`
}

func peerSummary(st PeerState) PeerSummary {
	ps := PeerSummary{
		Name: st.Name, URL: st.URL, OK: st.OK, Error: st.Error, AgeMS: st.AgeMS,
	}
	if !st.OK {
		return ps
	}
	b := st.Board
	ps.Task, ps.ID, ps.LR = b.Task, b.ID, b.LR
	ps.Status, ps.Running = b.Status, b.Running
	ps.Plan, ps.Done = b.Plan, b.EpochDone
	ps.ProgressPct = b.ProgressPct
	return ps
}

func oceanView(s Snapshot) OceanView {
	h := s.Holistic
	h.Heat.Points = nil
	peers := make([]PeerSummary, len(s.Peers))
	for i, p := range s.Peers {
		peers[i] = peerSummary(p)
	}
	rev := s.UpdatedAt.UnixNano()
	if rev == 0 {
		rev = time.Now().UnixNano()
	}
	return OceanView{
		UpdatedAt: s.UpdatedAt,
		ChartRev:  rev,
		Title:     s.Title,
		Holistic:  h,
		Peers:     peers,
	}
}
