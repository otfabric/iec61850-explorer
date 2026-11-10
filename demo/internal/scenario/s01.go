// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S01 is the station HMI starting up: it associates with three IEDs,
// identifies them, discovers their models and data sets, reads the
// types of what it displays, reads it, and polls the measurements.
var S01 = Scenario{
	File:    "01-hmi-browse-and-poll.pcapng",
	Title:   "HMI connects, discovers and polls",
	Start:   time.Date(2026, 11, 10, 8, 55, 0, 0, time.UTC),
	Seed:    1,
	Comment: "AA1 Northfield: the station HMI (AA1KA1) connects to feeder 1, the transformer and the busbar protection, discovers their models and polls measurements every 2 s.",
	Run:     runS01,
}

var pollRefs = map[string][]string{
	site.F1Name: {"MEAS/MMXU1.TotW[MX]", "MEAS/MMXU1.TotVAr[MX]", "MEAS/MMXU1.Hz[MX]", "MEAS/MMXU1.A[MX]", "CTRL/XCBR1.Pos[ST]", "CTRL/XSWI1.Pos[ST]"},
	site.T1Name: {"MEAS/MMXU2.TotW[MX]", "MEAS/MMXU2.Hz[MX]", "CTRL/YLTC1.TapPos[ST]", "CTRL/YLTC1.MotDrvTmp[MX]", "CTRL/XCBR2.Pos[ST]"},
	site.BBName: {"LD0/LPHD1.PhyHealth[ST]", "PROT/PTRC1.Tr[ST]", "PROT/RBRF1.OpEx[ST]", "PROT/RBRF2.OpEx[ST]"},
}

func refs(name string) []iec61850.Ref {
	var out []iec61850.Ref
	for _, r := range pollRefs[name] {
		out = append(out, Ref(name+r))
	}
	return out
}

func runS01(ctx context.Context, e *Env) error {
	names := []string{site.F1Name, site.T1Name, site.BBName}
	sessions := map[string]*Session{}
	for _, n := range names {
		s, err := e.Dial(ctx, n)
		if err != nil {
			return err
		}
		sessions[n] = s
		if err := discover(ctx, s); err != nil {
			return err
		}
		e.Think(850 * time.Millisecond)
	}
	for cycle := range 3 {
		e.Think(2 * time.Second)
		at := e.Clock.Now()
		e.IEDs[site.F1Name].Process(at.Add(-400*time.Millisecond), 1+0.02*float64(cycle), 0.003*float64(cycle))
		e.IEDs[site.T1Name].Process(at.Add(-300*time.Millisecond), 1+0.015*float64(cycle), 0)
		for _, n := range names {
			if _, err := sessions[n].C.ReadMultiple(ctx, refs(n)); err != nil {
				return fmt.Errorf("poll %s: %w", n, err)
			}
		}
	}
	e.Think(1500 * time.Millisecond)
	for _, n := range names {
		if err := sessions[n].Close(ctx); err != nil {
			return err
		}
	}
	return nil
}

// discover is what an HMI does once per IED: Identify, the logical
// devices and their variables, the data sets of each logical device and
// the types of the variables it shows.
func discover(ctx context.Context, s *Session) error {
	if _, err := s.C.MMS().Identify(ctx); err != nil {
		return fmt.Errorf("identify %s: %w", s.Name, err)
	}
	if _, err := s.C.TreeWithOptions(ctx, iec61850.TreeOptions{IncludeFCs: true}); err != nil {
		return fmt.Errorf("discover %s: %w", s.Name, err)
	}
	for _, ld := range s.IED.Site.LDs {
		for _, ds := range ld.LN0.DataSets {
			if _, err := s.C.GetDataSet(ctx, s.Name+ld.Inst, "LLN0$"+ds.Name); err != nil {
				return fmt.Errorf("data set %s/%s: %w", ld.Inst, ds.Name, err)
			}
		}
	}
	for _, r := range refs(s.Name) {
		if _, err := s.C.GetVariableType(ctx, r); err != nil {
			return fmt.Errorf("type %s: %w", r, err)
		}
	}
	if _, err := s.C.ReadMultiple(ctx, refs(s.Name)); err != nil {
		return fmt.Errorf("read %s: %w", s.Name, err)
	}
	return nil
}
