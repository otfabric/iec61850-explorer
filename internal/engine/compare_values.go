// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// canon is a value reduced to what two sources can agree on: a boolean,
// a number (integers, enumerations as their ordinal, floats) or a string.
type canon struct {
	kind string // "bool", "num", "str"
	b    bool
	f    float64
	s    string
}

func (a canon) equal(b canon) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case "bool":
		return a.b == b.b
	case "num":
		// A FLOAT32 travels as 32 bits: compare at that precision.
		return a.f == b.f || float32(a.f) == float32(b.f) ||
			math.Abs(a.f-b.f) <= 1e-6*math.Max(1, math.Max(math.Abs(a.f), math.Abs(b.f)))
	}
	return a.s == b.s
}

// sclCanon reads an SCL value text by its basic type. Types without a
// value two sources can agree on (quality, time stamps, coded enums) are
// not compared.
func sclCanon(v sclValue, text string) (canon, string, bool) {
	text = trimSCLText(text)
	switch bt := v.bType; {
	case bt == "BOOLEAN":
		switch strings.ToLower(text) {
		case "true", "1":
			return canon{kind: "bool", b: true}, "true", true
		case "false", "0":
			return canon{kind: "bool", b: false}, "false", true
		}
	case bt == "Enum":
		if ord, ok := v.enum[text]; ok {
			return canon{kind: "num", f: float64(ord)}, fmt.Sprintf("%s (%d)", text, ord), true
		}
		if n, err := strconv.Atoi(text); err == nil {
			return canon{kind: "num", f: float64(n)}, text, true
		}
	case strings.HasPrefix(bt, "INT"):
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return canon{kind: "num", f: float64(n)}, text, true
		}
		if n, err := strconv.ParseUint(text, 10, 64); err == nil {
			return canon{kind: "num", f: float64(n)}, text, true
		}
	case bt == "FLOAT32" || bt == "FLOAT64":
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return canon{kind: "num", f: f}, text, true
		}
	case strings.HasPrefix(bt, "VisString") || strings.HasPrefix(bt, "Unicode"):
		return canon{kind: "str", s: text}, text, true
	}
	return canon{}, "", false
}

// liveCanon reads a value an IED (or a snapshot) gave.
func liveCanon(v model.Value) (canon, bool) {
	switch v.Type {
	case model.TypeBoolean:
		if b, ok := v.JSON.(bool); ok {
			return canon{kind: "bool", b: b}, true
		}
	case model.TypeInteger, model.TypeUnsigned, model.TypeFloat:
		// values.go gives int64, uint64, float64, or a string beyond 2^53;
		// a snapshot read back from JSON gives float64.
		switch n := v.JSON.(type) {
		case float64:
			return canon{kind: "num", f: n}, true
		case int64:
			return canon{kind: "num", f: float64(n)}, true
		case uint64:
			return canon{kind: "num", f: float64(n)}, true
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return canon{kind: "num", f: f}, true
			}
		}
	case model.TypeVisible, model.TypeMMSString:
		return canon{kind: "str", s: v.Text}, true
	}
	return canon{}, false
}

// valueSide is what one session gives the value comparison: the SCL
// file's configured values, or values read (from the IED or a snapshot).
type valueSide struct {
	scl   map[string]sclValue
	read  map[string]model.Value
	actSG map[string]uint8 // per logical device
	// unread counts the data objects whose values could not be read.
	unread int
}

// valueSideOf collects a session's values for valueFCs.
func (e *Engine) valueSideOf(ctx context.Context, s *session) (valueSide, error) {
	if s.sclValues != nil && s.snapValues == nil {
		return valueSide{scl: s.sclValues, actSG: s.sclActSG}, nil
	}
	if s.snapValues == nil {
		if err := s.liveErr(); err != nil {
			return valueSide{}, err
		}
	}
	refs := valueRefs(s)
	side := valueSide{read: map[string]model.Value{}, actSG: map[string]uint8{}}
	for start := 0; start < len(refs); start += model.MaxReadRefs {
		res, err := e.Read(ctx, s.id, model.ReadRequest{Refs: refs[start:min(start+model.MaxReadRefs, len(refs))]})
		if err != nil {
			return valueSide{}, err
		}
		for _, rv := range res.Values {
			if rv.Value == nil {
				side.unread++
				continue
			}
			base, fc, _ := strings.Cut(strings.TrimSuffix(rv.Ref, "]"), "[")
			flattenValue(side.read, base, fc, *rv.Value)
		}
	}
	if s.snapValues == nil {
		for _, ld := range valueLDs(s) {
			if sg, err := e.SettingGroups(ctx, s.id, ld); err == nil && sg.ActSG > 0 {
				side.actSG[ld] = sg.ActSG
			}
		}
	}
	return side, nil
}

// valueRefs lists the data objects to read, one reference per value FC:
// "LD/LN.DO[CF]".
func valueRefs(s *session) []string {
	var out []string
	for _, n := range compareNodes(s) {
		if n.kind != model.NodeDO {
			continue
		}
		for _, fc := range n.fcs {
			if isValueFC(fc) {
				out = append(out, n.ref+"["+fc+"]")
			}
		}
	}
	return out
}

