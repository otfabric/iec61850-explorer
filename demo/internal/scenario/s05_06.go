// SPDX-License-Identifier: MIT

package scenario

import (
	"context"
	"time"

	"github.com/otfabric/iec61850-explorer/demo/internal/fault"
)

// S05: the GOOSE of the station bus around the fault: every publisher's
// heartbeat (1 s), then feeder 1's trip (stNum up, sqNum 0, the
// retransmission curve 4, 8, 16 ... ms up to 1 s), the busbar
// protection's breaker failure 152 ms later, and both resetting. Feeder
// 1's position GOOSE does not change: its breaker did not open.
var S05 = Scenario{
	File:    "05-goose.pcapng",
	Title:   "GOOSE: heartbeats, a trip and a breaker failure",
	Start:   fault.T0.Add(-5 * time.Second),
	Seed:    5,
	Comment: "AA1 Northfield, 09:00:05-09:00:13: GOOSE of all six publishers (VLAN 10). At 09:00:10.064 feeder 1 trips (AA1D1Q02A1PROT/LLN0$GO$gcbTrip), its breaker fails to open (gcbPos stays closed), at 09:00:10.216 the busbar protection publishes breaker failure (AA1D1W1A1PROT/LLN0$GO$gcbBF); both reset after the LV incomer cleared the fault.",
	Run: func(_ context.Context, e *Env) error {
		from, to := fault.T0.Add(-5*time.Second), fault.T0.Add(3*time.Second)
		n, err := newGOOSE(e, from)
		if err != nil {
			return err
		}
		return n.run(e, from, to, faultGOOSE())
	},
}

// S06: the merging unit's Sampled Values (UCA 9-2LE, 80 samples per
// cycle, 4000 frames per second) from 100 ms before the fault until the
// LV incomer cleared it, and on until 400 ms after: phases B and C
// rise to 2.4 kA with a decaying DC offset, the voltages of B and C
// collapse, then everything drops to zero.
var S06 = Scenario{
	File:    "06-sampled-values.pcapng",
	Title:   "Sampled Values: the merging unit's stream through the fault",
	Start:   fault.T0.Add(-100 * time.Millisecond),
	Seed:    6,
	Comment: "AA1 Northfield, 09:00:09.900-09:00:10.400: AA1D1Q02MU1 Sampled Values (svID AA1D1Q02MU1MU01, APPID 0x4001, VLAN 20, 9-2LE, 4000 frames/s, smpSynch global). Fault B-C at 09:00:10.000, cleared at 09:00:10.270; the merging unit's status GOOSE heartbeat is also on the segment.",
	Run: func(_ context.Context, e *Env) error {
		from, to := fault.T0.Add(-100*time.Millisecond), fault.T0.Add(400*time.Millisecond)
		if _, err := svStream(e, from, to, nil, nil); err != nil {
			return err
		}
		n, err := newGOOSE(e, from)
		if err != nil {
			return err
		}
		mu := n.byRef["AA1D1Q02MU1MU01/LLN0$GO$gcbStatus"]
		n.pubs = []*gpub{mu}
		return n.run(e, from, to, nil)
	},
}
