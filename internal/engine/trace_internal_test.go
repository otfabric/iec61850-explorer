// SPDX-License-Identifier: MIT

package engine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms-trace/trace"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func TestObjectOf(t *testing.T) {
	dom := func(d, item string) mms.ObjectName {
		return mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: mms.DomainID(d), ItemID: mms.ItemID(item)}
	}
	for _, c := range []struct {
		name      mms.ObjectName
		list      bool
		ref, kind string
		scope     string
	}{
		{dom("LD", "LLN0$ST$Mod$stVal"), false, "LD/LLN0.Mod.stVal[ST]", objDA, "domain"},
		{dom("LD", "MMXU1$MX$Hz"), false, "LD/MMXU1.Hz[MX]", objDO, "domain"},
		{dom("LD", "MMXU1"), false, "LD/MMXU1", objLN, "domain"},
		{dom("LD", "MMXU1$MX"), false, "LD/MMXU1[MX]", "", "domain"},
		{dom("LD", "LLN0$RP$urcb01$RptEna"), false, "LD/LLN0.urcb01.RptEna[RP]", objRCB, "domain"},
		{dom("LD", "LLN0$BR$brcb01"), false, "LD/LLN0.brcb01[BR]", objRCB, "domain"},
		{dom("LD", "LLN0$GO$gcb1"), false, "LD/LLN0.gcb1[GO]", objGoCB, "domain"},
		{dom("LD", "LLN0$LG$log1"), false, "LD/LLN0.log1[LG]", objLog, "domain"},
		{dom("LD", "LLN0$SP$SGCB$ActSG"), false, "LD/LLN0.SGCB.ActSG[SP]", objSGCB, "domain"},
		{dom("LD", "LLN0$XX$Mod"), false, "", "", "domain"},
		{dom("LD", "LLN0$$Mod"), false, "", "", "domain"},
		{dom("LD", "LLN0$Meas"), true, "LD/LLN0.Meas", objDataSet, "domain"},
		{dom("LD", "Meas"), true, "", "", "domain"},
		{mms.ObjectName{Scope: mms.ObjectScopeVMD, ItemID: "RPT"}, true, "", "", "vmd"},
		{mms.ObjectName{Scope: mms.ObjectScopeAssociation, ItemID: "@x"}, true, "", "", "aa"},
		{mms.ObjectName{Scope: 7, ItemID: "x"}, false, "", "", "scope 7"},
	} {
		o := objectOf(c.name, c.list)
		if o.Ref != c.ref || o.Kind != c.kind || o.Scope != c.scope || o.List != c.list {
			t.Errorf("objectOf(%+v, %v) = %+v", c.name, c.list, o)
		}
	}
	if o := domainObject("LD"); o.Ref != "LD" || o.Kind != objLD {
		t.Errorf("domainObject = %+v", o)
	}
	for _, k := range []string{objLD, objLN, objDO, objDA, objDataSet, objRCB, objGoCB, objSGCB, objLog, "x"} {
		if kindName(k) == "" {
			t.Errorf("kindName(%q) is empty", k)
		}
	}
}

