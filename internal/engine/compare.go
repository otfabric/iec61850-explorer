// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// controlFCs are the functional constraints of control blocks. The MMS
// model of an IED lists its control blocks as nodes of their logical node
// (8-1 clause 8.1.3); an SCL file defines them apart from the data. The
// model comparison leaves them out, and report control blocks are
// compared as such.
var controlFCs = []string{"RP", "BR", "LG", "GO", "MS", "US"}

// Compare implements service.Service. It reads only: the model of each
// session as discovered or imported, and the data sets and report control
// blocks as the session lists them (from the IED for a live session, from
// the SCL file for an SCL session).
func (e *Engine) Compare(ctx context.Context, req model.CompareRequest) (model.Comparison, error) {
	start := time.Now()
	if req.A == "" || req.B == "" || req.A == req.B {
		return model.Comparison{}, service.Errorf(service.CodeInvalidArgument, "compare needs two different sessions")
	}
	sa, err := e.session(req.A)
	if err != nil {
		return model.Comparison{}, err
	}
	sb, err := e.session(req.B)
	if err != nil {
		return model.Comparison{}, err
	}
	c := &comparer{}
	out := model.Comparison{A: sideOf(sa), B: sideOf(sb)}

	na, nb := compareNodes(sa), compareNodes(sb)
	out.Categories = append(out.Categories, c.models(na, nb))
	shared := sharedLDs(na, nb)
	out.Categories = append(out.Categories,
		c.dataSets(ctx, e, sa, sb, shared),
		c.reports(ctx, e, sa, sb, shared))
	if req.Values {
		out.Categories = append(out.Categories, c.values(ctx, e, sa, sb))
	}

	out.Differences, out.Truncated = c.diffs, c.truncated
	if out.Differences == nil {
		out.Differences = []model.Difference{}
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out, nil
}

func sideOf(s *session) model.CompareSide {
	info := s.info()
	side := model.CompareSide{SessionID: s.id}
	switch {
	case info.Snapshot != nil:
		side.Source, side.Label = model.SourceSnapshot, info.Snapshot.File
	case info.SCL != nil:
		side.Source, side.Label = model.SourceSCL, info.SCL.File+" ("+info.SCL.IED+"/"+info.SCL.AccessPoint+")"
	default:
		side.Source, side.Label = model.SourceIED, info.Target.Host+":"+strconv.Itoa(info.Target.Port)
	}
	return side
}

// comparer collects the differences, up to model.MaxDifferences.
type comparer struct {
	diffs     []model.Difference
	truncated bool
}

func (c *comparer) add(d model.Difference) {
	if len(c.diffs) >= model.MaxDifferences {
		c.truncated = true
		return
	}
	c.diffs = append(c.diffs, d)
}

// cmpNode is a model node as the comparison sees it.
type cmpNode struct {
	ref    string
	parent string
	kind   string
	class  string
	fcs    []string
}

// compareNodes lists a session's model, depth first, without control
// blocks.
func compareNodes(s *session) []cmpNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []cmpNode
	var walk func(n *iec61850.ModelNode, parent string)
	walk = func(n *iec61850.ModelNode, parent string) {
		for _, c := range n.Children {
			node := nodeOf(c)
			fcs := slices.DeleteFunc(slices.Clone(node.FCs), func(fc string) bool { return slices.Contains(controlFCs, fc) })
			if len(node.FCs) > 0 && len(fcs) == 0 || isSGCB(node) {
				continue
			}
			slices.Sort(fcs)
			out = append(out, cmpNode{ref: node.Ref, parent: parent, kind: node.Kind, class: node.LNClass, fcs: fcs})
			walk(c, node.Ref)
		}
	}
	walk(s.index[""], "")
	return out
}

// isSGCB reports the setting group control block, which the MMS model
// lists under LLN0 with FC SP.
func isSGCB(n model.Node) bool {
	return n.Kind == model.NodeDO && n.Name == "SGCB" && strings.HasSuffix(n.Ref, "/LLN0.SGCB")
}

