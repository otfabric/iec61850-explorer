// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850-model/catalog"
	gomodel "github.com/otfabric/go-iec61850-model/model"
	"github.com/otfabric/go-iec61850-model/resolver"
	"github.com/otfabric/go-iec61850-model/semantic"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// declarations are the namespaces a session declares: the values of the
// ldNs, lnNs and dataNs attributes, keyed by attribute path
// ("LD/LLN0.NamPlt.ldNs", "LD/GGIO1.NamPlt.lnNs", "LD/GGIO1.Vnd1.dataNs"),
// and, for an SCL session, the schema version as a fallback.
type declarations struct {
	attrs  map[string]string
	schema string
}

// declarationsOf collects the declarations of the logical nodes in nodes:
// from an SCL file, or read (from the IED in Observe mode, or from the
// snapshot): the data objects with FC EX, which hold ldNs (LLN0.NamPlt),
// lnNs (a node's NamPlt) and dataNs (an extension data object). What
// cannot be read is left undeclared.
func (e *Engine) declarationsOf(ctx context.Context, s *session, nodes []cmpNode) declarations {
	if s.sclDecls != nil && s.snapValues == nil {
		return declarations{attrs: s.sclDecls, schema: s.sclSchema}
	}
	d := declarations{attrs: map[string]string{}}
	if s.snapValues == nil && s.liveErr() != nil {
		return d
	}
	var refs []string
	for _, n := range nodes {
		if n.kind == model.NodeDO && slices.Contains(n.fcs, "EX") {
			refs = append(refs, n.ref+"[EX]")
		}
	}
	read := map[string]model.Value{}
	for start := 0; start < len(refs); start += model.MaxReadRefs {
		// The session is live or a snapshot: a read fails per value, and a
		// value that is not there leaves the declaration out.
		res, _ := e.Read(ctx, s.id, model.ReadRequest{Refs: refs[start:min(start+model.MaxReadRefs, len(refs))]})
		for _, rv := range res.Values {
			if rv.Value != nil {
				flattenValue(read, strings.TrimSuffix(rv.Ref, "[EX]"), "EX", *rv.Value)
			}
		}
	}
	for k, v := range read {
		path := strings.TrimSuffix(k, "[EX]")
		if slices.Contains(nsAttributes, path[strings.LastIndex(path, ".")+1:]) && v.Text != "" {
			d.attrs[path] = v.Text
		}
	}
	return d
}

// forLN is the namespace a logical node is checked against: its own lnNs,
// else its logical device's ldNs, else, for an SCL file of schema 1.7, IEC
// 61850-7-4 Edition 1.
func (d declarations) forLN(ln string) (decl, source string) {
	if v := d.attrs[ln+".NamPlt.lnNs"]; v != "" {
		return v, "lnNs"
	}
	ld, _, _ := strings.Cut(ln, "/")
	if v := d.attrs[ld+"/LLN0.NamPlt.ldNs"]; v != "" {
		return v, "ldNs"
	}
	if d.schema == "1.7" {
		return "IEC 61850-7-4:2003", "scl-schema"
	}
	return "", "none"
}

// dataNs is the namespace an extension data object declares, if any.
func (d declarations) dataNs(do string) string {
	return d.attrs[do+".dataNs"]
}

func namespaceIDs(p *catalog.Profile) []string {
	var out []string
	for _, id := range p.IDs() {
		out = append(out, id.String())
	}
	return out
}

