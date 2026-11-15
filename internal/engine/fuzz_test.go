// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// allocated returns the bytes fn allocates.
func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// A snapshot file either opens as a session whose tree holds each of its
// nodes once, and exports to a snapshot that opens to the same tree, or is
// refused as invalid or too large; what it allocates grows with the file.
func FuzzOpenSnapshot(f *testing.F) {
	e := New(Options{})
	f.Cleanup(e.Close)
	ctx := f.Context()
	scl, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		f.Fatal(err)
	}
	file, err := e.Export(ctx, scl.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		f.Fatal(err)
	}
	_ = e.CloseSession(ctx, scl.ID)
	f.Add([]byte(file.Text))
	f.Add([]byte(`{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LD","ref":"LD"},{"name":"LLN0","ref":"LD/LLN0"}]}`))
	f.Add([]byte(`{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LLN0","ref":"LD/LLN0"},{"name":"LD","ref":"LD"}]}`))
	f.Add([]byte(`{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LD","ref":"LD"},{"name":"LD","ref":"LD"}]}`))
	f.Add([]byte(`{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"x","ref":"LD/LLN0.Mod[ST]"}]}`))
	f.Add([]byte(`{"schema":"other"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var info model.SessionInfo
		var err error
		n := allocated(func() {
			info, err = e.OpenSnapshot(ctx, model.File{Name: "f.json", Data: data})
		})
		if budget := 64*uint64(len(data)) + 1<<20; n > budget {
			t.Fatalf("%d bytes of snapshot allocated %d bytes, budget %d", len(data), n, budget)
		}
		if err != nil {
			if c := service.CodeOf(err); c != service.CodeInvalidArgument && c != service.CodeLimitExceeded {
				t.Fatalf("refused with %q: %v", c, err)
			}
			return
		}
		defer func() { _ = e.CloseSession(ctx, info.ID) }()
		var snap model.Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			t.Fatalf("opened, but does not decode: %v", err)
		}
		d := info.Discovery
		if info.Kind != model.SessionKindSnapshot || d.LogicalDevices+d.LogicalNodes+d.Nodes != len(snap.Nodes) {
			t.Fatalf("%d nodes opened as %+v (%s)", len(snap.Nodes), d, info.Kind)
		}
		again, err := e.Export(ctx, info.ID, model.ExportRequest{Format: model.ExportSnapshot})
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		reopened, err := e.OpenSnapshot(ctx, model.File{Name: again.Name, Data: []byte(again.Text)})
		if err != nil {
			t.Fatalf("the exported snapshot does not open: %v", err)
		}
		_ = e.CloseSession(ctx, reopened.ID)
		if reopened.Discovery.LogicalDevices != d.LogicalDevices || reopened.Discovery.LogicalNodes != d.LogicalNodes || reopened.Discovery.Nodes != d.Nodes {
			t.Fatalf("reopened as %+v, opened as %+v", reopened.Discovery, d)
		}
	})
}

// An SCL file either opens as a session within the node bound, or is
// refused as invalid, too large or not found; what it allocates is bounded
// by its size and the node bound, whatever its data types expand to.
func FuzzOpenSCL(f *testing.F) {
	const nodes = 1 << 12
	withSCLNodes(f, nodes)
	e := New(Options{})
	f.Cleanup(e.Close)
	ctx := f.Context()
	icd := simied.ICD()
	f.Add(icd)
	f.Add(icd[:len(icd)/2])
	f.Add(wideSCL(3, 2))
	f.Add(wideSCL(64, 63)) // just over the bound
	f.Add(deepSCL(3))
	f.Add(deepSCL(70))
	f.Add([]byte(`<SCL xmlns="http://www.iec.ch/61850/2003/SCL"/>`))
	f.Add([]byte("not xml"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var info model.SessionInfo
		var err error
		n := allocated(func() {
			info, err = e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "f.icd", Data: data}})
		})
		if budget := 512*uint64(len(data)) + 4096*nodes + 8<<20; n > budget {
			t.Fatalf("%d bytes of SCL allocated %d bytes, budget %d", len(data), n, budget)
		}
		if err != nil {
			switch c := service.CodeOf(err); c {
			case service.CodeInvalidArgument, service.CodeLimitExceeded, service.CodeNotFound:
			default:
				t.Fatalf("refused with %q: %v", c, err)
			}
			return
		}
		defer func() { _ = e.CloseSession(ctx, info.ID) }()
		d := info.Discovery
		if info.Kind != model.SessionKindSCL || d.Nodes > nodes {
			t.Fatalf("opened as %+v (%s), bound %d", d, info.Kind, nodes)
		}
		if _, err := e.Browse(ctx, info.ID, ""); err != nil {
			t.Fatalf("browse: %v", err)
		}
	})
}
