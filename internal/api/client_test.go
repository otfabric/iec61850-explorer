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

// sclRequest posts an SCL document as the raw body.
func (c *httpService) sclRequest(ctx context.Context, path string, f model.File, query url.Values, out any) error {
	query.Set("name", f.Name)
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+path+"?"+query.Encode(), bytes.NewReader(f.Data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/xml")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 400 {
		return decodeError(res)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *httpService) Export(ctx context.Context, id string, req model.ExportRequest) (v model.ExportFile, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/export", req, &v)
}

func (c *httpService) OpenSnapshot(ctx context.Context, f model.File) (v model.SessionInfo, err error) {
	return v, c.sclRequest(ctx, "/api/v1/snapshots/open", f, url.Values{}, &v)
}

func (c *httpService) DataSets(ctx context.Context, id, ld string) (v []model.DataSet, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/datasets?ld="+url.QueryEscape(ld), nil, &v)
}

func (c *httpService) ReadDataSet(ctx context.Context, id, ref string) (v model.ReadResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/datasets/read", map[string]string{"ref": ref}, &v)
}

func (c *httpService) Compare(ctx context.Context, req model.CompareRequest) (v model.Comparison, err error) {
	return v, c.do(ctx, "POST", "/api/v1/compare", req, &v)
}

func (c *httpService) Semantics(ctx context.Context, id, ref string) (v model.Semantics, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/semantics?ref="+url.QueryEscape(ref), nil, &v)
}

func (c *httpService) Conformance(ctx context.Context, id string) (v model.Conformance, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/conformance", nil, &v)
}

func (c *httpService) Namespaces(ctx context.Context) (v model.Namespaces, err error) {
	return v, c.do(ctx, "GET", "/api/v1/namespaces", nil, &v)
}

func (c *httpService) AddNamespaces(ctx context.Context, f model.File) (v model.NamespacePackage, err error) {
	return v, c.sclRequest(ctx, "/api/v1/namespaces", f, url.Values{}, &v)
}

func (c *httpService) RemoveNamespaces(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/namespaces/"+url.PathEscape(id), nil, nil)
}

func (c *httpService) Reports(ctx context.Context, id, ld string) (v []model.RCB, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/reports?ld="+url.QueryEscape(ld), nil, &v)
}

func (c *httpService) Files(ctx context.Context, id, dir string) (v []model.FileEntry, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/files?dir="+url.QueryEscape(dir), nil, &v)
}

func (c *httpService) ReadFile(ctx context.Context, id, name string) (model.FileContent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/api/v1/sessions/"+id+"/files/download?name="+url.QueryEscape(name), nil)
	if err != nil {
		return model.FileContent{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return model.FileContent{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 400 {
		return model.FileContent{}, decodeError(res)
	}
	data, err := io.ReadAll(res.Body)
	return model.FileContent{Name: name, Size: int64(len(data)), Data: data}, err
}

func (c *httpService) Journals(ctx context.Context, id, ld string) (v []string, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/journals?ld="+url.QueryEscape(ld), nil, &v)
}

func (c *httpService) ReadJournal(ctx context.Context, id string, r model.JournalRequest) (v model.JournalResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/journals/read", r, &v)
}

func (c *httpService) SettingGroups(ctx context.Context, id, ld string) (v model.SettingGroups, err error) {
	return v, c.do(ctx, "GET", "/api/v1/sessions/"+id+"/setting-groups?ld="+url.QueryEscape(ld), nil, &v)
}

func (c *httpService) GrantCommission(ctx context.Context, id string, r model.GrantRequest) (v model.SessionInfo, err error) {
	return v, c.do(ctx, "PUT", "/api/v1/sessions/"+id+"/commission", r, &v)
}

func (c *httpService) RevokeCommission(ctx context.Context, id string) (v model.SessionInfo, err error) {
	return v, c.do(ctx, "DELETE", "/api/v1/sessions/"+id+"/commission", nil, &v)
}

func (c *httpService) Write(ctx context.Context, id string, r model.WriteRequest) (v model.WriteResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/write", r, &v)
}

func (c *httpService) Control(ctx context.Context, id string, r model.ControlRequest) (v model.ControlResult, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/control", r, &v)
}

func (c *httpService) SubscribeReport(ctx context.Context, id string, r model.SubscribeRequest) (v model.Subscription, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/reports/subscriptions", r, &v)
}

func (c *httpService) Unsubscribe(ctx context.Context, id, sub string) error {
	return c.do(ctx, "DELETE", "/api/v1/sessions/"+id+"/reports/subscriptions/"+url.PathEscape(sub), nil, nil)
}

func (c *httpService) TriggerGI(ctx context.Context, id, sub string) error {
	return c.do(ctx, "POST", "/api/v1/sessions/"+id+"/reports/subscriptions/"+url.PathEscape(sub)+"/gi", nil, nil)
}

func (c *httpService) CreateDataSet(ctx context.Context, id string, r model.CreateDataSetRequest) (v model.DataSet, err error) {
	return v, c.do(ctx, "POST", "/api/v1/sessions/"+id+"/datasets", r, &v)
}

func (c *httpService) DeleteDataSet(ctx context.Context, id, ref string) error {
	return c.do(ctx, "DELETE", "/api/v1/sessions/"+id+"/datasets?ref="+url.QueryEscape(ref), nil, nil)
}

func (c *httpService) SelectActiveSG(ctx context.Context, id string, r model.SelectSGRequest) (v model.SettingGroups, err error) {
	return v, c.do(ctx, "PUT", "/api/v1/sessions/"+id+"/setting-groups/active", r, &v)
}

func (c *httpService) Audit(ctx context.Context, q model.AuditQuery) (v []model.AuditRecord, err error) {
	return v, c.do(ctx, "GET", "/api/v1/audit?session="+url.QueryEscape(q.SessionID)+"&limit="+strconv.Itoa(q.Limit), nil, &v)
}

func (c *httpService) InspectSCL(ctx context.Context, f model.File) (v model.SCLInfo, err error) {
	return v, c.sclRequest(ctx, "/api/v1/scl/inspect", f, url.Values{}, &v)
}

func (c *httpService) OpenSCL(ctx context.Context, r model.OpenSCLRequest) (v model.SessionInfo, err error) {
	return v, c.sclRequest(ctx, "/api/v1/scl/open", r.File, url.Values{"ied": {r.IED}, "ap": {r.AccessPoint}}, &v)
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
