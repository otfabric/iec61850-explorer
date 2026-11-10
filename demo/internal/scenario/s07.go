// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/demo/internal/fault"
	"github.com/otfabric/iec61850-explorer/demo/internal/ied"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S07, the hero capture: the whole fault on the station bus. The HMI has
// just subscribed to the protection events of feeder 1, the busbar
// protection and the transformer; the merging unit's samples show the
// fault, feeder 1 trips by GOOSE and reports it, its breaker does not
// open, the busbar protection's breaker failure GOOSE trips the
// transformer's LV incomer, the reports follow, and the HMI reads the
// result.
var S07 = Scenario{
	File:    "07-station-fault.pcapng",
	Title:   "The station during the fault: SV, GOOSE and MMS together",
	Start:   fault.T0.Add(-6 * time.Second),
	Seed:    7,
	Comment: "AA1 Northfield, 09:00:04-09:00:13. MMS: the station HMI (10.20.1.10) subscribes to brcbTrip of feeder 1, the busbar protection and the transformer and to the transformer's urcbTap, then receives the fault's reports and reads the positions. GOOSE: all six publishers, the trip of feeder 1 at 09:00:10.064 and the breaker failure at 09:00:10.216. SV: the merging unit's 9-2LE stream from 09:00:09.500 to 09:00:11.000 only (the disturbance trigger of the mirror port).",
	Run:     runS07,
}

func runS07(ctx context.Context, e *Env) error {
	t0 := fault.T0
	f1, t1 := e.IEDs[site.F1Name], e.IEDs[site.T1Name]
	sess := map[string]*Session{}
	subscribe := func(name, ld, rcb string, urcb bool) error {
		s := sess[name]
		dom := name + ld
		if urcb {
			if err := s.C.ReserveURCB(ctx, dom, rcb); err != nil {
				return err
			}
		}
		if err := s.C.SetReportControlBlock(ctx, dom, rcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
			return fmt.Errorf("enable %s/%s: %w", dom, rcb, err)
		}
		return s.Do(2, func() error { return s.C.TriggerGI(ctx, dom, rcb) })
	}
	for _, n := range []string{site.F1Name, site.BBName, site.T1Name} {
		s, err := e.Dial(ctx, n)
		if err != nil {
			return err
		}
		sess[n] = s
		if err := subscribe(n, "PROT", "LLN0$BR$brcbTrip", false); err != nil {
			return err
		}
		e.Think(310 * time.Millisecond)
	}
	if err := subscribe(site.T1Name, "CTRL", "LLN0$RP$urcbTap", true); err != nil {
		return err
	}

	// The fault, in the order of its time stamps; every change is one
	// report to the HMI.
	acd := func(on bool) []any {
		dir := int64(0)
		if on {
			dir = 1
		}
		b, d := mms.NewBoolean(on), mms.NewInteger(dir)
		return []any{"dirGeneral", d, "phsB", b, "dirPhsB", d, "phsC", b, "dirPhsC", d, "general", b}
	}
	act := func(on bool) []any {
		b := mms.NewBoolean(on)
		return []any{"phsB", b, "phsC", b, "general", b}
	}
	gen := func(on bool) []any { return []any{"general", mms.NewBoolean(on)} }
	steps := []struct {
		dt   time.Duration
		ied  string
		ld   string
		ln   string
		do   string
		vals []any
	}{
		{fault.StrAt, site.F1Name, "PROT", "PTOC1", "Str", acd(true)},
		{fault.StrAt, site.F1Name, "PROT", "PTOC2", "Str", acd(true)},
		{fault.OpAt, site.F1Name, "PROT", "PTOC2", "Op", act(true)},
		{fault.TrAt, site.F1Name, "PROT", "PTRC1", "Tr", gen(true)},
		{fault.BFOpAt, site.BBName, "PROT", "RBRF1", "OpEx", gen(true)},
		{fault.BFOpAt, site.BBName, "PROT", "PTRC1", "Tr", gen(true)},
		{fault.LVTripAt, site.T1Name, "PROT", "PTRC1", "Tr", gen(true)},
		{fault.LVOpenAt, site.T1Name, "CTRL", "XCBR2", "Pos", []any{"stVal", nil}},
		{fault.StrResetAt, site.F1Name, "PROT", "PTOC1", "Str", acd(false)},
		{fault.StrResetAt, site.F1Name, "PROT", "PTOC2", "Str", acd(false)},
		{fault.TrResetAt, site.F1Name, "PROT", "PTOC2", "Op", act(false)},
		{fault.TrResetAt, site.F1Name, "PROT", "PTRC1", "Tr", gen(false)},
		{fault.TrResetAt, site.T1Name, "PROT", "PTRC1", "Tr", gen(false)},
		{fault.TrResetAt + 10*time.Millisecond, site.BBName, "PROT", "RBRF1", "OpEx", gen(false)},
		{fault.TrResetAt + 10*time.Millisecond, site.BBName, "PROT", "PTRC1", "Tr", gen(false)},
	}
	for _, st := range steps {
		at := t0.Add(st.dt)
		e.Clock.AdvanceTo(at)
		d := e.IEDs[st.ied]
		vals := st.vals
		if st.do == "Pos" {
			vals = []any{"stVal", ied.Dbpos(false)}
		}
		if err := sess[st.ied].Expect(1, func() { d.Status(st.ld, st.ln, st.do, at, vals...) }); err != nil {
			return fmt.Errorf("%s %s.%s: %w", st.ied, st.ln, st.do, err)
		}
	}
	// The measurements after the clearing (not subscribed: no reports).
	cleared := t0.Add(fault.ClearAt)
	f1.Process(cleared, 0, 0)
	t1.Process(cleared, 0, 0)

	// The HMI reads what happened.
	e.Clock.AdvanceTo(t0.Add(1200 * time.Millisecond))
	reads := map[string][]string{
		site.F1Name: {"CTRL/XCBR1.Pos[ST]", "PROT/PTRC1.Tr[ST]", "MEAS/MMXU1.A[MX]"},
		site.T1Name: {"CTRL/XCBR2.Pos[ST]", "PROT/PTRC1.Tr[ST]", "MEAS/MMXU2.TotW[MX]"},
		site.BBName: {"PROT/RBRF1.OpEx[ST]", "PROT/RBRF1.OpIn[ST]"},
	}
	for _, n := range []string{site.F1Name, site.T1Name, site.BBName} {
		var refs []iec61850.Ref
		for _, r := range reads[n] {
			refs = append(refs, Ref(n+r))
		}
		if _, err := sess[n].C.ReadMultiple(ctx, refs); err != nil {
			return err
		}
		e.Think(230 * time.Millisecond)
	}
	e.Clock.AdvanceTo(t0.Add(2500 * time.Millisecond))
	for _, n := range []string{site.F1Name, site.BBName, site.T1Name} {
		if err := sess[n].Close(ctx); err != nil {
			return err
		}
	}

	// GOOSE and SV of the same seconds.
	gn, err := newGOOSE(e, t0.Add(-6*time.Second))
	if err != nil {
		return err
	}
	if err := gn.run(e, t0.Add(-6*time.Second), t0.Add(3*time.Second), faultGOOSE()); err != nil {
		return err
	}
	_, err = svStream(e, t0.Add(-500*time.Millisecond), t0.Add(time.Second), nil, nil)
	return err
}
