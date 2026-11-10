// SPDX-License-Identifier: MIT

package engine

// This file and the engine's other commission*.go files are the only code
// outside tests that may hold a Commission association and call
// state-changing services (.golangci.yml, ADR 0004).

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/commission"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// grant is a Commission grant in force: the second association and what
// the engine must undo when the grant ends.
type grant struct {
	info   model.Grant
	client *iec61850.Client
	timer  *time.Timer
	// cleanup undoes, newest first, what the engine set up for itself (for
	// example an enabled report control block). It runs with s.mu held.
	cleanup []cleanupStep

	// subs are the report subscriptions, by ID; mu guards the map.
	mu   sync.Mutex
	subs map[string]*subscription
}

type cleanupStep struct {
	// id names the step for removal (a subscription ID); may be empty.
	id   string
	what string
	undo func(ctx context.Context, c *iec61850.Client) error
}

// infoCopy returns the grant's description with its subscriptions.
func (g *grant) infoCopy() model.Grant {
	out := g.info
	g.mu.Lock()
	defer g.mu.Unlock()
	out.Subscriptions = make([]model.Subscription, 0, len(g.subs))
	for _, sub := range g.subs {
		out.Subscriptions = append(out.Subscriptions, sub.snapshot())
	}
	slices.SortFunc(out.Subscriptions, func(a, b model.Subscription) int { return a.Since.Compare(b.Since) })
	return out
}

// removeCleanup takes the step with id out of the cleanup list.
func (g *grant) removeCleanup(id string) (cleanupStep, bool) {
	for i, step := range g.cleanup {
		if step.id == id {
			g.cleanup = slices.Delete(g.cleanup, i, i+1)
			return step, true
		}
	}
	return cleanupStep{}, false
}

// commissionErr says why the engine grants no Commission mode.
func (e *Engine) commissionErr() error {
	switch {
	case !e.opts.Commission:
		return service.Errorf(service.CodeNotAllowed, "this engine does not offer Commission mode (the daemon offers it when started with --allow-commission)")
	case e.audit == nil:
		return service.Errorf(service.CodeNotAllowed, "Commission mode needs a data directory for the audit trail")
	}
	return nil
}

func validateGrant(req model.GrantRequest) (model.GrantRequest, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	switch {
	case req.Reason == "":
		return req, service.Errorf(service.CodeInvalidArgument, "a grant needs a reason, for example a work order")
	case len(req.Reason) > model.MaxGrantReasonLen:
		return req, service.Errorf(service.CodeInvalidArgument, "the reason has at most %d characters", model.MaxGrantReasonLen)
	case strings.IndexFunc(req.Reason, unicode.IsControl) >= 0:
		return req, service.Errorf(service.CodeInvalidArgument, "the reason contains control characters")
	case req.Minutes < 0 || req.Minutes > model.MaxGrantMinutes:
		return req, service.Errorf(service.CodeInvalidArgument, "a grant lasts 1 to %d minutes", model.MaxGrantMinutes)
	case len(req.Capabilities) == 0:
		return req, service.Errorf(service.CodeInvalidArgument, "a grant needs at least one capability: %s", strings.Join(commission.Capabilities(), ", "))
	}
	if req.Minutes == 0 {
		req.Minutes = model.DefaultGrantMinutes
	}
	caps := []string{}
	for _, c := range req.Capabilities {
		if !commission.Grantable(c) {
			return req, service.Errorf(service.CodeInvalidArgument, "%q is not a grantable capability: %s", c, strings.Join(commission.Capabilities(), ", "))
		}
		if !slices.Contains(caps, c) {
			caps = append(caps, c)
		}
	}
	slices.Sort(caps)
	req.Capabilities = caps
	return req, nil
}

