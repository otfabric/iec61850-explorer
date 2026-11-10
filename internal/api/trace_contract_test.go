// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func traceFile(t *testing.T, name string) model.File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "traces", name))
	if err != nil {
		t.Fatal(err)
	}
	return model.File{Name: name, Data: b}
}

// TestTraceContract: trace sessions, their views, the correlation with a
// model session and the wire view behave the same through the engine and
// through the HTTP API.
func TestTraceContract(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{})
		t.Cleanup(eng.Close)
		traceContract(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		traceContract(t, newDaemon(t, engine.Options{}, api.Options{}).svc)
	})
}

func traceContract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)

	tr, err := svc.OpenTrace(ctx, model.OpenTraceRequest{File: traceFile(t, "substation.pcap")})
	if err != nil || tr.Kind != model.SessionKindTrace || tr.State != model.SessionOffline || tr.Trace == nil ||
		tr.Trace.File != "substation.pcap" || tr.Trace.Packets != 61 || !slices.Equal(tr.Trace.Ports, []int{102}) {
		t.Fatalf("OpenTrace = %+v, %v", tr, err)
	}
	list, _ := svc.Sessions(ctx)
	if !slices.ContainsFunc(list, func(s model.SessionInfo) bool { return s.ID == tr.ID && s.Trace != nil }) {
		t.Errorf("sessions = %+v", list)
	}
	sum, err := svc.TraceSummary(ctx, tr.ID)
	if err != nil || sum.Transactions != 9 || sum.ByService["Read"] == 0 || sum.Counters.Unsupported["other_tcp"] != 1 || sum.Findings[model.SeverityError] == 0 {
		t.Errorf("TraceSummary = %+v, %v", sum, err)
	}
	assocs, err := svc.TraceAssociations(ctx, tr.ID)
	if err != nil || len(assocs) < 3 || assocs[0].Open != "associate" || assocs[0].Initiate == nil || assocs[0].CallingTSAP != "0001" {
		t.Errorf("TraceAssociations = %+v, %v", assocs, err)
	}
	tl, err := svc.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Status: []string{"unanswered", "pending"}, Limit: 10})
	if err != nil || tl.Total != 2 || tl.Transactions[0].Objects[0].Ref != "LD0/MMXU1.Hz[MX]" || tl.Transactions[0].Request == nil {
		t.Errorf("TraceTimeline = %+v, %v", tl, err)
	}
	if tl, _ := svc.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Service: "read", Association: 3}); tl.Total != 3 {
		t.Errorf("Reads of association 3 = %d", tl.Total)
	}
	rep, err := svc.TraceReports(ctx, tr.ID, model.TraceReportsQuery{})
	if err != nil || rep.Total != 1 || rep.Reports[0].Decoded {
		t.Errorf("TraceReports = %+v, %v", rep, err)
	}
	streams, err := svc.TraceStreams(ctx, tr.ID)
	if err != nil || len(streams.GOOSE) == 0 || len(streams.SV) == 0 {
		t.Errorf("TraceStreams = %+v, %v", streams, err)
	}
	fs, err := svc.TraceFindings(ctx, tr.ID, model.FindingsQuery{Kind: "unanswered"})
	if err != nil || fs.Total != 1 || fs.Findings[0].Source != "Transaction.Status=unanswered" {
		t.Errorf("TraceFindings = %+v, %v", fs, err)
	}

	// Linked to an SCL session.
	scl, err := svc.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := svc.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: scl.ID})
	if err != nil || linked.Trace.Link == nil || linked.Trace.Link.Kind != model.SessionKindSCL || linked.Trace.Link.DataSets != 2 {
		t.Fatalf("LinkTrace = %+v, %v", linked.Trace, err)
	}
	if fs, _ := svc.TraceFindings(ctx, tr.ID, model.FindingsQuery{Kind: "not-in-model"}); fs.Total == 0 {
		t.Error("no not-in-model finding")
	}
	tl, _ = svc.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Limit: 1})
	if tl.Transactions[0].Objects == nil {
		t.Errorf("objects = %+v", tl.Transactions[0])
	}
	f, err := svc.Export(ctx, tr.ID, model.ExportRequest{Format: model.ExportTimelineJSON})
	if err != nil || f.MIME != "application/json" || !json.Valid([]byte(f.Text)) {
		t.Errorf("Export = %+v, %v", f.Name, err)
	}

	// A capture on another port, with the port given.
	ic, err := svc.OpenTrace(ctx, model.OpenTraceRequest{File: traceFile(t, "iec61850bean_ied-server_ied-reporter-libiec61850.pcap"), Ports: []int{1102}})
	if err != nil || !slices.Equal(ic.Trace.Ports, []int{1102}) {
		t.Fatalf("OpenTrace on 1102 = %+v, %v", ic, err)
	}
	if rep, _ := svc.TraceReports(ctx, ic.ID, model.TraceReportsQuery{}); rep.Total != 1 || rep.Reports[0].RptID == "" {
		t.Errorf("reporter capture = %+v", rep)
	}
	if err := svc.CloseSession(ctx, ic.ID); err != nil {
		t.Fatal(err)
	}

	// Errors.
	if _, err := svc.OpenTrace(ctx, model.OpenTraceRequest{File: model.File{Name: "x.pcap", Data: []byte("not a capture")}}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("not a capture: %v", err)
	}
	d, err := svc.OpenTrace(ctx, model.OpenTraceRequest{File: traceFile(t, "damaged.pcap")})
	if err != nil || !d.Trace.Damaged {
		t.Errorf("damaged = %+v, %v", d.Trace, err)
	}
	for name, call := range map[string]func() error{
		"summary of SCL": func() error { _, e := svc.TraceSummary(ctx, scl.ID); return e },
		"browse trace":   func() error { _, e := svc.Browse(ctx, tr.ID, ""); return e },
		"bad status": func() error {
			_, e := svc.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Status: []string{"x"}})
			return e
		},
		"bad severity":   func() error { _, e := svc.TraceFindings(ctx, tr.ID, model.FindingsQuery{Severity: "x"}); return e },
		"bad page":       func() error { _, e := svc.TraceReports(ctx, tr.ID, model.TraceReportsQuery{Limit: -1}); return e },
		"assocs of SCL":  func() error { _, e := svc.TraceAssociations(ctx, scl.ID); return e },
		"streams of SCL": func() error { _, e := svc.TraceStreams(ctx, scl.ID); return e },
		"wire of trace":  func() error { _, e := svc.Wire(ctx, tr.ID, 1); return e },
		"link to trace":  func() error { _, e := svc.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: d.ID}); return e },
	} {
		if err := call(); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s = %v, want invalid_argument", name, err)
		}
	}
	if _, err := svc.TraceSummary(ctx, "nope"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session: %v", err)
	}

	// The wire view of a live session's operation.
	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	events, _ := svc.Subscribe(sub, 0)
	live, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Read(ctx, live.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); err != nil {
		t.Fatal(err)
	}
	op := next(t, events, "read operation", func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.SessionID == live.ID && ev.Operation.Action == "read"
	}).Operation
	wv, err := svc.Wire(ctx, live.ID, op.Wire)
	if err != nil || wv.Operation != op.Wire || len(wv.Units) < 2 ||
		!slices.ContainsFunc(wv.Units, func(u model.WireUnit) bool {
			return u.PDU != nil && u.PDU.InvokeID != nil && *u.PDU.InvokeID == op.Requests[0].InvokeID && u.Direction == "recv"
		}) {
		t.Errorf("Wire = %+v, %v", wv, err)
	}
	if _, err := svc.Wire(ctx, live.ID, op.Wire+100); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("Wire of a future operation: %v", err)
	}
	// Linking the live session reads its data sets and control blocks.
	linked, err = svc.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: live.ID})
	if err != nil || linked.Trace.Link.Kind != model.SessionKindLive || linked.Trace.Link.RCBs == 0 {
		t.Errorf("live link = %+v, %v", linked.Trace, err)
	}
	for _, id := range []string{tr.ID, d.ID, scl.ID, live.ID} {
		if err := svc.CloseSession(ctx, id); err != nil {
			t.Errorf("close %s: %v", id, err)
		}
	}
}

