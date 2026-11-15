// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// withSCLNodes sets maxSCLNodes for the test.
func withSCLNodes(t testing.TB, n int) {
	t.Helper()
	old := maxSCLNodes
	maxSCLNodes = n
	t.Cleanup(func() { maxSCLNodes = old })
}

// wideSCL is an IED with lns logical nodes, each with one data object of
// das configuration attributes. Expanded, its access point has
// wideSCLNodes(lns, das) data objects and attributes.
func wideSCL(lns, das int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<SCL xmlns="http://www.iec.ch/61850/2003/SCL" version="2007" revision="B">
<Header id="W"/>
<IED name="W"><AccessPoint name="S1"><Server><Authentication/>
<LDevice inst="LD0"><LN0 lnClass="LLN0" inst="" lnType="L0"/>
`)
	for i := 1; i <= lns; i++ {
		fmt.Fprintf(&b, `<LN lnClass="GGIO" inst="%d" lnType="G"/>`+"\n", i)
	}
	b.WriteString(`</LDevice></Server></AccessPoint></IED>
<DataTypeTemplates>
<LNodeType id="L0" lnClass="LLN0"><DO name="Mod" type="M"/></LNodeType>
<LNodeType id="G" lnClass="GGIO"><DO name="Set" type="S"/></LNodeType>
<DOType id="M" cdc="ENC"><DA name="stVal" bType="BOOLEAN" fc="ST"/></DOType>
<DOType id="S" cdc="ING">`)
	for i := range das {
		fmt.Fprintf(&b, `<DA name="v%d" bType="INT32" fc="CF"><Val>%d</Val></DA>`, i, i)
	}
	b.WriteString("</DOType>\n</DataTypeTemplates></SCL>\n")
	return []byte(b.String())
}

// wideSCLNodes is what wideSCL expands to: LLN0.Mod with stVal, and each
// logical node's data object with its attributes.
func wideSCLNodes(lns, das int) int { return 2 + lns*(1+das) }

// deepSCL is an IED whose data object has a structure nested depth types
// deep below its first attribute: a path of depth+1 type references (the
// DOType, then depth DATypes).
func deepSCL(depth int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<SCL xmlns="http://www.iec.ch/61850/2003/SCL" version="2007" revision="B">
<Header id="D"/>
<IED name="D"><AccessPoint name="S1"><Server><Authentication/>
<LDevice inst="LD0"><LN0 lnClass="LLN0" inst="" lnType="L0"/><LN lnClass="GGIO" inst="1" lnType="G"/></LDevice>
</Server></AccessPoint></IED>
<DataTypeTemplates>
<LNodeType id="L0" lnClass="LLN0"><DO name="Mod" type="M"/></LNodeType>
<LNodeType id="G" lnClass="GGIO"><DO name="Str" type="D"/><DO name="Last" type="M"/></LNodeType>
<DOType id="M" cdc="ENC"><DA name="stVal" bType="BOOLEAN" fc="ST"/></DOType>
<DOType id="D" cdc="SPG"><DA name="a" bType="Struct" type="T1" fc="CF"/></DOType>
`)
	for i := 1; i < depth; i++ {
		fmt.Fprintf(&b, `<DAType id="T%d"><BDA name="n" bType="Struct" type="T%d"/></DAType>`+"\n", i, i+1)
	}
	fmt.Fprintf(&b, `<DAType id="T%d"><BDA name="v" bType="BOOLEAN"/></DAType>`+"\n", depth)
	b.WriteString("</DataTypeTemplates></SCL>\n")
	return []byte(b.String())
}