// GrantCommission implements service.Service.
func (e *Engine) GrantCommission(ctx context.Context, id string, req model.GrantRequest) (model.SessionInfo, error) {
	if err := e.commissionErr(); err != nil {
		return model.SessionInfo{}, err
	}
	req, err := validateGrant(req)
	if err != nil {
		return model.SessionInfo{}, err
	}
	s, err := e.session(id)
	if err != nil {
		return model.SessionInfo{}, err
	}
	if err := s.liveErr(); err != nil {
		return model.SessionInfo{}, err
	}
	if err := e.audit.writable(); err != nil {
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the audit trail cannot be written: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentGrant() != nil {
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the session already holds a grant; revoke it first")
	}
	s.rec.start()
	start := time.Now()
	client, err := e.dial(ctx, s, commission.Services(req.Capabilities), wireCommission)
	if err != nil {
		detail := classify(err, true)
		e.publishOperation(s.rec, s.id, "grant", net.JoinHostPort(s.target.Host, strconv.Itoa(s.target.Port)), start, detail)
		return model.SessionInfo{}, &service.Error{
			Code: service.CodeConnectFailed, Message: "the Commission association could not be opened: " + detail.Message,
			Detail: detail, Err: err,
		}
	}
	now := time.Now().UTC()
	g := &grant{client: client, subs: map[string]*subscription{}, info: model.Grant{
		Reason: req.Reason, Capabilities: req.Capabilities,
		GrantedAt: now, ExpiresAt: now.Add(time.Duration(req.Minutes) * time.Minute),
	}}
	if _, err := e.audit.append(e.auditRecord(s, model.AuditRecord{
		Action: model.AuditGrant, Outcome: model.OutcomeOK, Reason: req.Reason, Capabilities: req.Capabilities,
	})); err != nil {
		_ = client.Abort(context.Background())
		e.publishOperation(s.rec, s.id, "grant", "", start, &model.OpError{Code: model.ErrCodeOther, Message: err.Error()})
		return model.SessionInfo{}, service.Errorf(service.CodeInternal, "%v; no grant", err)
	}

	s.stateMu.Lock()
	if s.state != model.SessionConnected {
		s.stateMu.Unlock()
		_ = client.Abort(context.Background())
		e.publishOperation(s.rec, s.id, "grant", "", start, nil)
		e.auditEnd(s, g, model.AuditEnd, "the session ended while the grant was opened", nil)
		return model.SessionInfo{}, service.Errorf(service.CodeSessionLost, "the session is %s", s.state)
	}
	s.grant = g
	s.stateMu.Unlock()
	g.timer = time.AfterFunc(time.Until(g.info.ExpiresAt), func() {
		e.endGrant(context.Background(), s, model.AuditExpire, "the grant expired")
	})
	go func() {
		<-client.MMS().Done()
		e.endGrant(context.Background(), s, model.AuditEnd, "the Commission association ended")
	}()

	e.publishOperation(s.rec, s.id, "grant", strings.Join(req.Capabilities, ", "), start, nil)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	e.log.Info("commission granted", "session", s.id, "capabilities", req.Capabilities, "until", g.info.ExpiresAt)
	return info, nil
}

// RevokeCommission implements service.Service.
func (e *Engine) RevokeCommission(ctx context.Context, id string) (model.SessionInfo, error) {
	s, err := e.session(id)
	if err != nil {
		return model.SessionInfo{}, err
	}
	e.endGrant(ctx, s, model.AuditRevoke, "")
	return s.info(), nil
}

func (s *session) currentGrant() *grant {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.grant
}

// endGrant ends the session's grant, if any: it runs the cleanup steps,
// concludes the Commission association and records the end. It is safe to
// call more than once and from any goroutine; only the first call acts.
func (e *Engine) endGrant(ctx context.Context, s *session, action, why string) {
	s.stateMu.Lock()
	g := s.grant
	s.grant = nil
	s.stateMu.Unlock()
	if g == nil {
		return
	}
	if g.timer != nil {
		g.timer.Stop()
	}

	s.mu.Lock()
	s.rec.start()
	start := time.Now()
	var failed []string
	for i := len(g.cleanup) - 1; i >= 0; i-- {
		step := g.cleanup[i]
		cctx, cancel := s.timeoutCtx(ctx)
		err := step.undo(cctx, g.client)
		cancel()
		rec := model.AuditRecord{Action: model.AuditCleanup, Outcome: model.OutcomeOK, Ref: step.what}
		if err != nil {
			rec.Outcome, rec.Error = model.OutcomeFailed, err.Error()
			failed = append(failed, step.what)
		}
		e.appendAudit(s, rec)
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := g.client.Close(cctx); err != nil {
		_ = g.client.Abort(context.Background())
	}
	cancel()
	e.publishOperation(s.rec, s.id, "revoke", action, start, nil)
	s.mu.Unlock()

	e.auditEnd(s, g, action, why, failed)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	e.log.Info("commission ended", "session", s.id, "action", action, "reason", why)
}

func (e *Engine) auditEnd(s *session, g *grant, action, why string, failed []string) {
	rec := model.AuditRecord{Action: action, Outcome: model.OutcomeOK, Reason: why, Capabilities: g.info.Capabilities}
	if len(failed) > 0 {
		rec.Outcome = model.OutcomeFailed
		rec.Error = "not undone: " + strings.Join(failed, ", ")
	}
	e.appendAudit(s, rec)
}

// appendAudit records an event the engine cannot refuse any more (the end
// of a grant, a write the IED already received). A trail that cannot be
// written is logged.
func (e *Engine) appendAudit(s *session, rec model.AuditRecord) {
	if _, err := e.audit.append(e.auditRecord(s, rec)); err != nil {
		e.log.Error("audit record lost", "session", s.id, "action", rec.Action, "ref", rec.Ref, "error", err)
	}
}

func (e *Engine) auditRecord(s *session, rec model.AuditRecord) model.AuditRecord {
	rec.SessionID = s.id
	rec.Target = net.JoinHostPort(s.target.Host, strconv.Itoa(s.target.Port))
	if s.identity != nil {
		rec.IED = strings.TrimSpace(s.identity.Vendor + " " + s.identity.Model)
	}
	return rec
}

// commissionAction runs fn on a connected session holding a grant with
// capability c, as one operation: s.mu held, the timeout applied, the
// requests recorded and published.
func (e *Engine) commissionAction(ctx context.Context, id, c, action, target string, fn func(ctx context.Context, s *session, g *grant) error) error {
	s, err := e.session(id)
	if err != nil {
		return err
	}
	if err := s.liveErr(); err != nil {
		return err
	}
	if _, err := s.requireGrant(c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.requireGrant(c) // the grant may have ended meanwhile
	if err != nil {
		return err
	}
	s.rec.start()
	start := time.Now()
	tctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	err = fn(tctx, s, g)
	var opErr *model.OpError
	if err != nil {
		if se, ok := errors.AsType[*service.Error](err); ok && se.Detail != nil {
			opErr = se.Detail
		} else {
			opErr = classify(err, false)
		}
	}
	e.publishOperation(s.rec, s.id, action, target, start, opErr)
	return err
}

// requireGrant returns the grant when it carries capability c.
func (s *session) requireGrant(c string) (*grant, error) {
	g := s.currentGrant()
	if g == nil || !slices.Contains(g.info.Capabilities, c) {
		return nil, service.Errorf(service.CodeNotAllowed, "this needs Commission mode with the %s capability", c)
	}
	return g, nil
}

// Write implements service.Service.
func (e *Engine) Write(ctx context.Context, id string, req model.WriteRequest) (model.WriteResult, error) {
	s, err := e.session(id)
	if err != nil {
		return model.WriteResult{}, err
	}
	if err := s.liveErr(); err != nil {
		return model.WriteResult{}, err
	}
	ref, err := iec61850.ParseRef(strings.TrimSpace(req.Ref))
	switch {
	case err != nil:
		return model.WriteResult{}, service.Errorf(service.CodeInvalidArgument, "%q is not a reference: %v", req.Ref, err)
	case !ref.IsObject() || ref.LN == "" || len(ref.Path) < 2:
		return model.WriteResult{}, service.Errorf(service.CodeInvalidArgument, "a write needs a data attribute, such as LD/LN.DO.da[SP]")
	case !slices.Contains(model.WritableFCs, string(ref.FC)):
		return model.WriteResult{}, service.Errorf(service.CodeInvalidArgument, "functional constraint %q is not written; use one of %s", ref.FC, strings.Join(model.WritableFCs, ", "))
	case isControlBlock(ref):
		return model.WriteResult{}, service.Errorf(service.CodeInvalidArgument, "%s belongs to a control block; it is changed with its own capability, not written", ref.String())
	}
	if _, err := s.requireGrant(model.CapWrite); err != nil {
		return model.WriteResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.requireGrant(model.CapWrite) // the grant may have ended meanwhile
	if err != nil {
		return model.WriteResult{}, err
	}
	s.rec.start()
	start := time.Now()
	res := model.WriteResult{Ref: ref.String(), At: start.UTC()}
	if dom, item, err := ref.ToMMS(); err == nil {
		res.MMS = model.MMSName{Domain: string(dom), ItemID: string(item)}
	}
	finish := func(opErr *model.OpError) (model.WriteResult, error) {
		res.DurationMS = since(start)
		e.publishOperation(s.rec, s.id, "write", res.Ref, start, opErr)
		return res, nil
	}

	tctx, cancel := s.timeoutCtx(ctx)
	ts := e.typeOf(tctx, s, ref)
	cancel()
	value, err := parseValue(req.Value, ts)
	if err != nil {
		e.publishOperation(s.rec, s.id, "write", res.Ref, start, &model.OpError{Code: model.ErrCodeInvalidRef, Message: err.Error()})
		return model.WriteResult{}, service.Errorf(service.CodeInvalidArgument, "%s: %v", res.Ref, err)
	}
	name := ref.Path[len(ref.Path)-1]
	written := convertValue(value, ts, name)
	res.Written = &written

	var before model.ReadValue
	e.readSingle(ctx, s, ref, &before)
	res.Before = before.Value

	tctx, cancel = s.timeoutCtx(ctx)
	err = g.client.Write(tctx, ref, value)
	cancel()
	rec := model.AuditRecord{Action: model.AuditWrite, Ref: res.Ref, Written: written.Text, Before: textOf(res.Before)}
	if err != nil {
		res.Error = classify(err, false)
		rec.Outcome, rec.Error = model.OutcomeFailed, res.Error.Message
		e.appendAudit(s, rec)
		return finish(res.Error)
	}

	var after model.ReadValue
	e.readSingle(ctx, s, ref, &after)
	res.After = after.Value
	res.Verified = after.Value != nil && sameValue(written, *after.Value)
	rec.After, rec.Outcome = textOf(res.After), model.OutcomeOK
	if !res.Verified {
		rec.Outcome = model.OutcomeMismatch
		if after.Error != nil {
			rec.Error = "readback: " + after.Error.Message
		}
	}
	e.appendAudit(s, rec)
	return finish(nil)
}

// isControlBlock reports a reference into a control block that a writable
// functional constraint reaches: the setting group control block (LLN0.SGCB,
// FC SP). Changing it selects setting groups, which needs the
// setting-groups capability. Report, log, GOOSE and sampled value control
// blocks have functional constraints that are not written (BR, RP, LG, GO,
// MS, US).
func isControlBlock(ref iec61850.Ref) bool {
	return ref.LN == "LLN0" && len(ref.Path) > 0 && ref.Path[0] == "SGCB"
}

func textOf(v *model.Value) string {
	if v == nil {
		return ""
	}
	return v.Text
}

// sameValue compares a written value with the value read back. A UTC time
// is equal within its resolution (2^-24 s, below a microsecond).
func sameValue(written, read model.Value) bool {
	if written.Type != read.Type {
		return false
	}
	if written.Time != nil && read.Time != nil {
		d := written.Time.Time.Sub(read.Time.Time)
		return d < time.Microsecond && d > -time.Microsecond
	}
	return written.Text == read.Text
}
