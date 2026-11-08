// SPDX-License-Identifier: MIT

package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

const testToken = "test-token-0123456789abcdefghijklmnopqrstuv"

// httpService presents the HTTP API as a service.Service, with plain
// net/http requests. It exists for the tests only: the programs have no Go
// client of the daemon, a browser is its client. Running the contract tests
// through it proves that the handlers carry the whole interface.
type httpService struct {
	base  string
	token string
	http  *http.Client
}

var _ service.Service = (*httpService)(nil)

// do sends a request and decodes the answer into out, or returns the
// *service.Error of an error answer.
func (c *httpService) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 400 {
		return decodeError(res)
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func decodeError(res *http.Response) error {
	var body struct {
		Error *service.Error `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Error == nil {
		return fmt.Errorf("HTTP %d without an error body", res.StatusCode)
	}
	return body.Error
}

func (c *httpService) Health(ctx context.Context) (v model.Health, err error) {
	return v, c.do(ctx, "GET", "/api/v1/health", nil, &v)
}

func (c *httpService) OpenSession(ctx context.Context, cfg model.TargetConfig) (v model.SessionInfo, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions", cfg, &v)
}

func (c *httpService) CloseSession(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/sessions/"+id, nil, nil)
}

func (c *httpService) Sessions(ctx context.Context) (v []model.SessionInfo, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions", nil, &v)
}

func (c *httpService) Browse(ctx context.Context, id, ref string) (v []model.Node, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/browse?ref="+url.QueryEscape(ref), nil, &v)
}

func (c *httpService) Read(ctx context.Context, id string, req model.ReadRequest) (v model.ReadResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/read", req, &v)
}

func (c *httpService) StartPolling(ctx context.Context, id string, cfg model.PollConfig) error {
	return c.do(ctx, "PUT", "/api/v1/sessions/"+id+"/polling", cfg, nil)
}

func (c *httpService) StopPolling(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/sessions/"+id+"/polling", nil, nil)
}

func (c *httpService) Search(ctx context.Context, id string, q model.SearchQuery) (v model.SearchResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/search", q, &v)
}

func (c *httpService) Profiles(ctx context.Context) (v []model.Profile, err error) {
	return v, c.do(ctx, "GET", "/api/v1/profiles", nil, &v)
}

func (c *httpService) SaveProfile(ctx context.Context, p model.Profile) (v model.Profile, err error) {
	if p.ID == "" {
		return v, c.do(ctx, "POST", "/api/v1/profiles", p, &v)
	}
	return v, c.do(ctx, "PUT", "/api/v1/profiles/"+url.PathEscape(p.ID), p, &v)
}

func (c *httpService) DeleteProfile(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/profiles/"+url.PathEscape(id), nil, nil)
}

// Subscribe reads the event stream until ctx ends or the server closes it.
func (c *httpService) Subscribe(ctx context.Context, sinceSeq uint64) (<-chan model.Event, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/api/v1/events?since="+strconv.FormatUint(sinceSeq, 10), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer func() { _ = res.Body.Close() }()
		return nil, decodeError(res)
	}
	out := make(chan model.Event, 64)
	go func() {
		defer close(out)
		defer func() { _ = res.Body.Close() }()
		readSSE(res.Body, func(_, _, data string) {
			var ev model.Event
			if json.Unmarshal([]byte(data), &ev) == nil {
				select {
				case out <- ev:
				case <-ctx.Done():
				}
			}
		})
	}()
	return out, nil
}

// readSSE calls frame for every server-sent event with data.
func readSSE(r io.Reader, frame func(id, event, data string)) {
	var id, event, data string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if data != "" {
				frame(id, event, data)
			}
			id, event, data = "", "", ""
		case strings.HasPrefix(line, "id: "):
			id = line[4:]
		case strings.HasPrefix(line, "event: "):
			event = line[7:]
		case strings.HasPrefix(line, "data: "):
			data += line[6:]
		}
	}
}

// daemon is an engine behind the HTTP API on an httptest server.
type daemon struct {
	eng *engine.Engine
	srv *httptest.Server
	svc *httpService
}

func newDaemon(t testing.TB, eopts engine.Options, aopts api.Options) *daemon {
	t.Helper()
	eng := engine.New(eopts)
	if aopts.Token == nil {
		aopts.Token = func() string { return testToken }
	}
	if aopts.Hosts == nil {
		aopts.Hosts = []string{"127.0.0.1", "localhost", "::1"}
	}
	srv := httptest.NewServer(api.New(eng, aopts))
	// Streams must end before the server can close.
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
		eng.Close()
	})
	return &daemon{eng: eng, srv: srv, svc: &httpService{base: srv.URL, token: testToken, http: srv.Client()}}
}

// request sends a plain request and returns status, headers and body.
func (d *daemon) request(t testing.TB, method, path, token string, body string, edit func(*http.Request)) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, d.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if edit != nil {
		edit(req)
	}
	res, err := d.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}
