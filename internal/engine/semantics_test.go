// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/otfabric/go-iec61850-model/catalog"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func findingKeys(c model.Conformance) map[string]string {
	out := map[string]string{}
	for _, f := range c.Findings {
		out[f.Code+" "+f.Ref] = f.Severity
	}
	return out
}

// TestConformance: demo.icd against IEC 61850-7-4:2007B6 lacks four
// mandatory data objects; an unknown class, an extra data object and a CDC
// the namespace does not give are found in a changed file (M3c).
func TestConformance(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	ld := simied.Domain
	want := map[string]string{
		"missing-mandatory-do " + ld + "/LPHD1.PhyNam": model.SeverityError,
		"missing-mandatory-do " + ld + "/XCBR1.Loc":    model.SeverityError,
		"missing-mandatory-do " + ld + "/XCBR1.BlkOpn": model.SeverityError,
		"missing-mandatory-do " + ld + "/XCBR1.BlkCls": model.SeverityError,
	}
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{openICD(t, e, simied.ICD()).ID, live.ID} {
		c, err := e.Conformance(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got := findingKeys(c)
		if len(got) != len(want) || c.LogicalNodes != 5 || c.DataObjects == 0 || len(c.Namespaces) != 5 || c.Truncated {
			t.Errorf("conformance = %+v", c)
		}
		for k, sev := range want {
			if got[k] != sev {
				t.Errorf("finding %q = %q, want %q", k, got[k], sev)
			}
		}
	}

	icd := string(simied.ICD())
	for _, r := range [][2]string{
		{`<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/>`, `<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/><LN lnClass="ZXYZ" inst="1" lnType="LPHD_T"/>`},
		{`<DO name="TotW" type="MV_T"/>`, `<DO name="TotW" type="MV_T"/><DO name="Private1" type="MV_T"/>`},
		{`<DOType id="MV_T" cdc="MV">`, `<DOType id="MV_T" cdc="CMV">`},
	} {
		if !strings.Contains(icd, r[0]) {
			t.Fatalf("demo.icd has no %q", r[0])
		}
		icd = strings.Replace(icd, r[0], r[1], 1)
	}
	c, err := e.Conformance(ctx, openICD(t, e, []byte(icd)).ID)
	if err != nil {
		t.Fatal(err)
	}
	got := findingKeys(c)
	for k, sev := range map[string]string{
		"unknown-ln-class " + ld + "/ZXYZ1":    model.SeverityWarning,
		"unknown-do " + ld + "/MMXU1.Private1": model.SeverityInfo,
		"cdc-mismatch " + ld + "/MMXU1.Hz":     model.SeverityWarning,
		"cdc-mismatch " + ld + "/MMXU1.TotW":   model.SeverityWarning,
	} {
		if got[k] != sev {
			t.Errorf("finding %q = %q, want %q (all %v)", k, got[k], sev, got)
		}
	}
	if _, err := e.Conformance(ctx, "nope"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}
}

func TestSemantics(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	id := openICD(t, e, simied.ICD()).ID
	ld := simied.Domain

	ln, err := e.Semantics(ctx, id, ld+"/MMXU1")
	if err != nil {
		t.Fatal(err)
	}
	if !ln.Known || ln.LNClass != "MMXU" || !strings.HasPrefix(ln.Namespace, "IEC 61850-7-4") || ln.Base == "" {
		t.Errorf("LN = %+v", ln)
	}
	present := map[string]int{}
	for _, d := range ln.DataObjects {
		present[d.Name] = d.Present
		if d.Name == "Beh" && (!d.Mandatory || d.CDC != "ENS") {
			t.Errorf("Beh = %+v", d)
		}
	}
	if present["Hz"] != 1 || present["TotW"] != 1 || present["PhV"] != 0 {
		t.Errorf("present = %v", present)
	}
	ggio, _ := e.Semantics(ctx, id, ld+"/GGIO1")
	for _, d := range ggio.DataObjects {
		if d.Name == "SPCSO" && d.Present != 4 {
			t.Errorf("SPCSO1..4 counted as %d of SPCSO", d.Present)
		}
	}

	f, err := e.Semantics(ctx, id, ld+"/MMXU1.Hz.mag.f")
	if err != nil {
		t.Fatal(err)
	}
	if f.DataObject != "Hz" || f.CDC != "MV" || f.InstanceCDC != "MV" || f.Attribute != "mag.f" || f.Type != "FLOAT32" || f.FC != "MX" ||
		f.Quantity != "frequency" || f.Unit != "Hz" || f.Presence != "O" || f.DeclaredIn != "MMXU" {
		t.Errorf("Hz.mag.f = %+v", f)
	}
	if beh, _ := e.Semantics(ctx, id, ld+"/MMXU1.Beh"); beh.DeclaredIn == "MMXU" || beh.DeclaredIn == "" {
		t.Errorf("Beh declared in %q, want a base class", beh.DeclaredIn)
	}
	if spc, _ := e.Semantics(ctx, id, ld+"/GGIO1.SPCSO2"); spc.CDC != "SPC" || spc.Presence != "Omulti" {
		t.Errorf("SPCSO2 = %+v", spc)
	}
	if l, _ := e.Semantics(ctx, id, ld); l.Kind != model.NodeLD || l.Note == "" {
		t.Errorf("LD = %+v", l)
	}

	icd := strings.Replace(string(simied.ICD()), `<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/>`,
		`<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/><LN lnClass="ZXYZ" inst="1" lnType="MMXU_T"/>`, 1)
	icd = strings.Replace(icd, `<DO name="TotW" type="MV_T"/>`, `<DO name="TotW" type="MV_T"/><DO name="Private1" type="MV_T"/>`, 1)
	odd := openICD(t, e, []byte(icd)).ID
	if s, _ := e.Semantics(ctx, odd, ld+"/ZXYZ1"); s.Known || s.Note == "" {
		t.Errorf("unknown class = %+v", s)
	}
	if s, _ := e.Semantics(ctx, odd, ld+"/MMXU1.Private1"); s.CDC != "" || !strings.Contains(s.Note, "does not define Private1") {
		t.Errorf("unknown data object = %+v", s)
	}

	for _, tc := range []struct {
		ref  string
		code string
	}{
		{ld + "/MMXU1.Hz[MX]", service.CodeInvalidArgument},
		{ld + "/NOPE1", service.CodeNotFound},
	} {
		if _, err := e.Semantics(ctx, id, tc.ref); service.CodeOf(err) != tc.code {
			t.Errorf("Semantics(%q) = %v", tc.ref, err)
		}
	}
	if _, err := e.Semantics(ctx, "nope", ld); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}
}

