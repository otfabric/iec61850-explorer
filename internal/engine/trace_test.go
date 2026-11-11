// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// fixture reads a capture of testdata/traces.
func fixture(t *testing.T, name string) model.File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "traces", name))
	if err != nil {
		t.Fatal(err)
	}
	return model.File{Name: name, Data: b}
}

func openTrace(t *testing.T, e *Engine, f model.File, ports ...int) model.SessionInfo {
	t.Helper()
	info, err := e.OpenTrace(t.Context(), model.OpenTraceRequest{File: f, Ports: ports})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func findings(t *testing.T, e *Engine, id string) []model.TraceFinding {
	t.Helper()
	res, err := e.TraceFindings(t.Context(), id, model.FindingsQuery{Limit: model.MaxTracePage})
	if err != nil {
		t.Fatal(err)
	}
	return res.Findings
}

func findingKinds(fs []model.TraceFinding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

func openSimSCL(t *testing.T, e *Engine) model.SessionInfo {
	t.Helper()
	info, err := e.OpenSCL(t.Context(), model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// TestTraceGapsOfSimIED is M4's exit test: a capture of the Explorer
// reading the simulated IED, with one segment lost, is analysed offline;
// the lost segment and the request it leaves unanswered are findings, and
// the transactions' MMS names resolve to the model of the IED.
func TestTraceGapsOfSimIED(t *testing.T) {
	ied := startIED(t)
	r := startRelay(t, ied.Addr)
	e := newEngine(t)
	ctx := t.Context()

	captured, err := e.OpenSession(ctx, target(t, r.addr))
	if err != nil {
		t.Fatal(err)
	}
	before := len(r.recording())
	if _, err := e.Read(ctx, captured.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]", simied.Domain + "/LLN0.Mod.stVal[ST]"}}); err != nil {
		t.Fatal(err)
	}
	r.waitQuiet()
	// A reference the IED does not have: the IED answers with an error,
	// and the model has no such object.
	if _, err := e.Read(ctx, captured.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Nope[MX]"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.CloseSession(ctx, captured.ID); err != nil {
		t.Fatal(err)
	}
	r.waitQuiet()

	// Drop the IED's segment that carries the response of the first Read.
	chunks := r.recording()
	drop := -1
	for i := before; i < len(chunks) && drop < 0; i++ {
		for _, p := range pdusOf(chunks[i]) {
			if chunks[i].fromPeer && p.Kind == "confirmed-response" && p.Service == "Read" {
				drop = i
			}
		}
	}
	if drop < 0 {
		t.Fatal("no Read response recorded")
	}
	capture := pcapOf(chunks, 102, map[int]bool{drop: true})

	tr := openTrace(t, e, model.File{Name: "simied.pcap", Data: capture})
	if tr.Kind != model.SessionKindTrace || tr.Trace == nil || tr.State != model.SessionOffline || tr.Trace.Format != "pcap" {
		t.Fatalf("trace session = %+v", tr)
	}
	assocs, err := e.TraceAssociations(ctx, tr.ID)
	if err != nil || len(assocs) != 1 {
		t.Fatalf("associations = %+v, %v", assocs, err)
	}
	if a := assocs[0]; a.Open != "associate" || a.Close != "release" || a.Accepted == nil || !*a.Accepted || a.Gaps < 1 ||
		a.Authentication != "none" || a.Initiate == nil || a.CalledTSAP != "0001" || a.Server != "10.0.0.2:102" {
		t.Errorf("association = %+v", a)
	}

	// Link it to the IED's model (a second, direct session).
	model1, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	linked, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: model1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if l := linked.Trace.Link; l == nil || l.SessionID != model1.ID || l.Kind != model.SessionKindLive || l.DataSets != 2 || l.RCBs == 0 {
		t.Fatalf("link = %+v", linked.Trace.Link)
	}

	fs := findings(t, e, tr.ID)
	var gap, unanswered, missing *model.TraceFinding
	for i, f := range fs {
		switch f.Kind {
		case "tcp-gap":
			gap = &fs[i]
		case model.TraceStatusUnanswered:
			unanswered = &fs[i]
		case "not-in-model":
			missing = &fs[i]
		}
	}
	if gap == nil || gap.Association != 1 || gap.Source != "Association.Gaps" || gap.Severity != model.SeverityError {
		t.Errorf("gap finding = %+v in %v", gap, findingKinds(fs))
	}
	if unanswered == nil || unanswered.Transaction == 0 || unanswered.Frame == 0 || unanswered.At == nil ||
		unanswered.Source != "Transaction.Status=unanswered" || !strings.HasPrefix(unanswered.Message, "Read") {
		t.Fatalf("unanswered finding = %+v in %v", unanswered, findingKinds(fs))
	}
	if missing == nil || !strings.Contains(missing.Message, simied.Domain+"/MMXU1.Nope[MX]") {
		t.Errorf("not-in-model finding = %+v", missing)
	}

	// The timeline: the unanswered Read names objects that resolve.
	tl, err := e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Status: []string{model.TraceStatusUnanswered}})
	if err != nil || tl.Total != 1 {
		t.Fatalf("unanswered = %+v, %v", tl, err)
	}
	tx := tl.Transactions[0]
	if tx.Index != unanswered.Transaction || tx.Service != "Read" || tx.Response != nil || tx.LatencyMS != nil || len(tx.Objects) != 2 {
		t.Fatalf("unanswered Read = %+v", tx)
	}
	hz := tx.Objects[0]
	if hz.Ref != simied.Domain+"/MMXU1.Hz[MX]" || hz.MMS.ItemID != "MMXU1$MX$Hz" || hz.Kind != objDO || hz.Model != model.ResolveFound ||
		!slices.Contains(hz.Concerns, "data object "+simied.Domain+"/MMXU1.Hz") || !slices.Contains(hz.Concerns, "data set "+simied.Domain+"/LLN0.Meas") {
		t.Errorf("Hz object = %+v", hz)
	}
	if mod := tx.Objects[1]; mod.Ref != simied.Domain+"/LLN0.Mod.stVal[ST]" || mod.Kind != objDA || mod.Model != model.ResolveFound {
		t.Errorf("Mod object = %+v", mod)
	}
	// Everything else was answered; the second read failed at the IED.
	all, _ := e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Limit: model.MaxTracePage})
	var nope *model.TraceTransaction
	for i, t2 := range all.Transactions {
		if len(t2.Objects) > 0 && t2.Objects[0].Ref == simied.Domain+"/MMXU1.Nope[MX]" {
			nope = &all.Transactions[i]
		}
	}
	if nope == nil || nope.Objects[0].Model != model.ResolveMissing || nope.Status != model.TraceStatusOK || nope.LatencyMS == nil {
		t.Errorf("Nope read = %+v", nope)
	}
	sum, err := e.TraceSummary(ctx, tr.ID)
	if err != nil || sum.ByStatus[model.TraceStatusUnanswered] != 1 || sum.Link == nil || sum.Findings[model.SeverityError] < 2 ||
		!strings.HasPrefix(sum.Engine, "go-mms-trace") {
		t.Errorf("summary = %+v, %v", sum, err)
	}

	// Unlinking removes the resolution and the model's findings.
	if info, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{}); err != nil || info.Trace.Link != nil {
		t.Fatalf("unlink = %+v, %v", info, err)
	}
	if slices.Contains(findingKinds(findings(t, e, tr.ID)), "not-in-model") {
		t.Error("not-in-model findings after unlinking")
	}
	tl, _ = e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Status: []string{model.TraceStatusUnanswered}})
	if tl.Transactions[0].Objects[0].Model != "" {
		t.Errorf("object without a link = %+v", tl.Transactions[0].Objects[0])
	}
}

