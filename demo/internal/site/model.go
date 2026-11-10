// SPDX-License-Identifier: MIT

// Package site describes the demo substation, AA1 Northfield 110/20 kV,
// once: its IEDs, logical devices, data sets, control blocks, addresses
// and data type templates. The SCL files (the SCD and one CID per IED),
// the live IEDs and the captures are all built from it, so that names,
// addresses, APPIDs and data sets agree everywhere.
package site

// IED is one intelligent electronic device of the substation.
type IED struct {
	Name         string
	Desc         string
	Manufacturer string
	Type         string
	ConfigVer    string
	// Ed1 marks an IED engineered with an Edition 1 tool: its CID is
	// written in SCL schema 1.7 and its name plates declare no namespace.
	Ed1 bool
	// Client marks the station HMI: it has no MMS server of its own and
	// is not served live.
	Client bool
	AP     string // access point name
	IP     string
	// MAC is the station bus MAC address of the IED (not part of SCL for
	// MMS; used as the source of its frames in the captures).
	MAC string
	// Port is the TCP port the live IED listens on (127.0.0.1).
	Port     int
	Services []string // SCL Services elements, verbatim
	LDs      []LD
	// History is the IED's configuration history (CID Header).
	History []Hitem
}

// LD is a logical device.
type LD struct {
	Inst string
	Desc string
	LN0  LN0
	LNs  []LN
}

// LN is a logical node other than LLN0.
type LN struct {
	Prefix string
	Class  string
	Inst   string
	Type   string
	Desc   string
	DOIs   []DOI
	Inputs []ExtRef
}

// Name is the logical node's name (prefix, class, instance).
func (ln LN) Name() string { return ln.Prefix + ln.Class + ln.Inst }

// LN0 is a logical device's LLN0 with its control blocks.
type LN0 struct {
	Type     string
	Desc     string
	DOIs     []DOI
	DataSets []DataSet
	Reports  []Report
	Logs     []LogControl
	GSE      []GSEControl
	SMV      []SMVControl
	SG       *SettingControl
	Inputs   []ExtRef
}

// DOI is an instantiated data object with values.
type DOI struct {
	Name string
	Desc string
	DAIs []DAI
}

// DAI is a value of a data attribute, Path relative to the data object
// ("setMag.f", "ctlModel"). Vals with SGroup > 0 are setting group values.
type DAI struct {
	Path string
	Vals []Val
}

// Val is one SCL Val.
type Val struct {
	SGroup int
	V      string
}

// V is a DAI with one value.
func V(path, v string) DAI { return DAI{Path: path, Vals: []Val{{V: v}}} }

// SG is a DAI with one value per setting group.
func SG(path string, vs ...string) DAI {
	d := DAI{Path: path}
	for i, v := range vs {
		d.Vals = append(d.Vals, Val{SGroup: i + 1, V: v})
	}
	return d
}

// DataSet is an SCL DataSet.
type DataSet struct {
	Name  string
	Desc  string
	FCDAs []FCDA
}

// FCDA is a data set member. DA empty means the whole data object
// under the functional constraint (an FCD).
type FCDA struct {
	LD, Prefix, Class, Inst, DO, DA, FC string
}

// Report is a ReportControl.
type Report struct {
	Name     string
	Desc     string
	RptID    string
	DatSet   string
	ConfRev  int
	Buffered bool
	BufTime  int
	IntgPd   int
	Trg      Trg
	Opt      Opt
	Max      int
	Clients  []ClientLN
}

// Trg are trigger options.
type Trg struct{ Dchg, Qchg, Dupd, Period, GI bool }

// Opt are report optional fields.
type Opt struct{ SeqNum, TimeStamp, DataSet, ReasonCode, DataRef, EntryID, ConfigRef, BufOvfl bool }

// ClientLN is a client the RCB is configured for.
type ClientLN struct{ IED, LD, Class, Inst, AP string }

// LogControl is an SCL LogControl with its Log.
type LogControl struct {
	Name, Desc, DatSet, LogName string
	Trg                         Trg
}

// GSEControl is a GOOSE control block with its GSE address.
type GSEControl struct {
	Name    string
	Desc    string
	GoID    string // SCL appID
	DatSet  string
	ConfRev int
	MAC     string // 01-0C-CD-01-xx-xx
	APPID   uint16
	VLAN    uint16
	Prio    int
	MinTime int // ms
	MaxTime int // ms
}

// SMVControl is a sampled value control block with its SMV address.
type SMVControl struct {
	Name    string
	Desc    string
	SmvID   string
	DatSet  string
	ConfRev int
	SmpRate int
	NofASDU int
	MAC     string
	APPID   uint16
	VLAN    uint16
	Prio    int
}

// SettingControl is the setting group control block.
type SettingControl struct{ NumOfSGs, ActSG int }

// ExtRef is a subscribed input (Edition 2 form).
type ExtRef struct {
	IED, LD, Prefix, Class, Inst, DO, DA string
	Service                              string // GOOSE or SMV
	SrcLD, SrcCB                         string
	IntAddr                              string
	Desc                                 string
}

// Hitem is a Header history item.
type Hitem struct{ Version, Revision, When, Who, What, Why string }

// LDByInst returns the logical device with the instance name, or nil.
func (d *IED) LDByInst(inst string) *LD {
	for i := range d.LDs {
		if d.LDs[i].Inst == inst {
			return &d.LDs[i]
		}
	}
	return nil
}

// Domain is the MMS domain name of a logical device (IED name + inst).
func (d *IED) Domain(inst string) string { return d.Name + inst }
