// SPDX-License-Identifier: MIT

package site

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Tool is the toolID of the SCL headers.
const Tool = "OT Fabric demo generator 1.0"

// xw writes indented XML.
type xw struct {
	b     strings.Builder
	depth int
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), "&#34;", "&quot;")
}

func (w *xw) attrs(kv []string) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "" && !strings.HasPrefix(kv[i], "!") {
			continue
		}
		name := strings.TrimPrefix(kv[i], "!")
		fmt.Fprintf(&b, ` %s="%s"`, name, esc(kv[i+1]))
	}
	return b.String()
}

func (w *xw) line(s string) {
	w.b.WriteString(strings.Repeat("\t", w.depth))
	w.b.WriteString(s)
	w.b.WriteString("\n")
}

// open writes a start tag; attributes with an empty value are left out
// unless their name starts with "!".
func (w *xw) open(tag string, kv ...string) {
	w.line("<" + tag + w.attrs(kv) + ">")
	w.depth++
}

func (w *xw) close(tag string) {
	w.depth--
	w.line("</" + tag + ">")
}

func (w *xw) leaf(tag string, kv ...string) { w.line("<" + tag + w.attrs(kv) + "/>") }

func (w *xw) text(tag, text string, kv ...string) {
	w.line("<" + tag + w.attrs(kv) + ">" + esc(text) + "</" + tag + ">")
}

func b2s(b bool) string {
	if b {
		return "true"
	}
	return ""
}

// SCD returns the substation configuration description: every IED,
// the substation section, the communication section and all templates.
func SCD() []byte {
	w := &xw{}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.open("SCL", "xmlns", "http://www.iec.ch/61850/2003/SCL", "xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance",
		"version", "2007", "revision", "B", "release", "4")
	w.open("Header", "id", "AA1_Northfield", "version", "3", "revision", "B", "toolID", Tool, "nameStructure", "IEDName")
	w.text("Text", "AA1 Northfield 110/20 kV substation: system configuration. Generated demo data, not a real installation.")
	w.open("History")
	for _, h := range hist {
		w.leaf("Hitem", "version", h.Version, "revision", h.Revision, "when", h.When, "who", h.Who, "what", h.What, "why", h.Why)
	}
	w.close("History")
	w.close("Header")
	writeSubstation(w)
	ieds := IEDs()
	writeCommunication(w, ieds)
	for _, d := range ieds {
		writeIED(w, d, false)
	}
	writeTemplates(w, ieds)
	w.close("SCL")
	return []byte(w.b.String())
}

// CID returns the configured IED description of one IED, consistent with
// the SCD: its own communication section, its IED section and the
// templates it uses. An Edition 1 IED gets a schema 1.7 file.
func CID(d *IED) []byte {
	w := &xw{}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	if d.Ed1 {
		w.open("SCL", "xmlns", "http://www.iec.ch/61850/2003/SCL", "xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance",
			"xsi:schemaLocation", "http://www.iec.ch/61850/2003/SCL SCL.xsd")
	} else {
		w.open("SCL", "xmlns", "http://www.iec.ch/61850/2003/SCL", "xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance",
			"version", "2007", "revision", "B", "release", "4")
	}
	hv := d.History[len(d.History)-1]
	w.open("Header", "id", d.Name, "version", hv.Version, "revision", hv.Revision, "toolID", Tool, "nameStructure", "IEDName")
	w.open("History")
	for _, h := range d.History {
		w.leaf("Hitem", "version", h.Version, "revision", h.Revision, "when", h.When, "who", h.Who, "what", h.What, "why", h.Why)
	}
	w.close("History")
	w.close("Header")
	writeCommunication(w, []*IED{d})
	writeIED(w, d, true)
	writeTemplates(w, []*IED{d})
	w.close("SCL")
	return []byte(w.b.String())
}

