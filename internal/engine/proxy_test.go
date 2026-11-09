// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/proxy"
	"github.com/otfabric/iec61850-explorer/internal/proxy/proxytest"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// Each proxy type reaches the simulated IED, per target and as the default;
// "direct" bypasses the default.
func TestOpenThroughProxy(t *testing.T) {
	ctx := context.Background()
	ied := startIED(t)
	_, port, _ := net.SplitHostPort(ied.Addr)
	socks := proxytest.SOCKS5(t, "op", "s3cret")
	socksh := proxytest.SOCKS5(t, "", "")
	httpProxy := proxytest.HTTP(t, "op", "s3cret", nil)

	for _, tc := range []struct {
		name, proxy string
		srv         *proxytest.Server
		host, want  string
	}{
		{"socks5", "socks5://op:s3cret@" + socks.Addr, socks, "localhost", ied.Addr},
		{"socks5h", "socks5h://" + socksh.Addr, socksh, "localhost", "name:localhost:" + port},
		{"http", "http://op:s3cret@" + httpProxy.Addr, httpProxy, "localhost", "localhost:" + port},
	} {
		for _, asDefault := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s default=%v", tc.name, asDefault), func(t *testing.T) {
				e := New(Options{DataDir: t.TempDir()})
				t.Cleanup(e.Close)
				tgt := target(t, ied.Addr)
				tgt.Host = tc.host
				if asDefault {
					if _, err := e.SaveSettings(ctx, model.Settings{DefaultProxy: tc.proxy}); err != nil {
						t.Fatal(err)
					}
				} else {
					tgt.Proxy = tc.proxy
				}
				before := len(tc.srv.Targets())
				info, err := e.OpenSession(ctx, tgt)
				if err != nil {
					t.Fatalf("OpenSession: %v", err)
				}
				defer func() { _ = e.CloseSession(ctx, info.ID) }()
				got := tc.srv.Targets()
				// With socks5 the proxy may first be asked for an address
				// the IED does not listen on ("localhost" is ::1 first on
				// Linux); the connection is the last one asked for.
				if len(got) <= before || got[len(got)-1] != tc.want {
					t.Errorf("proxy targets %v, want %s", got, tc.want)
				}
				if strings.Contains(info.Proxy, "s3cret") || info.Proxy == "" {
					t.Errorf("session proxy %q", info.Proxy)
				}
				if _, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{"IEDLD0/LLN0.Mod.stVal[ST]"}}); err != nil {
					t.Errorf("Read: %v", err)
				}
			})
		}
	}

	t.Run("direct overrides the default", func(t *testing.T) {
		e := New(Options{DataDir: t.TempDir()})
		t.Cleanup(e.Close)
		if _, err := e.SaveSettings(ctx, model.Settings{DefaultProxy: "socks5://127.0.0.1:1"}); err != nil {
			t.Fatal(err)
		}
		tgt := target(t, ied.Addr)
		tgt.Proxy = "Direct"
		info, err := e.OpenSession(ctx, tgt)
		if err != nil || info.Proxy != "" || info.Target.Proxy != model.ProxyDirect {
			t.Fatalf("open = %+v, %v", info, err)
		}
		_ = e.CloseSession(ctx, info.ID)
		// Without "direct" the unreachable default is used, and named.
		_, err = e.OpenSession(ctx, target(t, ied.Addr))
		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Detail == nil || se.Detail.Stage != stageProxy || se.Detail.Hint == "" {
			t.Fatalf("open through a dead default = %#v", err)
		}
	})

	t.Run("operator default", func(t *testing.T) {
		e := New(Options{DefaultProxy: "socks5://op:s3cret@" + socks.Addr})
		t.Cleanup(e.Close)
		if s, _ := e.Settings(ctx); !s.DefaultProxyLocked || strings.Contains(s.DefaultProxy, "s3cret") {
			t.Errorf("Settings = %+v", s)
		}
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil || !strings.HasPrefix(info.Proxy, "socks5://op:xxxxx@") {
			t.Fatalf("open = %+v, %v", info, err)
		}
		_ = e.CloseSession(ctx, info.ID)
	})
}

// A Commission grant opens its second association through the session's
// proxy.
func TestCommissionThroughProxy(t *testing.T) {
	ctx := context.Background()
	ied := startIED(t)
	socks := proxytest.SOCKS5(t, "", "")
	e, _ := newCommissionEngine(t)
	tgt := target(t, ied.Addr)
	tgt.Proxy = "socks5://" + socks.Addr
	info, err := e.OpenSession(ctx, tgt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
		t.Fatalf("GrantCommission: %v", err)
	}
	if got := socks.Targets(); len(got) != 2 {
		t.Errorf("proxy targets %v, want two associations", got)
	}
	_ = e.CloseSession(ctx, info.ID)
}

