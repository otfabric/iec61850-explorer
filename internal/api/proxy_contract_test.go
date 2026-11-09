// SPDX-License-Identifier: MIT

package api_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/proxy/proxytest"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// TestContractProxy: the settings (the default proxy) and connecting through
// a proxy behave the same through the engine and through the HTTP API.
func TestContractProxy(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{DataDir: t.TempDir()})
		t.Cleanup(eng.Close)
		contractProxy(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		contractProxy(t, newDaemon(t, engine.Options{DataDir: t.TempDir()}, api.Options{}).svc)
	})
	t.Run("engine locked", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "op", "s3cret")
		eng := engine.New(engine.Options{DataDir: t.TempDir(), DefaultProxy: "socks5://op:s3cret@" + s.Addr})
		t.Cleanup(eng.Close)
		contractLocked(t, eng, s)
	})
	t.Run("http locked", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "op", "s3cret")
		contractLocked(t, newDaemon(t, engine.Options{DataDir: t.TempDir(), DefaultProxy: "socks5://op:s3cret@" + s.Addr}, api.Options{}).svc, s)
	})
}

// noPassword fails when v, as JSON, contains the password.
func noPassword(t *testing.T, what string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "s3cret") {
		t.Errorf("%s contains the proxy password: %s", what, b)
	}
}

func contractProxy(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	socks := proxytest.SOCKS5(t, "op", "s3cret")
	httpProxy := proxytest.HTTP(t, "", "", nil)

	// Settings: empty at first, changed, refused when invalid.
	if s, err := svc.Settings(ctx); err != nil || s.DefaultProxy != "" || s.DefaultProxyLocked {
		t.Fatalf("Settings = %+v, %v", s, err)
	}
	for _, bad := range []string{"ftp://x", "socks5://h/p", strings.Repeat("a", model.MaxProxyLen+1)} {
		if _, err := svc.SaveSettings(ctx, model.Settings{DefaultProxy: bad}); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("SaveSettings(%.20q) = %v", bad, err)
		}
	}
	if s, err := svc.SaveSettings(ctx, model.Settings{DefaultProxy: " DIRECT "}); err != nil || s.DefaultProxy != model.ProxyDirect {
		t.Errorf("SaveSettings(direct) = %+v, %v", s, err)
	}

	// A per-target proxy: the session says which, without the password.
	target := targetOf(t, ied.Addr)
	target.Proxy = "socks5://op:s3cret@" + socks.Addr
	info, err := svc.OpenSession(ctx, target)
	if err != nil {
		t.Fatalf("open through socks5: %v", err)
	}
	if info.Proxy != "socks5://op:xxxxx@"+socks.Addr || info.Target.Proxy != info.Proxy {
		t.Errorf("session proxy %q, target proxy %q", info.Proxy, info.Target.Proxy)
	}
	noPassword(t, "session info", info)
	list, _ := svc.Sessions(ctx)
	noPassword(t, "sessions", list)
	exp, err := svc.Export(ctx, info.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	noPassword(t, "snapshot", exp)
	if got := socks.Targets(); len(got) != 1 || got[0] != ied.Addr {
		t.Errorf("the proxy was asked for %v", got)
	}
	_ = svc.CloseSession(ctx, info.ID)

	// The default proxy applies to a target without one; "direct" overrides it.
	if _, err := svc.SaveSettings(ctx, model.Settings{DefaultProxy: "http://" + httpProxy.Addr}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Settings(ctx); s.DefaultProxy != "http://"+httpProxy.Addr {
		t.Errorf("Settings = %+v", s)
	}
	info, err = svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil || info.Proxy != "http://"+httpProxy.Addr || info.Target.Proxy != "" {
		t.Fatalf("open through the default = %+v, %v", info, err)
	}
	_ = svc.CloseSession(ctx, info.ID)
	direct := targetOf(t, ied.Addr)
	direct.Proxy = "direct"
	info, err = svc.OpenSession(ctx, direct)
	if err != nil || info.Proxy != "" || info.Target.Proxy != model.ProxyDirect {
		t.Fatalf("open direct = %+v, %v", info, err)
	}
	_ = svc.CloseSession(ctx, info.ID)
	if got := httpProxy.Targets(); len(got) != 1 {
		t.Errorf("the HTTP proxy was asked for %v", got)
	}

	// A refusal by the proxy is classified as such.
	wrong := targetOf(t, ied.Addr)
	wrong.Proxy = "socks5://op:wrong@" + socks.Addr
	_, err = svc.OpenSession(ctx, wrong)
	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != service.CodeConnectFailed || se.Detail == nil || se.Detail.Stage != "proxy" || se.Detail.Code != model.ErrCodeAccessDenied {
		t.Errorf("open with a wrong proxy password = %#v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Errorf("the error contains the password: %v", err)
	}
	bad := targetOf(t, ied.Addr)
	bad.Proxy = "socks4://x"
	if _, err := svc.OpenSession(ctx, bad); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("open with a bad proxy = %v", err)
	}

	// Profiles keep the per-target proxy as given.
	p, err := svc.SaveProfile(ctx, model.Profile{Name: "via proxy", Target: target})
	if err != nil || p.Target.Proxy != target.Proxy {
		t.Errorf("SaveProfile = %+v, %v", p, err)
	}
	bp := model.Profile{Name: "bad proxy", Target: bad}
	if _, err := svc.SaveProfile(ctx, bp); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("SaveProfile with a bad proxy = %v", err)
	}
	if _, err := svc.SaveSettings(ctx, model.Settings{}); err != nil {
		t.Error(err)
	}
}

// contractLocked: a default proxy the operator set is used, shown without its
// password and cannot be changed.
func contractLocked(t *testing.T, svc service.Service, socks *proxytest.Server) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	s, err := svc.Settings(ctx)
	if err != nil || !s.DefaultProxyLocked || s.DefaultProxy != "socks5://op:xxxxx@"+socks.Addr {
		t.Fatalf("Settings = %+v, %v", s, err)
	}
	if _, err := svc.SaveSettings(ctx, model.Settings{DefaultProxy: "direct"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("SaveSettings = %v, want not_allowed", err)
	}
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil || info.Proxy != s.DefaultProxy {
		t.Fatalf("open = %+v, %v", info, err)
	}
	noPassword(t, "session", info)
	_ = svc.CloseSession(ctx, info.ID)
}