func writeSubstation(w *xw) {
	ln := func(ied, ld, prefix, class, inst string) {
		w.leaf("LNode", "iedName", ied, "ldInst", ld, "prefix", prefix, "lnClass", class, "!lnInst", inst)
	}
	cn := func(vl, bay, name string) string { return Substation + "/" + vl + "/" + bay + "/" + name }
	term := func(name, vl, bay, node string) {
		w.leaf("Terminal", "name", name, "connectivityNode", cn(vl, bay, node), "substationName", Substation, "voltageLevelName", vl, "bayName", bay, "cNodeName", node)
	}
	w.open("Substation", "name", Substation, "desc", SubstationDesc)
	w.open("PowerTransformer", "name", "T1", "type", "PTR", "desc", "110/20 kV 40 MVA YNd11")
	ln(T1Name, "CTRL", "", "YPTR", "1")
	ln(T1Name, "PROT", "", "PDIF", "1")
	w.open("TransformerWinding", "name", "W1", "type", "PTW", "desc", "110 kV winding")
	term("T1", "E1", "Q01", "CN2")
	w.open("TapChanger", "name", "LTC", "type", "LTC")
	ln(T1Name, "CTRL", "", "YLTC", "1")
	ln(T1Name, "CTRL", "", "ATCC", "1")
	w.close("TapChanger")
	w.close("TransformerWinding")
	w.open("TransformerWinding", "name", "W2", "type", "PTW", "desc", "20 kV winding")
	term("T1", "D1", "Q01", "CN2")
	w.close("TransformerWinding")
	w.close("PowerTransformer")

	w.open("VoltageLevel", "name", "E1", "desc", "110 kV")
	w.text("Voltage", "110", "unit", "V", "multiplier", "k")
	w.open("Bay", "name", "Q01", "desc", "Transformer T1 HV")
	ln(T1Name, "LD0", "", "LLN0", "")
	w.open("ConductingEquipment", "name", "QA1", "type", "CBR", "desc", "HV circuit breaker")
	ln(T1Name, "CTRL", "", "XCBR", "1")
	term("T1", "E1", "Q01", "CN1")
	term("T2", "E1", "Q01", "CN2")
	w.close("ConductingEquipment")
	w.leaf("ConnectivityNode", "name", "CN1", "pathName", cn("E1", "Q01", "CN1"))
	w.leaf("ConnectivityNode", "name", "CN2", "pathName", cn("E1", "Q01", "CN2"))
	w.close("Bay")
	w.close("VoltageLevel")

	w.open("VoltageLevel", "name", "D1", "desc", "20 kV")
	w.text("Voltage", "20", "unit", "V", "multiplier", "k")
	w.open("Bay", "name", "W1", "desc", "Busbar 1")
	ln(BBName, "PROT", "", "PDIF", "1")
	w.leaf("ConnectivityNode", "name", "CN1", "pathName", cn("D1", "W1", "CN1"))
	w.close("Bay")
	w.open("Bay", "name", "Q01", "desc", "Transformer T1 LV incomer")
	w.open("ConductingEquipment", "name", "QA1", "type", "CBR", "desc", "LV incomer circuit breaker")
	ln(T1Name, "CTRL", "", "XCBR", "2")
	term("T1", "D1", "Q01", "CN2")
	term("T2", "D1", "W1", "CN1")
	w.close("ConductingEquipment")
	w.leaf("ConnectivityNode", "name", "CN2", "pathName", cn("D1", "Q01", "CN2"))
	w.close("Bay")
	for _, f := range []struct{ ied, bay, desc string }{{F1Name, "Q02", "Feeder 1 Riverside"}, {F2Name, "Q03", "Feeder 2 Hillcrest"}} {
		w.open("Bay", "name", f.bay, "desc", f.desc)
		ln(f.ied, "PROT", "", "PTOC", "1")
		ln(f.ied, "PROT", "", "PTOC", "2")
		ln(f.ied, "PROT", "", "PTRC", "1")
		ln(f.ied, "MEAS", "", "MMXU", "1")
		ln(BBName, "PROT", "", "RBRF", map[string]string{"Q02": "1", "Q03": "2"}[f.bay])
		w.open("ConductingEquipment", "name", "QB1", "type", "DIS", "desc", "Busbar disconnector")
		ln(f.ied, "CTRL", "", "CSWI", "2")
		ln(f.ied, "CTRL", "", "XSWI", "1")
		ln(f.ied, "CTRL", "", "CILO", "2")
		term("T1", "D1", "W1", "CN1")
		term("T2", "D1", f.bay, "CN1")
		w.close("ConductingEquipment")
		w.open("ConductingEquipment", "name", "QA1", "type", "CBR", "desc", "Feeder circuit breaker")
		ln(f.ied, "CTRL", "", "CSWI", "1")
		ln(f.ied, "CTRL", "", "XCBR", "1")
		ln(f.ied, "CTRL", "", "CILO", "1")
		term("T1", "D1", f.bay, "CN1")
		term("T2", "D1", f.bay, "CN2")
		w.close("ConductingEquipment")
		if f.ied == F1Name {
			w.open("ConductingEquipment", "name", "BI1", "type", "CTR", "desc", "Current transformer 600/1 A")
			for i := 1; i <= 4; i++ {
				ln(MUName, "MU01", "I", "TCTR", fmt.Sprint(i))
			}
			term("T1", "D1", f.bay, "CN2")
			w.close("ConductingEquipment")
			w.open("ConductingEquipment", "name", "BU1", "type", "VTR", "desc", "Voltage transformer 20/0.1 kV")
			for i := 1; i <= 4; i++ {
				ln(MUName, "MU01", "U", "TVTR", fmt.Sprint(i))
			}
			term("T1", "D1", f.bay, "CN2")
			w.close("ConductingEquipment")
		}
		w.leaf("ConnectivityNode", "name", "CN1", "pathName", cn("D1", f.bay, "CN1"))
		w.leaf("ConnectivityNode", "name", "CN2", "pathName", cn("D1", f.bay, "CN2"))
		w.close("Bay")
	}
	w.close("VoltageLevel")
	w.close("Substation")
}

