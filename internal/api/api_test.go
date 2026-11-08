// SPDX-License-Identifier: MIT

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
)

func TestAuthAndHost(t *testing.T) {
	d := newDaemon(t, engine.Options{}, api.Options{})
	for _, tc := range []struct {
		name, method, path, token string
		edit                      func(*http.Request)
		status                    int
		body                      string
	}{
		{"no token", "GET", "/api/v1/health", "", nil, http.StatusUnauthorized, `"unauthorized"`},
		{"wrong token", "GET", "/api/v1/health", "x", nil, http.StatusUnauthorized, `"unauthorized"`},
		{"token", "GET", "/api/v1/health", testToken, nil, http.StatusOK, `"mode":"observe"`},
		{"rebinding host", "GET", "/api/v1/health", testToken, func(r *http.Request) { r.Host = "evil.example:80" }, http.StatusForbidden, `"unauthorized"`},
		{"query token is ignored", "GET", "/api/v1/health?token=" + testToken, "", nil, http.StatusUnauthorized, ""},
		{"unknown endpoint", "GET", "/api/v1/nope", testToken, nil, http.StatusNotFound, `"not_found"`},
		{"no write endpoint", "POST", "/api/v1/sessions/x/write", testToken, nil, http.StatusNotFound, `"not_found"`},
		{"bad JSON", "POST", "/api/v1/sessions", testToken, nil, http.StatusBadRequest, `"invalid_argument"`},
		{"unknown session", "GET", "/api/v1/sessions/x", testToken, nil, http.StatusNotFound, `"not_found"`},
		{"browse unknown session", "GET", "/api/v1/sessions/x/browse?ref=a", testToken, nil, http.StatusNotFound, `"not_found"`},
	} {
		body := ""
		if tc.name == "bad JSON" {
			body = "{"
		}
		status, h, got := d.request(t, tc.method, tc.path, tc.token, body, tc.edit)
		if status != tc.status || !strings.Contains(got, tc.body) {
			t.Errorf("%s: %d %s, want %d with %s", tc.name, status, got, tc.status, tc.body)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: headers %v", tc.name, h)
		}
	}
}
