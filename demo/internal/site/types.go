// SPDX-License-Identifier: MIT

package site

// The data type templates of the substation. Every IED takes the types
// it uses from here; the SCD holds all of them.

// LNodeType is an SCL LNodeType.
type LNodeType struct {
	ID, Class, Desc string
	DOs             []DORef
}

// DORef is a DO of an LNodeType.
type DORef struct{ Name, Type, Desc string }

// DOType is an SCL DOType: SDOs and DAs in order.
type DOType struct {
	ID, CDC string
	Elems   []DOElem
}

// DOElem is an SDO (SDO true) or a DA of a DOType.
type DOElem struct {
	SDO                   bool
	Name, FC, BType, Type string
	Dchg, Qchg, Dupd      bool
	Val                   string
}

// DAType is an SCL DAType.
type DAType struct {
	ID   string
	BDAs []BDA
}

// BDA is a basic data attribute of a DAType.
type BDA struct{ Name, BType, Type string }

// EnumType is an SCL EnumType.
type EnumType struct {
	ID   string
	Vals []EnumVal
}

// EnumVal is one literal.
type EnumVal struct {
	Ord  int
	Name string
}

func do(name, typ string) DORef { return DORef{Name: name, Type: typ} }

func sdo(name, typ string) DOElem { return DOElem{SDO: true, Name: name, Type: typ} }

func da(name, fc, btype string) DOElem { return DOElem{Name: name, FC: fc, BType: btype} }

func daT(name, fc, btype, typ string) DOElem {
	return DOElem{Name: name, FC: fc, BType: btype, Type: typ}
}

func dchg(e DOElem) DOElem { e.Dchg = true; return e }
func qchg(e DOElem) DOElem { e.Qchg = true; return e }
func val(e DOElem, v string) DOElem {
	e.Val = v
	return e
}

// status returns stVal, q and t of a status CDC.
func status(stVal DOElem) []DOElem {
	return []DOElem{dchg(stVal), qchg(da("q", "ST", "Quality")), da("t", "ST", "Timestamp")}
}