func p(w *xw, typ, v string) {
	// xsi:type comes first: go-iec61850 v1.16.0 takes a later xsi:type
	// for the P type (see README, gaps).
	w.text("P", v, "xsi:type", "tP_"+typ, "type", typ)
}

func writeCommunication(w *xw, ieds []*IED) {
	w.open("Communication")
	w.open("SubNetwork", "name", SubNetwork, "type", "8-MMS", "desc", "Station bus (MMS, GOOSE and Sampled Values)")
	w.text("BitRate", "100", "unit", "b/s", "multiplier", "M")
	for _, d := range ieds {
		w.open("ConnectedAP", "iedName", d.Name, "apName", d.AP)
		w.open("Address")
		p(w, "IP", d.IP)
		p(w, "IP-SUBNET", "255.255.255.0")
		p(w, "IP-GATEWAY", Gateway)
		p(w, "OSI-AP-Title", "1,3,9999,33")
		p(w, "OSI-AE-Qualifier", "33")
		p(w, "OSI-PSEL", "00000001")
		p(w, "OSI-SSEL", "0001")
		p(w, "OSI-TSEL", "0001")
		w.close("Address")
		for _, ld := range d.LDs {
			for _, g := range ld.LN0.GSE {
				w.open("GSE", "ldInst", ld.Inst, "cbName", g.Name)
				w.open("Address")
				p(w, "MAC-Address", g.MAC)
				p(w, "APPID", fmt.Sprintf("%04X", g.APPID))
				p(w, "VLAN-ID", fmt.Sprintf("%03X", g.VLAN))
				p(w, "VLAN-PRIORITY", fmt.Sprint(g.Prio))
				w.close("Address")
				w.text("MinTime", fmt.Sprint(g.MinTime), "unit", "s", "multiplier", "m")
				w.text("MaxTime", fmt.Sprint(g.MaxTime), "unit", "s", "multiplier", "m")
				w.close("GSE")
			}
			for _, s := range ld.LN0.SMV {
				w.open("SMV", "ldInst", ld.Inst, "cbName", s.Name)
				w.open("Address")
				p(w, "MAC-Address", s.MAC)
				p(w, "APPID", fmt.Sprintf("%04X", s.APPID))
				p(w, "VLAN-ID", fmt.Sprintf("%03X", s.VLAN))
				p(w, "VLAN-PRIORITY", fmt.Sprint(s.Prio))
				w.close("Address")
				w.close("SMV")
			}
		}
		w.open("PhysConn", "type", "Connection")
		p(w, "Type", "FOC")
		p(w, "Plug", "LC")
		w.close("PhysConn")
		w.close("ConnectedAP")
	}
	w.close("SubNetwork")
	w.close("Communication")
}

