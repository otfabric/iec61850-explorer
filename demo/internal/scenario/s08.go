// SPDX-License-Identifier: MIT

package scenario

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// S08 collects what goes wrong on a station bus, for the findings view.
var S08 = Scenario{
	File:  "08-problems.pcapng",
	Title: "Problems: lost segments, silence, rejects, aborts, midstream, GOOSE and SV loss",
	Start: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
	Seed:  8,
	Comment: "AA1 Northfield, 10:00-10:00:40, a bad morning on the station bus. " +
		"Feeder 2: the HMI asks for a data set the IED does not have (confirmed error), and one read response is missing from the capture (TCP gap, request unanswered). " +
		"Transformer: the IED stops answering a read; the HMI times out and aborts the association. " +
		"Busbar protection: the capture started after the association was opened (midstream). " +
		"Merging unit: the HMI sends a malformed request (the invoke ID is not an INTEGER); the IED rejects it. " +
		"GOOSE: two heartbeats of feeder 2's gcbPos lost, the merging unit restarts (stNum back to 1), one frame of feeder 1's gcbTrip duplicated. " +
		"SV: 12 samples lost, one sample repeated.",
	Run: runS08,
}

func runS08(ctx context.Context, e *Env) error {
	// The busbar protection's association was opened before the capture.
	bb, err := e.dialMuted(ctx, site.BBName)
	if err != nil {
		return err
	}
	bbRefs := refs(site.BBName)

	// Feeder 2: a wrong data set name, then a poll whose response the
	// capture lost.
	e.Think(400 * time.Millisecond)
	if _, err := bb.C.ReadMultiple(ctx, bbRefs); err != nil {
		return err
	}
	e.Think(600 * time.Millisecond)
	f2, err := e.Dial(ctx, site.F2Name)
	if err != nil {
		return err
	}
	if _, err := f2.C.MMS().Identify(ctx); err != nil {
		return err
	}
	if _, err := f2.C.GetDataSet(ctx, site.F2Name+"MEAS", "LLN0$dsMeas01"); !errors.Is(err, iec61850.ErrNotFound) {
		return fmt.Errorf("feeder 2 dsMeas01: %v, want not found", err)
	}
	f2Refs := []iec61850.Ref{Ref(site.F2Name + "MEAS/MMXU1.TotW[MX]"), Ref(site.F2Name + "MEAS/MMXU1.A[MX]"), Ref(site.F2Name + "CTRL/XCBR1.Pos[ST]")}
	for cycle := range 3 {
		e.Think(2 * time.Second)
		if cycle == 1 {
			f2.Conn.DropServerUnit(1)
		}
		if _, err := f2.C.ReadMultiple(ctx, f2Refs); err != nil {
			return err
		}
		if _, err := bb.C.ReadMultiple(ctx, bbRefs); err != nil {
			return err
		}
	}
	e.Think(700 * time.Millisecond)
	if err := f2.Close(ctx); err != nil {
		return err
	}

	// The transformer stops answering; the HMI aborts after its 3 s
	// request timeout.
	e.Think(1200 * time.Millisecond)
	t1, err := e.Dial(ctx, site.T1Name)
	if err != nil {
		return err
	}
	if _, err := t1.C.ReadMultiple(ctx, refs(site.T1Name)); err != nil {
		return err
	}
	e.Think(2 * time.Second)
	t1.Conn.Blackhole()
	if _, err := t1.C.ReadMultiple(ctx, refs(site.T1Name)); err == nil {
		return errors.New("transformer answered while silent")
	}
	e.Think(3 * time.Second)
	_ = t1.C.Abort(ctx)
	if err := t1.Conn.WaitClosed(); err != nil {
		return err
	}

	// The merging unit rejects a malformed request.
	e.Think(1500 * time.Millisecond)
	mu, err := e.Dial(ctx, site.MUName)
	if err != nil {
		return err
	}
	muRefs := []iec61850.Ref{Ref(site.MUName + "MU01/LPHD1.PhyHealth[ST]")}
	if _, err := mu.C.ReadMultiple(ctx, muRefs); err != nil {
		return err
	}
	e.Think(1 * time.Second)
	mu.Conn.CorruptNextClientUnit(corruptInvokeID)
	short, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	_, rerr := mu.C.ReadMultiple(short, muRefs)
	cancel()
	if rerr == nil {
		return errors.New("merging unit answered a malformed request")
	}
	e.Think(1500 * time.Millisecond)
	if _, err := mu.C.ReadMultiple(ctx, muRefs); err != nil {
		return fmt.Errorf("merging unit after the reject: %w", err)
	}
	e.Think(800 * time.Millisecond)
	if err := mu.Close(ctx); err != nil {
		return err
	}

	// Busbar protection: two more polls, then the end of the capture.
	for range 2 {
		e.Think(2 * time.Second)
		if _, err := bb.C.ReadMultiple(ctx, bbRefs); err != nil {
			return err
		}
	}
	e.Think(500 * time.Millisecond)
	if err := bb.Close(ctx); err != nil {
		return err
	}

	// GOOSE and SV.
	start := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	gn, err := newGOOSE(e, start)
	if err != nil {
		return err
	}
	pos2 := gn.byRef[site.GoCBRef(site.F2Name, "CTRL", "gcbPos")]
	pos2.drop[[2]uint32{pos2.stNum, pos2.sqNum + 12}] = true
	pos2.drop[[2]uint32{pos2.stNum, pos2.sqNum + 13}] = true
	trip1 := gn.byRef[site.GoCBRef(site.F1Name, "PROT", "gcbTrip")]
	trip1.dup[[2]uint32{trip1.stNum, trip1.sqNum + 20}] = true
	muStatus := site.GoCBRef(site.MUName, "MU01", "gcbStatus")
	// The merging unit restarts: silent 18 s (from 10:00:07.058), back
	// at 10:00:25.300 with stNum 1.
	mup := gn.byRef[muStatus]
	for sq := mup.sqNum + 7; sq < mup.sqNum+27; sq++ {
		mup.drop[[2]uint32{mup.stNum, sq}] = true
	}
	changes := []gooseChange{{at: start.Add(25*time.Second + 300*time.Millisecond), gocb: muStatus, restart: true}}
	if err := gn.run(e, start, start.Add(40*time.Second), changes); err != nil {
		return err
	}
	lost := map[uint16]bool{}
	for c := uint16(400); c < 412; c++ {
		lost[c] = true
	}
	_, err = svStream(e, start.Add(30*time.Second), start.Add(30*time.Second+300*time.Millisecond), lost, map[uint16]bool{800: true})
	return err
}

