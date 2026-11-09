// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850-model/catalog"
	gomodel "github.com/otfabric/go-iec61850-model/model"
	"github.com/otfabric/go-iec61850-model/official"
	"github.com/otfabric/go-iec61850-model/resolver"
	"github.com/otfabric/go-iec61850-model/semantic"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// The namespaces of go-iec61850-model's default profile (IEC 61850-7-2,
// 7-3, 7-4, 7-410, 7-420), embedded in the binary and loaded once, on
// first use (about 30 ms).
var (
	nsdOnce    sync.Once
	nsdProfile *catalog.Profile
	nsdErr     error
	// loadNamespace is replaced by tests.
	loadNamespace = official.Default
)

func namespace() (*catalog.Profile, error) {
	nsdOnce.Do(func() {
		nsdProfile, nsdErr = loadNamespace()
	})
	if nsdErr != nil {
		return nil, service.Errorf(service.CodeInternal, "the embedded IEC 61850 namespaces: %v", nsdErr)
	}
	return nsdProfile, nil
}

func namespaceIDs(p *catalog.Profile) []string {
	var out []string
	for _, id := range p.IDs() {
		out = append(out, id.String())
	}
	return out
}

// Semantics implements service.Service.
func (e *Engine) Semantics(_ context.Context, id, ref string) (model.Semantics, error) {
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
	p, err := namespace()
	if err != nil {
		return model.Semantics{}, err
	}
	out := model.Semantics{Ref: key, Kind: node.Kind}
	if node.Kind == model.NodeLD {
		out.Note = "a logical device has no class in the namespace"
		return out, nil
	}
	out.LNClass = lnClass(r.LN)
	cls, known := logicalNodeClass(p, out.LNClass)
	if !known {
		out.Note = fmt.Sprintf("the namespace (%s) does not define the logical node class %s", strings.Join(namespaceIDs(p), ", "), out.LNClass)
		return out, nil
	}
	out.Known, out.Namespace, out.Base, out.Deprecated = true, cls.Source.String(), cls.Base, cls.Deprecated
	if node.Kind == model.NodeLN {
		out.DataObjects = lnDataObjects(p, out.LNClass, children)
		return out, nil
	}

	res, err := resolver.New(p).ResolveString(key)
	if err != nil {
		out.Note = fmt.Sprintf("the namespace does not define %s: %v", strings.Join(r.Path, "."), err)
		return out, nil
	}
	if len(res.Steps) > 1 {
		do := res.Steps[1]
		out.DataObject, out.CDC, out.Presence = do.Name, do.Type, do.Presence.String()
		if c, err := p.DataObject(out.LNClass, do.Name); err == nil && c.DeclaredIn != nil {
			out.DeclaredIn = c.DeclaredIn.Name
		}
	}
	out.InstanceCDC = s.sclCDC[r.LD+"/"+r.LN+"."+r.Path[0]]
	if res.IsAttribute() {
		leaf := res.Leaf()
		out.Attribute, out.Type, out.FC = res.AttributePath(), leaf.Type, string(res.FC())
	}
	if d, err := semantic.Describe(p, key); err == nil {
		out.Quantity, out.Phase, out.Unit = string(d.Quantity), string(d.Phase), d.UnitHint.String()
	}
	return out, nil
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
// data objects against the namespace. A logical node class the namespace
// does not define is a warning, a mandatory data object (presence M) the
// node lacks an error, a data object the class does not define
// information (an extension or a newer edition), and, for an SCL session,
// a data object whose CDC differs from the namespace's a warning.
// Conditional presence (MOcond, AtLeastOne, ...) is not evaluated.
func (e *Engine) Conformance(_ context.Context, id string) (model.Conformance, error) {
	start := time.Now()
	s, err := e.session(id)
	if err != nil {
		return model.Conformance{}, err
	}
	p, err := namespace()
	if err != nil {
		return model.Conformance{}, err
	}
	out := model.Conformance{Namespaces: namespaceIDs(p), Findings: []model.Finding{}}
	add := func(f model.Finding) { addFinding(&out, f) }
	nodes := compareNodes(s)
	children := map[string][]string{}
	for _, n := range nodes {
		if n.kind == model.NodeDO {
			children[n.parent] = append(children[n.parent], n.ref[strings.LastIndex(n.ref, ".")+1:])
		}
	}
	for _, n := range nodes {
		if n.kind != model.NodeLN {
			continue
		}
		out.LogicalNodes++
		cls, known := logicalNodeClass(p, n.class)
		if !known {
			add(model.Finding{Code: model.FindingUnknownClass, Severity: model.SeverityWarning, Ref: n.ref,
				Message: fmt.Sprintf("the namespace does not define the logical node class %s", n.class)})
			continue
		}
		present := map[string]bool{}
		for _, name := range children[n.ref] {
			out.DataObjects++
			m, ok := nsdName(p, n.class, name)
			if !ok {
				add(model.Finding{Code: model.FindingUnknownDataObject, Severity: model.SeverityInfo, Ref: n.ref + "." + name,
					Message: fmt.Sprintf("%s (%s) does not define the data object %s", n.class, cls.Source, name)})
				continue
			}
			present[m] = true
			if cdc := s.sclCDC[n.ref+"."+name]; cdc != "" {
				if d, err := p.DataObject(n.class, m); err == nil && d.Type != cdc {
					add(model.Finding{Code: model.FindingCDCMismatch, Severity: model.SeverityWarning, Ref: n.ref + "." + name,
						Message: fmt.Sprintf("CDC %s, the namespace defines %s.%s as %s", cdc, n.class, m, d.Type)})
				}
			}
		}
		dos, _ := p.DataObjects(n.class)
		for _, d := range dos {
			if d.Presence.IsMandatory() && !present[d.Name] {
				add(model.Finding{Code: model.FindingMissingMandatory, Severity: model.SeverityError, Ref: n.ref + "." + d.Name,
					Message: fmt.Sprintf("%s requires the data object %s (%s, presence %s)", n.class, d.Name, d.Type, d.Presence)})
			}
		}
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out, nil
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
