// SPDX-License-Identifier: MIT

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func newCommissionEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	e := New(Options{DataDir: dir, Commission: true})
	t.Cleanup(e.Close)
	return e, dir
}

var writeGrant = model.GrantRequest{Reason: "WO-4711 commissioning", Minutes: 5, Capabilities: []string{model.CapWrite}}

// readAuditFiles returns every record in the audit files of dir.
func readAuditFiles(t *testing.T, dir string) []model.AuditRecord {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "audit", "audit-*.jsonl"))
	var out []model.AuditRecord
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %v", f, info.Mode().Perm())
		}
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			var r model.AuditRecord
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			out = append(out, r)
		}
		_ = fh.Close()
	}
	return out
}

func actions(recs []model.AuditRecord) string {
	a := make([]string, len(recs))
	for i, r := range recs {
		a[i] = r.Action + ":" + r.Outcome
	}
	return strings.Join(a, " ")
}

// A grant opens a second association, a write is verified by readback and
// recorded, and revoking ends the grant; the Observe association never
// writes.
func TestCommissionWriteVerified(t *testing.T) {
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	vendor := simied.Domain + "/LLN0.NamPlt.vendor[DC]"
	if _, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: vendor, Value: "x"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Fatalf("write without grant = %v", err)
	}
	if n := ied.Services()[mms.ServiceWrite]; n != 0 {
		t.Fatalf("%d writes reached the IED without a grant", n)
	}

	conns := ied.Connections()
	granted, err := e.GrantCommission(ctx, info.ID, writeGrant)
	if err != nil {
		t.Fatal(err)
	}
	if granted.Mode != model.ModeCommission || granted.Grant == nil || granted.Grant.Reason != writeGrant.Reason ||
		granted.Grant.ExpiresAt.Sub(granted.Grant.GrantedAt) != 5*time.Minute {
		t.Fatalf("granted = %+v", granted)
	}
	if ied.Connections() != conns+1 {
		t.Errorf("connections %d -> %d: no second association", conns, ied.Connections())
	}
	if _, err := e.GrantCommission(ctx, info.ID, writeGrant); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("second grant = %v", err)
	}
	waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventAudit && ev.Audit.Action == model.AuditGrant })

	res, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: vendor, Value: "Explorer test"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Error != nil || res.Before == nil || res.Before.Text != "OT Fabric" || res.After == nil || res.After.Text != "Explorer test" ||
		res.MMS != (model.MMSName{Domain: simied.Domain, ItemID: "LLN0$DC$NamPlt$vendor"}) {
		t.Fatalf("write = %+v", res)
	}
	read, _ := e.Read(ctx, info.ID, model.ReadRequest{Refs: []string{vendor}})
	if read.Values[0].Value.Text != "Explorer test" {
		t.Errorf("read after write = %+v", read.Values[0])
	}

	// The Observe association still refuses writes itself.
	s, _ := e.session(info.ID)
	_, err = s.client.MMS().WriteObject(ctx, mms.ObjectName{Scope: mms.ObjectScopeDomain, Domain: simied.Domain, ItemID: "LLN0$DC$NamPlt$vendor"}, mms.NewVisibleString("y"))
	if !errors.Is(err, mms.ErrServiceNotAllowed) {
		t.Errorf("Observe association write = %v", err)
	}

	// Values the IED refuses, and values that cannot be written.
	bad := map[string]string{
		simied.Domain + "/MMXU1.Hz.mag.f[MX]":           service.CodeInvalidArgument, // not a writable FC
		simied.Domain + "/LLN0.NamPlt[DC]":              service.CodeInvalidArgument, // a data object, not an attribute
		simied.Domain + "/LLN0.Mod.ctlModel[CF]":        service.CodeInvalidArgument, // "x" is not an integer
		"not a ref":                                     service.CodeInvalidArgument,
		simied.Domain + "/XCBR1.Pos.Oper.ctlVal[CO]":    service.CodeInvalidArgument,
		simied.Domain + "/LLN0.SGCB.ActSG[SP]":          service.CodeInvalidArgument, // setting groups have their own capability
		simied.Domain + "/LLN0.NamPlt.vendor.extra[DC]": "",
	}
	for ref, code := range bad {
		r, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: ref, Value: "x"})
		switch {
		case code != "" && service.CodeOf(err) != code:
			t.Errorf("write %s = %+v, %v; want %s", ref, r, err, code)
		case code == "" && (err != nil || r.Error == nil):
			// An attribute the IED does not have: its type is unknown.
			if service.CodeOf(err) != service.CodeInvalidArgument {
				t.Errorf("write %s = %+v, %v", ref, r, err)
			}
		}
	}

	if _, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: simied.Domain + "/LLN0.SGCB.ActSG[SP]", Value: "2"}); err == nil || !strings.Contains(err.Error(), "control block") {
		t.Errorf("write SGCB = %v", err)
	}

	revoked, err := e.RevokeCommission(ctx, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Mode != model.ModeObserve || revoked.Grant != nil {
		t.Errorf("revoked = %+v", revoked)
	}
	if _, err := e.RevokeCommission(ctx, info.ID); err != nil {
		t.Errorf("second revoke = %v", err)
	}
	if _, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: vendor, Value: "z"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("write after revoke = %v", err)
	}

	recs := readAuditFiles(t, dir)
	if got := actions(recs); got != "grant:ok write:ok revoke:ok" {
		t.Errorf("audit trail = %s", got)
	}
	w := recs[1]
	if w.Ref != simied.Domain+"/LLN0.NamPlt.vendor[DC]" || w.Before != "OT Fabric" || w.Written != "Explorer test" || w.After != "Explorer test" ||
		w.SessionID != info.ID || w.Target != ied.Addr || !strings.Contains(w.IED, "Explorer demo IED") || w.Seq != 2 {
		t.Errorf("write record = %+v", w)
	}
	recent, _ := e.Audit(ctx, model.AuditQuery{SessionID: info.ID, Limit: 2})
	if len(recent) != 2 || recent[0].Action != model.AuditRevoke {
		t.Errorf("Audit = %+v", recent)
	}
	if other, _ := e.Audit(ctx, model.AuditQuery{SessionID: "nope"}); len(other) != 0 {
		t.Errorf("Audit other session = %+v", other)
	}
}

