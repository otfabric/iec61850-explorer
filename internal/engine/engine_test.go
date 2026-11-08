// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/readonly"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func startIED(t *testing.T) *simied.IED {
	t.Helper()
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	return ied
}

func target(t *testing.T, addr string) model.TargetConfig {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return model.TargetConfig{Host: host, Port: p, TimeoutMS: 5000}
}

func newEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(Options{})
	t.Cleanup(e.Close)
	return e
}

// waitEvent returns the first event that satisfies match.
func waitEvent(t *testing.T, ch <-chan model.Event, match func(model.Event) bool) model.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("event stream closed")
			}
			if match(ev) {
				return ev
			}
		case <-timeout:
			t.Fatal("event not seen")
		}
	}
}

func TestOpenBrowseRead(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)

	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if info.State != model.SessionConnected || info.Mode != model.ModeObserve {
		t.Errorf("info = %+v", info)
	}
	if info.Identity == nil || info.Identity.Vendor != "OT Fabric" {
		t.Errorf("identity = %+v", info.Identity)
	}
	if d := info.Discovery; d.LogicalDevices != 1 || d.LogicalNodes != 5 || d.DataObjects == 0 {
		t.Errorf("discovery = %+v", d)
	}
	connect := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "connect"
	})
	if !connect.Operation.OK || !slices.ContainsFunc(connect.Operation.Requests, func(r model.MMSRequest) bool {
		return r.Service == "Identify"
	}) {
		t.Errorf("connect operation = %+v", connect.Operation)
	}

	lds, err := e.Browse(ctx, info.ID, "")
	if err != nil || len(lds) != 1 || lds[0].Ref != simied.Domain || lds[0].Kind != model.NodeLD {
		t.Fatalf("Browse root = %+v, %v", lds, err)
	}
	lns, err := e.Browse(ctx, info.ID, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	classes := map[string]string{}
	for _, n := range lns {
		classes[n.Name] = n.LNClass
	}
	if classes["MMXU1"] != "MMXU" || classes["LLN0"] != "LLN0" || classes["XCBR1"] != "XCBR" {
		t.Errorf("logical nodes = %+v", lns)
	}
	dos, err := e.Browse(ctx, info.ID, simied.Domain+"/MMXU1")
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(dos, func(n model.Node) bool { return n.Name == "Hz" })
	if i < 0 || dos[i].Kind != model.NodeDO || !slices.Contains(dos[i].FCs, "MX") || dos[i].Children == 0 {
		t.Fatalf("data objects = %+v", dos)
	}
	if _, err := e.Browse(ctx, info.ID, simied.Domain+"/NOPE1"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("Browse unknown = %v", err)
	}
	if _, err := e.Browse(ctx, info.ID, simied.Domain+"/MMXU1.Hz[MX]"); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("Browse with FC = %v", err)
	}

	res, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{
		simied.Domain + "/MMXU1.Hz[MX]",
		simied.Domain + "/MMXU1.TotW.q[MX]",
		simied.Domain + "/XCBR1.Pos.stVal[ST]",
		simied.Domain + "/MMXU1.Hz",
		simied.Domain + "/MMXU1.Nope[MX]",
	}})
	if err != nil {
		t.Fatal(err)
	}
	hz := res.Values[0]
	if hz.Error != nil || hz.Value == nil || hz.Value.Type != model.TypeStructure {
		t.Fatalf("Hz = %+v", hz)
	}
	if hz.MMS.Domain != simied.Domain || hz.MMS.ItemID != "MMXU1$MX$Hz" {
		t.Errorf("Hz MMS name = %+v", hz.MMS)
	}
	byName := map[string]model.Value{}
	for _, c := range hz.Value.Children {
		byName[c.Name] = c
	}
	mag := byName["mag"]
	if len(mag.Children) != 1 || mag.Children[0].Name != "f" || mag.Children[0].Text != "50.002" {
		t.Errorf("Hz.mag = %+v", mag)
	}
	if q := byName["q"].Quality; q == nil || q.Validity != "good" || len(q.Flags) != 0 {
		t.Errorf("Hz.q = %+v", byName["q"])
	}
	if ts := byName["t"].Time; ts == nil || ts.Time.Year() != 2026 {
		t.Errorf("Hz.t = %+v", byName["t"])
	}
	if q := res.Values[1].Value; q == nil || q.Quality == nil || q.Quality.Validity != "questionable" ||
		!slices.Equal(q.Quality.Flags, []string{"old-data"}) {
		t.Errorf("TotW.q = %+v", res.Values[1])
	}
	if pos := res.Values[2].Value; pos == nil || pos.Type != model.TypeBitString || pos.Text != "10" || pos.Bits != 2 {
		t.Errorf("Pos.stVal = %+v", res.Values[2])
	}
	if e := res.Values[3].Error; e == nil || e.Code != model.ErrCodeInvalidRef {
		t.Errorf("no FC = %+v", res.Values[3])
	}
	if e := res.Values[4].Error; e == nil || e.Stage != stageResponse {
		t.Errorf("unknown object = %+v", res.Values[4])
	}

	read := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "read" && ev.Operation.OK
	})
	if !slices.ContainsFunc(read.Operation.Requests, func(r model.MMSRequest) bool { return r.Service == "Read" }) {
		t.Errorf("read operation = %+v", read.Operation)
	}

	if err := e.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.Sessions(ctx); len(list) != 0 {
		t.Errorf("sessions after close = %+v", list)
	}
	if _, err := e.Browse(ctx, info.ID, ""); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("Browse after close = %v", err)
	}
}

