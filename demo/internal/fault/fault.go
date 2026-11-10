// SPDX-License-Identifier: MIT

// Package fault is the story of the demo day: at 09:00:10 a two-phase
// fault (B-C) on feeder 1. Feeder 1's I>> stage trips, but its breaker
// QA1 does not open (stuck); the busbar protection's breaker failure
// function trips the transformer's LV incomer, which clears the fault
// and leaves the 20 kV busbar dead. The Sampled Values, the GOOSE
// messages, the MMS reports and the disturbance record of the demo all
// follow this timeline.
package fault

import (
	"math"
	"time"
)

// T0 is the fault inception.
var T0 = time.Date(2026, 11, 10, 9, 0, 10, 0, time.UTC)

// Times after T0.
const (
	StrAt      = 12 * time.Millisecond  // feeder 1 PTOC1/PTOC2 start
	OpAt       = 62 * time.Millisecond  // feeder 1 PTOC2 operates (OpDlTmms 50 ms)
	TrAt       = 64 * time.Millisecond  // feeder 1 PTRC1 trip, GOOSE gcbTrip
	BFStrAt    = 66 * time.Millisecond  // busbar RBRF1 started by the trip GOOSE
	BFOpAt     = 216 * time.Millisecond // RBRF1 OpEx after FailTmms 150 ms, GOOSE gcbBF
	LVTripAt   = 218 * time.Millisecond // transformer PTRC1 intertrip of XCBR2
	LVOpenAt   = 268 * time.Millisecond // XCBR2 open (50 ms breaker time)
	ClearAt    = 270 * time.Millisecond // currents and voltages gone
	StrResetAt = 285 * time.Millisecond // starts drop off
	TrResetAt  = 330 * time.Millisecond // trip and breaker failure reset
)

// Rated values.
const (
	Freq      = 50.0
	LoadA     = 182.0  // A rms before the fault
	FaultA    = 2410.0 // A rms in phases B and C
	PhaseV    = 11547.0
	FaultVpu  = 0.58  // voltage of phases B and C during the fault
	loadAngle = -18.0 // degrees, current behind voltage
	faultAng  = -76.0
	tau       = 0.035 // s, DC time constant
)

// Instant returns the instantaneous currents (A) and voltages (V) of
// phases A, B, C and neutral at time dt after T0 (negative: before).
func Instant(dt time.Duration) (amps, volts [4]float64) {
	t := dt.Seconds()
	w := 2 * math.Pi * Freq
	if dt >= ClearAt {
		return
	}
	ph := [3]float64{0, -120, 120}
	for i := range 3 {
		vRMS, iRMS, iAng := PhaseV, LoadA, loadAngle
		faulted := dt >= 0 && i > 0
		if faulted {
			vRMS *= FaultVpu
			iRMS, iAng = FaultA, faultAng
		}
		va := (ph[i]) * math.Pi / 180
		volts[i] = math.Sqrt2 * vRMS * math.Sin(w*t+va)
		ia := (ph[i] + iAng) * math.Pi / 180
		amps[i] = math.Sqrt2 * iRMS * math.Sin(w*t+ia)
		if faulted {
			// DC offset: continuity with the load current at inception.
			pre := math.Sqrt2 * LoadA * math.Sin((ph[i]+loadAngle)*math.Pi/180)
			post := math.Sqrt2 * FaultA * math.Sin(ia)
			amps[i] += (pre - post) * math.Exp(-t/tau)
		}
	}
	amps[3] = amps[0] + amps[1] + amps[2]
	volts[3] = (volts[0] + volts[1] + volts[2]) / 3
	return
}

// RMS returns the rms currents and voltages of phases A, B and C at dt.
func RMS(dt time.Duration) (amps, volts [3]float64) {
	if dt >= ClearAt {
		return
	}
	for i := range 3 {
		amps[i], volts[i] = LoadA, PhaseV
		if dt >= 0 && i > 0 {
			amps[i], volts[i] = FaultA, PhaseV*FaultVpu
		}
	}
	return
}
