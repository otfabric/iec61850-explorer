// SPDX-License-Identifier: MIT

package netcap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// Clock is the scenario's virtual clock. The IEDs read it, the recorder
// stamps the MMS units with it and moves it on by a plausible latency.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock starts a clock at t.
func NewClock(t time.Time) *Clock { return &Clock{t: t} }

// Now returns the clock's time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance moves the clock on by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// AdvanceTo moves the clock to t if t is later.
func (c *Clock) AdvanceTo(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.After(c.t) {
		c.t = t
	}
}

// Host is a station bus node.
type Host struct {
	Name string
	MAC  [6]byte
	IP   netip.Addr
}

// NewHost parses a MAC ("02:61:85:01:00:31") and an IPv4 address.
func NewHost(name, mac, ip string) Host {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		panic(err)
	}
	h := Host{Name: name, IP: netip.MustParseAddr(ip)}
	copy(h.MAC[:], hw)
	return h
}

// Capture collects the frames of one scenario.
type Capture struct {
	Clock *Clock

	mu    sync.Mutex
	rng   *rand.Rand
	l2    []Packet
	conns []*Conn
}

// New starts a capture on clock with a fixed seed.
func New(clock *Clock, seed uint64) *Capture {
	return &Capture{Clock: clock, rng: rand.New(rand.NewPCG(seed, 0x61850))}
}

// AddFrame adds an Ethernet frame (GOOSE, SV) at its time.
func (c *Capture) AddFrame(t time.Time, frame []byte, comment string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.l2 = append(c.l2, Packet{T: t, Data: append([]byte(nil), frame...), Comment: comment})
}

type evKind int

const (
	evSYN evKind = iota
	evData
	evFIN
	evRST
)

type event struct {
	t          time.Time
	fromClient bool
	kind       evKind
	data       []byte
	muted      bool
	dropped    bool
	comment    string
}

// Conn is one TCP connection of a capture, relayed on loopback.
type Conn struct {
	cap            *Capture
	client, server Host
	cport          uint16
	cisn, sisn     uint32

	mu          sync.Mutex
	cond        *sync.Cond
	events      []event
	serverUnits int
	clientUnits int
	closed      [2]bool
	muted       bool
	blackhole   bool
	dropServer  int // drop the n-th next server unit from the capture (1: the next)
	corrupt     func([]byte) []byte
	lastClient  *bool
}

// Connect relays one TCP connection to target (a real listener): it
// returns the address to dial instead. The connection is captured as
// client:cport -> server:102.
func (c *Capture) Connect(client, server Host, cport uint16, target string) (string, *Conn, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	conn := &Conn{cap: c, client: client, server: server, cport: cport, cisn: c.rng.Uint32(), sisn: c.rng.Uint32()}
	c.conns = append(c.conns, conn)
	c.mu.Unlock()
	conn.cond = sync.NewCond(&conn.mu)
	go func() {
		defer func() { _ = ln.Close() }()
		a, err := ln.Accept()
		if err != nil {
			return
		}
		b, err := net.Dial("tcp", target)
		if err != nil {
			_ = a.Close()
			return
		}
		conn.record(true, evSYN, nil)
		go conn.pump(a.(*net.TCPConn), b.(*net.TCPConn), true)
		go conn.pump(b.(*net.TCPConn), a.(*net.TCPConn), false)
	}()
	return ln.Addr().String(), conn, nil
}

// Mute stops (true) or resumes (false) capturing the connection: muted
// traffic still flows but is not in the file, as when the capture
// started late.
func (k *Conn) Mute(m bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.muted = m
}

// Blackhole stops forwarding (and capturing) what the server sends: an
// IED that stopped answering.
func (k *Conn) Blackhole() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.blackhole = true
}

// DropServerUnit leaves the n-th next unit the server sends out of the
// capture (it is still delivered): a frame the capture point lost.
func (k *Conn) DropServerUnit(n int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.dropServer = n
}

// CorruptNextClientUnit changes the next unit the client sends with fn
// before it is captured and forwarded: a client that sends a malformed
// request.
func (k *Conn) CorruptNextClientUnit(fn func([]byte) []byte) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.corrupt = fn
}