// An access point whose data types expand to more than maxSCLNodes data
// objects and attributes is refused; one that expands to exactly that many
// opens with all of them.
func TestSCLNodeBound(t *testing.T) {
	const lns, das = 20, 9
	nodes := wideSCLNodes(lns, das)
	data := wideSCL(lns, das)
	e := newEngine(t)
	for _, tc := range []struct {
		bound int
		open  bool
	}{
		{nodes + 1, true},
		{nodes, true},
		{nodes - 1, false},
		{1, false},
	} {
		withSCLNodes(t, tc.bound)
		info, err := e.OpenSCL(t.Context(), model.OpenSCLRequest{File: model.File{Name: "w.icd", Data: data}})
		switch {
		case tc.open && err != nil:
			t.Errorf("bound %d (%d nodes): %v", tc.bound, nodes, err)
		case tc.open && info.Discovery.Nodes != lns*(1+das)+2:
			t.Errorf("bound %d: %d nodes in the tree, want %d", tc.bound, info.Discovery.Nodes, lns*(1+das)+2)
		case !tc.open && service.CodeOf(err) != service.CodeLimitExceeded:
			t.Errorf("bound %d (%d nodes): %v, want limit_exceeded", tc.bound, nodes, err)
		}
		if err == nil {
			_ = e.CloseSession(t.Context(), info.ID)
		}
	}
}

// A data type nested deeper than go-iec61850 expands is refused, instead of
// a model that ends where the expansion stopped.
func TestSCLTypeDepthRefused(t *testing.T) {
	e := newEngine(t)
	for _, tc := range []struct {
		depth int
		open  bool
	}{
		{scl.DefaultMaxTypeDepth - 2, true},
		{scl.DefaultMaxTypeDepth - 1, true},
		{scl.DefaultMaxTypeDepth, false},
		{scl.DefaultMaxTypeDepth + 1, false},
	} {
		info, err := e.OpenSCL(t.Context(), model.OpenSCLRequest{File: model.File{Name: "d.icd", Data: deepSCL(tc.depth)}})
		if !tc.open {
			if service.CodeOf(err) != service.CodeLimitExceeded {
				t.Errorf("depth %d: %v, want limit_exceeded", tc.depth, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("depth %d: %v", tc.depth, err)
		}
		// The data object after the nested one is in the model.
		if _, err := e.Browse(t.Context(), info.ID, "DLD0/GGIO1.Last"); err != nil {
			t.Errorf("depth %d: %v", tc.depth, err)
		}
		_ = e.CloseSession(t.Context(), info.ID)
	}
}

// The walk for the configured values has the same bound.
func TestSCLValuesBound(t *testing.T) {
	const lns, das = 5, 4
	res, err := scl.ParseBytes(wideSCL(lns, das), scl.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ied := &res.Document.IEDs[0]
	// LLN0.Mod, and each data object with its attributes (stVal is not a
	// configured value).
	walk := 1 + lns*(1+das)
	for _, tc := range []struct {
		bound int
		ok    bool
	}{{walk + 1, true}, {walk, true}, {walk - 1, false}} {
		withSCLNodes(t, tc.bound)
		m, err := sclValues(res.Document, ied, &ied.AccessPoints[0])
		switch {
		case tc.ok && (err != nil || len(m.values) != lns*das):
			t.Errorf("bound %d: %d values, %v", tc.bound, len(m.values), err)
		case !tc.ok && service.CodeOf(err) != service.CodeLimitExceeded:
			t.Errorf("bound %d: %v, want limit_exceeded", tc.bound, err)
		}
	}
}

// The type of one reference is told only within the bound; beyond it the
// value is shown without names.
func TestSCLTypeOfBound(t *testing.T) {
	const das = 6
	res, err := scl.ParseBytes(wideSCL(1, das), scl.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ied := &res.Document.IEDs[0]
	ty := newSCLTypes(res.Document, ied, &ied.AccessPoints[0])
	ref := iec61850.Ref{LD: "WLD0", LN: "GGIO1", Path: []string{"Set"}, FC: "CF"}
	for _, tc := range []struct {
		bound int
		told  bool
	}{{das + 2, true}, {das + 1, true}, {das, false}} {
		withSCLNodes(t, tc.bound)
		if ts := ty.typeOf(ref); (ts != nil) != tc.told || ts != nil && len(ts.Elements) != das {
			t.Errorf("bound %d: type %+v", tc.bound, ts)
		}
	}
}

// OpenSCL ends with the request: a cancelled context opens no session.
func TestOpenSCLCancelled(t *testing.T) {
	e := newEngine(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "w.icd", Data: wideSCL(2, 2)}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenSCL with a cancelled context = %v", err)
	}
	if list, _ := e.Sessions(t.Context()); len(list) != 0 {
		t.Fatalf("sessions: %+v", list)
	}
}
