// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otfabric/go-iec61850/scl"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

func TestProfileEdges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := New(Options{DataDir: dir})
	defer e.Close()
	if h, _ := e.Health(ctx); len(h.Features) != 1 || h.Features[0] != model.FeatureProfiles {
		t.Errorf("features = %v", h.Features)
	}
	if err := e.DeleteProfile(ctx, "nope"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("delete unknown = %v", err)
	}
	p, err := e.SaveProfile(ctx, model.Profile{Name: "a", Target: model.TargetConfig{Host: "h"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.Profiles(ctx); len(list) != 0 {
		t.Errorf("after delete = %v", list)
	}
	for name, bad := range map[string]model.Profile{
		"long notes": {Name: "a", Notes: string(make([]byte, model.MaxProfileNotesLen+1)), Target: model.TargetConfig{Host: "h"}},
		"port":       {Name: "a", Target: model.TargetConfig{Host: "h", Port: -1}},
	} {
		if _, err := e.SaveProfile(ctx, bad); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s = %v", name, err)
		}
	}
	// The profile limit.
	list := make([]model.Profile, model.MaxProfiles)
	for i := range list {
		list[i] = model.Profile{ID: newID(), Name: "p", Target: model.TargetConfig{Host: "h", Port: 102}}
	}
	if err := e.store.save(list); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveProfile(ctx, model.Profile{Name: "one more", Target: model.TargetConfig{Host: "h"}}); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("over the limit = %v", err)
	}
	// A data directory that is gone: the error is reported.
	e2 := New(Options{DataDir: filepath.Join(dir, "missing")})
	defer e2.Close()
	if _, err := e2.SaveProfile(ctx, model.Profile{Name: "x", Target: model.TargetConfig{Host: "h"}}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("save into a missing dir = %v", err)
	}
	// An oversized file is refused.
	big := filepath.Join(dir, "big")
	if err := os.Mkdir(big, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(big, "profiles.json"), make([]byte, maxProfilesFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{DataDir: big}).Profiles(ctx); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("oversized file = %v", err)
	}
	if _, err := New(Options{}).SaveProfile(ctx, model.Profile{}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("no store save = %v", err)
	}
	if err := New(Options{}).DeleteProfile(ctx, "x"); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("no store delete = %v", err)
	}
}

func TestKindAndNames(t *testing.T) {
	for name, want := range map[string]scl.DocumentKind{
		"a.SCD": scl.KindSCD, "b.cid": scl.KindCID, "c.icd": scl.KindICD, "d.iid": scl.KindIID, "e.ssd": scl.KindSSD, "f.xml": "", "": "",
	} {
		if got := kindOf(name); got != want {
			t.Errorf("kindOf(%q) = %q", name, got)
		}
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := exportBase(model.SessionInfo{SCL: &model.SCLOrigin{IED: "IED 1"}}, at); got != "iec61850-IED_1-20260102T030405Z" {
		t.Errorf("SCL name = %q", got)
	}
	if got := exportBase(model.SessionInfo{}, at); got != "iec61850-ied-20260102T030405Z" {
		t.Errorf("empty name = %q", got)
	}
	for in, want := range map[[2]string]string{{"", "a"}: "a", {"a", "b"}: "a.b", {"a", "[2]"}: "a[2]"} {
		if got := joinComponent(in[0], in[1]); got != want {
			t.Errorf("joinComponent(%v) = %q", in, got)
		}
	}
}

// Parser findings of a document with semantic errors are listed.
func TestInspectSCLDiagnostics(t *testing.T) {
	doc := []byte(`<?xml version="1.0"?><SCL xmlns="http://www.iec.ch/61850/2003/SCL" version="2007" revision="B">
<Header id="x"/><IED name="A"><AccessPoint name="S1"><Server><LDevice inst="LD0"><LN0 lnClass="LLN0" inst="" lnType="MISSING"/></LDevice></Server></AccessPoint></IED></SCL>`)
	info, err := New(Options{}).InspectSCL(context.Background(), model.File{Name: "x.cid", Data: doc})
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != "cid" || len(info.Diagnostics) == 0 || len(info.IEDs) != 1 || info.IEDs[0].AccessPoints[0].Target != nil {
		t.Errorf("info = %+v", info)
	}
}
