// SPDX-License-Identifier: MIT

// Package engine is the one Go implementation of service.Service: it owns the
// sessions, the MMS associations and the event stream. The desktop GUI runs
// it in-process; the daemon serves it over HTTP.
//
// Every MMS client is created with the read-only allow-list of
// internal/readonly, so the engine cannot send a state-changing service even
// by mistake.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/readonly"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// Defaults and limits.
const (
	DefaultMaxSessions = 8
	DefaultTimeout     = 10 * time.Second
	MaxTimeout         = 120 * time.Second
	// DefaultEventBuffer is the number of events kept for replay.
	DefaultEventBuffer = 1024
	// maxLostSessions bounds the lost sessions kept for the user to see.
	maxLostSessions = 16
)

// Options configures an engine.
type Options struct {
	Logger *slog.Logger
	// MaxSessions bounds the open sessions. Zero selects DefaultMaxSessions.
	MaxSessions int
	// AllowTarget, when set, is asked before connecting; an error refuses the
	// target. The daemon restricts targets with it; the desktop GUI does not.
	AllowTarget func(ctx context.Context, host string) error
	// EventBuffer overrides DefaultEventBuffer.
	EventBuffer int
}

// Engine implements service.Service.
type Engine struct {
	opts    Options
	log     *slog.Logger
	started time.Time
	hub     *hub

	mu       sync.Mutex
	sessions map[string]*session
	order    []string // session IDs, oldest first
	closed   bool
}

var _ service.Service = (*Engine)(nil)

// New returns an engine.
func New(opts Options) *Engine {
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = DefaultMaxSessions
	}
	if opts.EventBuffer <= 0 {
		opts.EventBuffer = DefaultEventBuffer
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Engine{
		opts: opts, log: log, started: time.Now(), hub: newHub(opts.EventBuffer),
		sessions: make(map[string]*session),
	}
}

// Close closes every session. The engine refuses new sessions afterwards.
func (e *Engine) Close() {
	e.mu.Lock()
	e.closed = true
	sessions := make([]*session, 0, len(e.sessions))
	for _, s := range e.sessions {
		sessions = append(sessions, s)
	}
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, s := range sessions {
		_ = e.closeSession(ctx, s)
	}
}

// Health implements service.Service.
func (e *Engine) Health(context.Context) (model.Health, error) {
	return model.Health{
		Status: "ok", Version: buildinfo.Version, APIVersion: model.APIVersion,
		UptimeS:    int64(time.Since(e.started).Seconds()),
		GoIEC61850: buildinfo.ModuleVersion(buildinfo.GoIEC61850Path),
		GoMMS:      buildinfo.ModuleVersion(buildinfo.GoMMSPath),
		Mode:       model.ModeObserve, Time: time.Now().UTC(), Features: []string{},
	}, nil
}

// validateTarget fills defaults and checks the target.
func validateTarget(cfg model.TargetConfig) (model.TargetConfig, time.Duration, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" {
		return cfg, 0, service.Errorf(service.CodeInvalidArgument, "the host is empty")
	}
	if strings.ContainsAny(cfg.Host, "/@?# ") || len(cfg.Host) > 253 {
		return cfg, 0, service.Errorf(service.CodeInvalidArgument, "%q is not a host name or address", cfg.Host)
	}
	if cfg.Port == 0 {
		cfg.Port = model.DefaultPort
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, 0, service.Errorf(service.CodeInvalidArgument, "port %d is out of range", cfg.Port)
	}
	timeout := DefaultTimeout
	if cfg.TimeoutMS != 0 {
		timeout = time.Duration(cfg.TimeoutMS) * time.Millisecond
		if timeout < 100*time.Millisecond || timeout > MaxTimeout {
			return cfg, 0, service.Errorf(service.CodeInvalidArgument, "timeout_ms must be between 100 and %d", MaxTimeout.Milliseconds())
		}
	}
	return cfg, timeout, nil
}