// Semantics implements service.Service.
func (e *Engine) Semantics(ctx context.Context, id, ref string) (model.Semantics, error) {
	s, err := e.session(id)
	if err != nil {
		return model.Semantics{}, err
	}
	r, err := iec61850.ParseRef(strings.TrimSpace(ref))
	if err != nil || r.FC != "" {
		return model.Semantics{}, service.Errorf(service.CodeInvalidArgument, "%q is not an object reference without functional constraint", ref)
	}
	key := refKey(r)
	s.mu.Lock()
	n, ok := s.index[key]
	var node model.Node
	var children []string
	if ok {
		node = nodeOf(n)
		for _, c := range n.Children {
			children = append(children, c.Name)
		}
	}
	s.mu.Unlock()
	if !ok || key == "" {
		return model.Semantics{}, service.Errorf(service.CodeNotFound, "%q is not in the discovered model", ref)
	}
	out := model.Semantics{Ref: key, Kind: node.Kind}
	if node.Kind == model.NodeLD {
		out.Note = "a logical device has no class in the namespace"
		return out, nil
	}
	ln := r.LD + "/" + r.LN
	decls := e.declarationsOf(ctx, s, nodesOfLN(s, ln))
	decl, _ := decls.forLN(ln)
	p, dn, err := e.ns.profileFor(decl)
	if err != nil {
		return model.Semantics{}, err
	}
	out.Declared, out.CheckedAgainst, out.Edition1, out.Note = dn.Declared, dn.CheckedAgainst, dn.Edition1, dn.Note
	out.LNClass = lnClass(r.LN)
	cls, known := logicalNodeClass(p, out.LNClass)
	if !known {
		out.Note = joinSentences(out.Note, fmt.Sprintf("the namespace (%s) does not define the logical node class %s", strings.Join(namespaceIDs(p), ", "), out.LNClass))
		return out, nil
	}
	out.Known, out.Namespace, out.Base, out.Deprecated = true, cls.Source.String(), cls.Base, cls.Deprecated
	out.Title, out.Description = cls.Doc.Title, cls.Doc.Description
	if node.Kind == model.NodeLN {
		out.DataObjects = lnDataObjects(p, out.LNClass, children)
		return out, nil
	}

	// An extension data object is defined by the namespace it declares.
	doRef := ln + "." + r.Path[0]
	if ns := decls.dataNs(doRef); ns != "" {
		p, _, _ = e.ns.profileFor(ns) // the catalog is loaded already
	}
	res, err := resolver.New(p).ResolveString(key)
	if err != nil {
		out.Note = joinSentences(out.Note, fmt.Sprintf("the namespace does not define %s: %v", strings.Join(r.Path, "."), err))
		return out, nil
	}
	if len(res.Steps) > 1 {
		do := res.Steps[1]
		out.DataObject, out.CDC, out.Presence = do.Name, do.Type, do.Presence.String()
		if c, err := p.DataObject(out.LNClass, do.Name); err == nil {
			if c.DeclaredIn != nil {
				out.DeclaredIn = c.DeclaredIn.Name
			}
			out.Title, out.Description = c.Doc.Title, c.Doc.Description
		}
	}
	out.InstanceCDC = s.sclCDC[doRef]
	if res.IsAttribute() {
		leaf := res.Leaf()
		out.Attribute, out.Type, out.FC = res.AttributePath(), leaf.Type, string(res.FC())
	}
	if d, err := semantic.Describe(p, key); err == nil {
		out.Quantity, out.Phase, out.Unit = string(d.Quantity), string(d.Phase), d.UnitHint.String()
	}
	return out, nil
}

