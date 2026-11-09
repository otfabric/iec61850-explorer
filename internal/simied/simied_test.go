// SPDX-License-Identifier: MIT

package simied

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"
)

func TestServesAndCounts(t *testing.T) {
	ied, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer ied.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := iec61850.Dial(ctx, ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := iec61850.ParseRef(Domain + "/MMXU1.Hz.mag.f[MX]")
	v, err := c.Read(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := v.Float32(); err != nil || f < 50.001 || f > 50.003 {
		t.Errorf("Hz = %v, %v", f, err)
	}
	_ = c.Close(ctx)
	if got := ied.Services()[mms.ServiceRead]; got != 1 {
		t.Errorf("Read count = %d", got)
	}
	if ied.Connections() != 1 {
		t.Errorf("connections = %d", ied.Connections())
	}
	ied.Close()
	ied.Close() // a second Close returns at once
}

func TestICDIsACopy(t *testing.T) {
	a := ICD()
	a[0] = 'x'
	if ICD()[0] == 'x' {
		t.Error("ICD returned the package's own bytes")
	}
}

func TestFileStore(t *testing.T) {
	ctx := context.Background()
	res, err := Files.List(ctx, mms.FileListRequest{FileSpec: "COMTRADE/", MaxEntries: 1})
	if err != nil || len(res.Entries) != 1 || !res.MoreFollows || res.Entries[0].Name != "COMTRADE/fault1.cfg" {
		t.Fatalf("List = %+v, %v", res, err)
	}
	res, _ = Files.List(ctx, mms.FileListRequest{FileSpec: "COMTRADE/", ContinueAfter: "COMTRADE/fault1.cfg"})
	if len(res.Entries) != 1 || res.Entries[0].Name != "COMTRADE/fault1.dat" || res.MoreFollows {
		t.Errorf("continued = %+v", res)
	}
	h, attrs, err := Files.Open(ctx, "demo.icd")
	if err != nil || attrs.Size != int64(len(ICD())) {
		t.Fatalf("Open = %v %+v", err, attrs)
	}
	var got []byte
	for {
		chunk, more, err := Files.Read(ctx, h, 1000)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, chunk...)
		if !more {
			break
		}
	}
	if string(got) != string(ICD()) {
		t.Error("content differs")
	}
	if err := Files.Close(ctx, h); err != nil {
		t.Error(err)
	}
	if _, _, err := Files.Open(ctx, "nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open missing = %v", err)
	}
	if _, _, err := Files.Read(ctx, "not a handle", 1); err == nil {
		t.Error("Read with a foreign handle")
	}
	for _, err := range []error{Files.Delete(ctx, "demo.icd"), Files.Rename(ctx, "a", "b"), Files.ObtainFile(ctx, "a", "b")} {
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("write operation = %v", err)
		}
	}
}

func TestControls(t *testing.T) {
	ied, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer ied.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := iec61850.Dial(ctx, ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx) //nolint:errcheck
	on := iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(true), CtlNum: 1}
	stVal := func(do string) bool {
		t.Helper()
		ref, _ := iec61850.ParseRef(Domain + "/" + do + ".stVal[ST]")
		v, err := c.Read(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := v.Bool()
		return b
	}
	ctl := func(do string) iec61850.Ref {
		ref, _ := iec61850.ParseRef(Domain + "/" + do)
		return ref
	}

	// One control model each.
	for do, m := range controlModels {
		if got, err := c.ReadCtlModel(ctx, ctl(do)); err != nil || got != m {
			t.Errorf("%s ctlModel = %v, %v; want %v", do, got, err, m)
		}
	}
	if err := c.Operate(ctx, ctl("GGIO1.SPCSO1"), on); err != nil || !stVal("GGIO1.SPCSO1") {
		t.Errorf("SPCSO1 direct: %v", err)
	}
	if _, err := c.Select(ctx, ctl("GGIO1.SPCSO2")); err != nil {
		t.Fatal(err)
	}
	if err := c.Operate(ctx, ctl("GGIO1.SPCSO2"), on); err != nil || !stVal("GGIO1.SPCSO2") {
		t.Errorf("SPCSO2 SBO: %v", err)
	}
	if term, err := c.OperateWait(ctx, ctl("GGIO1.SPCSO3"), on); err != nil || !term.Positive() || !stVal("GGIO1.SPCSO3") {
		t.Errorf("SPCSO3 direct enhanced: %+v, %v", term, err)
	}
	if err := c.SelectWithValue(ctx, ctl("GGIO1.SPCSO4"), on); err != nil {
		t.Fatal(err)
	}
	if term, err := c.OperateWait(ctx, ctl("GGIO1.SPCSO4"), on); err != nil || !term.Positive() || !stVal("GGIO1.SPCSO4") {
		t.Errorf("SPCSO4 SBO enhanced: %+v, %v", term, err)
	}

	// The breaker opens while the interlock is released (Ind1 true) ...
	pos := ctl("XCBR1.Pos")
	open := iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(false), CtlNum: 2}
	if err := c.SelectWithValue(ctx, pos, open); err != nil {
		t.Fatal(err)
	}
	if term, err := c.OperateWait(ctx, pos, open); err != nil || !term.Positive() {
		t.Fatalf("Pos open: %+v, %v", term, err)
	}
	posRef, _ := iec61850.ParseRef(Domain + "/XCBR1.Pos.stVal[ST]")
	if v, err := c.Read(ctx, posRef); err != nil {
		t.Fatal(err)
	} else if bits, _ := v.MMS().BitString(); len(bits) == 0 || bits[0]&0xC0 != 0x40 {
		t.Errorf("Pos.stVal = %v, want off", v)
	}
	// ... and is blocked by interlocking when Ind1 is false.
	ied.Set("GGIO1$ST$Ind1$stVal", mms.NewBoolean(false))
	closeIt := iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(true), CtlNum: 3, Check: iec61850.CheckInterlockCheck}
	if err := c.SelectWithValue(ctx, pos, closeIt); err != nil {
		t.Fatal(err)
	}
	term, err := c.OperateWait(ctx, pos, closeIt)
	if !errors.Is(err, iec61850.ErrCommandTerminationNegative) || term.LastApplError.AddCause != iec61850.AddCauseBlockedByInterlocking {
		t.Errorf("Pos close while interlocked: %+v, %v", term, err)
	}
}

// Asked for a synchrocheck, the single point controls refuse.
func TestControlsSynchrocheck(t *testing.T) {
	ied, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer ied.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := iec61850.Dial(ctx, ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx) //nolint:errcheck
	ref, _ := iec61850.ParseRef(Domain + "/GGIO1.SPCSO1")
	err = c.Operate(ctx, ref, iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(true), CtlNum: 9, Check: iec61850.CheckSynchroCheck})
	var ce *iec61850.ControlError
	if !errors.As(err, &ce) || ce.AddCause != iec61850.AddCauseBlockedBySynchrocheck {
		t.Errorf("operate with synchrocheck = %v", err)
	}
}
