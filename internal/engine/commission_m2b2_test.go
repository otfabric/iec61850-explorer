// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

var (
	urcb = simied.Domain + "/LLN0.RP.urcbEvents01"
	brcb = simied.Domain + "/LLN0.BR.brcbEvents01"
)

func grantOn(t *testing.T, e *Engine, id string, caps ...string) {
	t.Helper()
	if _, err := e.GrantCommission(t.Context(), id, model.GrantRequest{Reason: "M2b2 test", Minutes: 5, Capabilities: caps}); err != nil {
		t.Fatal(err)
	}
}

func rcbState(t *testing.T, e *Engine, id, ref string) model.RCB {
	t.Helper()
	list, err := e.Reports(t.Context(), id, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(list, func(r model.RCB) bool { return r.Ref == ref })
	if i < 0 {
		t.Fatalf("no RCB %s in %+v", ref, list)
	}
	return list[i]
}

func nextReport(t *testing.T, events <-chan model.Event, match func(*model.Report) bool) *model.Report {
	t.Helper()
	return waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventReport && ev.Report != nil && match(ev.Report)
	}).Report
}

func entryRefs(r *model.Report) []string {
	out := make([]string, len(r.Entries))
	for i, en := range r.Entries {
		out[i] = en.Ref
	}
	return out
}

// Subscribing reserves, enables and interrogates an RCB; reports arrive as
// events named by the data set's members; unsubscribing and revoking
// disable and release it again, and every step is audited.
func TestReportSubscription(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("subscribe without grant = %v", err)
	}
	grantOn(t, e, info.ID, model.CapReports)

	sub, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb, GI: true})
	if err != nil {
		t.Fatal(err)
	}
	if sub.RptID != "Events" || sub.Buffered || sub.DatSet != simied.Domain+"/LLN0$Events" || sub.ID == "" {
		t.Fatalf("subscription = %+v", sub)
	}
	gi := nextReport(t, events, func(r *model.Report) bool { return slices.Contains(r.Entries[0].Reasons, "GI") })
	wantRefs := []string{simied.Domain + "/GGIO1.Ind1.stVal[ST]", simied.Domain + "/XCBR1.OpCnt.stVal[ST]", simied.Domain + "/MMXU1.Hz[MX]"}
	if gi.SubscriptionID != sub.ID || gi.RptID != "Events" || !slices.Equal(entryRefs(gi), wantRefs) || gi.SeqNum == nil || gi.Time == nil {
		t.Fatalf("GI report = %+v", gi)
	}
	if v := gi.Entries; v[0].Value.Text != "true" || v[1].Value.Text != "17" || v[2].Value.Name != "Hz" || hzMag(v[2].Value) == nil || hzMag(v[2].Value).Type != model.TypeFloat {
		t.Errorf("GI values = %+v", v)
	}

	ied.Set("MMXU1$MX$Hz$mag$f", mms.NewFloat(49.5))
	dchg := nextReport(t, events, func(r *model.Report) bool {
		return len(r.Entries) == 1 && slices.Contains(r.Entries[0].Reasons, "data-change")
	})
	if dchg.Entries[0].Ref != wantRefs[2] || hzMag(dchg.Entries[0].Value) == nil || hzMag(dchg.Entries[0].Value).Text != "49.5" {
		t.Errorf("data change report = %+v", dchg)
	}
	if err := e.TriggerGI(ctx, info.ID, sub.ID); err != nil {
		t.Fatal(err)
	}
	nextReport(t, events, func(r *model.Report) bool { return len(r.Entries) == 3 && slices.Contains(r.Entries[1].Reasons, "GI") })

	// The RCB is enabled and reserved; the session shows the subscription.
	state := rcbState(t, e, info.ID, urcb)
	if !*state.RptEna || !*state.Resv {
		t.Errorf("RCB while subscribed = %+v", state)
	}
	if s, _ := e.session(info.ID); len(s.info().Grant.Subscriptions) != 1 || s.info().Grant.Subscriptions[0].Reports < 3 {
		t.Errorf("grant = %+v", s.info().Grant)
	}
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("second subscription to the same RCB = %v", err)
	}

	// A buffered RCB: entry IDs and the configuration revision.
	bsub, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: brcb, GI: true})
	if err != nil {
		t.Fatal(err)
	}
	br := nextReport(t, events, func(r *model.Report) bool { return r.SubscriptionID == bsub.ID })
	if !bsub.Buffered || br.RptID != "EventsBuffered" || br.ConfRev == nil || *br.ConfRev != 1 || br.EntryID == "" {
		t.Errorf("buffered report = %+v", br)
	}

	if err := e.Unsubscribe(ctx, info.ID, sub.ID); err != nil {
		t.Fatal(err)
	}
	if state := rcbState(t, e, info.ID, urcb); *state.RptEna || *state.Resv {
		t.Errorf("RCB after unsubscribe = %+v", state)
	}
	if err := e.Unsubscribe(ctx, info.ID, sub.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("second unsubscribe = %v", err)
	}
	if err := e.TriggerGI(ctx, info.ID, "nope"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("GI on unknown subscription = %v", err)
	}

	// Revoking cleans up the buffered subscription.
	if _, err := e.RevokeCommission(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if state := rcbState(t, e, info.ID, brcb); *state.RptEna {
		t.Errorf("BRCB after revoke = %+v", state)
	}
	got := actions(readAuditFiles(t, dir))
	want := "grant:ok subscribe:ok gi:ok subscribe:ok unsubscribe:ok cleanup:ok revoke:ok"
	if got != want {
		t.Errorf("audit = %s\nwant    %s", got, want)
	}
}

