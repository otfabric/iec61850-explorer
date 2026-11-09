// SPDX-License-Identifier: MIT

// Package api serves a service.Service over HTTP: JSON requests under
// /api/v1, the event stream as server-sent events, and, next to them, the
// web UI. It is what a browser talks to; there is no Go client.
//
// Every request must name a host the daemon answers to, and every /api/v1
// request must carry the bearer token, also on loopback: other local users
// and other pages in the user's browser can reach a loopback port too.
//
// No endpoint accepts a file path, an MMS PDU, a service name or a value to
// write.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

const (
	prefix = "/api/v1"
	// maxBody bounds a request body; the largest real one is a read of
	// model.MaxReadRefs references.
	maxBody = 64 << 10
	// DefaultKeepAlive is the interval of the comment lines that keep an idle
	// event stream open through proxies and tunnels.
	DefaultKeepAlive = 15 * time.Second
)

// Options configures the handler.
type Options struct {
	// Token returns the bearer token that is valid now. It is called for
	// every request, so that a rotated token takes effect at once.
	Token func() string
	// Hosts lists the host names (without port) the daemon answers to, such
	// as "127.0.0.1" and "localhost". A request with another Host header is
	// refused: that is what stops a hostile web page from reaching the daemon
	// through a DNS name it points at 127.0.0.1. The port is not compared: an
	// SSH forward may use another local port.
	Hosts []string
	// UI serves everything outside /api/v1. Nil answers 404 there.
	UI http.Handler
	// KeepAlive overrides DefaultKeepAlive.
	KeepAlive time.Duration
	// Logger receives request errors. Nil discards them.
	Logger *slog.Logger
}

type server struct {
	svc   service.Service
	opts  Options
	hosts map[string]bool
	log   *slog.Logger
}

// New returns the handler of the daemon.
func New(svc service.Service, opts Options) http.Handler {
	s := &server{svc: svc, opts: opts, hosts: make(map[string]bool), log: opts.Logger}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.opts.KeepAlive <= 0 {
		s.opts.KeepAlive = DefaultKeepAlive
	}
	for _, h := range opts.Hosts {
		s.hosts[strings.ToLower(h)] = true
	}

	api := http.NewServeMux()
	api.HandleFunc("GET "+prefix+"/health", s.health)
	api.HandleFunc("GET "+prefix+"/sessions", s.sessions)
	api.HandleFunc("POST "+prefix+"/sessions", s.openSession)
	api.HandleFunc("GET "+prefix+"/sessions/{id}", s.session)
	api.HandleFunc("DELETE "+prefix+"/sessions/{id}", s.closeSession)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/browse", s.browse)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/read", s.read)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/search", s.search)
	api.HandleFunc("PUT "+prefix+"/sessions/{id}/polling", s.startPolling)
	api.HandleFunc("DELETE "+prefix+"/sessions/{id}/polling", s.stopPolling)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/export", s.export)
	api.HandleFunc("POST "+prefix+"/snapshots/open", s.openSnapshot)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/datasets", s.dataSets)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/datasets/read", s.readDataSet)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/reports", s.reports)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/files", s.files)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/files/download", s.downloadFile)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/journals", s.journals)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/journals/read", s.readJournal)
	api.HandleFunc("GET "+prefix+"/sessions/{id}/setting-groups", s.settingGroups)
	api.HandleFunc("PUT "+prefix+"/sessions/{id}/commission", s.grantCommission)
	api.HandleFunc("DELETE "+prefix+"/sessions/{id}/commission", s.revokeCommission)
	api.HandleFunc("POST "+prefix+"/sessions/{id}/write", s.write)
	api.HandleFunc("GET "+prefix+"/audit", s.audit)
	api.HandleFunc("POST "+prefix+"/scl/inspect", s.inspectSCL)
	api.HandleFunc("POST "+prefix+"/scl/open", s.openSCL)
	api.HandleFunc("GET "+prefix+"/profiles", s.profiles)
	api.HandleFunc("POST "+prefix+"/profiles", s.createProfile)
	api.HandleFunc("PUT "+prefix+"/profiles/{id}", s.replaceProfile)
	api.HandleFunc("DELETE "+prefix+"/profiles/{id}", s.deleteProfile)
	api.HandleFunc("GET "+prefix+"/events", s.events)
	api.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, service.Errorf(service.CodeNotFound, "no such endpoint: %s %s", r.Method, r.URL.Path))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")

		if !s.hostAllowed(r.Host) {
			writeError(w, http.StatusForbidden, service.Errorf(service.CodeUnauthorized,
				"this daemon does not answer to the host name in the request"))
			return
		}
		if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
			if s.opts.UI == nil {
				http.NotFound(w, r)
				return
			}
			s.opts.UI.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-store")
		if !s.authorized(r) {
			h.Set("WWW-Authenticate", `Bearer realm="iec61850-explorerd"`)
			writeError(w, http.StatusUnauthorized, service.Errorf(service.CodeUnauthorized, "missing or wrong bearer token"))
			return
		}
		api.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether a Host header names a host the daemon answers