// TestWireReadOnly: everything the engine does reaches the IED as read
// services only (the server counts what it received).
func TestWireReadOnly(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", simied.Domain, simied.Domain + "/XCBR1", simied.Domain + "/XCBR1.Pos"} {
		if _, err := e.Browse(ctx, info.ID, ref); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{
		simied.Domain + "/XCBR1.Pos[ST]", simied.Domain + "/XCBR1.Pos.ctlModel[CF]", simied.Domain + "/LLN0.NamPlt[DC]",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := e.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	got := ied.Services()
	if got[mms.ServiceRead] == 0 || got[mms.ServiceGetNameList] == 0 {
		t.Errorf("services = %v", got)
	}
	for s, n := range got {
		if !readonly.Allowed(s) {
			t.Errorf("the IED received %d %s requests", n, s)
		}
	}
}

// TestGuardRefusesWrite: the client the engine builds refuses a write
// itself; nothing reaches the IED.
func TestGuardRefusesWrite(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.session(info.ID)
	_, err = s.client.MMS().WriteObject(ctx, mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: simied.Domain, ItemID: "GGIO1$ST$Ind1$stVal"}, mms.NewBoolean(false))
	if !errors.Is(err, mms.ErrServiceNotAllowed) {
		t.Fatalf("WriteObject = %v", err)
	}
	if op := classify(err, false); op.Code != model.ErrCodeNotAllowed {
		t.Errorf("classify = %+v", op)
	}
	if n := ied.Services()[mms.ServiceWrite]; n != 0 {
		t.Errorf("the IED received %d writes", n)
	}
}

func TestConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	e := newEngine(t)
	_, err = e.OpenSession(t.Context(), target(t, addr))
	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != service.CodeConnectFailed || se.Detail == nil || se.Detail.Stage != stageTCP || se.Detail.Hint == "" {
		t.Fatalf("OpenSession = %#v", err)
	}
}

// TestConnectNotMMS: something accepts TCP but does not speak ISO-on-TCP.
func TestConnectNotMMS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	e := newEngine(t)
	_, err = e.OpenSession(t.Context(), target(t, ln.Addr().String()))
	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != service.CodeConnectFailed || se.Detail == nil || se.Detail.Stage == stageTCP {
		t.Fatalf("OpenSession = %#v (detail %+v)", err, se.Detail)
	}
}

func TestSessionLost(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	ied.Close()
	ev := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventSession && ev.Session.State == model.SessionLost
	})
	if ev.Session.ID != info.ID || ev.Session.Error == nil {
		t.Errorf("lost event = %+v", ev.Session)
	}
	if _, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); service.CodeOf(err) != service.CodeSessionLost {
		t.Errorf("Read on lost session = %v", err)
	}
	// The tree stays browsable; closing forgets the session.
	if _, err := e.Browse(ctx, info.ID, simied.Domain); err != nil {
		t.Errorf("Browse lost session = %v", err)
	}
	if err := e.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	e := New(Options{MaxSessions: 1, AllowTarget: func(_ context.Context, host string) error {
		if host == "10.9.9.9" {
			return errors.New("outside the allowed targets")
		}
		return nil
	}})
	defer e.Close()
	ctx := t.Context()
	for _, tc := range []struct {
		cfg  model.TargetConfig
		code string
	}{
		{model.TargetConfig{}, service.CodeInvalidArgument},
		{model.TargetConfig{Host: "a/b"}, service.CodeInvalidArgument},
		{model.TargetConfig{Host: "h", Port: 70000}, service.CodeInvalidArgument},
		{model.TargetConfig{Host: "h", TimeoutMS: 5}, service.CodeInvalidArgument},
		{model.TargetConfig{Host: "10.9.9.9"}, service.CodeNotAllowed},
	} {
		if _, err := e.OpenSession(ctx, tc.cfg); service.CodeOf(err) != tc.code {
			t.Errorf("OpenSession(%+v) = %v, want %s", tc.cfg, err, tc.code)
		}
	}
	if _, err := e.Read(ctx, "x", model.ReadRequest{}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("empty read = %v", err)
	}
	if _, err := e.Read(ctx, "x", model.ReadRequest{Refs: make([]string, model.MaxReadRefs+1)}); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("large read = %v", err)
	}
	if err := e.CloseSession(ctx, "x"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("close unknown = %v", err)
	}

	ied := startIED(t)
	if _, err := e.OpenSession(ctx, target(t, ied.Addr)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.OpenSession(ctx, target(t, ied.Addr)); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("second session = %v", err)
	}
	h, _ := e.Health(ctx)
	if h.Mode != model.ModeObserve || h.APIVersion != model.APIVersion {
		t.Errorf("health = %+v", h)
	}
}

func TestLNClass(t *testing.T) {
	for in, want := range map[string]string{
		"MMXU1": "MMXU", "Q0MMXU12": "MMXU", "LLN0": "LLN0", "XCBR": "XCBR", "ab1": "", "lowr1": "",
	} {
		if got := lnClass(in); got != want {
			t.Errorf("lnClass(%q) = %q, want %q", in, got, want)
		}
	}
}
