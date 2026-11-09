// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"testing"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// Reading SBO selects a control with normal security (IEC 61850-8-1). The
// simulated IED's server grants the select on such a read; Observe mode
// must never send it, in any form of reference. Whether a select is held is
// seen from a second connection: its operate is refused as
// object-not-selected, not as locked-by-other-client.
func TestObserveNeverReadsControls(t *testing.T) {
	ied := startIED(t)
	e, _ := newCommissionEngine(t)
	ctx := t.Context()
	info, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	refs := []string{
		simied.Domain + "/GGIO1.SPCSO2.SBO[CO]",
		simied.Domain + "/GGIO1.SPCSO2[CO]",
		simied.Domain + "/GGIO1[CO]",
		simied.Domain + "/GGIO1.SPCSO2.Oper.ctlVal[CO]",
		simied.Domain + "/GGIO1.SPCSO2.stVal[ST]",
	}
	res, err := e.Read(ctx, info.ID, model.ReadRequest{Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range res.Values[:4] {
		if v.Error == nil || v.Error.Code != model.ErrCodeNotAllowed || v.Value != nil || v.Ref != refs[i] {
			t.Errorf("%s = %+v, want refused", refs[i], v)
		}
	}
	if v := res.Values[4]; v.Error != nil || v.Value == nil {
		t.Errorf("stVal = %+v", v)
	}

	// A data set with a CO member is not read either.
	grantOn(t, e, info.ID, model.CapDataSets)
	dsRef := simied.Domain + "/LLN0.Ctl"
	if _, err := e.CreateDataSet(ctx, info.ID, model.CreateDataSetRequest{Ref: dsRef, Members: []string{simied.Domain + "/GGIO1.SPCSO2.SBO[CO]"}}); err != nil {
		t.Fatalf("create a data set with a CO member: %v", err)
	}
	if _, err := e.ReadDataSet(ctx, info.ID, dsRef); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("ReadDataSet with a CO member = %v, want refused", err)
	}

	if cause := operateCause(t, ied); cause != iec61850.AddCauseObjectNotSelected {
		t.Errorf("operate from another connection refused with %s: the Explorer holds a select", cause)
	}
}

// The probe itself: a read of SBO on the Explorer's association does select.
func TestSBOReadSelects(t *testing.T) {
	ied := startIED(t)
	c := dialIED(t, ied)
	ref, _ := iec61850.ParseRef(simied.Domain + "/GGIO1.SPCSO2.SBO[CO]")
	if _, err := c.ReadRaw(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if cause := operateCause(t, ied); cause != iec61850.AddCauseLockedByOtherClient {
		t.Errorf("operate after another connection read SBO refused with %s, want locked-by-other-client", cause)
	}
}

func dialIED(t *testing.T, ied *simied.IED) *iec61850.Client {
	t.Helper()
	c, err := iec61850.Dial(t.Context(), ied.Addr, iec61850.DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// operateCause operates GGIO1.SPCSO2 (SBO, normal security) without a
// select on a new connection and returns the AddCause of the refusal.
func operateCause(t *testing.T, ied *simied.IED) iec61850.AddCause {
	t.Helper()
	c := dialIED(t, ied)
	ref, _ := iec61850.ParseRef(simied.Domain + "/GGIO1.SPCSO2")
	err := c.Operate(t.Context(), ref, iec61850.OperateParams{CtlVal: iec61850.BoolCtlVal(true)})
	var ce *iec61850.ControlError
	if !errors.As(err, &ce) {
		t.Fatalf("operate without select = %v, want a refusal", err)
	}
	return ce.AddCause
}
