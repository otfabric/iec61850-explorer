// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms-trace/trace"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// Correlation of a capture with a model session (application
// correlation): MMS names become IEC 61850 references and are looked up in
// the linked session's model, data sets and control blocks. The link keeps
// a copy of what the session had when it was linked, so closing that
// session later does not change the trace's view.

// resolveUnknown: the object can be neither found nor missed.
const resolveUnknown = model.ResolveUnknown

// maxConcerns bounds the data sets and control blocks named per object.
const maxConcerns = 6

// gocbDef is a GOOSE control block of a model.
type gocbDef struct {
	// item is the MMS item of the control block, "LLN0$GO$gcb1"; domain
	// its logical device.
	domain, item string
	datSet, goID string
}

// traceLink is what a trace is correlated with.
type traceLink struct {
	info model.TraceLink
	lds  map[string]bool
	// nodes maps an object reference without FC to the FCs it has.
	nodes map[string][]string
	// dataSets by the data set's MMS name, "LD/LLN0$Meas"; known says the
	// session could tell its data sets.
	dataSets      map[string]model.DataSet
	dataSetsKnown bool
	rcbs          []model.RCB
	rcbsKnown     bool
	gocbs         []gocbDef
	gocbsKnown    bool
	// findings are what the model does not have.
	findings []model.TraceFinding
}

func (ts *traceState) currentLink() *traceLink {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.link
}

func (ts *traceState) linkInfo() *model.TraceLink {
	l := ts.currentLink()
	if l == nil {
		return nil
	}
	info := l.info
	info.Notes = slices.Clone(info.Notes)
	return &info
}

// LinkTrace implements service.Service.
func (e *Engine) LinkTrace(ctx context.Context, id string, req model.LinkTraceRequest) (model.SessionInfo, error) {
	s, ts, err := e.traceOf(id)
	if err != nil {
		return model.SessionInfo{}, err
	}
	var link *traceLink
	if target := strings.TrimSpace(req.Session); target != "" {
		if target == id {
			return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "a trace cannot be linked to itself")
		}
		m, err := e.lookup(target)
		if err != nil {
			return model.SessionInfo{}, err
		}
		if m.trace != nil {
			return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "session %s is a trace session: link a live, SCL or snapshot session", target)
		}
		if link, err = e.buildLink(ctx, m); err != nil {
			return model.SessionInfo{}, err
		}
		link.findings = linkFindings(ts.res, link)
	}
	ts.mu.Lock()
	ts.link = link
	ts.mu.Unlock()
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	return info, nil
}

// buildLink copies what a model session knows: its model and, as far as
// it can tell them, its data sets and control blocks. For a live session
// that means reading them from the IED (Observe mode).
func (e *Engine) buildLink(ctx context.Context, m *session) (*traceLink, error) {
	info := m.info()
	l := &traceLink{
		info:  model.TraceLink{SessionID: m.id, Kind: info.Kind, Label: linkLabel(info), LinkedAt: time.Now().UTC(), Notes: []string{}},
		lds:   map[string]bool{},
		nodes: map[string][]string{}, dataSets: map[string]model.DataSet{},
	}
	var lds []string
	var gocbNodes []string
	m.mu.Lock()
	for key, n := range m.index {
		if key == "" {
			continue
		}
		fcs := make([]string, 0, len(n.FCs)+1)
		for _, fc := range n.FCs {
			fcs = append(fcs, string(fc))
		}
		if len(fcs) == 0 && n.FC != "" {
			fcs = append(fcs, string(n.FC))
		}
		l.nodes[key] = fcs
		if n.Reference.IsLD() {
			lds = append(lds, key)
			l.lds[key] = true
		}
		if len(n.Reference.Path) == 1 && slices.Contains(fcs, string(iec61850.FCGO)) {
			gocbNodes = append(gocbNodes, key)
		}
	}
	gocbs := slices.Clone(m.sclGoCBs)
	m.mu.Unlock()
	slices.Sort(lds)
	slices.Sort(gocbNodes)

	switch info.Kind {
	case model.SessionKindSnapshot:
		l.info.Notes = append(l.info.Notes, "a snapshot holds no data set or control block definitions: reports are shown as captured")
	case model.SessionKindSCL:
		l.collect(ctx, e, m.id, lds)
		l.gocbs, l.gocbsKnown = gocbs, true
		l.info.Notes = append(l.info.Notes, "data sets and control blocks come from the SCL file; indexed report control block instances are matched to their definition")
	default:
		if err := m.liveErr(); err != nil {
			return nil, err
		}
		l.collect(ctx, e, m.id, lds)
		l.gocbs = e.liveGoCBs(ctx, m.id, gocbNodes)
		l.gocbsKnown = len(l.gocbs) > 0
		if !l.gocbsKnown {
			l.info.Notes = append(l.info.Notes, "the IED's model lists no GOOSE control blocks")
		}
	}
	l.info.DataSets, l.info.RCBs, l.info.GoCBs = len(l.dataSets), len(l.rcbs), len(l.gocbs)
	return l, nil
}

