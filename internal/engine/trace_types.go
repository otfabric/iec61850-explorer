// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"maps"
	"slices"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms-trace/trace"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// The components of a captured value carry no names: an MMS structure is a
// sequence. A linked model names them: the MMS type of a reference (for a
// live session the IED's GetVariableAccessAttributes, for an SCL session its
// data type templates, in their order, which is the MMS order). A value is
// named only when it has exactly the shape of that type; otherwise it is
// shown unnamed. Names are never guessed.

// maxLinkTypes bounds the types a live session is asked for when a trace is
// linked to it.
const maxLinkTypes = 512

// sclLeafTypes maps the basic types of IEC 61850-7-2 to the MMS type of
// their value (IEC 61850-8-1). A basic type not listed leaves a value
// unnamed.
var sclLeafTypes = map[string]mms.TypeSpec{
	"BOOLEAN":      {Type: mms.ValueTypeBoolean},
	"INT8":         {Type: mms.ValueTypeInteger, Size: 8},
	"INT16":        {Type: mms.ValueTypeInteger, Size: 16},
	"INT24":        {Type: mms.ValueTypeInteger, Size: 24},
	"INT32":        {Type: mms.ValueTypeInteger, Size: 32},
	"INT64":        {Type: mms.ValueTypeInteger, Size: 64},
	"INT8U":        {Type: mms.ValueTypeUnsigned, Size: 8},
	"INT16U":       {Type: mms.ValueTypeUnsigned, Size: 16},
	"INT24U":       {Type: mms.ValueTypeUnsigned, Size: 24},
	"INT32U":       {Type: mms.ValueTypeUnsigned, Size: 32},
	"FLOAT32":      {Type: mms.ValueTypeFloat, FormatWidth: 32, ExponentWidth: 8},
	"FLOAT64":      {Type: mms.ValueTypeFloat, FormatWidth: 64, ExponentWidth: 11},
	"Enum":         {Type: mms.ValueTypeInteger, Size: 8},
	"Dbpos":        {Type: mms.ValueTypeBitString, Size: 2},
	"Quality":      {Type: mms.ValueTypeBitString, Size: 13},
	"Timestamp":    {Type: mms.ValueTypeUTCTime},
	"EntryTime":    {Type: mms.ValueTypeBinaryTime},
	"VisString32":  {Type: mms.ValueTypeVisibleString, Size: 32},
	"VisString64":  {Type: mms.ValueTypeVisibleString, Size: 64},
	"VisString65":  {Type: mms.ValueTypeVisibleString, Size: 65},
	"VisString129": {Type: mms.ValueTypeVisibleString, Size: 129},
	"VisString255": {Type: mms.ValueTypeVisibleString, Size: 255},
	"ObjRef":       {Type: mms.ValueTypeVisibleString, Size: 129},
	"Unicode255":   {Type: mms.ValueTypeMmsString, Size: 255},
	"Octet64":      {Type: mms.ValueTypeOctetString, Size: 64},
}

// sclTypes are the data type templates of an SCL session, with the type of
// each logical node.
type sclTypes struct {
	// lns maps "LD/LN" to its LNodeType.
	lns map[string]*scl.LNodeType
	dot map[string]*scl.DOType
	dat map[string]*scl.DAType
}

func newSCLTypes(doc *scl.SCL, ied *scl.IED, ap *scl.AccessPoint) *sclTypes {
	t := &sclTypes{lns: map[string]*scl.LNodeType{}, dot: map[string]*scl.DOType{}, dat: map[string]*scl.DAType{}}
	dtt := &doc.DataTypeTemplates
	lnt := map[string]*scl.LNodeType{}
	for i := range dtt.LNodeTypes {
		lnt[dtt.LNodeTypes[i].ID] = &dtt.LNodeTypes[i]
	}
	for i := range dtt.DOTypes {
		t.dot[dtt.DOTypes[i].ID] = &dtt.DOTypes[i]
	}
	for i := range dtt.DATypes {
		t.dat[dtt.DATypes[i].ID] = &dtt.DATypes[i]
	}
	for _, ld := range ap.Server.LDevices {
		domain := ldDomain(ied, ld)
		if ld.LN0 != nil {
			if lt, ok := lnt[ld.LN0.LNType]; ok {
				t.lns[domain+"/LLN0"] = lt
			}
		}
		for _, ln := range ld.LNs {
			if lt, ok := lnt[ln.LNType]; ok {
				t.lns[domain+"/"+ln.Prefix+ln.LNClass+ln.Inst] = lt
			}
		}
	}
	return t
}

