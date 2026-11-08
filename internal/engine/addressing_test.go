// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

func ptr[T any](v T) *T { return &v }

func TestApplyAddressing(t *testing.T) {
	var o mms.DialOptions
	if err := applyAddressing(nil, &o); err != nil || o.Transport.RemoteTSelector != nil {
		t.Fatalf("nil = %v %+v", err, o)
	}
	a := &model.Addressing{
		LocalTSelector: ptr(""), RemoteTSelector: ptr("0x00 01"), RemoteSSelector: ptr("0001"), RemotePSelector: ptr("00000001"),
		LocalPSelector: ptr("01"), LocalSSelector: ptr("02"),
		LocalAPTitle: ptr(""), RemoteAPTitle: ptr("1,3,9999,23"), LocalAEQualifier: ptr(0), RemoteAEQualifier: ptr(23),
	}
	if err := applyAddressing(a, &o); err != nil {
		t.Fatal(err)
	}
	if o.Transport.LocalTSelector == nil || len(o.Transport.LocalTSelector) != 0 || !slices.Equal(o.Transport.RemoteTSelector, []byte{0, 1}) ||
		!slices.Equal(o.ISO.RemotePSelector, []byte{0, 0, 0, 1}) || !slices.Equal(o.ISO.LocalSSelector, []byte{2}) {
		t.Errorf("selectors = %+v %+v", o.Transport, o.ISO)
	}
	if o.ISO.LocalAPTitle == nil || len(o.ISO.LocalAPTitle) != 0 || !slices.Equal([]int(o.ISO.RemoteAPTitle), []int{1, 3, 9999, 23}) || o.ISO.RemoteAEQualifier != 23 {
		t.Errorf("titles = %+v", o.ISO)
	}
	for name, a := range map[string]*model.Addressing{
		"hex":        {RemoteSSelector: ptr("xyz")},
		"long tsel":  {RemoteTSelector: ptr("0102030405")},
		"long psel":  {RemotePSelector: ptr("000102030405060708090a0b0c0d0e0f10")},
		"one arc":    {RemoteAPTitle: ptr("1")},
		"word":       {LocalAPTitle: ptr("1.a.2")},
		"first arc":  {RemoteAPTitle: ptr("3.1")},
		"second arc": {RemoteAPTitle: ptr("1.40")},
		"aeq":        {RemoteAEQualifier: ptr(-1)},
	} {
		if err := applyAddressing(a, &mms.DialOptions{}); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := parseAPTitle("2.999.1"); err != nil {
		t.Errorf("2.999.1 = %v", err)
	}
}

func TestProfilesStore(t *testing.T) {
	ctx := context.Background()
	if _, err := New(Options{}).Profiles(ctx); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("without data dir = %v", err)
	}
	dir := t.TempDir()
	e := New(Options{DataDir: dir})
	defer e.Close()
	for _, name := range []string{"b", "A", "c"} {
		if _, err := e.SaveProfile(ctx, model.Profile{Name: name, Target: model.TargetConfig{Host: "h"}}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := e.Profiles(ctx)
	if len(list) != 3 || list[0].Name != "A" || list[2].Name != "c" {
		t.Errorf("order = %+v", list)
	}
	info, err := os.Stat(filepath.Join(dir, "profiles.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file = %v, %v", info, err)
	}
	for name, p := range map[string]model.Profile{
		"control": {Name: "a\x00b", Target: model.TargetConfig{Host: "h"}},
		"notes":   {Name: "a", Notes: "x\x07", Target: model.TargetConfig{Host: "h"}},
		"long":    {Name: string(make([]byte, model.MaxProfileNameLen+1)), Target: model.TargetConfig{Host: "h"}},
	} {
		if _, err := e.SaveProfile(ctx, p); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := e.SaveProfile(ctx, model.Profile{Name: "n", Notes: "line 1\nline 2\tx", Target: model.TargetConfig{Host: "h"}}); err != nil {
		t.Errorf("notes with line breaks = %v", err)
	}
	// A broken file is reported, not overwritten.
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Profiles(ctx); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("broken file = %v", err)
	}
	if _, err := e.SaveProfile(ctx, model.Profile{Name: "x", Target: model.TargetConfig{Host: "h"}}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("save over broken file = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(`{"schema":"other","profiles":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteProfile(ctx, "x"); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("other schema = %v", err)
	}
}