// A grant is refused when the engine does not offer Commission mode, for an
// offline session, and for invalid requests.
func TestCommissionRefused(t *testing.T) {
	ied := startIED(t)
	ctx := t.Context()

	plain := newEngine(t)
	info, err := plain.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.GrantCommission(ctx, info.ID, writeGrant); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("grant on an engine without Commission = %v", err)
	}
	if _, err := plain.Audit(ctx, model.AuditQuery{}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("Audit without Commission = %v", err)
	}
	if h, _ := plain.Health(ctx); strings.Contains(strings.Join(h.Features, ","), model.FeatureCommission) {
		t.Errorf("features = %v", h.Features)
	}
	noDir := New(Options{Commission: true})
	defer noDir.Close()
	if _, err := noDir.GrantCommission(ctx, "x", writeGrant); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("grant without a data directory = %v", err)
	}

	e, dir := newCommissionEngine(t)
	if h, _ := e.Health(ctx); !strings.Contains(strings.Join(h.Features, ","), model.FeatureCommission) {
		t.Errorf("features = %v", h.Features)
	}
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, offline.ID, writeGrant); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("grant on SCL session = %v", err)
	}
	if _, err := e.GrantCommission(ctx, "nope", writeGrant); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("grant on unknown session = %v", err)
	}
	if _, err := e.Write(ctx, offline.ID, model.WriteRequest{Ref: "x", Value: "1"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("write on SCL session = %v", err)
	}
	if _, err := e.Write(ctx, "nope", model.WriteRequest{}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("write on unknown session = %v", err)
	}
	if _, err := e.RevokeCommission(ctx, "nope"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("revoke on unknown session = %v", err)
	}
	invalid := []model.GrantRequest{
		{Reason: "", Capabilities: []string{model.CapWrite}},
		{Reason: "  ", Capabilities: []string{model.CapWrite}},
		{Reason: strings.Repeat("r", model.MaxGrantReasonLen+1), Capabilities: []string{model.CapWrite}},
		{Reason: "a\x07b", Capabilities: []string{model.CapWrite}},
		{Reason: "r", Minutes: -1, Capabilities: []string{model.CapWrite}},
		{Reason: "r", Minutes: model.MaxGrantMinutes + 1, Capabilities: []string{model.CapWrite}},
		{Reason: "r"},
		{Reason: "r", Capabilities: []string{"laboratory"}},
	}
	for _, req := range invalid {
		if _, err := e.GrantCommission(ctx, live.ID, req); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("grant %+v = %v", req, err)
		}
	}
	// The default duration, and duplicate capabilities.
	g, err := e.GrantCommission(ctx, live.ID, model.GrantRequest{Reason: "r", Capabilities: []string{model.CapWrite, model.CapDataSets, model.CapWrite}})
	if err != nil {
		t.Fatal(err)
	}
	if d := g.Grant.ExpiresAt.Sub(g.Grant.GrantedAt); d != model.DefaultGrantMinutes*time.Minute ||
		strings.Join(g.Grant.Capabilities, ",") != "datasets,write" {
		t.Errorf("grant = %+v", g.Grant)
	}

	// An audit trail that cannot be written refuses the grant.
	if _, err := e.RevokeCommission(ctx, live.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "audit")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audit"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, live.ID, writeGrant); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("grant without an audit trail = %v", err)
	}
	if info, _ := e.session(live.ID); info.currentGrant() != nil {
		t.Error("a grant without an audit trail")
	}
}

