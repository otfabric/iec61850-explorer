// SPDX-License-Identifier: MIT

package engine

import (
	"testing"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// Many object references go out as one MMS Read per batch; one operation.
func TestReadBatched(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	events, _ := e.Subscribe(ctx, 0)
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	refs := []string{
		simied.Domain + "/MMXU1.Hz.mag.f[MX]",
		simied.Domain + "/MMXU1.TotW.mag.f[MX]",
		simied.Domain + "/XCBR1.OpCnt.stVal[ST]",
		simied.Domain + "/GGIO1.Ind1.stVal[ST]",
		simied.Domain + "/MMXU1.Nope[MX]",
		"bad[",
	}
	before := ied.Services()[mms.ServiceRead]
	res, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	if n := ied.Services()[mms.ServiceRead] - before; n != 1 {
		t.Errorf("%d MMS Reads for one batch", n)
	}
	if v := res.Values[2].Value; v == nil || v.Text != "17" {
		t.Errorf("OpCnt = %+v", res.Values[2])
	}
	if v := res.Values[3].Value; v == nil || v.JSON != true {
		t.Errorf("Ind1 = %+v", res.Values[3])
	}
	if e := res.Values[4].Error; e == nil || e.Code != model.ErrCodeNotFound {
		t.Errorf("unknown = %+v", res.Values[4])
	}
	if e := res.Values[5].Error; e == nil || e.Code != model.ErrCodeInvalidRef {
		t.Errorf("bad = %+v", res.Values[5])
	}
	op := waitEvent(t, events, func(ev model.Event) bool {
		return ev.Kind == model.EventOperation && ev.Operation.Action == "read"
	})
	if op.Operation.Target != "6 references" || !op.Operation.OK {
		t.Errorf("operation = %+v", op.Operation)
	}

	// More than one batch, and a logical-node read, which is not batched.
	many := []string{simied.Domain + "/MMXU1[MX]"}
	for range readBatch + 5 {
		many = append(many, simied.Domain+"/MMXU1.Hz.mag.f[MX]")
	}
	before = ied.Services()[mms.ServiceRead]
	res, err = e.Read(ctx, info.ID, model.ReadRequest{Refs: many})
	if err != nil {
		t.Fatal(err)
	}
	if n := ied.Services()[mms.ServiceRead] - before; n != 3 {
		t.Errorf("%d MMS Reads, want 1 for the LN and 2 batches", n)
	}
	for i, v := range res.Values {
		if v.Error != nil {
			t.Errorf("value %d = %+v", i, v.Error)
		}
	}
	if res.Values[0].Value == nil || res.Values[0].Value.Type != model.TypeStructure {
		t.Errorf("LN read = %+v", res.Values[0])
	}
}

func TestSearch(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	refsOf := func(r model.SearchResult) []string {
		out := make([]string, len(r.Hits))
		for i, h := range r.Hits {
			out[i] = h.Ref
		}
		return out
	}
	for name, tc := range map[string]struct {
		q    model.SearchQuery
		want string
	}{
		"text":        {model.SearchQuery{Text: "hz.MAG.F"}, simied.Domain + "/MMXU1.Hz.mag.f"},
		"class + do":  {model.SearchQuery{LNClass: "xcbr", Kind: model.NodeDO, FC: "CF"}, simied.Domain + "/XCBR1.Pos"},
		"class + fc":  {model.SearchQuery{LNClass: "MMXU", Text: "totw", Kind: model.NodeDO}, simied.Domain + "/MMXU1.TotW"},
		"ln by class": {model.SearchQuery{LNClass: "LPHD", Kind: model.NodeLN}, simied.Domain + "/LPHD1"},
	} {
		res, err := e.Search(ctx, info.ID, tc.q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(res.Hits) != 1 || res.Hits[0].Ref != tc.want {
			t.Errorf("%s = %v, want %s", name, refsOf(res), tc.want)
		}
	}
	res, err := e.Search(ctx, info.ID, model.SearchQuery{Text: simied.Domain, Limit: 3})
	if err != nil || len(res.Hits) != 3 || !res.Truncated {
		t.Errorf("limit = %v %v %v", refsOf(res), res.Truncated, err)
	}
	if _, err := e.Search(ctx, info.ID, model.SearchQuery{}); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("empty = %v", err)
	}
	if _, err := e.Search(ctx, "nope", model.SearchQuery{Text: "x"}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session = %v", err)
	}
}
