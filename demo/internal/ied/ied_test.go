// SPDX-License-Identifier: MIT

package ied

import (
	"context"
	"errors"
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

var now = time.Date(2026, 11, 10, 9, 20, 0, 0, time.UTC)

func start(t *testing.T, d *site.IED, o Options) (*IED, *iec61850.Client) {
	t.Helper()
	if o.Now == nil {
		o.Now = func() time.Time { return now }
	}
	i, err := Start(d, "127.0.0.1:0", o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(i.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := iec61850.Dial(ctx, i.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return i, c
}

// TestServeAll serves every IED and discovers its model.
func TestServeAll(t *testing.T) {
	for _, d := range site.ServedIEDs() {
		_, c := start(t, d, Options{})
		lds, err := c.ListLogicalDevices(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(lds) != len(d.LDs) {
			t.Errorf("%s: %d LDs, want %d", d.Name, len(lds), len(d.LDs))
		}
		if _, err := c.TreeWithOptions(t.Context(), iec61850.TreeOptions{IncludeFCs: true}); err != nil {
			t.Errorf("%s: tree: %v", d.Name, err)
		}
	}
}

// TestFeederControls: the disconnector is interlocked while the breaker
// is closed; the breaker opens (SBO, enhanced security); then the
// disconnector opens.
func TestFeederControls(t *testing.T) {
	_, c := start(t, site.Feeder(1), Options{})
	ctx := t.Context()
	ds := ref(t, site.F1Name+"CTRL/CSWI2.Pos")
	cb := ref(t, site.F1Name+"CTRL/CSWI1.Pos")
	open := iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(false), CtlNum: 1}
	if err := c.SelectWithValue(ctx, ds, open); err != nil {
		t.Fatal(err)
	}
	_, err := c.OperateWait(ctx, ds, open)
	var ce *iec61850.ControlError
	if !errors.As(err, &ce) || ce.AddCause != iec61850.AddCauseBlockedByInterlocking {
		t.Fatalf("disconnector: %v, want blocked by interlocking", err)
	}
	for _, r := range []iec61850.Ref{cb, ds} {
		open.CtlNum++
		if err := c.SelectWithValue(ctx, r, open); err != nil {
			t.Fatal(err)
		}
		term, err := c.OperateWait(ctx, r, open)
		if err != nil || !term.Positive() {
			t.Fatalf("%s: %v %+v", r, err, term)
		}
	}
	v, err := c.Read(ctx, ref(t, site.F1Name+"CTRL/XSWI1.Pos.stVal[ST]"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("XSWI1 %v", v)
	res, err := c.ReadJournal(ctx, site.F1Name+"CTRL", "EventLog", now.Add(-24*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 6 {
		t.Errorf("%d journal entries, want 6", len(res.Entries))
	}
}

// TestSettingGroups switches feeder 1 to group 2 and reads the I> start.
func TestSettingGroups(t *testing.T) {
	_, c := start(t, site.Feeder(1), Options{Deviate: true})
	ctx := t.Context()
	sg := ref(t, site.F1Name+"PROT/PTOC1.StrVal.setMag.f[SG]")
	v, err := c.Read(ctx, sg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("group 1: %v", v)
	if err := c.SelectActiveSG(ctx, site.F1Name+"PROT", 2); err != nil {
		t.Fatal(err)
	}
	v, err = c.Read(ctx, sg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("group 2: %v", v)
	files, err := c.ListFiles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("files %+v", files)
	}
}

func ref(t *testing.T, s string) iec61850.Ref {
	t.Helper()
	r, err := iec61850.ParseRef(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
