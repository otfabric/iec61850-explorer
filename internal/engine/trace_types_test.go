// SPDX-License-Identifier: MIT

package engine

import (
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"
)

// typesOf is a small set of templates: an ACD-like DO with attributes, a
// WYE-like DO with sub-data objects, a DO that mixes both under ST, an
// attribute of an unknown basic type and an array.
func typesOf() *sclTypes {
	dot := map[string]*scl.DOType{
		"ACD": {ID: "ACD", DAs: []scl.DA{
			{Name: "general", FC: "ST", BType: "BOOLEAN"}, {Name: "dirGeneral", FC: "ST", BType: "Enum"},
			{Name: "q", FC: "ST", BType: "Quality"}, {Name: "t", FC: "ST", BType: "Timestamp"},
			{Name: "d", FC: "DC", BType: "VisString255"},
		}},
		"CMV": {ID: "CMV", DAs: []scl.DA{{Name: "cVal", FC: "MX", BType: "Struct", Type: "Vector"}, {Name: "q", FC: "MX", BType: "Quality"}}},
		"WYE": {ID: "WYE", SDOs: []scl.SDO{{Name: "phsA", Type: "CMV"}, {Name: "phsB", Type: "CMV"}}},
		"MIX": {ID: "MIX", DAs: []scl.DA{{Name: "q", FC: "MX", BType: "Quality"}}, SDOs: []scl.SDO{{Name: "phsA", Type: "CMV"}}},
		"ODD": {ID: "ODD", DAs: []scl.DA{{Name: "x", FC: "ST", BType: "Currency"}}},
		"ARR": {ID: "ARR", DAs: []scl.DA{{Name: "vals", FC: "SP", BType: "INT32", Count: 3}}},
	}
	dat := map[string]*scl.DAType{
		"Vector": {ID: "Vector", BDAs: []scl.BDA{{Name: "mag", BType: "Struct", Type: "AV"}}},
		"AV":     {ID: "AV", BDAs: []scl.BDA{{Name: "f", BType: "FLOAT32"}}},
	}
	lnt := &scl.LNodeType{ID: "LN", DOs: []scl.DO{
		{Name: "Str", Type: "ACD"}, {Name: "A", Type: "WYE"}, {Name: "M", Type: "MIX"}, {Name: "O", Type: "ODD"}, {Name: "R", Type: "ARR"}, {Name: "X", Type: "NONE"},
	}}
	return &sclTypes{lns: map[string]*scl.LNodeType{"LD/LN1": lnt}, dot: dot, dat: dat}
}

func names(ts *mms.TypeSpec) []string {
	if ts == nil {
		return nil
	}
	var out []string
	for _, el := range ts.Elements {
		out = append(out, el.Name)
	}
	return out
}

