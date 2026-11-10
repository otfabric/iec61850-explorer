// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms-trace/trace"

	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// Trace sessions (M4, ADR 0005): a capture the user chose, analysed offline
// by go-mms-trace. The engine decodes nothing itself; it keeps the result,
// derives findings from its counters and statuses, and correlates it with a
// model session (trace_link.go).

// maxCaptureFindings bounds the findings kept per capture; beyond it one
// finding says how many were left out.
const maxCaptureFindings = 20000

// traceState is what a trace session holds.
type traceState struct {
	origin   model.TraceOrigin
	file     model.TraceFile
	res      *trace.Result
	findings []model.TraceFinding

	mu   sync.Mutex
	link *traceLink
}

// traceOf returns the trace of a session, or an error for a session that
// is not a trace.
func (e *Engine) traceOf(id string) (*session, *traceState, error) {
	s, err := e.lookup(id)
	if err != nil {
		return nil, nil, err
	}
	if s.trace == nil {
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "session %s is not a trace session", id)
	}
	return s, s.trace, nil
}

// nsTime converts Unix nanoseconds to a time; nil for 0 (no timestamp).
func nsTime(ns int64) *time.Time {
	if ns == 0 {
		return nil
	}
	t := time.Unix(0, ns).UTC()
	return &t
}

// tracePorts validates the MMS ports of a request; none means 102.
func tracePorts(ports []int) ([]uint16, []int, error) {
	if len(ports) == 0 {
		return []uint16{model.DefaultPort}, []int{model.DefaultPort}, nil
	}
	if len(ports) > model.MaxTracePorts {
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "at most %d MMS ports", model.MaxTracePorts)
	}
	var out []uint16
	var shown []int
	for _, p := range ports {
		if p < 1 || p > 65535 {
			return nil, nil, service.Errorf(service.CodeInvalidArgument, "port %d is out of range", p)
		}
		if !slices.Contains(shown, p) {
			out, shown = append(out, uint16(p)), append(shown, p)
		}
	}
	return out, shown, nil
}

// OpenTrace implements service.Service.
func (e *Engine) OpenTrace(ctx context.Context, req model.OpenTraceRequest) (model.SessionInfo, error) {
	f := req.File
	switch {
	case len(f.Data) == 0:
		return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "the capture file is empty")
	case len(f.Data) > model.MaxTraceBytes:
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "the capture is larger than %d MiB", model.MaxTraceBytes>>20)
	}
	ports, shown, err := tracePorts(req.Ports)
	if err != nil {
		return model.SessionInfo{}, err
	}
	if err := e.roomForTrace(); err != nil {
		return model.SessionInfo{}, err
	}
	res, err := trace.Analyze(ctx, bytes.NewReader(f.Data), trace.Options{Ports: ports})
	var damage error
	switch {
	case err == nil:
	case errors.Is(err, trace.ErrCorrupt) && res != nil:
		damage = err
	case errors.Is(err, trace.ErrFormat):
		return model.SessionInfo{}, &service.Error{Code: service.CodeInvalidArgument,
			Message: displayName(f.Name) + " is not a pcap or pcapng capture (plain or gzip)", Err: err}
	case ctx.Err() != nil:
		return model.SessionInfo{}, ctx.Err()
	default:
		return model.SessionInfo{}, &service.Error{Code: service.CodeInvalidArgument, Message: displayName(f.Name) + ": " + err.Error(), Err: err}
	}
	ts := newTraceState(displayName(f.Name), shown, res, damage)

	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case e.closed:
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the engine is shutting down")
	case len(e.sessions) >= e.opts.MaxSessions+maxLostSessions:
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "too many sessions; close some first")
	case e.traceCount() >= model.MaxTraceSessions:
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "at most %d trace sessions may be open", model.MaxTraceSessions)
	}
	s := &session{id: newID(), openedAt: time.Now().UTC(), state: model.SessionOffline, rec: &recorder{}, trace: ts}
	s.setTree(nil, 0)
	e.sessions[s.id] = s
	e.order = append(e.order, s.id)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	e.log.Info("trace opened", "session", s.id, "packets", res.File.Packets, "associations", len(res.Associations),
		"transactions", len(res.Transactions), "damaged", damage != nil)
	return info, nil
}

