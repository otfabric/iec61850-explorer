// SPDX-License-Identifier: MIT

package api_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/api"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

var ctx = context.Background()

// next returns the first event that matches, failing after a timeout.
func next(t testing.TB, ch <-chan model.Event, what string, match func(model.Event) bool) model.Event {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event stream closed while waiting for %s", what)
			}
			if match(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func targetOf(t testing.TB, addr string) model.TargetConfig {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return model.TargetConfig{Host: host, Port: p, TimeoutMS: 5000}
}

// TestContract: the same contract holds for the engine and for the engine
// behind the HTTP API (the M0 exit criterion: one service, identical
// behaviour in the desktop and the daemon form).
func TestContract(t *testing.T) {
	t.Run("engine", func(t *testing.T) {
		eng := engine.New(engine.Options{DataDir: t.TempDir()})
		t.Cleanup(eng.Close)
		contract(t, eng)
	})
	t.Run("http", func(t *testing.T) {
		contract(t, newDaemon(t, engine.Options{DataDir: t.TempDir()}, api.Options{}).svc)
	})
}

func contract(t *testing.T, svc service.Service) {
	ied, err := simied.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ied.Close)

	h, err := svc.Health(ctx)
	if err != nil || h.Status != "ok" || h.APIVersion != model.APIVersion || h.Mode != model.ModeObserve {
		t.Fatalf("Health = %+v, %v", h, err)
	}

	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := svc.Subscribe(sub, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Errors keep their code, and a failed connect its classified detail.
	closed := targetOf(t, ied.Addr)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed.Port = ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	_, err = svc.OpenSession(ctx, closed)
	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != service.CodeConnectFailed || se.Detail == nil || se.Detail.Stage != "tcp" {
		t.Fatalf("open on a closed port = %#v", err)
	}
	failed := next(t, events, "failed connect operation", func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "connect"
	})
	if failed.Operation.OK || failed.Operation.Error == nil || failed.Operation.Error.Stage != "tcp" {
		t.Errorf("failed connect = %+v", failed.Operation)
	}
	if _, err = svc.OpenSession(ctx, model.TargetConfig{Host: ""}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("open without host = %v", err)
	}
	for name, call := range map[string]func() error{
		"Browse":       func() error { _, e := svc.Browse(ctx, "nope", ""); return e },
		"Read":         func() error { _, e := svc.Read(ctx, "nope", model.ReadRequest{Refs: []string{"a/b.c[MX]"}}); return e },
		"CloseSession": func() error { return svc.CloseSession(ctx, "nope") },
	} {
		if err := call(); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("%s on an unknown session = %v, want not_found", name, err)
		}
	}

	// Profiles: create, replace, list, delete; the target is validated.
	if !slices.Contains(h.Features, model.FeatureProfiles) {
		t.Errorf("features = %v", h.Features)
	}
	omit, tsel := "", "0001"
	saved, err := svc.SaveProfile(ctx, model.Profile{Name: " Feeder 1 ", Target: model.TargetConfig{
		Host: "10.0.0.5", Addressing: &model.Addressing{RemoteTSelector: &tsel, LocalAPTitle: &omit},
	}})
	if err != nil || saved.ID == "" || saved.Name != "Feeder 1" || saved.Target.Port != model.DefaultPort || saved.UpdatedAt.IsZero() {
		t.Fatalf("SaveProfile = %+v, %v", saved, err)
	}
	saved.Notes = "bay 3"
	if again, err := svc.SaveProfile(ctx, saved); err != nil || again.ID != saved.ID || again.Notes != "bay 3" {
		t.Errorf("replace = %+v, %v", again, err)
	}
	bad := "zz"
	for name, p := range map[string]model.Profile{
		"no name":      {Target: model.TargetConfig{Host: "h"}},
		"no host":      {Name: "x"},
		"bad selector": {Name: "x", Target: model.TargetConfig{Host: "h", Addressing: &model.Addressing{RemotePSelector: &bad}}},
	} {
		if _, err := svc.SaveProfile(ctx, p); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("SaveProfile %s = %v", name, err)
		}
	}
	if _, err := svc.SaveProfile(ctx, model.Profile{ID: "nope", Name: "x", Target: model.TargetConfig{Host: "h"}}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("replace unknown = %v", err)
	}
	list, err := svc.Profiles(ctx)
	if err != nil || len(list) != 1 || list[0].Target.Addressing == nil || *list[0].Target.Addressing.LocalAPTitle != "" {
		t.Errorf("Profiles = %+v, %v", list, err)
	}
	if err := svc.DeleteProfile(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProfile(ctx, saved.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("delete twice = %v", err)
	}

	// A bad selector is refused before connecting.
	badTarget := targetOf(t, ied.Addr)
	badTarget.Addressing = &model.Addressing{RemoteTSelector: &bad}
	if _, err := svc.OpenSession(ctx, badTarget); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("open with a bad selector = %v", err)
	}

	// Open and discover, with explicit addressing the simulated IED accepts.
	target := targetOf(t, ied.Addr)
	psel, title, aeq := "00000001", "1.1.1.999.1", 12
	target.Addressing = &model.Addressing{RemotePSelector: &psel, RemoteAPTitle: &title, RemoteAEQualifier: &aeq, LocalAPTitle: &omit}
	info, err := svc.OpenSession(ctx, target)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if info.State != model.SessionConnected || info.Identity == nil || info.Discovery.LogicalNodes != 5 {
		t.Fatalf("session = %+v", info)
	}
	opened := next(t, events, "session event", func(ev model.Event) bool { return ev.Kind == model.EventSession })
	if opened.SessionID != info.ID || opened.Session == nil || opened.Seq == 0 {
		t.Errorf("event = %+v", opened)
	}
	if list, err := svc.Sessions(ctx); err != nil || len(list) != 1 || list[0].ID != info.ID {
		t.Errorf("Sessions = %+v, %v", list, err)
	}

	// Browse.
	lns, err := svc.Browse(ctx, info.ID, simied.Domain)
	if err != nil || !slices.ContainsFunc(lns, func(n model.Node) bool { return n.Name == "XCBR1" && n.LNClass == "XCBR" }) {
		t.Fatalf("Browse LD = %+v, %v", lns, err)
	}
	if _, err := svc.Browse(ctx, info.ID, "not a ref["); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("Browse bad ref = %v", err)
	}

	// Read.
	res, err := svc.Read(ctx, info.ID, model.ReadRequest{Refs: []string{
		simied.Domain + "/MMXU1.Hz.mag.f[MX]", simied.Domain + "/GGIO1.Ind1.stVal[ST]", simied.Domain + "/XCBR1.Nope[ST]",
	}})
	if err != nil || len(res.Values) != 3 {
		t.Fatalf("Read = %+v, %v", res, err)
	}
	if v := res.Values[0].Value; v == nil || v.Type != model.TypeFloat || v.JSON == nil {
		t.Errorf("Hz.mag.f = %+v", res.Values[0])
	} else if f, ok := v.JSON.(float64); !ok || f < 50.001 || f > 50.003 {
		t.Errorf("Hz.mag.f JSON = %#v", v.JSON)
	}
	if v := res.Values[1].Value; v == nil || v.JSON != true {
		t.Errorf("Ind1.stVal = %+v", res.Values[1])
	}
	if res.Values[2].Error == nil || res.Values[2].Value != nil {
		t.Errorf("unknown object = %+v", res.Values[2])
	}
	if _, err := svc.Read(ctx, info.ID, model.ReadRequest{}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("empty read = %v", err)
	}

	// Search.
	found, err := svc.Search(ctx, info.ID, model.SearchQuery{LNClass: "MMXU", Kind: model.NodeDO, FC: "MX"})
	if err != nil || len(found.Hits) != 2 {
		t.Errorf("Search = %+v, %v", found, err)
	}

	// Poll: samples arrive as events; the session shows the poller.
	if err := svc.StartPolling(ctx, info.ID, model.PollConfig{Refs: []string{simied.Domain + "/MMXU1.Hz.mag.f[MX]"}, IntervalMS: 200}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []uint64{1, 2} {
		ev := next(t, events, "sample", func(ev model.Event) bool { return ev.Kind == model.EventSample })
		if ev.Sample == nil || ev.Sample.Seq != want || len(ev.Sample.Values) != 1 || ev.Sample.Values[0].Value == nil {
			t.Fatalf("sample = %+v", ev.Sample)
		}
	}
	if list, _ := svc.Sessions(ctx); len(list) != 1 || list[0].Polling == nil || list[0].Polling.IntervalMS != 200 {
		t.Errorf("polling state = %+v", list)
	}
	if err := svc.StartPolling(ctx, info.ID, model.PollConfig{Refs: []string{"x"}, IntervalMS: 10}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("fast polling = %v", err)
	}
	if err := svc.StopPolling(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.StopPolling(ctx, info.ID); err != nil {
		t.Errorf("stop twice = %v", err)
	}

	// Inspection: data sets, reports, files, journals, setting groups.
	sets, err := svc.DataSets(ctx, info.ID, simied.Domain)
	if err != nil || len(sets) != 2 || len(sets[0].Members)+len(sets[1].Members) != 5 {
		t.Fatalf("DataSets = %+v, %v", sets, err)
	}
	if res, err := svc.ReadDataSet(ctx, info.ID, simied.Domain+"/LLN0.Meas"); err != nil || len(res.Values) != 2 || res.Values[0].Value == nil {
		t.Errorf("ReadDataSet = %+v, %v", res, err)
	}
	if _, err := svc.ReadDataSet(ctx, info.ID, simied.Domain+"/LLN0.Nope"); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("ReadDataSet unknown = %v", err)
	}
	if rcbs, err := svc.Reports(ctx, info.ID, simied.Domain); err != nil || len(rcbs) != 3 || rcbs[0].RptEna == nil {
		t.Errorf("Reports = %+v, %v", rcbs, err)
	}
	if files, err := svc.Files(ctx, info.ID, ""); err != nil || len(files) != 3 {
		t.Errorf("Files = %+v, %v", files, err)
	}
	if f, err := svc.ReadFile(ctx, info.ID, "COMTRADE/fault1.cfg"); err != nil || !strings.HasPrefix(string(f.Data), "Explorer demo IED") {
		t.Errorf("ReadFile = %q, %v", f.Data, err)
	}
	if _, err := svc.ReadFile(ctx, info.ID, "nope"); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("ReadFile missing = %v", err)
	}
	logs, err := svc.Journals(ctx, info.ID, simied.Domain)
	if err != nil || len(logs) != 1 {
		t.Fatalf("Journals = %v, %v", logs, err)
	}
	if j, err := svc.ReadJournal(ctx, info.ID, model.JournalRequest{LD: simied.Domain, Journal: logs[0]}); err != nil || len(j.Entries) == 0 {
		t.Errorf("ReadJournal = %+v, %v", j, err)
	}
	if sg, err := svc.SettingGroups(ctx, info.ID, simied.Domain); err != nil || sg.NumOfSGs != 2 {
		t.Errorf("SettingGroups = %+v, %v", sg, err)
	}

	// Export a snapshot and open it again.
	exp, err := svc.Export(ctx, info.ID, model.ExportRequest{Format: model.ExportSnapshot, Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}})
	if err != nil || !strings.Contains(exp.Text, model.SnapshotSchema) {
		t.Fatalf("Export = %v %.100s", err, exp.Text)
	}
	snapSession, err := svc.OpenSnapshot(ctx, model.File{Name: exp.Name, Data: []byte(exp.Text)})
	if err != nil || snapSession.Snapshot == nil {
		t.Fatalf("OpenSnapshot = %+v, %v", snapSession, err)
	}
	if res, err := svc.Read(ctx, snapSession.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); err != nil || res.Source != model.SourceSnapshot || res.Values[0].Value == nil {
		t.Errorf("Read snapshot = %+v, %v", res, err)
	}
	if err := svc.CloseSession(ctx, snapSession.ID); err != nil {
		t.Fatal(err)
	}

	// SCL: inspect, open offline, browse; no reads.
	file := model.File{Name: "demo.icd", Data: simied.ICD()}
	sclInfo, err := svc.InspectSCL(ctx, file)
	if err != nil || len(sclInfo.IEDs) != 1 || sclInfo.IEDs[0].AccessPoints[0].Target == nil {
		t.Fatalf("InspectSCL = %+v, %v", sclInfo, err)
	}
	if _, err := svc.InspectSCL(ctx, model.File{Name: "x", Data: []byte("<nope/>")}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("InspectSCL of garbage = %v", err)
	}
	offline, err := svc.OpenSCL(ctx, model.OpenSCLRequest{File: file, IED: simied.IEDName, AccessPoint: "S1"})
	if err != nil || offline.State != model.SessionOffline || offline.SCL == nil {
		t.Fatalf("OpenSCL = %+v, %v", offline, err)
	}
	if nodes, err := svc.Browse(ctx, offline.ID, simied.Domain+"/MMXU1"); err != nil || len(nodes) != 3 {
		t.Errorf("Browse offline = %+v, %v", nodes, err)
	}
	if _, err := svc.Read(ctx, offline.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("Read offline = %v", err)
	}
	if err := svc.CloseSession(ctx, offline.ID); err != nil {
		t.Fatal(err)
	}

	// Close.
	if err := svc.CloseSession(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	gone := next(t, events, "closed event", func(ev model.Event) bool {
		return ev.Kind == model.EventSession && ev.Session.State == model.SessionClosed && ev.SessionID == info.ID
	})
	if gone.SessionID != info.ID {
		t.Errorf("closed event = %+v", gone)
	}
	if list, _ := svc.Sessions(ctx); len(list) != 0 {
		t.Errorf("Sessions after close = %+v", list)
	}
}
