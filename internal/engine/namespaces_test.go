// SPDX-License-Identifier: MIT

package engine

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// vendorNSD is a vendor extension namespace in the shape ABB, Siemens and
// Schneider publish theirs: it depends on IEC 61850-7-4 and adds a logical
// node class and an extension data object. Independently authored, not
// vendor content.
const vendorNSD = `<?xml version="1.0" encoding="UTF-8"?>
<NS xmlns="http://www.iec.ch/61850/2016/NSD" id="OTF ACME" version="2026" revision="A" release="1" publicationStage="IS">
  <DependsOn id="IEC 61850-7-4" version="2007" revision="B"/>
  <LNClasses>
    <LNClass name="ZACM" base="DomainLN" titleID="acme.ZACM.title" descID="acme.ZACM.desc">
      <DataObject name="Load" type="MV" presCond="M" descID="acme.ZACM.Load.desc"/>
      <DataObject name="Note" type="SPS" presCond="O"/>
    </LNClass>
    <LNClass name="MMXU" isExtension="true">
      <DataObject name="HiAlm" type="SPS" presCond="O"/>
    </LNClass>
  </LNClasses>
</NS>`

const vendorNSDoc = `<?xml version="1.0" encoding="UTF-8"?>
<NSDoc xmlns="http://www.iec.ch/61850/2016/NSD" id="OTF ACME" version="2026" revision="A" release="1" lang="en">
  <Doc id="acme.ZACM.title">Acme feeder supervision</Doc>
  <Doc id="acme.ZACM.desc">Supervises a feeder.</Doc>
  <Doc id="acme.ZACM.Load.desc">Feeder load.</Doc>
</NSDoc>`

