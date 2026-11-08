// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"slices"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func TestInspectSCL(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.InspectSCL(ctx, model.File{Name: "demo.icd", Data: simied.ICD()})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.IEDs) != 1 || info.IEDs[0].Name != simied.IEDName || len(info.IEDs[0].AccessPoints) != 1 {
		t.Fatalf("info = %+v", info)
	}
	ap := info.IEDs[0].AccessPoints[0]
	if ap.Name != "S1" || ap.LogicalDevices != 1 || ap.Target == nil || ap.Target.Host != "127.0.0.1" ||
		ap.Target.Addressing == nil || *ap.Target.Addressing.RemotePSelector != "00000001" {
		t.Errorf("access point = %+v", ap)
	}
	for name, f := range map[string]model.File{
		"empty":   {},
		"not xml": {Data: []byte("hello")},
		"huge":    {Data: make([]byte, model.MaxSCLBytes+1)},
	} {
		if _, err := e.InspectSCL(ctx, f); service.CodeOf(err) == "" {
			t.Errorf("%s accepted", name)
		}
	}
}

// The data model imported from the IED's SCL file is the data model the IED
// serves: the same tree, with the same functional constraints. Control
// blocks (reports, logs, GOOSE, setting groups) are not built from SCL: the
// live tree has them, the SCL tree does not.
func TestOpenSCLMatchesDiscovery(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	if offline.State != model.SessionOffline || offline.SCL == nil || offline.SCL.IED != simied.IEDName || offline.Target.Host != "127.0.0.1" {
		t.Errorf("offline = %+v", offline)
	}
	if offline.Discovery.LogicalNodes != live.Discovery.LogicalNodes {
		t.Errorf("discovery offline %+v, live %+v", offline.Discovery, live.Discovery)
	}
	control := []string{"RP", "BR", "LG", "GO", "MS", "US", "SP"}
	dataOnly := func(nodes []model.Node) []model.Node {
		var out []model.Node
		for _, n := range nodes {
			fcs := slices.DeleteFunc(slices.Clone(n.FCs), func(fc string) bool { return slices.Contains(control, fc) })
			if len(n.FCs) > 0 && len(fcs) == 0 {
				continue // a control block
			}
			n.FCs = fcs
			out = append(out, n)
		}
		return out
	}
	var walk func(ref string)
	walk = func(ref string) {
		a, errA := e.Browse(ctx, live.ID, ref)
		b, errB := e.Browse(ctx, offline.ID, ref)
		if errA != nil || errB != nil {
			t.Fatalf("browse %q: %v, %v", ref, errA, errB)
		}
		a, b = dataOnly(a), dataOnly(b)
		if !slices.EqualFunc(a, b, func(x, y model.Node) bool {
			return x.Ref == y.Ref && x.Kind == y.Kind && slices.Equal(x.FCs, y.FCs) && (x.Kind == model.NodeLN || x.Children == y.Children)
		}) {
			t.Errorf("%q: live %+v\noffline %+v", ref, a, b)
			return
		}
		for _, n := range a {
			walk(n.Ref)
		}
	}
	walk("")

	if _, err := e.Read(ctx, offline.ID, model.ReadRequest{Refs: []string{simied.Domain + "/MMXU1.Hz[MX]"}}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("read offline = %v", err)
	}
	if err := e.StartPolling(ctx, offline.ID, model.PollConfig{Refs: []string{"x"}}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("poll offline = %v", err)
	}
	if res, err := e.Search(ctx, offline.ID, model.SearchQuery{LNClass: "XCBR", Kind: model.NodeDO}); err != nil || len(res.Hits) != 3 {
		t.Errorf("search offline = %+v %v", res, err)
	}
	if err := e.CloseSession(ctx, offline.ID); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSCLChoice(t *testing.T) {
	e := newEngine(t)
	ctx := t.Context()
	two := bytes.Replace(simied.ICD(), []byte("<DataTypeTemplates>"),
		[]byte(`</IED_PLACEHOLDER><DataTypeTemplates>`), 1)
	two = bytes.Replace(two, []byte("</IED_PLACEHOLDER>"), nil, 1)
	two = bytes.Replace(two, []byte("  </IED>"), []byte(`  </IED>
  <IED name="Other"><AccessPoint name="S1"><Server><LDevice inst="X"><LN0 lnClass="LLN0" inst="" lnType="LLN0_T"/></LDevice></Server></AccessPoint></IED>`), 1)
	f := model.File{Name: "two.scd", Data: two}
	if _, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: f}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("no IED chosen = %v", err)
	}
	if _, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: f, IED: "Nope"}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown IED = %v", err)
	}
	if _, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: f, IED: "Other", AccessPoint: "S9"}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown AP = %v", err)
	}
	s, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: f, IED: "Other"})
	if err != nil || s.Discovery.LogicalDevices != 1 || s.Target.Host != "" {
		t.Fatalf("Other = %+v, %v", s, err)
	}
	if lds, _ := e.Browse(ctx, s.ID, ""); len(lds) != 1 || lds[0].Ref != "OtherX" {
		t.Errorf("domain = %+v", lds)
	}
}