func TestPDUOf(t *testing.T) {
	if pduOf(nil) != nil {
		t.Error("pduOf(nil)")
	}
	var vars []mms.CapturedVariable
	for range model.MaxTraceObjects + 2 {
		vars = append(vars, mms.CapturedVariable{Name: mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "MMXU1$MX$Hz"}})
	}
	var vals []*mms.Value
	for range model.MaxTraceValues + 1 {
		vals = append(vals, mms.NewInteger(1))
	}
	var names []string
	for range model.MaxTraceObjects + 1 {
		names = append(names, "n")
	}
	p := pduOf(&mms.CapturedPDU{
		Kind: "confirmed-response", HasInvokeID: true, InvokeID: 7, Service: "Write", Variables: vars, Values: vals,
		WriteResults: []*mms.DataAccessError{nil, {Code: mms.DataAccessErrorHardwareFault}}, NameList: names,
		Identify: &mms.CapturedIdentify{Vendor: "v"}, Initiate: &mms.CapturedInitiate{Version: 1},
		Error: &mms.CapturedServiceError{Class: 2, Code: 1}, Raw: make([]byte, maxRawShown+1),
		NamedListMembers: []mms.CapturedVariable{{Name: mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "LLN0$ST$Mod"}}},
		VariableName:     &mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "LLN0"},
		NameListRequest:  &mms.CapturedNameListRequest{Scope: mms.ObjectScopeDomain, Domain: "LD"},
	})
	if *p.InvokeID != 7 || len(p.Objects) != model.MaxTraceObjects || p.ObjectsDropped != 5 || p.ValuesDropped != 1 ||
		!slices.Equal(p.WriteResults[:1], []string{"ok"}) || p.WriteResults[1] == "" || !p.NameListMore || p.Identity.Vendor != "v" ||
		p.Initiate.Version != 1 || p.Error != "error class 2, code 1" || len(p.Raw) != 2*maxRawShown {
		t.Errorf("pduOf = %+v", p)
	}
	if e := pduError(&mms.CapturedPDU{Reject: &mms.CapturedReject{Type: 1, Reason: 2}}); e != "reject: PDU type 1, reason 2" {
		t.Errorf("reject = %q", e)
	}
}

// report builds the values of an IEC 61850 report.
func report(opt []byte, fields ...*mms.Value) []*mms.Value {
	return append([]*mms.Value{mms.NewVisibleString("R"), mms.NewBitStringWithLength(opt, 10)}, fields...)
}