// roomForTrace refuses a capture early when no trace session can be opened.
func (e *Engine) roomForTrace() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.traceCount() >= model.MaxTraceSessions {
		return service.Errorf(service.CodeLimitExceeded, "at most %d trace sessions may be open", model.MaxTraceSessions)
	}
	return nil
}

// traceCount counts the trace sessions. e.mu must be held.
func (e *Engine) traceCount() int {
	n := 0
	for _, s := range e.sessions {
		if s.trace != nil {
			n++
		}
	}
	return n
}

func newTraceState(name string, ports []int, res *trace.Result, damage error) *traceState {
	f := res.File
	tf := model.TraceFile{
		Name: name, Format: f.Format, Gzip: f.Gzip, SHA256: f.SHA256, Bytes: f.InputBytes, Packets: f.Packets,
		Interfaces: f.Interfaces, Links: append([]string{}, f.Links...), TruncatedTail: f.TruncatedTail,
		SkippedBlocks: f.SkippedBlocks, First: nsTime(f.FirstNS), Last: nsTime(f.LastNS), Ports: ports,
	}
	if f.FirstNS != 0 && f.LastNS >= f.FirstNS {
		tf.DurationMS = float64(f.LastNS-f.FirstNS) / 1e6
	}
	if damage != nil {
		tf.Damaged, tf.Damage = true, damage.Error()
	}
	ts := &traceState{
		origin: model.TraceOrigin{File: name, Format: f.Format, Gzip: f.Gzip, Packets: f.Packets, SHA256: f.SHA256, Ports: ports, Damaged: damage != nil},
		file:   tf, res: res,
	}
	ts.findings = captureFindings(res, damage)
	return ts
}

// counters converts the analyser's counters.
func counters(c trace.Counters) model.TraceCounters {
	out := model.TraceCounters{
		Unsupported: map[string]uint64{}, TPKTResync: c.TPKTResync, COTPErrors: c.COTPErrors, TSDUErrors: c.TSDUErrors,
		BareMMS: c.BareMMS, TSOSegments: c.TSOSegments, GOOSEErrors: c.GOOSEErrors, SVErrors: c.SVErrors,
	}
	for k, v := range c.Unsupported {
		out.Unsupported[k] = v
	}
	return out
}

// TraceSummary implements service.Service.
func (e *Engine) TraceSummary(_ context.Context, id string) (model.TraceSummary, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return model.TraceSummary{}, err
	}
	res := ts.res
	sum := model.TraceSummary{
		File: ts.file, Counters: counters(res.Counters), Associations: len(res.Associations), Transactions: len(res.Transactions),
		ByStatus: map[string]int{}, ByService: map[string]int{}, Reports: len(res.Reports), GOOSE: len(res.GOOSE), SV: len(res.SV),
		Findings: map[string]int{}, Engine: "go-mms-trace " + buildinfo.ModuleVersion(buildinfo.GoMMSTracePath),
	}
	for _, t := range res.Transactions {
		sum.ByStatus[t.Status]++
		sum.ByService[serviceOf(t)]++
	}
	for _, f := range ts.allFindings() {
		sum.Findings[f.Severity]++
	}
	sum.Link = ts.linkInfo()
	return sum, nil
}

// serviceOf names a transaction's service; an orphan error has none.
func serviceOf(t *trace.Transaction) string {
	if t.Service == "" {
		return "unknown"
	}
	return t.Service
}

// TraceAssociations implements service.Service.
func (e *Engine) TraceAssociations(_ context.Context, id string) ([]model.TraceAssociation, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return nil, err
	}
	out := make([]model.TraceAssociation, 0, len(ts.res.Associations))
	for _, a := range ts.res.Associations {
		ta := model.TraceAssociation{
			ID: a.ID, Client: a.Client.String(), Server: a.Server.String(), Open: a.Open, Close: a.Close,
			Accepted: a.Accepted, Authentication: a.Authentication, First: nsTime(a.FirstNS), Last: nsTime(a.LastNS),
			Transactions: a.Transactions, Reports: a.Reports, Unanswered: a.Unanswered, Gaps: a.Gaps,
		}
		if a.CallingTSAP != nil {
			ta.CallingTSAP = hex.EncodeToString(a.CallingTSAP)
		}
		if a.CalledTSAP != nil {
			ta.CalledTSAP = hex.EncodeToString(a.CalledTSAP)
		}
		if a.Initiate != nil {
			ta.Initiate = initiateOf(a.Initiate)
		}
		out = append(out, ta)
	}
	return out, nil
}

