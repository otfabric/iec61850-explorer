// SPDX-License-Identifier: MIT

// Package scenario records the demo captures: each scenario starts the
// IEDs it needs from their CIDs on a virtual clock, runs the station
// HMI (a go-iec61850 client) against them through the capture's relay,
// and adds the GOOSE and Sampled Values frames of its story.
package scenario

import (
	"context"
	"fmt"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/demo/internal/ied"
	"github.com/otfabric/iec61850-explorer/demo/internal/netcap"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// Scenario is one capture of the demo.
type Scenario struct {
	File    string // file name in demo/captures
	Title   string
	Start   time.Time
	Seed    uint64
	Comment string
	Run     func(ctx context.Context, e *Env) error
}

// Env is a running scenario: the clock, the capture and the IEDs.
type Env struct {
	Clock *netcap.Clock
	Cap   *netcap.Capture
	IEDs  map[string]*ied.IED
	HMI   netcap.Host

	scd      *scl.SCL
	nextPort uint16
	ctlNum   uint8
}

// Host is the capture host of an IED of the substation.
func Host(d *site.IED) netcap.Host { return netcap.NewHost(d.Name, d.MAC, d.IP) }

// NewEnv starts the named IEDs on a clock at start.
func NewEnv(start time.Time, seed uint64, names ...string) (*Env, error) {
	clock := netcap.NewClock(start)
	res, err := scl.ParseBytes(site.SCD(), scl.ParseOptions{})
	if err != nil {
		return nil, err
	}
	e := &Env{Clock: clock, Cap: netcap.New(clock, seed), IEDs: map[string]*ied.IED{}, HMI: Host(site.HMI()), scd: res.Document, nextPort: 49731 + uint16(seed%97)}
	for _, n := range names {
		d, err := ied.Start(site.ByName(n), "127.0.0.1:0", ied.Options{Now: clock.Now, Pace: 30 * time.Millisecond})
		if err != nil {
			e.Close()
			return nil, err
		}
		e.IEDs[n] = d
	}
	return e, nil
}

// Close stops the IEDs.
func (e *Env) Close() {
	for _, d := range e.IEDs {
		d.Close()
	}
}

// Session is the HMI's association with one IED.
type Session struct {
	C    *iec61850.Client
	Conn *netcap.Conn
	IED  *ied.IED
	Name string
}

// Dial opens an association from the HMI to the IED, addressed as the
// SCD's Communication section says (OSI selectors, AP-title,
// AE-qualifier).
func (e *Env) Dial(ctx context.Context, name string) (*Session, error) {
	return e.dialWith(ctx, name, false)
}

// dialWith dials; muted leaves the connection out of the capture until
// Conn.Mute(false).
func (e *Env) dialWith(ctx context.Context, name string, muted bool) (*Session, error) {
	d := e.IEDs[name]
	addr, conn, err := e.Cap.Connect(e.HMI, Host(d.Site), e.nextPort, d.Addr)
	if err != nil {
		return nil, err
	}
	conn.Mute(muted)
	e.nextPort++
	var opts mms.DialOptions
	for _, sn := range e.scd.Communication.SubNetworks {
		for _, ap := range sn.ConnectedAPs {
			if ap.IEDName == name {
				if err := iec61850.ApplySCLAddress(&opts, ap); err != nil {
					return nil, err
				}
			}
		}
	}
	opts.RequestTimeout = 3 * time.Second
	c, err := iec61850.Dial(ctx, addr, iec61850.DialOptions{MMS: opts})
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", name, err)
	}
	return &Session{C: c, Conn: conn, IED: d, Name: name}, nil
}

// Close concludes the association and waits for the TCP close.
func (s *Session) Close(ctx context.Context) error {
	if err := s.C.Close(ctx); err != nil {
		return fmt.Errorf("close %s: %w", s.Name, err)
	}
	return s.Conn.WaitClosed()
}

// Ref parses an object reference.
func Ref(s string) iec61850.Ref {
	r, err := iec61850.ParseRef(s)
	if err != nil {
		panic(err)
	}
	return r
}

// Think moves the clock on by d: the operator or the HMI's poll cycle.
func (e *Env) Think(d time.Duration) { e.Clock.Advance(d) }

// Expect waits for n more units from the server after fn ran.
func (s *Session) Expect(n int, fn func()) error {
	have := s.Conn.ServerUnits()
	fn()
	return s.Conn.WaitServerUnits(have + n)
}

// Do runs fn (a request) and waits until the server sent n units since:
// its response and what the request causes (a report, a termination).
func (s *Session) Do(n int, fn func() error) error {
	have := s.Conn.ServerUnits()
	if err := fn(); err != nil {
		return err
	}
	return s.Conn.WaitServerUnits(have + n)
}

// Settle waits until the server sent nothing for 300 ms: the reports a
// change causes come about 100 ms after it.
func (s *Session) Settle() {
	n := s.Conn.ServerUnits()
	for {
		time.Sleep(300 * time.Millisecond)
		m := s.Conn.ServerUnits()
		if m == n {
			return
		}
		n = m
	}
}