func TestReportLayout(t *testing.T) {
	incl := mms.NewBitStringWithLength([]byte{0xa0}, 3) // members 1 and 3
	// OptFlds: seq-num, timestamp, reason-code, data-set, data-ref, buf-ovfl, entry-id, conf-rev, segmentation.
	all := report([]byte{0x7f, 0xc0},
		mms.NewUnsigned(5), mms.NewVisibleString("t"), mms.NewVisibleString("LD/LLN0$DS"), mms.NewBoolean(false),
		mms.NewVisibleString("e"), mms.NewUnsigned(3), mms.NewUnsigned(0), mms.NewBoolean(false),
		incl, mms.NewVisibleString("LD/A$ST$x"), mms.NewVisibleString("LD/C$ST$z"), mms.NewInteger(1), mms.NewInteger(3),
		mms.NewBitStringWithLength([]byte{0x40}, 7), mms.NewBitStringWithLength([]byte{0x04}, 7))
	r, err := reportLayout(all)
	if err != nil || r.rptID != "R" || r.seqNum != 5 || r.datSet != "LD/LLN0$DS" || r.confRev != 3 || len(r.values) != 2 ||
		!slices.Equal(r.inclusion, []bool{true, false, true}) || len(r.dataRefs) != 2 || r.reasons[0].String() != "data-change" ||
		r.reasons[1].String() != "GI" {
		t.Fatalf("layout = %+v, %v", r, err)
	}
	for name, values := range map[string][]*mms.Value{
		"empty":         nil,
		"no OptFlds":    {mms.NewVisibleString("R")},
		"RptID":         {mms.NewInteger(1), mms.NewBitStringWithLength([]byte{0, 0}, 10)},
		"OptFlds type":  {mms.NewVisibleString("R"), mms.NewInteger(1)},
		"OptFlds short": {mms.NewVisibleString("R"), mms.NewBitStringWithLength([]byte{0}, 8)},
		"no seq":        report([]byte{0x40, 0}),
		"no inclusion":  report([]byte{0, 0}),
		"inclusion":     report([]byte{0, 0}, mms.NewInteger(1)),
		"no ref":        report([]byte{0x08, 0}, incl),
		"no value":      report([]byte{0, 0}, incl, mms.NewInteger(1)),
		"no reason":     report([]byte{0x20, 0}, incl, mms.NewInteger(1), mms.NewInteger(2)),
		"reason type":   report([]byte{0x20, 0}, incl, mms.NewInteger(1), mms.NewInteger(2), mms.NewInteger(1), mms.NewInteger(1)),
		"left over":     report([]byte{0, 0}, incl, mms.NewInteger(1), mms.NewInteger(2), mms.NewInteger(3)),
	} {
		if _, err := reportLayout(values); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestReportOf(t *testing.T) {
	ds := model.DataSet{Ref: "LD/LLN0.DS", MMS: model.MMSName{Domain: "LD", ItemID: "LLN0$DS"},
		Members: []model.DataSetMember{{Ref: "LD/A.x[ST]"}, {Ref: "LD/B.y[ST]"}, {Ref: "LD/C.z[ST]"}}}
	l := &traceLink{dataSets: map[string]model.DataSet{"LD/LLN0$DS": ds}, dataSetsKnown: true, rcbsKnown: true,
		rcbs: []model.RCB{{Ref: "LD/LLN0.urcb", MMS: model.MMSName{Domain: "LD", ItemID: "LLN0$RP$urcb"}, DatSet: "LD/LLN0$DS"}}}
	rpt := func(values ...*mms.Value) *trace.Report {
		return &trace.Report{PDU: &mms.CapturedPDU{ListName: &mms.ObjectName{Scope: mms.ObjectScopeVMD, ItemID: "RPT"}, Values: values}}
	}
	incl := mms.NewBitStringWithLength([]byte{0x40}, 3)
	// An RptID left empty: the IED sends the RCB's reference.
	r := reportOf(1, rpt(report([]byte{0, 0}, incl, mms.NewInteger(9))...), l)
	if !r.Decoded && r.Note == "" {
		t.Errorf("report = %+v", r)
	}
	l.rcbs[0].RptID = ""
	vals := append([]*mms.Value{mms.NewVisibleString("LD/LLN0.urcb"), mms.NewBitStringWithLength([]byte{0x00, 0x80}, 10)}, mms.NewUnsigned(2), incl, mms.NewInteger(9))
	if r := reportOf(1, rpt(vals...), l); !r.Decoded || r.RCB != "LD/LLN0.urcb" || r.Entries[0].Ref != "LD/B.y[ST]" || r.ConfRev == nil {
		t.Errorf("report by RCB reference = %+v", r)
	}
	for name, c := range map[string]struct {
		r    *trace.Report
		note string
	}{
		"no PDU":     {&trace.Report{}, "not captured"},
		"not RPT":    {&trace.Report{PDU: &mms.CapturedPDU{Variables: []mms.CapturedVariable{{Name: mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "LLN0$ST$Mod"}}}, Values: []*mms.Value{mms.NewInteger(1)}}}, "not an IEC 61850 report"},
		"bad layout": {rpt(mms.NewInteger(1)), "do not follow"},
		"no RCB":     {rpt(report([]byte{0, 0}, incl, mms.NewInteger(9))...), "no report control block"},
		"no data set": {rpt(mms.NewVisibleString("X"), mms.NewBitStringWithLength([]byte{0x08, 0}, 10), mms.NewVisibleString("LD/LLN0$Other"),
			incl, mms.NewInteger(9)), "not in the linked model"},
		"bits": {rpt(mms.NewVisibleString("X"), mms.NewBitStringWithLength([]byte{0x08, 0}, 10), mms.NewVisibleString("LD/LLN0$DS"),
			mms.NewBitStringWithLength([]byte{0x80}, 1), mms.NewInteger(9)), "inclusion bit string has 1 bits"},
	} {
		if r := reportOf(1, c.r, l); r.Decoded || !strings.Contains(r.Note, c.note) {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestGooseModel(t *testing.T) {
	g := &trace.GooseStream{GocbRef: "LD/LLN0$GO$gcb1", DatSet: "LD/LLN0$ds1", Messages: 3}
	var none *traceLink
	if m, _ := none.gooseModel(g); m != "" {
		t.Errorf("no link: %q", m)
	}
	l := &traceLink{}
	if m, note := l.gooseModel(g); m != resolveUnknown || note == "" {
		t.Errorf("no GoCBs: %q", m)
	}
	if m, _ := l.gooseModel(&trace.GooseStream{GocbRef: "x"}); m != model.ResolveNotIEC61850 {
		t.Errorf("bad gocbRef: %q", m)
	}
	l.gocbsKnown = true
	if m, _ := l.gooseModel(g); m != model.ResolveMissing {
		t.Errorf("missing: %q", m)
	}
	l.gocbs = []gocbDef{{domain: "LD", item: "LLN0$GO$gcb1", datSet: "LD/LLN0$ds2"}}
	if m, note := l.gooseModel(g); m != model.ResolveFound || !strings.Contains(note, "LD/LLN0$ds2") {
		t.Errorf("other data set: %q %q", m, note)
	}
	l.gocbs[0].datSet = g.DatSet
	if m, note := l.gooseModel(g); m != model.ResolveFound || note != "" {
		t.Errorf("found: %q %q", m, note)
	}
	// A GoCB object of a transaction.
	o := objectOf(mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "LLN0$GO$gcb1$GoEna"}, false)
	l.resolve(&o)
	if o.Model != model.ResolveFound || len(o.Concerns) != 1 {
		t.Errorf("GoCB object = %+v", o)
	}
	o = objectOf(mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: "LD", ItemID: "LLN0$GO$gcb2"}, false)
	l.resolve(&o)
	if o.Model != model.ResolveMissing {
		t.Errorf("other GoCB = %+v", o)
	}
	l.gocbsKnown = false
	l.resolve(&o)
	if o.Model != resolveUnknown {
		t.Errorf("unknown GoCB = %+v", o)
	}
	for _, kind := range []string{objRCB, objGoCB} {
		o := model.TraceObject{Ref: "LD/LLN0.x[RP]", Kind: kind, MMS: model.MMSName{Domain: "LD", ItemID: "LLN0"}}
		l.resolve(&o)
		if o.Model != model.ResolveNotIEC61850 {
			t.Errorf("%s with a short item = %+v", kind, o)
		}
	}
	o = model.TraceObject{Ref: "not a ref[", Kind: objDA}
	l.resolve(&o)
	if o.Model != model.ResolveNotIEC61850 {
		t.Errorf("bad ref = %+v", o)
	}
	o = model.TraceObject{Ref: "LD", Kind: objLD}
	l.resolve(&o)
	if o.Model != model.ResolveMissing {
		t.Errorf("unknown LD = %+v", o)
	}
	o = model.TraceObject{Ref: "LD/LLN0.DS", Kind: objDataSet, MMS: model.MMSName{Domain: "LD", ItemID: "LLN0$DS"}}
	l.resolve(&o)
	if o.Model != resolveUnknown {
		t.Errorf("data set without definitions = %+v", o)
	}
}

// TestSCLGoCBs: GOOSE control blocks of an SCL file are linked.
func TestSCLGoCBs(t *testing.T) {
	icd := strings.Replace(string(simied.ICD()), `<DataSet name="Meas"`, `<GSEControl name="gcb1" appID="Goose1" datSet="Meas"/>`+"\n"+`<DataSet name="Meas"`, 1)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "gse.icd", Data: []byte(icd)}})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.session(info.ID)
	if len(s.sclGoCBs) != 1 || s.sclGoCBs[0].item != "LLN0$GO$gcb1" || s.sclGoCBs[0].datSet != simied.Domain+"/LLN0$Meas" || s.sclGoCBs[0].goID != "Goose1" {
		t.Fatalf("GoCBs = %+v", s.sclGoCBs)
	}
	tr := openTrace(t, e, fixture(t, "substation.pcap"))
	linked, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: info.ID})
	if err != nil || linked.Trace.Link.GoCBs != 1 || !strings.HasPrefix(linked.Trace.Link.Label, "SCL gse.icd") {
		t.Fatalf("link = %+v, %v", linked.Trace.Link, err)
	}
}

