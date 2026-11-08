// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func errorCode(t testing.TB, body string) string {
	t.Helper()
	var v struct {
		Error service.Error `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("error body %q: %v", body, err)
	}
	return v.Error.Code
}

func collect(t testing.TB, ch <-chan model.Event, n int) []model.Event {
	t.Helper()
	var out []model.Event
	deadline := time.After(10 * time.Second)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("got %d of %d events", len(out), n)
		}
	}
	return out
}

func seqs(events []model.Event) []uint64 {
	out := make([]uint64, len(events))
	for i, ev := range events {
		out[i] = ev.Seq
	}
	return out
}

// closedPort returns a loopback target where nothing listens.
func closedPort(t testing.TB) model.TargetConfig {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := targetOf(t, ln.Addr().String())
	_ = ln.Close()
	return cfg
}

func TestHostCheck(t *testing.T) {
	ui := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ui") })
	d := newDaemon(t, engine.Options{}, api.Options{UI: ui})

	for _, host := range []string{
		"evil.example", "evil.example:80", "127.0.0.1.evil.example", "192.168.1.10:8765", "localhost.evil.example",
		"]localhost", "[localhost", "localhost]", "[localhost]", "[localhost]:8765", "[127.0.0.1]", "[[::1]]", "[::1", "::1]",
	} {
		for _, path := range []string{"/api/v1/health", "/"} {
			status, _, body := d.request(t, "GET", path, testToken, "", func(r *http.Request) { r.Host = host })
			if status != http.StatusForbidden || errorCode(t, body) != service.CodeUnauthorized {
				t.Errorf("Host %s, %s = %d %s", host, path, status, body)
			}
		}
	}
	// The port is free: an SSH forward may use another local port.
	for _, host := range []string{"127.0.0.1", "127.0.0.1:9999", "localhost:8765", "LOCALHOST", "[::1]", "[::1]:8765"} {
		if status, _, _ := d.request(t, "GET", "/api/v1/health", testToken, "", func(r *http.Request) { r.Host = host }); status != http.StatusOK {
			t.Errorf("Host %s = %d", host, status)
		}
		if status, _, body := d.request(t, "GET", "/", "", "", func(r *http.Request) { r.Host = host }); status != http.StatusOK || body != "ui" {
			t.Errorf("UI with Host %s = %d", host, status)
		}
	}
	bare := newDaemon(t, engine.Options{}, api.Options{})
	if status, _, _ := bare.request(t, "GET", "/", "", "", nil); status != http.StatusNotFound {
		t.Errorf("/ without UI = %d", status)
	}
}

// The status of every error code, and a session found by ID.
func TestStatusCodes(t *testing.T) {
	d := newDaemon(t, engine.Options{MaxSessions: 1}, api.Options{})
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)

	body := func(v any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	status, _, got := d.request(t, "POST", "/api/v1/sessions", testToken, body(closedPort(t)), nil)
	if status != http.StatusBadGateway || errorCode(t, got) != service.CodeConnectFailed || !strings.Contains(got, `"stage":"tcp"`) {
		t.Errorf("closed port = %d %s", status, got)
	}
	status, _, got = d.request(t, "POST", "/api/v1/sessions", testToken, body(targetOf(t, ied.Addr)), nil)
	if status != http.StatusCreated {
		t.Fatalf("open = %d %s", status, got)
	}
	var info model.SessionInfo
	_ = json.Unmarshal([]byte(got), &info)
	if status, _, _ := d.request(t, "POST", "/api/v1/sessions", testToken, body(targetOf(t, ied.Addr)), nil); status != http.StatusTooManyRequests {
		t.Errorf("second session = %d", status)
	}
	if status, _, got := d.request(t, "GET", "/api/v1/sessions/"+info.ID, testToken, "", nil); status != http.StatusOK || !strings.Contains(got, info.ID) {
		t.Errorf("GET session = %d %s", status, got)
	}
	if status, _, got := d.request(t, "POST", "/api/v1/sessions/"+info.ID+"/read", testToken, "[", nil); status != http.StatusBadRequest {
		t.Errorf("read bad JSON = %d %s", status, got)
	}
	if status, _, _ := d.request(t, "POST", "/api/v1/sessions/"+info.ID+"/read", testToken, strings.Repeat("x", 70<<10), nil); status != http.StatusBadRequest {
		t.Errorf("read huge body = %d", status)
	}
	ied.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, _, got = d.request(t, "POST", "/api/v1/sessions/"+info.ID+"/read", testToken, body(model.ReadRequest{Refs: []string{"a/b.c[MX]"}}), nil)
		if status == http.StatusConflict || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != http.StatusConflict || errorCode(t, got) != service.CodeSessionLost {
		t.Errorf("read on lost session = %d %s", status, got)
	}
	if status, _, _ := d.request(t, "DELETE", "/api/v1/sessions/"+info.ID, testToken, "", nil); status != http.StatusNoContent {
		t.Errorf("close = %d", status)
	}
	if status, _, _ := d.request(t, "GET", "/api/v1/events?since=x", testToken, "", nil); status != http.StatusBadRequest {
		t.Errorf("events since=x = %d", status)
	}
}

// The tunnel drops and comes back: the stream resumes after the last event
// seen, without a loss and without a duplicate; when the buffer wrapped in
// between, the gap is reported.
func TestEventsResume(t *testing.T) {
	d := newDaemon(t, engine.Options{EventBuffer: 6}, api.Options{})
	closed := closedPort(t)
	// A failed connect makes exactly one event, its operation.
	cycle := func() {
		if _, err := d.svc.OpenSession(ctx, closed); service.CodeOf(err) != service.CodeConnectFailed {
			t.Fatalf("OpenSession = %v", err)
		}
	}

	first, drop := context.WithCancel(ctx)
	live, err := d.svc.Subscribe(first, 0)
	if err != nil {
		t.Fatal(err)
	}
	cycle()
	cycle()
	if got := collect(t, live, 2); !slices.Equal(seqs(got), []uint64{1, 2}) {
		t.Fatalf("live events = %v", seqs(got))
	}
	drop()

	cycle()
	cycle()
	again, stop := context.WithCancel(ctx)
	resumed, err := d.svc.Subscribe(again, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := collect(t, resumed, 2); !slices.Equal(seqs(got), []uint64{3, 4}) || got[0].Kind != model.EventOperation {
		t.Fatalf("resumed = %v, want exactly 3 and 4", seqs(got))
	}
	stop()

	for range 8 {
		cycle()
	}
	late, stopLate := context.WithCancel(ctx)
	defer stopLate()
	wrapped, err := d.svc.Subscribe(late, 4)
	if err != nil {
		t.Fatal(err)
	}
	got := collect(t, wrapped, 7)
	if got[0].Kind != model.EventResync || !slices.Equal(seqs(got[1:]), []uint64{7, 8, 9, 10, 11, 12}) {
		t.Errorf("after the buffer wrapped = %v (first %s)", seqs(got), got[0].Kind)
	}

	// Last-Event-ID does what ?since= does; the frames carry id and event.
	req, _ := http.NewRequest("GET", d.srv.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Last-Event-ID", "11")
	res, err := d.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("stream headers = %v", res.Header)
	}
	frames := make(chan [3]string, 1)
	go readSSE(res.Body, func(id, event, data string) {
		select {
		case frames <- [3]string{id, event, data}:
		default:
		}
	})
	select {
	case f := <-frames:
		if f[0] != "12" || f[1] != model.EventOperation || !strings.Contains(f[2], `"seq":12`) {
			t.Errorf("frame = %v", f)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no frame")
	}
}

// A rotated token ends the streams that were opened with the old one.
func TestEventsEndWhenTokenRotates(t *testing.T) {
	var current atomic.Value
	current.Store(testToken)
	d := newDaemon(t, engine.Options{}, api.Options{
		Token:     func() string { return current.Load().(string) },
		KeepAlive: 20 * time.Millisecond,
	})
	events, err := d.svc.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-events:
		if !ok {
			t.Fatal("stream closed with a valid token")
		}
	case <-time.After(100 * time.Millisecond):
	}
	current.Store("another-token-0123456789abcdefghijklmnop")
	select {
	case _, ok := <-events:
		if ok {
			t.Error("event after rotation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream still open after the token was rotated")
	}
	if _, err := d.svc.Health(ctx); service.CodeOf(err) != service.CodeUnauthorized {
		t.Errorf("old token after rotation = %v", err)
	}
}

// failing is a Service whose every call fails with err.
type failing struct {
	service.Service
	err error
}

func (f failing) Health(context.Context) (model.Health, error)          { return model.Health{}, f.err }
func (f failing) Sessions(context.Context) ([]model.SessionInfo, error) { return nil, f.err }

// An error that is not a service.Error is logged, not passed on.
func TestInternalErrors(t *testing.T) {
	for err, want := range map[error]string{
		errors.New("secret detail"): service.CodeInternal,
		context.Canceled:            service.CodeCanceled,
	} {
		srv := api.New(failing{err: err}, api.Options{Token: func() string { return testToken }, Hosts: []string{"127.0.0.1"}})
		for _, path := range []string{"/api/v1/health", "/api/v1/sessions/x"} {
			req, _ := http.NewRequest("GET", "http://127.0.0.1"+path, nil)
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := &recorder{header: http.Header{}}
			srv.ServeHTTP(rec, req)
			if code := errorCode(t, rec.body.String()); code != want || strings.Contains(rec.body.String(), "secret") {
				t.Errorf("%v %s = %d %s", err, path, rec.status, rec.body.String())
			}
		}
	}
}

type recorder struct {
	header http.Header
	status int
	body   strings.Builder
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(status int)      { r.status = status }
