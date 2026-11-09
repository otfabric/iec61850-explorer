// SPDX-License-Identifier: MIT

package engine

import (
	"slices"
	"strings"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func TestCanonValues(t *testing.T) {
	enum := sclValue{bType: "Enum", enum: map[string]int{"on": 1}}
	for _, tc := range []struct {
		v    sclValue
		text string
		want canon
		ok   bool
	}{
		{sclValue{bType: "BOOLEAN"}, " TRUE ", canon{kind: "bool", b: true}, true},
		{sclValue{bType: "BOOLEAN"}, "0", canon{kind: "bool"}, true},
		{sclValue{bType: "BOOLEAN"}, "maybe", canon{}, false},
		{enum, "on", canon{kind: "num", f: 1}, true},
		{enum, "3", canon{kind: "num", f: 3}, true},
		{enum, "off", canon{}, false},
		{sclValue{bType: "INT32"}, "-5", canon{kind: "num", f: -5}, true},
		{sclValue{bType: "INT64U"}, "18446744073709551615", canon{kind: "num", f: 18446744073709551615}, true},
		{sclValue{bType: "INT8"}, "x", canon{}, false},
		{sclValue{bType: "FLOAT32"}, "1.5", canon{kind: "num", f: 1.5}, true},
		{sclValue{bType: "FLOAT64"}, "x", canon{}, false},
		{sclValue{bType: "Unicode255"}, "ü", canon{kind: "str", s: "ü"}, true},
		{sclValue{bType: "Quality"}, "0", canon{}, false},
	} {
		got, _, ok := sclCanon(tc.v, tc.text)
		if ok != tc.ok || got != tc.want {
			t.Errorf("sclCanon(%s, %q) = %+v %v, want %+v %v", tc.v.bType, tc.text, got, ok, tc.want, tc.ok)
		}
	}
	for _, tc := range []struct {
		v    model.Value
		want canon
		ok   bool
	}{
		{model.Value{Type: model.TypeBoolean, JSON: true}, canon{kind: "bool", b: true}, true},
		{model.Value{Type: model.TypeBoolean}, canon{}, false},
		{model.Value{Type: model.TypeInteger, JSON: int64(-3)}, canon{kind: "num", f: -3}, true},
		{model.Value{Type: model.TypeUnsigned, JSON: uint64(7)}, canon{kind: "num", f: 7}, true},
		{model.Value{Type: model.TypeFloat, JSON: 2.5}, canon{kind: "num", f: 2.5}, true},
		{model.Value{Type: model.TypeUnsigned, JSON: "18446744073709551615"}, canon{kind: "num", f: 18446744073709551615}, true},
		{model.Value{Type: model.TypeInteger, JSON: "x"}, canon{}, false},
		{model.Value{Type: model.TypeFloat}, canon{}, false},
		{model.Value{Type: model.TypeMMSString, Text: "a"}, canon{kind: "str", s: "a"}, true},
		{model.Value{Type: model.TypeUTCTime}, canon{}, false},
	} {
		got, ok := liveCanon(tc.v)
		if ok != tc.ok || got != tc.want {
			t.Errorf("liveCanon(%+v) = %+v %v", tc.v, got, ok)
		}
	}
	f32 := float64(float32(0.1))
	for _, tc := range []struct {
		a, b canon
		eq   bool
	}{
		{canon{kind: "num", f: 0.1}, canon{kind: "num", f: f32}, true},
		{canon{kind: "num", f: 1e6}, canon{kind: "num", f: 1e6 + 0.5}, true},
		{canon{kind: "num", f: 1}, canon{kind: "num", f: 1.01}, false},
		{canon{kind: "bool", b: true}, canon{kind: "bool"}, false},
		{canon{kind: "str", s: "a"}, canon{kind: "str", s: "a"}, true},
		{canon{kind: "num", f: 1}, canon{kind: "bool", b: true}, false},
	} {
		if got := tc.a.equal(tc.b); got != tc.eq {
			t.Errorf("%+v == %+v: %v", tc.a, tc.b, got)
		}
	}
}

func TestSettingGroupChoice(t *testing.T) {
	sv := sclValue{vals: []scl.Value{{Value: "1", SGroup: 1}, {Value: "2", SGroup: 2}}}
	if v, ok := sv.forGroup(2); !ok || v != "2" {
		t.Errorf("forGroup(2) = %q %v", v, ok)
	}
	if _, ok := sv.forGroup(3); ok {
		t.Error("forGroup(3) found a value")
	}
	if v, ok := (sclValue{vals: []scl.Value{{Value: "x"}}}).forGroup(4); !ok || v != "x" {
		t.Errorf("a value for every group = %q %v", v, ok)
	}
	sclA := valueSide{scl: map[string]sclValue{"k": sv}}
	sclB := valueSide{scl: map[string]sclValue{"k": {vals: []scl.Value{{SGroup: 3}, {SGroup: 300}}}}}
	if g := settingGroups(sclA, sclB, "k", "LD"); !slices.Equal(g, []uint8{1, 2, 3}) {
		t.Errorf("groups of two SCL files = %v", g)
	}
	live := valueSide{read: map[string]model.Value{}, actSG: map[string]uint8{"LD": 2}}
	other := valueSide{read: map[string]model.Value{}, actSG: map[string]uint8{"LD": 1}}
	if g := settingGroups(sclA, live, "k", "LD"); !slices.Equal(g, []uint8{2}) {
		t.Errorf("groups against an IED = %v", g)
	}
	if g := settingGroups(sclA, valueSide{read: map[string]model.Value{}}, "k", "LD"); !slices.Equal(g, []uint8{1}) {
		t.Errorf("groups without an active group = %v", g)
	}
	for _, tc := range []struct {
		a, b valueSide
		want string
	}{
		{sclA, sclB, "setting group 2"},
		{live, other, "active setting group 2 / 1"},
		{sclA, live, "setting group 2, active on the IED"},
	} {
		if got := sgNote(tc.a, tc.b, "LD", 2); got != tc.want {
			t.Errorf("sgNote = %q, want %q", got, tc.want)
		}
	}
	read := valueSide{read: map[string]model.Value{"b": {}, "a": {}}}
	if !read.has("a") || read.has("z") || len(read.keys()) != 2 {
		t.Error("read side keys")
	}
	if _, _, ok := sclA.canonFor("k", 9, read); ok {
		t.Error("a setting group the file has no value for was compared")
	}
	enumSide := valueSide{scl: map[string]sclValue{"e": {bType: "Enum", enum: map[string]int{"on": 1, "off": 2}}}}
	readEnum := valueSide{read: map[string]model.Value{"e": {Type: model.TypeInteger, Text: "2", JSON: int64(2)}, "t": {Type: model.TypeUTCTime}}}
	if _, text, ok := readEnum.canonFor("e", 0, enumSide); !ok || text != "off (2)" {
		t.Errorf("enumeration read = %q %v", text, ok)
	}
	if _, _, ok := readEnum.canonFor("t", 0, enumSide); ok {
		t.Error("a time stamp was compared")
	}
}

const typesSCL = `<?xml version="1.0" encoding="UTF-8"?>
<SCL xmlns="http://www.iec.ch/61850/2003/SCL" version="2007" revision="B" release="4">
  <Header id="t"/>
  <IED name="I"><AccessPoint name="A"><Server><Authentication/>
    <LDevice inst="L">
      <LN lnClass="GGIO" inst="1" lnType="G"><DOI name="W"><SDI name="sub"><DAI name="v"><Val>7</Val></DAI></SDI></DOI></LN>
      <LN lnClass="GGIO" inst="2" lnType="MISSING"/>
    </LDevice>
  </Server></AccessPoint></IED>
  <DataTypeTemplates>
    <LNodeType id="G" lnClass="GGIO"><DO name="W" type="WT"/><DO name="X" type="NOPE"/><DO name="R" type="REC"/></LNodeType>
    <DOType id="WT" cdc="WYE">
      <SDO name="sub" type="SUB"/>
      <DA name="arr" bType="INT32" fc="CF" count="3"><Val>1</Val></DA>
      <DA name="s" bType="Struct" type="NOTYPE" fc="CF"/>
      <DA name="st" bType="Struct" type="ST" fc="CF"/>
      <DA name="none" bType="INT32" fc="CF"/>
    </DOType>
    <DOType id="SUB" cdc="X"><DA name="v" bType="INT32" fc="SP"/></DOType>
    <DOType id="REC" cdc="X"><SDO name="again" type="REC"/></DOType>
    <DAType id="ST"><BDA name="a" bType="INT32" count="2"/><BDA name="b" bType="INT32"><Val>5</Val></BDA></DAType>
  </DataTypeTemplates>
</SCL>`

func TestSCLValuesTypes(t *testing.T) {
	res, err := scl.ParseBytes([]byte(typesSCL), scl.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ied := &res.Document.IEDs[0]
	vals, act, cdcs := sclValues(res.Document, ied, &ied.AccessPoints[0])
	if cdcs["IL/GGIO1.W"] != "WYE" || cdcs["IL/GGIO1.X"] != "" {
		t.Errorf("CDCs = %v", cdcs)
	}
	var keys []string
	for k := range vals {
		keys = append(keys, k)
	}
	// The SDO's value from the SDI, the BDA default; no arrays, no
	// unknown types, a recursive type stops; no setting control.
	if len(vals) != 2 || vals["IL/GGIO1.W.sub.v[SP]"].vals[0].Value != "7" || vals["IL/GGIO1.W.st.b[CF]"].vals[0].Value != "5" || len(act) != 0 {
		t.Errorf("values = %v, act %v", strings.Join(keys, " "), act)
	}
}

func TestFlattenValueSkipsArrays(t *testing.T) {
	out := map[string]model.Value{}
	flattenValue(out, "L/N.D", "CF", model.Value{Type: model.TypeStructure, Children: []model.Value{
		{Name: "a", Type: model.TypeInteger},
		{Name: "list", Type: model.TypeArray, Children: []model.Value{{Type: model.TypeInteger}}},
		{Type: model.TypeInteger}, // unnamed: not addressable
	}})
	if len(out) != 1 || out["L/N.D.a[CF]"].Type != model.TypeInteger {
		t.Errorf("flattened = %+v", out)
	}
}

func TestValueSideOfUnknownSession(t *testing.T) {
	e := newEngine(t)
	ghost := &session{id: "ghost", state: model.SessionConnected, rec: &recorder{}}
	ghost.index = map[string]*iec61850.ModelNode{"": {Children: []*iec61850.ModelNode{{
		Name: "LD", Reference: iec61850.Ref{LD: "LD"}, Children: []*iec61850.ModelNode{{
			Name: "LLN0", Reference: iec61850.Ref{LD: "LD", LN: "LLN0"}, Children: []*iec61850.ModelNode{{
				Name: "Mod", Reference: iec61850.Ref{LD: "LD", LN: "LLN0", Path: []string{"Mod"}}, FCs: []iec61850.FunctionalConstraint{"CF"},
			}},
		}},
	}}}}
	if _, err := e.valueSideOf(t.Context(), ghost); err == nil {
		t.Error("values of a session the engine does not know were read")
	}
}

func TestCompareValuesLiveAndSnapshot(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	one, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	two, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Compare(ctx, model.CompareRequest{A: one.ID, B: two.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareValues); cat.Items == 0 || cat.Changed != 0 {
		t.Errorf("two sessions to one IED: %+v", cat)
	}
	s, _ := e.session(one.ID)
	exp, err := e.Export(ctx, one.ID, model.ExportRequest{Format: model.ExportSnapshot, Refs: valueRefs(s)})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := e.OpenSnapshot(ctx, model.File{Name: exp.Name, Data: []byte(exp.Text)})
	if err != nil {
		t.Fatal(err)
	}
	c, err = e.Compare(ctx, model.CompareRequest{A: snap.ID, B: two.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareValues); cat.Items == 0 || cat.Changed != 0 {
		t.Errorf("snapshot with the IED: %+v", cat)
	}
	// A snapshot without those values compares nothing and says why.
	bare, err := e.Export(ctx, one.ID, model.ExportRequest{Format: model.ExportSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	bareSnap, err := e.OpenSnapshot(ctx, model.File{Name: "bare.json", Data: []byte(bare.Text)})
	if err != nil {
		t.Fatal(err)
	}
	c, err = e.Compare(ctx, model.CompareRequest{A: bareSnap.ID, B: two.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareValues); cat.Items != 0 || !strings.Contains(cat.Reason, "could not be read") {
		t.Errorf("bare snapshot: %+v", cat)
	}

	// A lost IED is not compared.
	ied.Close()
	sclInfo := openICD(t, e, simied.ICD())
	deadline := 0
	for {
		c, err = e.Compare(ctx, model.CompareRequest{A: sclInfo.ID, B: two.ID, Values: true})
		if err != nil {
			t.Fatal(err)
		}
		if cat := category(t, c, model.CompareValues); !cat.Compared {
			if !strings.HasPrefix(cat.Reason, "B: ") {
				t.Errorf("lost IED: %+v", cat)
			}
			break
		}
		if deadline++; deadline > 50 {
			t.Fatal("values still compared after the IED closed")
		}
	}
	c, err = e.Compare(ctx, model.CompareRequest{A: two.ID, B: sclInfo.ID, Values: true})
	if err != nil {
		t.Fatal(err)
	}
	if cat := category(t, c, model.CompareValues); cat.Compared || !strings.HasPrefix(cat.Reason, "A: ") {
		t.Errorf("lost IED as A: %+v", cat)
	}
}