// TestLinkLive: a live link reads the GoCBs its model lists; a lost
// session cannot be linked.
func TestLinkLive(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	defs := e.liveGoCBs(ctx, info.ID, []string{simied.Domain + "/LLN0.gcbX", "bad ref["})
	if len(defs) != 1 || defs[0].item != "LLN0$GO$gcbX" || defs[0].datSet != "" {
		t.Errorf("GoCBs = %+v", defs)
	}
	if l := linkLabel(info); !strings.Contains(l, "127.0.0.1") {
		t.Errorf("label = %q", l)
	}
	tr := openTrace(t, e, fixture(t, "substation.pcap"))
	ied.Close()
	waitLost(t, e, info.ID)
	if _, err := e.LinkTrace(ctx, tr.ID, model.LinkTraceRequest{Session: info.ID}); err == nil {
		t.Error("linked a lost session")
	}
	if l := linkLabel(model.SessionInfo{Target: model.TargetConfig{Host: "h", Port: 1}}); l != "h:1" {
		t.Errorf("label = %q", l)
	}
	// The data sets of a logical device that fails are noted.
	l := &traceLink{dataSets: map[string]model.DataSet{}}
	l.collect(ctx, e, info.ID, []string{simied.Domain})
	if l.dataSetsKnown || l.rcbsKnown || len(l.info.Notes) != 2 {
		t.Errorf("collect on a lost session = %+v", l)
	}
}

