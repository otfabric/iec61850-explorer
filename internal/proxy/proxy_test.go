// SPDX-License-Identifier: MIT

package proxy_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/proxy"
	"github.com/otfabric/iec61850-explorer/internal/proxy/proxytest"
)

func mustParse(t *testing.T, raw string) proxy.Config {
	t.Helper()
	c, err := proxy.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	return c
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		raw, redacted string
		scheme        proxy.Scheme
		port          int
		user          string
		remote        bool
	}{
		{"socks5://gw", "socks5://gw:1080", proxy.SOCKS5, 1080, "", false},
		{"SOCKS5H://gw:9050/", "socks5h://gw:9050", proxy.SOCKS5H, 9050, "", true},
		{"http://op:s3cret@10.0.0.1", "http://op:xxxxx@10.0.0.1:8080", proxy.HTTP, 8080, "op", true},
		{"socks5://op@[fe80::1]:1", "socks5://op@[fe80::1]:1", proxy.SOCKS5, 1, "op", false},
		{" socks5://op:@gw ", "socks5://op:xxxxx@gw:1080", proxy.SOCKS5, 1080, "op", false},
	} {
		c := mustParse(t, tc.raw)
		if c.Redacted() != tc.redacted || c.Scheme != tc.scheme || c.Port != tc.port || c.Username() != tc.user || c.ResolvesRemotely() != tc.remote {
			t.Errorf("Parse(%q) = %s %s %d %q %v", tc.raw, c.Redacted(), c.Scheme, c.Port, c.Username(), c.ResolvesRemotely())
		}
	}
	for _, raw := range []string{
		"", "gw:1080", "https://gw", "socks4://gw", "socks5://gw/path", "socks5://gw?x=1", "socks5://gw#f", "socks5://gw?",
		"socks5://", "socks5://gw:0", "socks5://gw:65536", "socks5://gw:x", "socks5://gw:", "socks5://:pw@gw",
		"socks5://" + strings.Repeat("u", 256) + "@gw", "socks5://u:p%0a@gw", "http://u%3Ax:p@gw", "socks5://%zz", "socks5:opaque", "http://a%3Ab:c@gw",
		"socks5://g%20w",
	} {
		if c, err := proxy.Parse(raw); err == nil {
			t.Errorf("Parse(%q) = %s, want an error", raw, c.Redacted())
		} else if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("error leaks the password: %v", err)
		}
	}
	if _, err := proxy.Parse("socks5://u:s3cret@gw/%zz"); err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("bad URL error = %v", err)
	}
}

func TestNeverPrintsPassword(t *testing.T) {
	c := mustParse(t, "socks5://op:s3cret@gw")
	for _, s := range []string{c.String(), c.Redacted(), fmt.Sprintf("%v %+v %#v %s", c, c, c, c)} {
		if strings.Contains(s, "s3cret") {
			t.Errorf("%q contains the password", s)
		}
	}
	if !c.HasPassword() || (proxy.Config{}).Redacted() != "" {
		t.Error("HasPassword or empty Redacted")
	}
}