// page checks and fills paging parameters.
func page(offset, limit int) (int, int, error) {
	switch {
	case offset < 0:
		return 0, 0, service.Errorf(service.CodeInvalidArgument, "offset must not be negative")
	case limit < 0 || limit > model.MaxTracePage:
		return 0, 0, service.Errorf(service.CodeInvalidArgument, "limit must be between 0 and %d", model.MaxTracePage)
	case limit == 0:
		limit = model.DefaultTracePage
	}
	return offset, limit, nil
}

// at is the time of a transaction: its request, or the response of an
// orphan.
func at(t *trace.Transaction) int64 {
	if t.RequestNS != 0 {
		return t.RequestNS
	}
	return t.ResponseNS
}

// timelineFilter checks a query and returns its predicate.
func timelineFilter(q model.TimelineQuery) (func(*trace.Transaction) bool, error) {
	statuses := map[string]bool{}
	for _, st := range q.Status {
		switch st {
		case model.TraceStatusOK, model.TraceStatusError, model.TraceStatusReject, model.TraceStatusUnanswered,
			model.TraceStatusOrphan, model.TraceStatusPending:
			statuses[st] = true
		default:
			return nil, service.Errorf(service.CodeInvalidArgument, "status %q: use ok, error, reject, unanswered, orphan or pending", st)
		}
	}
	if q.Association < 0 {
		return nil, service.Errorf(service.CodeInvalidArgument, "association must not be negative")
	}
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return nil, service.Errorf(service.CodeInvalidArgument, "the time window ends before it starts")
	}
	svc := strings.TrimSpace(q.Service)
	return func(t *trace.Transaction) bool {
		ns := at(t)
		switch {
		case q.Association != 0 && t.Association != q.Association,
			svc != "" && !strings.EqualFold(serviceOf(t), svc),
			len(statuses) > 0 && !statuses[t.Status],
			q.From != nil && (ns == 0 || ns < q.From.UnixNano()),
			q.To != nil && (ns == 0 || ns > q.To.UnixNano()):
			return false
		}
		return true
	}, nil
}

// TraceTimeline implements service.Service.
func (e *Engine) TraceTimeline(_ context.Context, id string, q model.TimelineQuery) (model.Timeline, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return model.Timeline{}, err
	}
	offset, limit, err := page(q.Offset, q.Limit)
	if err != nil {
		return model.Timeline{}, err
	}
	keep, err := timelineFilter(q)
	if err != nil {
		return model.Timeline{}, err
	}
	link := ts.currentLink()
	out := model.Timeline{Offset: offset, Transactions: []model.TraceTransaction{}}
	for i, t := range ts.res.Transactions {
		if !keep(t) {
			continue
		}
		if out.Total >= offset && len(out.Transactions) < limit {
			out.Transactions = append(out.Transactions, transactionOf(i+1, t, link))
		}
		out.Total++
	}
	return out, nil
}

// transactionOf converts a transaction; link, when not nil, resolves its
// objects.
func transactionOf(index int, t *trace.Transaction, link *traceLink) model.TraceTransaction {
	out := model.TraceTransaction{
		Index: index, Association: t.Association, InvokeID: t.InvokeID, Service: serviceOf(t), Status: t.Status,
		ServerRequest: t.ServerRequest, RequestAt: nsTime(t.RequestNS), ResponseAt: nsTime(t.ResponseNS),
		RequestFrame: t.RequestFrame, ResponseFrame: t.ResponseFrame, Request: pduOf(t.Request), Response: pduOf(t.Response),
	}
	if l := t.LatencyNS(); l >= 0 {
		ms := float64(l) / 1e6
		out.LatencyMS = &ms
	}
	src := t.Request
	if src == nil {
		src = t.Response
	}
	out.Objects, _ = pduObjects(src)
	if out.Objects == nil {
		out.Objects = []model.TraceObject{}
	}
	link.resolveAll(out.Objects)
	if out.Request != nil {
		link.resolveAll(out.Request.Objects)
	}
	if out.Response != nil {
		link.resolveAll(out.Response.Objects)
	}
	// A Write request's data and a Read response's results are the
	// variables the request names, in order: the link names their
	// components.
	if req := t.Request; req != nil && req.ListName == nil {
		if out.Request != nil {
			link.valuesOf(req.Variables, req.Values, out.Request.Values)
		}
		if out.Response != nil && t.Response != nil {
			link.valuesOf(req.Variables, t.Response.Values, out.Response.Values)
		}
	}
	out.Error = transactionError(t)
	return out
}