func waitLost(t *testing.T, e *Engine, id string) {
	t.Helper()
	for range 200 {
		if s, err := e.session(id); err == nil && s.info().State == model.SessionLost {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the session was not lost")
}

// TestCaptureFindings: every counter, issue and status becomes a finding
// with its source.
func TestCaptureFindings(t *testing.T) {
	no := false
	res := &trace.Result{
		File: trace.File{TruncatedTail: true, SkippedBlocks: 2},
		Associations: []*trace.Association{
			{ID: 1, Open: "connect", Close: "reset", Accepted: &no},
			{ID: 2, Open: "associate", Close: "abort"},
		},
		Transactions: []*trace.Transaction{
			{Association: 1, Status: "reject", Response: &mms.CapturedPDU{Reject: &mms.CapturedReject{}, DecodeError: "x"}, ResponseFrame: 4},
			{Association: 1, Status: "ok", Service: "Read", Request: &mms.CapturedPDU{DecodeError: "y"}},
		},
		Reports: []*trace.Report{{PDU: &mms.CapturedPDU{DecodeError: "z"}}},
		Counters: trace.Counters{Unsupported: map[string]uint64{"not_ip": 1}, TPKTResync: 1, COTPErrors: 1, TSDUErrors: 1,
			BareMMS: 1, TSOSegments: 1, GOOSEErrors: 1, SVErrors: 1},
		GOOSE: []*trace.GooseStream{{GocbRef: "g", Issues: map[string]int{"sqNum gap": 1, "something new": 1}, Simulation: 2}},
		SV:    []*trace.SVStream{{SvID: "s", Lost: 3, Repeated: 1}},
	}
	kinds := findingKinds(captureFindings(res, nil))
	for _, k := range []string{"truncated-tail", "skipped-blocks", "no-associate", "association-rejected", "association-reset",
		"association-abort", "reject", "decode-error", "tpkt-resync", "cotp-errors", "tsdu-errors", "bare-mms", "tso-segments",
		"goose-errors", "sv-errors", "unsupported", "goose-sqnum-gap", "goose-issue", "goose-simulation", "sv-lost", "sv-repeated"} {
		if !slices.Contains(kinds, k) {
			t.Errorf("no %s in %v", k, kinds)
		}
	}
	var l findingList
	for range maxCaptureFindings + 3 {
		l.add(model.TraceFinding{Kind: "x"})
	}
	out := l.done()
	if len(out) != maxCaptureFindings+1 || out[len(out)-1].Kind != "findings-dropped" || out[len(out)-1].Count != 3 {
		t.Errorf("bounded findings: %d, last %+v", len(out), out[len(out)-1])
	}
	if serviceOf(&trace.Transaction{}) != "unknown" {
		t.Error("serviceOf")
	}
	if (&findingList{}).done() == nil {
		t.Error("done() is nil")
	}
}
