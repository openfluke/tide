package dash

import (
	"context"
	"time"
)

// Started reports whether training has been released from the pause gate.
func (s *Server) Started() bool {
	s.ensureGate()
	s.startMu.Lock()
	defer s.startMu.Unlock()
	return s.started
}

// SignalStart releases AwaitStart. Safe to call more than once.
func (s *Server) SignalStart() {
	s.ensureGate()
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.started {
		return
	}
	s.started = true
	close(s.startCh)
	s.resetPaceAnchor()
}

// ResetPaceAnchor restarts wall-clock ETA from "now" (e.g. after seeding ckpt cells).
func (s *Server) ResetPaceAnchor() {
	if s == nil {
		return
	}
	s.resetPaceAnchor()
}

func (s *Server) resetPaceAnchor() {
	s.paceMu.Lock()
	defer s.paceMu.Unlock()
	s.paceAt = time.Now()
	n := 0
	if s.Tracker != nil {
		n = len(s.Tracker.SnapshotLive().Completed)
	}
	s.paceDoneBase = n
}

func (s *Server) paceAnchor() (at time.Time, base int) {
	s.paceMu.Lock()
	defer s.paceMu.Unlock()
	return s.paceAt, s.paceDoneBase
}

// AwaitStart blocks until SignalStart or ctx cancel.
func (s *Server) AwaitStart(ctx context.Context) error {
	s.ensureGate()
	select {
	case <-s.startCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) ensureGate() {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.startCh == nil {
		s.startCh = make(chan struct{})
	}
}
