// SPDX-License-Identifier: MIT

package engine

import (
	"slices"
	"strings"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

func controlSession(t *testing.T) (*Engine, *simied.IED, string, string) {
	t.Helper()
	ied := startIED(t)
	e, dir := newCommissionEngine(t)
	info, err := e.OpenSession(t.Context(), target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, info.ID, model.CapControls)
	return e, ied, info.ID, dir
}

func stepNames(res model.ControlResult) []string {
	var out []string
	for _, st := range res.Steps {
		name := st.Step
		if !st.OK {
			name += "!"
		}
		out = append(out, name)
	}
	return out
}

// Each control model runs its own sequence and sets stVal.
func TestControlModels(t *testing.T) {
	e, _, id, dir := controlSession(t)
	ctx := t.Context()
	for do, want := range map[string][]string{
		"GGIO1.SPCSO1": {model.StepOperate},
		"GGIO1.SPCSO2": {model.StepSelect, model.StepOperate},
		"GGIO1.SPCSO3": {model.StepOperate, model.StepTermination},
		"GGIO1.SPCSO4": {model.StepSelect, model.StepOperate, model.StepTermination},
	} {
		res, err := e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/" + do, Value: "true"})
		if err != nil {
			t.Fatalf("%s: %v", do, err)
		}
		if res.Outcome != model.ControlOK || !slices.Equal(stepNames(res), want) || res.CtlNum == 0 {
			t.Errorf("%s = %+v, steps %v", do, res, stepNames(res))
		}
		if res.Before == nil || res.Before.Text != "false" || res.After == nil || res.After.Text != "true" || res.Written.Text != "true" {
			t.Errorf("%s stVal %v -> %v, written %v", do, res.Before, res.After, res.Written)
		}
	}
	recs := readAuditFiles(t, dir)
	var controls []model.AuditRecord
	for _, r := range recs {
		if r.Action == model.AuditControl {
			controls = append(controls, r)
		}
	}
	if len(controls) != 4 {
		t.Fatalf("%d control records", len(controls))
	}
	for _, r := range controls {
		if r.Outcome != model.OutcomeOK || r.Written != "true" || r.Before != "false" || r.After != "true" || !strings.Contains(r.Detail, "ctlNum") {
			t.Errorf("audit %+v", r)
		}
	}
}

// With an interlock check the breaker is interlocked by GGIO1.Ind1:
// accepted, then a negative CommandTermination with the IED's cause.
func TestControlNegativeTermination(t *testing.T) {
	e, ied, id, dir := controlSession(t)
	ctx := t.Context()
	pos := simied.Domain + "/XCBR1.Pos"
	res, err := e.Control(ctx, id, model.ControlRequest{Ref: pos, Value: "false", Test: true})
	if err != nil || res.Outcome != model.ControlOK || res.CtlModel != iec61850.CtlModelSBOEnhanced.String() || !res.Test {
		t.Fatalf("open = %+v, %v", res, err)
	}
	if res.After == nil || res.After.Text != "01" {
		t.Errorf("Pos after open = %+v", res.After)
	}

	ied.Set("GGIO1$ST$Ind1$stVal", mms.NewBoolean(false))
	res, err = e.Control(ctx, id, model.ControlRequest{Ref: pos, Value: "true", InterlockCheck: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != model.ControlNegative || res.AddCause != "blocked-by-interlocking" ||
		!slices.Equal(stepNames(res), []string{model.StepSelect, model.StepOperate, model.StepTermination + "!"}) {
		t.Errorf("close while interlocked = %+v, steps %v", res, stepNames(res))
	}
	if res.After == nil || res.After.Text != "01" {
		t.Errorf("Pos after the refused close = %+v", res.After)
	}
	if !res.InterlockCheck || res.SynchroCheck {
		t.Errorf("check flags = %v, %v", res.InterlockCheck, res.SynchroCheck)
	}
	last := readAuditFiles(t, dir)
	rec := last[len(last)-1]
	if rec.Before != "off (01)" || rec.After != "off (01)" {
		t.Errorf("audit Dbpos %q -> %q, want the names with the bits", rec.Before, rec.After)
	}
	if rec.Action != model.AuditControl || rec.Outcome != model.OutcomeFailed ||
		!strings.Contains(rec.Error, "negative: blocked-by-interlocking") || !strings.Contains(rec.Detail, "termination blocked-by-interlocking") || !strings.Contains(rec.Detail, "interlock-check") {
		t.Errorf("audit %+v", rec)
	}
}

// A select another connection holds is refused with the IED's cause; the
// operate is not sent.
func TestControlRefused(t *testing.T) {
	e, ied, id, _ := controlSession(t)
	ctx := t.Context()
	other := dialIED(t, ied)
	sbow, _ := iec61850.ParseRef(simied.Domain + "/GGIO1.SPCSO4")
	if err := other.SelectWithValue(ctx, sbow, iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(true), CtlNum: 200}); err != nil {
		t.Fatal(err)
	}
	res, err := e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/GGIO1.SPCSO4", Value: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != model.ControlRefused || res.AddCause != "object-already-selected" || !slices.Equal(stepNames(res), []string{model.StepSelect + "!"}) {
		t.Errorf("select held elsewhere = %+v, steps %v", res, stepNames(res))
	}

	// Normal security: the other connection's select makes our operate fail.
	sbo, _ := iec61850.ParseRef(simied.Domain + "/GGIO1.SPCSO2")
	if _, err := other.Select(ctx, sbo); err != nil {
		t.Fatal(err)
	}
	res, err = e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/GGIO1.SPCSO2", Value: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome == model.ControlOK {
		t.Errorf("SBO while another connection holds the select = %+v, steps %v", res, stepNames(res))
	}
}

