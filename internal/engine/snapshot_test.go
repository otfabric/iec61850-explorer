// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/csv"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// A snapshot taken from a live session opens again as an offline session
// with the same tree, and its reads return the stored values, marked as
// such.
func TestSnapshotRoundTrip(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	refs := []string{simied.Domain + "/MMXU1.Hz[MX]", simied.Domain + "/GGIO1.Ind1.stVal[ST]"}
	file, err := e.Export(ctx, live.ID, model.ExportRequest{Format: model.ExportSnapshot, Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(file.Name, "iec61850-Explorer_demo_IED-127.0.0.1-") || !strings.HasSuffix(file.Name, ".json") || file.MIME != "application/json" {
		t.Errorf("file = %s %s", file.Name, file.MIME)
	}
	var snap model.Snapshot
	if err := json.Unmarshal([]byte(file.Text), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Schema != model.SnapshotSchema || snap.App.Name != model.AppName || len(snap.Values) != 2 || snap.Values[0].Value == nil ||
		snap.Identity == nil || len(snap.Nodes) == 0 || snap.ValuesAt.IsZero() {
		t.Fatalf("snapshot = %+v", snap)
	}

	off, err := e.OpenSnapshot(ctx, model.File{Name: file.Name, Data: []byte(file.Text)})
	if err != nil {
		t.Fatal(err)
	}
	if off.State != model.SessionOffline || off.Snapshot == nil || off.Snapshot.File != file.Name || off.Identity == nil ||
		off.Discovery.LogicalNodes != live.Discovery.LogicalNodes || off.Discovery.Nodes != live.Discovery.Nodes {
		t.Errorf("offline = %+v (live %+v)", off, live.Discovery)
	}
	a, _ := e.Browse(ctx, live.ID, simied.Domain+"/MMXU1.Hz")
	b, _ := e.Browse(ctx, off.ID, simied.Domain+"/MMXU1.Hz")
	if !slices.EqualFunc(a, b, func(x, y model.Node) bool {
		return x.Ref == y.Ref && slices.Equal(x.FCs, y.FCs) && x.Children == y.Children
	}) {
		t.Errorf("browse live %+v, snapshot %+v", a, b)
	}
	res, err := e.Read(ctx, off.ID, model.ReadRequest{Refs: append(refs, simied.Domain+"/XCBR1.Pos[ST]")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != model.SourceSnapshot || !res.At.Equal(snap.ValuesAt) || res.Values[0].Value == nil ||
		res.Values[1].Value.JSON != true || res.Values[2].Error == nil || res.Values[2].Error.Code != model.ErrCodeNotFound {
		t.Errorf("snapshot read = %+v", res)
	}
	if err := e.StartPolling(ctx, off.ID, model.PollConfig{Refs: refs}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("poll snapshot = %v", err)
	}

	// A snapshot of the offline session: its model, no fresh values.
	again, err := e.Export(ctx, off.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil || !strings.Contains(again.Text, `"values": []`) {
		t.Errorf("snapshot of a snapshot = %v %.200s", err, again.Text)
	}
}

func TestExportCSV(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	s, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	file, err := e.Export(ctx, s.ID, model.ExportRequest{Format: model.ExportCSV, Refs: []string{
		simied.Domain + "/MMXU1.TotW[MX]", simied.Domain + "/LLN0.NamPlt.vendor[DC]", simied.Domain + "/MMXU1.Nope[MX]",
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(file.Text)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rows[0], csvColumns) || file.MIME != "text/csv" {
		t.Fatalf("header = %v", rows[0])
	}
	byComponent := map[string][]string{}
	for _, r := range rows[1:] {
		byComponent[r[1]+"|"+r[2]] = r
	}
	if r := byComponent[simied.Domain+"/MMXU1.TotW[MX]|mag.f"]; r == nil || r[6] != "1.245e+06" || r[4] != "MMXU1$MX$TotW" {
		t.Errorf("mag.f row = %v", r)
	}
	if r := byComponent[simied.Domain+"/MMXU1.TotW[MX]|q"]; r == nil || r[7] != "questionable" {
		t.Errorf("q row = %v", r)
	}
	if r := byComponent[simied.Domain+"/LLN0.NamPlt.vendor[DC]|"]; r == nil || r[6] != "OT Fabric" {
		t.Errorf("vendor row = %v", r)
	}
	if r := byComponent[simied.Domain+"/MMXU1.Nope[MX]|"]; r == nil || r[8] == "" {
		t.Errorf("error row = %v", r)
	}
	for name, req := range map[string]model.ExportRequest{
		"format":  {Format: "xml"},
		"no refs": {Format: model.ExportCSV},
		"many":    {Format: model.ExportSnapshot, Refs: make([]string, model.MaxReadRefs+1)},
	} {
		if _, err := e.Export(ctx, s.ID, req); service.CodeOf(err) == "" {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{
		"=1+1": "'=1+1", "@SUM(A1)": "'@SUM(A1)", "-7": "-7", "+1e3": "+1e3", "-x": "'-x", "plain": "plain", "": "",
	} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
	if got := exportBase(model.SessionInfo{Target: model.TargetConfig{Host: "::1"}}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); got != "iec61850-1-20260102T030405Z" {
		t.Errorf("exportBase = %q", got)
	}
}

func TestOpenSnapshotRejects(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	for name, data := range map[string]string{
		"empty":      "",
		"not json":   "{",
		"schema":     `{"schema":"x"}`,
		"orphan":     `{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LN1","ref":"LD/LN1"}]}`,
		"duplicate":  `{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LD","ref":"LD"},{"name":"LD","ref":"LD"}]}`,
		"fc in node": `{"schema":"` + model.SnapshotSchema + `","nodes":[{"name":"LD","ref":"LD"},{"name":"LN1","ref":"LD/LN1[MX]"}]}`,
	} {
		if _, err := e.OpenSnapshot(ctx, model.File{Name: name, Data: []byte(data)}); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s = %v", name, err)
		}
	}
}