func (c *comparer) models(a, b []cmpNode) model.CompareCategory {
	cat := model.CompareCategory{Name: model.CompareModel, Compared: true}
	inA, inB := map[string]cmpNode{}, map[string]cmpNode{}
	for _, n := range a {
		inA[n.ref] = n
	}
	for _, n := range b {
		inB[n.ref] = n
	}
	// missing reports a node missing on the other side once: not when its
	// parent is missing too.
	missing := func(nodes []cmpNode, other map[string]cmpNode, kind string, count *int) {
		for _, n := range nodes {
			if _, ok := other[n.ref]; ok {
				continue
			}
			if _, parentThere := other[n.parent]; n.parent != "" && !parentThere {
				continue
			}
			*count++
			c.add(model.Difference{Category: model.CompareModel, Kind: kind, Ref: n.ref, Note: n.kind})
		}
	}
	missing(a, inB, model.DiffOnlyA, &cat.OnlyA)
	missing(b, inA, model.DiffOnlyB, &cat.OnlyB)
	for _, x := range a {
		y, ok := inB[x.ref]
		if !ok {
			continue
		}
		cat.Items++
		changed := false
		if x.kind != y.kind {
			c.add(model.Difference{Category: model.CompareModel, Kind: model.DiffChanged, Ref: x.ref, Field: "kind", A: x.kind, B: y.kind})
			changed = true
		}
		if x.class != y.class {
			c.add(model.Difference{Category: model.CompareModel, Kind: model.DiffChanged, Ref: x.ref, Field: "ln_class", A: x.class, B: y.class})
			changed = true
		}
		// The FCs of a logical device or node gather those of its data;
		// compare them where they belong.
		if (x.kind == model.NodeDO || x.kind == model.NodeChild) && !slices.Equal(x.fcs, y.fcs) {
			c.add(model.Difference{Category: model.CompareModel, Kind: model.DiffChanged, Ref: x.ref, Field: "fcs",
				A: strings.Join(x.fcs, ","), B: strings.Join(y.fcs, ",")})
			changed = true
		}
		if changed {
			cat.Changed++
		} else {
			cat.Same++
		}
	}
	return cat
}

// sharedLDs lists the logical devices both models have, in A's order.
func sharedLDs(a, b []cmpNode) []string {
	inB := map[string]bool{}
	for _, n := range b {
		if n.kind == model.NodeLD {
			inB[n.ref] = true
		}
	}
	var out []string
	for _, n := range a {
		if n.kind == model.NodeLD && inB[n.ref] {
			out = append(out, n.ref)
		}
	}
	return out
}

// definitions gives what a session lists per logical device, or why it
// cannot.
func definitions[T any](ctx context.Context, s *session, lds []string, list func(context.Context, string, string) ([]T, error)) (map[string][]T, string) {
	if s.snapshot != nil {
		return nil, "a snapshot holds the model and values, not the data set and report control block definitions"
	}
	out := map[string][]T{}
	var failed []string
	for _, ld := range lds {
		v, err := list(ctx, s.id, ld)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err().Error()
			}
			failed = append(failed, fmt.Sprintf("%s: %v", ld, err))
			continue
		}
		out[ld] = v
	}
	if len(failed) == len(lds) && len(lds) > 0 {
		return nil, strings.Join(failed, "; ")
	}
	return out, strings.Join(failed, "; ")
}

// unavailable is a category that could not be compared.
func unavailable(name, side, reason string) model.CompareCategory {
	return model.CompareCategory{Name: name, Reason: side + ": " + reason}
}

func joinReasons(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

func (c *comparer) dataSets(ctx context.Context, e *Engine, sa, sb *session, lds []string) model.CompareCategory {
	da, ra := definitions(ctx, sa, lds, e.DataSets)
	if da == nil {
		return unavailable(model.CompareDataSets, "A", ra)
	}
	db, rb := definitions(ctx, sb, lds, e.DataSets)
	if db == nil {
		return unavailable(model.CompareDataSets, "B", rb)
	}
	return c.compareDataSets(da, db, lds, joinReasons(prefixed("A", ra), prefixed("B", rb)))
}

// compareDataSets compares the data sets both sides list, per logical
// device.
func (c *comparer) compareDataSets(da, db map[string][]model.DataSet, lds []string, reason string) model.CompareCategory {
	cat := model.CompareCategory{Name: model.CompareDataSets, Compared: true, Reason: reason}
	for _, ld := range lds {
		as, aok := da[ld]
		bs, bok := db[ld]
		if !aok || !bok {
			continue
		}
		inB := map[string]model.DataSet{}
		for _, d := range bs {
			inB[d.Ref] = d
		}
		inA := map[string]bool{}
		for _, x := range as {
			inA[x.Ref] = true
			y, ok := inB[x.Ref]
			if !ok {
				cat.OnlyA++
				c.add(model.Difference{Category: model.CompareDataSets, Kind: model.DiffOnlyA, Ref: x.Ref, Note: fmt.Sprintf("%d members", len(x.Members))})
				continue
			}
			cat.Items++
			ma, mb := memberRefs(x), memberRefs(y)
			if slices.Equal(ma, mb) {
				cat.Same++
				continue
			}
			cat.Changed++
			c.add(model.Difference{Category: model.CompareDataSets, Kind: model.DiffChanged, Ref: x.Ref, Field: "members",
				A: strings.Join(ma, "\n"), B: strings.Join(mb, "\n"), Note: firstMemberDiff(ma, mb)})
		}
		for _, y := range bs {
			if !inA[y.Ref] {
				cat.OnlyB++
				c.add(model.Difference{Category: model.CompareDataSets, Kind: model.DiffOnlyB, Ref: y.Ref, Note: fmt.Sprintf("%d members", len(y.Members))})
			}
		}
	}
	return cat
}

func prefixed(side, reason string) string {
	if reason == "" {
		return ""
	}
	return side + ": " + reason
}

func memberRefs(d model.DataSet) []string {
	out := make([]string, len(d.Members))
	for i, m := range d.Members {
		out[i] = m.Ref
	}
	return out
}

// firstMemberDiff names the first position where two member lists differ:
// the order of a data set's members is part of its definition (it is the
// order of the values in a report or GOOSE message).
func firstMemberDiff(a, b []string) string {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return fmt.Sprintf("member %d differs: %s / %s", i+1, a[i], b[i])
		}
	}
	return fmt.Sprintf("%d members / %d members", len(a), len(b))
}

