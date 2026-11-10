// SPDX-License-Identifier: MIT

package site

import "fmt"

// Names of the substation.
const (
	Substation     = "AA1"
	SubstationDesc = "Northfield 110/20 kV"
	SubNetwork     = "AA1WA1"
	Gateway        = "10.20.1.1"

	HMIName = "AA1KA1"
	F1Name  = "AA1D1Q02A1"
	F2Name  = "AA1D1Q03A1"
	T1Name  = "AA1E1Q01A1"
	BBName  = "AA1D1W1A1"
	MUName  = "AA1D1Q02MU1"

	// VLANs of the station bus: GOOSE and Sampled Values.
	GOOSEVLAN = 10
	SVVLAN    = 20
)

// Manufacturer of every device: the demo generator. No device here
// describes a real product.
const Manufacturer = "OT Fabric Demo"

// IEDs returns the IEDs of the substation, in SCD order.
func IEDs() []*IED {
	return []*IED{HMI(), T1(), Feeder(1), Feeder(2), Busbar(), MU()}
}

// ServedIEDs are the IEDs with an MMS server (all but the HMI).
func ServedIEDs() []*IED {
	var out []*IED
	for _, d := range IEDs() {
		if !d.Client {
			out = append(out, d)
		}
	}
	return out
}

// ByName returns the IED with the name, or nil.
func ByName(name string) *IED {
	for _, d := range IEDs() {
		if d.Name == name {
			return d
		}
	}
	return nil
}

var serverServices = []string{
	`<DynAssociation max="4"/>`,
	`<SettingGroups><SGEdit/><ConfSG/></SettingGroups>`,
	`<GetDirectory/>`,
	`<GetDataObjectDefinition/>`,
	`<DataObjectDirectory/>`,
	`<GetDataSetValue/>`,
	`<DataSetDirectory/>`,
	`<ConfDataSet max="12" maxAttributes="64"/>`,
	`<ReadWrite/>`,
	`<ConfReportControl max="12"/>`,
	`<GetCBValues/>`,
	`<ReportSettings rptID="Dyn" optFields="Dyn" bufTime="Dyn" trgOps="Dyn" intgPd="Dyn"/>`,
	`<GSESettings appID="Conf" dataLabel="Fix"/>`,
	`<GOOSE max="4"/>`,
	`<ConfLogControl max="2"/>`,
	`<FileHandling/>`,
	`<ConfLNs fixPrefix="true" fixLnInst="true"/>`,
}

var hist = []Hitem{
	{"1", "A", "2026-09-14T10:12:00Z", "j.devries (substation engineering)", "Initial system specification (SSD)", "Project start"},
	{"2", "A", "2026-10-02T14:40:00Z", "j.devries (substation engineering)", "IEDs added, data flow engineered", "Detailed design"},
	{"3", "B", "2026-11-03T08:55:00Z", "m.okafor (commissioning)", "GOOSE APPIDs and VLANs aligned with the network plan", "FAT findings"},
}

func namPlt(d string) DOI {
	return DOI{Name: "NamPlt", DAIs: []DAI{V("vendor", Manufacturer), V("swRev", "2.4.0"), V("d", d), V("configRev", "AA1-2026-11-03")}}
}

func phyNam(model, serial string) DOI {
	return DOI{Name: "PhyNam", DAIs: []DAI{V("vendor", Manufacturer), V("hwRev", "C"), V("swRev", "2.4.0"), V("serNum", serial), V("model", model), V("location", "AA1 Northfield, relay room")}}
}

func ln0(ed1 bool, d string) LN0 {
	t := "LLN0_A"
	if ed1 {
		t = "LLN0_E1"
	}
	return LN0{Type: t, DOIs: []DOI{namPlt(d)}}
}

func fcd(ld, prefix, class, inst, doName, fc string) FCDA {
	return FCDA{LD: ld, Prefix: prefix, Class: class, Inst: inst, DO: doName, FC: fc}
}

func fcda(ld, prefix, class, inst, doName, daName, fc string) FCDA {
	return FCDA{LD: ld, Prefix: prefix, Class: class, Inst: inst, DO: doName, DA: daName, FC: fc}
}