// attr is a data attribute or a component of one.
type attr struct {
	bType, typ string
	count      int
}

// typeOf is the MMS type of an FC-qualified reference to a data object or
// attribute, nil when the templates do not tell it exactly: an unknown type
// or basic type, or a data object with both attributes and sub-data
// objects under the FC (the parsed templates do not keep their order).
func (t *sclTypes) typeOf(ref iec61850.Ref) *mms.TypeSpec {
	if t == nil || len(ref.Path) == 0 || ref.FC == "" {
		return nil
	}
	lt := t.lns[ref.LD+"/"+ref.LN]
	if lt == nil {
		return nil
	}
	var dot *scl.DOType
	for _, do := range lt.DOs {
		if do.Name == ref.Path[0] {
			dot = t.dot[do.Type]
		}
	}
	if dot == nil {
		return nil
	}
	fc := string(ref.FC)
	var cur *attr
	for _, name := range ref.Path[1:] {
		next, ok := t.step(dot, cur, name, fc)
		if !ok {
			return nil
		}
		dot, cur = next.dot, next.attr
	}
	var ts *mms.TypeSpec
	if cur != nil {
		ts = t.attrSpec(*cur, 0)
	} else {
		ts, _ = t.doSpec(dot, fc, 0)
	}
	if ts == nil || (ts.Type == mms.ValueTypeStructure && len(ts.Elements) == 0) {
		return nil
	}
	return ts
}

// position is where a path stands: in a data object, or in an attribute.
type position struct {
	dot  *scl.DOType
	attr *attr
}

// step follows one path component from a data object or an attribute.
func (t *sclTypes) step(dot *scl.DOType, cur *attr, name, fc string) (position, bool) {
	if cur != nil {
		dat := t.dat[cur.typ]
		if cur.bType != "Struct" || dat == nil || cur.count > 0 {
			return position{}, false
		}
		for _, b := range dat.BDAs {
			if b.Name == name {
				return position{attr: &attr{bType: b.BType, typ: b.Type, count: b.Count}}, true
			}
		}
		return position{}, false
	}
	for _, sdo := range dot.SDOs {
		if sdo.Name == name {
			if sub := t.dot[sdo.Type]; sub != nil {
				return position{dot: sub}, true
			}
			return position{}, false
		}
	}
	for _, da := range dot.DAs {
		if da.Name == name && da.FC == fc {
			return position{attr: &attr{bType: da.BType, typ: da.Type, count: da.Count}}, true
		}
	}
	return position{}, false
}

// doSpec is the structure of a data object under an FC; ok is false when
// it cannot be told exactly.
func (t *sclTypes) doSpec(dot *scl.DOType, fc string, depth int) (*mms.TypeSpec, bool) {
	if depth >= maxValueDepth {
		return nil, false
	}
	out := &mms.TypeSpec{Type: mms.ValueTypeStructure}
	das, sdos := 0, 0
	for _, da := range dot.DAs {
		if da.FC != fc {
			continue
		}
		ts := t.attrSpec(attr{bType: da.BType, typ: da.Type, count: da.Count}, depth+1)
		if ts == nil {
			return nil, false
		}
		out.Elements = append(out.Elements, mms.TypeSpecElement{Name: da.Name, Type: *ts})
		das++
	}
	for _, sdo := range dot.SDOs {
		sub := t.dot[sdo.Type]
		if sub == nil {
			return nil, false
		}
		ts, ok := t.doSpec(sub, fc, depth+1)
		if !ok {
			return nil, false
		}
		if len(ts.Elements) > 0 {
			out.Elements = append(out.Elements, mms.TypeSpecElement{Name: sdo.Name, Type: *ts})
			sdos++
		}
	}
	if das > 0 && sdos > 0 {
		return nil, false
	}
	return out, true
}

// attrSpec is the type of an attribute, nil when it cannot be told.
func (t *sclTypes) attrSpec(a attr, depth int) *mms.TypeSpec {
	if depth >= maxValueDepth {
		return nil
	}
	var elem *mms.TypeSpec
	switch a.bType {
	case "Struct":
		dat := t.dat[a.typ]
		if dat == nil {
			return nil
		}
		elem = &mms.TypeSpec{Type: mms.ValueTypeStructure}
		for _, b := range dat.BDAs {
			ts := t.attrSpec(attr{bType: b.BType, typ: b.Type, count: b.Count}, depth+1)
			if ts == nil {
				return nil
			}
			elem.Elements = append(elem.Elements, mms.TypeSpecElement{Name: b.Name, Type: *ts})
		}
	default:
		leaf, ok := sclLeafTypes[a.bType]
		if !ok {
			return nil
		}
		elem = &leaf
	}
	if a.count > 0 {
		return &mms.TypeSpec{Type: mms.ValueTypeArray, Count: a.count, Element: elem}
	}
	return elem
}

