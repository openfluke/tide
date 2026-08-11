package dash

import (
	"context"
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