// TestTraceReportsDecoded: reports captured from a Commission
// subscription are decoded against the data set of their report control
// block, from the IED's model or its SCL file, and shown raw otherwise.
func TestTraceReportsDecoded(t *testing.T) {
	ied := startIED(t)
	r := startRelay(t, ied.Addr)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	live, err := e.OpenSession(ctx, target(t, r.addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, live.ID, model.CapReports)
	if _, err := e.SubscribeReport(ctx, live.ID, model.SubscribeRequest{RCB: urcb, GI: true}); err != nil {
		t.Fatal(err)
	}
	nextReport(t, events, func(*model.Report) bool { return true })
	ied.Set("MMXU1$MX$Hz$mag$f", mms.NewFloat(49.5))
	nextReport(t, events, func(r *model.Report) bool { return len(r.Entries) == 1 })
	if err := e.CloseSession(ctx, live.ID); err != nil {
		t.Fatal(err)
	}
	r.waitQuiet()
	tr := openTrace(t, e, model.File{Name: "reports.pcap", Data: pcapOf(r.recording(), 102, nil)})

	// Without a link, the values are shown as captured.
	rep, err := e.TraceReports(ctx, tr.ID, model.TraceReportsQuery{})
	if err != nil || rep.Total < 2 {
		t.Fatalf("reports = %+v, %v", rep, err)
	}
	if r0 := rep.Reports[0]; r0.Decoded || r0.RptID != "Events" || r0.ListName != "RPT" || len(r0.Values) == 0 || !strings.Contains(r0.Note, "link") || r0.SeqNum == nil {
		t.Errorf("unlinked report = %+v", r0)
	}

	wantRefs := []string{simied.Domain + "/GGIO1.Ind1.stVal[ST]", simied.Domain + "/XCBR1.OpCnt.stVal[ST]", simied.Domain + "/MMXU1.Hz[MX]"}
	for _, link := range []func() model.SessionInfo{
		func() model.SessionInfo { return openSimSCL(t, e) },
		func() model.SessionInfo {
			s, err := e.OpenSession(ctx, target(t, ied.Addr))
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
	} {
		m := link()
		if _, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: m.ID}); err != nil {
			t.Fatal(err)
		}
		rep, _ = e.TraceReports(ctx, tr.ID, model.TraceReportsQuery{Association: 2})
		gi := rep.Reports[0]
		var refs []string
		for _, en := range gi.Entries {
			refs = append(refs, en.Ref)
		}
		if !gi.Decoded || gi.DatSet != simied.Domain+"/LLN0.Events" || !strings.Contains(gi.RCB, "urcbEvents") || !slices.Equal(refs, wantRefs) ||
			!slices.Contains(gi.Entries[0].Reasons, "GI") || gi.Entries[0].Value.Text != "true" || gi.Entries[1].Value.Text != "17" {
			t.Errorf("%s: GI report = %+v", m.Kind, gi)
		}
		dchg := rep.Reports[len(rep.Reports)-1]
		if !dchg.Decoded || len(dchg.Entries) != 1 || dchg.Entries[0].Ref != wantRefs[2] || dchg.Entries[0].Value.Name != "Hz" {
			t.Errorf("%s: data change report = %+v", m.Kind, dchg)
		}
		// The components are named by the model: mag.f, the quality, the time.
		if hz := dchg.Entries[0].Value; !namedComponents(hz) || childNamed(hz, "q") == nil || childNamed(hz, "q").Quality == nil ||
			childNamed(hz, "t") == nil || childNamed(hz, "t").Time == nil || childNamed(hz, "mag") == nil || childNamed(*childNamed(hz, "mag"), "f") == nil {
			t.Errorf("%s: Hz components = %+v", m.Kind, hz)
		}
		// The Commission writes resolve to the RCB.
		tl, _ := e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Service: "write", Limit: 5})
		if tl.Total == 0 || tl.Transactions[0].Objects[0].Kind != objRCB || tl.Transactions[0].Objects[0].Model != model.ResolveFound {
			t.Errorf("%s: write timeline = %+v", m.Kind, tl.Transactions)
		}
	}

	// A snapshot holds no data sets: shown raw, with the reason.
	snapFile, err := e.Export(ctx, openSimSCL(t, e).ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := e.OpenSnapshot(ctx, model.File{Name: "s.json", Data: []byte(snapFile.Text)})
	if err != nil {
		t.Fatal(err)
	}
	info, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: snap.ID})
	if err != nil || info.Trace.Link.Kind != model.SessionKindSnapshot || len(info.Trace.Link.Notes) == 0 {
		t.Fatalf("snapshot link = %+v, %v", info.Trace, err)
	}
	rep, _ = e.TraceReports(ctx, tr.ID, model.TraceReportsQuery{Association: 2, Limit: 1})
	if rep.Reports[0].Decoded || !strings.Contains(rep.Reports[0].Note, "no data set definitions") {
		t.Errorf("report against a snapshot = %+v", rep.Reports[0])
	}
	tl, _ := e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Service: "Write", Limit: 1})
	if o := tl.Transactions[0].Objects[0]; o.Model != resolveUnknown {
		t.Errorf("RCB object against a snapshot = %+v", o)
	}
}