func joinSentences(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// nodesOfLN lists the data objects of a logical node and of its logical
// device's LLN0, for declarationsOf.
func nodesOfLN(s *session, ln string) []cmpNode {
	ld, _, _ := strings.Cut(ln, "/")
	var out []cmpNode
	for _, n := range compareNodes(s) {
		if n.parent == ln || n.parent == ld+"/LLN0" {
			out = append(out, n)
		}
	}
	return out
}

// logicalNodeClass looks a class up; an unknown class is a finding, not an
// error.
func logicalNodeClass(p *catalog.Profile, name string) (*gomodel.LogicalNodeClass, bool) {
	cls, err := p.LogicalNode(name)
	return cls, err == nil
}

// lnDataObjects lists a class's data objects and how many instances of each
// the node has (by the namespace's name: Ind1 and Ind2 are two of Ind).
func lnDataObjects(p *catalog.Profile, class string, children []string) []model.NSDDataObject {
	dos, _ := p.DataObjects(class) // a class the namespace defines
	present := map[string]int{}
	for _, name := range children {
		if m, ok := nsdName(p, class, name); ok {
			present[m]++
		}
	}
	out := make([]model.NSDDataObject, 0, len(dos))
	for _, d := range dos {
		out = append(out, model.NSDDataObject{Name: d.Name, CDC: d.Type, Presence: d.Presence.String(),
			Mandatory: d.Presence.IsMandatory(), Present: present[d.Name]})
	}
	return out
}

// nsdName is the namespace's name of a data object instance: itself, or a
// multi-instance data object without its number (Ind for Ind3).
func nsdName(p *catalog.Profile, class, name string) (string, bool) {
	if _, err := p.DataObject(class, name); err == nil {
		return name, true
	}
	base := strings.TrimRight(name, "0123456789")
	if base == name || base == "" {
		return "", false
	}
	if d, err := p.DataObject(class, base); err == nil && strings.Contains(d.Presence.Name, "multi") {
		return base, true
	}
	return "", false
}

// Conformance implements service.Service: the session's logical nodes and
// data objects, each against the namespace its logical device or node
// declares (ldNs, lnNs; an extension data object's dataNs). A declared
// namespace that is neither embedded nor loaded is a warning, a logical
// node class the namespace does not define a warning, a mandatory data
// object (presence M) the node lacks an error (a warning for a device of
// Edition 1, whose definitions IEC does not publish), a data object the
// class does not define information (not listed under a namespace that is
// not loaded), and, for an SCL session, a data object whose CDC differs
// from the namespace's a warning (information for Edition 1). Conditional
// presence (MOcond, AtLeastOne, ...) is not evaluated.
func (e *Engine) Conformance(ctx context.Context, id string) (model.Conformance, error) {
	start := time.Now()
	s, err := e.session(id)
	if err != nil {
		return model.Conformance{}, err
	}
	if _, err := embedded(); err != nil {
		return model.Conformance{}, err
	}
	c := &conformance{e: e, out: model.Conformance{Namespaces: []string{}, Declared: []model.DeclaredNamespace{}, Findings: []model.Finding{}},
		used: map[string]bool{}, notLoaded: map[string]bool{}}
	nodes := compareNodes(s)
	decls := e.declarationsOf(ctx, s, nodes)
	children := map[string][]string{}
	for _, n := range nodes {
		if n.kind == model.NodeDO {
			children[n.parent] = append(children[n.parent], n.ref[strings.LastIndex(n.ref, ".")+1:])
		}
	}
	seenLD := map[string]bool{}
	for _, n := range nodes {
		if n.kind != model.NodeLN {
			continue
		}
		c.out.LogicalNodes++
		decl, source := decls.forLN(n.ref)
		ld, _, _ := strings.Cut(n.ref, "/")
		p, dn := c.profile(n.ref, decl, source)
		switch {
		case source == "lnNs":
			c.out.Declared = append(c.out.Declared, dn)
		case !seenLD[ld]:
			seenLD[ld] = true
			dn.Ref = ld
			c.out.Declared = append(c.out.Declared, dn)
		}
		c.logicalNode(s, n, p, dn, decls, children[n.ref])
	}
	for ns := range c.used {
		c.out.Namespaces = append(c.out.Namespaces, ns)
	}
	slices.Sort(c.out.Namespaces)
	c.out.DurationMS = time.Since(start).Milliseconds()
	return c.out, nil
}

type conformance struct {
	e         *Engine
	out       model.Conformance
	used      map[string]bool
	notLoaded map[string]bool
}

func (c *conformance) add(f model.Finding) { addFinding(&c.out, f) }

// profile gives the profile for a declaration and reports, once, a
// declared namespace that is neither embedded nor loaded. The embedded
// catalog loaded before the first call, so it cannot fail.
func (c *conformance) profile(ref, decl, source string) (*catalog.Profile, model.DeclaredNamespace) {
	p, dn, _ := c.e.ns.profileFor(decl)
	for _, ns := range dn.CheckedAgainst {
		c.used[ns] = true
	}
	if dn.Unknown && !c.notLoaded[decl] {
		c.notLoaded[decl] = true
		c.add(model.Finding{Code: model.FindingNamespaceNotLoaded, Severity: model.SeverityWarning, Ref: ref,
			Message: fmt.Sprintf("%s declares %s, which is neither embedded nor loaded: load its definitions to check against them", ref, decl)})
	}
	dn.Ref, dn.Source = ref, source
	return p, dn
}

func (c *conformance) logicalNode(s *session, n cmpNode, p *catalog.Profile, dn model.DeclaredNamespace, decls declarations, children []string) {
	cls, known := logicalNodeClass(p, n.class)
	if !known {
		c.add(model.Finding{Code: model.FindingUnknownClass, Severity: model.SeverityWarning, Ref: n.ref,
			Message: fmt.Sprintf("%s does not define the logical node class %s", strings.Join(dn.CheckedAgainst, ", "), n.class)})
		return
	}
	present := map[string]bool{}
	for _, name := range children {
		c.out.DataObjects++
		doRef := n.ref + "." + name
		dp := p
		if ns := decls.dataNs(doRef); ns != "" {
			dp, _ = c.profile(doRef, ns, "dataNs")
		}
		m, ok := nsdName(dp, n.class, name)
		if !ok {
			// A vendor's extension data object in a standard logical node
			// often declares no dataNs: look in the other namespaces the
			// logical device declares (the vendor's, loaded).
			dp, m, ok = c.inOtherDeclared(n, name, decls)
		}
		if !ok {
			// Under a namespace that is not loaded, the one
			// namespace-not-loaded finding explains these.
			if !dn.Unknown {
				c.add(model.Finding{Code: model.FindingUnknownDataObject, Severity: model.SeverityInfo, Ref: doRef,
					Message: fmt.Sprintf("%s (%s) does not define the data object %s", n.class, cls.Source, name)})
			}
			continue
		}
		present[m] = true
		if cdc := s.sclCDC[doRef]; cdc != "" {
			if d, err := dp.DataObject(n.class, m); err == nil && d.Type != cdc {
				f := model.Finding{Code: model.FindingCDCMismatch, Severity: model.SeverityWarning, Ref: doRef,
					Message: fmt.Sprintf("CDC %s, the namespace defines %s.%s as %s", cdc, n.class, m, d.Type)}
				if dn.Edition1 {
					f.Severity, f.Message = model.SeverityInfo, f.Message+" since Edition 2; the device declares Edition 1"
				}
				c.add(f)
			}
		}
	}
	dos, _ := p.DataObjects(n.class)
	for _, d := range dos {
		if !d.Presence.IsMandatory() || present[d.Name] {
			continue
		}
		f := model.Finding{Code: model.FindingMissingMandatory, Severity: model.SeverityError, Ref: n.ref + "." + d.Name,
			Message: fmt.Sprintf("%s requires the data object %s (%s, presence %s)", n.class, d.Name, d.Type, d.Presence)}
		if dn.Edition1 {
			f.Severity = model.SeverityWarning
			f.Message += " since Edition 2; the device declares Edition 1"
		}
		c.add(f)
	}
}

// inOtherDeclared looks a data object up in the loaded or embedded
// namespaces declared elsewhere in the node's logical device.
func (c *conformance) inOtherDeclared(n cmpNode, name string, decls declarations) (*catalog.Profile, string, bool) {
	ld, _, _ := strings.Cut(n.ref, "/")
	var seen []string
	for key, ns := range decls.attrs {
		if strings.HasPrefix(key, ld+"/") {
			seen = append(seen, ns)
		}
	}
	slices.Sort(seen)
	seen = slices.Compact(seen)
	for _, ns := range seen {
		p, dn, _ := c.e.ns.profileFor(ns)
		if dn.Unknown {
			continue
		}
		if m, ok := nsdName(p, n.class, name); ok {
			return p, m, true
		}
	}
	return nil, "", false
}

// addFinding adds a finding, up to model.MaxFindings.
func addFinding(c *model.Conformance, f model.Finding) {
	if len(c.Findings) >= model.MaxFindings {
		c.Truncated = true
		return
	}
	c.Findings = append(c.Findings, f)
}

// quantityOf is the measured quantity of a data object, by the namespace's
// semantic rules; "" when none applies.
func quantityOf(p *catalog.Profile, ref string) string {
	d, err := semantic.Describe(p, ref)
	if err != nil {
		return ""
	}
	return string(d.Quantity)
}

// Quantities lists the measured quantities a search can ask for, from
// go-iec61850-model's semantic rules.
func Quantities() []string {
	var out []string
	for _, q := range semantic.Quantities() {
		out = append(out, string(q))
	}
	return out
}

// QuantityLabels names each of Quantities in words ("Active power"), from
// go-iec61850-model.
func QuantityLabels() map[string]string {
	out := map[string]string{}
	for _, q := range semantic.Quantities() {
		out[string(q)] = q.Label()
	}
	return out
}

// lnProfiles gives the profile and resolver for each logical node, by the
// namespace it declares, built once per declaration.
type lnProfiles struct {
	e     *Engine
	decls declarations
	byNS  map[string]lnProfile
}

type lnProfile struct {
	profile  *catalog.Profile
	resolver *resolver.Resolver
}

func (e *Engine) newLNProfiles(d declarations) *lnProfiles {
	if d.attrs == nil {
		d.attrs = map[string]string{}
	}
	return &lnProfiles{e: e, decls: d, byNS: map[string]lnProfile{}}
}

// forRef is the profile of the logical node a reference is in. The
// embedded catalog is loaded before the first call, so it cannot fail.
func (l *lnProfiles) forRef(ref string) lnProfile {
	ld, rest, _ := strings.Cut(ref, "/")
	ln, _, _ := strings.Cut(rest, ".")
	decl, _ := l.decls.forLN(ld + "/" + ln)
	if lp, ok := l.byNS[decl]; ok {
		return lp
	}
	p, _, _ := l.e.ns.profileFor(decl)
	lp := lnProfile{profile: p, resolver: resolver.New(p)}
	l.byNS[decl] = lp
	return lp
}