// OpenSession implements service.Service.
func (e *Engine) OpenSession(ctx context.Context, cfg model.TargetConfig) (model.SessionInfo, error) {
	cfg, timeout, err := validateTarget(cfg)
	if err != nil {
		return model.SessionInfo{}, err
	}
	if e.opts.AllowTarget != nil {
		if err := e.opts.AllowTarget(ctx, cfg.Host); err != nil {
			return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "%v", err)
		}
	}
	e.mu.Lock()
	switch {
	case e.closed:
		e.mu.Unlock()
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the engine is shutting down")
	case e.openCount() >= e.opts.MaxSessions:
		e.mu.Unlock()
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "at most %d sessions may be open", e.opts.MaxSessions)
	}
	e.mu.Unlock()

	s := &session{
		id: newID(), target: cfg, timeout: timeout, openedAt: time.Now().UTC(),
		rec: &recorder{}, types: make(map[string]*mms.TypeSpec), state: model.SessionConnected,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	// Connect.
	s.rec.start()
	start := time.Now()
	dctx, cancel := context.WithTimeout(ctx, timeout)
	client, err := iec61850.Dial(dctx, addr, iec61850.DialOptions{
		Logger: e.log.With("session", s.id),
		Cache:  iec61850.CacheLazy,
		MMS: mms.DialOptions{
			AllowedServices: readonly.AllowedServices(),
			RequestTimeout:  timeout,
			Metrics:         s.rec,
		},
	})
	cancel()
	if err != nil {
		detail := classify(err, true)
		e.publishOperation(s.rec, s.id, "connect", addr, start, detail)
		return model.SessionInfo{}, &service.Error{
			Code: service.CodeConnectFailed, Message: fmt.Sprintf("cannot connect to %s: %s", addr, detail.Message),
			Detail: detail, Err: err,
		}
	}
	s.client = client
	if id, err := e.identify(ctx, s); err == nil {
		s.identity = id
	}
	e.publishOperation(s.rec, s.id, "connect", addr, start, nil)

	// Discover.
	s.rec.start()
	start = time.Now()
	tctx, cancel := s.timeoutCtx(ctx)
	tree, err := client.TreeWithOptions(tctx, iec61850.TreeOptions{IncludeFCs: true})
	cancel()
	if err != nil {
		detail := classify(err, false)
		e.publishOperation(s.rec, s.id, "discover", addr, start, detail)
		_ = client.Abort(context.Background())
		return model.SessionInfo{}, &service.Error{
			Code: service.CodeDiscoveryFailed, Message: "the model could not be listed: " + detail.Message,
			Detail: detail, Err: err,
		}
	}
	s.setTree(tree, time.Since(start))
	e.publishOperation(s.rec, s.id, "discover", addr, start, nil)

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		_ = client.Abort(context.Background())
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the engine is shutting down")
	}
	e.sessions[s.id] = s
	e.order = append(e.order, s.id)
	e.mu.Unlock()

	go e.watch(s)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	e.log.Info("session opened", "session", s.id, "addr", addr,
		"lds", s.summary.LogicalDevices, "lns", s.summary.LogicalNodes)
	return info, nil
}

// identify asks for the IED's identity. An IED without Identify is normal;
// the request still appears in the connect operation.
func (e *Engine) identify(ctx context.Context, s *session) (*model.Identity, error) {
	tctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	id, err := s.client.MMS().Identify(tctx)
	if err != nil {
		return nil, err
	}
	return &model.Identity{Vendor: id.Vendor, Model: id.Model, Revision: id.Revision}, nil
}

// watch marks a session lost when its association ends without the user
// closing it.
func (e *Engine) watch(s *session) {
	m := s.client.MMS()
	<-m.Done()
	s.stateMu.Lock()
	if s.closing {
		s.stateMu.Unlock()
		return
	}
	s.state = model.SessionLost
	s.lostErr = classify(m.Err(), false)
	s.stateMu.Unlock()
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	e.log.Warn("session lost", "session", s.id, "error", m.Err())
	e.pruneLost()
}

// pruneLost forgets the oldest lost sessions beyond maxLostSessions.
func (e *Engine) pruneLost() {
	e.mu.Lock()
	defer e.mu.Unlock()
	lost := 0
	for i := len(e.order) - 1; i >= 0; i-- {
		id := e.order[i]
		if e.sessions[id].info().State != model.SessionLost {
			continue
		}
		if lost++; lost > maxLostSessions {
			delete(e.sessions, id)
			e.order = slices.Delete(e.order, i, i+1)
		}
	}
}

// openCount counts the connected sessions. e.mu must be held.
func (e *Engine) openCount() int {
	n := 0
	for _, s := range e.sessions {
		if s.info().State == model.SessionConnected {
			n++
		}
	}
	return n
}

func (e *Engine) session(id string) (*session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sessions[id]
	if !ok {
		return nil, service.Errorf(service.CodeNotFound, "no session %q", id)
	}
	return s, nil
}

// CloseSession implements service.Service. Closing a lost session forgets it.
func (e *Engine) CloseSession(ctx context.Context, id string) error {
	s, err := e.session(id)
	if err != nil {
		return err
	}
	return e.closeSession(ctx, s)
}

func (e *Engine) closeSession(ctx context.Context, s *session) error {
	s.stateMu.Lock()
	wasConnected := s.state == model.SessionConnected
	s.closing, s.state = true, model.SessionClosed
	s.stateMu.Unlock()

	if wasConnected {
		s.mu.Lock()
		s.rec.start()
		start := time.Now()
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.client.Close(cctx)
		cancel()
		if err != nil {
			_ = s.client.Abort(context.Background())
		}
		e.publishOperation(s.rec, s.id, "close", "", start, classify(err, false))
		s.mu.Unlock()
	}

	e.mu.Lock()
	delete(e.sessions, s.id)
	if i := slices.Index(e.order, s.id); i >= 0 {
		e.order = slices.Delete(e.order, i, i+1)
	}
	e.mu.Unlock()
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	return nil
}

