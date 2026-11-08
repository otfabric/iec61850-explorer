// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

var ctx = context.Background()

// next returns the first event that matches, failing after a timeout.
func next(t testing.TB, ch <-chan model.Event, what string, match func(model.Event) bool) model.Event {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event stream closed while waiting for %s", what)
			}
			if match(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func targetOf(t testing.TB, addr string) model.TargetConfig {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return model.TargetConfig{Host: host, Port: p, TimeoutMS: 5000}
}

// TestContract: the same contract holds for the engine and for the engine
// behind the HTTP API (the M0 exit criterion: one service, identical
// behaviour in the desktop and the daemon form).
func TestContract(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{})
		t.Cleanup(eng.Close)
		contract(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		contract(t, newDaemon(t, engine.Options{}, api.Options{}).svc)
	})
}

func contract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)

	h, err := svc.Health(ctx)
	if err != nil || h.Status != "ok" || h.APIVersion != model.APIVersion || h.Mode != model.ModeObserve {
		t.Fatalf("Health = %+v, %v", h, err)
	}

	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := svc.Subscribe(sub, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Errors keep their code, and a failed connect its classified detail.
	closed := targetOf(t, ied.Addr)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed.Port = ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	_, err = svc.OpenSession(ctx, closed)
	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != service.CodeConnectFailed || se.Detail == nil || se.Detail.Stage != "tcp" {
		t.Fatalf("open on a closed port = %#v", err)
	}
	failed := next(t, events, "failed connect operation", func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "connect"
	})
	if failed.Operation.OK || failed.Operation.Error == nil || failed.Operation.Error.Stage != "tcp" {
		t.Errorf("failed connect = %+v", failed.Operation)
	}
	if _, err = svc.OpenSession(ctx, model.TargetConfig{Host: ""}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("open without host = %v", err)
	}
	for name, call := range map[string]func() error{
		"Browse":       func() error { _, e := svc.Browse(ctx, "nope", ""); return e },
		"Read":         func() error { _, e := svc.Read(ctx, "nope", model.ReadRequest{Refs: []string{"a/b.c[MX]"}}); return e },
		"CloseSession": func() error { return svc.CloseSession(ctx, "nope") },
	} {
		if err := call(); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("%s on an unknown session = %v, want not_found", name, err)
		}
	}

	// Open and discover.
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if info.State != model.SessionConnected || info.Identity == nil || info.Discovery.LogicalNodes != 5 {
		t.Fatalf("session = %+v", info)
	}
	opened := next(t, events, "session event", func(ev model.Event) bool { return ev.Kind == model.EventSession })
	if opened.SessionID != info.ID || opened.Session == nil || opened.Seq == 0 {
		t.Errorf("event = %+v", opened)
	}
	if list, err := svc.Sessions(ctx); err != nil || len(list) != 1 || list[0].ID != info.ID {
		t.Errorf("Sessions = %+v, %v", list, err)
	}

	// Browse.
	lns, err := svc.Browse(ctx, info.ID, simied.Domain)
	if err != nil || !slices.ContainsFunc(lns, func(n model.Node) bool { return n.Name == "XCBR1" && n.LNClass == "XCBR" }) {
		t.Fatalf("Browse LD = %+v, %v", lns, err)
	}
	if _, err := svc.Browse(ctx, info.ID, "not a ref["); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("Browse bad ref = %v", err)
	}

	// Read.
	res, err := svc.Read(ctx, info.ID, model.ReadRequest{Refs: []string{
		simied.Domain + "/MMXU1.Hz.mag.f[MX]", simied.Domain + "/GGIO1.Ind1.stVal[ST]", simied.Domain + "/XCBR1.Nope[ST]",
	}})
	if err != nil || len(res.Values) != 3 {
		t.Fatalf("Read = %+v, %v", res, err)
	}
	if v := res.Values[0].Value; v == nil || v.Type != model.TypeFloat || v.JSON == nil {
		t.Errorf("Hz.mag.f = %+v", res.Values[0])
	} else if f, ok := v.JSON.(float64); !ok || f < 50.001 || f > 50.003 {
		t.Errorf("Hz.mag.f JSON = %#v", v.JSON)
	}
	if v := res.Values[1].Value; v == nil || v.JSON != true {
		t.Errorf("Ind1.stVal = %+v", res.Values[1])
	}
	if res.Values[2].Error == nil || res.Values[2].Value != nil {
		t.Errorf("unknown object = %+v", res.Values[2])
	}
	if _, err := svc.Read(ctx, info.ID, model.ReadRequest{}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("empty read = %v", err)
	}

	// Close.
	if err := svc.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	gone := next(t, events, "closed event", func(ev model.Event) bool {
		return ev.Kind == model.EventSession && ev.Session.State == model.SessionClosed
	})
	if gone.SessionID != info.ID {
		t.Errorf("closed event = %+v", gone)
	}
	if list, _ := svc.Sessions(ctx); len(list) != 0 {
		t.Errorf("Sessions after close = %+v", list)
	}
}
