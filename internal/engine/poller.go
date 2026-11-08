// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// poller reads a session's references every interval until stopped.
type poller struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu    sync.Mutex
	state model.PollState
}

func (p *poller) snapshot() *model.PollState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.state
	st.Refs = slices.Clone(st.Refs)
	return &st
}

// StartPolling implements service.Service.
func (e *Engine) StartPolling(_ context.Context, id string, cfg model.PollConfig) error {
	if cfg.IntervalMS == 0 {
		cfg.IntervalMS = model.DefaultPollIntervalMS
	}
	switch {
	case cfg.IntervalMS < model.MinPollIntervalMS || cfg.IntervalMS > model.MaxPollIntervalMS:
		return service.Errorf(service.CodeInvalidArgument, "interval_ms must be between %d and %d", model.MinPollIntervalMS, model.MaxPollIntervalMS)
	case len(cfg.Refs) == 0:
		return service.Errorf(service.CodeInvalidArgument, "nothing to poll")
	case len(cfg.Refs) > model.MaxReadRefs:
		return service.Errorf(service.CodeLimitExceeded, "at most %d references", model.MaxReadRefs)
	}
	refs := make([]string, len(cfg.Refs))
	for i, r := range cfg.Refs {
		refs[i] = strings.TrimSpace(r)
	}
	s, err := e.session(id)
	if err != nil {
		return err
	}
	if st := s.info().State; st != model.SessionConnected {
		return service.Errorf(service.CodeSessionLost, "the session is %s", st)
	}
	e.stopPoller(s)

	ctx, cancel := context.WithCancel(context.Background())
	p := &poller{cancel: cancel, done: make(chan struct{}), state: model.PollState{
		Refs: refs, IntervalMS: cfg.IntervalMS, StartedAt: time.Now().UTC(),
	}}
	s.stateMu.Lock()
	s.poller = p
	s.stateMu.Unlock()
	go e.poll(ctx, s, p)
	e.publishSession(s)
	return nil
}

// StopPolling implements service.Service.
func (e *Engine) StopPolling(_ context.Context, id string) error {
	s, err := e.session(id)
	if err != nil {
		return err
	}
	if e.stopPoller(s) {
		e.publishSession(s)
	}
	return nil
}

// stopPoller stops the session's poller and waits for it. It reports
// whether one was running.
func (e *Engine) stopPoller(s *session) bool {
	s.stateMu.Lock()
	p := s.poller
	s.poller = nil
	s.stateMu.Unlock()
	if p == nil {
		return false
	}
	p.cancel()
	<-p.done
	return true
}

// poll runs the cycles on an absolute schedule, start to start. A cycle
// that ends after the next start skips the starts it missed and counts them
// as overruns; there is no catching up. The first cycle runs at once.
func (e *Engine) poll(ctx context.Context, s *session, p *poller) {
	defer close(p.done)
	interval := time.Duration(p.state.IntervalMS) * time.Millisecond
	next := time.Now()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		e.cycle(ctx, s, p)
		next = next.Add(interval)
		if late := time.Since(next); late >= 0 {
			missed := late/interval + 1
			p.mu.Lock()
			p.state.Overruns += uint64(missed)
			p.mu.Unlock()
			next = next.Add(missed * interval)
		}
		timer.Reset(time.Until(next))
	}
}

// cycle reads once and publishes the sample. A session that is no longer
// connected ends polling.
func (e *Engine) cycle(ctx context.Context, s *session, p *poller) {
	if s.info().State != model.SessionConnected {
		p.cancel()
		return
	}
	s.mu.Lock()
	res, whole := e.readLocked(ctx, s, p.state.Refs)
	s.rec.stop() // poll cycles are samples, not operations
	s.mu.Unlock()
	if ctx.Err() != nil {
		return
	}

	failed := slices.ContainsFunc(res.Values, func(v model.ReadValue) bool { return v.Error != nil })
	p.mu.Lock()
	p.state.Samples++
	if failed {
		p.state.Failures++
	}
	sample := &model.Sample{
		SessionID: s.id, Seq: p.state.Samples, At: res.At, DurationMS: res.DurationMS,
		Values: res.Values, Overruns: p.state.Overruns,
	}
	p.mu.Unlock()
	e.hub.publish(model.Event{Kind: model.EventSample, SessionID: s.id, Sample: sample})
	if whole != nil {
		e.log.Warn("poll cycle failed", "session", s.id, "error", whole.Message)
	}
}

// publishSession publishes the session's current state.
func (e *Engine) publishSession(s *session) {
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
}