// Sessions implements service.Service.
func (e *Engine) Sessions(context.Context) ([]model.SessionInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.SessionInfo, 0, len(e.order))
	for _, id := range e.order {
		out = append(out, e.sessions[id].info())
	}
	return out, nil
}

// Browse implements service.Service.
func (e *Engine) Browse(_ context.Context, id, ref string) ([]model.Node, error) {
	s, err := e.session(id)
	if err != nil {
		return nil, err
	}
	key := ""
	if ref = strings.TrimSpace(ref); ref != "" {
		r, err := iec61850.ParseRef(ref)
		if err != nil {
			return nil, service.Errorf(service.CodeInvalidArgument, "%q is not an object reference", ref)
		}
		if r.FC != "" {
			return nil, service.Errorf(service.CodeInvalidArgument, "browse takes a reference without functional constraint")
		}
		key = refKey(r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.index[key]
	if !ok {
		return nil, service.Errorf(service.CodeNotFound, "%q is not in the discovered model", ref)
	}
	out := make([]model.Node, len(n.Children))
	for i, c := range n.Children {
		out[i] = nodeOf(c)
	}
	return out, nil
}

// Read implements service.Service.
func (e *Engine) Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error) {
	switch {
	case len(req.Refs) == 0:
		return model.ReadResult{}, service.Errorf(service.CodeInvalidArgument, "no reference to read")
	case len(req.Refs) > model.MaxReadRefs:
		return model.ReadResult{}, service.Errorf(service.CodeLimitExceeded, "at most %d references per read", model.MaxReadRefs)
	}
	s, err := e.session(id)
	if err != nil {
		return model.ReadResult{}, err
	}
	if st := s.info().State; st != model.SessionConnected {
		return model.ReadResult{}, service.Errorf(service.CodeSessionLost, "the session is %s", st)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	res := model.ReadResult{At: time.Now().UTC(), Values: make([]model.ReadValue, len(req.Refs))}
	start := time.Now()
	for i, raw := range req.Refs {
		res.Values[i] = e.readOne(ctx, s, raw)
	}
	res.DurationMS = since(start)
	return res, nil
}

// readOne reads one reference as its own operation.
func (e *Engine) readOne(ctx context.Context, s *session, raw string) model.ReadValue {
	out := model.ReadValue{Ref: raw}
	ref, err := iec61850.ParseRef(strings.TrimSpace(raw))
	if err == nil && ref.FC == "" {
		err = &iec61850.ReferenceError{Input: raw, Reason: "functional constraint required, such as [MX]"}
	}
	if err != nil {
		out.Error = classify(err, false)
		out.Error.Code = model.ErrCodeInvalidRef
		return out
	}
	out.Ref = ref.String()
	if dom, item, err := ref.ToMMS(); err == nil {
		out.MMS = model.MMSName{Domain: string(dom), ItemID: string(item)}
	}

	s.rec.start()
	start := time.Now()
	tctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	v, err := s.client.ReadRaw(tctx, ref)
	if err != nil {
		out.Error = classify(err, false)
		e.publishOperation(s.rec, s.id, "read", out.Ref, start, out.Error)
		return out
	}
	ts := e.typeOf(tctx, s, ref)
	name := ""
	if len(ref.Path) > 0 {
		name = ref.Path[len(ref.Path)-1]
	}
	val := convertValue(v, ts, name)
	out.Value = &val
	e.publishOperation(s.rec, s.id, "read", out.Ref, start, nil)
	return out
}

// typeOf returns the MMS type of ref, from the cache or the IED. A failure
// leaves components unnamed; it does not fail the read. s.mu must be held.
func (e *Engine) typeOf(ctx context.Context, s *session, ref iec61850.Ref) *mms.TypeSpec {
	key := ref.String()
	if ts, ok := s.types[key]; ok {
		return ts
	}
	if !ref.IsObject() {
		return nil
	}
	ts, err := s.client.GetVariableType(ctx, ref)
	if err != nil {
		e.log.Debug("no type", "ref", key, "error", err)
		ts = nil
	}
	s.types[key] = ts
	return ts
}

// Subscribe implements service.Service.
func (e *Engine) Subscribe(ctx context.Context, sinceSeq uint64) (<-chan model.Event, error) {
	return e.hub.subscribe(ctx, sinceSeq), nil
}

// publishOperation ends the recorder's action and publishes it.
func (e *Engine) publishOperation(rec *recorder, sessionID, action, target string, start time.Time, opErr *model.OpError) {
	op := model.Operation{
		Action: action, Target: target, StartedAt: start.UTC(), DurationMS: since(start),
		OK: opErr == nil, Error: opErr,
	}
	op.Requests, op.RequestsDropped = rec.stop()
	e.hub.publish(model.Event{Kind: model.EventOperation, SessionID: sessionID, Operation: &op})
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(errors.New("crypto/rand failed"))
	}
	return hex.EncodeToString(b[:])
}
