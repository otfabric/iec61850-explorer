// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S02: the HMI subscribes to feeder 1's measurements (unbuffered RCB,
// reserved) and switch positions (buffered RCB), asks for a general
// interrogation of each, receives data-change, quality-change and
// integrity reports, and releases both.
var S02 = Scenario{
	File:    "02-reports.pcapng",
	Title:   "Report subscription: reserve, enable, GI, data change, integrity, disable",
	Start:   time.Date(2026, 11, 10, 8, 57, 0, 0, time.UTC),
	Seed:    2,
	Comment: "AA1 Northfield: the station HMI subscribes to feeder 1's urcbMeas (reserved, integrity period 2 s) and brcbPos, with a general interrogation each; a load change, a disconnector position supervision alarm and an integrity report follow; both RCBs are disabled.",
	Run:     runS02,
}

func runS02(ctx context.Context, e *Env) error {
	f1 := e.IEDs[site.F1Name]
	s, err := e.Dial(ctx, site.F1Name)
	if err != nil {
		return err
	}
	c := s.C
	if _, err := c.MMS().Identify(ctx); err != nil {
		return err
	}
	meas, ctrl := site.F1Name+"MEAS", site.F1Name+"CTRL"
	for _, ds := range [][2]string{{meas, "LLN0$dsMeas"}, {ctrl, "LLN0$dsPos"}} {
		if _, err := c.GetDataSet(ctx, ds[0], ds[1]); err != nil {
			return err
		}
	}
	e.Think(420 * time.Millisecond)

	// The unbuffered RCB: read, reserve, configure, subscribe, enable.
	const urcb = "LLN0$RP$urcbMeas"
	if _, err := c.GetReportControlBlock(ctx, meas, urcb); err != nil {
		return err
	}
	if err := c.ReserveURCB(ctx, meas, urcb); err != nil {
		return err
	}
	if err := c.SetReportControlBlock(ctx, meas, urcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldIntgPd, IntgPd: 2000}); err != nil {
		return err
	}
	subM, err := c.SubscribeReport(ctx, site.F1Name+"MEAS/LLN0.urcbMeas", iec61850.SubscribeReportOptions{QueueSize: 64})
	if err != nil {
		return err
	}
	defer func() { _ = subM.Close() }()
	if err := c.SetReportControlBlock(ctx, meas, urcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
		return err
	}
	enabled := e.Clock.Now()
	if err := s.Do(2, func() error { return c.TriggerGI(ctx, meas, urcb) }); err != nil {
		return fmt.Errorf("GI urcbMeas: %w", err)
	}

	// A load change: TotW, TotVAr, Hz, A and SeqA change; go-iec61850's
	// server sends one report per changed member (it does not apply
	// BufTm), each about 100 ms later.
	e.Think(700 * time.Millisecond)
	change := e.Clock.Now()
	for _, step := range f1.LoadSteps(change, 1.04, 0.006) {
		if err := s.Expect(1, step); err != nil {
			return fmt.Errorf("dchg report: %w", err)
		}
		e.Think(150 * time.Microsecond)
	}
	// The integrity report, 2 s after enabling.
	e.Clock.AdvanceTo(enabled.Add(2 * time.Second))
	if err := s.Conn.WaitServerUnits(s.Conn.ServerUnits() + 1); err != nil {
		return fmt.Errorf("integrity report: %w", err)
	}
	drain(subM.Reports(), 16)

	// The buffered RCB: read, purge, subscribe, enable with GI.
	e.Think(600 * time.Millisecond)
	const brcb = "LLN0$BR$brcbPos"
	if _, err := c.GetReportControlBlock(ctx, ctrl, brcb); err != nil {
		return err
	}
	if err := c.SetReportControlBlock(ctx, ctrl, brcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldPurgeBuf, PurgeBuf: true}); err != nil {
		return err
	}
	subP, err := c.SubscribeReport(ctx, site.F1Name+"CTRL/LLN0.brcbPos", iec61850.SubscribeReportOptions{QueueSize: 64})
	if err != nil {
		return err
	}
	defer func() { _ = subP.Close() }()
	if err := c.SetReportControlBlock(ctx, ctrl, brcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
		return err
	}
	if err := s.Do(2, func() error { return c.TriggerGI(ctx, ctrl, brcb) }); err != nil {
		return fmt.Errorf("GI brcbPos: %w", err)
	}
	// The disconnector's position supervision: invalid for 1.4 s.
	e.Think(1100 * time.Millisecond)
	for _, q := range []iec61850.Quality{iec61850.Quality(iec61850.ValidityInvalid) | iec61850.QualityInconsistent, 0} {
		at := e.Clock.Now()
		if err := s.Expect(1, func() {
			f1.Status("CTRL", "XSWI1", "Pos", at, "q", iec61850.EncodeQuality(q))
		}); err != nil {
			return fmt.Errorf("qchg report: %w", err)
		}
		e.Think(1400 * time.Millisecond)
	}
	drain(subP.Reports(), 16)

	// Disable both, release the URCB.
	e.Think(2 * time.Second)
	if err := c.SetReportControlBlock(ctx, ctrl, brcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna}); err != nil {
		return err
	}
	if err := c.SetReportControlBlock(ctx, meas, urcb, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna}); err != nil {
		return err
	}
	if err := c.ReleaseURCB(ctx, meas, urcb); err != nil {
		return err
	}
	e.Think(300 * time.Millisecond)
	return s.Close(ctx)
}

// drain empties a report channel without blocking.
func drain(ch <-chan *iec61850.ReportIndication, max int) {
	for range max {
		select {
		case <-ch:
		default:
			return
		}
	}
}
