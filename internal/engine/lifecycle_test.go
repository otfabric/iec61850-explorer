// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// The lifecycle contract of the engine (TESTING.md, rows L1 to L16): closing
// sessions and the engine, during reads and opens, at the same time as other
// callers, and what is left running afterwards.

var hzRef = simied.Domain + "/MMXU1.Hz.mag.f[MX]"

// recv returns the next value of ch, or fails the test after a bound that
// only stops a test that hangs.
func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return *new(T)
	}
}

// gate forwards TCP connections to an IED. While it is held, what the IED
// sends is kept back; sent tells of every chunk the Explorer sends then.
type gate struct {
	addr string
	ln   net.Listener
	sent chan struct{}

	mu    sync.Mutex
	open  chan struct{} // closed when the gate is released; nil when not held
	conns []net.Conn
	wg    sync.WaitGroup
}

func startGate(t *testing.T, upstream string) *gate {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &gate{addr: ln.Addr().String(), ln: ln, sent: make(chan struct{}, 64)}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = c.Close()
				continue
			}
			g.mu.Lock()
			g.conns = append(g.conns, c, up)
			g.mu.Unlock()
			g.wg.Add(2)
			go g.pipe(c, up, false)
			go g.pipe(up, c, true)
		}
	}()
	t.Cleanup(func() {
		g.release()
		_ = ln.Close()
		g.mu.Lock()
		for _, c := range g.conns {
			_ = c.Close()
		}
		g.mu.Unlock()
		g.wg.Wait()
	})
	return g
}

func (g *gate) hold() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open == nil {
		g.open = make(chan struct{})
	}
}

func (g *gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open != nil {
		close(g.open)
		g.open = nil
	}
}

func (g *gate) pipe(from, to net.Conn, fromIED bool) {
	defer g.wg.Done()
	defer func() { _ = from.Close(); _ = to.Close() }()
	buf := make([]byte, 32<<10)
	for {
		n, err := from.Read(buf)
		if n > 0 {
			g.mu.Lock()
			open := g.open
			g.mu.Unlock()
			switch {
			case open != nil && fromIED:
				<-open
			case open != nil:
				select {
				case g.sent <- struct{}{}:
				default:
				}
			}
			if _, werr := to.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// L1: closing a session or the engine twice is safe; the second close of a
// session finds none, the engine refuses new sessions after Close.
func TestL01CloseTwice(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.CloseSession(ctx, info.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Fatalf("second CloseSession = %v, want not_found", err)
	}
	if _, err := e.OpenSession(ctx, target(t, ied.Addr)); err != nil {
		t.Fatal(err)
	}
	e.Close()
	e.Close()
	if list, _ := e.Sessions(ctx); len(list) != 0 {
		t.Fatalf("sessions after Close: %+v", list)
	}
	if _, err := e.OpenSession(ctx, target(t, ied.Addr)); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("OpenSession after Close = %v, want not_allowed", err)
	}
	checkGoroutines(t, "engine.(*Engine).watch", nil)
}

// L2: CloseSession from several callers at once, and racing Engine.Close:
// every call returns, the association is closed once, nothing is left.
func TestL02ConcurrentClose(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n)
	for range n {
		go func() { <-start; errs <- e.CloseSession(ctx, info.ID) }()
	}
	close(start)
	for range n {
		if err := recv(t, errs, "CloseSession"); err != nil && service.CodeOf(err) != service.CodeNotFound {
			t.Fatalf("CloseSession = %v", err)
		}
	}
	closes := 0
	for _, ev := range drain(events) {
		if ev.Kind == model.EventOperation && ev.SessionID == info.ID && ev.Operation.Action == "close" {
			closes++
		}
	}
	if closes != 1 {
		t.Errorf("%d close operations, want 1", closes)
	}

	a, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = e.CloseSession(ctx, a.ID); done <- struct{}{} }()
	go func() { e.Close(); done <- struct{}{} }()
	recv(t, done, "the first close")
	recv(t, done, "the second close")
	for _, id := range []string{a.ID, b.ID} {
		if _, err := e.lookup(id); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("session %s after Close: %v", id, err)
		}
	}
	checkGoroutines(t, "engine.(*Engine).watch", nil)
}