// A grant without the write capability cannot write: the engine refuses,
// and the Commission association's own allow-list refuses too.
func TestCommissionCapabilities(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.GrantCommission(ctx, info.ID, model.GrantRequest{Reason: "data sets only", Capabilities: []string{model.CapDataSets}}); err != nil {
		t.Fatal(err)
	}
	vendor := simied.Domain + "/LLN0.NamPlt.vendor[DC]"
	if _, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: vendor, Value: "x"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("write with the datasets capability = %v", err)
	}
	s, _ := e.session(info.ID)
	ref, _ := iec61850.ParseRef(vendor)
	if err := s.currentGrant().client.Write(ctx, ref, mms.NewVisibleString("x")); !errors.Is(err, mms.ErrServiceNotAllowed) {
		t.Errorf("Commission association write without the capability = %v", err)
	}
	if n := ied.Services()[mms.ServiceWrite]; n != 0 {
		t.Errorf("the IED received %d writes", n)
	}
}

// A grant ends when it expires, when the session closes, when the session is
// lost and when the IED refuses a write; each end is in the audit trail.
func TestCommissionEnds(t *testing.T) {
	ctx := t.Context()

	t.Run("expiry", func(t *testing.T) {
		ied := startIED(t)
		e, dir := newCommissionEngine(t)
		events, _ := e.Subscribe(ctx, 0)
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		s, _ := e.session(info.ID)
		s.currentGrant().timer.Reset(time.Millisecond)
		waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventAudit && ev.Audit.Action == model.AuditExpire })
		if got := s.info(); got.Mode != model.ModeObserve {
			t.Errorf("after expiry = %+v", got)
		}
		if got := actions(readAuditFiles(t, dir)); got != "grant:ok expire:ok" {
			t.Errorf("audit = %s", got)
		}
	})

	t.Run("close", func(t *testing.T) {
		ied := startIED(t)
		e, dir := newCommissionEngine(t)
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		if err := e.CloseSession(ctx, info.ID); err != nil {
			t.Fatal(err)
		}
		recs := readAuditFiles(t, dir)
		if got := actions(recs); got != "grant:ok end:ok" || recs[1].Reason != "the session was closed" {
			t.Errorf("audit = %s %+v", got, recs)
		}
	})

	t.Run("lost", func(t *testing.T) {
		ied := startIED(t)
		e, dir := newCommissionEngine(t)
		events, _ := e.Subscribe(ctx, 0)
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		ied.Close()
		waitEvent(t, events, func(ev model.Event) bool { return ev.Kind == model.EventAudit && ev.Audit.Action == model.AuditEnd })
		if got := actions(readAuditFiles(t, dir)); got != "grant:ok end:ok" {
			t.Errorf("audit = %s", got)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		ied := startIED(t)
		e, dir := newCommissionEngine(t)
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		s, _ := e.session(info.ID)
		g := s.currentGrant()
		undone := 0
		g.cleanup = append(g.cleanup,
			cleanupStep{what: "first", undo: func(context.Context, *iec61850.Client) error { undone++; return nil }},
			cleanupStep{what: "second", undo: func(context.Context, *iec61850.Client) error { return errors.New("refused") }},
		)
		if _, err := e.RevokeCommission(ctx, info.ID); err != nil {
			t.Fatal(err)
		}
		recs := readAuditFiles(t, dir)
		if got := actions(recs); undone != 1 || got != "grant:ok cleanup:failed cleanup:ok revoke:failed" ||
			recs[1].Ref != "second" || recs[3].Error != "not undone: second" {
			t.Errorf("audit = %s %+v", got, recs)
		}
	})

	t.Run("refused write", func(t *testing.T) {
		ied := startIED(t)
		e, dir := newCommissionEngine(t)
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		// VisString255: 256 characters are refused by the engine; the IED
		// refuses a write to an attribute it does not have.
		res, err := e.Write(ctx, info.ID, model.WriteRequest{Ref: simied.Domain + "/LLN0.NamPlt.vendor[DC]", Value: strings.Repeat("v", 256)})
		if service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("long string = %+v, %v", res, err)
		}
		recs := readAuditFiles(t, dir)
		if got := actions(recs); got != "grant:ok" {
			t.Errorf("audit = %s", got)
		}
	})
}

