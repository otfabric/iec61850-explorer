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