// TestTraceFixtures: captures between independent stacks and public
// captures open, with their content and findings.
func TestTraceFixtures(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()

	// substation.pcap: every transaction status, a report, GOOSE and SV.
	sub := openTrace(t, e, fixture(t, "substation.pcap"))
	sum, err := e.TraceSummary(ctx, sub.ID)
	if err != nil || sum.File.Packets != 61 || sum.File.SHA256 != "45d605515eb57c0264ccca1c548ed7d5819e0793ed37eb82c37f07ac3f6df220" ||
		sum.Transactions != 9 || sum.ByStatus["orphan"] != 1 || sum.ByStatus["pending"] != 1 || sum.Reports != 1 ||
		sum.File.First == nil || sum.File.DurationMS != 60 || !slices.Equal(sum.File.Ports, []int{102}) {
		t.Fatalf("summary = %+v, %v", sum, err)
	}
	kinds := findingKinds(findings(t, e, sub.ID))
	for _, k := range []string{"unanswered", "orphan", "pending", "error", "unsupported"} {
		if !slices.Contains(kinds, k) {
			t.Errorf("findings %v lack %s", kinds, k)
		}
	}
	if errs, _ := e.TraceFindings(ctx, sub.ID, model.FindingsQuery{Severity: model.SeverityError}); errs.Total == 0 || errs.BySeverity[model.SeverityWarning] != 0 {
		t.Errorf("error findings = %+v", errs)
	}
	if pg, _ := e.TraceFindings(ctx, sub.ID, model.FindingsQuery{Kind: "orphan", Offset: 0, Limit: 1}); pg.Total != 1 || pg.Findings[0].Transaction == 0 {
		t.Errorf("orphan finding = %+v", pg)
	}
	tl, _ := e.TraceTimeline(ctx, sub.ID, model.TimelineQuery{Association: 1, Limit: 2, Offset: 1})
	if tl.Total != 5 || len(tl.Transactions) != 2 || tl.Transactions[0].Service != "Read" || tl.Transactions[0].Index != 2 {
		t.Errorf("association 1, page 2 = %+v", tl)
	}
	from := time.Unix(0, 1794225600030000000).UTC()
	to := from.Add(100 * time.Millisecond)
	if tl, _ := e.TraceTimeline(ctx, sub.ID, model.TimelineQuery{From: &from, To: &to}); tl.Total == 0 || tl.Total >= 9 {
		t.Errorf("time window = %d of 9", tl.Total)
	}
	streams, _ := e.TraceStreams(ctx, sub.ID)
	if len(streams.GOOSE) == 0 || len(streams.SV) == 0 || streams.GOOSE[0].Model != "" {
		t.Errorf("streams = %+v", streams)
	}
	// Linked to the simulated IED's SCL file: other names, so findings.
	scl := openSimSCL(t, e)
	if _, err := e.LinkTrace(ctx, sub.ID, model.LinkTraceRequest{Session: scl.ID}); err != nil {
		t.Fatal(err)
	}
	streams, _ = e.TraceStreams(ctx, sub.ID)
	if g := streams.GOOSE[0]; g.Model != model.ResolveMissing || g.Note == "" {
		t.Errorf("GOOSE against SCL = %+v", g)
	}
	rep, _ := e.TraceReports(ctx, sub.ID, model.TraceReportsQuery{})
	if r0 := rep.Reports[0]; r0.Decoded || len(r0.Objects) != 1 || r0.Objects[0].Kind != objRCB || r0.Objects[0].Model != model.ResolveMissing {
		t.Errorf("substation report = %+v", r0)
	}
	if !slices.Contains(findingKinds(findings(t, e, sub.ID)), "not-in-model") {
		t.Error("no not-in-model finding")
	}

	// An interop capture on port 1102: nothing on 102.
	ic := openTrace(t, e, fixture(t, "libiec61850_ied-server_ied-client-iec61850bean.pcap"))
	if a, _ := e.TraceAssociations(ctx, ic.ID); len(a) != 0 {
		t.Errorf("associations on 102 = %+v", a)
	}
	if !slices.Contains(findingKinds(findings(t, e, ic.ID)), "unsupported") {
		t.Error("other_tcp not reported")
	}
	_ = e.CloseSession(ctx, ic.ID)
	ic = openTrace(t, e, fixture(t, "libiec61850_ied-server_ied-client-iec61850bean.pcap"), 1102, 1102)
	if a, _ := e.TraceAssociations(ctx, ic.ID); len(a) != 1 || a[0].Transactions != 20 || !slices.Equal(ic.Trace.Ports, []int{1102}) {
		t.Errorf("associations on 1102 = %+v", a)
	}
	tl, _ = e.TraceTimeline(ctx, ic.ID, model.TimelineQuery{Service: "GetNameList", Limit: 1})
	if tl.Total == 0 || tl.Transactions[0].Response == nil || len(tl.Transactions[0].Response.NameList) == 0 {
		t.Errorf("GetNameList = %+v", tl)
	}
	_ = e.CloseSession(ctx, ic.ID)

	// A public capture, captured midstream.
	pub := openTrace(t, e, fixture(t, "adsc-zeek_read.pcap"))
	kinds = findingKinds(findings(t, e, pub.ID))
	if !slices.Contains(kinds, "midstream") || !slices.Contains(kinds, "open-at-end") {
		t.Errorf("public capture findings = %v", kinds)
	}
	tl, _ = e.TraceTimeline(ctx, pub.ID, model.TimelineQuery{})
	if tl.Transactions[1].Objects[0].Kind != objDataSet || tl.Transactions[1].Objects[0].Ref != "WAGO61850ServerLogicalDevice/LLN0.VSD1" {
		t.Errorf("data set read = %+v", tl.Transactions[1].Objects)
	}
	_ = e.CloseSession(ctx, pub.ID)

	// GOOSE: the libiec61850 publisher.
	g := openTrace(t, e, fixture(t, "libiec61850_goose-publisher.pcap"))
	streams, _ = e.TraceStreams(ctx, g.ID)
	if len(streams.GOOSE) != 1 || streams.GOOSE[0].GocbRef != "InteropIEDInteropLD/LLN0$GO$gcbAll" || streams.GOOSE[0].Changes != 1 {
		t.Errorf("GOOSE = %+v", streams.GOOSE)
	}
}

