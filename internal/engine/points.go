// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/otfabric/go-iec61850-model/catalog"
	"github.com/otfabric/go-iec61850-model/resolver"
	"github.com/otfabric/go-iec61850-model/semantic"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// exportPoints renders the point list of a session (M3d): every basic
// attribute with each of its functional constraints, control blocks left
// out; the namespace's CDC, type and presence; the measured quantity and
// its unit; the data sets that carry the attribute (or the data object or
// structure above it) and the report control blocks that report those data
// sets. A live session's data sets and report control blocks are read from
// the IED (Observe mode); a snapshot holds none.
func (e *Engine) exportPoints(ctx context.Context, s *session, format string) (model.ExportFile, error) {
	if _, err := embedded(); err != nil {
		return model.ExportFile{}, err
	}
	nodes := compareNodes(s)
	profiles := e.newLNProfiles(e.declarationsOf(ctx, s, nodes))
	lds := valueLDsOf(nodes)
	dsets, dsReason := definitions(ctx, s, lds, e.DataSets)
	rcbs, rcbReason := definitions(ctx, s, lds, e.Reports)
	members, reportsOf := membership(dsets, rcbs)

	points := []model.Point{}
	for _, n := range nodes {
		if !n.leaf || (n.kind != model.NodeDO && n.kind != model.NodeChild) {
			continue
		}
		for _, fc := range n.fcs {
			lp := profiles.forRef(n.ref)
			points = append(points, point(lp.profile, lp.resolver, n.ref, fc, members, reportsOf))
		}
	}

	info := s.info()
	base := exportBase(info, time.Now().UTC()) + "-points"
	note := joinReasons(dsReason, rcbReason)
	if format == model.ExportPointsJSON {
		side := sideOf(s)
		list := model.PointList{Schema: model.PointListSchema, App: appInfo(), Source: side.Source + " " + side.Label, Points: points, Note: note}
		b, _ := json.MarshalIndent(list, "", "  ") // strings and slices of them: cannot fail
		return model.ExportFile{Name: base + ".json", MIME: "application/json", Text: string(b) + "\n"}, nil
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"ref", "ld", "ln", "ln_class", "data_object", "attribute", "fc", "cdc", "type", "presence", "quantity", "unit", "data_sets", "reports"})
	for _, pt := range points {
		_ = w.Write(csvRow([]string{pt.Ref, pt.LD, pt.LN, pt.LNClass, pt.DataObject, pt.Attribute, pt.FC, pt.CDC, pt.Type, pt.Presence,
			pt.Quantity, pt.Unit, strings.Join(pt.DataSets, ";"), strings.Join(pt.Reports, ";")}))
	}
	w.Flush()
	return model.ExportFile{Name: base + ".csv", MIME: "text/csv", Text: buf.String()}, nil
}

// csvRow protects every cell with csvSafe.
func csvRow(row []string) []string {
	for i, c := range row {
		row[i] = csvSafe(c)
	}
	return row
}

func valueLDsOf(nodes []cmpNode) []string {
	var out []string
	for _, n := range nodes {
		if n.kind == model.NodeLD {
			out = append(out, n.ref)
		}
	}
	return out
}

// membership indexes data set members by their FC-qualified reference and
// report control blocks by the data set they report.
func membership(dsets map[string][]model.DataSet, rcbs map[string][]model.RCB) (map[string][]string, map[string][]string) {
	members := map[string][]string{}
	for _, list := range dsets {
		for _, d := range list {
			for _, m := range d.Members {
				members[m.Ref] = append(members[m.Ref], d.Ref)
			}
		}
	}
	reportsOf := map[string][]string{}
	for _, list := range rcbs {
		for _, r := range list {
			if r.DatSet != "" {
				ds := strings.ReplaceAll(r.DatSet, "$", ".") // "LD/LLN0$Meas" is LD/LLN0.Meas
				reportsOf[ds] = append(reportsOf[ds], r.Ref)
			}
		}
	}
	return members, reportsOf
}

func point(p *catalog.Profile, res *resolver.Resolver, ref, fc string, members, reportsOf map[string][]string) model.Point {
	ld, rest, _ := strings.Cut(ref, "/")
	ln, path, _ := strings.Cut(rest, ".")
	do, attr, _ := strings.Cut(path, ".")
	pt := model.Point{Ref: ref + "[" + fc + "]", LD: ld, LN: ln, LNClass: lnClass(ln), DataObject: do, Attribute: attr, FC: fc,
		DataSets: []string{}, Reports: []string{}}
	if r, err := res.ResolveString(pt.Ref); err == nil {
		if len(r.Steps) > 1 {
			pt.CDC, pt.Presence = r.Steps[1].Type, r.Steps[1].Presence.String()
		}
		if r.IsAttribute() {
			pt.Type = r.Leaf().Type
		}
	}
	if d, err := semantic.Describe(p, pt.Ref); err == nil {
		pt.Quantity, pt.Unit = string(d.Quantity), d.UnitHint.String()
	}
	// The attribute, or the data object or structure above it, as a member.
	for at := ref; ; {
		for _, ds := range members[at+"["+fc+"]"] {
			if !slices.Contains(pt.DataSets, ds) {
				pt.DataSets = append(pt.DataSets, ds)
			}
		}
		i := strings.LastIndex(at, ".")
		if i <= strings.Index(at, "/") {
			break
		}
		at = at[:i]
	}
	slices.Sort(pt.DataSets)
	for _, ds := range pt.DataSets {
		pt.Reports = append(pt.Reports, reportsOf[ds]...)
	}
	slices.Sort(pt.Reports)
	pt.Reports = slices.Compact(pt.Reports)
	return pt
}