// ServerUnits is how many units the server sent so far.
func (k *Conn) ServerUnits() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.serverUnits
}

// WaitServerUnits waits until the server sent n units.
func (k *Conn) WaitServerUnits(n int) error {
	deadline := time.Now().Add(15 * time.Second)
	timer := time.AfterFunc(15*time.Second, func() { k.mu.Lock(); k.cond.Broadcast(); k.mu.Unlock() })
	defer timer.Stop()
	k.mu.Lock()
	defer k.mu.Unlock()
	for k.serverUnits < n {
		if time.Now().After(deadline) {
			return fmt.Errorf("netcap: %s: waited for %d server units, have %d", k.server.Name, n, k.serverUnits)
		}
		k.cond.Wait()
	}
	return nil
}

// WaitClosed waits until both directions ended.
func (k *Conn) WaitClosed() error {
	deadline := time.Now().Add(15 * time.Second)
	timer := time.AfterFunc(15*time.Second, func() { k.mu.Lock(); k.cond.Broadcast(); k.mu.Unlock() })
	defer timer.Stop()
	k.mu.Lock()
	defer k.mu.Unlock()
	for !k.closed[0] || !k.closed[1] {
		if time.Now().After(deadline) {
			return fmt.Errorf("netcap: %s: connection did not close", k.server.Name)
		}
		k.cond.Wait()
	}
	return nil
}

// record appends an event, stamped on the virtual clock: a response
// comes 1.2 to 4.8 ms after the request, a request 0.3 to 1.1 ms after
// the response it follows, back-to-back units 60 us apart.
func (k *Conn) record(fromClient bool, kind evKind, data []byte) {
	c := k.cap
	if kind == evFIN || kind == evRST {
		// Which side's close the relay sees first is a race: the closes
		// are placed in frames, after the last unit, client first.
		k.mu.Lock()
		defer k.mu.Unlock()
		idx := 1
		if fromClient {
			idx = 0
		}
		k.closed[idx] = true
		k.events = append(k.events, event{fromClient: fromClient, kind: kind, muted: k.muted})
		k.cond.Broadcast()
		return
	}
	c.mu.Lock()
	var d time.Duration
	switch {
	case kind == evSYN:
		d = 0
	case k.lastClient != nil && *k.lastClient != fromClient && !fromClient:
		d = time.Duration(1200+c.rng.IntN(3600)) * time.Microsecond
	case k.lastClient != nil && *k.lastClient != fromClient:
		d = time.Duration(300+c.rng.IntN(800)) * time.Microsecond
	default:
		d = time.Duration(40+c.rng.IntN(40)) * time.Microsecond
	}
	c.mu.Unlock()
	c.Clock.Advance(d)
	t := c.Clock.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	ev := event{t: t, fromClient: fromClient, kind: kind, data: data, muted: k.muted}
	if kind == evData && !fromClient {
		k.serverUnits++
		if k.dropServer > 0 {
			k.dropServer--
			if k.dropServer == 0 {
				ev.dropped = true
			}
		}
	}
	if kind == evData && fromClient {
		k.clientUnits++
	}
	fc := fromClient
	if kind == evSYN {
		// The handshake ends with the client's ACK; the server answers
		// the first request.
		fc = false
	}
	k.lastClient = &fc
	k.events = append(k.events, ev)
	k.cond.Broadcast()
}

func readTPKT(r io.Reader) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(h[2:]))
	if h[0] != 3 || n < 4 {
		return nil, fmt.Errorf("netcap: not a TPKT header % x", h)
	}
	b := make([]byte, n)
	copy(b, h[:])
	if _, err := io.ReadFull(r, b[4:]); err != nil {
		return nil, err
	}
	return b, nil
}