// to. Brackets belong around an IPv6 literal and nowhere else, as a pair: a
// header that is not well formed names no host.
func (s *server) hostAllowed(hostport string) bool {
	host, bracketed := hostport, strings.HasPrefix(hostport, "[")
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	} else if bracketed && strings.HasSuffix(hostport, "]") {
		host = hostport[1 : len(hostport)-1]
	}
	if bracketed && !strings.Contains(host, ":") {
		return false
	}
	return s.hosts[strings.ToLower(host)]
}

// authorized compares the bearer token in constant time. The token is only
// taken from the Authorization header: never from the URL, where it would
// end up in logs and histories, and never from a cookie.
func (s *server) authorized(r *http.Request) bool {
	return s.tokenValid(bearer(r))
}

func (s *server) tokenValid(got string) bool {
	want := s.opts.Token()
	return want != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearer(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Health(r.Context())
	s.reply(w, r, v, err)
}

func (s *server) sessions(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Sessions(r.Context())
	s.reply(w, r, v, err)
}

func (s *server) openSession(w http.ResponseWriter, r *http.Request) {
	var cfg model.TargetConfig
	if !s.decode(w, r, &cfg) {
		return
	}
	v, err := s.svc.OpenSession(r.Context(), cfg)
	s.replyStatus(w, r, http.StatusCreated, v, err)
}

func (s *server) session(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Sessions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id := r.PathValue("id")
	i := slices.IndexFunc(list, func(si model.SessionInfo) bool { return si.ID == id })
	if i < 0 {
		s.fail(w, r, service.Errorf(service.CodeNotFound, "no session %q", id))
		return
	}
	writeJSON(w, http.StatusOK, list[i])
}

func (s *server) closeSession(w http.ResponseWriter, r *http.Request) {
	s.replyEmpty(w, r, s.svc.CloseSession(r.Context(), r.PathValue("id")))
}

// browse lists the children of ?ref= (the logical devices without it).
func (s *server) browse(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Browse(r.Context(), r.PathValue("id"), r.URL.Query().Get("ref"))
	s.reply(w, r, v, err)
}

func (s *server) read(w http.ResponseWriter, r *http.Request) {
	var req model.ReadRequest
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.Read(r.Context(), r.PathValue("id"), req)
	s.reply(w, r, v, err)
}

func (s *server) startPolling(w http.ResponseWriter, r *http.Request) {
	var cfg model.PollConfig
	if !s.decode(w, r, &cfg) {
		return
	}
	s.replyEmpty(w, r, s.svc.StartPolling(r.Context(), r.PathValue("id"), cfg))
}

func (s *server) stopPolling(w http.ResponseWriter, r *http.Request) {
	s.replyEmpty(w, r, s.svc.StopPolling(r.Context(), r.PathValue("id")))
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	var q model.SearchQuery
	if !s.decode(w, r, &q) {
		return
	}
	v, err := s.svc.Search(r.Context(), r.PathValue("id"), q)
	s.reply(w, r, v, err)
}

// export renders a session. The client names a format, never a file: it
// gets the text back and saves it itself.
func (s *server) export(w http.ResponseWriter, r *http.Request) {
	var req model.ExportRequest
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.Export(r.Context(), r.PathValue("id"), req)
	s.reply(w, r, v, err)
}

func (s *server) dataSets(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.DataSets(r.Context(), r.PathValue("id"), r.URL.Query().Get("ld"))
	s.reply(w, r, v, err)
}

func (s *server) readDataSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref string `json:"ref"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.ReadDataSet(r.Context(), r.PathValue("id"), req.Ref)
	s.reply(w, r, v, err)
}

func (s *server) reports(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Reports(r.Context(), r.PathValue("id"), r.URL.Query().Get("ld"))
	s.reply(w, r, v, err)
}

func (s *server) files(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Files(r.Context(), r.PathValue("id"), r.URL.Query().Get("dir"))
	s.reply(w, r, v, err)
}

// downloadFile streams a file of the IED. The name is the IED's; what the
// browser saves it as is its base name, cleaned.
func (s *server) downloadFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.svc.ReadFile(r.Context(), r.PathValue("id"), r.URL.Query().Get("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName(f.Name)))
	h.Set("Content-Length", strconv.Itoa(len(f.Data)))
	_, _ = w.Write(f.Data)
}

// downloadName is the base name of a file of the IED, with anything but
// letters, digits, dot, dash and underscore replaced.
func downloadName(name string) string {
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	out := []rune{}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if s := strings.Trim(string(out), "."); s != "" {
		return s
	}
	return "file"
}

func (s *server) journals(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Journals(r.Context(), r.PathValue("id"), r.URL.Query().Get("ld"))
	s.reply(w, r, v, err)
}

func (s *server) readJournal(w http.ResponseWriter, r *http.Request) {
	var req model.JournalRequest
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.ReadJournal(r.Context(), r.PathValue("id"), req)
	s.reply(w, r, v, err)
}

func (s *server) settingGroups(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.SettingGroups(r.Context(), r.PathValue("id"), r.URL.Query().Get("ld"))
	s.reply(w, r, v, err)
}

func (s *server) grantCommission(w http.ResponseWriter, r *http.Request) {
	var req model.GrantRequest
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.GrantCommission(r.Context(), r.PathValue("id"), req)
	s.reply(w, r, v, err)
}

func (s *server) revokeCommission(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.RevokeCommission(r.Context(), r.PathValue("id"))
	s.reply(w, r, v, err)
}

func (s *server) write(w http.ResponseWriter, r *http.Request) {
	var req model.WriteRequest
	if !s.decode(w, r, &req) {
		return
	}
	v, err := s.svc.Write(r.Context(), r.PathValue("id"), req)
	s.reply(w, r, v, err)
}

func (s *server) audit(w http.ResponseWriter, r *http.Request) {
	q := model.AuditQuery{SessionID: r.URL.Query().Get("session")}
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, service.Errorf(service.CodeInvalidArgument, "limit %q is not a count", l))
			return
		}
		q.Limit = n
	}
	v, err := s.svc.Audit(r.Context(), q)
	s.reply(w, r, v, err)
}

func (s *server) openSnapshot(w http.ResponseWriter, r *http.Request) {
	f, ok := s.rawFile(w, r, model.MaxSnapshotBytes)
	if !ok {
		return
	}
	v, err := s.svc.OpenSnapshot(r.Context(), f)
	s.replyStatus(w, r, http.StatusCreated, v, err)
}

// sclFile reads an SCL document sent as the raw request body; ?name= names
// it for display.
func (s *server) sclFile(w http.ResponseWriter, r *http.Request) (model.File, bool) {
	return s.rawFile(w, r, model.MaxSCLBytes)
}

// rawFile reads a file sent as the raw request body, up to limit bytes.
func (s *server) rawFile(w http.ResponseWriter, r *http.Request, limit int64) (model.File, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		s.fail(w, r, service.Errorf(service.CodeLimitExceeded, "the file is larger than %d MiB or unreadable", limit>>20))
		return model.File{}, false
	}
	return model.File{Name: r.URL.Query().Get("name"), Data: data}, true
}

func (s *server) inspectSCL(w http.ResponseWriter, r *http.Request) {
	f, ok := s.sclFile(w, r)
	if !ok {
		return
	}
	v, err := s.svc.InspectSCL(r.Context(), f)
	s.reply(w, r, v, err)
}

func (s *server) openSCL(w http.ResponseWriter, r *http.Request) {
	f, ok := s.sclFile(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	v, err := s.svc.OpenSCL(r.Context(), model.OpenSCLRequest{File: f, IED: q.Get("ied"), AccessPoint: q.Get("ap")})
	s.replyStatus(w, r, http.StatusCreated, v, err)
}

func (s *server) profiles(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Profiles(r.Context())
	s.reply(w, r, v, err)
}

func (s *server) createProfile(w http.ResponseWriter, r *http.Request) {
	var p model.Profile
	if !s.decode(w, r, &p) {
		return
	}
	if p.ID != "" {
		s.fail(w, r, service.Errorf(service.CodeInvalidArgument, "a new profile has no id; use PUT /profiles/{id} to replace one"))
		return
	}
	v, err := s.svc.SaveProfile(r.Context(), p)
	s.replyStatus(w, r, http.StatusCreated, v, err)
}

func (s *server) replaceProfile(w http.ResponseWriter, r *http.Request) {
	var p model.Profile
	if !s.decode(w, r, &p) {
		return
	}
	p.ID = r.PathValue("id")
	v, err := s.svc.SaveProfile(r.Context(), p)
	s.reply(w, r, v, err)
}

func (s *server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	s.replyEmpty(w, r, s.svc.DeleteProfile(r.Context(), r.PathValue("id")))
}

// events streams the events after ?since= (or Last-Event-ID) as server-sent
// events until the client goes away or its token stops being valid.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, service.Errorf(service.CodeInternal, "streaming is not supported"))
		return
	}
	raw := r.URL.Query().Get("since")
	if raw == "" {
		raw = r.Header.Get("Last-Event-ID")
	}
	var since uint64
	if raw != "" {
		var err error
		if since, err = strconv.ParseUint(raw, 10, 64); err != nil {
			s.fail(w, r, service.Errorf(service.CodeInvalidArgument, "since must be a sequence number"))
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	events, err := s.svc.Subscribe(ctx, since)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	token := bearer(r)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(s.opts.KeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				s.log.Error("event not encoded", "kind", ev.Kind, "error", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Kind, data); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			// A stream opened with a token that was rotated away ends here.
			if !s.tokenValid(token) {
				return
			}
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// decode reads a JSON request body into v. An empty body leaves v as it is.
func (s *server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		s.fail(w, r, service.Errorf(service.CodeInvalidArgument, "request body is too large or unreadable"))
		return false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return true
	}
	if err := json.Unmarshal(body, v); err != nil {
		s.fail(w, r, service.Errorf(service.CodeInvalidArgument, "request body is not valid JSON for this endpoint"))
		return false
	}
	return true
}

func (s *server) reply(w http.ResponseWriter, r *http.Request, v any, err error) {
	s.replyStatus(w, r, http.StatusOK, v, err)
}

func (s *server) replyStatus(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *server) replyEmpty(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail answers with the error as JSON. The message of a service.Error is
// written for the user; anything else is logged and not passed on.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var se *service.Error
	switch {
	case errors.As(err, &se):
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		se = service.Errorf(service.CodeCanceled, "the request was canceled")
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		se = service.Errorf(service.CodeInternal, "internal error")
	}
	writeError(w, statusOf(se.Code), se)
}

func statusOf(code string) int {
	switch code {
	case service.CodeInvalidArgument:
		return http.StatusBadRequest
	case service.CodeUnauthorized:
		return http.StatusUnauthorized
	case service.CodeNotAllowed:
		return http.StatusForbidden
	case service.CodeNotFound:
		return http.StatusNotFound
	case service.CodeLimitExceeded:
		return http.StatusTooManyRequests
	case service.CodeConnectFailed, service.CodeDiscoveryFailed, service.CodeIEDError:
		return http.StatusBadGateway
	case service.CodeSessionLost:
		return http.StatusConflict
	case service.CodeCanceled:
		return http.StatusRequestTimeout
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, status int, e *service.Error) {
	writeJSON(w, status, struct {
		Error *service.Error `json:"error"`
	}{e})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
