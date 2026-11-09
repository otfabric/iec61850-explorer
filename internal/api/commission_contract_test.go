// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// TestContractCommission: Commission mode behaves the same through the
// engine and through the HTTP API, and is refused where it is not offered.
func TestContractCommission(t *testing.T) {
	opts := func(t *testing.T, on bool) engine.Options {
		return engine.Options{DataDir: t.TempDir(), Commission: on}
	}
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(opts(t, true))
		t.Cleanup(eng.Close)
		commissionContract(t, eng)
		off := engine.New(opts(t, false))
		t.Cleanup(off.Close)
		commissionRefused(t, off)
	})
	t.Run("http", func(t *testing.T) {
		commissionContract(t, newDaemon(t, opts(t, true), api.Options{}).svc)
		commissionRefused(t, newDaemon(t, opts(t, false), api.Options{}).svc)
	})
}

func commissionContract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	h, _ := svc.Health(ctx)
	if !slices.Contains(h.Features, model.FeatureCommission) {
		t.Fatalf("features = %v", h.Features)
	}
	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := svc.Subscribe(sub, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseSession(ctx, info.ID) })

	ref := simied.Domain + "/LLN0.NamPlt.swRev[DC]"
	if _, err := svc.Write(ctx, info.ID, model.WriteRequest{Ref: ref, Value: "2.0"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("write without grant = %v", err)
	}
	if _, err := svc.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "r", Capabilities: []string{"laboratory"}}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("grant laboratory = %v", err)
	}
	g, err := svc.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "contract", Minutes: 2, Capabilities: []string{model.CapWrite}})
	if err != nil {
		t.Fatal(err)
	}
	if g.Mode != model.ModeCommission || g.Grant == nil || g.Grant.Reason != "contract" {
		t.Fatalf("grant = %+v", g)
	}
	next(t, events, "grant session event", func(ev model.Event) bool {
		return ev.Kind == model.EventSession && ev.Session.Mode == model.ModeCommission
	})

	res, err := svc.Write(ctx, info.ID, model.WriteRequest{Ref: ref, Value: "2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Before.Text != "1.0" || res.After.Text != "2.0" || res.Written.Text != "2.0" {
		t.Errorf("write = %+v", res)
	}
	if _, err := svc.Write(ctx, info.ID, model.WriteRequest{Ref: ref + "x", Value: "1"}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("write unknown attribute = %v", err)
	}
	ev := next(t, events, "write audit event", func(ev model.Event) bool {
		return ev.Kind == model.EventAudit && ev.Audit.Action == model.AuditWrite
	})
	if ev.Audit.Before != "1.0" || ev.Audit.After != "2.0" || ev.SessionID != info.ID {
		t.Errorf("audit event = %+v", ev.Audit)
	}

	r, err := svc.RevokeCommission(ctx, info.ID)
	if err != nil || r.Mode != model.ModeObserve || r.Grant != nil {
		t.Fatalf("revoke = %+v, %v", r, err)
	}
	recs, err := svc.Audit(ctx, model.AuditQuery{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].Action != model.AuditRevoke || recs[1].Action != model.AuditWrite || recs[2].Action != model.AuditGrant {
		t.Errorf("audit = %+v", recs)
	}
	if recs, _ := svc.Audit(ctx, model.AuditQuery{Limit: 1}); len(recs) != 1 {
		t.Errorf("audit limit 1 = %+v", recs)
	}
}

func commissionRefused(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	h, _ := svc.Health(ctx)
	if slices.Contains(h.Features, model.FeatureCommission) {
		t.Errorf("features = %v", h.Features)
	}
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseSession(ctx, info.ID) })
	if _, err := svc.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "r", Capabilities: []string{model.CapWrite}}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("grant = %v", err)
	}
	if _, err := svc.Audit(ctx, model.AuditQuery{}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("audit = %v", err)
	}
	if info, err := svc.RevokeCommission(ctx, info.ID); err != nil || info.Mode != model.ModeObserve {
		t.Errorf("revoke without grant = %+v, %v", info, err)
	}
}

// TestContractM2b2: report subscription, data sets and setting groups
// behave the same through the engine and the HTTP API; reports arrive as
// events either way.
func TestContractM2b2(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{DataDir: t.TempDir(), Commission: true})
		t.Cleanup(eng.Close)
		m2b2Contract(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		m2b2Contract(t, newDaemon(t, engine.Options{DataDir: t.TempDir(), Commission: true}, api.Options{}).svc)
	})
}