func TestControlInvalid(t *testing.T) {
	e, ied, id, _ := controlSession(t)
	ctx := t.Context()
	for name, req := range map[string]model.ControlRequest{
		"not a reference":    {Ref: "a/b/c.", Value: "true"},
		"with an FC":         {Ref: simied.Domain + "/GGIO1.SPCSO1[CO]", Value: "true"},
		"logical node":       {Ref: simied.Domain + "/GGIO1", Value: "true"},
		"status-only":        {Ref: simied.Domain + "/LLN0.Mod", Value: "1"},
		"not a boolean":      {Ref: simied.Domain + "/GGIO1.SPCSO1", Value: "maybe"},
		"no ctlModel":        {Ref: simied.Domain + "/GGIO1.Ind1", Value: "true"},
		"negative timeout":   {Ref: simied.Domain + "/GGIO1.SPCSO3", Value: "true", TimeoutMS: -1},
		"too long a timeout": {Ref: simied.Domain + "/GGIO1.SPCSO3", Value: "true", TimeoutMS: model.MaxControlTimeoutMS + 1},
	} {
		if _, err := e.Control(ctx, id, req); service.CodeOf(err) != service.CodeInvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/GGIO1.Ind1", Value: "true"}); err == nil || !strings.Contains(err.Error(), "has no ctlModel[CF]: it is not a control") {
		t.Errorf("no ctlModel: %v", err)
	}
	if _, err := e.Control(ctx, "nope", model.ControlRequest{}); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown session: %v", err)
	}

	// Without the capability.
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	grantOn(t, e, info.ID, model.CapWrite)
	if _, err := e.Control(ctx, info.ID, model.ControlRequest{Ref: simied.Domain + "/GGIO1.SPCSO1", Value: "true"}); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("without the controls capability: %v", err)
	}
}

func TestNextCtlNumSkipsZero(t *testing.T) {
	ctlNums.Store(254)
	if a, b := nextCtlNum(), nextCtlNum(); a != 255 || b != 1 {
		t.Errorf("ctlNums after 254 = %d, %d", a, b)
	}
}

// Without an interlock check the breaker closes although Ind1 is false;
// a synchrocheck is refused by the single point controls.
func TestControlCheckFlags(t *testing.T) {
	e, ied, id, _ := controlSession(t)
	ctx := t.Context()
	ied.Set("GGIO1$ST$Ind1$stVal", mms.NewBoolean(false))
	res, err := e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/XCBR1.Pos", Value: "true"})
	if err != nil || res.Outcome != model.ControlOK {
		t.Errorf("close without interlock check = %+v, %v", res, err)
	}
	res, err = e.Control(ctx, id, model.ControlRequest{Ref: simied.Domain + "/GGIO1.SPCSO1", Value: "true", SynchroCheck: true})
	if err != nil || res.Outcome != model.ControlRefused || res.AddCause != "blocked-by-synchrocheck" || !res.SynchroCheck {
		t.Errorf("operate with synchrocheck = %+v, %v", res, err)
	}
}

func TestStateText(t *testing.T) {
	for _, tc := range []struct {
		v    *model.Value
		want string
	}{
		{nil, ""},
		{&model.Value{Type: model.TypeBitString, Bits: 2, Text: "00"}, "intermediate-state (00)"},
		{&model.Value{Type: model.TypeBitString, Bits: 2, Text: "10"}, "on (10)"},
		{&model.Value{Type: model.TypeBitString, Bits: 2, Text: "11"}, "bad-state (11)"},
		{&model.Value{Type: model.TypeBitString, Bits: 3, Text: "100"}, "100"},
		{&model.Value{Type: model.TypeInteger, Text: "10"}, "10"},
	} {
		if got := stateText(tc.v); got != tc.want {
			t.Errorf("stateText(%+v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}
