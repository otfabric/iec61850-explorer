// SPDX-License-Identifier: MIT

package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/otfabric/go-iec61850/scl"
)

// valueFCs are the functional constraints whose values a configuration
// sets and the value comparison checks: configuration (CF), description
// (DC), set points (SP) and settings (SG). Settings being edited (SE) and
// process values (ST, MX) are not configuration.
var valueFCs = []string{"CF", "DC", "SP", "SG"}

// sclValue is the value an SCL file configures for one attribute: the
// instance's DAI values or, without them, the type's default.
type sclValue struct {
	bType string
	vals  []scl.Value
	// enum maps an enumeration's literals to their ordinals; nil for other
	// types.
	enum map[string]int
}

// sclValues resolves the configured values of an access point's server,
// keyed by FC-qualified reference ("LD/LN.DO.a.b[FC]"), for valueFCs.
// Attributes without a value are left out, and so are arrays. It also
// gives the active setting group per logical device and the CDC each data
// object's type declares ("LD/LN.DO").
// sclModel is what an SCL session keeps besides its tree: the configured
// values, the active setting group per logical device, the CDC of each data
// object, and the namespaces the file declares (the values of ldNs, lnNs and
// dataNs attributes, keyed by attribute path such as "LD/LLN0.NamPlt.ldNs").
type sclModel struct {
	values map[string]sclValue
	actSG  map[string]uint8
	cdcs   map[string]string
	decls  map[string]string
}

func sclValues(doc *scl.SCL, ied *scl.IED, ap *scl.AccessPoint) sclModel {
	t := doc.DataTypeTemplates
	lnTypes := map[string]scl.LNodeType{}
	for _, x := range t.LNodeTypes {
		lnTypes[x.ID] = x
	}
	doTypes := map[string]scl.DOType{}
	for _, x := range t.DOTypes {
		doTypes[x.ID] = x
	}
	daTypes := map[string]scl.DAType{}
	for _, x := range t.DATypes {
		daTypes[x.ID] = x
	}
	enums := map[string]map[string]int{}
	for _, x := range t.EnumTypes {
		m := map[string]int{}
		for _, v := range x.Vals {
			m[v.Value] = v.Ord
		}
		enums[x.ID] = m
	}

	out := map[string]sclValue{}
	actSG := map[string]uint8{}
	cdcs := map[string]string{}
	r := &valueResolver{doTypes: doTypes, daTypes: daTypes, enums: enums, out: out, decls: map[string]string{}}
	for _, ld := range ap.Server.LDevices {
		domain := ldDomain(ied, ld)
		lns := ld.LNs
		if ld.LN0 != nil {
			lns = append([]scl.LN{*ld.LN0}, lns...)
			if sc := ld.LN0.SettingControl; sc != nil {
				actSG[domain] = sc.ActSG
			}
		}
		for i, ln := range lns {
			name := ln.Prefix + ln.LNClass + ln.Inst
			if i == 0 && ld.LN0 != nil {
				name = "LLN0"
			}
			lnt, ok := lnTypes[ln.LNType]
			if !ok {
				continue
			}
			dois := map[string]scl.DOI{}
			for _, d := range ln.DOIs {
				dois[d.Name] = d
			}
			for _, do := range lnt.DOs {
				doi := dois[do.Name]
				if dot, ok := doTypes[do.Type]; ok && dot.CDC != "" {
					cdcs[domain+"/"+name+"."+do.Name] = dot.CDC
				}
				r.dataObject(domain+"/"+name+"."+do.Name, do.Type, override{dais: doi.DAIs, sdis: doi.SDIs}, 0)
			}
		}
	}
	return sclModel{values: out, actSG: actSG, cdcs: cdcs, decls: r.decls}
}

// override is the instance part of a data object or structure: its DAI
// and SDI elements.
type override struct {
	dais []scl.DAI
	sdis []scl.SDI
}

func (o override) dai(name string) (scl.DAI, bool) {
	for _, d := range o.dais {
		if d.Name == name {
			return d, true
		}
	}
	return scl.DAI{}, false
}

func (o override) sdi(name string) override {
	for _, s := range o.sdis {
		if s.Name == name {
			return override{dais: s.DAIs, sdis: s.SDIs}
		}
	}
	return override{}
}

type valueResolver struct {
	doTypes map[string]scl.DOType
	daTypes map[string]scl.DAType
	enums   map[string]map[string]int
	out     map[string]sclValue
	decls   map[string]string
}

// nsAttributes are the attributes that declare a namespace (FC EX).
var nsAttributes = []string{"ldNs", "lnNs", "dataNs"}

// maxTypeDepth stops a recursive type definition.
const maxTypeDepth = 16

func (r *valueResolver) dataObject(path, typeID string, o override, depth int) {
	dot, ok := r.doTypes[typeID]
	if !ok || depth > maxTypeDepth {
		return
	}
	for _, da := range dot.DAs {
		if da.Count > 0 || !isValueFC(da.FC) && da.FC != "EX" {
			continue
		}
		r.attribute(path+"."+da.Name, da.FC, da.BType, da.Type, da.Vals, o, da.Name, depth)
	}
	for _, sdo := range dot.SDOs {
		r.dataObject(path+"."+sdo.Name, sdo.Type, o.sdi(sdo.Name), depth+1)
	}
}

// attribute records a basic attribute or descends into a structure.
// The parser fills Vals whenever an element has a value (go-iec61850
// v1.14.0); Val is only the first of them.
func (r *valueResolver) attribute(path, fc, bType, typeID string, vals []scl.Value, o override, name string, depth int) {
	if bType == "Struct" {
		dat, ok := r.daTypes[typeID]
		if !ok || depth > maxTypeDepth {
			return
		}
		inner := o.sdi(name)
		for _, b := range dat.BDAs {
			if b.Count > 0 {
				continue
			}
			r.attribute(path+"."+b.Name, fc, b.BType, b.Type, b.Vals, inner, b.Name, depth+1)
		}
		return
	}
	if d, ok := o.dai(name); ok && len(d.Vals) > 0 {
		vals = d.Vals
	}
	if len(vals) == 0 {
		return
	}
	if fc == "EX" {
		if slices.Contains(nsAttributes, name) {
			r.decls[path] = trimSCLText(vals[0].Value)
		}
		return
	}
	v := sclValue{bType: bType, vals: vals}
	if bType == "Enum" {
		v.enum = r.enums[typeID]
	}
	r.out[fmt.Sprintf("%s[%s]", path, fc)] = v
}

func isValueFC(fc string) bool {
	for _, f := range valueFCs {
		if f == fc {
			return true
		}
	}
	return false
}

// forGroup is the value of setting group sg: the value with that sGroup or,
// when the values carry none, the only value.
func (v sclValue) forGroup(sg uint8) (string, bool) {
	for _, x := range v.vals {
		if x.SGroup == uint32(sg) {
			return x.Value, true
		}
	}
	if len(v.vals) == 1 && v.vals[0].SGroup == 0 {
		return v.vals[0].Value, true
	}
	return "", false
}

// groups lists the setting groups the values are given for; 0 is a value
// for every group.
func (v sclValue) groups() []uint8 {
	var out []uint8
	for _, x := range v.vals {
		if x.SGroup <= 255 {
			out = append(out, uint8(x.SGroup))
		}
	}
	return out
}

// trimSCLText removes the white space SCL text may carry around a value.
func trimSCLText(s string) string { return strings.TrimSpace(s) }
