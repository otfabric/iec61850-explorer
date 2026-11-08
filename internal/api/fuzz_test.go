// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
)

// A request of any shape, from whoever reached the port: the wrong Host or a
// wrong token gets nothing but a refusal; with the right ones, a body is
// decoded or refused, and an error is always the API's JSON error, never a
// 500. The engine may connect nowhere, so that a request cannot leave the
// process.
func FuzzRequest(f *testing.F) {
	const good = "Bearer " + testToken
	for _, c := range []struct{ method, path, host, auth, rng, body string }{
		{"GET", "/api/v1/health", "127.0.0.1", good, "", ""},
		{"GET", "/api/v1/health", "evil.example", good, "", ""},
		{"GET", "/api/v1/health", "127.0.0.1", "Bearer nope", "", ""},
		{"GET", "/api/v1/health", "[::1]:80", "bearer  " + testToken, "", ""},
		{"POST", "/api/v1/sessions", "localhost:8765", good, "", `{"host":"10.0.0.1","port":102}`},
		{"POST", "/api/v1/sessions", "localhost", good, "", `{"host":"a/b","timeout_ms":-1}`},
		{"POST", "/api/v1/sessions", "localhost", good, "", `{"host":5}`},
		{"GET", "/api/v1/sessions/x/browse?ref=LD0%2FMMXU1", "localhost", good, "", ""},
		{"GET", "/api/v1/sessions/x/browse?ref=%5B%5B", "localhost", good, "", ""},
		{"POST", "/api/v1/sessions/x/read", "localhost", good, "", `{"refs":["LD0/MMXU1.Hz[MX]",""]}`},
		{"POST", "/api/v1/sessions/x/write", "localhost", good, "", `{}`},
		{"DELETE", "/api/v1/sessions/..%2f..%2fx", "localhost", good, "", ""},
		{"GET", "/api/v1", "localhost", good, "", ""},
		{"TRACE", "/x", "localhost", "", "", ""},
		{"GET", "/api/v1/health", "LOCALHOST:8765", good, "", ""},
		{"GET", "/api/v1/health", "::1", good, "", ""},
		{"GET", "/api/v1/health", "]localhost", good, "", ""},
		{"GET", "/api/v1/health", "[localhost]", good, "", ""},
		{"GET", "/api/v1/health", "localhost:[", good, "", ""},
	} {
		f.Add(c.method, c.path, c.host, c.auth, c.rng, []byte(c.body))
	}

	eng := engine.New(engine.Options{
		AllowTarget: func(context.Context, string) error { return errors.New("no target in this test") },
	})
	f.Cleanup(eng.Close)
	h := api.New(eng, api.Options{Token: func() string { return testToken }, Hosts: []string{"127.0.0.1", "localhost", "::1"}})

	f.Fuzz(func(t *testing.T, method, path, host, auth, rng string, body []byte) {
		// The events endpoint streams until the client leaves.
		if !strings.HasPrefix(path, "/") || strings.Contains(path, "events") {
			return
		}
		req, err := http.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(string(body)))
		if err != nil {
			return
		}
		req.Host = host
		req.Header.Set("Authorization", auth)
		req.Header.Set("Range", rng)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		status := rec.Code
		switch hostOK, tokenOK := hostAllowed(host), goodToken(auth); {
		case !hostOK && status != http.StatusForbidden:
			t.Fatalf("host %q got %d", host, status)
		case hostOK && !tokenOK && isAPI(req) && status != http.StatusUnauthorized:
			t.Fatalf("Authorization %q on %s got %d", auth, path, status)
		}
		if status >= 500 {
			t.Fatalf("%s %s: %d: %s", method, path, status, rec.Body)
		}
		if status >= 400 && isAPI(req) && hostAllowed(host) {
			var e struct {
				Error *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error == nil || e.Error.Code == "" {
				t.Fatalf("%s %s: %d is not an API error: %q", method, path, status, rec.Body)
			}
		}
	})
}

func isAPI(r *http.Request) bool {
	return r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/")
}

func goodToken(auth string) bool {
	scheme, token, ok := strings.Cut(auth, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && strings.TrimSpace(token) == testToken
}

// hostAllowed says what the handler must decide, written out from the grammar
// of a Host header and not from the handler's code: a host, then an optional
// port. An IPv6 literal goes between one pair of brackets, which nothing else
// may use. The one tolerance is ::1 without brackets.
func hostAllowed(hostport string) bool {
	host, port := strings.ToLower(hostport), ""
	if rest, ok := strings.CutPrefix(host, "["); ok {
		var closed bool
		if host, port, closed = strings.Cut(rest, "]"); !closed {
			return false
		}
		if port != "" && port[0] != ':' {
			return false
		}
		port = strings.TrimPrefix(port, ":")
		return host == "::1" && !strings.ContainsAny(port, ":[]")
	}
	if strings.Count(host, ":") == 1 {
		host, port, _ = strings.Cut(host, ":")
	}
	return (host == "127.0.0.1" || host == "localhost" || host == "::1") && !strings.ContainsAny(port, "[]")
}
