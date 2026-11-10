// SPDX-License-Identifier: MIT

// Package ied serves a demo IED from its CID on go-iec61850's server,
// with the behaviour of the device: process values, controls with
// interlocking, setting group banks, a file store with a disturbance
// record and an event journal. The live IEDs (demo/ieds) and the
// captures (demo/gen) both run it.
package ied

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms/transport/iso"

	"github.com/otfabric/iec61850-explorer/demo/internal/fault"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

// Options configure an IED.
type Options struct {
	// Now is the IED's clock; nil uses time.Now. The captures run the
	// IEDs on the scenario's clock.
	Now func() time.Time
	// Live simulates the process: measurements change, the voltage
	// controller moves the tap changer.
	Live bool
	// Deviate makes feeder 1 differ from its CID: firmware 2.4.1 (the
	// CID says 2.4.0) and I> start value 650 A in setting group 1 (the
	// CID says 600 A), for Compare (SCL against IED).
	Deviate bool
	// Pace separates the changes of one operation in real time: go-iec61850's
	// server sends each report from a goroutine of its own after 100 ms,
	// so changes made at once can reach the client in any order. The
	// captures pace them so that the order is the same on every run.
	Pace   time.Duration
	Logger *slog.Logger
}

// IED is a running demo IED.
type IED struct {
	Site    *site.IED
	Srv     *iec61850.Server
	Addr    string
	Journal *iec61850.MemoryJournalProvider

	opts   Options
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	banks  map[string]map[int][]bankVal // LD inst -> group -> values
	ctlMu  sync.Mutex
}

type bankVal struct {
	item string
	v    *mms.Value
}

// Boot is how long before the IED's clock the demo IEDs were last
// started: the time stamps of values that have not changed since.
const Boot = 3*time.Hour + 17*time.Minute + 41*time.Second + 231*time.Millisecond