// transactionError is what went wrong with a transaction, as text.
func transactionError(t *trace.Transaction) string {
	var parts []string
	if s := pduError(t.Response); s != "" {
		parts = append(parts, s)
	}
	for _, p := range []struct {
		side string
		pdu  string
	}{{"request", decodeErrorOf(t.Request)}, {"response", decodeErrorOf(t.Response)}} {
		if p.pdu != "" {
			parts = append(parts, p.side+" did not decode: "+p.pdu)
		}
	}
	switch t.Status {
	case model.TraceStatusUnanswered:
		parts = append(parts, "no response before the association ended or the invoke ID was used again")
	case model.TraceStatusOrphan:
		parts = append(parts, "the request was not captured")
	case model.TraceStatusPending:
		parts = append(parts, "no response before the capture ended")
	}
	return strings.Join(parts, "; ")
}

func decodeErrorOf(p *mms.CapturedPDU) string {
	if p == nil {
		return ""
	}
	return p.DecodeError
}

// TraceStreams implements service.Service.
func (e *Engine) TraceStreams(_ context.Context, id string) (model.TraceStreams, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return model.TraceStreams{}, err
	}
	link := ts.currentLink()
	out := model.TraceStreams{GOOSE: []model.TraceGoose{}, SV: []model.TraceSV{}}
	for _, g := range ts.res.GOOSE {
		tg := model.TraceGoose{
			Src: g.Src, Dst: g.Dst, AppID: g.AppID, VLAN: g.VLAN, GocbRef: g.GocbRef, DatSet: g.DatSet, GoID: g.GoID,
			ConfRev: g.ConfRev, Messages: g.Messages, Changes: g.Changes, First: nsTime(g.FirstNS), Last: nsTime(g.LastNS),
			Simulation: g.Simulation, NdsCom: g.NdsCom, Issues: map[string]int{}, MaxPerSecond: g.MaxPerSecond,
		}
		for k, v := range g.Issues {
			tg.Issues[k] = v
		}
		tg.Model, tg.Note = link.gooseModel(g)
		out.GOOSE = append(out.GOOSE, tg)
	}
	for _, v := range ts.res.SV {
		out.SV = append(out.SV, model.TraceSV{
			Src: v.Src, Dst: v.Dst, AppID: v.AppID, VLAN: v.VLAN, SvID: v.SvID, DatSet: v.DatSet, ConfRev: v.ConfRev,
			Messages: v.Messages, ASDUs: v.ASDUs, Lost: v.Lost, Repeated: v.Repeated, First: nsTime(v.FirstNS),
			Last: nsTime(v.LastNS), Wrap: v.Wrap,
		})
	}
	return out, nil
}

// allFindings are the capture's findings followed by the link's.
func (ts *traceState) allFindings() []model.TraceFinding {
	out := ts.findings
	if l := ts.currentLink(); l != nil && len(l.findings) > 0 {
		out = append(slices.Clip(out), l.findings...)
	}
	return out
}