func vendorZIP(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, text := range map[string]string{"acme.nsd": vendorNSD, "acme.en.nsdoc": vendorNSDoc} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// icdDeclaring is demo.icd declaring ldNs and with a vendor logical node.
func icdDeclaring(t *testing.T, ldNs string) []byte {
	t.Helper()
	icd := string(simied.ICD())
	for _, r := range [][2]string{
		{`<Val>IEC 61850-7-4:2007B</Val>`, `<Val>` + ldNs + `</Val>`},
		{`<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/>`, `<LN lnClass="LPHD" inst="1" lnType="LPHD_T"/><LN lnClass="ZACM" inst="1" lnType="ZACM_T"/>`},
		{`<LNodeType id="MMXU_T" lnClass="MMXU">`, `<LNodeType id="ZACM_T" lnClass="ZACM"><DO name="Beh" type="ENS_Beh"/><DO name="Load" type="MV_T"/></LNodeType><LNodeType id="MMXU_T" lnClass="MMXU">`},
	} {
		if !strings.Contains(icd, r[0]) {
			t.Fatalf("demo.icd has no %q", r[0])
		}
		icd = strings.Replace(icd, r[0], r[1], 1)
	}
	return []byte(icd)
}

func declaredFor(c model.Conformance, ref string) model.DeclaredNamespace {
	for _, d := range c.Declared {
		if d.Ref == ref {
			return d
		}
	}
	return model.DeclaredNamespace{}
}

// TestEditionDetection: the namespace a logical device declares decides
// what it is checked against; Edition 1 makes missing Edition 2 data
// objects warnings.
func TestEditionDetection(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	ld := simied.Domain

	// The IED and its file declare IEC 61850-7-4:2007B (read from the IED).
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{live.ID, openICD(t, e, simied.ICD()).ID} {
		c, err := e.Conformance(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		d := declaredFor(c, ld)
		if d.Declared != "IEC 61850-7-4:2007B" || d.Source != "ldNs" || d.Edition1 || d.Unknown || !strings.Contains(strings.Join(d.CheckedAgainst, " "), "IEC 61850-7-4:2007B") {
			t.Errorf("declared = %+v", d)
		}
		if got := findingKeys(c)["missing-mandatory-do "+ld+"/XCBR1.Loc"]; got != model.SeverityError {
			t.Errorf("XCBR1.Loc = %q", got)
		}
	}

	// Edition 1, declared: missing data objects are warnings, CDC
	// differences information.
	ed1Decl := strings.Replace(string(icdDeclaring(t, "IEC 61850-7-4:2003")), `<DOType id="MV_T" cdc="MV">`, `<DOType id="MV_T" cdc="CMV">`, 1)
	c, err := e.Conformance(ctx, openICD(t, e, []byte(ed1Decl)).ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := declaredFor(c, ld); !d.Edition1 || d.Note == "" {
		t.Errorf("edition 1 = %+v", d)
	}
	for _, f := range c.Findings {
		if f.Code == model.FindingMissingMandatory && (f.Severity != model.SeverityWarning || !strings.Contains(f.Message, "since Edition 2")) {
			t.Errorf("edition 1 finding = %+v", f)
		}
	}
	if got := findingKeys(c)["cdc-mismatch "+ld+"/MMXU1.Hz"]; got != model.SeverityInfo {
		t.Errorf("edition 1 CDC difference = %q", got)
	}

	// Edition 1 by the SCL schema version (1.7, no declaration).
	ed1 := strings.Replace(string(simied.ICD()), `<DA name="ldNs" fc="EX" bType="VisString255"><Val>IEC 61850-7-4:2007B</Val></DA>`, ``, 1)
	ed1 = strings.Replace(ed1, ` version="2007" revision="B"`, ``, 1)
	ed1Info := openICD(t, e, []byte(ed1))
	c, err = e.Conformance(ctx, ed1Info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := declaredFor(c, ld); !d.Edition1 || d.Source != "scl-schema" {
		t.Errorf("schema 1.7 = %+v", d)
	}
	if sem, _ := e.Semantics(ctx, ed1Info.ID, ld+"/XCBR1"); !sem.Edition1 || sem.Declared != "IEC 61850-7-4:2003" {
		t.Errorf("semantics of an edition 1 node = %+v", sem)
	}

	// A namespace neither embedded nor loaded: its unknown data objects are
	// not listed one by one.
	unkICD := strings.Replace(string(icdDeclaring(t, "OTF ACME:2026A")), `<DO name="TotW" type="MV_T"/>`, `<DO name="TotW" type="MV_T"/><DO name="Private1" type="MV_T"/>`, 1)
	unk := openICD(t, e, []byte(unkICD))
	c, err = e.Conformance(ctx, unk.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := findingKeys(c)
	if got["namespace-not-loaded "+ld+"/GGIO1"] == "" && got["namespace-not-loaded "+ld+"/LLN0"] == "" {
		t.Errorf("no namespace-not-loaded finding: %v", got)
	}
	if got["unknown-ln-class "+ld+"/ZACM1"] != model.SeverityWarning {
		t.Errorf("ZACM1 = %v", got)
	}
	if d := declaredFor(c, ld); !d.Unknown {
		t.Errorf("unknown declaration = %+v", d)
	}
	if got["unknown-do "+ld+"/MMXU1.Private1"] != "" {
		t.Error("an unknown data object listed under a namespace that is not loaded")
	}
}

func TestLoadVendorNamespace(t *testing.T) {
	dir := t.TempDir()
	e := New(Options{DataDir: dir})
	t.Cleanup(e.Close)
	ctx := t.Context()
	ld := simied.Domain

	pkg, err := e.AddNamespaces(ctx, model.File{Name: "acme.zip", Data: vendorZIP(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Namespaces) != 1 || pkg.Namespaces[0] != "OTF ACME:2026A1" || !pkg.Docs || pkg.ID == "" || pkg.File != "acme.zip" {
		t.Errorf("package = %+v", pkg)
	}
	list, err := e.Namespaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Loaded) != 1 || len(list.Embedded) < 20 || len(list.Profiles) == 0 {
		t.Errorf("namespaces = %d loaded, %d embedded, %d profiles", len(list.Loaded), len(list.Embedded), len(list.Profiles))
	}

	id := openICD(t, e, icdDeclaring(t, "OTF ACME:2026A")).ID
	c, err := e.Conformance(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	got := findingKeys(c)
	for k := range got {
		if strings.Contains(k, "ZACM1") || strings.HasPrefix(k, "namespace-not-loaded") {
			t.Errorf("finding %q with the vendor namespace loaded", k)
		}
	}
	if d := declaredFor(c, ld); d.Unknown || !strings.Contains(strings.Join(d.CheckedAgainst, " "), "OTF ACME:2026A1") {
		t.Errorf("declared = %+v", d)
	}
	// Standard classes still resolve through the vendor namespace's
	// dependency on 7-4.
	if got["missing-mandatory-do "+ld+"/XCBR1.Loc"] != model.SeverityError {
		t.Errorf("XCBR1 with the vendor profile: %v", got)
	}
	sem, err := e.Semantics(ctx, id, ld+"/ZACM1")
	if err != nil {
		t.Fatal(err)
	}
	if !sem.Known || sem.Namespace != "OTF ACME:2026A1" || sem.Title != "Acme feeder supervision" || sem.Description != "Supervises a feeder." {
		t.Errorf("ZACM1 = %+v", sem)
	}
	if do, _ := e.Semantics(ctx, id, ld+"/ZACM1.Load"); do.CDC != "MV" || do.Description != "Feeder load." {
		t.Errorf("ZACM1.Load = %+v", do)
	}

	// A vendor extension data object in a standard logical node without
	// dataNs is found in the vendor namespace the device declares elsewhere.
	ext := strings.Replace(string(icdDeclaring(t, "IEC 61850-7-4:2007B")), `<DO name="TotW" type="MV_T"/>`, `<DO name="TotW" type="MV_T"/><DO name="HiAlm" type="ENS_Beh"/><DO name="Other" type="ENS_Beh"/>`, 1)
	ext = strings.Replace(ext, `<LNodeType id="ZACM_T" lnClass="ZACM"><DO name="Beh" type="ENS_Beh"/>`, `<LNodeType id="ZACM_T" lnClass="ZACM"><DO name="NamPlt" type="LPL_LN"/><DO name="Beh" type="ENS_Beh"/>`, 1)
	ext = strings.Replace(ext, `<DOType id="LPL_T" cdc="LPL">`, `<DOType id="LPL_LN" cdc="LPL"><DA name="lnNs" fc="EX" bType="VisString255"><Val>OTF ACME:2026A</Val></DA></DOType><DOType id="LPL_T" cdc="LPL">`, 1)
	c, err = e.Conformance(ctx, openICD(t, e, []byte(ext)).ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := findingKeys(c); got["unknown-do "+ld+"/MMXU1.HiAlm"] != "" || got["unknown-do "+ld+"/MMXU1.Other"] != model.SeverityInfo {
		t.Errorf("extension data object = %v", got)
	}

	// Kept in the data directory: a new engine has it.
	e2 := New(Options{DataDir: dir})
	t.Cleanup(e2.Close)
	if l, _ := e2.Namespaces(ctx); len(l.Loaded) != 1 || l.Loaded[0].ID != pkg.ID {
		t.Errorf("after restart = %+v", l.Loaded)
	}
	// Loading it again conflicts; removing it removes the file.
	if _, err := e.AddNamespaces(ctx, model.File{Name: "again.zip", Data: vendorZIP(t)}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("loaded twice = %v", err)
	}
	if err := e.RemoveNamespaces(ctx, pkg.ID); err != nil {
		t.Fatal(err)
	}
	if files, _ := os.ReadDir(filepath.Join(dir, "namespaces")); len(files) != 0 {
		t.Errorf("files left: %v", files)
	}
	if err := e.RemoveNamespaces(ctx, pkg.ID); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("removed twice = %v", err)
	}
	if c, _ := e.Conformance(ctx, id); findingKeys(c)["unknown-ln-class "+ld+"/ZACM1"] == "" {
		t.Error("ZACM1 still known after the package was removed")
	}
}

func TestAddNamespacesRefused(t *testing.T) {
	e := newEngine(t) // no data directory: kept in memory
	ctx := t.Context()
	nsdoc := `<?xml version="1.0"?><NSDoc xmlns="http://www.iec.ch/61850/2016/NSD" id="X" version="2026" revision="A" lang="en"><Doc id="a">b</Doc></NSDoc>`
	official := strings.Replace(vendorNSD, `id="OTF ACME" version="2026" revision="A" release="1"`, `id="IEC 61850-7-4" version="2007" revision="B" release="6"`, 1)
	for _, tc := range []struct {
		name string
		data []byte
		code string
	}{
		{"empty.nsd", nil, service.CodeInvalidArgument},
		{"big.zip", make([]byte, model.MaxNamespaceBytes+1), service.CodeLimitExceeded},
		{"garbage.nsd", []byte("<nope/>"), service.CodeInvalidArgument},
		{"bad.zip", []byte("PK\x03\x04broken"), service.CodeInvalidArgument},
		{"docs.nsdoc", []byte(nsdoc), service.CodeInvalidArgument},
		{"7-4.nsd", []byte(official), service.CodeInvalidArgument}, // embedded already
	} {
		if _, err := e.AddNamespaces(ctx, model.File{Name: tc.name, Data: tc.data}); service.CodeOf(err) != tc.code {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.code)
		}
	}
	// One NSD document, in memory.
	pkg, err := e.AddNamespaces(ctx, model.File{Name: "acme.nsd", Data: []byte(vendorNSD)})
	if err != nil || pkg.Docs {
		t.Fatalf("single NSD = %+v, %v", pkg, err)
	}
	if err := e.RemoveNamespaces(ctx, pkg.ID); err != nil {
		t.Fatal(err)
	}
	for i := range model.MaxNamespacePackages {
		data := strings.Replace(vendorNSD, `id="OTF ACME"`, `id="OTF ACME`+string(rune('A'+i%26))+string(rune('A'+i/26))+`"`, 1)
		if _, err := e.AddNamespaces(ctx, model.File{Name: "x.nsd", Data: []byte(data)}); err != nil {
			t.Fatalf("package %d: %v", i, err)
		}
	}
	if _, err := e.AddNamespaces(ctx, model.File{Name: "acme.nsd", Data: []byte(vendorNSD)}); service.CodeOf(err) != service.CodeLimitExceeded {
		t.Errorf("one package too many = %v", err)
	}
}

func TestNamespaceRegistryHelpers(t *testing.T) {
	if normStandard("IEC TR 61850-90-9") != normStandard("(Tr)IEC 61850-90-9") {
		t.Error("TR markers")
	}
	if fileNameOf("abc--acme.zip") != "acme.zip" || fileNameOf("plain") != "plain" {
		t.Error("fileNameOf")
	}
	// A data directory with a broken and a hidden file: both skipped.
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "namespaces")
	if err := os.MkdirAll(filepath.Join(nsDir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"x--broken.nsd": "<nope/>", ".hidden": "x", "y--acme.nsd": vendorNSD} {
		if err := os.WriteFile(filepath.Join(nsDir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var warnings []string
	r := newNSRegistry(dir, func(f string, a ...any) { warnings = append(warnings, f) })
	if len(r.loaded) != 1 || len(warnings) != 1 {
		t.Errorf("loaded %d, warnings %v", len(r.loaded), warnings)
	}
	// A loaded release whose dependency is missing: checked against the
	// default profile, with a note.
	orphan := strings.Replace(vendorNSD, `<DependsOn id="IEC 61850-7-4" version="2007" revision="B"/>`, `<DependsOn id="OTF NOPE" version="2020" revision="A"/>`, 1)
	e := newEngine(t)
	if _, err := e.AddNamespaces(t.Context(), model.File{Name: "o.nsd", Data: []byte(orphan)}); err != nil {
		t.Skipf("the catalog refuses a missing dependency at load: %v", err)
	}
	_, dn, err := e.ns.profileFor("OTF ACME:2026A")
	if err != nil || !strings.Contains(dn.Note, "default profile") {
		t.Errorf("orphan = %+v, %v", dn, err)
	}
	if _, dn, _ := e.ns.profileFor("IEC 61850-7-4:xyz"); !dn.Unknown {
		t.Errorf("unparsable declaration = %+v", dn)
	}
}

// TestVendorDeclarations: a vendor logical node declares its namespace in
// its own NamPlt.lnNs; an extension data object in dataNs.
func TestVendorDeclarations(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	ld := simied.Domain
	if _, err := e.AddNamespaces(ctx, model.File{Name: "acme.zip", Data: vendorZIP(t)}); err != nil {
		t.Fatal(err)
	}
	icd := string(icdDeclaring(t, "IEC 61850-7-4:2007B"))
	for _, r := range [][2]string{
		{`<LNodeType id="ZACM_T" lnClass="ZACM"><DO name="Beh" type="ENS_Beh"/>`, `<LNodeType id="ZACM_T" lnClass="ZACM"><DO name="NamPlt" type="LPL_LN"/><DO name="Beh" type="ENS_Beh"/>`},
		{`<DOType id="LPL_T" cdc="LPL">`, `<DOType id="LPL_LN" cdc="LPL"><DA name="lnNs" fc="EX" bType="VisString255"><Val>OTF ACME:2026A</Val></DA></DOType><DOType id="EXT_T" cdc="SPS"><DA name="stVal" fc="ST" bType="BOOLEAN"/><DA name="dataNs" fc="EX" bType="VisString255"><Val>OTF NOPE:2026A</Val></DA></DOType><DOType id="LPL_T" cdc="LPL">`},
		{`<DO name="TotW" type="MV_T"/>`, `<DO name="TotW" type="MV_T"/><DO name="Vnd1" type="EXT_T"/>`},
	} {
		if !strings.Contains(icd, r[0]) {
			t.Fatalf("no %q", r[0])
		}
		icd = strings.Replace(icd, r[0], r[1], 1)
	}
	id := openICD(t, e, []byte(icd)).ID
	c, err := e.Conformance(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d := declaredFor(c, ld+"/ZACM1"); d.Source != "lnNs" || d.Declared != "OTF ACME:2026A" {
		t.Errorf("ZACM1 declared = %+v (all %+v)", d, c.Declared)
	}
	got := findingKeys(c)
	if got["unknown-ln-class "+ld+"/ZACM1"] != "" {
		t.Errorf("ZACM1 unknown with its lnNs loaded: %v", got)
	}
	if got["namespace-not-loaded "+ld+"/MMXU1.Vnd1"] != model.SeverityWarning || got["unknown-do "+ld+"/MMXU1.Vnd1"] != model.SeverityInfo {
		t.Errorf("Vnd1 = %v", got)
	}
	if sem, _ := e.Semantics(ctx, id, ld+"/MMXU1.Vnd1"); !strings.Contains(sem.Note, "does not define Vnd1") {
		t.Errorf("Vnd1 semantics = %+v", sem)
	}
	if sem, _ := e.Semantics(ctx, id, ld+"/ZACM1"); sem.Declared != "OTF ACME:2026A" || !sem.Known {
		t.Errorf("ZACM1 semantics = %+v", sem)
	}
	// A class the declared, unknown namespace does not define: both notes.
	unk := openICD(t, e, icdDeclaring(t, "OTF NOPE:2026A")).ID
	if sem, _ := e.Semantics(ctx, unk, ld+"/ZACM1"); !strings.Contains(sem.Note, "check against them; the namespace") {
		t.Errorf("unknown class in an unknown namespace = %+v", sem)
	}
}

func TestDeclarationsLiveAndLost(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	if res, err := e.Search(ctx, live.ID, model.SearchQuery{Quantity: "frequency"}); err != nil || len(res.Hits) != 1 {
		t.Errorf("live search by quantity = %+v, %v", res, err)
	}
	ied.Close()
	s, _ := e.session(live.ID)
	for range 100 {
		if s.liveErr() != nil {
			break
		}
		_, _ = e.Read(ctx, live.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}})
	}
	if d := e.declarationsOf(ctx, s, compareNodes(s)); len(d.attrs) != 0 {
		t.Errorf("declarations of a lost session = %v", d.attrs)
	}
}

func TestNamespaceFileErrors(t *testing.T) {
	ctx := t.Context()
	// The namespaces path is a file: listed with a warning.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "namespaces"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var warned []string
	newNSRegistry(dir, func(f string, a ...any) { warned = append(warned, f) })
	if len(warned) != 1 {
		t.Errorf("warnings = %v", warned)
	}
	e := New(Options{DataDir: dir})
	t.Cleanup(e.Close)
	if _, err := e.AddNamespaces(ctx, model.File{Name: "acme.nsd", Data: []byte(vendorNSD)}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("add with a file in the way = %v", err)
	}

	// A dangling link, a file too large, two packages of one namespace.
	dir = t.TempDir()
	nsDir := filepath.Join(dir, "namespaces")
	if err := os.MkdirAll(nsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(nsDir, "l--link.nsd")); err != nil {
		t.Fatal(err)
	}
	big, err := os.Create(filepath.Join(nsDir, "b--big.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(model.MaxNamespaceBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = big.Close()
	for _, n := range []string{"a--one.nsd", "c--two.nsd"} {
		if err := os.WriteFile(filepath.Join(nsDir, n), []byte(vendorNSD), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	warned = nil
	r := newNSRegistry(dir, func(f string, a ...any) { warned = append(warned, f) })
	if len(warned) != 2 || len(r.loaded) != 2 {
		t.Errorf("warnings %v, loaded %d", warned, len(r.loaded))
	}
	if _, _, err := r.profileFor(""); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("a catalog with a namespace twice = %v", err)
	}

	// A read-only namespaces directory: adding and removing fail.
	dir = t.TempDir()
	e = New(Options{DataDir: dir})
	t.Cleanup(e.Close)
	pkg, err := e.AddNamespaces(ctx, model.File{Name: "acme.nsd", Data: []byte(vendorNSD)})
	if err != nil {
		t.Fatal(err)
	}
	ro := filepath.Join(dir, "namespaces")
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	other := strings.Replace(vendorNSD, `id="OTF ACME"`, `id="OTF OTHER"`, 1)
	if _, err := e.AddNamespaces(ctx, model.File{Name: "other.nsd", Data: []byte(other)}); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("add to a read-only directory = %v", err)
	}
	if err := e.RemoveNamespaces(ctx, pkg.ID); service.CodeOf(err) != service.CodeInternal {
		t.Errorf("remove from a read-only directory = %v", err)
	}
}