// TestTraceHTTP: the request forms only the HTTP API has.
func TestTraceHTTP(t *testing.T) {
	d := newDaemon(t, engine.Options{}, api.Options{})
	for _, c := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"POST", "/api/v1/traces/open?ports=102,x", "abc", http.StatusBadRequest, service.CodeInvalidArgument},
		{"GET", "/api/v1/sessions/x/wire?operation=abc", "", http.StatusBadRequest, service.CodeInvalidArgument},
		{"GET", "/api/v1/sessions/x/wire?operation=1", "", http.StatusNotFound, service.CodeNotFound},
		{"POST", "/api/v1/sessions/x/trace/timeline", "{", http.StatusBadRequest, service.CodeInvalidArgument},
		{"POST", "/api/v1/sessions/x/trace/reports", "{", http.StatusBadRequest, service.CodeInvalidArgument},
		{"POST", "/api/v1/sessions/x/trace/findings", "{", http.StatusBadRequest, service.CodeInvalidArgument},
		{"PUT", "/api/v1/sessions/x/trace/link", "{", http.StatusBadRequest, service.CodeInvalidArgument},
	} {
		status, _, body := d.request(t, c.method, c.path, testToken, c.body, nil)
		if status != c.status || !strings.Contains(body, `"code":"`+c.code+`"`) {
			t.Errorf("%s %s = %d %s", c.method, c.path, status, body)
		}
	}
}
