// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

func diffKeys(c *comparer) []string {
	out := make([]string, len(c.diffs))
	for i, d := range c.diffs {
		out[i] = d.Kind + " " + d.Ref + " " + d.Field
	}
	return out
}

func TestCompareModelsFields(t *testing.T) {
	a := []cmpNode{
		{ref: "LD", kind: model.NodeLD},
		{ref: "LD/X1", parent: "LD", kind: model.NodeLN, class: "XCBR"},
		{ref: "LD/X1.Pos", parent: "LD/X1", kind: model.NodeDO, fcs: []string{"CF", "ST"}},
		{ref: "LD/X1.Gone", parent: "LD/X1", kind: model.NodeDO},
		{ref: "LD/X1.Gone.stVal", parent: "LD/X1.Gone", kind: model.NodeChild},
	}
	b := []cmpNode{
		{ref: "LD", kind: model.NodeLD},
		{ref: "LD/X1", parent: "LD", kind: model.NodeLN, class: "CSWI"},
		{ref: "LD/X1.Pos", parent: "LD/X1", kind: model.NodeChild, fcs: []string{"ST"}},
		{ref: "LD2", kind: model.NodeLD},
		{ref: "LD2/L1", parent: "LD2", kind: model.NodeLN},
	}
	c := &comparer{}
	cat := c.models(a, b)
	want := []string{
		"only_a LD/X1.Gone ", // its stVal is not listed again
		"only_b LD2 ",        // nor LD2's logical node
		"changed LD/X1 ln_class",
		"changed LD/X1.Pos kind",
		"changed LD/X1.Pos fcs",
	}
	if got := diffKeys(c); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("differences = %v, want %v", got, want)
	}
	if cat.Items != 3 || cat.Same != 1 || cat.Changed != 2 || cat.OnlyA != 1 || cat.OnlyB != 1 {
		t.Errorf("category = %+v", cat)
	}
	if got := sharedLDs(a, b); len(got) != 1 || got[0] != "LD" {
		t.Errorf("sharedLDs = %v", got)
	}
}

func TestCompareTruncates(t *testing.T) {
	c := &comparer{}
	for range model.MaxDifferences + 3 {
		c.add(model.Difference{})
	}
	if len(c.diffs) != model.MaxDifferences || !c.truncated {
		t.Errorf("%d differences, truncated %v", len(c.diffs), c.truncated)
	}
}

func TestCompareDataSetLists(t *testing.T) {
	ds := func(ref string, members ...string) model.DataSet {
		d := model.DataSet{Ref: ref}
		for _, m := range members {
			d.Members = append(d.Members, model.DataSetMember{Ref: m})
		}
		return d
	}
	da := map[string][]model.DataSet{
		"LD": {ds("LD/LLN0.A", "x", "y"), ds("LD/LLN0.B", "x"), ds("LD/LLN0.OnlyA")},
		"L2": {ds("L2/LLN0.Z")},
	}
	db := map[string][]model.DataSet{
		"LD": {ds("LD/LLN0.A", "x", "z"), ds("LD/LLN0.B", "x"), ds("LD/LLN0.OnlyB", "q")},
	}
	c := &comparer{}
	cat := c.compareDataSets(da, db, []string{"LD", "L2"}, "B: L2: refused")
	if cat.Items != 2 || cat.Same != 1 || cat.Changed != 1 || cat.OnlyA != 1 || cat.OnlyB != 1 || cat.Reason != "B: L2: refused" {
		t.Errorf("category = %+v", cat)
	}
	if c.diffs[0].Note != "member 2 differs: y / z" {
		t.Errorf("note = %q", c.diffs[0].Note)
	}
	if got := firstMemberDiff([]string{"x"}, []string{"x", "y"}); got != "1 members / 2 members" {
		t.Errorf("firstMemberDiff = %q", got)
	}
}

func TestCompareRCBLists(t *testing.T) {
	rcb := func(ref string, confRev uint32) model.RCB {
		return model.RCB{Ref: ref, ConfRev: confRev, RptID: "id", OptFlds: []string{}, TrgOps: []string{}}
	}
	broken := rcb("LD/LLN0.RP.urcbE", 1)
	broken.Error = &model.OpError{Message: "access denied"}
	da := map[string][]model.RCB{"LD": {rcb("LD/LLN0.BR.brcbA", 1), rcb("LD/LLN0.RP.urcbE", 1), rcb("LD/LLN0.RP.urcbGone", 1)}}
	db := map[string][]model.RCB{"LD": {
		rcb("LD/LLN0.BR.brcbA01", 1), rcb("LD/LLN0.BR.brcbA02", 2), broken,
		rcb("LD/LLN0.BR.brcbAx1", 1), rcb("LD/LLN0.RP.urcbNew01", 1),
	}}
	c := &comparer{}
	cat := c.compareRCBs(da, db, []string{"LD", "L9"}, "")
	want := []string{
		"changed LD/LLN0.BR.brcbA02 conf_rev",
		"changed LD/LLN0.RP.urcbE error",
		"only_a LD/LLN0.RP.urcbGone ",
		"only_b LD/LLN0.BR.brcbAx1 ",
		"only_b LD/LLN0.RP.urcbNew01 ",
	}
	if got := diffKeys(c); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("differences = %v, want %v", got, want)
	}
	if cat.Items != 3 || cat.Same != 1 || cat.Changed != 2 || cat.OnlyA != 1 || cat.OnlyB != 2 {
		t.Errorf("category = %+v", cat)
	}
	if isDigits("") || isDigits("1a") || !isDigits("07") {
		t.Error("isDigits")
	}
	// An empty report ID (assigned by the IED) is not compared.
	x, y := rcb("r", 1), rcb("r", 1)
	x.RptID = ""
	y.RptID = "LD/LLN0$RP$r"
	if (&comparer{}).rcbFields("r", x, y) {
		t.Error("an empty SCL report ID counted as a difference")
	}
}

func TestCompareDefinitions(t *testing.T) {
	s := &session{id: "s"}
	errNo := errors.New("refused")
	list := func(_ context.Context, _, ld string) ([]int, error) {
		if ld == "bad" {
			return nil, errNo
		}
		return []int{1}, nil
	}
	got, reason := definitions(t.Context(), s, []string{"ok", "bad"}, list)
	if len(got) != 1 || reason != "bad: refused" {
		t.Errorf("partial = %v, %q", got, reason)
	}
	if got, reason := definitions(t.Context(), s, []string{"bad"}, list); got != nil || reason != "bad: refused" {
		t.Errorf("all failed = %v, %q", got, reason)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, reason := definitions(ctx, s, []string{"bad"}, list); got != nil || reason != context.Canceled.Error() {
		t.Errorf("cancelled = %v, %q", got, reason)
	}
	if joinReasons("", "b") != "b" || joinReasons("a", "") != "a" || joinReasons("a", "b") != "a; b" || prefixed("A", "") != "" || prefixed("A", "x") != "A: x" {
		t.Error("reasons")
	}
}