func m2b2Contract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := svc.Subscribe(sub, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseSession(ctx, info.ID) })
	rcb := simied.Domain + "/LLN0.RP.urcbEvents"
	if _, err := svc.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: rcb}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("subscribe without grant = %v", err)
	}
	if _, err := svc.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "contract", Capabilities: []string{model.CapReports, model.CapDataSets, model.CapSettingGroups}}); err != nil {
		t.Fatal(err)
	}

	s, err := svc.SubscribeReport(ctx, info.ID, model.SubscribeRequest{RCB: rcb, GI: true})
	if err != nil {
		t.Fatal(err)
	}
	ev := next(t, events, "GI report", func(ev model.Event) bool { return ev.Kind == model.EventReport && ev.Report.SubscriptionID == s.ID })
	if len(ev.Report.Entries) != 3 || ev.Report.Entries[0].Ref != simied.Domain+"/GGIO1.Ind1.stVal[ST]" || ev.SessionID != info.ID {
		t.Errorf("report = %+v", ev.Report)
	}
	if err := svc.TriggerGI(ctx, info.ID, s.ID); err != nil {
		t.Errorf("GI = %v", err)
	}
	if err := svc.Unsubscribe(ctx, info.ID, s.ID); err != nil {
		t.Errorf("unsubscribe = %v", err)
	}
	if err := svc.Unsubscribe(ctx, info.ID, s.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unsubscribe twice = %v", err)
	}

	ds, err := svc.CreateDataSet(ctx, info.ID, model.CreateDataSetRequest{Ref: simied.Domain + "/LLN0.Contract", Members: []string{simied.Domain + "/MMXU1.Hz[MX]"}})
	if err != nil || !ds.Deletable || len(ds.Members) != 1 {
		t.Fatalf("create = %+v, %v", ds, err)
	}
	if err := svc.DeleteDataSet(ctx, info.ID, ds.Ref); err != nil {
		t.Errorf("delete = %v", err)
	}
	if err := svc.DeleteDataSet(ctx, info.ID, simied.Domain+"/LLN0.Meas"); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("delete configured = %v", err)
	}
	sg, err := svc.SelectActiveSG(ctx, info.ID, model.SelectSGRequest{LD: simied.Domain, SG: 2})
	if err != nil || sg.ActSG != 2 {
		t.Errorf("select SG = %+v, %v", sg, err)
	}
	recs, _ := svc.Audit(ctx, model.AuditQuery{SessionID: info.ID})
	var acts []string
	for i := len(recs) - 1; i >= 0; i-- {
		acts = append(acts, recs[i].Action)
	}
	if got := strings.Join(acts, " "); got != "grant subscribe gi unsubscribe create-dataset delete-dataset select-active-sg" {
		t.Errorf("audit = %s", got)
	}
}

func TestContractM2b3(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{DataDir: t.TempDir(), Commission: true})
		t.Cleanup(eng.Close)
		m2b3Contract(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		m2b3Contract(t, newDaemon(t, engine.Options{DataDir: t.TempDir(), Commission: true}, api.Options{}).svc)
	})
}

func m2b3Contract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)
	info, err := svc.OpenSession(ctx, targetOf(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseSession(ctx, info.ID) })
	pos := simied.Domain + "/XCBR1.Pos"
	if _, err := svc.Control(ctx, info.ID, model.ControlRequest{Ref: pos, Value: "false"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("control without grant = %v", err)
	}
	if _, err := svc.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "contract", Minutes: 2, Capabilities: []string{model.CapControls}}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Control(ctx, info.ID, model.ControlRequest{Ref: pos, Value: "false", TimeoutMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != model.ControlOK || len(res.Steps) != 3 || res.CtlModel != "sbo-with-enhanced-security" || res.After == nil {
		t.Errorf("open = %+v", res)
	}
	ied.Set("GGIO1$ST$Ind1$stVal", mms.NewBoolean(false))
	res, err = svc.Control(ctx, info.ID, model.ControlRequest{Ref: pos, Value: "true", InterlockCheck: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != model.ControlNegative || res.AddCause != "blocked-by-interlocking" || res.Steps[2].OK {
		t.Errorf("close while interlocked = %+v", res)
	}
	if _, err := svc.Control(ctx, info.ID, model.ControlRequest{Ref: pos + "[CO]", Value: "true"}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("control with an FC = %v", err)
	}
	audit, err := svc.Audit(ctx, model.AuditQuery{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) == 0 || audit[0].Action != model.AuditControl || audit[0].Outcome != model.OutcomeFailed {
		t.Errorf("audit = %+v", audit)
	}
}