// collect lists the data sets and report control blocks of every logical
// device (from the IED or the SCL file). A logical device that fails is
// noted, not fatal.
func (l *traceLink) collect(ctx context.Context, e *Engine, id string, lds []string) {
	l.dataSetsKnown, l.rcbsKnown = true, true
	for _, ld := range lds {
		dss, err := e.DataSets(ctx, id, ld)
		if err != nil {
			l.dataSetsKnown = false
			l.info.Notes = append(l.info.Notes, fmt.Sprintf("the data sets of %s could not be listed: %v", ld, err))
		}
		for _, ds := range dss {
			l.dataSets[ds.MMS.Domain+"/"+ds.MMS.ItemID] = ds
		}
		rcbs, err := e.Reports(ctx, id, ld)
		if err != nil {
			l.rcbsKnown = false
			l.info.Notes = append(l.info.Notes, fmt.Sprintf("the report control blocks of %s could not be listed: %v", ld, err))
		}
		l.rcbs = append(l.rcbs, rcbs...)
	}
}

// liveGoCBs reads the data set and GoID of the GOOSE control blocks a live
// model lists (FC GO, read in Observe mode).
func (e *Engine) liveGoCBs(ctx context.Context, id string, keys []string) []gocbDef {
	var out []gocbDef
	for len(keys) > 0 {
		n := min(len(keys), model.MaxReadRefs/2)
		refs := make([]string, 0, 2*n)
		for _, k := range keys[:n] {
			refs = append(refs, k+".DatSet[GO]", k+".GoID[GO]")
		}
		res, err := e.Read(ctx, id, model.ReadRequest{Refs: refs})
		for i, k := range keys[:n] {
			ref, perr := iec61850.ParseRef(k)
			if perr != nil {
				continue
			}
			d := gocbDef{domain: ref.LD, item: ref.LN + "$GO$" + ref.Path[0]}
			if err == nil && 2*i+1 < len(res.Values) {
				d.datSet, d.goID = textOf(res.Values[2*i].Value), textOf(res.Values[2*i+1].Value)
			}
			out = append(out, d)
		}
		keys = keys[n:]
	}
	return out
}

// sclGoCBs lists the GOOSE control blocks of an access point's logical
// devices (LLN0 GSEControl).
func sclGoCBs(ied *scl.IED, ap *scl.AccessPoint) []gocbDef {
	var out []gocbDef
	for _, ld := range ap.Server.LDevices {
		if ld.LN0 == nil {
			continue
		}
		domain := ldDomain(ied, ld)
		for _, gc := range ld.LN0.GSEControls {
			d := gocbDef{domain: domain, item: "LLN0$GO$" + gc.Name, goID: gc.AppID}
			if gc.DatSet != "" {
				d.datSet = domain + "/LLN0$" + gc.DatSet
			}
			out = append(out, d)
		}
	}
	return out
}