// roundTrip dials through d and checks that the echo server answers.
func roundTrip(t *testing.T, d proxy.DialFunc, addr string, greeting string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := d(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer func() { _ = c.Close() }()
	if greeting != "" {
		buf := make([]byte, len(greeting))
		if _, err := io.ReadFull(c, buf); err != nil || string(buf) != greeting {
			t.Fatalf("greeting %q, %v", buf, err)
		}
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo %q, %v", buf, err)
	}
}

func TestSOCKS5(t *testing.T) {
	echo := proxytest.Echo(t, nil)
	_, port, _ := net.SplitHostPort(echo)

	t.Run("no auth, address", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "", "")
		roundTrip(t, proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{}), echo, "")
		if got := s.Targets(); len(got) != 1 || got[0] != echo {
			t.Errorf("targets %v", got)
		}
	})
	t.Run("auth, local resolution", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "op", "s3cret")
		var checked []netip.Addr
		d := proxy.Dialer(mustParse(t, "socks5://op:s3cret@"+s.Addr), proxy.Options{
			Resolver: func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("::ffff:127.0.0.1")}, nil
			},
			CheckAddr: func(a netip.Addr) error { checked = append(checked, a); return nil },
		})
		roundTrip(t, d, net.JoinHostPort("ied.example", port), "")
		if got := s.Targets(); len(got) != 1 || got[0] != echo {
			t.Errorf("targets %v, want the resolved address", got)
		}
		if len(checked) != 1 || checked[0] != netip.MustParseAddr("127.0.0.1") {
			t.Errorf("checked %v", checked)
		}
	})
	t.Run("socks5h sends the name", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "", "")
		d := proxy.Dialer(mustParse(t, "socks5h://"+s.Addr), proxy.Options{
			CheckAddr: func(netip.Addr) error { t.Error("a name is not checked locally"); return nil },
		})
		roundTrip(t, d, net.JoinHostPort("localhost", port), "")
		if got := s.Targets(); len(got) != 1 || got[0] != "name:localhost:"+port {
			t.Errorf("targets %v", got)
		}
	})
	t.Run("IPv6", func(t *testing.T) {
		ln, err := net.Listen("tcp", "[::1]:0")
		if err != nil {
			t.Skip("no IPv6 loopback")
		}
		_ = ln.Close()
		s := proxytest.SOCKS5(t, "", "")
		_, err = proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{})(context.Background(), "tcp", "[::1]:1")
		// The test proxy cannot reach port 1 and answers reply 5.
		var pe *proxy.Error
		if !errors.As(err, &pe) || pe.Stage != proxy.StageRefused || pe.Reply != 5 {
			t.Fatalf("err = %v", err)
		}
		if got := s.Targets(); len(got) != 1 || got[0] != "[::1]:1" {
			t.Errorf("targets %v", got)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "op", "s3cret")
		_, err := proxy.Dialer(mustParse(t, "socks5://op:wrong@"+s.Addr), proxy.Options{})(context.Background(), "tcp", echo)
		wantStage(t, err, proxy.StageAuth)
		if strings.Contains(err.Error(), "wrong") {
			t.Errorf("error leaks the password: %v", err)
		}
	})
	t.Run("auth required", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "op", "s3cret")
		_, err := proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{})(context.Background(), "tcp", echo)
		wantStage(t, err, proxy.StageAuth)
	})
	t.Run("not allowed", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "", "")
		_, err := proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{
			CheckAddr: func(netip.Addr) error { return errors.New("outside") },
		})(context.Background(), "tcp", echo)
		wantStage(t, err, proxy.StageNotAllowed)
		if len(s.Targets()) != 0 {
			t.Error("a refused address reached the proxy")
		}
	})
	t.Run("resolve fails", func(t *testing.T) {
		for _, res := range []func(context.Context, string) ([]netip.Addr, error){
			func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("nxdomain") },
			func(context.Context, string) ([]netip.Addr, error) { return nil, nil },
		} {
			_, err := proxy.Dialer(mustParse(t, "socks5://127.0.0.1:1"), proxy.Options{Resolver: res})(context.Background(), "tcp", "ied.invalid:102")
			wantStage(t, err, proxy.StageResolve)
		}
	})
	t.Run("default resolver", func(t *testing.T) {
		s := proxytest.SOCKS5(t, "", "")
		roundTrip(t, proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{}), net.JoinHostPort("localhost", port), "")
	})
}

// failure is what a test checks of a *proxy.Error.
type failure struct {
	Reply, Status int
	Proxy, Msg    string
}

func wantStage(t *testing.T, err error, stage proxy.Stage) failure {
	t.Helper()
	var pe *proxy.Error
	if !errors.As(err, &pe) || pe.Stage != stage {
		t.Fatalf("err = %v, want stage %s", err, stage)
	}
	if pe.Error() == "" {
		t.Error("empty message")
	}
	return failure{Reply: pe.Reply, Status: pe.Status, Proxy: pe.Proxy, Msg: pe.Error()}
}

