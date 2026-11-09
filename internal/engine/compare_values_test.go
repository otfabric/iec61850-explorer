// SPDX-License-Identifier: MIT

package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// TestCompareValuesSCLWithIED: the simulated IED serves demo.icd's
// configured values; changed values in the file show up with both sides,
// enumerations by literal and ordinal (M3b).
func TestCompareValuesSCLWithIED(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	same := openICD(t, e, simied.ICD())
	c, err := e.Compare(ctx, model.CompareRequest{A: same.ID, B: live.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	cat := category(t, c, model.CompareValues)
	if !cat.Compared || cat.Items == 0 || cat.Changed != 0 || len(c.Differences) != 0 {
		t.Fatalf("values = %+v, differences %+v", cat, c.Differences)
	}
	// Without the flag there is no values category.
	if c2, _ := e.Compare(ctx, model.CompareRequest{A: same.ID, B: live.ID}); len(c2.Categories) != 3 {
		t.Errorf("categories without values = %+v", c2.Categories)
	}

	icd := string(simied.ICD())
	for _, r := range [][2]string{
		{`<DOI name="SPCSO2"><DAI name="ctlModel"><Val>sbo-with-normal-security</Val>`, `<DOI name="SPCSO2"><DAI name="ctlModel"><Val>direct-with-normal-security</Val>`},
		{`<DA name="vendor" fc="DC" bType="VisString255"><Val>OT Fabric</Val>`, `<DA name="vendor" fc="DC" bType="VisString255"><Val>Someone else</Val>`},
	} {
		if !strings.Contains(icd, r[0]) {
			t.Fatalf("demo.icd has no %q", r[0])
		}
		icd = strings.Replace(icd, r[0], r[1], 1)
	}
	changed := openICD(t, e, []byte(icd))
	c, err = e.Compare(ctx, model.CompareRequest{A: changed.ID, B: live.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.Difference{}
	for _, d := range c.Differences {
		got[d.Ref] = d
	}
	ld := simied.Domain
	ctl := got[ld+"/GGIO1.SPCSO2.ctlModel[CF]"]
	if ctl.A != "direct-with-normal-security (1)" || ctl.B != "sbo-with-normal-security (2)" || ctl.Field != "value" {
		t.Errorf("ctlModel difference = %+v (all %+v)", ctl, c.Differences)
	}
	var vendor model.Difference
	for ref, d := range got {
		if strings.HasSuffix(ref, ".vendor[DC]") {
			vendor = d
		}
	}
	if vendor.A != "Someone else" || vendor.B != "OT Fabric" {
		t.Errorf("vendor difference = %+v", vendor)
	}
	if cat := category(t, c, model.CompareValues); cat.Changed != len(c.Differences) || cat.Changed < 2 {
		t.Errorf("values = %+v, %d differences", cat, len(c.Differences))
	}
}

const sgSCL = `<?xml version="1.0" encoding="UTF-8"?>
<SCL xmlns="http://www.iec.ch/61850/2003/SCL" version="2007" revision="B" release="4">
  <Header id="sg"/>
  <IED name="P">
    <AccessPoint name="AP1"><Server><Authentication/>
      <LDevice inst="Prot">
        <LN0 lnClass="LLN0" inst="" lnType="LLN0_T"><SettingControl numOfSGs="2" actSG="2"/></LN0>
        <LN lnClass="PTOC" inst="1" lnType="PTOC_T">
          <DOI name="StrVal"><SDI name="setMag"><DAI name="f"><Val sGroup="1">%s</Val><Val sGroup="2">%s</Val></DAI></SDI></DOI>
          <DOI name="OpDlTmms"><DAI name="setVal"><Val sGroup="1">100</Val><Val sGroup="2">200</Val></DAI></DOI>
          <DOI name="Mod"><DAI name="ctlModel"><Val>%s</Val></DAI></DOI>
        </LN>
      </LDevice>
    </Server></AccessPoint>
  </IED>
  <DataTypeTemplates>
    <LNodeType id="LLN0_T" lnClass="LLN0"/>
    <LNodeType id="PTOC_T" lnClass="PTOC">
      <DO name="StrVal" type="ASG_T"/><DO name="OpDlTmms" type="ING_T"/><DO name="Mod" type="ENC_T"/>
    </LNodeType>
    <DOType id="ASG_T" cdc="ASG"><DA name="setMag" bType="Struct" type="AV_T" fc="SG"/><DA name="q" bType="Quality" fc="SG"><Val>0</Val></DA></DOType>
    <DOType id="ING_T" cdc="ING"><DA name="setVal" bType="INT32" fc="SG"/></DOType>
    <DOType id="ENC_T" cdc="ENC"><DA name="ctlModel" bType="Enum" type="CtlModel_E" fc="CF"/><DA name="stVal" bType="Enum" type="CtlModel_E" fc="ST"><Val>1</Val></DA></DOType>
    <DAType id="AV_T"><BDA name="f" bType="FLOAT32"/></DAType>
    <EnumType id="CtlModel_E"><EnumVal ord="0">status-only</EnumVal><EnumVal ord="1">direct-with-normal-security</EnumVal></EnumType>
  </DataTypeTemplates>
</SCL>`

func TestCompareValuesSettingGroups(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	open := func(g1, g2, ctl string) model.SessionInfo {
		t.Helper()
		info, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "p.cid", Data: []byte(fmt.Sprintf(sgSCL, g1, g2, ctl))}})
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	a := open("1.5", "2.5", "direct-with-normal-security")
	b := open("1.5", "2.75", "1")
	c, err := e.Compare(ctx, model.CompareRequest{A: a.ID, B: b.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Differences) != 1 {
		t.Fatalf("differences = %+v", c.Differences)
	}
	d := c.Differences[0]
	if d.Ref != "PProt/PTOC1.StrVal.setMag.f[SG]" || d.A != "2.5" || d.B != "2.75" || d.Note != "setting group 2" {
		t.Errorf("difference = %+v", d)
	}
	// Groups 1 and 2 of setMag.f and setVal, and ctlModel: the enumeration
	// literal and its ordinal are the same value. The quality is not
	// compared, nor is the status value (FC ST).
	cat := category(t, c, model.CompareValues)
	if cat.Items != 5 || cat.Same != 4 || cat.Changed != 1 || !strings.Contains(cat.Reason, "1 attributes have a type") {
		t.Errorf("values = %+v", cat)
	}
}