func writeIED(w *xw, d *IED, cid bool) {
	if d.Ed1 && cid {
		w.open("IED", "name", d.Name, "desc", d.Desc, "type", d.Type, "manufacturer", d.Manufacturer, "configVersion", d.ConfigVer)
	} else if d.Ed1 {
		w.open("IED", "name", d.Name, "desc", d.Desc, "type", d.Type, "manufacturer", d.Manufacturer, "configVersion", d.ConfigVer,
			"originalSclVersion", "2003", "originalSclRevision", "A")
	} else {
		w.open("IED", "name", d.Name, "desc", d.Desc, "type", d.Type, "manufacturer", d.Manufacturer, "configVersion", d.ConfigVer,
			"originalSclVersion", "2007", "originalSclRevision", "B", "originalSclRelease", "4")
	}
	w.open("Services")
	for _, s := range d.Services {
		w.line(s)
	}
	w.close("Services")
	w.open("AccessPoint", "name", d.AP, "desc", "Station bus")
	w.open("Server")
	w.leaf("Authentication", "none", "true")
	for _, ld := range d.LDs {
		w.open("LDevice", "inst", ld.Inst, "desc", ld.Desc)
		writeLN0(w, d.Ed1 && cid, &ld)
		for _, ln := range ld.LNs {
			if len(ln.DOIs) == 0 && len(ln.Inputs) == 0 {
				w.leaf("LN", "prefix", ln.Prefix, "lnClass", ln.Class, "inst", ln.Inst, "lnType", ln.Type, "desc", ln.Desc)
				continue
			}
			w.open("LN", "prefix", ln.Prefix, "lnClass", ln.Class, "inst", ln.Inst, "lnType", ln.Type, "desc", ln.Desc)
			for _, doi := range ln.DOIs {
				writeDOI(w, doi)
			}
			writeInputs(w, ln.Inputs, d.Ed1 && cid)
			w.close("LN")
		}
		w.close("LDevice")
	}
	w.close("Server")
	w.close("AccessPoint")
	w.close("IED")
}