// TestTraceDamagedAndInvalid: a damaged capture opens with what was read;
// what is not a capture, too large or badly asked is refused.
func TestTraceDamagedAndInvalid(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	d := openTrace(t, e, fixture(t, "damaged.pcap"))
	if !d.Trace.Damaged {
		t.Errorf("damaged = %+v", d.Trace)
	}
	fs := findings(t, e, d.ID)
	if fs[0].Kind != "damaged" || fs[0].Severity != model.SeverityError || fs[0].Source != "trace.ErrCorrupt" || fs[0].Frame != 10 {
		t.Errorf("damage finding = %+v", fs[0])
	}
	if sum, _ := e.TraceSummary(ctx, d.ID); !sum.File.Damaged || sum.File.Damage == "" || sum.Associations != 1 {
		t.Errorf("damaged summary = %+v", sum.File)
	}

	// gzip.
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(fixture(t, "substation.pcap").Data)
	_ = zw.Close()
	if info := openTrace(t, e, model.File{Name: "substation.pcap.gz", Data: gz.Bytes()}); !info.Trace.Gzip || info.Trace.Packets != 61 {
		t.Errorf("gzip trace = %+v", info.Trace)
	}

	for name, req := range map[string]model.OpenTraceRequest{
		"empty":      {File: model.File{Name: "x.pcap"}},
		"not pcap":   {File: model.File{Name: "x.pcap", Data: []byte("this is not a capture")}},
		"bad gzip":   {File: model.File{Name: "x.gz", Data: []byte{0x1f, 0x8b, 0, 0, 0}}},
		"port 0":     {File: fixture(t, "substation.pcap"), Ports: []int{0}},
		"port 70000": {File: fixture(t, "substation.pcap"), Ports: []int{70000}},
		"many ports": {File: fixture(t, "substation.pcap"), Ports: make([]int, model.MaxTracePorts+1)},
	} {
		if _, err := e.OpenTrace(ctx, req); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	big := model.OpenTraceRequest{File: model.File{Name: "big.pcap", Data: make([]byte, model.MaxTraceBytes+1)}}
	if _, err := e.OpenTrace(ctx, big); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("over limit: %v", err)
	}
	// The number of trace sessions is bounded.
	for e.traceSessions() < model.MaxTraceSessions {
		openTrace(t, e, fixture(t, "adsc-zeek_read.pcap"))
	}
	if _, err := e.OpenTrace(ctx, model.OpenTraceRequest{File: fixture(t, "adsc-zeek_read.pcap")}); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("one trace too many: %v", err)
	}

	// Paging and filters are checked.
	for name, call := range map[string]func() error{
		"offset": func() error { _, err := e.TraceTimeline(ctx, d.ID, model.TimelineQuery{Offset: -1}); return err },
		"limit": func() error {
			_, err := e.TraceTimeline(ctx, d.ID, model.TimelineQuery{Limit: model.MaxTracePage + 1})
			return err
		},
		"status": func() error {
			_, err := e.TraceTimeline(ctx, d.ID, model.TimelineQuery{Status: []string{"lost"}})
			return err
		},
		"association": func() error { _, err := e.TraceTimeline(ctx, d.ID, model.TimelineQuery{Association: -1}); return err },
		"window": func() error {
			now := time.Now()
			before := now.Add(-time.Second)
			_, err := e.TraceTimeline(ctx, d.ID, model.TimelineQuery{From: &now, To: &before})
			return err
		},
		"severity": func() error { _, err := e.TraceFindings(ctx, d.ID, model.FindingsQuery{Severity: "fatal"}); return err },
		"findings": func() error { _, err := e.TraceFindings(ctx, d.ID, model.FindingsQuery{Limit: -1}); return err },
		"reports":  func() error { _, err := e.TraceReports(ctx, d.ID, model.TraceReportsQuery{Offset: -1}); return err },
		"report assoc": func() error {
			_, err := e.TraceReports(ctx, d.ID, model.TraceReportsQuery{Association: -1})
			return err
		},
		"self link": func() error { _, err := e.LinkTrace(ctx, d.ID, model.LinkTraceRequest{Session: d.ID}); return err },
	} {
		if err := call(); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	other := openSimSCL(t, e)
	for name, call := range map[string]func() error{
		"summary of SCL": func() error { _, err := e.TraceSummary(ctx, other.ID); return err },
		"assocs of SCL":  func() error { _, err := e.TraceAssociations(ctx, other.ID); return err },
		"streams of SCL": func() error { _, err := e.TraceStreams(ctx, other.ID); return err },
		"browse trace":   func() error { _, err := e.Browse(ctx, d.ID, ""); return err },
		"read trace":     func() error { _, err := e.Read(ctx, d.ID, model.ReadRequest{Refs: []string{"a/b.c[ST]"}}); return err },
		"link to trace": func() error {
			_, err := e.LinkTrace(ctx, d.ID, model.LinkTraceRequest{Session: openedTrace(e, d.ID)})
			return err
		},
		"export points": func() error {
			_, err := e.Export(ctx, d.ID, model.ExportRequest{Format: model.ExportPointsCSV})
			return err
		},
		"export trace": func() error {
			_, err := e.Export(ctx, other.ID, model.ExportRequest{Format: model.ExportFindingsCSV})
			return err
		},
		"wire offline": func() error { _, err := e.Wire(ctx, other.ID, 1); return err },
	} {
		if err := call(); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, call := range map[string]func() error{
		"unknown trace": func() error { _, err := e.TraceSummary(ctx, "nope"); return err },
		"unknown link":  func() error { _, err := e.LinkTrace(ctx, d.ID, model.LinkTraceRequest{Session: "nope"}); return err },
		"wire unknown":  func() error { _, err := e.Wire(ctx, "nope", 1); return err },
	} {
		if err := call(); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Closing a trace forgets it.
	if err := e.CloseSession(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TraceSummary(ctx, d.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("closed trace: %v", err)
	}
}

// openedTrace returns another open trace session than id.
func openedTrace(e *Engine, id string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for sid, s := range e.sessions {
		if s.trace != nil && sid != id {
			return sid
		}
	}
	return ""
}

func (e *Engine) traceSessions() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.traceCount()
}

// TestTraceExports: findings and timeline as CSV and JSON, with cells
// protected against formula injection.
func TestTraceExports(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	tr := openTrace(t, e, fixture(t, "substation.pcap"))
	if _, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: openSimSCL(t, e).ID}); err != nil {
		t.Fatal(err)
	}
	csvF, err := e.Export(ctx, tr.ID, model.ExportRequest{Format: model.ExportFindingsCSV})
	if err != nil || csvF.Name != "substation-findings.csv" || csvF.MIME != "text/csv" ||
		!strings.HasPrefix(csvF.Text, "severity,kind,association,frame,time,transaction,stream,source,count,message\n") ||
		!strings.Contains(csvF.Text, "not-in-model") {
		t.Fatalf("findings CSV = %+v, %v", csvF, err)
	}
	jsonF, err := e.Export(ctx, tr.ID, model.ExportRequest{Format: model.ExportFindingsJSON})
	var doc traceExport
	if err != nil || json.Unmarshal([]byte(jsonF.Text), &doc) != nil || doc.Schema != TraceExportSchema || len(doc.Findings) == 0 || doc.Link == nil {
		t.Fatalf("findings JSON = %v, %v", jsonF.Name, err)
	}
	tlCSV, err := e.Export(ctx, tr.ID, model.ExportRequest{Format: model.ExportTimelineCSV})
	if err != nil || strings.Count(tlCSV.Text, "\n") != 10 || !strings.Contains(tlCSV.Text, "LD0/MMXU1.Hz[MX]") || !strings.Contains(tlCSV.Text, ",missing") {
		t.Fatalf("timeline CSV = %q, %v", tlCSV.Text, err)
	}
	tlJSON, err := e.Export(ctx, tr.ID, model.ExportRequest{Format: model.ExportTimelineJSON})
	if err != nil || json.Unmarshal([]byte(tlJSON.Text), &doc) != nil || len(doc.Transactions) != 9 || tlJSON.Name != "substation-timeline.json" {
		t.Fatalf("timeline JSON = %v, %v", tlJSON.Name, err)
	}
	if downloadBase("=cmd|' /C calc'!A0") != "_cmd____C_calc__A0" || downloadBase("..") != "trace" {
		t.Error("downloadBase")
	}
	if csvRow([]string{"=HYPERLINK(1)"})[0] != "'=HYPERLINK(1)" {
		t.Error("CSV cells are not protected")
	}
}