// TraceFindings implements service.Service.
func (e *Engine) TraceFindings(_ context.Context, id string, q model.FindingsQuery) (model.TraceFindings, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return model.TraceFindings{}, err
	}
	offset, limit, err := page(q.Offset, q.Limit)
	if err != nil {
		return model.TraceFindings{}, err
	}
	switch q.Severity {
	case "", model.SeverityError, model.SeverityWarning, model.SeverityInfo:
	default:
		return model.TraceFindings{}, service.Errorf(service.CodeInvalidArgument, "severity %q: use error, warning or info", q.Severity)
	}
	out := model.TraceFindings{Offset: offset, BySeverity: map[string]int{}, Findings: []model.TraceFinding{}}
	for _, f := range ts.allFindings() {
		if (q.Severity != "" && f.Severity != q.Severity) || (q.Kind != "" && f.Kind != q.Kind) {
			continue
		}
		out.BySeverity[f.Severity]++
		if out.Total >= offset && len(out.Findings) < limit {
			out.Findings = append(out.Findings, f)
		}
		out.Total++
	}
	return out, nil
}

// findingList collects findings up to maxCaptureFindings.
type findingList struct {
	list    []model.TraceFinding
	dropped int
}

func (l *findingList) add(f model.TraceFinding) {
	if len(l.list) >= maxCaptureFindings {
		l.dropped++
		return
	}
	l.list = append(l.list, f)
}

func (l *findingList) done() []model.TraceFinding {
	if l.dropped > 0 {
		l.list = append(l.list, model.TraceFinding{
			Severity: model.SeverityInfo, Kind: "findings-dropped", Source: "engine",
			Message: fmt.Sprintf("%d more findings are not listed (at most %d per capture)", l.dropped, maxCaptureFindings),
			Count:   uint64(l.dropped),
		})
	}
	if l.list == nil {
		return []model.TraceFinding{}
	}
	return l.list
}

// captureFindings derives the gaps and anomalies of a capture from what
// go-mms-trace counted and decided. Nothing is inferred beyond those
// fields: each finding names the field it comes from.
func captureFindings(res *trace.Result, damage error) []model.TraceFinding {
	var l findingList
	f := res.File
	if damage != nil {
		l.add(model.TraceFinding{Severity: model.SeverityError, Kind: "damaged", Source: "trace.ErrCorrupt",
			Message: "the capture is damaged; what follows the damage was not read: " + damage.Error(),
			Frame:   f.Packets, At: nsTime(f.LastNS)})
	}
	if f.TruncatedTail {
		l.add(model.TraceFinding{Severity: model.SeverityWarning, Kind: "truncated-tail", Source: "File.TruncatedTail",
			Message: "the last packet record is cut short (the capture was not closed cleanly)", At: nsTime(f.LastNS)})
	}
	if f.SkippedBlocks > 0 {
		l.add(model.TraceFinding{Severity: model.SeverityWarning, Kind: "skipped-blocks", Source: "File.SkippedBlocks", Count: f.SkippedBlocks,
			Message: fmt.Sprintf("%d damaged pcapng blocks were skipped", f.SkippedBlocks)})
	}
	for _, a := range res.Associations {
		associationFindings(&l, a)
	}
	for i, t := range res.Transactions {
		transactionFindings(&l, i+1, t)
	}
	for _, r := range res.Reports {
		if r.PDU != nil && r.PDU.DecodeError != "" {
			l.add(model.TraceFinding{Severity: model.SeverityError, Kind: "decode-error", Source: "Report.PDU.DecodeError",
				Association: r.Association, Frame: r.Frame, At: nsTime(r.TimeNS),
				Message: "an unconfirmed PDU did not decode: " + r.PDU.DecodeError})
		}
	}
	counterFindings(&l, res.Counters)
	for _, g := range res.GOOSE {
		gooseFindings(&l, g)
	}
	for _, v := range res.SV {
		if v.Lost > 0 {
			l.add(model.TraceFinding{Severity: model.SeverityError, Kind: "sv-lost", Source: "SVStream.Lost", Stream: v.SvID,
				Count: uint64(v.Lost), At: nsTime(v.FirstNS), Message: fmt.Sprintf("Sampled Values %s: %d samples lost (smpCnt skipped)", v.SvID, v.Lost)})
		}
		if v.Repeated > 0 {
			l.add(model.TraceFinding{Severity: model.SeverityWarning, Kind: "sv-repeated", Source: "SVStream.Repeated", Stream: v.SvID,
				Count: uint64(v.Repeated), At: nsTime(v.FirstNS), Message: fmt.Sprintf("Sampled Values %s: %d samples repeated (smpCnt unchanged)", v.SvID, v.Repeated)})
		}
	}
	return l.done()
}