func (c *comparer) reports(ctx context.Context, e *Engine, sa, sb *session, lds []string) model.CompareCategory {
	da, ra := definitions(ctx, sa, lds, e.Reports)
	if da == nil {
		return unavailable(model.CompareReports, "A", ra)
	}
	db, rb := definitions(ctx, sb, lds, e.Reports)
	if db == nil {
		return unavailable(model.CompareReports, "B", rb)
	}
	return c.compareRCBs(da, db, lds, joinReasons(prefixed("A", ra), prefixed("B", rb)))
}

// compareRCBs compares the report control blocks both sides list, per
// logical device.
func (c *comparer) compareRCBs(da, db map[string][]model.RCB, lds []string, reason string) model.CompareCategory {
	cat := model.CompareCategory{Name: model.CompareReports, Compared: true, Reason: reason}
	for _, ld := range lds {
		as, aok := da[ld]
		bs, bok := db[ld]
		if !aok || !bok {
			continue
		}
		matchedB := map[string]bool{}
		for _, x := range as {
			ys := rcbInstances(x.Ref, bs)
			if len(ys) == 0 {
				cat.OnlyA++
				c.add(model.Difference{Category: model.CompareReports, Kind: model.DiffOnlyA, Ref: x.Ref})
				continue
			}
			for _, y := range ys {
				matchedB[y.Ref] = true
				cat.Items++
				if c.rcbFields(y.Ref, x, y) {
					cat.Changed++
				} else {
					cat.Same++
				}
			}
		}
		for _, y := range bs {
			if !matchedB[y.Ref] && len(rcbInstances(y.Ref, as)) == 0 {
				cat.OnlyB++
				c.add(model.Difference{Category: model.CompareReports, Kind: model.DiffOnlyB, Ref: y.Ref})
			}
		}
	}
	return cat
}

// rcbInstances finds the report control blocks of list that are ref: the
// same name, or an indexed instance of it. An SCL ReportControl with
// RptEnabled max greater than 1 is served as instances whose names add a
// two-digit index ("brcbA" as "brcbA01", "brcbA02", ...).
func rcbInstances(ref string, list []model.RCB) []model.RCB {
	var out []model.RCB
	for _, r := range list {
		if r.Ref == ref {
			return []model.RCB{r}
		}
		if suffix, ok := strings.CutPrefix(r.Ref, ref); ok && len(suffix) == 2 && isDigits(suffix) {
			out = append(out, r)
		}
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// rcbFields compares the configuration of two report control blocks, as
// both an SCL file and an IED state it; the run-time attributes (enabled,
// reserved, sequence number, owner) are not configuration. A report ID
// left empty in SCL is assigned by the IED and not compared.
func (c *comparer) rcbFields(ref string, a, b model.RCB) bool {
	if a.Error != nil || b.Error != nil {
		msg := func(r model.RCB) string {
			if r.Error == nil {
				return ""
			}
			return r.Error.Message
		}
		c.add(model.Difference{Category: model.CompareReports, Kind: model.DiffChanged, Ref: ref, Field: "error", A: msg(a), B: msg(b),
			Note: "the report control block could not be read"})
		return true
	}
	changed := false
	field := func(name, x, y string) {
		if x != y {
			c.add(model.Difference{Category: model.CompareReports, Kind: model.DiffChanged, Ref: ref, Field: name, A: x, B: y})
			changed = true
		}
	}
	field("buffered", strconv.FormatBool(a.Buffered), strconv.FormatBool(b.Buffered))
	if a.RptID != "" && b.RptID != "" {
		field("rpt_id", a.RptID, b.RptID)
	}
	field("dat_set", a.DatSet, b.DatSet)
	field("conf_rev", strconv.FormatUint(uint64(a.ConfRev), 10), strconv.FormatUint(uint64(b.ConfRev), 10))
	field("opt_flds", strings.Join(a.OptFlds, ","), strings.Join(b.OptFlds, ","))
	field("trg_ops", strings.Join(a.TrgOps, ","), strings.Join(b.TrgOps, ","))
	field("buf_tm_ms", strconv.FormatUint(uint64(a.BufTmMS), 10), strconv.FormatUint(uint64(b.BufTmMS), 10))
	field("intg_pd_ms", strconv.FormatUint(uint64(a.IntgPdMS), 10), strconv.FormatUint(uint64(b.IntgPdMS), 10))
	return changed
}
