// SPDX-License-Identifier: MIT

package scenario

import (
	"fmt"
	"sort"
	"time"

	"github.com/otfabric/go-iec61850/goose"
	"github.com/otfabric/go-iec61850/sv"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/demo/internal/fault"
	"github.com/otfabric/iec61850-explorer/demo/internal/ied"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// GOOSE publishers and the SV stream are encoded with go-iec61850's goose
// and sv packages from the control blocks the SCD defines
// (goose.ControlBlocksFromSCL, sv.ControlBlocksFromSCL): destination,
// APPID, VLAN, gocbRef, datSet, goID, confRev, MinTime and MaxTime. The
// timing follows go-iec61850's goose.Publisher (first retransmission
// after MinTime, the interval doubling up to MaxTime, the heartbeat at
// MaxTime) on the scenario's clock instead of the host's.

// gpub is one GOOSE publisher.
type gpub struct {
	cfg      goose.PublisherConfig
	stNum    uint32
	sqNum    uint32
	tChange  time.Time
	values   []*mms.Value
	interval time.Duration
	next     time.Time
	// drop and dup leave frames out or send them twice (scenario 08).
	drop map[[2]uint32]bool
	dup  map[[2]uint32]bool
}

// gooseChange is a new state of a publisher at a time.
type gooseChange struct {
	at     time.Time
	gocb   string
	values func(old []*mms.Value) []*mms.Value
	// restart: the publisher restarts (stNum 1, sqNum 0) at this time.
	restart bool
}

// gooseNet holds the station's publishers.
type gooseNet struct {
	pubs  []*gpub
	byRef map[string]*gpub
}

// Initial states: the stNum and the time of the last change of each
// publisher (from the IED's start), and the phase of its heartbeat.
var gooseInit = map[string]struct {
	stNum uint32
	phase time.Duration
}{
	site.GoCBRef(site.F1Name, "PROT", "gcbTrip"):   {4, 137 * time.Millisecond},
	site.GoCBRef(site.F1Name, "CTRL", "gcbPos"):    {9, 412 * time.Millisecond},
	site.GoCBRef(site.F2Name, "PROT", "gcbTrip"):   {2, 655 * time.Millisecond},
	site.GoCBRef(site.F2Name, "CTRL", "gcbPos"):    {7, 871 * time.Millisecond},
	site.GoCBRef(site.BBName, "PROT", "gcbBF"):     {1, 293 * time.Millisecond},
	site.GoCBRef(site.MUName, "MU01", "gcbStatus"): {3, 58 * time.Millisecond},
}

// newGOOSE builds the publishers at time from: each has been publishing
// since its IED's start (heartbeat every MaxTime).
func newGOOSE(e *Env, from time.Time) (*gooseNet, error) {
	cbs, err := goose.ControlBlocksFromSCL(e.scd)
	if err != nil {
		return nil, err
	}
	n := &gooseNet{byRef: map[string]*gpub{}}
	for _, cb := range cbs {
		cfg, err := cb.PublisherConfig()
		if err != nil {
			return nil, err
		}
		d := site.ByName(cb.IEDName)
		cfg.Src = Host(d).MAC
		init, ok := gooseInit[cfg.GocbRef]
		if !ok {
			return nil, fmt.Errorf("goose: no initial state for %s", cfg.GocbRef)
		}
		boot := from.Add(-ied.Boot).Truncate(time.Millisecond)
		p := &gpub{cfg: cfg, stNum: init.stNum, tChange: boot, interval: cfg.MaxTime,
			drop: map[[2]uint32]bool{}, dup: map[[2]uint32]bool{}}
		// The heartbeat count since the last change.
		p.sqNum = uint32(from.Sub(boot)/cfg.MaxTime) + uint32(init.phase/time.Millisecond)%23
		p.next = from.Add(init.phase)
		p.values = initialValues(cb, boot)
		dv := cb.DefaultValues()
		for i := range dv {
			if !goose.SameShape(dv[i], p.values[i]) {
				return nil, fmt.Errorf("goose: %s member %d: value does not match the data set", cfg.GocbRef, i)
			}
		}
		n.pubs = append(n.pubs, p)
		n.byRef[cfg.GocbRef] = p
	}
	return n, nil
}

// initialValues are the values of a data set in the normal state.
func initialValues(cb goose.ControlBlock, boot time.Time) []*mms.Value {
	var out []*mms.Value
	for _, m := range cb.Members {
		switch {
		case m.Ref[len(m.Ref)-2:] == ".q" || m.Type.Type == mms.ValueTypeBitString && m.Type.Size == 13:
			out = append(out, ied.Good)
		case m.Type.Type == mms.ValueTypeBitString && m.Type.Size == 2:
			out = append(out, ied.Dbpos(true))
		case m.Type.Type == mms.ValueTypeUTCTime:
			out = append(out, ied.TS(boot))
		case m.Type.Type == mms.ValueTypeBoolean:
			out = append(out, mms.NewBoolean(false))
		default: // PhyHealth, Beh: 1 (Ok, on)
			out = append(out, mms.NewInteger(1))
		}
	}
	return out
}

func (p *gpub) frame() ([]byte, error) {
	c := &p.cfg
	return goose.Encode(&goose.Frame{
		Dst: c.Dst, Src: c.Src, VLAN: c.VLAN, AppID: c.AppID,
		PDU: goose.PDU{
			GocbRef: c.GocbRef, TimeAllowedToLive: uint32(c.TimeAllowedToLive / time.Millisecond),
			DatSet: c.DatSet, GoID: c.GoID, T: p.tChange, TimeQuality: 0x0a,
			StNum: p.stNum, SqNum: p.sqNum, ConfRev: c.ConfRev,
			NumDatSetEntries: uint32(len(p.values)), AllData: p.values,
		},
	})
}

// run adds the frames of every publisher from..to with the changes.
func (n *gooseNet) run(e *Env, from, to time.Time, changes []gooseChange) error {
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].at.Before(changes[j].at) })
	send := func(p *gpub, at time.Time) error {
		key := [2]uint32{p.stNum, p.sqNum}
		if p.drop[key] {
			return nil
		}
		b, err := p.frame()
		if err != nil {
			return err
		}
		e.Cap.AddFrame(at, b, "")
		if p.dup[key] {
			e.Cap.AddFrame(at.Add(3*time.Microsecond), b, "")
		}
		return nil
	}
	for {
		// The next event: a change or a publisher's next frame.
		var np *gpub
		for _, p := range n.pubs {
			if np == nil || p.next.Before(np.next) {
				np = p
			}
		}
		if len(changes) > 0 && !changes[0].at.After(np.next) {
			ch := changes[0]
			changes = changes[1:]
			p := n.byRef[ch.gocb]
			if p == nil {
				return fmt.Errorf("goose: unknown GoCB %s", ch.gocb)
			}
			p.tChange = ch.at
			if ch.restart {
				p.stNum, p.sqNum = 1, 0
			} else {
				p.stNum++
				p.sqNum = 0
			}
			if ch.values != nil {
				p.values = ch.values(p.values)
			}
			p.interval = p.cfg.MinTime
			p.next = ch.at.Add(p.interval)
			if !ch.at.Before(from) && ch.at.Before(to) {
				if err := send(p, ch.at); err != nil {
					return err
				}
			}
			continue
		}
		if !np.next.Before(to) {
			return nil
		}
		at := np.next
		np.sqNum++
		np.interval = min(2*np.interval, np.cfg.MaxTime)
		np.next = at.Add(np.interval)
		if !at.Before(from) {
			if err := send(np, at); err != nil {
				return err
			}
		}
	}
}