func hmiClient() []ClientLN {
	return []ClientLN{{IED: HMIName, LD: "LD0", Class: "IHMI", Inst: "1", AP: "S1"}}
}

func rptID(ied, ld, rcb string) string { return ied + ld + "/LLN0." + rcb }

// GoCBRef is the reference a GoCB has on the wire (gocbRef).
func GoCBRef(ied, ld, cb string) string { return ied + ld + "/LLN0$GO$" + cb }

// HMI is the station HMI and gateway: the MMS client of the captures and
// the client the report control blocks are configured for.
func HMI() *IED {
	return &IED{
		Name: HMIName, Desc: "Station HMI and SCADA gateway", Manufacturer: Manufacturer, Type: "Station computer",
		ConfigVer: "1.2", Client: true, AP: "S1", IP: "10.20.1.10", MAC: "02:61:85:01:00:0a",
		Services: []string{`<GetDirectory/>`, `<GetDataObjectDefinition/>`, `<GetDataSetValue/>`, `<DataSetDirectory/>`, `<ReadWrite/>`, `<GetCBValues/>`, `<FileHandling/>`, `<ClientServices goose="false" gsse="false" bufReport="true" unbufReport="true" readLog="true" sv="false"/>`},
		LDs: []LD{{Inst: "LD0", Desc: "Station HMI", LN0: ln0(false, "Station HMI"), LNs: []LN{
			{Class: "LPHD", Inst: "1", Type: "LPHD_A", DOIs: []DOI{phyNam("KA-200 station computer", "KA200-0019")}},
			{Class: "IHMI", Inst: "1", Type: "IHMI_A", Desc: "Operator workplace"},
		}}},
	}
}

