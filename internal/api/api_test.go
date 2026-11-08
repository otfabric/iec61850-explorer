// SPDX-License-Identifier: MIT

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
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

func TestM1RequestErrors(t *testing.T) {
	d := newDaemon(t, engine.Options{DataDir: t.TempDir()}, api.Options{})
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"profile with id on POST", "POST", "/api/v1/profiles", `{"id":"x","name":"a","target":{"host":"h"}}`, http.StatusBadRequest},
		{"profile bad JSON", "POST", "/api/v1/profiles", `{`, http.StatusBadRequest},
		{"replace bad JSON", "PUT", "/api/v1/profiles/x", `{`, http.StatusBadRequest},
		{"poll bad JSON", "PUT", "/api/v1/sessions/x/polling", `{`, http.StatusBadRequest},
		{"poll unknown", "PUT", "/api/v1/sessions/x/polling", `{"refs":["a"]}`, http.StatusNotFound},
		{"stop unknown", "DELETE", "/api/v1/sessions/x/polling", ``, http.StatusNotFound},
		{"search bad JSON", "POST", "/api/v1/sessions/x/search", `{`, http.StatusBadRequest},
		{"export bad JSON", "POST", "/api/v1/sessions/x/export", `{`, http.StatusBadRequest},
		{"empty snapshot", "POST", "/api/v1/snapshots/open?name=x.json", ``, http.StatusBadRequest},
		{"empty SCL", "POST", "/api/v1/scl/open?name=x.scd", ``, http.StatusBadRequest},
	} {
		if status, _, body := d.request(t, tc.method, tc.path, testToken, tc.body, nil); status != tc.status {
			t.Errorf("%s: %d %s", tc.name, status, body)
		}
	}
	huge := strings.Repeat("x", int(model.MaxSnapshotBytes)+1)
	if status, _, _ := d.request(t, "POST", "/api/v1/snapshots/open", testToken, huge, nil); status != http.StatusTooManyRequests {
		t.Errorf("huge snapshot = %d", status)
	}
}