// pump relays one direction, one TPKT at a time, recording each before
// it is forwarded (so that the answer is never recorded before it).
func (k *Conn) pump(src, dst *net.TCPConn, fromClient bool) {
	for {
		unit, err := readTPKT(src)
		if err != nil {
			kind := evFIN
			var ne *net.OpError
			if errors.As(err, &ne) {
				kind = evRST
			}
			k.mu.Lock()
			bh := k.blackhole && !fromClient
			k.mu.Unlock()
			if !bh {
				k.record(fromClient, kind, nil)
			} else {
				k.mu.Lock()
				k.closed[1] = true
				k.cond.Broadcast()
				k.mu.Unlock()
			}
			if kind == evRST {
				_ = dst.SetLinger(0)
				_ = dst.Close()
			} else {
				_ = dst.CloseWrite()
			}
			_ = src.CloseRead()
			return
		}
		k.mu.Lock()
		bh := k.blackhole && !fromClient
		if fromClient && k.corrupt != nil {
			unit = k.corrupt(unit)
			k.corrupt = nil
		}
		k.mu.Unlock()
		if bh {
			continue
		}
		k.record(fromClient, evData, unit)
		if _, err := dst.Write(unit); err != nil {
			continue
		}
	}
}

// Packets builds the capture's frames: the TCP connections framed with
// handshakes, acknowledgements and sequence numbers, merged with the
// layer 2 frames in time order.
func (c *Capture) Packets() []Packet {
	c.mu.Lock()
	defer c.mu.Unlock()
	pkts := append([]Packet(nil), c.l2...)
	ipid := map[netip.Addr]uint16{}
	for _, k := range c.conns {
		pkts = append(pkts, k.frames(ipid, c.rng)...)
	}
	sort.SliceStable(pkts, func(i, j int) bool { return pkts[i].T.Before(pkts[j].T) })
	return pkts
}

// PCAPNG returns the capture as a pcapng file.
func (c *Capture) PCAPNG(comment string) []byte {
	return WritePCAPNG(c.Packets(), comment, "mirror0", "AA1WA1 station bus, mirror port of the station switch")
}

const (
	flFIN = 0x01
	flSYN = 0x02
	flRST = 0x04
	flPSH = 0x08
	flACK = 0x10
	mss   = 1460
)

func (k *Conn) frames(ipid map[netip.Addr]uint16, rng *rand.Rand) []Packet {
	k.mu.Lock()
	evs := append([]event(nil), k.events...)
	k.mu.Unlock()
	// The closes: after the last unit, the client's first.
	var closes []event
	var last time.Time
	kept := evs[:0:0]
	for _, ev := range evs {
		if ev.kind == evFIN || ev.kind == evRST {
			closes = append(closes, ev)
			continue
		}
		kept = append(kept, ev)
		last = ev.t
	}
	sort.SliceStable(closes, func(i, j int) bool { return closes[i].fromClient && !closes[j].fromClient })
	for i := range closes {
		closes[i].t = last.Add(300*time.Microsecond + time.Duration(i)*150*time.Microsecond)
	}
	evs = append(kept, closes...)
	next := [2]uint32{k.cisn, k.sisn} // next sequence number: client, server
	var out []Packet
	side := func(fromClient bool) int {
		if fromClient {
			return 0
		}
		return 1
	}
	emit := func(t time.Time, fromClient bool, flags byte, payload []byte, opts []byte, muted bool, comment string) {
		s := side(fromClient)
		seq, ack := next[s], next[1-s]
		next[s] += uint32(len(payload))
		if flags&(flSYN|flFIN) != 0 {
			next[s]++
		}
		if muted {
			return
		}
		src, dst, sp, dp := k.client, k.server, k.cport, uint16(102)
		if !fromClient {
			src, dst, sp, dp = k.server, k.client, 102, k.cport
		}
		if flags&flSYN != 0 && flags&flACK == 0 {
			ack = 0
		}
		if _, ok := ipid[src.IP]; !ok {
			ipid[src.IP] = uint16(rng.Uint32())
		}
		ipid[src.IP]++
		out = append(out, Packet{T: t, Data: tcpFrame(src, dst, sp, dp, seq, ack, flags, payload, opts, ipid[src.IP]), Comment: comment})
	}
	synOpts := []byte{2, 4, 0x05, 0xb4, 1, 1, 4, 2} // MSS 1460, NOP, NOP, SACK permitted
	ackDelay := 40 * time.Millisecond
	for i, ev := range evs {
		var nxt *event
		if i+1 < len(evs) {
			nxt = &evs[i+1]
		}
		switch ev.kind {
		case evSYN:
			emit(ev.t, true, flSYN, nil, synOpts, ev.muted, "")
			emit(ev.t.Add(140*time.Microsecond), false, flSYN|flACK, nil, synOpts, ev.muted, "")
			emit(ev.t.Add(210*time.Microsecond), true, flACK, nil, nil, ev.muted, "")
		case evData:
			payload := ev.data
			comment := ev.comment
			for len(payload) > 0 {
				n := min(len(payload), mss)
				emit(ev.t, ev.fromClient, flPSH|flACK, payload[:n], nil, ev.muted || ev.dropped, comment)
				payload = payload[n:]
			}
			// The receiver acknowledges with its next segment, or on its
			// own after the delayed-ACK time.
			if nxt != nil && nxt.fromClient != ev.fromClient && nxt.kind == evData && nxt.t.Sub(ev.t) < ackDelay {
				continue
			}
			at := ev.t.Add(ackDelay)
			if nxt != nil && nxt.t.Before(at) {
				at = nxt.t.Add(-15 * time.Microsecond)
				if !at.After(ev.t) {
					at = ev.t.Add(5 * time.Microsecond)
				}
			}
			emit(at, !ev.fromClient, flACK, nil, nil, ev.muted, "")
		case evFIN:
			emit(ev.t, ev.fromClient, flFIN|flACK, nil, nil, ev.muted, "")
			if nxt != nil && nxt.kind == evFIN && nxt.fromClient != ev.fromClient {
				continue
			}
			emit(ev.t.Add(60*time.Microsecond), !ev.fromClient, flACK, nil, nil, ev.muted, "")
		case evRST:
			emit(ev.t, ev.fromClient, flRST|flACK, nil, nil, ev.muted, "")
		}
	}
	return out
}