func linkLabel(info model.SessionInfo) string {
	switch {
	case info.Snapshot != nil:
		return "snapshot " + info.Snapshot.File
	case info.SCL != nil:
		return fmt.Sprintf("SCL %s (%s/%s)", info.SCL.File, info.SCL.IED, info.SCL.AccessPoint)
	case info.Identity != nil && info.Identity.Model != "":
		return fmt.Sprintf("%s:%d (%s)", info.Target.Host, info.Target.Port, info.Identity.Model)
	}
	return fmt.Sprintf("%s:%d", info.Target.Host, info.Target.Port)
}

// resolveAll resolves objects in place; a nil link leaves them as they are.
func (l *traceLink) resolveAll(objs []model.TraceObject) {
	if l == nil {
		return
	}
	for i := range objs {
		l.resolve(&objs[i])
	}
}

// resolve looks an object up in the linked model.
func (l *traceLink) resolve(o *model.TraceObject) {
	if o.Ref == "" {
		o.Model = model.ResolveNotIEC61850
		return
	}
	switch o.Kind {
	case objLD:
		o.Model = found(l.lds[o.Ref])
		return
	case objDataSet:
		if !l.dataSetsKnown {
			o.Model = resolveUnknown
			return
		}
		ds, ok := l.dataSets[o.MMS.Domain+"/"+o.MMS.ItemID]
		o.Model = found(ok)
		if ok {
			o.Concerns = append(o.Concerns, fmt.Sprintf("data set %s (%d members)", ds.Ref, len(ds.Members)))
			o.Concerns = append(o.Concerns, l.rcbsOf(ds.MMS.Domain+"/"+ds.MMS.ItemID)...)
		}
		return
	case objRCB:
		l.resolveRCB(o)
		return
	case objGoCB:
		l.resolveGoCB(o)
		return
	}
	ref, err := iec61850.ParseRef(o.Ref)
	if err != nil {
		o.Model = model.ResolveNotIEC61850
		return
	}
	fcs, ok := l.nodes[refKey(ref)]
	if ok && ref.FC != "" && len(fcs) > 0 && !slices.Contains(fcs, string(ref.FC)) {
		ok = false
	}
	o.Model = found(ok)
	if !ok || len(ref.Path) == 0 {
		return
	}
	do := iec61850.Ref{LD: ref.LD, LN: ref.LN, Path: ref.Path[:1]}
	o.Concerns = append(o.Concerns, "data object "+do.String())
	o.Concerns = append(o.Concerns, l.dataSetsWith(ref)...)
}

func found(ok bool) string {
	if ok {
		return model.ResolveFound
	}
	return model.ResolveMissing
}