// The audit sequence continues across engine runs and the recent records
// are read back; a damaged file is reported, not fatal.
func TestAuditAcrossRuns(t *testing.T) {
	ied := startIED(t)
	ctx := t.Context()
	dir := t.TempDir()
	run := func() (*Engine, string) {
		e := New(Options{DataDir: dir, Commission: true})
		info, err := e.OpenSession(ctx, target(t, ied.Addr))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.GrantCommission(ctx, info.ID, writeGrant); err != nil {
			t.Fatal(err)
		}
		return e, info.ID
	}
	e1, _ := run()
	e1.Close() // grant, end
	e2, id := run()
	defer e2.Close()
	recs, err := e2.Audit(ctx, model.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].Seq != 3 || recs[0].SessionID != id || recs[2].Seq != 1 {
		t.Fatalf("records = %+v", recs)
	}

	// A file the engine cannot parse: the records before the damage count.
	e3dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(e3dir, "audit"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e3dir, "audit", "audit-2026-01-01.jsonl"), []byte(`{"seq":7,"action":"grant"}`+"\n{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e3 := New(Options{DataDir: e3dir, Commission: true})
	defer e3.Close()
	recs, _ = e3.Audit(ctx, model.AuditQuery{})
	if len(recs) != 1 || recs[0].Seq != 7 {
		t.Errorf("damaged file: %+v", recs)
	}
	if _, err := readAuditFile(filepath.Join(e3dir, "nope.jsonl")); err == nil {
		t.Error("missing file read")
	}
}
