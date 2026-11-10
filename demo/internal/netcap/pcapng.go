// SPDX-License-Identifier: MIT

// Package netcap records the demo's traffic: MMS associations between a
// go-iec61850 client and go-iec61850 servers through a TCP relay on
// loopback, framed again as Ethernet/IPv4/TCP between the substation's
// addresses on a virtual clock, plus GOOSE and Sampled Values frames,
// written as pcapng.
package netcap

import (
	"bytes"
	"encoding/binary"
	"time"
)

// Packet is one captured Ethernet frame.
type Packet struct {
	T       time.Time
	Data    []byte
	Comment string
}

func pad4(n int) int { return (n + 3) &^ 3 }

func option(b *bytes.Buffer, code uint16, val []byte) {
	_ = binary.Write(b, binary.LittleEndian, code)
	_ = binary.Write(b, binary.LittleEndian, uint16(len(val)))
	b.Write(val)
	b.Write(make([]byte, pad4(len(val))-len(val)))
}

func block(out *bytes.Buffer, typ uint32, body []byte) {
	total := uint32(12 + len(body))
	_ = binary.Write(out, binary.LittleEndian, typ)
	_ = binary.Write(out, binary.LittleEndian, total)
	out.Write(body)
	_ = binary.Write(out, binary.LittleEndian, total)
}

// WritePCAPNG writes packets as one pcapng section with one Ethernet
// interface (microsecond time stamps). comment goes into the section
// header; a packet's Comment into its block.
func WritePCAPNG(pkts []Packet, comment, ifName, ifDesc string) []byte {
	var out bytes.Buffer
	var shb bytes.Buffer
	_ = binary.Write(&shb, binary.LittleEndian, uint32(0x1A2B3C4D))
	_ = binary.Write(&shb, binary.LittleEndian, uint16(1))
	_ = binary.Write(&shb, binary.LittleEndian, uint16(0))
	_ = binary.Write(&shb, binary.LittleEndian, int64(-1))
	if comment != "" {
		option(&shb, 1, []byte(comment))
	}
	option(&shb, 4, []byte("OT Fabric iec61850-explorer demo generator"))
	option(&shb, 0, nil)
	block(&out, 0x0A0D0D0A, shb.Bytes())

	var idb bytes.Buffer
	_ = binary.Write(&idb, binary.LittleEndian, uint16(1)) // LINKTYPE_ETHERNET
	_ = binary.Write(&idb, binary.LittleEndian, uint16(0))
	_ = binary.Write(&idb, binary.LittleEndian, uint32(262144))
	option(&idb, 2, []byte(ifName))
	option(&idb, 3, []byte(ifDesc))
	option(&idb, 0, nil)
	block(&out, 1, idb.Bytes())

	for _, p := range pkts {
		var epb bytes.Buffer
		us := uint64(p.T.UnixMicro())
		_ = binary.Write(&epb, binary.LittleEndian, uint32(0))
		_ = binary.Write(&epb, binary.LittleEndian, uint32(us>>32))
		_ = binary.Write(&epb, binary.LittleEndian, uint32(us))
		_ = binary.Write(&epb, binary.LittleEndian, uint32(len(p.Data)))
		_ = binary.Write(&epb, binary.LittleEndian, uint32(len(p.Data)))
		epb.Write(p.Data)
		epb.Write(make([]byte, pad4(len(p.Data))-len(p.Data)))
		if p.Comment != "" {
			option(&epb, 1, []byte(p.Comment))
			option(&epb, 0, nil)
		}
		block(&out, 6, epb.Bytes())
	}
	return out.Bytes()
}