func TestSCLTypes(t *testing.T) {
	ty := typesOf()
	ref := func(s string) iec61850.Ref {
		r, err := iec61850.ParseRef(s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, tc := range []struct {
		ref  string
		want []string
		nil_ bool
	}{
		{"LD/LN1.Str[ST]", []string{"general", "dirGeneral", "q", "t"}, false},
		{"LD/LN1.Str[DC]", []string{"d"}, false},
		{"LD/LN1.A[MX]", []string{"phsA", "phsB"}, false},
		{"LD/LN1.A.phsA[MX]", []string{"cVal", "q"}, false},
		{"LD/LN1.A.phsA.cVal[MX]", []string{"mag"}, false},
		{"LD/LN1.A.phsA.cVal.mag.f[MX]", nil, false},
		{"LD/LN1.Str.general[ST]", nil, false},
		{"LD/LN1.M[MX]", nil, true},             // attributes and sub-data objects: order unknown
		{"LD/LN1.O[ST]", nil, true},             // an unknown basic type
		{"LD/LN1.X[ST]", nil, true},             // an unknown DOType
		{"LD/LN1.Str[MX]", nil, true},           // no attribute under that FC
		{"LD/LN1.Str.general.x[ST]", nil, true}, // below a leaf
		{"LD/LN1.Str.nope[ST]", nil, true},
		{"LD/LN1.A.phsC[MX]", nil, true},
		{"LD/LN1.Nope[ST]", nil, true},
		{"LD/LN2.Str[ST]", nil, true},
		{"LD/LN1.Str.general[MX]", nil, true}, // an attribute of another FC
	} {
		ts := ty.typeOf(ref(tc.ref))
		if (ts == nil) != tc.nil_ {
			t.Errorf("%s: type %+v", tc.ref, ts)
			continue
		}
		if ts != nil && tc.want != nil && !equalStrings(names(ts), tc.want) {
			t.Errorf("%s: components %v, want %v", tc.ref, names(ts), tc.want)
		}
	}
	if ts := ty.typeOf(ref("LD/LN1.R.vals[SP]")); ts == nil || ts.Type != mms.ValueTypeArray || ts.Count != 3 {
		t.Errorf("array = %+v", ts)
	}
	var none *sclTypes
	if none.typeOf(ref("LD/LN1.Str[ST]")) != nil || ty.typeOf(iec61850.Ref{LD: "LD", LN: "LN1"}) != nil {
		t.Error("a type without templates or without a path")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A value is named only when it has exactly the type's shape.
func TestTypedValues(t *testing.T) {
	ty := typesOf()
	l := &traceLink{sclTypes: ty}
	q := mms.NewBitStringWithLength([]byte{0, 0}, 13)
	str := mms.NewStructure([]*mms.Value{mms.NewBoolean(true), mms.NewInteger(1), q, mms.NewUTCTime(time.Date(2026, 11, 10, 9, 0, 10, 0, time.UTC))})
	v := l.valueOf(str, "LD/LN1.Str[ST]", "Str")
	if got := []string{v.Children[0].Name, v.Children[2].Name, v.Children[3].Name}; !equalStrings(got, []string{"general", "q", "t"}) || v.Children[2].Quality == nil || v.Children[3].Time == nil {
		t.Errorf("named = %+v", v)
	}
	for name, val := range map[string]*mms.Value{
		"one component fewer": mms.NewStructure([]*mms.Value{mms.NewBoolean(true), mms.NewInteger(1), q}),
		"another kind":        mms.NewStructure([]*mms.Value{mms.NewInteger(1), mms.NewInteger(1), q, mms.NewUTCTime(time.Unix(0, 0))}),
		"not a structure":     mms.NewBoolean(true),
	} {
		got := l.valueOf(val, "LD/LN1.Str[ST]", "Str")
		for _, c := range got.Children {
			if c.Name != "" {
				t.Errorf("%s: named %+v", name, got)
			}
		}
	}
	arr := mms.NewArray([]*mms.Value{mms.NewInteger(1), mms.NewInteger(2), mms.NewInteger(3)})
	if !fits(arr, ty.typeOf(iec61850.Ref{LD: "LD", LN: "LN1", Path: []string{"R", "vals"}, FC: "SP"}), 0) {
		t.Error("an array of the type does not fit")
	}
	if fits(mms.NewArray([]*mms.Value{mms.NewBoolean(true)}), ty.typeOf(iec61850.Ref{LD: "LD", LN: "LN1", Path: []string{"R", "vals"}, FC: "SP"}), 0) {
		t.Error("an array of another element type fits")
	}
	// A live link knows types by reference; an unknown one stays unnamed.
	live := &traceLink{types: map[string]*mms.TypeSpec{"LD/LN1.Str[ST]": ty.typeOf(iec61850.Ref{LD: "LD", LN: "LN1", Path: []string{"Str"}, FC: "ST"})}}
	if live.valueOf(str, "LD/LN1.Str[ST]", "Str").Children[0].Name != "general" || live.valueOf(str, "LD/LN1.Op[ST]", "Op").Children[0].Name != "" {
		t.Error("live types")
	}
	var nolink *traceLink
	if nolink.typeOf("LD/LN1.Str[ST]") != nil || l.typeOf("not a ref") != nil {
		t.Error("types without a link or of a bad reference")
	}
	nolink.valuesOf(nil, nil, nil)
}