// The allow-list hooks: the pre-check of the IED and of a user's proxy, the
// IP literal rule for proxies that resolve names, and the check when the
// connection is opened.
func TestProxyAllowList(t *testing.T) {
	ctx := context.Background()
	ied := startIED(t)
	socks := proxytest.SOCKS5(t, "", "")
	allowed := map[string]bool{"127.0.0.1": true}
	var checked []netip.Addr
	newAllowEngine := func(t *testing.T, opts Options) *Engine {
		opts.AllowTarget = func(_ context.Context, host string) error {
			if host == "localhost" || allowed[host] {
				return nil
			}
			return fmt.Errorf("%s is outside", host)
		}
		opts.AllowAddr = func(a netip.Addr) error {
			checked = append(checked, a)
			if allowed[a.String()] {
				return nil
			}
			return fmt.Errorf("%s is outside", a)
		}
		e := New(opts)
		t.Cleanup(e.Close)
		return e
	}

	t.Run("direct is checked when dialling", func(t *testing.T) {
		e := newAllowEngine(t, Options{})
		checked = nil
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		_ = e.CloseSession(ctx, info.ID)
		if len(checked) == 0 || checked[0] != netip.MustParseAddr("127.0.0.1") {
			t.Errorf("checked %v", checked)
		}
		// localhost passes the pre-check but resolves to an address the
		// dial-time check refuses (here: ::1 or 127.0.0.1 removed).
		delete(allowed, "127.0.0.1")
		defer func() { allowed["127.0.0.1"] = true }()
		tgt := target(t, ied.Addr)
		tgt.Host = "localhost"
		_, err = e.OpenSession(ctx, tgt)
		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Detail == nil || se.Detail.Code != model.ErrCodeNotAllowed {
			t.Fatalf("open = %#v", err)
		}
	})
	t.Run("user proxy outside the allow-list", func(t *testing.T) {
		e := newAllowEngine(t, Options{})
		tgt := target(t, ied.Addr)
		tgt.Proxy = "socks5://192.0.2.1:1080"
		if _, err := e.OpenSession(ctx, tgt); service.CodeOf(err) != service.CodeNotAllowed {
			t.Errorf("open = %v", err)
		}
	})
	t.Run("user proxy checked when dialling", func(t *testing.T) {
		e := newAllowEngine(t, Options{})
		tgt := target(t, ied.Addr)
		_, port, _ := net.SplitHostPort(socks.Addr)
		tgt.Proxy = "socks5://localhost:" + port // pre-check passes by name
		delete(allowed, "127.0.0.1")
		defer func() { allowed["127.0.0.1"] = true }()
		tgt.Host = "127.0.0.2"
		allowed["127.0.0.2"] = true
		defer delete(allowed, "127.0.0.2")
		_, err := e.OpenSession(ctx, tgt)
		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Detail == nil || se.Detail.Code != model.ErrCodeNotAllowed || se.Detail.Stage != stageProxy {
			t.Fatalf("open = %#v", err)
		}
	})
	t.Run("names need an IP with a remote-resolving proxy", func(t *testing.T) {
		e := newAllowEngine(t, Options{DefaultProxy: "socks5h://" + socks.Addr})
		tgt := target(t, ied.Addr)
		tgt.Host = "localhost"
		_, err := e.OpenSession(ctx, tgt)
		if service.CodeOf(err) != service.CodeNotAllowed || !strings.Contains(err.Error(), "IP address") {
			t.Errorf("open = %v", err)
		}
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatalf("open by address: %v", err)
		}
		_ = e.CloseSession(ctx, info.ID)
	})
	t.Run("operator proxy is trusted, the IED is not", func(t *testing.T) {
		e := newAllowEngine(t, Options{DefaultProxy: "socks5://" + socks.Addr})
		delete(allowed, "127.0.0.1")
		defer func() { allowed["127.0.0.1"] = true }()
		allowed["127.0.0.3"] = true
		defer delete(allowed, "127.0.0.3")
		tgt := target(t, ied.Addr)
		tgt.Host, tgt.TimeoutMS = "127.0.0.3", 500
		// The proxy at 127.0.0.1 is reached; the IED address is allowed
		// and the test proxy cannot reach it (nothing listens there).
		_, err := e.OpenSession(ctx, tgt)
		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Detail == nil || se.Detail.Stage != stageProxy || se.Detail.Code == model.ErrCodeNotAllowed {
			t.Fatalf("open = %#v", err)
		}
		tgt.Host = "localhost" // resolved locally, then refused
		_, err = e.OpenSession(ctx, tgt)
		se, ok = errors.AsType[*service.Error](err)
		if !ok || se.Detail == nil || se.Detail.Code != model.ErrCodeNotAllowed {
			t.Fatalf("open = %#v", err)
		}
	})
}

func TestSettingsStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := New(Options{DataDir: dir})
	t.Cleanup(e.Close)
	if _, err := e.SaveSettings(ctx, model.Settings{DefaultProxy: " socks5://op:s3cret@gw "}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings file %v, %v", info, err)
	}
	// Loaded again at start.
	again := New(Options{DataDir: dir})
	t.Cleanup(again.Close)
	if s, _ := again.Settings(ctx); s.DefaultProxy != "socks5://op:s3cret@gw" {
		t.Errorf("Settings = %+v", s)
	}
	// A bad file is ignored with a warning.
	for _, content := range []string{"{", `{"schema":"other"}`, `{"schema":"` + SettingsSchema + `","default_proxy":"ftp://x"}`, strings.Repeat(" ", maxSettingsFile+1)} {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		bad := New(Options{DataDir: dir})
		if s, _ := bad.Settings(ctx); s.DefaultProxy != "" {
			t.Errorf("Settings from %.20q = %+v", content, s)
		}
		bad.Close()
	}
	// No data directory: settings are read-only and empty.
	none := newEngine(t)
	if _, err := none.SaveSettings(ctx, model.Settings{}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("SaveSettings without a data dir = %v", err)
	}
	// An unreadable path.
	if err := os.Remove(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveSettings(ctx, model.Settings{DefaultProxy: "direct"}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("SaveSettings onto a directory = %v", err)
	}
	if err := (&settingsStore{path: filepath.Join(dir, "settings.json")}).load(); err == nil {
		t.Error("loading a directory succeeded")
	}
	if err := (&settingsStore{}).save(""); err == nil {
		t.Error("save without a path succeeded")
	}
	if err := writeFileAtomic(filepath.Join(dir, "missing", "f"), "x-*", nil); err == nil {
		t.Error("write into a missing directory succeeded")
	}
}

func TestProxyHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"": "", " direct ": "direct", "socks5://u:p@h": "socks5://u:xxxxx@h:1080", "ftp://x": "",
	} {
		if got := redactProxySetting(in); got != want {
			t.Errorf("redactProxySetting(%q) = %q, want %q", in, got, want)
		}
	}
	e := New(Options{DefaultProxy: "ftp://broken"})
	t.Cleanup(e.Close)
	if _, err := e.chooseProxy(""); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("chooseProxy with a broken operator default = %v", err)
	}
	e.opts.DefaultProxy = ""
	e.settings.defaultProxy = "ftp://broken"
	if _, err := e.chooseProxy(""); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("chooseProxy with a broken default = %v", err)
	}
	// The dial-time check refuses an address that is not one.
	d := New(Options{AllowAddr: func(netip.Addr) error { return nil }}).dialer(proxyChoice{})
	if _, err := d(context.Background(), "unix", "/nonexistent"); err == nil {
		t.Error("dial of a non-IP address succeeded")
	}
	refused := &AddrRefusedError{Addr: netip.MustParseAddr("192.0.2.1"), Err: errors.New("outside")}
	if refused.Error() != "outside" || !errors.Is(refused, refused.Err) {
		t.Error("AddrRefusedError")
	}
	// Classification of every proxy stage.
	for _, st := range []proxy.Stage{proxy.StageConnect, proxy.StageResolve, proxy.StageNotAllowed, proxy.StageHandshake, proxy.StageAuth, proxy.StageRefused} {
		got := classify(fmt.Errorf("dial: %w", &proxy.Error{Proxy: "socks5://h:1", Stage: st}), true)
		if got.Stage != stageProxy || got.Hint == "" {
			t.Errorf("classify(%s) = %+v", st, got)
		}
	}
	got := classify(&proxy.Error{Stage: proxy.StageHandshake, Err: context.DeadlineExceeded}, true)
	if got.Code != model.ErrCodeTimeout {
		t.Errorf("classify(timeout) = %+v", got)
	}
}

// A snapshot that holds a proxy password does not show it.
func TestSnapshotProxyRedacted(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t)
	ied := startIED(t)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	exp, err := e.Export(ctx, info.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(exp.Text, `"target": {`, `"target": {"proxy": "socks5://u:s3cret@gw",`, 1)
	if text == exp.Text {
		t.Fatal("no target in the snapshot")
	}
	snap, err := e.OpenSnapshot(ctx, model.File{Name: "s.json", Data: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Target.Proxy != "socks5://u:xxxxx@gw:1080" {
		t.Errorf("snapshot target proxy %q", snap.Target.Proxy)
	}
}
