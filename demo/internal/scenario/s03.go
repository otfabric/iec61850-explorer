// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"errors"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S03: after the fault, the operator isolates feeder 1. Opening the
// busbar disconnector QB1 first is refused (interlocked: the breaker is
// closed), the breaker QA1 opens (SBO with enhanced security: select,
// operate, CommandTermination), then QB1 opens.
var S03 = Scenario{
	File:    "03-control-sbo.pcapng",
	Title:   "Controls: SBO with enhanced security, an interlocked switch refuses",
	Start:   time.Date(2026, 11, 10, 9, 20, 0, 0, time.UTC),
	Seed:    3,
	Comment: "AA1 Northfield, 09:20: the operator isolates feeder 1 from the station HMI. Opening disconnector QB1 (CSWI2) is refused with LastApplError blocked-by-interlocking; breaker QA1 (CSWI1) opens with select-before-operate and enhanced security (SBOw, Oper, CommandTermination); then QB1 opens. brcbPos reports every position.",
	Run:     runS03,
}

// hmiOrigin is the originator of the HMI's commands.
var hmiOrigin = mms.NewStructure([]*mms.Value{mms.NewInteger(int64(iec61850.OrCatStationControl)), mms.NewOctetString([]byte("AA1KA1 operator 1"))})

// oper builds an Oper (or SBOw) value as IEC 61850-8-1 maps it. The
// demo builds it itself, with T from the scenario's clock: go-iec61850
// v1.16.0's control client stamps T with the host's clock, which would
// make the captures differ on every run.
func oper(ctlVal bool, ctlNum uint8, t time.Time, check byte) *mms.Value {
	return mms.NewStructure([]*mms.Value{
		mms.NewBoolean(ctlVal), hmiOrigin, mms.NewUnsigned(uint64(ctlNum)),
		mms.NewUTCTimeWithQuality(t, 0x0a), mms.NewBoolean(false), mms.NewBitStringWithLength([]byte{check}, 2),
	})
}

func cancelValue(ctlVal bool, ctlNum uint8, t time.Time) *mms.Value {
	return mms.NewStructure([]*mms.Value{
		mms.NewBoolean(ctlVal), hmiOrigin, mms.NewUnsigned(uint64(ctlNum)),
		mms.NewUTCTimeWithQuality(t, 0x0a), mms.NewBoolean(false),
	})
}

const checkInterlock = 0x40 // Check: interlock-check (bit 1)

// sbo runs select-before-operate with enhanced security on obj (an
// object reference to a controllable data object): SBOw, then Oper; the
// server answers the operate and sends the CommandTermination (or, when
// it refuses, a LastApplError before the negative response, and the HMI
// cancels the selection). It returns the operate's error.
func (e *Env) sbo(ctx context.Context, s *Session, obj string, ctlVal bool) error {
	e.ctlNum++
	n := e.ctlNum
	if err := s.C.Write(ctx, Ref(obj+".SBOw[CO]"), oper(ctlVal, n, e.Clock.Now(), checkInterlock)); err != nil {
		return fmt.Errorf("select %s: %w", obj, err)
	}
	e.Think(850 * time.Millisecond) // the operator confirms
	err := s.C.Write(ctx, Ref(obj+".Oper[CO]"), oper(ctlVal, n, e.Clock.Now(), checkInterlock))
	s.Settle()
	if err != nil {
		e.Think(120 * time.Millisecond)
		if cerr := s.C.Write(ctx, Ref(obj+".Cancel[CO]"), cancelValue(ctlVal, n, e.Clock.Now())); cerr != nil {
			err = errors.Join(err, fmt.Errorf("cancel: %w", cerr))
		}
	}
	return err
}

func runS03(ctx context.Context, e *Env) error {
	f1 := site.F1Name
	s, err := e.Dial(ctx, f1)
	if err != nil {
		return err
	}
	c := s.C
	ctrl := f1 + "CTRL"
	// The HMI enables the switch position reports with a GI.
	if err := c.SetReportControlBlock(ctx, ctrl, "LLN0$BR$brcbPos", iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
		return err
	}
	if err := s.Do(2, func() error { return c.TriggerGI(ctx, ctrl, "LLN0$BR$brcbPos") }); err != nil {
		return err
	}
	e.Think(3200 * time.Millisecond)
	state := []iec61850.Ref{
		Ref(ctrl + "/XCBR1.Pos.stVal[ST]"), Ref(ctrl + "/XSWI1.Pos.stVal[ST]"),
		Ref(ctrl + "/CILO2.EnaOpn.stVal[ST]"), Ref(ctrl + "/CILO2.EnaCls.stVal[ST]"),
		Ref(ctrl + "/CSWI2.Pos.ctlModel[CF]"), Ref(ctrl + "/CSWI2.Pos.sboTimeout[CF]"),
	}
	if _, err := c.ReadMultiple(ctx, state); err != nil {
		return err
	}
	e.Think(4100 * time.Millisecond)

	// 1. Open QB1 while QA1 is closed: refused.
	err = e.sbo(ctx, s, ctrl+"/CSWI2.Pos", false)
	var ce *iec61850.ControlError
	if err == nil {
		return errors.New("QB1 opened although QA1 is closed")
	}
	if errors.As(err, &ce) && ce.AddCause != iec61850.AddCauseBlockedByInterlocking {
		return fmt.Errorf("QB1 refused with %v, want blocked by interlocking", ce.AddCause)
	}
	e.Think(2600 * time.Millisecond)
	if _, err := c.ReadMultiple(ctx, state[:4]); err != nil {
		return err
	}

	// 2. Open QA1.
	e.Think(5200 * time.Millisecond)
	if err := e.sbo(ctx, s, ctrl+"/CSWI1.Pos", false); err != nil {
		return fmt.Errorf("open QA1: %w", err)
	}
	e.Think(1900 * time.Millisecond)
	if _, err := c.ReadMultiple(ctx, state[:4]); err != nil {
		return err
	}

	// 3. Open QB1, now released.
	e.Think(3700 * time.Millisecond)
	if err := e.sbo(ctx, s, ctrl+"/CSWI2.Pos", false); err != nil {
		return fmt.Errorf("open QB1: %w", err)
	}
	e.Think(3 * time.Second)
	if _, err := c.ReadMultiple(ctx, state[:4]); err != nil {
		return err
	}
	e.Think(1500 * time.Millisecond)
	if err := c.SetReportControlBlock(ctx, ctrl, "LLN0$BR$brcbPos", iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna}); err != nil {
		return err
	}
	e.Think(400 * time.Millisecond)
	return s.Close(ctx)
}