func TestSearchByQuantity(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	id := openICD(t, e, simied.ICD()).ID
	res, err := e.Search(ctx, id, model.SearchQuery{Quantity: "Frequency"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Ref != simied.Domain+"/MMXU1.Hz" {
		t.Errorf("frequency = %+v", res.Hits)
	}
	if res, _ := e.Search(ctx, id, model.SearchQuery{Quantity: "voltage"}); len(res.Hits) != 0 {
		t.Errorf("voltage = %+v", res.Hits)
	}
	if res, _ := e.Search(ctx, id, model.SearchQuery{Quantity: "active-power", LNClass: "MMXU"}); len(res.Hits) != 1 {
		t.Errorf("active power = %+v (quantities %v)", res.Hits, Quantities())
	}
}

func TestExportPoints(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	ld := simied.Domain
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{openICD(t, e, simied.ICD()).ID, live.ID} {
		f, err := e.Export(ctx, id, model.ExportRequest{Format: model.ExportPointsJSON})
		if err != nil {
			t.Fatal(err)
		}
		var list model.PointList
		if err := json.Unmarshal([]byte(f.Text), &list); err != nil {
			t.Fatal(err)
		}
		if list.Schema != model.PointListSchema || !strings.HasSuffix(f.Name, "-points.json") || list.Note != "" {
			t.Errorf("list = %s %s %q", list.Schema, f.Name, list.Note)
		}
		byRef := map[string]model.Point{}
		for _, p := range list.Points {
			byRef[p.Ref] = p
			if strings.Contains(p.Ref, ".RP.") || strings.Contains(p.Ref, ".BR.") || strings.Contains(p.Ref, "SGCB") {
				t.Errorf("a control block in the point list: %s", p.Ref)
			}
		}
		hz := byRef[ld+"/MMXU1.Hz.mag.f[MX]"]
		if hz.CDC != "MV" || hz.Type != "FLOAT32" || hz.Quantity != "frequency" || hz.Unit != "Hz" || hz.LNClass != "MMXU" ||
			hz.DataObject != "Hz" || hz.Attribute != "mag.f" ||
			strings.Join(hz.DataSets, " ") != ld+"/LLN0.Events "+ld+"/LLN0.Meas" || len(hz.Reports) != 3 {
			t.Errorf("Hz.mag.f = %+v", hz)
		}
		if ind := byRef[ld+"/GGIO1.Ind1.stVal[ST]"]; ind.CDC != "SPS" || ind.Type != "BOOLEAN" || len(ind.DataSets) != 1 {
			t.Errorf("Ind1.stVal = %+v", ind)
		}
	}

	c, err := e.Export(ctx, live.ID, model.ExportRequest{Format: model.ExportPointsCSV})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(c.Text), "\n")
	if !strings.HasPrefix(lines[0], "ref,ld,ln,ln_class,") || !strings.Contains(c.Text, ld+"/MMXU1.Hz.mag.f[MX],"+ld+",MMXU1,MMXU,Hz,mag.f,MX,MV,FLOAT32,O,frequency,Hz,") {
		t.Errorf("CSV:\n%s", strings.Join(lines[:min(5, len(lines))], "\n"))
	}

	// A snapshot has no data sets: the list says so.
	snapFile, err := e.Export(ctx, live.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := e.OpenSnapshot(ctx, model.File{Name: "s.json", Data: []byte(snapFile.Text)})
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.Export(ctx, snap.ID, model.ExportRequest{Format: model.ExportPointsJSON})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Text, `"note": "a snapshot holds`) {
		t.Errorf("snapshot point list has no note")
	}
	if _, err := e.Export(ctx, "nope", model.ExportRequest{Format: model.ExportPointsCSV}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}
	if _, err := e.Export(ctx, live.ID, model.ExportRequest{Format: "xml"}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("unknown format = %v", err)
	}
	if got := csvRow([]string{"=1+1", "-7", "a"}); strings.Join(got, ",") != "'=1+1,-7,a" {
		t.Errorf("csvRow = %v", got)
	}
}

// TestNamespaceUnavailable: the embedded namespaces never fail to load,
// but if they did, every semantic function would say so.
func TestNamespaceUnavailable(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	id := openICD(t, e, simied.ICD()).ID
	saved := loadEmbedded
	reset := func() { embeddedOnce, embeddedCat, embeddedErr = sync.Once{}, nil, nil }
	t.Cleanup(func() { loadEmbedded = saved; reset() })
	loadEmbedded = func() (*catalog.Catalog, error) { return nil, errors.New("broken") }
	reset()

	if _, err := e.Semantics(ctx, id, simied.Domain+"/MMXU1"); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("Semantics = %v", err)
	}
	if _, err := e.Conformance(ctx, id); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("Conformance = %v", err)
	}
	if _, err := e.Search(ctx, id, model.SearchQuery{Quantity: "frequency"}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("Search = %v", err)
	}
	if _, err := e.Export(ctx, id, model.ExportRequest{Format: model.ExportPointsCSV}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("Export = %v", err)
	}
	if _, err := e.AddNamespaces(ctx, model.File{Name: "acme.nsd", Data: []byte(vendorNSD)}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("AddNamespaces = %v", err)
	}
	if h, _ := e.Health(ctx); len(h.Quantities) == 0 {
		t.Error("Health lists no quantities")
	} else {
		for _, q := range h.Quantities {
			if h.QuantityLabels[q] == "" || h.QuantityLabels[q] == q {
				t.Errorf("quantity %s has label %q", q, h.QuantityLabels[q])
			}
		}
	}
}

func TestConformanceHelpers(t *testing.T) {
	p, _, err := newEngine(t).ns.profileFor("")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ class, name, want string }{
		{"GGIO", "Ind12", "Ind"},
		{"MMXU", "Hz", "Hz"},
		{"MMXU", "Beh2", ""}, // Beh is not a multi-instance data object
		{"MMXU", "Nope", ""},
		{"MMXU", "7", ""},
	} {
		if got, ok := nsdName(p, tc.class, tc.name); got != tc.want || ok != (tc.want != "") {
			t.Errorf("nsdName(%s, %s) = %q %v", tc.class, tc.name, got, ok)
		}
	}
	var c model.Conformance
	for range model.MaxFindings + 2 {
		addFinding(&c, model.Finding{})
	}
	if len(c.Findings) != model.MaxFindings || !c.Truncated {
		t.Errorf("%d findings, truncated %v", len(c.Findings), c.Truncated)
	}
}
