// SPDX-License-Identifier: MIT

package fault

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Record is a COMTRADE disturbance record (IEEE C37.111-1999, ASCII).
type Record struct {
	Name     string // base file name without extension
	CFG, DAT []byte
}

// Comtrade returns feeder 1's disturbance record of the fault: 400 ms at
// 1 kHz (100 ms before T0), four currents, four voltages and four binary
// channels, as the relay of station and device would store it.
func Comtrade(station, device string) Record {
	const rate = 1000
	const pre = 100 * time.Millisecond
	const n = 400
	start := T0.Add(-pre)
	name := fmt.Sprintf("%s_%s", device, T0.Format("20060102_150405"))
	ia := [4]string{"IA", "IB", "IC", "IN"}
	va := [4]string{"VA", "VB", "VC", "VN"}
	const aScale, vScale = 0.1, 1.0 // A and V per count
	var cfg strings.Builder
	fmt.Fprintf(&cfg, "%s,%s,1999\r\n", station, device)
	fmt.Fprintf(&cfg, "12,8A,4D\r\n")
	for i, ch := range ia {
		fmt.Fprintf(&cfg, "%d,%s,%s,QA1,A,%g,0,0,-32767,32767,600,1,P\r\n", i+1, ch, "ABCN"[i:i+1], aScale)
	}
	for i, ch := range va {
		fmt.Fprintf(&cfg, "%d,%s,%s,BU1,V,%g,0,0,-32767,32767,20000,100,P\r\n", i+5, ch, "ABCN"[i:i+1], vScale)
	}
	digital := []string{"PTOC2.Str", "PTRC1.Tr", "XCBR1.Pos closed", "RBRF1.OpEx received"}
	for i, ch := range digital {
		fmt.Fprintf(&cfg, "%d,%s,,,0\r\n", i+1, ch)
	}
	fmt.Fprintf(&cfg, "50\r\n1\r\n%d,%d\r\n", rate, n)
	fmt.Fprintf(&cfg, "%s\r\n%s\r\nASCII\r\n1\r\n", start.Format("02/01/2006,15:04:05.000000"), T0.Format("02/01/2006,15:04:05.000000"))
	var dat strings.Builder
	for k := range n {
		dt := time.Duration(k)*time.Second/rate - pre
		amps, volts := Instant(dt)
		fmt.Fprintf(&dat, "%d,%d", k+1, k*1000000/rate)
		for _, a := range amps {
			fmt.Fprintf(&dat, ",%d", int(math.Round(a/aScale)))
		}
		for _, v := range volts {
			fmt.Fprintf(&dat, ",%d", int(math.Round(v/vScale)))
		}
		bits := []bool{dt >= StrAt && dt < StrResetAt, dt >= TrAt && dt < TrResetAt, true, dt >= BFOpAt+time.Millisecond && dt < TrResetAt}
		for _, b := range bits {
			if b {
				dat.WriteString(",1")
			} else {
				dat.WriteString(",0")
			}
		}
		dat.WriteString("\r\n")
	}
	return Record{Name: name, CFG: []byte(cfg.String()), DAT: []byte(dat.String())}
}