// L3: CloseSession during a read the IED does not answer waits for the read,
// which ends at its request timeout; then the session closes and nothing of
// it is left.
func TestL03CloseDuringBlockedRead(t *testing.T) {
	ied := startIED(t)
	g := startGate(t, ied.Addr)
	e := newEngine(t)
	ctx := t.Context()
	cfg := target(t, g.addr)
	cfg.TimeoutMS = 500
	info, err := e.OpenSession(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	g.hold()
	read := make(chan model.ReadResult, 1)
	go func() {
		res, _ := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{hzRef}})
		read <- res
	}()
	recv(t, g.sent, "the read request")
	closed := make(chan error, 1)
	go func() { closed <- e.CloseSession(ctx, info.ID) }()
	if res := recv(t, read, "the read"); res.Values[0].Error == nil {
		t.Errorf("the unanswered read returned a value: %+v", res.Values[0])
	}
	g.release()
	if err := recv(t, closed, "CloseSession"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.lookup(info.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("session after CloseSession: %v", err)
	}
	checkGoroutines(t, "engine.(*Engine).watch", nil)
}

// L5: Engine.Close while a session is being opened: Close returns, and the
// open that completes afterwards is refused and its association ended.
func TestL05CloseDuringOpen(t *testing.T) {
	ied := startIED(t)
	g := startGate(t, ied.Addr)
	e := newEngine(t)
	ctx := t.Context()
	g.hold()
	opened := make(chan error, 1)
	go func() {
		_, err := e.OpenSession(ctx, target(t, g.addr))
		opened <- err
	}()
	recv(t, g.sent, "the connect request")
	e.Close()
	g.release()
	if err := recv(t, opened, "OpenSession"); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("OpenSession during Close = %v, want not_allowed", err)
	}
	if list, _ := e.Sessions(ctx); len(list) != 0 {
		t.Fatalf("sessions: %+v", list)
	}
	checkGoroutines(t, "engine.(*Engine).watch", nil)
}

// L6: cancelling a read the IED does not answer ends it at once, long
// before its request timeout; the late answer is dropped and the session
// reads again.
func TestL06CancelPendingRead(t *testing.T) {
	ied := startIED(t)
	g := startGate(t, ied.Addr)
	e := newEngine(t)
	ctx := t.Context()
	cfg := target(t, g.addr)
	cfg.TimeoutMS = int(MaxTimeout.Milliseconds())
	info, err := e.OpenSession(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	g.hold()
	rctx, cancel := context.WithCancel(ctx)
	read := make(chan model.ReadResult, 1)
	go func() {
		res, _ := e.Read(rctx, info.ID, model.ReadRequest{Refs: []string{hzRef}})
		read <- res
	}()
	recv(t, g.sent, "the read request")
	cancel()
	if res := recv(t, read, "the cancelled read"); res.Values[0].Error == nil {
		t.Errorf("the cancelled read returned a value: %+v", res.Values[0])
	}
	g.release()
	res, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{hzRef}})
	if err != nil || res.Values[0].Value == nil {
		t.Fatalf("read after the cancelled one = %+v, %v", res, err)
	}
}