// writeLN0 writes an LLN0; ed1 writes it in the form of SCL schema 1.7.
func writeLN0(w *xw, ed1 bool, ld *LD) {
	n := ld.LN0
	w.open("LN0", "lnClass", "LLN0", "!inst", "", "lnType", n.Type, "desc", n.Desc)
	for _, ds := range n.DataSets {
		w.open("DataSet", "name", ds.Name, "desc", ds.Desc)
		for _, f := range ds.FCDAs {
			w.leaf("FCDA", "ldInst", f.LD, "prefix", f.Prefix, "lnClass", f.Class, "lnInst", f.Inst, "doName", f.DO, "daName", f.DA, "fc", f.FC)
		}
		w.close("DataSet")
	}
	for _, r := range n.Reports {
		bufTime, intg := "", ""
		if r.BufTime > 0 {
			bufTime = fmt.Sprint(r.BufTime)
		}
		if r.IntgPd > 0 {
			intg = fmt.Sprint(r.IntgPd)
		}
		indexed := "false"
		if ed1 {
			indexed = ""
		}
		buffered := "false"
		if r.Buffered {
			buffered = "true"
		}
		w.open("ReportControl", "name", r.Name, "desc", r.Desc, "datSet", r.DatSet, "intgPd", intg, "rptID", r.RptID, "confRev", fmt.Sprint(r.ConfRev),
			"buffered", buffered, "bufTime", bufTime, "indexed", indexed)
		gi := b2s(r.Trg.GI)
		if ed1 {
			gi = "" // SCL 1.7 has no gi attribute: Edition 1 always offers GI
		}
		w.leaf("TrgOps", "dchg", b2s(r.Trg.Dchg), "qchg", b2s(r.Trg.Qchg), "dupd", b2s(r.Trg.Dupd), "period", b2s(r.Trg.Period), "gi", gi)
		o := r.Opt
		w.leaf("OptFields", "seqNum", b2s(o.SeqNum), "timeStamp", b2s(o.TimeStamp), "dataSet", b2s(o.DataSet), "reasonCode", b2s(o.ReasonCode),
			"dataRef", b2s(o.DataRef), "entryID", b2s(o.EntryID), "configRef", b2s(o.ConfigRef), "bufOvfl", b2s(o.BufOvfl))
		w.open("RptEnabled", "max", fmt.Sprint(r.Max))
		for _, c := range r.Clients {
			w.leaf("ClientLN", "iedName", c.IED, "ldInst", c.LD, "lnClass", c.Class, "lnInst", c.Inst, "apRef", c.AP)
		}
		w.close("RptEnabled")
		w.close("ReportControl")
	}
	for _, l := range n.Logs {
		w.open("LogControl", "name", l.Name, "desc", l.Desc, "datSet", l.DatSet, "logName", l.LogName, "logEna", "true", "reasonCode", "true")
		w.leaf("TrgOps", "dchg", b2s(l.Trg.Dchg), "qchg", b2s(l.Trg.Qchg))
		w.close("LogControl")
	}
	for _, doi := range n.DOIs {
		writeDOI(w, doi)
	}
	writeInputs(w, n.Inputs, ed1)
	if !ed1 {
		for _, l := range n.Logs {
			w.leaf("Log", "name", l.LogName)
		}
	}
	for _, g := range n.GSE {
		w.leaf("GSEControl", "name", g.Name, "desc", g.Desc, "datSet", g.DatSet, "confRev", fmt.Sprint(g.ConfRev), "appID", g.GoID, "type", "GOOSE")
	}
	for _, s := range n.SMV {
		w.open("SampledValueControl", "name", s.Name, "desc", s.Desc, "datSet", s.DatSet, "confRev", fmt.Sprint(s.ConfRev), "smvID", s.SmvID,
			"smpRate", fmt.Sprint(s.SmpRate), "nofASDU", fmt.Sprint(s.NofASDU), "multicast", "true", "smpMod", "SmpPerPeriod")
		w.leaf("SmvOpts", "sampleSynchronized", "true")
		w.close("SampledValueControl")
	}
	if n.SG != nil {
		w.leaf("SettingControl", "numOfSGs", fmt.Sprint(n.SG.NumOfSGs), "actSG", fmt.Sprint(n.SG.ActSG))
	}
	if ed1 {
		for _, l := range n.Logs {
			w.leaf("Log", "name", l.LogName)
		}
	}
	w.close("LN0")
}

func writeInputs(w *xw, refs []ExtRef, ed1 bool) {
	if len(refs) == 0 {
		return
	}
	w.open("Inputs")
	for _, r := range refs {
		if ed1 {
			w.leaf("ExtRef", "iedName", r.IED, "ldInst", r.LD, "prefix", r.Prefix, "lnClass", r.Class, "lnInst", r.Inst, "doName", r.DO, "daName", r.DA, "intAddr", r.IntAddr)
			continue
		}
		w.leaf("ExtRef", "desc", r.Desc, "iedName", r.IED, "ldInst", r.LD, "prefix", r.Prefix, "lnClass", r.Class, "lnInst", r.Inst, "doName", r.DO, "daName", r.DA,
			"intAddr", r.IntAddr, "serviceType", r.Service, "srcLDInst", r.SrcLD, "srcLNClass", "LLN0", "srcCBName", r.SrcCB)
	}
	w.close("Inputs")
}

// writeDOI writes a DOI; DAI paths with dots become SDIs.
func writeDOI(w *xw, doi DOI) {
	w.open("DOI", "name", doi.Name, "desc", doi.Desc)
	writeDAIs(w, doi.DAIs)
	w.close("DOI")
}