// An RCB another client enabled or reserved is not taken over; invalid
// references and the limit are refused.
func TestReportSubscriptionRefused(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, info.ID, model.CapReports)

	other, err := iec61850.Dial(ctx, ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close(context.Background()) }()
	if err := other.ReserveURCB(ctx, simied.Domain, "LLN0$RP$urcbEvents01"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb}); service.CodeOf(err) != service.CodeNotAllowed || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("reserved URCB = %v", err)
	}
	if err := other.SetReportControlBlock(ctx, simied.Domain, "LLN0$BR$brcbEvents01", iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: brcb}); service.CodeOf(err) != service.CodeNotAllowed || !strings.Contains(err.Error(), "enabled") {
		t.Errorf("enabled BRCB = %v", err)
	}
	for _, ref := range []string{"nope", simied.Domain + "/LLN0.Meas", simied.Domain + "/LLN0.XX.urcbMeas"} {
		if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: ref}); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("subscribe %q = %v", ref, err)
		}
	}
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: simied.Domain + "/LLN0.RP.nope"}); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("unknown RCB = %v", err)
	}
	if err := e.Unsubscribe(ctx, "nope", "x"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unsubscribe unknown session = %v", err)
	}
	s, _ := e.session(info.ID)
	g := s.currentGrant()
	g.mu.Lock()
	for i := range model.MaxSubscriptions {
		g.subs[string(rune('a'+i))] = &subscription{ld: "x", item: string(rune('a' + i))}
	}
	g.mu.Unlock()
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb}); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("beyond the limit = %v", err)
	}
	g.mu.Lock()
	clear(g.subs)
	g.mu.Unlock()
}

// The Commission association's loss ends the grant; the cleanup that can
// no longer reach the IED is audited as failed.
func TestReportSubscriptionLost(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, info.ID, model.CapReports)
	if _, err := e.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: urcb}); err != nil {
		t.Fatal(err)
	}
	ied.Close()
	waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventAudit && ev.Audit.Action == model.AuditEnd })
	recs := readAuditFiles(t, dir)
	if got := actions(recs); got != "grant:ok subscribe:ok cleanup:failed end:failed" {
		t.Errorf("audit = %s", got)
	}
}

func TestCreateDeleteDataSet(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	req := model.CreateDataSetRequest{
		Ref:     simied.Domain + "/LLN0.Explorer1",
		Members: []string{simied.Domain + "/MMXU1.Hz[MX]", simied.Domain + "/XCBR1.Pos.stVal[ST]"},
	}
	if _, err := e.CreateDataSet(ctx, info.ID, req); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("create without grant = %v", err)
	}
	grantOn(t, e, info.ID, model.CapDataSets)

	bad := []model.CreateDataSetRequest{
		{Ref: "nope", Members: req.Members},
		{Ref: simied.Domain + "/LLN0.1bad", Members: req.Members},
		{Ref: simied.Domain + "/LLN0." + strings.Repeat("a", model.MaxDataSetNameLen+1), Members: req.Members},
		{Ref: req.Ref},
		{Ref: req.Ref, Members: []string{simied.Domain + "/MMXU1.Hz"}},
		{Ref: req.Ref, Members: []string{simied.Domain + "/MMXU1[MX]"}},
		{Ref: req.Ref, Members: make([]string, model.MaxDataSetMembers+1)},
	}
	for _, r := range bad {
		if _, err := e.CreateDataSet(ctx, info.ID, r); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("create %+v = %v", r.Ref, err)
		}
	}
	if _, err := e.CreateDataSet(ctx, info.ID, model.CreateDataSetRequest{Ref: simied.Domain + "/NOLN1.X", Members: req.Members}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown LN = %v", err)
	}
	if _, err := e.CreateDataSet(ctx, info.ID, model.CreateDataSetRequest{Ref: "NoLD/LLN0.X", Members: req.Members}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown LD = %v", err)
	}

	ds, err := e.CreateDataSet(ctx, info.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Ref != req.Ref || !ds.Deletable || len(ds.Members) != 2 || ds.Members[1].Ref != req.Members[1] {
		t.Fatalf("created = %+v", ds)
	}
	list, _ := e.DataSets(ctx, info.ID, simied.Domain)
	if len(list) != 3 {
		t.Errorf("data sets = %+v", list)
	}
	if _, err := e.CreateDataSet(ctx, info.ID, req); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("create twice = %v", err)
	}
	if err := e.DeleteDataSet(ctx, info.ID, simied.Domain+"/LLN0.Meas"); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("delete a configured data set = %v", err)
	}
	if err := e.DeleteDataSet(ctx, info.ID, "nope"); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("delete bad ref = %v", err)
	}
	if err := e.DeleteDataSet(ctx, info.ID, req.Ref); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.DataSets(ctx, info.ID, simied.Domain); len(list) != 2 {
		t.Errorf("after delete = %+v", list)
	}
	if err := e.DeleteDataSet(ctx, info.ID, req.Ref); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("delete twice = %v", err)
	}
	recs := readAuditFiles(t, dir)
	if got := actions(recs); got != "grant:ok create-dataset:ok create-dataset:failed delete-dataset:ok" {
		t.Errorf("audit = %s", got)
	}
	if !strings.Contains(recs[1].Detail, "2 members") || recs[3].Before != "2 members" {
		t.Errorf("records = %+v", recs)
	}
}