func cat(parts ...[]DOElem) []DOElem {
	var out []DOElem
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Vendor is the made-up vendor whose extension namespace the transformer
// IED declares. It is not a real company.
const (
	VendorNS     = "CORVANE 7-4X:2026A"
	IECNamespace = "IEC 61850-7-4:2007B"
)

// LNodeTypes of the substation.
var LNodeTypes = []LNodeType{
	{ID: "LLN0_A", Class: "LLN0", Desc: "Logical device root, Edition 2", DOs: []DORef{do("Mod", "ENC_Mod"), do("Beh", "ENS_Beh"), do("Health", "ENS_Health"), do("NamPlt", "LPL_LD")}},
	{ID: "LLN0_E1", Class: "LLN0", Desc: "Logical device root, Edition 1", DOs: []DORef{do("Mod", "ENC_Mod"), do("Beh", "ENS_Beh"), do("Health", "ENS_Health"), do("NamPlt", "LPL_E1")}},
	{ID: "LPHD_A", Class: "LPHD", DOs: []DORef{do("PhyNam", "DPL_A"), do("PhyHealth", "ENS_Health"), do("Proxy", "SPS_A")}},
	{ID: "GGIO_IND3", Class: "GGIO", Desc: "Three binary inputs", DOs: []DORef{do("Beh", "ENS_Beh"), {Name: "Ind1", Type: "SPS_A", Desc: "SF6 pressure low"}, {Name: "Ind2", Type: "SPS_A", Desc: "Spring charged"}, {Name: "Ind3", Type: "SPS_A", Desc: "Trip circuit supervision"}}},
	{ID: "PTOC_A", Class: "PTOC", DOs: []DORef{do("Beh", "ENS_Beh"), do("Str", "ACD_A"), do("Op", "ACT_A"), do("StrVal", "ASG_SG"), do("OpDlTmms", "ING_SG")}},
	{ID: "PTRC_A", Class: "PTRC", DOs: []DORef{do("Beh", "ENS_Beh"), do("Str", "ACD_A"), do("Op", "ACT_A"), do("Tr", "ACT_G")}},
	{ID: "PDIF_A", Class: "PDIF", DOs: []DORef{do("Beh", "ENS_Beh"), do("Str", "ACD_A"), do("Op", "ACT_A"), do("LoSet", "ASG_SG")}},
	{ID: "RBRF_A", Class: "RBRF", DOs: []DORef{do("Beh", "ENS_Beh"), do("Str", "ACD_A"), do("OpEx", "ACT_G"), do("OpIn", "ACT_G"), do("FailTmms", "ING_SG")}},
	{ID: "CSWI_SBOE", Class: "CSWI", Desc: "Switch controller, SBO with enhanced security", DOs: []DORef{do("Beh", "ENS_Beh"), do("Pos", "DPC_SBOE")}},
	{ID: "XCBR_A", Class: "XCBR", DOs: []DORef{do("Beh", "ENS_Beh"), do("Loc", "SPS_A"), do("OpCnt", "INS_A"), do("Pos", "DPC_ST"), do("BlkOpn", "SPC_ST"), do("BlkCls", "SPC_ST")}},
	{ID: "XSWI_A", Class: "XSWI", DOs: []DORef{do("Beh", "ENS_Beh"), do("Loc", "SPS_A"), do("OpCnt", "INS_A"), do("Pos", "DPC_ST"), do("BlkOpn", "SPC_ST"), do("BlkCls", "SPC_ST"), do("SwTyp", "ENS_SwTyp")}},
	{ID: "CILO_A", Class: "CILO", DOs: []DORef{do("Beh", "ENS_Beh"), do("EnaOpn", "SPS_A"), do("EnaCls", "SPS_A")}},
	{ID: "MMXU_A", Class: "MMXU", DOs: []DORef{do("Beh", "ENS_Beh"), do("TotW", "MV_A"), do("TotVAr", "MV_A"), do("Hz", "MV_A"), do("PhV", "WYE_A"), do("A", "WYE_A")}},
	{ID: "MSQI_A", Class: "MSQI", DOs: []DORef{do("Beh", "ENS_Beh"), do("SeqA", "SEQ_A")}},
	{ID: "YPTR_A", Class: "YPTR", DOs: []DORef{do("Beh", "ENS_Beh"), do("OpOvA", "SPS_A"), do("LodFact", "MV_A")}},
	{ID: "YLTC_CV", Class: "YLTC", Desc: "Tap changer with Corvane extension data objects", DOs: []DORef{do("Beh", "ENS_Beh"), do("NamPlt", "LPL_LN"), do("EndPosR", "SPS_A"), do("EndPosL", "SPS_A"), do("OpCnt", "INS_A"), do("TapPos", "ISC_A"), {Name: "MotDrvTmp", Type: "MV_VND", Desc: "Motor drive temperature (vendor extension)"}, {Name: "MotDrvA", Type: "MV_VND", Desc: "Motor drive current (vendor extension)"}}},
	{ID: "ATCC_A", Class: "ATCC", DOs: []DORef{do("Beh", "ENS_Beh"), do("Auto", "SPC_DIR"), do("TapPos", "ISC_A"), do("BndCtr", "ASG_SG"), do("BndWid", "ASG_SG")}},
	{ID: "TCTR_LE", Class: "TCTR", Desc: "Current transformer, UCA 9-2LE", DOs: []DORef{do("Beh", "ENS_Beh"), do("HzRtg", "ASG_SP"), do("ARtg", "ASG_SP"), do("AmpSv", "SAV_A")}},
	{ID: "TVTR_LE", Class: "TVTR", Desc: "Voltage transformer, UCA 9-2LE", DOs: []DORef{do("Beh", "ENS_Beh"), do("HzRtg", "ASG_SP"), do("VRtg", "ASG_SP"), do("VolSv", "SAV_A")}},
	{ID: "IHMI_A", Class: "IHMI", DOs: []DORef{do("Beh", "ENS_Beh")}},
}

// DOTypes of the substation.
var DOTypes = []DOType{
	{ID: "ENC_Mod", CDC: "ENC", Elems: cat(status(daT("stVal", "ST", "Enum", "BehaviourModeKind")), []DOElem{val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "status-only")})},
	{ID: "ENS_Beh", CDC: "ENS", Elems: status(daT("stVal", "ST", "Enum", "BehaviourModeKind"))},
	{ID: "ENS_Health", CDC: "ENS", Elems: status(daT("stVal", "ST", "Enum", "HealthKind"))},
	{ID: "ENS_SwTyp", CDC: "ENS", Elems: status(daT("stVal", "ST", "Enum", "SwitchFunctionKind"))},
	{ID: "LPL_LD", CDC: "LPL", Elems: []DOElem{da("vendor", "DC", "VisString255"), da("swRev", "DC", "VisString255"), da("d", "DC", "VisString255"), da("configRev", "DC", "VisString255"), val(da("ldNs", "EX", "VisString255"), IECNamespace)}},
	{ID: "LPL_E1", CDC: "LPL", Elems: []DOElem{da("vendor", "DC", "VisString255"), da("swRev", "DC", "VisString255"), da("d", "DC", "VisString255"), da("configRev", "DC", "VisString255")}},
	{ID: "LPL_LN", CDC: "LPL", Elems: []DOElem{da("vendor", "DC", "VisString255"), da("swRev", "DC", "VisString255"), da("d", "DC", "VisString255"), da("lnNs", "EX", "VisString255")}},
	{ID: "DPL_A", CDC: "DPL", Elems: []DOElem{da("vendor", "DC", "VisString255"), da("hwRev", "DC", "VisString255"), da("swRev", "DC", "VisString255"), da("serNum", "DC", "VisString255"), da("model", "DC", "VisString255"), da("location", "DC", "VisString255")}},
	{ID: "SPS_A", CDC: "SPS", Elems: status(da("stVal", "ST", "BOOLEAN"))},
	{ID: "INS_A", CDC: "INS", Elems: status(da("stVal", "ST", "INT32"))},
	{ID: "ACD_A", CDC: "ACD", Elems: []DOElem{
		dchg(da("general", "ST", "BOOLEAN")), dchg(daT("dirGeneral", "ST", "Enum", "FaultDirectionKind")),
		dchg(da("phsA", "ST", "BOOLEAN")), dchg(daT("dirPhsA", "ST", "Enum", "PhaseFaultDirectionKind")),
		dchg(da("phsB", "ST", "BOOLEAN")), dchg(daT("dirPhsB", "ST", "Enum", "PhaseFaultDirectionKind")),
		dchg(da("phsC", "ST", "BOOLEAN")), dchg(daT("dirPhsC", "ST", "Enum", "PhaseFaultDirectionKind")),
		qchg(da("q", "ST", "Quality")), da("t", "ST", "Timestamp")}},
	{ID: "ACT_A", CDC: "ACT", Elems: []DOElem{dchg(da("general", "ST", "BOOLEAN")), dchg(da("phsA", "ST", "BOOLEAN")), dchg(da("phsB", "ST", "BOOLEAN")), dchg(da("phsC", "ST", "BOOLEAN")), qchg(da("q", "ST", "Quality")), da("t", "ST", "Timestamp")}},
	{ID: "ACT_G", CDC: "ACT", Elems: status(da("general", "ST", "BOOLEAN"))},
	{ID: "MV_A", CDC: "MV", Elems: []DOElem{dchg(daT("mag", "MX", "Struct", "AnalogueValue")), qchg(da("q", "MX", "Quality")), da("t", "MX", "Timestamp"), da("db", "CF", "INT32U")}},
	{ID: "MV_VND", CDC: "MV", Elems: []DOElem{dchg(daT("mag", "MX", "Struct", "AnalogueValue")), qchg(da("q", "MX", "Quality")), da("t", "MX", "Timestamp"), val(da("dataNs", "EX", "VisString255"), VendorNS)}},
	{ID: "CMV_A", CDC: "CMV", Elems: []DOElem{dchg(daT("cVal", "MX", "Struct", "Vector")), qchg(da("q", "MX", "Quality")), da("t", "MX", "Timestamp")}},
	{ID: "WYE_A", CDC: "WYE", Elems: []DOElem{sdo("phsA", "CMV_A"), sdo("phsB", "CMV_A"), sdo("phsC", "CMV_A")}},
	{ID: "SEQ_A", CDC: "SEQ", Elems: []DOElem{sdo("c1", "CMV_A"), sdo("c2", "CMV_A"), sdo("c3", "CMV_A"), val(daT("seqT", "MX", "Enum", "SequenceKind"), "pos-neg-zero")}},
	{ID: "DPC_SBOE", CDC: "DPC", Elems: cat(status(da("stVal", "ST", "Dbpos")), []DOElem{
		val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "sbo-with-enhanced-security"),
		val(da("sboTimeout", "CF", "INT32U"), "30000"),
		daT("SBOw", "CO", "Struct", "DPCOperate"), daT("Oper", "CO", "Struct", "DPCOperate"), daT("Cancel", "CO", "Struct", "DPCCancel")})},
	{ID: "DPC_ST", CDC: "DPC", Elems: cat(status(da("stVal", "ST", "Dbpos")), []DOElem{val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "status-only")})},
	{ID: "SPC_ST", CDC: "SPC", Elems: cat(status(da("stVal", "ST", "BOOLEAN")), []DOElem{val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "status-only")})},
	{ID: "SPC_DIR", CDC: "SPC", Elems: cat(status(da("stVal", "ST", "BOOLEAN")), []DOElem{val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "direct-with-normal-security"), daT("Oper", "CO", "Struct", "SPCOperate")})},
	{ID: "ISC_A", CDC: "ISC", Elems: cat(status(daT("valWTr", "ST", "Struct", "ValWithTrans")), []DOElem{val(daT("ctlModel", "CF", "Enum", "CtlModelKind"), "status-only")})},
	{ID: "ASG_SG", CDC: "ASG", Elems: []DOElem{daT("setMag", "SG", "Struct", "AnalogueValue"), daT("minVal", "CF", "Struct", "AnalogueValue"), daT("maxVal", "CF", "Struct", "AnalogueValue"), daT("stepSize", "CF", "Struct", "AnalogueValue")}},
	{ID: "ASG_SP", CDC: "ASG", Elems: []DOElem{daT("setMag", "SP", "Struct", "AnalogueValue")}},
	{ID: "ING_SG", CDC: "ING", Elems: []DOElem{da("setVal", "SG", "INT32"), da("minVal", "CF", "INT32"), da("maxVal", "CF", "INT32"), da("stepSize", "CF", "INT32U")}},
	{ID: "SAV_A", CDC: "SAV", Elems: []DOElem{daT("instMag", "MX", "Struct", "AnalogueValue_I"), qchg(da("q", "MX", "Quality"))}},
}