func associationFindings(l *findingList, a *trace.Association) {
	where := func(f model.TraceFinding) model.TraceFinding {
		f.Association = a.ID
		if f.At == nil {
			f.At = nsTime(a.FirstNS)
		}
		return f
	}
	if a.Gaps > 0 {
		l.add(where(model.TraceFinding{Severity: model.SeverityError, Kind: "tcp-gap", Source: "Association.Gaps", Count: uint64(a.Gaps),
			Message: fmt.Sprintf("association %d: %s of the TCP stream not captured; PDUs in them are lost", a.ID, plural(a.Gaps, "range", "ranges"))}))
	}
	switch a.Open {
	case "midstream":
		l.add(where(model.TraceFinding{Severity: model.SeverityWarning, Kind: "midstream", Source: "Association.Open=midstream",
			Message: fmt.Sprintf("association %d was captured midstream: no AARQ, AARE or TSAPs", a.ID)}))
	case "connect":
		l.add(where(model.TraceFinding{Severity: model.SeverityWarning, Kind: "no-associate", Source: "Association.Open=connect",
			Message: fmt.Sprintf("association %d: a COTP connection without a captured AARQ", a.ID)}))
	}
	if a.Accepted != nil && !*a.Accepted {
		l.add(where(model.TraceFinding{Severity: model.SeverityError, Kind: "association-rejected", Source: "Association.Accepted",
			Message: fmt.Sprintf("association %d was rejected (AARE result)", a.ID)}))
	}
	switch a.Close {
	case "":
		l.add(where(model.TraceFinding{Severity: model.SeverityInfo, Kind: "open-at-end", Source: "Association.Close", At: nsTime(a.LastNS),
			Message: fmt.Sprintf("association %d was still open when the capture ended (or its end was not captured)", a.ID)}))
	case "abort", "reset":
		l.add(where(model.TraceFinding{Severity: model.SeverityWarning, Kind: "association-" + a.Close, Source: "Association.Close=" + a.Close, At: nsTime(a.LastNS),
			Message: fmt.Sprintf("association %d ended with %s", a.ID, map[string]string{"abort": "an abort", "reset": "a TCP reset"}[a.Close])}))
	}
}

// statusFindings maps a transaction status to its finding.
var statusFindings = map[string]struct {
	severity, message string
}{
	model.TraceStatusUnanswered: {model.SeverityError, "no response before the association ended or the invoke ID was used again"},
	model.TraceStatusReject:     {model.SeverityError, "rejected"},
	model.TraceStatusError:      {model.SeverityWarning, "answered with a confirmed error"},
	model.TraceStatusOrphan:     {model.SeverityWarning, "a response whose request was not captured"},
	model.TraceStatusPending:    {model.SeverityWarning, "no response before the capture ended"},
}

func transactionFindings(l *findingList, index int, t *trace.Transaction) {
	frame, ns := t.RequestFrame, t.RequestNS
	if frame == 0 {
		frame, ns = t.ResponseFrame, t.ResponseNS
	}
	if sf, ok := statusFindings[t.Status]; ok {
		msg := fmt.Sprintf("%s (invoke ID %d): %s", serviceOf(t), t.InvokeID, sf.message)
		if e := pduError(t.Response); e != "" {
			msg += ": " + e
		}
		l.add(model.TraceFinding{Severity: sf.severity, Kind: t.Status, Source: "Transaction.Status=" + t.Status,
			Association: t.Association, Transaction: index, Frame: frame, At: nsTime(ns), Message: msg})
	}
	for _, side := range []struct {
		name  string
		pdu   string
		frame uint64
		ns    int64
	}{{"request", decodeErrorOf(t.Request), t.RequestFrame, t.RequestNS}, {"response", decodeErrorOf(t.Response), t.ResponseFrame, t.ResponseNS}} {
		if side.pdu != "" {
			l.add(model.TraceFinding{Severity: model.SeverityError, Kind: "decode-error", Source: "Transaction." + strings.ToUpper(side.name[:1]) + side.name[1:] + ".DecodeError",
				Association: t.Association, Transaction: index, Frame: side.frame, At: nsTime(side.ns),
				Message: fmt.Sprintf("%s %s (invoke ID %d) did not decode: %s", serviceOf(t), side.name, t.InvokeID, side.pdu)})
		}
	}
}