// A data set an active subscription uses is not deleted.
func TestDeleteDataSetInUse(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, info.ID, model.CapDataSets, model.CapReports)
	s, _ := e.session(info.ID)
	g := s.currentGrant()
	g.mu.Lock()
	g.subs["x"] = &subscription{info: model.Subscription{RCB: "r", DatSet: simied.Domain + "/LLN0$Tmp"}}
	g.mu.Unlock()
	if _, err := e.CreateDataSet(ctx, info.ID, model.CreateDataSetRequest{Ref: simied.Domain + "/LLN0.Tmp", Members: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteDataSet(ctx, info.ID, simied.Domain+"/LLN0.Tmp"); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("delete a data set in use = %v", err)
	}
	g.mu.Lock()
	clear(g.subs)
	g.mu.Unlock()
}

func TestSelectActiveSG(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SelectActiveSG(ctx, info.ID, model.SelectSGRequest{LD: simied.Domain, SG: 2}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("without grant = %v", err)
	}
	grantOn(t, e, info.ID, model.CapSettingGroups)
	for _, sg := range []uint8{0, 3} {
		if _, err := e.SelectActiveSG(ctx, info.ID, model.SelectSGRequest{LD: simied.Domain, SG: sg}); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("group %d = %v", sg, err)
		}
	}
	if _, err := e.SelectActiveSG(ctx, info.ID, model.SelectSGRequest{LD: "NoLD", SG: 1}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown LD = %v", err)
	}
	if _, err := e.SelectActiveSG(ctx, "nope", model.SelectSGRequest{LD: simied.Domain, SG: 1}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}
	got, err := e.SelectActiveSG(ctx, info.ID, model.SelectSGRequest{LD: simied.Domain, SG: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActSG != 2 || got.NumOfSGs != 2 {
		t.Errorf("after select = %+v", got)
	}
	if read, _ := e.SettingGroups(ctx, info.ID, simied.Domain); read.ActSG != 2 {
		t.Errorf("read back = %+v", read)
	}
	recs := readAuditFiles(t, dir)
	if a := actions(recs); a != "grant:ok select-active-sg:ok" || recs[1].Before != "1" || recs[1].After != "2" {
		t.Errorf("audit = %s %+v", a, recs)
	}
}

func TestSplitRCBRef(t *testing.T) {
	ld, item, buffered, err := splitRCBRef("LD0/LLN0.BR.brcb01")
	if err != nil || ld != "LD0" || item != "LLN0$BR$brcb01" || !buffered {
		t.Errorf("= %s %s %v %v", ld, item, buffered, err)
	}
	if _, _, ok := splitDatSet("nodomain"); ok {
		t.Error("splitDatSet without domain")
	}
	for in, want := range map[string]string{
		"LD0/GGIO1$ST$Ind1$stVal": "LD0/GGIO1.Ind1.stVal[ST]",
		"LD0/MMXU1$MX$Hz":         "LD0/MMXU1.Hz[MX]",
		"no-domain":               "no-domain",
		"LD0/LLN0$Meas":           "LD0/LLN0$Meas",
		"LD0/LN$XYZ$a":            "LD0/LN$XYZ$a",
	} {
		if got := refOfMMS(in); got != want {
			t.Errorf("refOfMMS(%q) = %q, want %q", in, got, want)
		}
	}
	if clip("abcdef", 3) != "abc…" || clip("ab", 3) != "ab" {
		t.Error("clip")
	}
}

// hzMag returns mag.f of a reported MMXU1.Hz[MX] data object (a data
// set member naming a whole data object is reported as its structure).
func hzMag(v model.Value) *model.Value {
	for i := range v.Children {
		if v.Children[i].Name != "mag" {
			continue
		}
		for j := range v.Children[i].Children {
			if v.Children[i].Children[j].Name == "f" {
				return &v.Children[i].Children[j]
			}
		}
	}
	return nil
}
