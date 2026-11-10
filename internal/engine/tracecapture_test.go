// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-mms"
)

// The test-only capture: a TCP relay between the Explorer and the
// simulated IED that records what each side sent, and a writer of classic
// pcap files that turns the recording into Ethernet/IPv4/TCP packets with
// consistent sequence numbers. Dropping a recorded chunk from the file
// while its sequence numbers still count makes a gap in the TCP stream,
// as a lossy capture point would.

// chunk is what one side sent in one read of the relay.
type chunk struct {
	conn     int
	fromPeer bool // sent by the IED (server)
	at       time.Time
	data     []byte
}

type relay struct {
	addr     string
	upstream string
	ln       net.Listener

	mu     sync.Mutex
	chunks []chunk
	conns  int
	wg     sync.WaitGroup
}

// startRelay relays 127.0.0.1:<port> to upstream and records both
// directions.
func startRelay(t *testing.T, upstream string) *relay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &relay{addr: ln.Addr().String(), upstream: upstream, ln: ln}
	go r.accept()
	t.Cleanup(func() {
		_ = ln.Close()
		r.wg.Wait()
	})
	return r
}

func (r *relay) accept() {
	for {
		c, err := r.ln.Accept()
		if err != nil {
			return
		}
		up, err := net.Dial("tcp", r.upstream)
		if err != nil {
			_ = c.Close()
			continue
		}
		r.mu.Lock()
		id := r.conns
		r.conns++
		r.mu.Unlock()
		r.wg.Add(2)
		go r.pipe(id, c, up, false)
		go r.pipe(id, up, c, true)
	}
}

func (r *relay) pipe(id int, from, to net.Conn, fromPeer bool) {
	defer r.wg.Done()
	buf := make([]byte, 32<<10)
	for {
		n, err := from.Read(buf)
		if n > 0 {
			// Record before forwarding, so that the order of the recording
			// is the order on the wire.
			r.mu.Lock()
			r.chunks = append(r.chunks, chunk{conn: id, fromPeer: fromPeer, at: time.Now(), data: bytes.Clone(buf[:n])})
			r.mu.Unlock()
			if _, werr := to.Write(buf[:n]); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if tc, ok := to.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	} else {
		_ = to.Close()
	}
	_ = from.Close()
}

// recording returns the chunks so far.
func (r *relay) recording() []chunk {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]chunk(nil), r.chunks...)
}

// pdusOf decodes the MMS PDUs of a chunk: each TPKT carries a COTP data
// TPDU (3 bytes) and one whole session unit, as go-mms sends small units.
func pdusOf(c chunk) []*mms.CapturedPDU {
	var out []*mms.CapturedPDU
	b := c.data
	for len(b) >= 7 && b[0] == 3 {
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if n < 7 || n > len(b) {
			break
		}
		if b[5] == 0xf0 { // DT
			if cap, err := mms.DecodeTSDU(b[7:n]); err == nil && cap.PDU != nil {
				out = append(out, cap.PDU)
			}
		}
		b = b[n:]
	}
	return out
}

// pcapOf writes the recording as a classic pcap file (Ethernet, IPv4,
// TCP): connection i runs between 10.0.0.1:(50000+i) and 10.0.0.2:port,
// with a handshake, the recorded data and a closing FIN exchange. Chunks
// whose index drop marks are left out, their sequence numbers kept.
func pcapOf(chunks []chunk, port uint16, drop map[int]bool) []byte {
	var out bytes.Buffer
	hdr := make([]byte, 24)
	binary.LittleEndian.PutUint32(hdr[0:], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(hdr[4:], 2)
	binary.LittleEndian.PutUint16(hdr[6:], 4)
	binary.LittleEndian.PutUint32(hdr[16:], 262144)
	binary.LittleEndian.PutUint32(hdr[20:], 1) // Ethernet
	out.Write(hdr)

	type side struct{ seq uint32 }
	type conn struct {
		client, server side
		started        bool
		last           time.Time
	}
	conns := map[int]*conn{}
	var order []int
	write := func(at time.Time, connID int, fromServer bool, flags byte, seq, ack uint32, payload []byte) {
		srcIP, dstIP := []byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}
		srcPort, dstPort := uint16(50000+connID), port
		if fromServer {
			srcIP, dstIP, srcPort, dstPort = dstIP, srcIP, dstPort, srcPort
		}
		pkt := make([]byte, 14+20+20+len(payload))
		copy(pkt[0:6], []byte{2, 0, 0, 0, 0, 2})
		copy(pkt[6:12], []byte{2, 0, 0, 0, 0, 1})
		binary.BigEndian.PutUint16(pkt[12:], 0x0800)
		ip := pkt[14:]
		ip[0] = 0x45
		binary.BigEndian.PutUint16(ip[2:], uint16(20+20+len(payload)))
		ip[8], ip[9] = 64, 6
		copy(ip[12:16], srcIP)
		copy(ip[16:20], dstIP)
		tcp := ip[20:]
		binary.BigEndian.PutUint16(tcp[0:], srcPort)
		binary.BigEndian.PutUint16(tcp[2:], dstPort)
		binary.BigEndian.PutUint32(tcp[4:], seq)
		binary.BigEndian.PutUint32(tcp[8:], ack)
		tcp[12] = 5 << 4
		tcp[13] = flags
		binary.BigEndian.PutUint16(tcp[14:], 65535)
		copy(tcp[20:], payload)
		rec := make([]byte, 16)
		binary.LittleEndian.PutUint32(rec[0:], uint32(at.Unix()))
		binary.LittleEndian.PutUint32(rec[4:], uint32(at.Nanosecond()/1000))
		binary.LittleEndian.PutUint32(rec[8:], uint32(len(pkt)))
		binary.LittleEndian.PutUint32(rec[12:], uint32(len(pkt)))
		out.Write(rec)
		out.Write(pkt)
	}
	const syn, ackF, psh, fin = 0x02, 0x10, 0x08, 0x01
	for i, c := range chunks {
		cn := conns[c.conn]
		if cn == nil {
			cn = &conn{client: side{seq: 1000}, server: side{seq: 9000}}
			conns[c.conn] = cn
			order = append(order, c.conn)
		}
		if !cn.started {
			cn.started = true
			write(c.at, c.conn, false, syn, cn.client.seq, 0, nil)
			write(c.at, c.conn, true, syn|ackF, cn.server.seq, cn.client.seq+1, nil)
			cn.client.seq++
			cn.server.seq++
			write(c.at, c.conn, false, ackF, cn.client.seq, cn.server.seq, nil)
		}
		from, to := &cn.client, &cn.server
		if c.fromPeer {
			from, to = to, from
		}
		if !drop[i] {
			write(c.at, c.conn, c.fromPeer, psh|ackF, from.seq, to.seq, c.data)
		}
		from.seq += uint32(len(c.data))
		cn.last = c.at
	}
	for _, id := range order {
		cn := conns[id]
		write(cn.last, id, false, fin|ackF, cn.client.seq, cn.server.seq, nil)
		write(cn.last, id, true, fin|ackF, cn.server.seq, cn.client.seq+1, nil)
		write(cn.last, id, false, ackF, cn.client.seq+1, cn.server.seq+1, nil)
	}
	return out.Bytes()
}

// waitQuiet waits until the relay has recorded nothing new for a moment.
func (r *relay) waitQuiet() {
	last := -1
	for range 50 {
		n := len(r.recording())
		if n == last {
			return
		}
		last = n
		time.Sleep(20 * time.Millisecond)
	}
}