// dialMuted opens an association whose start the capture missed.
func (e *Env) dialMuted(ctx context.Context, name string) (*Session, error) {
	e.Clock.Advance(-41 * time.Minute) // opened at 09:19, long before the capture
	defer e.Clock.Advance(41 * time.Minute)
	s, err := e.dialWith(ctx, name, true)
	if err != nil {
		return nil, err
	}
	if _, err := s.C.MMS().Identify(ctx); err != nil {
		return nil, err
	}
	s.Conn.Mute(false)
	return s, nil
}

// corruptInvokeID turns the invoke ID of a confirmed request (INTEGER,
// tag 0x02) into an OCTET STRING (tag 0x04): the PDU no longer decodes.
func corruptInvokeID(unit []byte) []byte {
	out := append([]byte(nil), unit...)
	// presentation-context-identifier 3, single-ASN1-type [0], then the
	// MMS confirmed-RequestPDU [0] with its invokeID.
	i := bytes.Index(out, []byte{0x02, 0x01, 0x03, 0xa0})
	if i < 0 {
		return out
	}
	j := i + 4
	j += berLenSize(out[j:])
	if j < len(out) && out[j] == 0xa0 {
		j++
		j += berLenSize(out[j:])
		if j < len(out) && out[j] == 0x02 {
			out[j] = 0x04
		}
	}
	return out
}

func berLenSize(b []byte) int {
	if len(b) == 0 || b[0] < 0x80 {
		return 1
	}
	return 1 + int(b[0]&0x7f)
}
