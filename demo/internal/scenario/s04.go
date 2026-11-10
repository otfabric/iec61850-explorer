// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S04: the protection engineer, remotely: reads feeder 1's setting group
// control block and the overcurrent settings, switches to setting group
// 2 (the feeder is isolated, its protection is set for the backfeed),
// downloads the disturbance record of the fault and reads the event log.
var S04 = Scenario{
	File:    "04-settings-files-logs.pcapng",
	Title:   "Setting groups, file transfer and the event journal",
	Start:   time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC),
	Seed:    4,
	Comment: "AA1 Northfield, 09:30: the protection engineer reads feeder 1's SGCB and overcurrent settings, activates setting group 2, lists the file store and downloads the COMTRADE record of the 09:00:10 fault, and reads the switching journal (CTRL/EventLog).",
	Run:     runS04,
}

func runS04(ctx context.Context, e *Env) error {
	f1 := e.IEDs[site.F1Name]
	// What happened at 09:20 (capture 03): QA1 and QB1 opened.
	f1.Switch("CSWI1", "XCBR1", false, time.Date(2026, 11, 10, 9, 20, 16, 962_000_000, time.UTC))
	f1.Switch("CSWI2", "XSWI1", false, time.Date(2026, 11, 10, 9, 20, 23, 422_000_000, time.UTC))

	s, err := e.Dial(ctx, site.F1Name)
	if err != nil {
		return err
	}
	c := s.C
	if _, err := c.MMS().Identify(ctx); err != nil {
		return err
	}
	prot := site.F1Name + "PROT"
	settings := []iec61850.Ref{
		Ref(prot + "/PTOC1.StrVal.setMag.f[SG]"), Ref(prot + "/PTOC1.OpDlTmms.setVal[SG]"),
		Ref(prot + "/PTOC2.StrVal.setMag.f[SG]"), Ref(prot + "/PTOC2.OpDlTmms.setVal[SG]"),
	}
	e.Think(1300 * time.Millisecond)
	if _, err := c.GetSettingGroupInfo(ctx, prot); err != nil {
		return err
	}
	if _, err := c.ReadMultiple(ctx, settings); err != nil {
		return err
	}
	e.Think(9 * time.Second)
	if err := c.SelectActiveSG(ctx, prot, 2); err != nil {
		return fmt.Errorf("select SG 2: %w", err)
	}
	f1.StampSG("PROT", e.Clock.Now().Add(-time.Millisecond).Truncate(time.Millisecond))
	e.Think(700 * time.Millisecond)
	if _, err := c.GetSettingGroupInfo(ctx, prot); err != nil {
		return err
	}
	if _, err := c.ReadMultiple(ctx, settings); err != nil {
		return err
	}

	// The disturbance record.
	e.Think(6 * time.Second)
	files, err := c.ListFiles(ctx, "")
	if err != nil {
		return fmt.Errorf("file directory: %w", err)
	}
	e.Think(2 * time.Second)
	for _, f := range files {
		if len(f.Name) > 9 && f.Name[:9] == "COMTRADE/" {
			if _, _, err := c.ReadFile(ctx, f.Name); err != nil {
				return fmt.Errorf("read %s: %w", f.Name, err)
			}
			e.Think(400 * time.Millisecond)
		}
	}

	// The switching journal.
	e.Think(5 * time.Second)
	ctrl := site.F1Name + "CTRL"
	if _, err := c.ListJournals(ctx, ctrl); err != nil {
		return err
	}
	from := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	if _, err := c.ReadJournal(ctx, ctrl, "EventLog", from, e.Clock.Now()); err != nil {
		return fmt.Errorf("read journal: %w", err)
	}
	e.Think(3 * time.Second)
	return s.Close(ctx)
}