func TestHTTPConnect(t *testing.T) {
	echo := proxytest.Echo(t, []byte("hello"))
	_, port, _ := net.SplitHostPort(echo)

	t.Run("no auth", func(t *testing.T) {
		s := proxytest.HTTP(t, "", "", nil)
		roundTrip(t, proxy.Dialer(mustParse(t, "http://"+s.Addr), proxy.Options{}), echo, "hello")
	})
	t.Run("auth, name sent", func(t *testing.T) {
		s := proxytest.HTTP(t, "op", "s3cret", nil)
		roundTrip(t, proxy.Dialer(mustParse(t, "http://op:s3cret@"+s.Addr), proxy.Options{}), net.JoinHostPort("localhost", port), "hello")
		if got := s.Targets(); len(got) != 1 || got[0] != "localhost:"+port {
			t.Errorf("targets %v", got)
		}
	})
	t.Run("bytes after the header", func(t *testing.T) {
		s := proxytest.HTTP(t, "", "", []byte("early-"))
		roundTrip(t, proxy.Dialer(mustParse(t, "http://"+s.Addr), proxy.Options{}), echo, "early-hello")
	})
	t.Run("407", func(t *testing.T) {
		s := proxytest.HTTP(t, "op", "s3cret", nil)
		_, err := proxy.Dialer(mustParse(t, "http://op:bad@"+s.Addr), proxy.Options{})(context.Background(), "tcp", echo)
		if pe := wantStage(t, err, proxy.StageAuth); pe.Status != 407 {
			t.Errorf("status %d", pe.Status)
		}
	})
	t.Run("502", func(t *testing.T) {
		s := proxytest.HTTP(t, "", "", nil)
		_, err := proxy.Dialer(mustParse(t, "http://"+s.Addr), proxy.Options{})(context.Background(), "tcp", "127.0.0.1:1")
		if pe := wantStage(t, err, proxy.StageRefused); pe.Status != 502 || !strings.Contains(pe.Msg, "502") {
			t.Errorf("%v", pe)
		}
	})
	t.Run("IPv6 address", func(t *testing.T) {
		s := proxytest.HTTP(t, "", "", nil)
		_, _ = proxy.Dialer(mustParse(t, "http://"+s.Addr), proxy.Options{})(context.Background(), "tcp", "[::1]:1")
		if got := s.Targets(); len(got) != 1 || got[0] != "[::1]:1" {
			t.Errorf("targets %v", got)
		}
	})
}

// step reads read octets (a negative read: an HTTP request header), then
// writes reply.
type step struct {
	read  int
	reply string
}

// scripted runs the steps on each connection, then closes it.
func scripted(t *testing.T, steps ...step) string {
	return proxytest.Scripted(t, func(c net.Conn) {
		for _, st := range steps {
			if st.read >= 0 {
				if _, err := io.ReadFull(c, make([]byte, st.read)); err != nil {
					return
				}
			} else if _, err := http.ReadRequest(bufio.NewReader(c)); err != nil {
				return
			}
			_, _ = io.WriteString(c, st.reply)
		}
	}).Addr
}

func TestMalformed(t *testing.T) {
	for _, tc := range []struct {
		name, scheme string
		read         int
		reply        string
		stage        proxy.Stage
		reply5       int
	}{
		{"socks version", "socks5", 3, "\x04\x00", proxy.StageHandshake, 0},
		{"socks closed", "socks5", 3, "", proxy.StageHandshake, 0},
		{"socks no acceptable", "socks5", 3, "\x05\xff", proxy.StageAuth, 0},
		{"socks unoffered method", "socks5", 3, "\x05\x02", proxy.StageHandshake, 0},
		{"socks odd method", "socks5", 3, "\x05\x01", proxy.StageHandshake, 0},
		{"socks reply version", "socks5", 13, "\x05\x00\x04\x00\x00\x01", proxy.StageHandshake, 0},
		{"socks reserved", "socks5", 13, "\x05\x00\x05\x00\x01\x01", proxy.StageHandshake, 0},
		{"socks atyp", "socks5", 13, "\x05\x00\x05\x00\x00\x09", proxy.StageHandshake, 0},
		{"socks short addr", "socks5", 13, "\x05\x00\x05\x00\x00\x01\x7f", proxy.StageHandshake, 0},
		{"socks short head", "socks5", 13, "\x05\x00\x05", proxy.StageHandshake, 0},
		{"socks domain no len", "socks5", 13, "\x05\x00\x05\x00\x00\x03", proxy.StageHandshake, 0},
		{"socks refused", "socks5", 13, "\x05\x00\x05\x02\x00\x03\x02ab\x00\x00", proxy.StageRefused, 2},
		{"socks refused v6", "socks5", 13, "\x05\x00\x05\x09\x00\x04" + strings.Repeat("\x00", 18), proxy.StageRefused, 9},
		{"http garbage", "http", -1, "SSH-2.0-x\r\n\r\n", proxy.StageHandshake, 0},
		{"http bad code", "http", -1, "HTTP/1.1 2x0 OK\r\n\r\n", proxy.StageHandshake, 0},
		{"http no code", "http", -1, "HTTP/1.1\r\n\r\n", proxy.StageHandshake, 0},
		{"http cut", "http", -1, "HTTP/1.1 200 OK\r\nVia: x", proxy.StageHandshake, 0},
		{"http oversized", "http", -1, "HTTP/1.1 200 OK\r\nX: " + strings.Repeat("a", 9000) + "\r\n\r\n", proxy.StageHandshake, 0},
		{"http long status", "http", -1, "HTTP/1.1 " + strings.Repeat("9", 100) + "\r\n\r\n", proxy.StageHandshake, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := []step{{tc.read, tc.reply}}
			if tc.scheme == "socks5" && len(tc.reply) > 2 {
				// Method selection, then the reply to CONNECT (IPv4).
				steps = []step{{3, tc.reply[:2]}, {10, tc.reply[2:]}}
			}
			addr := scripted(t, steps...)
			raw := tc.scheme + "://" + addr
			if tc.name == "socks unoffered method" {
				raw = "socks5://" + addr // offers no auth only
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := proxy.Dialer(mustParse(t, raw), proxy.Options{})(ctx, "tcp", "127.0.0.1:102")
			pe := wantStage(t, err, tc.stage)
			if pe.Reply != tc.reply5 {
				t.Errorf("reply %d", pe.Reply)
			}
		})
	}
	// A SOCKS proxy that skips authentication although credentials were
	// offered, and one that refuses the credentials.
	addr := scripted(t, step{3, "\x05\x00"})
	_, err := proxy.Dialer(mustParse(t, "socks5://u:p@"+addr), proxy.Options{})(context.Background(), "tcp", "127.0.0.1:102")
	wantStage(t, err, proxy.StageHandshake)
	addr = scripted(t, step{3, "\x05\x02"}, step{5, "\x02\x00"})
	_, err = proxy.Dialer(mustParse(t, "socks5://u:p@"+addr), proxy.Options{})(context.Background(), "tcp", "127.0.0.1:102")
	wantStage(t, err, proxy.StageHandshake)
	addr = scripted(t, step{3, "\x05\x02"}, step{5, "\x01"})
	_, err = proxy.Dialer(mustParse(t, "socks5://u:p@"+addr), proxy.Options{})(context.Background(), "tcp", "127.0.0.1:102")
	wantStage(t, err, proxy.StageAuth)
}