func writeDAIs(w *xw, dais []DAI) {
	var order []string
	groups := map[string][]DAI{}
	for _, d := range dais {
		head, rest, nested := strings.Cut(d.Path, ".")
		if !nested {
			w.open("DAI", "name", d.Path)
			for _, v := range d.Vals {
				sg := ""
				if v.SGroup > 0 {
					sg = fmt.Sprint(v.SGroup)
				}
				w.text("Val", v.V, "sGroup", sg)
			}
			w.close("DAI")
			continue
		}
		if _, ok := groups[head]; !ok {
			order = append(order, head)
		}
		groups[head] = append(groups[head], DAI{Path: rest, Vals: d.Vals})
	}
	for _, head := range order {
		w.open("SDI", "name", head)
		writeDAIs(w, groups[head])
		w.close("SDI")
	}
}

// writeTemplates writes the templates the IEDs use, in the order of the
// tables (an SCD with every IED gets every template).
func writeTemplates(w *xw, ieds []*IED) {
	lnUsed := map[string]bool{}
	for _, d := range ieds {
		for _, ld := range d.LDs {
			lnUsed[ld.LN0.Type] = true
			for _, ln := range ld.LNs {
				lnUsed[ln.Type] = true
			}
		}
	}
	doIdx := map[string]DOType{}
	for _, t := range DOTypes {
		doIdx[t.ID] = t
	}
	daIdx := map[string]DAType{}
	for _, t := range DATypes {
		daIdx[t.ID] = t
	}
	doUsed, daUsed, enUsed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var useDA func(id string)
	useDA = func(id string) {
		if daUsed[id] {
			return
		}
		daUsed[id] = true
		for _, b := range daIdx[id].BDAs {
			switch b.BType {
			case "Struct":
				useDA(b.Type)
			case "Enum":
				enUsed[b.Type] = true
			}
		}
	}
	var useDO func(id string)
	useDO = func(id string) {
		if doUsed[id] {
			return
		}
		t, ok := doIdx[id]
		if !ok {
			panic("site: unknown DOType " + id)
		}
		doUsed[id] = true
		for _, e := range t.Elems {
			switch {
			case e.SDO:
				useDO(e.Type)
			case e.BType == "Struct":
				useDA(e.Type)
			case e.BType == "Enum":
				enUsed[e.Type] = true
			}
		}
	}
	w.open("DataTypeTemplates")
	for _, t := range LNodeTypes {
		if !lnUsed[t.ID] {
			continue
		}
		w.open("LNodeType", "id", t.ID, "lnClass", t.Class, "desc", t.Desc)
		for _, d := range t.DOs {
			useDO(d.Type)
			w.leaf("DO", "name", d.Name, "type", d.Type, "desc", d.Desc)
		}
		w.close("LNodeType")
	}
	for _, t := range DOTypes {
		if !doUsed[t.ID] {
			continue
		}
		w.open("DOType", "id", t.ID, "cdc", t.CDC)
		for _, e := range t.Elems {
			if e.SDO {
				w.leaf("SDO", "name", e.Name, "type", e.Type)
				continue
			}
			kv := []string{"name", e.Name, "fc", e.FC, "bType", e.BType, "type", e.Type, "dchg", b2s(e.Dchg), "qchg", b2s(e.Qchg), "dupd", b2s(e.Dupd)}
			if e.Val == "" {
				w.leaf("DA", kv...)
				continue
			}
			w.open("DA", kv...)
			w.text("Val", e.Val)
			w.close("DA")
		}
		w.close("DOType")
	}
	for _, t := range DATypes {
		if !daUsed[t.ID] {
			continue
		}
		w.open("DAType", "id", t.ID)
		for _, b := range t.BDAs {
			w.leaf("BDA", "name", b.Name, "bType", b.BType, "type", b.Type)
		}
		w.close("DAType")
	}
	for _, t := range EnumTypes {
		if !enUsed[t.ID] {
			continue
		}
		w.open("EnumType", "id", t.ID)
		for _, v := range t.Vals {
			w.text("EnumVal", v.Name, "ord", fmt.Sprint(v.Ord))
		}
		w.close("EnumType")
	}
	w.close("DataTypeTemplates")
}