// DATypes of the substation.
var DATypes = []DAType{
	{ID: "AnalogueValue", BDAs: []BDA{{"f", "FLOAT32", ""}}},
	{ID: "AnalogueValue_I", BDAs: []BDA{{"i", "INT32", ""}}},
	{ID: "Vector", BDAs: []BDA{{"mag", "Struct", "AnalogueValue"}, {"ang", "Struct", "AnalogueValue"}}},
	{ID: "Originator", BDAs: []BDA{{"orCat", "Enum", "OriginatorCategoryKind"}, {"orIdent", "Octet64", ""}}},
	{ID: "DPCOperate", BDAs: []BDA{{"ctlVal", "BOOLEAN", ""}, {"origin", "Struct", "Originator"}, {"ctlNum", "INT8U", ""}, {"T", "Timestamp", ""}, {"Test", "BOOLEAN", ""}, {"Check", "Check", ""}}},
	{ID: "DPCCancel", BDAs: []BDA{{"ctlVal", "BOOLEAN", ""}, {"origin", "Struct", "Originator"}, {"ctlNum", "INT8U", ""}, {"T", "Timestamp", ""}, {"Test", "BOOLEAN", ""}}},
	{ID: "SPCOperate", BDAs: []BDA{{"ctlVal", "BOOLEAN", ""}, {"origin", "Struct", "Originator"}, {"ctlNum", "INT8U", ""}, {"T", "Timestamp", ""}, {"Test", "BOOLEAN", ""}, {"Check", "Check", ""}}},
	{ID: "ValWithTrans", BDAs: []BDA{{"posVal", "INT8", ""}, {"transInd", "BOOLEAN", ""}}},
}