// TestWireView: an operation of a live session links to the MMS units it
// sent and received, decoded, with the invoke IDs of its requests.
func TestWireView(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	connect := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "connect"
	}).Operation
	if _, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); err != nil {
		t.Fatal(err)
	}
	read := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "read"
	}).Operation
	if read.Wire == 0 || connect.Wire == 0 || read.Wire <= connect.Wire {
		t.Fatalf("wire numbers: connect %d, read %d", connect.Wire, read.Wire)
	}

	cv, err := e.Wire(ctx, info.ID, connect.Wire)
	if err != nil || len(cv.Units) < 2 {
		t.Fatalf("connect units = %+v, %v", cv, err)
	}
	if u := cv.Units[0]; u.Direction != "send" || u.Association != wireObserve || u.Session != "connect" || u.ACSE != "AARQ" || u.Hex == "" ||
		u.PDU == nil || u.PDU.Kind != "initiate-request" {
		t.Errorf("AARQ unit = %+v", u)
	}

	wv, err := e.Wire(ctx, info.ID, read.Wire)
	if err != nil || wv.Operation != read.Wire || len(wv.Units) < 2 {
		t.Fatalf("read units = %+v, %v", wv, err)
	}
	ids := map[uint32]bool{}
	for _, r := range read.Requests {
		ids[r.InvokeID] = true
	}
	var req, resp *model.WireUnit
	for i, u := range wv.Units {
		if u.PDU == nil || u.PDU.InvokeID == nil || !ids[*u.PDU.InvokeID] || u.PDU.Service != "Read" {
			continue
		}
		if u.Direction == "send" {
			req = &wv.Units[i]
		} else {
			resp = &wv.Units[i]
		}
	}
	if req == nil || resp == nil || req.PDU.Kind != "confirmed-request" || req.PDU.Objects[0].Ref != simied.Domain+"/MMXU1.Hz[MX]" ||
		resp.PDU.Kind != "confirmed-response" || len(resp.PDU.Values) != 1 || resp.Bytes == 0 {
		t.Fatalf("read request %+v, response %+v", req, resp)
	}

	// Poll cycles are not operations: their units are not kept.
	if err := e.StartPolling(ctx, info.ID, model.PollConfig{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}, IntervalMS: 200}); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventSample })
	_ = e.StopPolling(ctx, info.ID)
	s, _ := e.session(info.ID)
	s.rec.wire.mu.Lock()
	for _, u := range s.rec.wire.units {
		if u.op != connect.Wire && u.op != read.Wire && u.op > read.Wire {
			t.Errorf("a unit of an unpublished action %d is kept", u.op)
		}
	}
	s.rec.wire.mu.Unlock()

	for _, op := range []uint64{0, 1 << 40} {
		if _, err := e.Wire(ctx, info.ID, op); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("Wire(%d) = %v", op, err)
		}
	}
}

