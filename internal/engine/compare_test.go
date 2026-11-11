// SPDX-License-Identifier: MIT

package engine

import (
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func category(t *testing.T, c model.Comparison, name string) model.CompareCategory {
	t.Helper()
	for _, cat := range c.Categories {
		if cat.Name == name {
			return cat
		}
	}
	t.Fatalf("no category %q in %+v", name, c.Categories)
	return model.CompareCategory{}
}

func openICD(t *testing.T, e *Engine, data []byte) model.SessionInfo {
	t.Helper()
	info, err := e.OpenSCL(t.Context(), model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// TestCompareSCLWithIED: the simulated IED serves demo.icd, so the file and
// the IED agree; the same file with introduced mismatches shows each of
// them, with the evidence on both sides (the M3 exit criterion).
func TestCompareSCLWithIED(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	same := openICD(t, e, simied.ICD())

	c, err := e.Compare(ctx, model.CompareRequest{A: same.ID, B: live.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.A.Source != model.SourceSCL || c.B.Source != model.SourceIED || !strings.Contains(c.A.Label, "demo.icd") {
		t.Errorf("sides = %+v / %+v", c.A, c.B)
	}
	if len(c.Differences) != 0 || c.Truncated {
		t.Errorf("the IED serving the file differs from it: %+v", c.Differences)
	}
	for _, name := range []string{model.CompareModel, model.CompareDataSets, model.CompareReports} {
		cat := category(t, c, name)
		if !cat.Compared || cat.Items == 0 || cat.Same != cat.Items || cat.Reason != "" {
			t.Errorf("%s = %+v", name, cat)
		}
	}

	icd := string(simied.ICD())
	for _, r := range [][2]string{
		// A logical node the IED does not have.
		{`<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/>`, `<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/><LN lnClass="LPHD" inst="9" lnType="LPHD_T"/>`},
		// A data set member left out.
		{`<FCDA ldInst="LD0" lnClass="MMXU" lnInst="1" doName="TotW" fc="MX"/>`, ``},
		// A report control block with another revision and integrity period.
		{`rptID="Meas" datSet="Meas" confRev="1"`, `rptID="Meas" datSet="Meas" confRev="7" intgPd="1000"`},
		// A data set the IED does not have.
		{`<DataSet name="Meas"`, `<DataSet name="Extra" desc="x"><FCDA ldInst="LD0" lnClass="MMXU" lnInst="1" doName="Hz" fc="MX"/></DataSet><DataSet name="Meas"`},
	} {
		if !strings.Contains(icd, r[0]) {
			t.Fatalf("demo.icd has no %q", r[0])
		}
		icd = strings.Replace(icd, r[0], r[1], 1)
	}
	changed := openICD(t, e, []byte(icd))
	c, err = e.Compare(ctx, model.CompareRequest{A: changed.ID, B: live.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.Difference{}
	for _, d := range c.Differences {
		got[d.Category+" "+d.Kind+" "+d.Ref+" "+d.Field] = d
	}
	ld := simied.Domain
	want := []string{
		"model only_a " + ld + "/LPHD9 ",
		"datasets changed " + ld + "/LLN0.Meas members",
		"datasets only_a " + ld + "/LLN0.Extra ",
		"reports changed " + ld + "/LLN0.RP.urcbMeas01 conf_rev",
		"reports changed " + ld + "/LLN0.RP.urcbMeas01 intg_pd_ms",
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing difference %q in %+v", k, c.Differences)
		}
	}
	if len(got) != len(want) {
		t.Errorf("differences = %+v, want only %v", c.Differences, want)
	}
	if d := got["datasets changed "+ld+"/LLN0.Meas members"]; d.Note != "1 members / 2 members" || !strings.Contains(d.B, "TotW") {
		t.Errorf("members difference = %+v", d)
	}
	if d := got["reports changed "+ld+"/LLN0.RP.urcbMeas01 conf_rev"]; d.A != "7" || d.B != "1" {
		t.Errorf("conf_rev difference = %+v", d)
	}
	if cat := category(t, c, model.CompareModel); cat.OnlyA != 1 || cat.OnlyB != 0 {
		t.Errorf("model = %+v", cat)
	}
	if cat := category(t, c, model.CompareReports); cat.Changed != 1 {
		t.Errorf("reports = %+v", cat)
	}

	// Reversed sides swap only_a and only_b.
	c, err = e.Compare(ctx, model.CompareRequest{A: live.ID, B: changed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareModel); cat.OnlyB != 1 || cat.OnlyA != 0 {
		t.Errorf("reversed model = %+v", cat)
	}
}

func TestCompareSnapshotAndErrors(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	exp, err := e.Export(ctx, live.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := e.OpenSnapshot(ctx, model.File{Name: exp.Name, Data: []byte(exp.Text)})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Compare(ctx, model.CompareRequest{A: snap.ID, B: live.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.A.Source != model.SourceSnapshot || len(c.Differences) != 0 {
		t.Errorf("snapshot of the IED vs the IED: %+v %+v", c.A, c.Differences)
	}
	if cat := category(t, c, model.CompareModel); !cat.Compared || cat.Same == 0 {
		t.Errorf("model = %+v", cat)
	}
	for _, name := range []string{model.CompareDataSets, model.CompareReports} {
		if cat := category(t, c, name); cat.Compared || !strings.HasPrefix(cat.Reason, "A: a snapshot") {
			t.Errorf("%s = %+v", name, cat)
		}
	}
	c, err = e.Compare(ctx, model.CompareRequest{A: live.ID, B: snap.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareReports); cat.Compared || !strings.HasPrefix(cat.Reason, "B: a snapshot") {
		t.Errorf("reversed reports = %+v", cat)
	}

	for _, req := range []model.CompareRequest{{}, {A: live.ID, B: live.ID}, {A: live.ID}} {
		if _, err := e.Compare(ctx, req); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("Compare(%+v) = %v", req, err)
		}
	}
	for _, req := range []model.CompareRequest{{A: "nope", B: live.ID}, {A: live.ID, B: "nope"}} {
		if _, err := e.Compare(ctx, req); service.CodeOf(err) != service.CodeNotFound {
			t.Errorf("Compare(%+v) = %v", req, err)
		}
	}
}