// EnumTypes of the substation.
var EnumTypes = []EnumType{
	{ID: "BehaviourModeKind", Vals: []EnumVal{{1, "on"}, {2, "on-blocked"}, {3, "test"}, {4, "test/blocked"}, {5, "off"}}},
	{ID: "HealthKind", Vals: []EnumVal{{1, "Ok"}, {2, "Warning"}, {3, "Alarm"}}},
	{ID: "CtlModelKind", Vals: []EnumVal{{0, "status-only"}, {1, "direct-with-normal-security"}, {2, "sbo-with-normal-security"}, {3, "direct-with-enhanced-security"}, {4, "sbo-with-enhanced-security"}}},
	{ID: "OriginatorCategoryKind", Vals: []EnumVal{{0, "not-supported"}, {1, "bay-control"}, {2, "station-control"}, {3, "remote-control"}, {4, "automatic-bay"}, {5, "automatic-station"}, {6, "automatic-remote"}, {7, "maintenance"}, {8, "process"}}},
	{ID: "FaultDirectionKind", Vals: []EnumVal{{0, "unknown"}, {1, "forward"}, {2, "backward"}, {3, "both"}}},
	{ID: "PhaseFaultDirectionKind", Vals: []EnumVal{{0, "unknown"}, {1, "forward"}, {2, "backward"}}},
	{ID: "SwitchFunctionKind", Vals: []EnumVal{{1, "Load Break Switch"}, {2, "Disconnector"}, {3, "Earthing Switch"}, {4, "High Speed Earthing Switch"}}},
	{ID: "SequenceKind", Vals: []EnumVal{{0, "pos-neg-zero"}, {1, "dir-quad-zero"}}},
}