func counterFindings(l *findingList, c trace.Counters) {
	add := func(n uint64, severity, kind, source, message string) {
		if n > 0 {
			l.add(model.TraceFinding{Severity: severity, Kind: kind, Source: source, Count: n, Message: fmt.Sprintf(message, n)})
		}
	}
	add(c.TPKTResync, model.SeverityWarning, "tpkt-resync", "Counters.TPKTResync", "%d bytes were skipped to find the next TPKT header (after a gap or garbage)")
	add(c.COTPErrors, model.SeverityError, "cotp-errors", "Counters.COTPErrors", "%d TPKT payloads did not decode as COTP")
	add(c.TSDUErrors, model.SeverityError, "tsdu-errors", "Counters.TSDUErrors", "%d reassembled units did not decode as session, presentation, ACSE or MMS (or were too large)")
	add(c.BareMMS, model.SeverityInfo, "bare-mms", "Counters.BareMMS", "%d MMS PDUs were sent straight in COTP, without session and presentation (not IEC 61850-8-1)")
	add(c.TSOSegments, model.SeverityInfo, "tso-segments", "Counters.TSOSegments", "%d IPv4 packets were captured before segmentation offload (total length 0); not an error")
	add(c.GOOSEErrors, model.SeverityError, "goose-errors", "Counters.GOOSEErrors", "%d GOOSE frames did not decode")
	add(c.SVErrors, model.SeverityError, "sv-errors", "Counters.SVErrors", "%d Sampled Values frames did not decode")
	reasons := make([]string, 0, len(c.Unsupported))
	for r := range c.Unsupported {
		reasons = append(reasons, r)
	}
	slices.Sort(reasons)
	for _, r := range reasons {
		msg := "%d packets were not used: " + r
		if r == "other_tcp" {
			msg = "%d TCP segments were on ports other than the MMS ports (set the MMS ports if MMS runs on another port)"
		}
		add(c.Unsupported[r], model.SeverityInfo, "unsupported", "Counters.Unsupported["+strconv.Quote(r)+"]", msg)
	}
}

// gooseIssues names the issue kinds go-mms-trace counts, with a severity:
// a skipped state or sequence number means a message was lost.
var gooseIssues = map[string]struct{ kind, severity string }{
	"stNum decreased":         {"goose-stnum-decreased", model.SeverityWarning},
	"stNum jumped":            {"goose-stnum-jumped", model.SeverityError},
	"sqNum gap":               {"goose-sqnum-gap", model.SeverityError},
	"repeated (stNum, sqNum)": {"goose-repeated", model.SeverityWarning},
	"confRev changed":         {"goose-confrev-changed", model.SeverityWarning},
	"entries changed":         {"goose-entries-changed", model.SeverityWarning},
}

func gooseFindings(l *findingList, g *trace.GooseStream) {
	issues := make([]string, 0, len(g.Issues))
	for k := range g.Issues {
		issues = append(issues, k)
	}
	slices.Sort(issues)
	for _, k := range issues {
		gi, ok := gooseIssues[k]
		if !ok {
			gi = struct{ kind, severity string }{"goose-issue", model.SeverityWarning}
		}
		l.add(model.TraceFinding{Severity: gi.severity, Kind: gi.kind, Source: "GooseStream.Issues[" + strconv.Quote(k) + "]",
			Stream: g.GocbRef, Count: uint64(g.Issues[k]), At: nsTime(g.FirstNS),
			Message: fmt.Sprintf("GOOSE %s: %s, %s", g.GocbRef, k, plural(g.Issues[k], "time", "times"))})
	}
	if g.Simulation > 0 {
		l.add(model.TraceFinding{Severity: model.SeverityInfo, Kind: "goose-simulation", Source: "GooseStream.Simulation", Stream: g.GocbRef,
			Count: uint64(g.Simulation), At: nsTime(g.FirstNS), Message: fmt.Sprintf("GOOSE %s: %d messages with the simulation or test flag", g.GocbRef, g.Simulation)})
	}
}

// plural is "1 range" or "3 ranges".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