func csum(b []byte, sum uint32) uint16 {
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func tcpFrame(src, dst Host, sp, dp uint16, seq, ack uint32, flags byte, payload, opts []byte, id uint16) []byte {
	thl := 20 + len(opts)
	tcp := make([]byte, thl+len(payload))
	binary.BigEndian.PutUint16(tcp[0:], sp)
	binary.BigEndian.PutUint16(tcp[2:], dp)
	binary.BigEndian.PutUint32(tcp[4:], seq)
	binary.BigEndian.PutUint32(tcp[8:], ack)
	tcp[12] = byte(thl/4) << 4
	tcp[13] = flags
	win := uint16(64240)
	if sp == 102 {
		win = 65535
	}
	binary.BigEndian.PutUint16(tcp[14:], win)
	copy(tcp[20:], opts)
	copy(tcp[thl:], payload)
	s4, d4 := src.IP.As4(), dst.IP.As4()
	var pseudo []byte
	pseudo = append(pseudo, s4[:]...)
	pseudo = append(pseudo, d4[:]...)
	pseudo = append(pseudo, 0, 6, byte(len(tcp)>>8), byte(len(tcp)))
	var sum uint32
	for i := 0; i+1 < len(pseudo); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pseudo[i:]))
	}
	binary.BigEndian.PutUint16(tcp[16:], csum(tcp, sum))

	ip := make([]byte, 20)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:], uint16(20+len(tcp)))
	binary.BigEndian.PutUint16(ip[4:], id)
	binary.BigEndian.PutUint16(ip[6:], 0x4000) // DF
	ip[8] = 64
	ip[9] = 6
	copy(ip[12:], s4[:])
	copy(ip[16:], d4[:])
	binary.BigEndian.PutUint16(ip[10:], csum(ip, 0))

	f := make([]byte, 0, 14+len(ip)+len(tcp))
	f = append(f, dst.MAC[:]...)
	f = append(f, src.MAC[:]...)
	f = append(f, 0x08, 0x00)
	f = append(f, ip...)
	f = append(f, tcp...)
	return f
}