// TestWireRing: the ring keeps the newest units within its bounds, units
// too large are kept as their size, and hex is cut.
func TestWireRing(t *testing.T) {
	rec := &recorder{wire: &wireRing{}}
	hook := rec.hook(wireObserve)
	hook("send", []byte{1}) // no action running: not kept
	rec.start()
	for range model.MaxWireUnits + 10 {
		hook("send", []byte{0x01, 0x00})
	}
	first := rec.commit()
	rec.stop()
	if units, evicted := rec.wire.of(first); len(units) != model.MaxWireUnits || evicted {
		t.Fatalf("first op: %d units, evicted %v", len(units), evicted)
	}
	rec.start()
	hook("recv", make([]byte, model.MaxWireUnitBytes+1))
	hook("recv", make([]byte, model.MaxWireHexBytes+10))
	second := rec.commit()
	rec.stop()
	units, _ := rec.wire.of(second)
	big, mid := wireUnitOf(units[0]), wireUnitOf(units[1])
	if big.Hex != "" || !strings.Contains(big.DecodeError, "not kept") || big.Bytes != model.MaxWireUnitBytes+1 {
		t.Errorf("big unit = %+v", big)
	}
	if !mid.Truncated || len(mid.Hex) != 2*model.MaxWireHexBytes || mid.DecodeError == "" {
		t.Errorf("unit = %+v", mid)
	}
	if _, evicted := rec.wire.of(first); !evicted {
		t.Error("the first op's units are not reported as evicted")
	}
	if (&recorder{}).hook(wireObserve) != nil || (&recorder{}).commit() != 0 {
		t.Error("a recorder without a ring records")
	}
	e := newEngine(t)
	if v, err := e.Wire(t.Context(), "x", 0); err == nil {
		t.Errorf("Wire = %+v", v)
	}
}

