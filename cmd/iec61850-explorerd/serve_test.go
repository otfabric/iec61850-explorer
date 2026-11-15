// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

// setVar sets *p to v for the test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// A connection that waits longer than idleTimeout for its next request is
// closed by the daemon, also after a request without a token. A daemon that
// stops with nothing running reports no error.
func TestIdleConnectionClosed(t *testing.T) {
	setVar(t, &idleTimeout, 200*time.Millisecond)
	d := start(t, "--no-ui")
	conn, err := net.Dial("tcp", d.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprintf(conn, "GET /api/v1/health HTTP/1.1\r\nHost: %s\r\n\r\n", d.addr); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", res.StatusCode)
	}
	// The deadline only bounds the test; the server closes the connection
	// after the idle timeout, long before it.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("an idle connection reads %v, want EOF: the daemon keeps it open", err)
	}
	if err := d.stop(); err != nil {
		t.Errorf("stop: %v, want nil", err)
	}
}

// A request that is still running when the shutdown time is up is cut, and
// serve says so: the daemon then exits with a non-zero status.
func TestShutdownCutShort(t *testing.T) {
	setVar(t, &shutdownTimeout, 50*time.Millisecond)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release // does not end with its context
	})
	reqCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	log := slog.New(slog.DiscardHandler)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, newServer(reqCtx, h, log), ln, cancelRequests, log) }()
	go func() {
		if res, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
			_ = res.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the request did not arrive")
	}
	stop()
	select {
	case err := <-served:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("serve = %v, want an error for the cut shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return")
	}
}

// A connection that has sent no request when the shutdown time is up (a
// client may open one in advance) is closed, and serve reports nothing cut.
func TestShutdownUnusedConnection(t *testing.T) {
	setVar(t, &shutdownTimeout, 50*time.Millisecond)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	reqCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	log := slog.New(slog.DiscardHandler)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, newServer(reqCtx, h, log), ln, cancelRequests, log) }()
	unused, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unused.Close() }()
	// The server accepts in order: once a later connection is answered, the
	// unused one is accepted and waits for its first request.
	res, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	stop()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serve = %v, want nil: no request was cut", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return")
	}
}