// Start serves d on addr (host:port; port 0 picks one) until Close.
func Start(d *site.IED, addr string, o Options) (*IED, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	res, err := scl.ParseBytes(site.CID(d), scl.ParseOptions{})
	if err != nil {
		return nil, fmt.Errorf("ied %s: SCL: %w", d.Name, err)
	}
	model, err := iec61850.NewServerModelFromSCL(res.Document, d.Name, "")
	if err != nil {
		return nil, fmt.Errorf("ied %s: model: %w", d.Name, err)
	}
	jp := iec61850.NewMemoryJournalProvider()
	for _, ld := range d.LDs {
		for _, l := range ld.LN0.Logs {
			jp.RegisterJournal(d.Domain(ld.Inst), l.LogName)
		}
	}
	rev := "2.4.0"
	if o.Deviate && d.Name == site.F1Name {
		rev = "2.4.1"
	}
	srv, err := iec61850.NewServer(model, iec61850.ServerOptions{
		Logger:       o.Logger,
		Identity:     &iec61850.ServerIdentity{Vendor: site.Manufacturer, Model: d.Type, Revision: rev},
		FileProvider: newFiles(d),
		MMS:          mms.ServerOptions{JournalProvider: jp, Logger: o.Logger},
	})
	if err != nil {
		return nil, fmt.Errorf("ied %s: server: %w", d.Name, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	i := &IED{Site: d, Srv: srv, Journal: jp, opts: o, cancel: cancel, done: make(chan struct{})}
	i.initValues(ctx)
	i.initBanks()
	if err := i.registerControls(); err != nil {
		cancel()
		srv.Close()
		return nil, err
	}
	srv.EnableReports()
	srv.EnableSettingGroups(iec61850.SettingGroupHandler{OnActiveSGChanged: i.activeSGChanged})
	for _, ld := range d.LDs {
		if ld.LN0.SG != nil {
			i.StampSG(ld.Inst, time.Date(2026, 11, 3, 10, 15, 0, 0, time.UTC))
		}
	}
	i.journalHistory()
	ln, err := iso.Listen(addr)
	if err != nil {
		cancel()
		srv.Close()
		return nil, fmt.Errorf("ied %s: listen: %w", d.Name, err)
	}
	i.Addr = ln.Addr().String()
	go func() {
		defer close(i.done)
		_ = srv.ListenAndServe(ctx, ln)
	}()
	if o.Live {
		go i.simulate(ctx)
	}
	return i, nil
}

// Close stops the IED.
func (i *IED) Close() {
	i.once.Do(func() {
		i.cancel()
		_ = i.Srv.Shutdown(context.Background())
		<-i.done
		i.Srv.Close()
	})
}

// Now is the IED's clock.
func (i *IED) Now() time.Time { return i.opts.Now() }

// Set changes a value: item is an MMS item of the logical device inst
// ("MMXU1$MX$Hz$mag$f"). Reports watching it are triggered.
func (i *IED) Set(inst, item string, v *mms.Value) {
	i.Srv.SetValue(context.Background(), i.Site.Domain(inst)+"/"+item, v)
}

// Get returns a value, or nil.
func (i *IED) Get(inst, item string) *mms.Value {
	return i.Srv.ValueStore().Get(i.Site.Domain(inst) + "/" + item)
}

// Values of IEC 61850 types.
var (
	Good = iec61850.EncodeQuality(0)
)

// TS is a time stamp with the accuracy of the demo IEDs (10 bits, clock
// synchronised).
func TS(t time.Time) *mms.Value {
	return iec61850.EncodeTimestamp(iec61850.Timestamp{Time: t, Quality: iec61850.TimeQuality{TimeAccuracy: 10}})
}

// Dbpos is a double point position: true closed (on), false open (off).
func Dbpos(closed bool) *mms.Value {
	if closed {
		return mms.NewBitStringWithLength([]byte{0x80}, 2)
	}
	return mms.NewBitStringWithLength([]byte{0x40}, 2)
}

// quiet stores a value without notifying the report engine: the parts
// of a data object that change together are stored first and the last
// one is set with Set, so that the change is reported once.
// go-iec61850 v1.16.0 reports every change of a data set member at once
// (it does not apply BufTm) and compares a whole FCD member, time
// stamp included.
func (i *IED) quiet(inst, item string, v *mms.Value) {
	i.Srv.ValueStore().Set(i.Site.Domain(inst)+"/"+item, v)
}

// Status sets the attributes of a status data object (FC ST) at time at:
// the time stamp and the values (attr name, value pairs), reported as
// one change.
func (i *IED) Status(inst, ln, do string, at time.Time, kv ...any) {
	i.quiet(inst, ln+"$ST$"+do+"$t", TS(at))
	for k := 0; k+1 < len(kv); k += 2 {
		item, v := ln+"$ST$"+do+"$"+kv[k].(string), kv[k+1].(*mms.Value)
		if k+2 < len(kv) {
			i.quiet(inst, item, v)
		} else {
			i.Set(inst, item, v)
		}
	}
}

// Meas sets a measured value (MV, FC MX).
func (i *IED) Meas(inst, ln, do string, at time.Time, v float64) {
	i.quiet(inst, ln+"$MX$"+do+"$t", TS(at))
	i.Set(inst, ln+"$MX$"+do+"$mag$f", mms.NewFloat(v))
}

// Phases sets the complex values (CMV) of a WYE or SEQ data object (FC
// MX), reported as one change.
func (i *IED) Phases(inst, ln, do string, at time.Time, phs []string, mag, ang []float64) {
	for k, ph := range phs {
		p := ln + "$MX$" + do + "$" + ph
		i.quiet(inst, p+"$t", TS(at))
		i.quiet(inst, p+"$cVal$ang$f", mms.NewFloat(ang[k]))
		if k == len(phs)-1 {
			i.Set(inst, p+"$cVal$mag$f", mms.NewFloat(mag[k]))
		} else {
			i.quiet(inst, p+"$cVal$mag$f", mms.NewFloat(mag[k]))
		}
	}
}

// initValues gives every status and measurement its value and a time
// stamp from the last start of the IED.
func (i *IED) initValues(ctx context.Context) {
	boot := i.Now().Add(-Boot).Truncate(time.Millisecond)
	ts := TS(boot)
	types := map[string]site.DOType{}
	for _, t := range site.DOTypes {
		types[t.ID] = t
	}
	lnTypes := map[string]site.LNodeType{}
	for _, t := range site.LNodeTypes {
		lnTypes[t.ID] = t
	}
	var walk func(inst, ln, path, typ string)
	walk = func(inst, ln, path, typ string) {
		for _, e := range types[typ].Elems {
			if e.SDO {
				walk(inst, ln, path+"$"+e.Name, e.Type)
				continue
			}
			if e.Name != "t" || (e.FC != "ST" && e.FC != "MX") {
				continue
			}
			parts := strings.SplitN(path, "$", 2)
			item := ln + "$" + e.FC + "$" + parts[0]
			if len(parts) > 1 {
				item += "$" + parts[1]
			}
			i.Srv.SetValue(ctx, i.Site.Domain(inst)+"/"+item+"$t", ts)
		}
	}
	for _, ld := range i.Site.LDs {
		lns := append([]site.LN{{Class: "LLN0", Type: ld.LN0.Type}}, ld.LNs...)
		for _, ln := range lns {
			for _, do := range lnTypes[ln.Type].DOs {
				walk(ld.Inst, ln.Name(), do.Name, do.Type)
				switch do.Type {
				case "ENS_Beh", "ENC_Mod", "ENS_Health":
					i.Set(ld.Inst, ln.Name()+"$ST$"+do.Name+"$stVal", mms.NewInteger(1))
				}
			}
		}
	}
	i.processValues(boot)
	if i.opts.Deviate && i.Site.Name == site.F1Name {
		i.Set("LD0", "LPHD1$DC$PhyNam$swRev", mms.NewVisibleString("2.4.1"))
	}
}

// processValues sets the values of the process at start.
func (i *IED) processValues(at time.Time) {
	b, f := mms.NewBoolean, func(v int64) *mms.Value { return mms.NewInteger(v) }
	switch i.Site.Name {
	case site.F1Name, site.F2Name:
		i.Status("LD0", "GGIO1", "Ind2", at, "stVal", b(true))
		i.Status("CTRL", "CSWI1", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "CSWI2", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "XCBR1", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "XSWI1", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "XCBR1", "OpCnt", at, "stVal", f(1287))
		i.Status("CTRL", "XSWI1", "OpCnt", at, "stVal", f(64))
		i.Status("CTRL", "XSWI1", "SwTyp", at, "stVal", f(2))
		i.interlocks(at, func() {})
		i.feederMeas(at, 1.0, 0, true)
	case site.T1Name:
		i.Status("CTRL", "XCBR1", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "XCBR2", "Pos", at, "stVal", Dbpos(true))
		i.Status("CTRL", "XCBR1", "OpCnt", at, "stVal", f(412))
		i.Status("CTRL", "XCBR2", "OpCnt", at, "stVal", f(733))
		i.Status("CTRL", "YLTC1", "OpCnt", at, "stVal", f(18342))
		i.Status("CTRL", "ATCC1", "Auto", at, "stVal", b(true))
		i.tap(at, 9)
		i.Meas("CTRL", "YLTC1", "MotDrvTmp", at, 31.4)
		i.Meas("CTRL", "YLTC1", "MotDrvA", at, 0)
		i.Meas("CTRL", "YPTR1", "LodFact", at, 0.27)
		i.transformerMeas(at, 1.0)
	case site.MUName:
		i.Status("MU01", "LPHD1", "PhyHealth", at, "stVal", f(1))
	}
}

func (i *IED) tap(at time.Time, pos int64) {
	v := mms.NewStructure([]*mms.Value{mms.NewInteger(pos), mms.NewBoolean(false)})
	i.Status("CTRL", "YLTC1", "TapPos", at, "valWTr", v)
	i.Status("CTRL", "ATCC1", "TapPos", at, "valWTr", v)
}

// Load of the feeders: rms current and power factor angle.
func (i *IED) load() (amps, ang float64) {
	if i.Site.Name == site.F2Name {
		return 141, -14
	}
	return fault.LoadA, -18
}

// feederMeas sets a feeder's measurements: scale scales the load, df
// shifts the frequency.
func (i *IED) feederMeas(at time.Time, scale, df float64, all bool) {
	for _, step := range i.feederSteps(at, scale, df, all) {
		step()
	}
	amps, ang := i.load()
	i.threePhase("MEAS", "MMXU1", at, fault.PhaseV, amps*scale, ang, 0)
	i.energy("MEAS", "MMTR1", at, 3*fault.PhaseV*amps*scale, ang)
}

// feederSteps are the changes of a feeder's measurements, one data set
// member each.
func (i *IED) feederSteps(at time.Time, scale, df float64, all bool) []func() {
	amps, ang := i.load()
	amps *= scale
	v := fault.PhaseV
	rad := ang * math.Pi / 180
	a := math.Round(amps*10) / 10
	abc := []string{"phsA", "phsB", "phsC"}
	steps := []func(){
		func() { i.Meas("MEAS", "MMXU1", "TotW", at, math.Round(3*v*amps*math.Cos(-rad)/100)*100) },
		func() { i.Meas("MEAS", "MMXU1", "TotVAr", at, math.Round(3*v*amps*math.Sin(-rad)/100)*100) },
		func() { i.Meas("MEAS", "MMXU1", "Hz", at, math.Round((50.012+df)*1000)/1000) },
		func() {
			i.Phases("MEAS", "MMXU1", "A", at, abc, []float64{a, a, a}, []float64{ang, ang - 120, ang + 120})
		},
	}
	if all {
		steps = append(steps, func() {
			i.Phases("MEAS", "MMXU1", "PhV", at, abc, []float64{v, v, v}, []float64{0, -120, 120})
		})
	}
	return append(steps, func() {
		i.Phases("MEAS", "MSQI1", "SeqA", at, []string{"c1", "c2", "c3"}, []float64{a, 0.8, 0.3}, []float64{ang, 37, 112})
	})
}

// LoadSteps are the changes of a load change of a feeder, one data set
// member each: go-iec61850's server sends each member's report from a
// goroutine of its own, so the captures wait for one before the next.
func (i *IED) LoadSteps(at time.Time, scale, df float64) []func() {
	return i.feederSteps(at, scale, df, false)
}

func (i *IED) transformerMeas(at time.Time, scale float64) {
	lv := (fault.LoadA + 141 + 12) * scale
	abc := []string{"phsA", "phsB", "phsC"}
	i.Meas("MEAS", "MMXU1", "TotW", at, math.Round(3*fault.PhaseV*lv*0.96/1000)*1000+21000)
	i.Meas("MEAS", "MMXU2", "TotW", at, math.Round(3*fault.PhaseV*lv*0.96/1000)*1000)
	i.Meas("MEAS", "MMXU2", "Hz", at, 50.012)
	hv, lvA := math.Round(lv*20/110*10)/10, math.Round(lv*10)/10
	angs := []float64{-16, -136, 104}
	i.Phases("MEAS", "MMXU1", "A", at, abc, []float64{hv, hv, hv}, angs)
	i.Phases("MEAS", "MMXU2", "A", at, abc, []float64{lvA, lvA, lvA}, angs)
	i.threePhase("MEAS", "MMXU1", at, 110000/math.Sqrt(3), lv*20/110, -16, 50.012)
	i.threePhase("MEAS", "MMXU2", at, fault.PhaseV, lv, -16, 0)
	// The tap changer's motor supply: 230 V, idle at 0.4 A.
	for do, v := range map[string]float64{"Vol": 231.2, "Amp": 0.4, "Watt": 74, "VolAmpr": 55, "VolAmp": 92.5, "PwrFact": 0.8, "Hz": 50.01} {
		i.quietMeas("MEAS", "MMXN1", do, at, v)
	}
	i.quietMeas("MEAS", "MHAI1", "Hz", at, 50.012)
	i.energy("MEAS", "MMTR1", at, 3*fault.PhaseV*lv, -16)
}

// threePhase sets the measurements of an MMXU that the reported data sets
// leave out, quietly (no report): apparent power and power factor, the
// voltages line to line and line to neutral, the per-phase powers and the
// impedance seen from the voltage and current. v is the phase voltage, a
// the phase current, ang the current's angle in degrees, hz the frequency
// (0 leaves Hz alone).
func (i *IED) threePhase(inst, ln string, at time.Time, v, a, ang, hz float64) {
	rad := -ang * math.Pi / 180
	round := func(x, step float64) float64 { return math.Round(x/step) * step }
	abc := []string{"phsA", "phsB", "phsC"}
	angs := []float64{0, -120, 120}
	i.quietMeas(inst, ln, "TotVA", at, round(3*v*a, 100))
	i.quietMeas(inst, ln, "TotPF", at, round(math.Cos(rad), 0.001))
	if hz > 0 { // 0: Hz is a reported data set member, set elsewhere
		i.quietMeas(inst, ln, "Hz", at, round(hz, 0.001))
	}
	vll := round(v*math.Sqrt(3), 1)
	i.quietPhases(inst, ln, "PPV", at, []string{"phsAB", "phsBC", "phsCA"}, []float64{vll, vll, vll}, []float64{30, -90, 150})
	vn := round(v, 1)
	i.quietPhases(inst, ln, "PNV", at, abc, []float64{vn, vn, vn}, angs)
	per := func(x float64) []float64 { return []float64{x, x, x} }
	i.quietPhases(inst, ln, "W", at, abc, per(round(v*a*math.Cos(rad), 100)), angs)
	i.quietPhases(inst, ln, "VAr", at, abc, per(round(v*a*math.Sin(rad), 100)), angs)
	i.quietPhases(inst, ln, "VA", at, abc, per(round(v*a, 100)), angs)
	i.quietPhases(inst, ln, "PF", at, abc, per(round(math.Cos(rad), 0.001)), angs)
	if a > 0 {
		z := round(v/a, 0.01)
		i.quietPhases(inst, ln, "Z", at, abc, per(z), per(-ang))
	}
}

// energy sets the counters of an MMTR (Wh, VArh, VAh), quietly: the energy
// delivered since 2026-01-01 at power p (W) and the current's angle ang,
// with 4 % returned (supply) and the rest drawn (demand).
func (i *IED) energy(inst, ln string, at time.Time, p, ang float64) {
	h := at.Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Hours()
	rad := -ang * math.Pi / 180
	wh, varh := p*math.Cos(rad)*h, p*math.Sin(rad)*h
	for do, v := range map[string]float64{
		"TotWh": wh, "TotVArh": varh, "TotVAh": p * h,
		"DmdWh": wh * 0.96, "DmdVArh": varh * 0.96, "SupWh": wh * 0.04, "SupVArh": varh * 0.04,
	} {
		i.quiet(inst, ln+"$ST$"+do+"$t", TS(at))
		i.quiet(inst, ln+"$ST$"+do+"$actVal", mms.NewInteger(int64(v)))
	}
}

// quietMeas sets an MV without a report.
func (i *IED) quietMeas(inst, ln, do string, at time.Time, v float64) {
	i.quiet(inst, ln+"$MX$"+do+"$t", TS(at))
	i.quiet(inst, ln+"$MX$"+do+"$mag$f", mms.NewFloat(v))
}

// quietPhases sets the CMVs of a WYE or DEL without a report.
func (i *IED) quietPhases(inst, ln, do string, at time.Time, phs []string, mag, ang []float64) {
	for k, ph := range phs {
		p := ln + "$MX$" + do + "$" + ph
		i.quiet(inst, p+"$t", TS(at))
		i.quiet(inst, p+"$cVal$mag$f", mms.NewFloat(mag[k]))
		i.quiet(inst, p+"$cVal$ang$f", mms.NewFloat(ang[k]))
	}
}

// interlocks derives the feeder's interlocking: the breaker may always
// open and may close with the disconnector closed; the disconnector may
// move only with the breaker open.
func (i *IED) interlocks(at time.Time, pace func()) {
	cb, ds := i.closed("XCBR1"), i.closed("XSWI1")
	set := func(ln, do string, v bool) {
		if old := i.Get("CTRL", ln+"$ST$"+do+"$stVal"); old != nil {
			if b, ok := old.Bool(); ok && b == v {
				return
			}
		}
		i.Status("CTRL", ln, do, at, "stVal", mms.NewBoolean(v))
		pace()
	}
	set("CILO1", "EnaOpn", true)
	set("CILO1", "EnaCls", ds)
	set("CILO2", "EnaOpn", !cb)
	set("CILO2", "EnaCls", !cb)
}

func (i *IED) closed(ln string) bool {
	v := i.Get("CTRL", ln+"$ST$Pos$stVal")
	if v == nil {
		return false
	}
	bs, _ := v.BitString()
	return len(bs) > 0 && bs[0]&0xc0 == 0x80
}

// --- Controls -------------------------------------------------------

func (i *IED) registerControls() error {
	switch i.Site.Name {
	case site.F1Name, site.F2Name:
		dom := i.Site.Domain("CTRL")
		for _, c := range []struct{ cswi, sw string }{{"CSWI1", "XCBR1"}, {"CSWI2", "XSWI1"}} {
			h := iec61850.ControlHandler{
				SelectTimeout: 30 * time.Second,
				OnOperate:     func(_ context.Context, req iec61850.ControlRequest) error { return i.checkInterlock(c.sw, req) },
				OnExecute: func(_ context.Context, req iec61850.ControlRequest) error {
					closed, _ := req.CtlVal.Bool()
					i.Switch(c.cswi, c.sw, closed, i.Now())
					return nil
				},
			}
			if err := i.Srv.RegisterControl(dom, c.cswi+".Pos", iec61850.CtlModelSBOEnhanced, h); err != nil {
				return fmt.Errorf("ied %s: control %s: %w", i.Site.Name, c.cswi, err)
			}
		}
	case site.T1Name:
		h := iec61850.ControlHandler{OnOperate: func(_ context.Context, req iec61850.ControlRequest) error {
			on, _ := req.CtlVal.Bool()
			i.Status("CTRL", "ATCC1", "Auto", i.Now(), "stVal", mms.NewBoolean(on))
			return nil
		}}
		if err := i.Srv.RegisterControl(i.Site.Domain("CTRL"), "ATCC1.Auto", iec61850.CtlModelDirectNormal, h); err != nil {
			return fmt.Errorf("ied %s: control ATCC1.Auto: %w", i.Site.Name, err)
		}
	}
	return nil
}

// checkInterlock refuses a disconnector operation while the breaker is
// closed (CILO2), and closing the breaker with the disconnector open.
func (i *IED) checkInterlock(sw string, req iec61850.ControlRequest) error {
	closing, _ := req.CtlVal.Bool()
	refuse := false
	switch sw {
	case "XSWI1":
		refuse = i.closed("XCBR1")
	case "XCBR1":
		refuse = closing && !i.closed("XSWI1")
	}
	if refuse {
		return &iec61850.ControlError{Ref: req.Ref, Operation: "operate", AddCause: iec61850.AddCauseBlockedByInterlocking}
	}
	return nil
}

// Switch moves a feeder switch (XCBR1 or XSWI1, controlled by cswi): the
// new position after the operating time, the operation counter, the
// interlocking and an entry in the event log.
func (i *IED) Switch(cswi, sw string, closed bool, at time.Time) {
	i.ctlMu.Lock()
	defer i.ctlMu.Unlock()
	opTime := 45 * time.Millisecond
	if sw == "XSWI1" {
		opTime = 2400 * time.Millisecond
	}
	t := at.Add(opTime).Truncate(time.Millisecond)
	n := int64(0)
	if v := i.Get("CTRL", sw+"$ST$OpCnt$stVal"); v != nil {
		n, _ = v.Int64()
	}
	pace := func() {
		if i.opts.Pace > 0 {
			time.Sleep(i.opts.Pace)
		}
	}
	i.Status("CTRL", sw, "OpCnt", t, "stVal", mms.NewInteger(n+1))
	pace()
	i.Status("CTRL", sw, "Pos", t, "stVal", Dbpos(closed))
	pace()
	i.Status("CTRL", cswi, "Pos", t, "stVal", Dbpos(closed))
	pace()
	i.interlocks(t, pace)
	i.LogPosition(sw, closed, t)
	if i.opts.Pace > 0 {
		// Let the position reports go out before the CommandTermination.
		time.Sleep(100*time.Millisecond + 2*i.opts.Pace)
	}
}

// --- Event journal ---------------------------------------------------

// LogPosition adds a switch position to the feeder's event log.
func (i *IED) LogPosition(sw string, closed bool, at time.Time) {
	dom := i.Site.Domain("CTRL")
	i.Journal.AddEntry(dom, "EventLog", at, []mms.JournalVariable{
		{Tag: dom + "/" + sw + "$ST$Pos$stVal", Value: Dbpos(closed)},
		{Tag: dom + "/" + sw + "$ST$Pos$q", Value: Good},
		{Tag: "ReasonCode", Value: mms.NewBitStringWithLength([]byte{0x40}, 7)}, // data-change
	})
}

// journalHistory fills the feeder's event log with last night's
// maintenance switching.
func (i *IED) journalHistory() {
	if i.Site.Name != site.F1Name && i.Site.Name != site.F2Name {
		return
	}
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for _, e := range []struct {
		at     time.Duration
		sw     string
		closed bool
	}{
		{22*time.Hour + 4*time.Minute + 12*time.Second + 304*time.Millisecond, "XCBR1", false},
		{22*time.Hour + 6*time.Minute + 47*time.Second + 811*time.Millisecond, "XSWI1", false},
		{23*time.Hour + 31*time.Minute + 5*time.Second + 92*time.Millisecond, "XSWI1", true},
		{23*time.Hour + 32*time.Minute + 40*time.Second + 518*time.Millisecond, "XCBR1", true},
	} {
		i.LogPosition(e.sw, e.closed, day.Add(e.at))
	}
}

// --- Setting groups --------------------------------------------------

func (i *IED) initBanks() {
	i.banks = map[string]map[int][]bankVal{}
	for _, ld := range i.Site.LDs {
		if ld.LN0.SG == nil {
			continue
		}
		groups := map[int][]bankVal{}
		for _, ln := range ld.LNs {
			for _, doi := range ln.DOIs {
				for _, dai := range doi.DAIs {
					for _, v := range dai.Vals {
						if v.SGroup == 0 {
							continue
						}
						val := v.V
						if i.opts.Deviate && i.Site.Name == site.F1Name && ln.Name() == "PTOC1" && doi.Name == "StrVal" && v.SGroup == 1 {
							val = "650"
						}
						item := ln.Name() + "$SG$" + doi.Name + "$" + strings.ReplaceAll(dai.Path, ".", "$")
						groups[v.SGroup] = append(groups[v.SGroup], bankVal{item: item, v: number(dai.Path, val)})
					}
				}
			}
		}
		i.banks[ld.Inst] = groups
		i.applyBank(ld.Inst, ld.LN0.SG.ActSG)
	}
}

func number(path, s string) *mms.Value {
	if strings.HasSuffix(path, ".f") {
		f, _ := strconv.ParseFloat(s, 64)
		return mms.NewFloat(f)
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return mms.NewInteger(n)
}

func (i *IED) applyBank(inst string, sg int) {
	for _, bv := range i.banks[inst][sg] {
		i.Set(inst, bv.item, bv.v)
	}
}

func (i *IED) activeSGChanged(_ context.Context, ld string, sg uint8) error {
	inst := strings.TrimPrefix(ld, i.Site.Name)
	if _, ok := i.banks[inst][int(sg)]; !ok {
		return fmt.Errorf("ied %s: no setting group %d in %s", i.Site.Name, sg, ld)
	}
	i.applyBank(inst, int(sg))
	return nil
}

// StampSG sets the SGCB's LActTm of a logical device to at. go-iec61850's
// server stamps a change with the host's clock (time.Now); the captures
// run the IED on the scenario's clock and stamp it again.
func (i *IED) StampSG(inst string, at time.Time) {
	i.Srv.ValueStore().Set(i.Site.Domain(inst)+"/LLN0$SP$SGCB$LActTm", mms.NewUTCTime(at))
}

// --- Files -------------------------------------------------------------

// files is the IED's read-only file store: its CID and, for feeder 1,
// the disturbance record of the fault.
type files map[string]fileEntry

type fileEntry struct {
	data []byte
	mod  time.Time
}

func newFiles(d *site.IED) files {
	h := d.History[len(d.History)-1]
	when, _ := time.Parse(time.RFC3339, h.When)
	f := files{d.Name + ".cid": {site.CID(d), when.Add(25 * time.Minute)}}
	if d.Name == site.F1Name {
		r := fault.Comtrade(site.Substation, d.Name)
		mod := fault.T0.Add(1200 * time.Millisecond)
		f["COMTRADE/"+r.Name+".cfg"] = fileEntry{r.CFG, mod}
		f["COMTRADE/"+r.Name+".dat"] = fileEntry{r.DAT, mod}
	}
	return f
}

type handle struct {
	data []byte
	off  int
}

func (f files) List(_ context.Context, req mms.FileListRequest) (*mms.FileListResult, error) {
	var names []string
	for name := range f {
		if strings.HasPrefix(name, strings.TrimPrefix(req.FileSpec, "/")) && name > req.ContinueAfter {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	res := &mms.FileListResult{}
	for _, name := range names {
		if req.MaxEntries > 0 && len(res.Entries) == req.MaxEntries {
			res.MoreFollows = true
			break
		}
		res.Entries = append(res.Entries, mms.FileEntry{Name: name, Size: int64(len(f[name].data)), LastModified: f[name].mod})
	}
	return res, nil
}

func (f files) Open(_ context.Context, path string) (mms.FileHandle, mms.FileAttributes, error) {
	e, ok := f[strings.TrimPrefix(path, "/")]
	if !ok {
		return nil, mms.FileAttributes{}, fs.ErrNotExist
	}
	return &handle{data: e.data}, mms.FileAttributes{Size: int64(len(e.data)), LastModified: e.mod}, nil
}

func (files) Read(_ context.Context, h mms.FileHandle, maxBytes int) ([]byte, bool, error) {
	fh, ok := h.(*handle)
	if !ok {
		return nil, false, fmt.Errorf("ied: bad file handle %T", h)
	}
	n := min(maxBytes, len(fh.data)-fh.off)
	chunk := fh.data[fh.off : fh.off+n]
	fh.off += n
	return chunk, fh.off < len(fh.data), nil
}

func (files) Close(context.Context, mms.FileHandle) error      { return nil }
func (files) Delete(context.Context, string) error             { return fs.ErrPermission }
func (files) Rename(context.Context, string, string) error     { return fs.ErrPermission }
func (files) ObtainFile(context.Context, string, string) error { return fs.ErrPermission }

// Process sets the measurements of a feeder or the transformer at time
// at: scale scales the load, df shifts the frequency (Hz).
func (i *IED) Process(at time.Time, scale, df float64) {
	switch i.Site.Name {
	case site.F1Name, site.F2Name:
		i.feederMeas(at, scale, df, false)
	case site.T1Name:
		i.transformerMeas(at, scale)
	}
}