// set returns a values function that sets member i to v (and its time
// stamp member, if the data set has one right after the quality).
func set(kv ...any) func([]*mms.Value) []*mms.Value {
	return func(old []*mms.Value) []*mms.Value {
		out := append([]*mms.Value(nil), old...)
		for k := 0; k+1 < len(kv); k += 2 {
			out[kv[k].(int)] = kv[k+1].(*mms.Value)
		}
		return out
	}
}

// faultGOOSE are the state changes of the fault at T0.
func faultGOOSE() []gooseChange {
	t0 := fault.T0
	f1trip := site.GoCBRef(site.F1Name, "PROT", "gcbTrip")
	bf := site.GoCBRef(site.BBName, "PROT", "gcbBF")
	return []gooseChange{
		{at: t0.Add(fault.TrAt), gocb: f1trip, values: set(0, mms.NewBoolean(true))},
		{at: t0.Add(fault.BFOpAt), gocb: bf, values: set(0, mms.NewBoolean(true))},
		{at: t0.Add(fault.TrResetAt), gocb: f1trip, values: set(0, mms.NewBoolean(false))},
		{at: t0.Add(fault.TrResetAt + 10*time.Millisecond), gocb: bf, values: set(0, mms.NewBoolean(false))},
	}
}

// --- Sampled Values ----------------------------------------------------

// svStream adds the merging unit's 9-2LE stream from..to (one ASDU per
// frame, 4000 frames per second, smpCnt aligned to the second, globally
// synchronised). lost leaves the samples whose smpCnt is in it out; dup
// sends them twice.
func svStream(e *Env, from, to time.Time, lost, dup map[uint16]bool) (int, error) {
	cbs, err := sv.ControlBlocksFromSCL(e.scd)
	if err != nil {
		return 0, err
	}
	if len(cbs) != 1 {
		return 0, fmt.Errorf("sv: %d SVCBs in the SCD", len(cbs))
	}
	cfg, err := cbs[0].PublisherConfig()
	if err != nil {
		return 0, err
	}
	src := Host(site.MU()).MAC
	const period = 250 * time.Microsecond
	start := from.Truncate(period)
	n := 0
	var buf []byte
	for t := start; t.Before(to); t = t.Add(period) {
		cnt := uint16(t.Sub(t.Truncate(time.Second)) / period)
		amps, volts := fault.Instant(t.Sub(fault.T0))
		var s sv.LESample
		for i := range 4 {
			s.Current[i] = sv.Point{Value: sv.AmpsCount(amps[i])}
			s.Voltage[i] = sv.Point{Value: sv.VoltsCount(volts[i])}
		}
		if lost[cnt] {
			continue
		}
		buf = s.Append(buf[:0])
		b, err := sv.Encode(&sv.Frame{
			Dst: cfg.Dst, Src: src, VLAN: cfg.VLAN, AppID: cfg.AppID,
			PDU: sv.PDU{ASDUs: []sv.ASDU{{SvID: cfg.SvID, SmpCnt: cnt, ConfRev: cfg.ConfRev, SmpSynch: sv.SmpSynchGlobal, Sample: buf}}},
		})
		if err != nil {
			return 0, err
		}
		// The merging unit sends each sample 30 us after its instant.
		e.Cap.AddFrame(t.Add(30*time.Microsecond), b, "")
		n++
		if dup[cnt] {
			e.Cap.AddFrame(t.Add(34*time.Microsecond), b, "")
			n++
		}
	}
	return n, nil
}