// namedComponents says whether every component of a structure, at every
// level, has a name.
func namedComponents(v model.Value) bool {
	if v.Type != model.TypeStructure || len(v.Children) == 0 {
		return v.Type != model.TypeStructure
	}
	for _, c := range v.Children {
		if c.Name == "" || !namedComponents(c) {
			return false
		}
	}
	return true
}

func childNamed(v model.Value, name string) *model.Value {
	for i := range v.Children {
		if v.Children[i].Name == name {
			return &v.Children[i]
		}
	}
	return nil
}

// TestTraceDemoReportComponents: the station fault capture of the demo set,
// linked to feeder 1's CID, shows the captured reports' structures with the
// names of the model, the quality and time decoded; reads of the linked
// IED are named too, those of other IEDs are not.
func TestTraceDemoReportComponents(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "demo", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	tr := openTrace(t, e, model.File{Name: "07-station-fault.pcapng", Data: read("captures/07-station-fault.pcapng")})
	cid, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "AA1D1Q02A1.cid", Data: read("scl/AA1D1Q02A1.cid")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: cid.ID}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.TraceReports(ctx, tr.ID, model.TraceReportsQuery{Limit: model.MaxTracePage})
	if err != nil {
		t.Fatal(err)
	}
	strs := 0
	for _, r := range rep.Reports {
		for _, en := range r.Entries {
			if !namedComponents(en.Value) {
				t.Errorf("%s: unnamed components %+v", en.Ref, en.Value)
			}
			if en.Ref != "AA1D1Q02A1PROT/PTOC1.Str[ST]" {
				continue
			}
			strs++
			var names []string
			for _, c := range en.Value.Children {
				names = append(names, c.Name)
			}
			want := []string{"general", "dirGeneral", "phsA", "dirPhsA", "phsB", "dirPhsB", "phsC", "dirPhsC", "q", "t"}
			if !slices.Equal(names, want) {
				t.Errorf("PTOC1.Str components %v, want %v", names, want)
			}
			if q := childNamed(en.Value, "q"); q == nil || q.Quality == nil || q.Text != "good" {
				t.Errorf("PTOC1.Str q = %+v", q)
			}
			if ts := childNamed(en.Value, "t"); ts == nil || ts.Time == nil {
				t.Errorf("PTOC1.Str t = %+v", ts)
			}
		}
	}
	if strs == 0 {
		t.Fatal("no PTOC1.Str entry in the decoded reports")
	}

	tl, err := e.TraceTimeline(ctx, tr.ID, model.TimelineQuery{Service: "Read", Limit: model.MaxTracePage})
	if err != nil {
		t.Fatal(err)
	}
	named, unnamed := 0, 0
	for _, x := range tl.Transactions {
		if x.Response == nil || len(x.Response.Values) == 0 || len(x.Objects) == 0 {
			continue
		}
		v := x.Response.Values[0]
		switch {
		case strings.HasPrefix(x.Objects[0].Ref, "AA1D1Q02A1"):
			if v.Type == model.TypeStructure && (!namedComponents(v) || childNamed(v, "q") == nil) {
				t.Errorf("read of %s: %+v", x.Objects[0].Ref, v)
			}
			named++
		case v.Type == model.TypeStructure:
			if len(v.Children) > 0 && v.Children[0].Name != "" {
				t.Errorf("read of %s, not in the linked model, is named: %+v", x.Objects[0].Ref, v)
			}
			unnamed++
		}
	}
	if named == 0 || unnamed == 0 {
		t.Errorf("reads named %d, unnamed %d", named, unnamed)
	}
}