// Feeder returns feeder n (1 or 2): protection and control of a 20 kV
// cable feeder. Feeder 2 was engineered with an Edition 1 tool.
func Feeder(n int) *IED {
	name, port, ip := F1Name, 10201, "10.20.1.31"
	desc, line := "Feeder 1 protection and control", "Riverside"
	if n == 2 {
		name, port, ip = F2Name, 10202, "10.20.1.32"
		desc, line = "Feeder 2 protection and control", "Hillcrest"
	}
	ed1 := n == 2
	appBase := uint16(0x0100 * n)
	macBase := 0x10 * n
	d := &IED{
		Name: name, Desc: desc + " (" + line + ")", Manufacturer: Manufacturer, Type: "FPC-615",
		ConfigVer: "3.1", Ed1: ed1, AP: "S1", IP: ip, MAC: fmt.Sprintf("02:61:85:01:00:%02x", 0x30+n), Port: port,
		Services: serverServices,
	}
	if ed1 {
		d.Type = "FPC-600"
		d.ConfigVer = "1.7"
	}
	// LD0: device.
	ld0 := LD{Inst: "LD0", Desc: "Device", LN0: ln0(ed1, desc), LNs: []LN{
		{Class: "LPHD", Inst: "1", Type: "LPHD_A", DOIs: []DOI{phyNam(d.Type, fmt.Sprintf("%s-26%04d", d.Type, 4410+n))}},
		{Class: "GGIO", Inst: "1", Type: "GGIO_IND3", Desc: "Breaker supervision inputs"},
	}}
	// PROT: overcurrent protection with two setting groups.
	prot := LD{Inst: "PROT", Desc: "Protection", LN0: ln0(ed1, "Protection")}
	prot.LN0.DataSets = []DataSet{
		{Name: "dsTrip", Desc: "Protection starts and trips", FCDAs: []FCDA{
			fcd("PROT", "", "PTOC", "1", "Str", "ST"), fcd("PROT", "", "PTOC", "1", "Op", "ST"),
			fcd("PROT", "", "PTOC", "2", "Str", "ST"), fcd("PROT", "", "PTOC", "2", "Op", "ST"),
			fcd("PROT", "", "PTRC", "1", "Tr", "ST")}},
		{Name: "dsGoTrip", Desc: "Trip for GOOSE", FCDAs: []FCDA{
			fcda("PROT", "", "PTRC", "1", "Tr", "general", "ST"), fcda("PROT", "", "PTRC", "1", "Tr", "q", "ST")}},
	}
	prot.LN0.Reports = []Report{{
		Name: "brcbTrip", Desc: "Protection events to the station HMI", RptID: rptID(name, "PROT", "brcbTrip"), DatSet: "dsTrip",
		ConfRev: 1, Buffered: true, BufTime: 50, Trg: Trg{Dchg: true, Qchg: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, DataRef: true, EntryID: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	prot.LN0.GSE = []GSEControl{{
		Name: "gcbTrip", Desc: "Trip to busbar and transformer protection", GoID: name + "_Trip", DatSet: "dsGoTrip", ConfRev: 1,
		MAC: fmt.Sprintf("01-0C-CD-01-00-%02X", macBase+1), APPID: appBase + 1, VLAN: GOOSEVLAN, Prio: 4, MinTime: 4, MaxTime: 1000,
	}}
	prot.LN0.SG = &SettingControl{NumOfSGs: 2, ActSG: 1}
	prot.LNs = []LN{
		{Class: "PTOC", Inst: "1", Type: "PTOC_A", Desc: "Overcurrent I> (51)", DOIs: []DOI{
			{Name: "StrVal", DAIs: []DAI{SG("setMag.f", "600", "800"), V("minVal.f", "50"), V("maxVal.f", "5000"), V("stepSize.f", "10")}},
			{Name: "OpDlTmms", DAIs: []DAI{SG("setVal", "300", "500"), V("minVal", "0"), V("maxVal", "60000"), V("stepSize", "10")}}}},
		{Class: "PTOC", Inst: "2", Type: "PTOC_A", Desc: "Overcurrent I>> (50)", DOIs: []DOI{
			{Name: "StrVal", DAIs: []DAI{SG("setMag.f", "4000", "5000"), V("minVal.f", "100"), V("maxVal.f", "20000"), V("stepSize.f", "10")}},
			{Name: "OpDlTmms", DAIs: []DAI{SG("setVal", "50", "50"), V("minVal", "0"), V("maxVal", "60000"), V("stepSize", "10")}}}},
		{Class: "PTRC", Inst: "1", Type: "PTRC_A", Desc: "Trip conditioning"},
	}
	// CTRL: breaker and disconnector, positions, the event log.
	ctrl := LD{Inst: "CTRL", Desc: "Control", LN0: ln0(ed1, "Control")}
	ctrl.LN0.DataSets = []DataSet{
		{Name: "dsPos", Desc: "Switch positions and interlocking", FCDAs: []FCDA{
			fcd("CTRL", "", "CSWI", "1", "Pos", "ST"), fcd("CTRL", "", "CSWI", "2", "Pos", "ST"),
			fcd("CTRL", "", "XCBR", "1", "Pos", "ST"), fcd("CTRL", "", "XSWI", "1", "Pos", "ST"),
			fcd("CTRL", "", "XCBR", "1", "OpCnt", "ST"),
			fcda("CTRL", "", "CILO", "2", "EnaOpn", "stVal", "ST"), fcda("CTRL", "", "CILO", "2", "EnaCls", "stVal", "ST")}},
		{Name: "dsGoPos", Desc: "Breaker position for GOOSE", FCDAs: []FCDA{
			fcda("CTRL", "", "XCBR", "1", "Pos", "stVal", "ST"), fcda("CTRL", "", "XCBR", "1", "Pos", "q", "ST"), fcda("CTRL", "", "XCBR", "1", "Pos", "t", "ST")}},
	}
	ctrl.LN0.Reports = []Report{{
		Name: "brcbPos", Desc: "Switch positions to the station HMI", RptID: rptID(name, "CTRL", "brcbPos"), DatSet: "dsPos",
		ConfRev: 1, Buffered: true, BufTime: 50, Trg: Trg{Dchg: true, Qchg: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, EntryID: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	ctrl.LN0.Logs = []LogControl{{Name: "lcbEvents", Desc: "Switching events", DatSet: "dsPos", LogName: "EventLog", Trg: Trg{Dchg: true, Qchg: true}}}
	ctrl.LN0.GSE = []GSEControl{{
		Name: "gcbPos", Desc: "Breaker position to the busbar protection", GoID: name + "_Pos", DatSet: "dsGoPos", ConfRev: 1,
		MAC: fmt.Sprintf("01-0C-CD-01-00-%02X", macBase+2), APPID: appBase + 2, VLAN: GOOSEVLAN, Prio: 4, MinTime: 4, MaxTime: 1000,
	}}
	ctrl.LNs = []LN{
		{Class: "CSWI", Inst: "1", Type: "CSWI_SBOE", Desc: "Circuit breaker QA1 control"},
		{Class: "CSWI", Inst: "2", Type: "CSWI_SBOE", Desc: "Disconnector QB1 control"},
		{Class: "XCBR", Inst: "1", Type: "XCBR_A", Desc: "Circuit breaker QA1"},
		{Class: "XSWI", Inst: "1", Type: "XSWI_A", Desc: "Disconnector QB1", DOIs: []DOI{{Name: "SwTyp", DAIs: []DAI{V("stVal", "Disconnector")}}}},
		{Class: "CILO", Inst: "1", Type: "CILO_A", Desc: "Interlocking QA1"},
		{Class: "CILO", Inst: "2", Type: "CILO_A", Desc: "Interlocking QB1 (open QA1 first)"},
	}
	// MEAS: measurements.
	meas := LD{Inst: "MEAS", Desc: "Measurements", LN0: ln0(ed1, "Measurements")}
	meas.LN0.DataSets = []DataSet{{Name: "dsMeas", Desc: "Feeder measurements", FCDAs: []FCDA{
		fcd("MEAS", "", "MMXU", "1", "TotW", "MX"), fcd("MEAS", "", "MMXU", "1", "TotVAr", "MX"),
		fcd("MEAS", "", "MMXU", "1", "Hz", "MX"), fcd("MEAS", "", "MMXU", "1", "A", "MX"),
		fcd("MEAS", "", "MMXU", "1", "PhV", "MX"), fcd("MEAS", "", "MSQI", "1", "SeqA", "MX")}}}
	meas.LN0.Reports = []Report{{
		Name: "urcbMeas", Desc: "Measurements to the station HMI", RptID: rptID(name, "MEAS", "urcbMeas"), DatSet: "dsMeas",
		ConfRev: 1, BufTime: 100, IntgPd: 10000, Trg: Trg{Dchg: true, Qchg: true, Period: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	meas.LNs = []LN{
		{Class: "MMXU", Inst: "1", Type: "MMXU_A", Desc: "Feeder measurement", DOIs: []DOI{
			{Name: "TotW", DAIs: []DAI{V("db", "2000")}}, {Name: "TotVAr", DAIs: []DAI{V("db", "2000")}}, {Name: "Hz", DAIs: []DAI{V("db", "100")}}}},
		{Class: "MSQI", Inst: "1", Type: "MSQI_A", Desc: "Sequence currents"},
	}
	d.LDs = []LD{ld0, prot, ctrl, meas}
	d.History = []Hitem{{"1", "A", "2026-10-02T15:10:00Z", "j.devries", "CID from the system configuration", "Detailed design"}, {"3", "B", "2026-11-03T09:20:00Z", "m.okafor", "GOOSE addresses updated", "FAT findings"}}
	return d
}

// T1 is the transformer protection and control: differential and
// overcurrent protection, tap changer and voltage control. It subscribes
// the feeders' trips (blocking of the LV overcurrent stage) and the
// busbar protection's breaker failure (intertrip of the LV breaker). Its
// tap changer carries data objects of the (made-up) vendor namespace.
func T1() *IED {
	name := T1Name
	d := &IED{
		Name: name, Desc: "Transformer T1 protection and control", Manufacturer: Manufacturer, Type: "TPC-640",
		ConfigVer: "2.0", AP: "S1", IP: "10.20.1.21", MAC: "02:61:85:01:00:21", Port: 10203, Services: serverServices,
	}
	ld0 := LD{Inst: "LD0", Desc: "Device", LN0: ln0(false, "Transformer T1"), LNs: []LN{
		{Class: "LPHD", Inst: "1", Type: "LPHD_A", DOIs: []DOI{phyNam(d.Type, "TPC640-260311")}},
	}}
	prot := LD{Inst: "PROT", Desc: "Protection", LN0: ln0(false, "Protection")}
	prot.LN0.DataSets = []DataSet{{Name: "dsTrip", Desc: "Protection starts and trips", FCDAs: []FCDA{
		fcd("PROT", "", "PDIF", "1", "Str", "ST"), fcd("PROT", "", "PDIF", "1", "Op", "ST"),
		fcd("PROT", "", "PTOC", "1", "Str", "ST"), fcd("PROT", "", "PTOC", "1", "Op", "ST"),
		fcd("PROT", "", "PTRC", "1", "Tr", "ST")}}}
	prot.LN0.Reports = []Report{{
		Name: "brcbTrip", Desc: "Protection events to the station HMI", RptID: rptID(name, "PROT", "brcbTrip"), DatSet: "dsTrip",
		ConfRev: 1, Buffered: true, BufTime: 50, Trg: Trg{Dchg: true, Qchg: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, DataRef: true, EntryID: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	prot.LNs = []LN{
		{Class: "PDIF", Inst: "1", Type: "PDIF_A", Desc: "Transformer differential (87T)", DOIs: []DOI{{Name: "LoSet", DAIs: []DAI{SG("setMag.f", "0.3")}}}},
		{Class: "PTOC", Inst: "1", Type: "PTOC_A", Desc: "LV overcurrent (51), blocked by feeder starts", DOIs: []DOI{
			{Name: "StrVal", DAIs: []DAI{SG("setMag.f", "1400")}}, {Name: "OpDlTmms", DAIs: []DAI{SG("setVal", "700")}}},
			Inputs: []ExtRef{
				{IED: F1Name, LD: "PROT", Class: "PTRC", Inst: "1", DO: "Tr", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbTrip", IntAddr: "BlkF1", Desc: "Feeder 1 trip blocks LV overcurrent"},
				{IED: F2Name, LD: "PROT", Class: "PTRC", Inst: "1", DO: "Tr", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbTrip", IntAddr: "BlkF2", Desc: "Feeder 2 trip blocks LV overcurrent"},
			}},
		{Class: "PTRC", Inst: "1", Type: "PTRC_A", Desc: "Trip conditioning", Inputs: []ExtRef{
			{IED: BBName, LD: "PROT", Class: "RBRF", Inst: "1", DO: "OpEx", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbBF", IntAddr: "BFTrip1", Desc: "Breaker failure feeder 1: trip LV breaker"},
			{IED: BBName, LD: "PROT", Class: "RBRF", Inst: "2", DO: "OpEx", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbBF", IntAddr: "BFTrip2", Desc: "Breaker failure feeder 2: trip LV breaker"},
		}},
	}
	ctrl := LD{Inst: "CTRL", Desc: "Control", LN0: ln0(false, "Control")}
	ctrl.LN0.DataSets = []DataSet{{Name: "dsTap", Desc: "Tap changer and breakers", FCDAs: []FCDA{
		fcd("CTRL", "", "YLTC", "1", "TapPos", "ST"), fcd("CTRL", "", "ATCC", "1", "Auto", "ST"),
		fcd("CTRL", "", "XCBR", "1", "Pos", "ST"), fcd("CTRL", "", "XCBR", "2", "Pos", "ST"),
		fcd("CTRL", "", "YLTC", "1", "MotDrvTmp", "MX")}}}
	ctrl.LN0.Reports = []Report{{
		Name: "urcbTap", Desc: "Tap changer to the station HMI", RptID: rptID(name, "CTRL", "urcbTap"), DatSet: "dsTap",
		ConfRev: 1, BufTime: 100, Trg: Trg{Dchg: true, Qchg: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	ctrl.LNs = []LN{
		{Class: "YPTR", Inst: "1", Type: "YPTR_A", Desc: "Power transformer T1 110/20 kV 40 MVA"},
		{Class: "YLTC", Inst: "1", Type: "YLTC_CV", Desc: "On-load tap changer", DOIs: []DOI{
			{Name: "NamPlt", DAIs: []DAI{V("vendor", "Corvane Energy (fictitious)"), V("swRev", "5.2"), V("d", "Tap changer with motor drive supervision"), V("lnNs", VendorNS)}}}},
		{Class: "ATCC", Inst: "1", Type: "ATCC_A", Desc: "Automatic voltage control", DOIs: []DOI{
			{Name: "BndCtr", DAIs: []DAI{SG("setMag.f", "20.5")}}, {Name: "BndWid", DAIs: []DAI{SG("setMag.f", "1.5")}}}},
		{Class: "XCBR", Inst: "1", Type: "XCBR_A", Desc: "HV circuit breaker E1Q01 QA1"},
		{Class: "XCBR", Inst: "2", Type: "XCBR_A", Desc: "LV incomer circuit breaker D1Q01 QA1"},
	}
	meas := LD{Inst: "MEAS", Desc: "Measurements", LN0: ln0(false, "Measurements")}
	meas.LN0.DataSets = []DataSet{{Name: "dsMeas", Desc: "Transformer measurements", FCDAs: []FCDA{
		fcd("MEAS", "", "MMXU", "1", "TotW", "MX"), fcd("MEAS", "", "MMXU", "1", "A", "MX"),
		fcd("MEAS", "", "MMXU", "2", "TotW", "MX"), fcd("MEAS", "", "MMXU", "2", "Hz", "MX"), fcd("MEAS", "", "MMXU", "2", "A", "MX")}}}
	meas.LN0.Reports = []Report{{
		Name: "urcbMeas", Desc: "Measurements to the station HMI", RptID: rptID(name, "MEAS", "urcbMeas"), DatSet: "dsMeas",
		ConfRev: 1, BufTime: 100, IntgPd: 10000, Trg: Trg{Dchg: true, Period: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	meas.LNs = []LN{
		{Class: "MMXU", Inst: "1", Type: "MMXU_A", Desc: "HV side"},
		{Class: "MMXU", Inst: "2", Type: "MMXU_A", Desc: "LV side"},
	}
	d.LDs = []LD{ld0, prot, ctrl, meas}
	d.History = []Hitem{{"2", "A", "2026-10-02T15:30:00Z", "j.devries", "CID from the system configuration", "Detailed design"}}
	return d
}

// Busbar is the 20 kV busbar protection with breaker failure for both
// feeders. It subscribes the feeders' trips (start of breaker failure)
// and the merging unit's sampled values, and publishes breaker failure.
func Busbar() *IED {
	name := BBName
	d := &IED{
		Name: name, Desc: "20 kV busbar and breaker failure protection", Manufacturer: Manufacturer, Type: "BPC-670",
		ConfigVer: "1.4", AP: "S1", IP: "10.20.1.41", MAC: "02:61:85:01:00:41", Port: 10204, Services: serverServices,
	}
	ld0 := LD{Inst: "LD0", Desc: "Device", LN0: ln0(false, "Busbar protection"), LNs: []LN{
		{Class: "LPHD", Inst: "1", Type: "LPHD_A", DOIs: []DOI{phyNam(d.Type, "BPC670-260077")}},
	}}
	prot := LD{Inst: "PROT", Desc: "Protection", LN0: ln0(false, "Protection")}
	prot.LN0.DataSets = []DataSet{
		{Name: "dsGoBF", Desc: "Breaker failure for GOOSE", FCDAs: []FCDA{
			fcda("PROT", "", "RBRF", "1", "OpEx", "general", "ST"), fcda("PROT", "", "RBRF", "1", "OpEx", "q", "ST"),
			fcda("PROT", "", "RBRF", "2", "OpEx", "general", "ST"), fcda("PROT", "", "RBRF", "2", "OpEx", "q", "ST")}},
		{Name: "dsTrip", Desc: "Busbar protection and breaker failure events", FCDAs: []FCDA{
			fcd("PROT", "", "PDIF", "1", "Op", "ST"),
			fcd("PROT", "", "RBRF", "1", "OpEx", "ST"), fcd("PROT", "", "RBRF", "1", "OpIn", "ST"),
			fcd("PROT", "", "RBRF", "2", "OpEx", "ST"), fcd("PROT", "", "RBRF", "2", "OpIn", "ST"),
			fcd("PROT", "", "PTRC", "1", "Tr", "ST")}},
	}
	prot.LN0.Reports = []Report{{
		Name: "brcbTrip", Desc: "Protection events to the station HMI", RptID: rptID(name, "PROT", "brcbTrip"), DatSet: "dsTrip",
		ConfRev: 1, Buffered: true, BufTime: 50, Trg: Trg{Dchg: true, Qchg: true, GI: true},
		Opt: Opt{SeqNum: true, DataSet: true, ReasonCode: true, DataRef: true, EntryID: true, ConfigRef: true}, Max: 2, Clients: hmiClient(),
	}}
	prot.LN0.GSE = []GSEControl{{
		Name: "gcbBF", Desc: "Breaker failure to the transformer protection", GoID: name + "_BF", DatSet: "dsGoBF", ConfRev: 1,
		MAC: "01-0C-CD-01-00-41", APPID: 0x0401, VLAN: GOOSEVLAN, Prio: 6, MinTime: 2, MaxTime: 1000,
	}}
	smv := func(i int) ExtRef {
		return ExtRef{IED: MUName, LD: "MU01", Prefix: "I", Class: "TCTR", Inst: fmt.Sprint(i), DO: "AmpSv", DA: "instMag.i", Service: "SMV", SrcLD: "MU01", SrcCB: "MSVCB01", IntAddr: fmt.Sprintf("IF1L%d", i), Desc: fmt.Sprintf("Feeder 1 current phase %c", 'A'+i-1)}
	}
	prot.LNs = []LN{
		{Class: "PDIF", Inst: "1", Type: "PDIF_A", Desc: "Busbar differential (87B)", DOIs: []DOI{{Name: "LoSet", DAIs: []DAI{SG("setMag.f", "1200")}}},
			Inputs: []ExtRef{smv(1), smv(2), smv(3)}},
		{Class: "RBRF", Inst: "1", Type: "RBRF_A", Desc: "Breaker failure feeder 1 (50BF)", DOIs: []DOI{{Name: "FailTmms", DAIs: []DAI{SG("setVal", "150")}}},
			Inputs: []ExtRef{{IED: F1Name, LD: "PROT", Class: "PTRC", Inst: "1", DO: "Tr", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbTrip", IntAddr: "StrBF1", Desc: "Feeder 1 trip starts breaker failure"}}},
		{Class: "RBRF", Inst: "2", Type: "RBRF_A", Desc: "Breaker failure feeder 2 (50BF)", DOIs: []DOI{{Name: "FailTmms", DAIs: []DAI{SG("setVal", "150")}}},
			Inputs: []ExtRef{{IED: F2Name, LD: "PROT", Class: "PTRC", Inst: "1", DO: "Tr", DA: "general", Service: "GOOSE", SrcLD: "PROT", SrcCB: "gcbTrip", IntAddr: "StrBF2", Desc: "Feeder 2 trip starts breaker failure"}}},
		{Class: "PTRC", Inst: "1", Type: "PTRC_A", Desc: "Trip conditioning"},
	}
	d.LDs = []LD{ld0, prot}
	d.History = []Hitem{{"3", "B", "2026-11-03T09:40:00Z", "m.okafor", "CID from the system configuration", "FAT findings"}}
	return d
}

// MU is the merging unit of feeder 1: four currents and four voltages
// as Sampled Values in the UCA 9-2LE profile (80 samples per cycle), and
// a status GOOSE.
func MU() *IED {
	name := MUName
	d := &IED{
		Name: name, Desc: "Merging unit feeder 1 (UCA 9-2LE)", Manufacturer: Manufacturer, Type: "MU-920",
		ConfigVer: "1.1", AP: "S1", IP: "10.20.1.51", MAC: "02:61:85:01:00:51", Port: 10205,
		Services: append(append([]string(nil), serverServices[2:9]...), `<GetCBValues/>`, `<GSESettings appID="Conf" dataLabel="Fix"/>`, `<SMVSettings svID="Conf" optFields="Fix" smpRate="Fix"><SmpRate>80</SmpRate></SMVSettings>`, `<GOOSE max="1"/>`, `<SMVsc max="1"/>`),
	}
	ln0mu := ln0(false, "Merging unit")
	var phs []FCDA
	for i := 1; i <= 4; i++ {
		phs = append(phs, fcda("MU01", "I", "TCTR", fmt.Sprint(i), "AmpSv", "instMag.i", "MX"), fcda("MU01", "I", "TCTR", fmt.Sprint(i), "AmpSv", "q", "MX"))
	}
	for i := 1; i <= 4; i++ {
		phs = append(phs, fcda("MU01", "U", "TVTR", fmt.Sprint(i), "VolSv", "instMag.i", "MX"), fcda("MU01", "U", "TVTR", fmt.Sprint(i), "VolSv", "q", "MX"))
	}
	ln0mu.DataSets = []DataSet{
		{Name: "PhsMeas1", Desc: "UCA 9-2LE: 4 currents, 4 voltages", FCDAs: phs},
		{Name: "dsStatus", Desc: "Merging unit status for GOOSE", FCDAs: []FCDA{
			fcda("MU01", "", "LPHD", "1", "PhyHealth", "stVal", "ST"), fcda("MU01", "", "LPHD", "1", "PhyHealth", "q", "ST"),
			fcda("MU01", "", "LLN0", "", "Beh", "stVal", "ST")}},
	}
	ln0mu.SMV = []SMVControl{{
		Name: "MSVCB01", Desc: "Feeder 1 currents and voltages", SmvID: name + "MU01", DatSet: "PhsMeas1", ConfRev: 1,
		SmpRate: 80, NofASDU: 1, MAC: "01-0C-CD-04-00-01", APPID: 0x4001, VLAN: SVVLAN, Prio: 4,
	}}
	ln0mu.GSE = []GSEControl{{
		Name: "gcbStatus", Desc: "Merging unit health", GoID: name + "_Status", DatSet: "dsStatus", ConfRev: 1,
		MAC: "01-0C-CD-01-00-51", APPID: 0x0501, VLAN: GOOSEVLAN, Prio: 4, MinTime: 4, MaxTime: 1000,
	}}
	lns := []LN{{Class: "LPHD", Inst: "1", Type: "LPHD_A", DOIs: []DOI{phyNam(d.Type, "MU920-260142")}}}
	for i := 1; i <= 4; i++ {
		desc := fmt.Sprintf("Current phase %c", "ABCN"[i-1])
		lns = append(lns, LN{Prefix: "I", Class: "TCTR", Inst: fmt.Sprint(i), Type: "TCTR_LE", Desc: desc, DOIs: []DOI{
			{Name: "HzRtg", DAIs: []DAI{V("setMag.f", "50")}}, {Name: "ARtg", DAIs: []DAI{V("setMag.f", "600")}}}})
	}
	for i := 1; i <= 4; i++ {
		desc := fmt.Sprintf("Voltage phase %c", "ABCN"[i-1])
		lns = append(lns, LN{Prefix: "U", Class: "TVTR", Inst: fmt.Sprint(i), Type: "TVTR_LE", Desc: desc, DOIs: []DOI{
			{Name: "HzRtg", DAIs: []DAI{V("setMag.f", "50")}}, {Name: "VRtg", DAIs: []DAI{V("setMag.f", "11547")}}}})
	}
	d.LDs = []LD{{Inst: "MU01", Desc: "Merging unit", LN0: ln0mu, LNs: lns}}
	d.History = []Hitem{{"3", "B", "2026-11-03T09:50:00Z", "m.okafor", "CID from the system configuration", "FAT findings"}}
	return d
}