func valueLDs(s *session) []string {
	return valueLDsOf(compareNodes(s))
}

// flattenValue records the basic attributes of a value read for a data
// object by their FC-qualified reference. Arrays are not compared.
func flattenValue(out map[string]model.Value, path, fc string, v model.Value) {
	switch v.Type {
	case model.TypeStructure:
		for _, c := range v.Children {
			if c.Name != "" {
				flattenValue(out, path+"."+c.Name, fc, c)
			}
		}
	case model.TypeArray:
	default:
		out[path+"["+fc+"]"] = v
	}
}

// ldOf is the logical device of a reference.
func ldOf(ref string) string {
	ld, _, _ := strings.Cut(ref, "/")
	return ld
}

func (c *comparer) values(ctx context.Context, e *Engine, sa, sb *session) model.CompareCategory {
	a, err := e.valueSideOf(ctx, sa)
	if err != nil {
		return unavailable(model.CompareValues, "A", err.Error())
	}
	b, err := e.valueSideOf(ctx, sb)
	if err != nil {
		return unavailable(model.CompareValues, "B", err.Error())
	}
	cat := model.CompareCategory{Name: model.CompareValues, Compared: true}
	skipped := 0
	for _, key := range sharedValueKeys(a, b) {
		fc := key[strings.LastIndex(key, "[")+1 : len(key)-1]
		ld := ldOf(key)
		groups := []uint8{0}
		if fc == "SG" {
			groups = settingGroups(a, b, key, ld)
		}
		compared := false
		for _, g := range groups {
			ca, ta, okA := a.canonFor(key, g, b)
			cb, tb, okB := b.canonFor(key, g, a)
			if !okA || !okB {
				continue
			}
			compared = true
			cat.Items++
			if ca.equal(cb) {
				cat.Same++
				continue
			}
			cat.Changed++
			d := model.Difference{Category: model.CompareValues, Kind: model.DiffChanged, Ref: key, Field: "value", A: ta, B: tb}
			if fc == "SG" {
				d.Note = sgNote(a, b, ld, g)
			}
			c.add(d)
		}
		if !compared {
			skipped++
		}
	}
	var reasons []string
	if skipped > 0 {
		reasons = append(reasons, fmt.Sprintf("%d attributes have a type or value that is not compared", skipped))
	}
	if n := a.unread + b.unread; n > 0 {
		reasons = append(reasons, fmt.Sprintf("%d data objects could not be read", n))
	}
	cat.Reason = strings.Join(reasons, "; ")
	return cat
}

// sharedValueKeys lists, sorted, the attributes both sides have a value
// for.
func sharedValueKeys(a, b valueSide) []string {
	var out []string
	for _, k := range a.keys() {
		if b.has(k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (v valueSide) keys() []string {
	if v.scl != nil {
		out := make([]string, 0, len(v.scl))
		for k := range v.scl {
			out = append(out, k)
		}
		return out
	}
	out := make([]string, 0, len(v.read))
	for k := range v.read {
		out = append(out, k)
	}
	return out
}

func (v valueSide) has(key string) bool {
	if v.scl != nil {
		_, ok := v.scl[key]
		return ok
	}
	_, ok := v.read[key]
	return ok
}

// settingGroups lists the setting groups to compare a setting in. Two SCL
// files compare every group either gives; otherwise the values read are
// those of the active group, which is the one compared.
func settingGroups(a, b valueSide, key, ld string) []uint8 {
	if a.scl != nil && b.scl != nil {
		gs := append(a.scl[key].groups(), b.scl[key].groups()...)
		slices.Sort(gs)
		return slices.Compact(gs)
	}
	for _, side := range []valueSide{a, b} {
		if side.read != nil {
			if sg, ok := side.actSG[ld]; ok {
				return []uint8{sg}
			}
		}
	}
	return []uint8{1}
}

func sgNote(a, b valueSide, ld string, g uint8) string {
	if a.scl != nil && b.scl != nil {
		return fmt.Sprintf("setting group %d", g)
	}
	if a.read != nil && b.read != nil {
		return fmt.Sprintf("active setting group %d / %d", a.actSG[ld], b.actSG[ld])
	}
	return fmt.Sprintf("setting group %d, active on the IED", g)
}

// canonFor gives the value of key for setting group g (0: not a setting)
// and its text; other is the opposite side, whose SCL types name the
// enumeration literal of a value read.
func (v valueSide) canonFor(key string, g uint8, other valueSide) (canon, string, bool) {
	if v.scl != nil {
		sv := v.scl[key]
		text, ok := "", false
		if g == 0 {
			text, ok = sv.vals[0].Value, true
		} else {
			text, ok = sv.forGroup(g)
		}
		if !ok {
			return canon{}, "", false
		}
		return sclCanon(sv, text)
	}
	rv := v.read[key]
	c, ok := liveCanon(rv)
	if !ok {
		return canon{}, "", false
	}
	text := rv.Text
	if sv, isSCL := other.scl[key]; isSCL && sv.bType == "Enum" && c.kind == "num" {
		for lit, ord := range sv.enum {
			if float64(ord) == c.f {
				text = fmt.Sprintf("%s (%s)", lit, rv.Text)
				break
			}
		}
	}
	return c, text, true
}