// dataSetsWith names the data sets with a member that covers ref or lies
// below it (same FC), and their report control blocks.
func (l *traceLink) dataSetsWith(ref iec61850.Ref) []string {
	key := refKey(ref)
	var out []string
	names := make([]string, 0, len(l.dataSets))
	for k := range l.dataSets {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, name := range names {
		ds := l.dataSets[name]
		for _, m := range ds.Members {
			mr, err := iec61850.ParseRef(m.Ref)
			if err != nil || mr.FC != ref.FC {
				continue
			}
			mk := refKey(mr)
			if mk == key || strings.HasPrefix(key, mk+".") || strings.HasPrefix(mk, key+".") {
				out = append(out, "data set "+ds.Ref)
				out = append(out, l.rcbsOf(name)...)
				break
			}
		}
		if len(out) >= maxConcerns {
			return out[:maxConcerns]
		}
	}
	return out
}

// rcbsOf names the report control blocks of a data set (MMS name).
func (l *traceLink) rcbsOf(datSet string) []string {
	var out []string
	for _, r := range l.rcbs {
		if r.DatSet == datSet {
			out = append(out, "report control block "+r.Ref)
		}
	}
	return out
}

// rcbFor finds the report control block an MMS item names ("LLN0$RP$
// urcbMeas01"), as such or as an indexed instance of an SCL definition.
func (l *traceLink) rcbFor(domain, item string) (model.RCB, bool) {
	for _, r := range l.rcbs {
		if r.MMS.Domain == domain && r.MMS.ItemID == item {
			return r, true
		}
	}
	if n := len(item); n > 2 && isDigits(item[n-2:]) {
		for _, r := range l.rcbs {
			if r.Source == model.SourceSCL && r.MMS.Domain == domain && r.MMS.ItemID == item[:n-2] {
				return r, true
			}
		}
	}
	return model.RCB{}, false
}

func (l *traceLink) resolveRCB(o *model.TraceObject) {
	parts := strings.SplitN(o.MMS.ItemID, "$", 4)
	if len(parts) < 3 {
		o.Model = model.ResolveNotIEC61850
		return
	}
	r, ok := l.rcbFor(o.MMS.Domain, strings.Join(parts[:3], "$"))
	switch {
	case ok:
		o.Model = model.ResolveFound
		c := fmt.Sprintf("report control block %s", r.Ref)
		if r.RptID != "" {
			c += " (RptID " + r.RptID + ")"
		}
		o.Concerns = append(o.Concerns, c)
		if r.DatSet != "" {
			o.Concerns = append(o.Concerns, "data set "+refOfDatSet(r.DatSet))
		}
	case !l.rcbsKnown:
		o.Model = resolveUnknown
	default:
		o.Model = model.ResolveMissing
	}
}

func (l *traceLink) resolveGoCB(o *model.TraceObject) {
	parts := strings.SplitN(o.MMS.ItemID, "$", 4)
	if len(parts) < 3 {
		o.Model = model.ResolveNotIEC61850
		return
	}
	g, ok := l.gocbFor(o.MMS.Domain, strings.Join(parts[:3], "$"))
	switch {
	case ok:
		o.Model = model.ResolveFound
		c := "GOOSE control block " + dataSetRef(g.domain, g.item)
		if g.datSet != "" {
			c += " (data set " + refOfDatSet(g.datSet) + ")"
		}
		o.Concerns = append(o.Concerns, c)
	case !l.gocbsKnown:
		o.Model = resolveUnknown
	default:
		o.Model = model.ResolveMissing
	}
}

func (l *traceLink) gocbFor(domain, item string) (gocbDef, bool) {
	for _, g := range l.gocbs {
		if g.domain == domain && g.item == item {
			return g, true
		}
	}
	return gocbDef{}, false
}

// refOfDatSet turns a data set in MMS form ("LD/LLN0$Meas") into its
// reference ("LD/LLN0.Meas").
func refOfDatSet(s string) string {
	if ld, item, ok := strings.Cut(s, "/"); ok {
		return dataSetRef(ld, item)
	}
	return s
}

// gooseModel resolves a GOOSE stream's control block against the link.
func (l *traceLink) gooseModel(g *trace.GooseStream) (string, string) {
	if l == nil {
		return "", ""
	}
	ld, item, ok := strings.Cut(g.GocbRef, "/")
	if !ok {
		return model.ResolveNotIEC61850, "the gocbRef does not name a logical device"
	}
	if !l.gocbsKnown {
		return resolveUnknown, "the linked model defines no GOOSE control blocks"
	}
	def, ok := l.gocbFor(ld, item)
	if !ok {
		return model.ResolveMissing, "the linked model has no GOOSE control block " + g.GocbRef
	}
	if def.datSet != "" && g.DatSet != "" && def.datSet != g.DatSet {
		return model.ResolveFound, fmt.Sprintf("the model's control block sends data set %s, the capture %s", def.datSet, g.DatSet)
	}
	return model.ResolveFound, ""
}

// linkFindings lists what the capture names that the linked model does
// not have: one finding per object, with where it first appears.
func linkFindings(res *trace.Result, l *traceLink) []model.TraceFinding {
	var out []model.TraceFinding
	seen := map[string]int{}
	note := func(o model.TraceObject, f model.TraceFinding) {
		l.resolve(&o)
		if o.Model != model.ResolveMissing {
			return
		}
		key := o.Kind + "|" + o.Ref
		if i, ok := seen[key]; ok {
			out[i].Count++
			return
		}
		seen[key] = len(out)
		what := o.Kind
		if what == "" {
			what = "object"
		}
		f.Severity, f.Kind, f.Count = model.SeverityWarning, "not-in-model", 1
		f.Message = fmt.Sprintf("%s %s (MMS %s/%s) is not in the linked model", kindName(what), o.Ref, o.MMS.Domain, o.MMS.ItemID)
		out = append(out, f)
	}
	for i, t := range res.Transactions {
		src := t.Request
		frame, ns := t.RequestFrame, t.RequestNS
		if src == nil {
			src, frame, ns = t.Response, t.ResponseFrame, t.ResponseNS
		}
		objs, _ := pduObjects(src)
		for _, o := range objs {
			note(o, model.TraceFinding{Association: t.Association, Transaction: i + 1, Frame: frame, At: nsTime(ns),
				Source: "Transaction object (" + serviceOf(t) + ")"})
		}
	}
	for _, r := range res.Reports {
		objs, _ := pduObjects(r.PDU)
		for _, o := range objs {
			note(o, model.TraceFinding{Association: r.Association, Frame: r.Frame, At: nsTime(r.TimeNS), Source: "Report variable"})
		}
	}
	for _, g := range res.GOOSE {
		if st, _ := l.gooseModel(g); st == model.ResolveMissing {
			out = append(out, model.TraceFinding{Severity: model.SeverityWarning, Kind: "not-in-model", Source: "GooseStream.GocbRef",
				Stream: g.GocbRef, At: nsTime(g.FirstNS), Count: uint64(g.Messages),
				Message: fmt.Sprintf("GOOSE control block %s is not in the linked model", g.GocbRef)})
		}
	}
	return out
}

func kindName(k string) string {
	switch k {
	case objLD:
		return "logical device"
	case objLN:
		return "logical node"
	case objDO:
		return "data object"
	case objDA:
		return "data attribute"
	case objDataSet:
		return "data set"
	case objRCB:
		return "report control block"
	case objGoCB:
		return "GOOSE control block"
	case objSGCB:
		return "setting group control block"
	case objLog:
		return "log control block"
	}
	return k
}

// TraceReports implements service.Service.
func (e *Engine) TraceReports(_ context.Context, id string, q model.TraceReportsQuery) (model.TraceReports, error) {
	_, ts, err := e.traceOf(id)
	if err != nil {
		return model.TraceReports{}, err
	}
	offset, limit, err := page(q.Offset, q.Limit)
	if err != nil {
		return model.TraceReports{}, err
	}
	if q.Association < 0 {
		return model.TraceReports{}, service.Errorf(service.CodeInvalidArgument, "association must not be negative")
	}
	link := ts.currentLink()
	out := model.TraceReports{Offset: offset, Reports: []model.TraceReport{}}
	for i, r := range ts.res.Reports {
		if q.Association != 0 && r.Association != q.Association {
			continue
		}
		if out.Total >= offset && len(out.Reports) < limit {
			out.Reports = append(out.Reports, reportOf(i+1, r, link))
		}
		out.Total++
	}
	return out, nil
}

// reportOf converts a captured InformationReport. An IEC 61850 report
// (variable list RPT) is decoded against the data set of its report
// control block when the link knows both; otherwise its values are shown
// as captured. Entries are never named by anything but the data set.
func reportOf(index int, r *trace.Report, l *traceLink) model.TraceReport {
	out := model.TraceReport{Index: index, Association: r.Association, At: nsTime(r.TimeNS), Frame: r.Frame}
	p := r.PDU
	if p == nil {
		out.Note = "the PDU was not captured"
		return out
	}
	out.DecodeError = p.DecodeError
	if p.ListName != nil {
		out.ListName = string(p.ListName.ItemID)
	}
	out.Objects, _ = pduObjects(p)
	for _, o := range out.Objects {
		if o.List && o.Scope == "vmd" {
			out.Objects = nil // RPT is not an object of the model
			break
		}
	}
	l.resolveAll(out.Objects)
	raw := func(note string) model.TraceReport {
		out.Note = note
		for _, v := range p.Values {
			if len(out.Values) == model.MaxTraceValues {
				break
			}
			out.Values = append(out.Values, convertValue(v, nil, ""))
		}
		return out
	}
	if p.ListName == nil || p.ListName.Scope != mms.ObjectScopeVMD || p.ListName.ItemID != "RPT" || len(p.Values) == 0 {
		return raw("not an IEC 61850 report (variable list RPT): shown as captured")
	}
	rpt, err := iec61850.DecodeReport(p.Values)
	if err != nil {
		return raw("the report's fields do not follow IEC 61850-8-1: " + err.Error())
	}
	out.RptID = rpt.RptID
	if rpt.OptFlds.Has(iec61850.OptFldSeqNum) {
		n := rpt.SeqNum
		out.SeqNum = &n
	}
	if rpt.OptFlds.Has(iec61850.OptFldConfRev) {
		n := rpt.ConfRev
		out.ConfRev = &n
	}
	if l == nil {
		return raw("link a model session to name the entries by their data set")
	}
	datSet := rpt.DatSet
	if rcb, ok := l.rcbForReport(rpt.RptID, datSet); ok {
		out.RCB = rcb.Ref
		if datSet == "" {
			datSet = rcb.DatSet
		}
	}
	out.DatSet = refOfDatSet(datSet)
	switch {
	case !l.dataSetsKnown:
		return raw("the linked session holds no data set definitions")
	case datSet == "":
		return raw(fmt.Sprintf("no report control block of the linked model has RptID %q, and the report names no data set", rpt.RptID))
	}
	ds, ok := l.dataSets[datSet]
	if !ok {
		return raw("data set " + out.DatSet + " is not in the linked model")
	}
	if len(rpt.Inclusion) != len(ds.Members) {
		return raw(fmt.Sprintf("the inclusion bit string has %d bits, data set %s has %d members", len(rpt.Inclusion), ds.Ref, len(ds.Members)))
	}
	out.Entries = []model.ReportEntry{}
	k := 0
	for i, included := range rpt.Inclusion {
		if !included {
			continue
		}
		m := ds.Members[i]
		entry := model.ReportEntry{Ref: m.Ref}
		if k < len(rpt.DataReferences) && rpt.DataReferences[k] != "" {
			entry.Ref = refOfMMS(rpt.DataReferences[k])
		}
		if k < len(rpt.ReasonCodes) {
			entry.Reasons = flagList(rpt.ReasonCodes[k].String())
		}
		name := ""
		if mr, err := iec61850.ParseRef(m.Ref); err == nil && len(mr.Path) > 0 {
			name = mr.Path[len(mr.Path)-1]
		}
		entry.Value = convertValue(rpt.Values[k].MMS(), nil, name)
		out.Entries = append(out.Entries, entry)
		k++
	}
	out.Decoded = true
	return out
}

// rcbForReport finds the report control block of a report: by its data
// set when the report names one and a single RCB sends it with this RptID,
// else by RptID.
func (l *traceLink) rcbForReport(rptID, datSet string) (model.RCB, bool) {
	var byID []model.RCB
	for _, r := range l.rcbs {
		// An RptID left empty is assigned by the IED: the RCB's reference.
		if r.RptID == rptID || (r.RptID == "" && (r.Ref == rptID || r.MMS.Domain+"/"+r.MMS.ItemID == rptID)) {
			byID = append(byID, r)
		}
	}
	for _, r := range byID {
		if datSet == "" || r.DatSet == datSet {
			return r, true
		}
	}
	return model.RCB{}, false
}
