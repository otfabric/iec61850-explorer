// SPDX-License-Identifier: MIT

// Package proxytest runs in-process SOCKS5 and HTTP CONNECT proxies for
// tests. It is imported by tests only (TestSimIEDOnlyInTests).
package proxytest

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// dialTimeout bounds the proxy's connection to a destination.
const dialTimeout = 2 * time.Second

// Server is a running test proxy.
type Server struct {
	// Addr is the proxy's host:port on the loopback interface.
	Addr string
	// User and Pass, when User is set, are the credentials it requires.
	User, Pass string
	// Early, for HTTP, is sent right after the 200 response header, in the
	// same write, as if the destination spoke first.
	Early []byte

	ln net.Listener
	mu sync.Mutex
	// targets are the destinations requested, as sent ("host:port"),
	// with "name:" in front of a SOCKS domain name.
	targets []string
	conns   map[net.Conn]struct{}
	wg      sync.WaitGroup
}

// Targets lists the destinations clients asked for.
func (s *Server) Targets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.targets...)
}

func (s *Server) record(t string) {
	s.mu.Lock()
	s.targets = append(s.targets, t)
	s.mu.Unlock()
}

func listen(t testing.TB) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func (s *Server) serve(t testing.TB, handle func(net.Conn)) {
	s.conns = map[net.Conn]struct{}{}
	t.Cleanup(func() {
		_ = s.ln.Close()
		s.mu.Lock()
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := s.ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns[c] = struct{}{}
			s.mu.Unlock()
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				handle(c)
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
			}()
		}
	}()
}

// SOCKS5 starts a SOCKS5 proxy; user "" accepts no authentication only.
func SOCKS5(t testing.TB, user, pass string) *Server {
	s := &Server{User: user, Pass: pass, ln: listen(t)}
	s.Addr = s.ln.Addr().String()
	s.serve(t, s.socks)
	return s
}

// HTTP starts an HTTP CONNECT proxy; user "" requires no authentication.
func HTTP(t testing.TB, user, pass string, early []byte) *Server {
	s := &Server{User: user, Pass: pass, Early: early, ln: listen(t)}
	s.Addr = s.ln.Addr().String()
	s.serve(t, s.http)
	return s
}

// Scripted starts a server that runs handle on each connection, for
// malformed and refusing proxies.
func Scripted(t testing.TB, handle func(net.Conn)) *Server {
	s := &Server{ln: listen(t)}
	s.Addr = s.ln.Addr().String()
	s.serve(t, func(c net.Conn) {
		defer func() { _ = c.Close() }()
		handle(c)
	})
	return s
}

func (s *Server) socks(c net.Conn) {
	defer func() { _ = c.Close() }()
	var head [2]byte
	if _, err := io.ReadFull(c, head[:]); err != nil || head[0] != 5 {
		return
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	want := byte(0)
	if s.User != "" {
		want = 2
	}
	if !strings.ContainsRune(string(methods), rune(want)) {
		_, _ = c.Write([]byte{5, 0xff})
		return
	}
	_, _ = c.Write([]byte{5, want})
	if want == 2 {
		var v [2]byte
		if _, err := io.ReadFull(c, v[:]); err != nil {
			return
		}
		u := make([]byte, v[1])
		if _, err := io.ReadFull(c, u); err != nil {
			return
		}
		var pl [1]byte
		if _, err := io.ReadFull(c, pl[:]); err != nil {
			return
		}
		p := make([]byte, pl[0])
		if _, err := io.ReadFull(c, p); err != nil {
			return
		}
		if string(u) != s.User || string(p) != s.Pass {
			_, _ = c.Write([]byte{1, 1})
			return
		}
		_, _ = c.Write([]byte{1, 0})
	}
	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil || req[1] != 1 {
		return
	}
	var host string
	switch req[3] {
	case 1, 4:
		b := make([]byte, 4)
		if req[3] == 4 {
			b = make([]byte, 16)
		}
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		a, _ := netip.AddrFromSlice(b)
		host = a.String()
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		host = "name:" + string(b)
	default:
		return
	}
	var pb [2]byte
	if _, err := io.ReadFull(c, pb[:]); err != nil {
		return
	}
	port := strconv.Itoa(int(binary.BigEndian.Uint16(pb[:])))
	dial := strings.TrimPrefix(host, "name:")
	if dial != host {
		s.record("name:" + net.JoinHostPort(dial, port))
	} else {
		s.record(net.JoinHostPort(host, port))
	}
	up, err := net.DialTimeout("tcp", net.JoinHostPort(dial, port), dialTimeout)
	if err != nil {
		_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	pipe(c, up)
}

func (s *Server) http(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		_, _ = io.WriteString(c, "HTTP/1.1 405 Method Not Allowed\r\n\r\n")
		return
	}
	if s.User != "" {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(s.User+":"+s.Pass))
		if req.Header.Get("Proxy-Authorization") != want {
			_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"test\"\r\n\r\n")
			return
		}
	}
	s.record(req.Host)
	up, err := net.DialTimeout("tcp", req.Host, dialTimeout)
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	_, _ = c.Write(append([]byte("HTTP/1.1 200 Connection established\r\nVia: test\r\n\r\n"), s.Early...))
	if br.Buffered() > 0 {
		b, _ := br.Peek(br.Buffered())
		_, _ = up.Write(b)
	}
	pipe(c, up)
}

// pipe copies both ways until one side ends.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
	_ = a.Close()
	_ = b.Close()
	<-done
}

// Echo starts a TCP server that echoes what it receives, after sending
// greeting.
func Echo(t testing.TB, greeting []byte) string {
	ln := listen(t)
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns = map[net.Conn]struct{}{}
	)
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns[c] = struct{}{}
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					_ = c.Close()
					mu.Lock()
					delete(conns, c)
					mu.Unlock()
				}()
				if len(greeting) > 0 {
					_, _ = c.Write(greeting)
				}
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}