// fits says whether a value has exactly the shape of a type: the same kind
// at every level and as many components as the type names.
func fits(v *mms.Value, ts *mms.TypeSpec, depth int) bool {
	if v == nil || ts == nil || depth >= maxValueDepth || v.Type() != ts.Type {
		return false
	}
	switch ts.Type {
	case mms.ValueTypeStructure:
		elems, _ := v.Structure()
		if len(elems) != len(ts.Elements) {
			return false
		}
		for i, el := range elems {
			if ts.Elements[i].Name == "" || !fits(el, &ts.Elements[i].Type, depth+1) {
				return false
			}
		}
	case mms.ValueTypeArray:
		elems, _ := v.ArrayElements()
		for _, el := range elems {
			if !fits(el, ts.Element, depth+1) {
				return false
			}
		}
	default:
		// A scalar fits by its kind alone.
	}
	return true
}

// valueOf converts a captured value of ref (FC-qualified) with the type the
// link knows for it, or unnamed when the link does not know it or the
// value does not fit.
func (l *traceLink) valueOf(v *mms.Value, ref, name string) model.Value {
	if ts := l.typeOf(ref); ts != nil && fits(v, ts, 0) {
		return convertValue(v, ts, name)
	}
	return convertValue(v, nil, name)
}

// typeOf is the MMS type of an FC-qualified reference in the linked model.
func (l *traceLink) typeOf(ref string) *mms.TypeSpec {
	if l == nil || ref == "" {
		return nil
	}
	if l.sclTypes != nil {
		r, err := iec61850.ParseRef(ref)
		if err != nil {
			return nil
		}
		return l.sclTypes.typeOf(r)
	}
	return l.types[ref]
}

// typedRefs lists the FC-qualified references whose values a trace may
// show: the members of the linked data sets and the variables its reads
// and writes name.
func typedRefs(res *trace.Result, l *traceLink) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ref string) {
		if ref != "" && !seen[ref] && len(out) < maxLinkTypes {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	keys := slices.Sorted(maps.Keys(l.dataSets))
	for _, k := range keys {
		for _, m := range l.dataSets[k].Members {
			add(m.Ref)
		}
	}
	if res != nil {
		for _, t := range res.Transactions {
			if t.Request == nil {
				continue
			}
			for _, v := range t.Request.Variables {
				if len(v.Access) == 0 {
					add(objectOf(v.Name, false).Ref)
				}
			}
		}
	}
	return out
}

// liveTypes asks a live session for the MMS types of refs (Observe mode,
// GetVariableAccessAttributes, cached by the session).
func (e *Engine) liveTypes(ctx context.Context, id string, refs []string) map[string]*mms.TypeSpec {
	out := map[string]*mms.TypeSpec{}
	if len(refs) == 0 {
		return out
	}
	_ = e.live(ctx, id, "types", "", 4, func(ctx context.Context, s *session) error {
		for _, ref := range refs {
			r, err := iec61850.ParseRef(ref)
			if err != nil || r.FC == "" || len(r.Path) == 0 {
				continue
			}
			if ts := e.typeOf(ctx, s, r); ts != nil {
				out[ref] = ts
			}
		}
		return nil
	})
	return out
}

// valuesOf converts the values of a captured PDU: named by the variables
// of the request when the link knows their types and each value is the
// variable's (a Read response or a Write request of a variable list).
func (l *traceLink) valuesOf(vars []mms.CapturedVariable, values []*mms.Value, out []model.Value) {
	if l == nil || len(vars) != len(values) {
		return
	}
	for i := range out {
		if len(vars[i].Access) > 0 {
			continue
		}
		ref := objectOf(vars[i].Name, false).Ref
		name := ""
		if r, err := iec61850.ParseRef(ref); err == nil && len(r.Path) > 0 {
			name = r.Path[len(r.Path)-1]
		}
		if ts := l.typeOf(ref); ts != nil && fits(values[i], ts, 0) {
			out[i] = convertValue(values[i], ts, name)
		}
	}
}
