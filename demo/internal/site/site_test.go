// SPDX-License-Identifier: MIT

package site

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/goose"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-iec61850/sv"
)

func parse(t *testing.T, name string, b []byte) *scl.Result {
	t.Helper()
	res, err := scl.ParseBytes(b, scl.ParseOptions{ValidateSemantic: true})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, d := range res.Diagnostics {
		t.Logf("%s: [%s] %s %s: %s", name, d.Severity, d.Code, d.Path, d.Message)
	}
	if res.HasErrors() {
		t.Errorf("%s: SCL has errors", name)
	}
	return res
}

// TestSCD parses and validates the SCD, builds a server model of every
// served IED from it and binds every GOOSE and SV control block.
func TestSCD(t *testing.T) {
	res := parse(t, "SCD", SCD())
	if res.Version.Schema != scl.Version2007B4 {
		t.Errorf("SCD schema %s, want 2007B4", res.Version.Schema)
	}
	for _, d := range ServedIEDs() {
		m, err := iec61850.NewServerModelFromSCL(res.Document, d.Name, "")
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		for _, w := range m.Warnings {
			t.Errorf("%s: model warning: %s", d.Name, w)
		}
	}
	gcbs, err := goose.ControlBlocksFromSCL(res.Document)
	if err != nil {
		t.Fatal(err)
	}
	if len(gcbs) != 6 {
		t.Errorf("%d GoCBs, want 6", len(gcbs))
	}
	svcbs, err := sv.ControlBlocksFromSCL(res.Document)
	if err != nil {
		t.Fatal(err)
	}
	if len(svcbs) != 1 || svcbs[0].SmpRate != 80 || len(svcbs[0].Layout) != 16 {
		t.Errorf("SVCBs %+v", svcbs)
	}
}

// TestCIDs parses every CID (feeder 2 as schema 1.7) and serves it.
func TestCIDs(t *testing.T) {
	for _, d := range ServedIEDs() {
		res := parse(t, d.Name, CID(d))
		want := scl.Version2007B4
		if d.Ed1 {
			want = scl.Version17
		}
		if res.Version.Schema != want {
			t.Errorf("%s: schema %s, want %s", d.Name, res.Version.Schema, want)
		}
		m, err := iec61850.NewServerModelFromSCL(res.Document, d.Name, "")
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		for _, w := range m.Warnings {
			t.Errorf("%s: model warning: %s", d.Name, w)
		}
	}
}

// TestFilesCurrent checks that demo/scl and demo/nsd hold what the
// generator writes (make demo regenerates them).
func TestFilesCurrent(t *testing.T) {
	want := map[string][]byte{"scl/AA1_Northfield.scd": SCD(), "nsd/corvane-7-4x-2026a.nsd": VendorNSD()}
	for _, d := range ServedIEDs() {
		want["scl/"+d.Name+".cid"] = CID(d)
	}
	for name, b := range want {
		got, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s is not what the generator writes: run make demo", name)
		}
	}
}