// L13: StartPolling from several callers at once leaves one poller,
// which StopPolling stops: no poller runs and no sample of the session is
// published after StopPolling returned.
func TestL13ConcurrentStartPolling(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	cfg := model.PollConfig{Refs: []string{hzRef}, IntervalMS: model.MinPollIntervalMS}
	for range 5 {
		const n = 16
		start := make(chan struct{})
		errs := make(chan error, n)
		for range n {
			go func() { <-start; errs <- e.StartPolling(ctx, info.ID, cfg) }()
		}
		close(start)
		for range n {
			if err := recv(t, errs, "StartPolling"); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.StopPolling(ctx, info.ID); err != nil {
			t.Fatal(err)
		}
		checkGoroutines(t, "engine.(*Engine).poll", nil)
	}
	e.hub.mu.Lock()
	mark := e.hub.seq
	e.hub.mu.Unlock()
	if err := e.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	events, _ := e.Subscribe(ctx, mark)
	for {
		ev := recv(t, events, "the close event")
		if ev.Kind == model.EventSample && ev.SessionID == info.ID {
			t.Fatalf("sample %d after StopPolling returned", ev.Sample.Seq)
		}
		if ev.Kind == model.EventSession && ev.SessionID == info.ID && ev.Session.State == model.SessionClosed {
			break
		}
	}
}

// L13: sessions opened at the same time stay within MaxSessions, and an
// open that fails gives its place back.
func TestL13ConcurrentOpenSession(t *testing.T) {
	ied := startIED(t)
	e := New(Options{MaxSessions: 1})
	t.Cleanup(e.Close)
	ctx := t.Context()
	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n)
	for range n {
		go func() {
			<-start
			_, err := e.OpenSession(ctx, target(t, ied.Addr))
			errs <- err
		}()
	}
	close(start)
	opened := 0
	for range n {
		switch err := recv(t, errs, "OpenSession"); service.CodeOf(err) {
		case "":
			opened++
		case service.CodeLimitExceeded:
		default:
			t.Fatalf("OpenSession = %v", err)
		}
	}
	if opened != 1 {
		t.Fatalf("%d sessions opened with MaxSessions 1", opened)
	}

	list, _ := e.Sessions(ctx)
	if err := e.CloseSession(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := ln.Addr().String()
	_ = ln.Close()
	if _, err := e.OpenSession(ctx, target(t, refused)); service.CodeOf(err) != service.CodeConnectFailed {
		t.Fatalf("open of a closed port = %v", err)
	}
	if _, err := e.OpenSession(ctx, target(t, ied.Addr)); err != nil {
		t.Fatalf("open after a failed one: %v", err)
	}
}

// The expiry and the association watch of a grant end that grant only:
// a grant made after the first was revoked is left in force.
func TestGrantEndsOnlyItself(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.session(info.ID)
	if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
		t.Fatal(err)
	}
	first := s.currentGrant()
	if _, err := e.RevokeCommission(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
		t.Fatal(err)
	}
	second := s.currentGrant()
	e.endGrantOf(context.Background(), s, first, model.AuditExpire, "the grant expired")
	e.endGrantOf(context.Background(), s, first, model.AuditEnd, "the Commission association ended")
	if got := s.currentGrant(); got != second || s.info().Mode != model.ModeCommission {
		t.Fatalf("the end of the first grant ended the second: grant now %v", got)
	}
}

// Grants and revokes at the same time: every grant that was made ends,
// once, and nothing of them is left running.
func TestConcurrentGrantRevoke(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		done := make(chan struct{}, 2)
		go func() { _, _ = e.GrantCommission(ctx, info.ID, writeGrant); done <- struct{}{} }()
		go func() { _, _ = e.RevokeCommission(ctx, info.ID); done <- struct{}{} }()
		recv(t, done, "the grant")
		recv(t, done, "the revoke")
		if _, err := e.RevokeCommission(ctx, info.ID); err != nil {
			t.Fatal(err)
		}
	}
	checkGoroutines(t, "engine.(*Engine).watchGrant", nil)
	grants, ends := 0, 0
	for _, r := range readAuditFiles(t, dir) {
		switch r.Action {
		case model.AuditGrant:
			grants++
		case model.AuditRevoke, model.AuditEnd, model.AuditExpire:
			ends++
		}
	}
	if grants == 0 || grants != ends {
		t.Fatalf("%d grants, %d ends", grants, ends)
	}
}

// Engine.Close returns after the goroutines it started for its sessions
// have ended: the association watchers, a poller and a grant's watcher are
// gone the moment it returns, without waiting.
func TestCloseWaitsForGoroutines(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	var ids []string
	for range 3 {
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, info.ID)
	}
	if err := e.StartPolling(ctx, ids[0], model.PollConfig{Refs: []string{hzRef}, IntervalMS: model.MinPollIntervalMS}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, ids[1], writeGrant); err != nil {
		t.Fatal(err)
	}
	e.Close()
	for _, frame := range []string{"engine.(*Engine).watch(", "engine.(*Engine).poll(", "engine.(*Engine).watchGrant("} {
		for _, stack := range goroutineStacks() {
			if strings.Contains(stack, frame) {
				t.Errorf("running after Close:\n%s", stack)
			}
		}
	}
}