func TestDialErrors(t *testing.T) {
	d := proxy.Dialer(mustParse(t, "socks5://127.0.0.1:1"), proxy.Options{})
	for _, tc := range []struct{ network, addr string }{
		{"udp", "127.0.0.1:102"}, {"tcp", "no-port"}, {"tcp", "h:0"}, {"tcp", "h:x"},
		{"tcp", net.JoinHostPort(strings.Repeat("h", 256), "102")},
	} {
		_, err := d(context.Background(), tc.network, tc.addr)
		wantStage(t, err, proxy.StageConnect)
	}
	// No proxy listening.
	_, err := d(context.Background(), "tcp", "127.0.0.1:102")
	wantStage(t, err, proxy.StageConnect)
}

func TestContext(t *testing.T) {
	// A proxy that never answers.
	silent := proxytest.Scripted(t, func(c net.Conn) { _, _ = io.Copy(io.Discard, c) })
	for _, scheme := range []string{"socks5", "http"} {
		t.Run(scheme+" deadline", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := proxy.Dialer(mustParse(t, scheme+"://"+silent.Addr), proxy.Options{})(ctx, "tcp", "127.0.0.1:102")
			wantStage(t, err, proxy.StageHandshake)
			var ne net.Error
			if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &ne) || !ne.Timeout()) {
				t.Errorf("err = %v, want a timeout", err)
			}
			if time.Since(start) > 3*time.Second {
				t.Error("the deadline was not honoured")
			}
		})
		t.Run(scheme+" cancel", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(50*time.Millisecond, cancel)
			_, err := proxy.Dialer(mustParse(t, scheme+"://"+silent.Addr), proxy.Options{})(ctx, "tcp", "127.0.0.1:102")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want canceled", err)
			}
			var pe *proxy.Error
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v is not a *proxy.Error", err)
			}
		})
	}
	// A connection outlives the dial context and its deadline.
	echo := proxytest.Echo(t, nil)
	s := proxytest.SOCKS5(t, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	c, err := proxy.Dialer(mustParse(t, "socks5://"+s.Addr), proxy.Options{})(ctx, "tcp", echo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	<-ctx.Done()
	cancel()
	time.Sleep(50 * time.Millisecond)
	if _, err := c.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
}

func TestBaseDialer(t *testing.T) {
	called := ""
	d := proxy.Dialer(mustParse(t, "socks5://op:s3cret@proxy.example:1"), proxy.Options{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			called = addr
			return nil, errors.New("refused by base")
		},
	})
	_, err := d(context.Background(), "tcp", "10.0.0.1:102")
	pe := wantStage(t, err, proxy.StageConnect)
	if called != "proxy.example:1" || strings.Contains(err.Error(), "s3cret") || pe.Proxy != "socks5://op:xxxxx@proxy.example:1" {
		t.Errorf("called %q, err %v", called, err)
	}
	for code := range 10 {
		if proxy.ReplyText(code) == "" {
			t.Errorf("reply %d has no text", code)
		}
	}
}
