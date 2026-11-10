// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/hex"
	"fmt"
	"strings"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// This file turns the PDUs go-mms decoded (mms.CapturedPDU, from a capture
// or from the Explorer's own association) into their DTOs, and maps MMS
// object names to IEC 61850 references (IEC 61850-8-1, clause 8.1.3). It
// decodes nothing itself: every byte was decoded by go-mms.

// maxRawShown bounds the hex of an undecoded service content.
const maxRawShown = 256

// Object kinds of a TraceObject.
const (
	objLD      = "ld"
	objLN      = "ln"
	objDO      = "do"
	objDA      = "da"
	objDataSet = "data-set"
	objRCB     = "rcb"
	objGoCB    = "gocb"
	objSGCB    = "sgcb"
	objLog     = "log"
)

func scopeName(s mms.ObjectScope) string {
	switch s {
	case mms.ObjectScopeVMD:
		return "vmd"
	case mms.ObjectScopeDomain:
		return "domain"
	case mms.ObjectScopeAssociation:
		return "aa"
	}
	return fmt.Sprintf("scope %d", int(s))
}

// objectOf maps an MMS object name to its IEC 61850 reference. list says
// the name is a named variable list. A name that does not follow the
// mapping keeps its MMS form only.
func objectOf(n mms.ObjectName, list bool) model.TraceObject {
	o := model.TraceObject{MMS: model.MMSName{Domain: string(n.Domain), ItemID: string(n.ItemID)}, Scope: scopeName(n.Scope), List: list}
	if n.Scope != mms.ObjectScopeDomain || n.Domain == "" {
		return o
	}
	item := string(n.ItemID)
	if list {
		// A data set: "LLN0$Meas" in domain "LD" is "LD/LLN0.Meas".
		if ln, name, ok := strings.Cut(item, "$"); ok && ln != "" && name != "" && !strings.Contains(name, "$") {
			o.Ref, o.Kind = dataSetRef(string(n.Domain), item), objDataSet
		}
		return o
	}
	ref, err := iec61850.RefFromMMS(n.Domain, n.ItemID)
	if err != nil {
		return o
	}
	switch {
	case ref.LN == "":
		o.Ref, o.Kind = ref.LD, objLD
		return o
	case ref.FC == "":
		o.Ref, o.Kind = ref.String(), objLN
		return o
	case !ref.FC.IsValid():
		return o
	}
	o.Ref = ref.String()
	switch {
	case len(ref.Path) == 0:
		o.Kind = "" // a whole FC of a logical node
	case ref.FC == iec61850.FCRP || ref.FC == iec61850.FCBR:
		o.Kind = objRCB
	case ref.FC == iec61850.FCGO:
		o.Kind = objGoCB
	case ref.FC == iec61850.FCLG:
		o.Kind = objLog
	case (ref.FC == iec61850.FCSP || ref.FC == iec61850.FCSG || ref.FC == iec61850.FCSE) && ref.Path[0] == "SGCB":
		o.Kind = objSGCB
	case len(ref.Path) == 1:
		o.Kind = objDO
	default:
		o.Kind = objDA
	}
	return o
}

// domainObject is the logical device a GetNameList request names.
func domainObject(domain string) model.TraceObject {
	return model.TraceObject{MMS: model.MMSName{Domain: domain}, Scope: "domain", Ref: domain, Kind: objLD}
}

// pduObjects lists the objects a PDU names, at most MaxTraceObjects, and
// how many more there are.
func pduObjects(p *mms.CapturedPDU) ([]model.TraceObject, int) {
	if p == nil {
		return nil, 0
	}
	var out []model.TraceObject
	dropped := 0
	add := func(o model.TraceObject) {
		if len(out) == model.MaxTraceObjects {
			dropped++
			return
		}
		out = append(out, o)
	}
	if p.ListName != nil {
		add(objectOf(*p.ListName, true))
	}
	for _, v := range p.Variables {
		add(objectOf(v.Name, false))
	}
	if p.VariableName != nil {
		add(objectOf(*p.VariableName, false))
	}
	if r := p.NameListRequest; r != nil && r.Scope == mms.ObjectScopeDomain && r.Domain != "" {
		add(domainObject(r.Domain))
	}
	for _, v := range p.NamedListMembers {
		add(objectOf(v.Name, false))
	}
	return out, dropped
}

// pduOf converts a decoded PDU to its DTO.
func pduOf(p *mms.CapturedPDU) *model.WirePDU {
	if p == nil {
		return nil
	}
	out := &model.WirePDU{Kind: p.Kind, Service: p.Service, DecodeError: p.DecodeError, NameListMore: p.NameListMore}
	if p.HasInvokeID {
		id := p.InvokeID
		out.InvokeID = &id
	}
	out.Objects, out.ObjectsDropped = pduObjects(p)
	for _, v := range p.Values {
		if len(out.Values) == model.MaxTraceValues {
			out.ValuesDropped++
			continue
		}
		out.Values = append(out.Values, convertValue(v, nil, ""))
	}
	for _, r := range p.WriteResults {
		if r == nil {
			out.WriteResults = append(out.WriteResults, "ok")
		} else {
			out.WriteResults = append(out.WriteResults, r.Code.String())
		}
	}
	for _, n := range p.NameList {
		if len(out.NameList) == model.MaxTraceObjects {
			out.NameListMore = true
			break
		}
		out.NameList = append(out.NameList, n)
	}
	if p.Identify != nil {
		out.Identity = &model.Identity{Vendor: p.Identify.Vendor, Model: p.Identify.Model, Revision: p.Identify.Revision}
	}
	if p.Initiate != nil {
		out.Initiate = initiateOf(p.Initiate)
	}
	out.Error = pduError(p)
	if len(p.Raw) > 0 {
		raw := p.Raw
		if len(raw) > maxRawShown {
			raw = raw[:maxRawShown]
		}
		out.Raw = hex.EncodeToString(raw)
	}
	return out
}

func initiateOf(i *mms.CapturedInitiate) *model.TraceInitiate {
	return &model.TraceInitiate{
		LocalDetail: i.LocalDetail, MaxServicesOutstandingCall: i.MaxServicesOutstandingCall,
		MaxServicesOutstandingCalled: i.MaxServicesOutstandingCalled, DataStructureNesting: i.DataStructureNesting, Version: i.Version,
	}
}

// pduError is a confirmed error or reject as text: the class and code of
// ISO 9506-2, as the PDU carries them.
func pduError(p *mms.CapturedPDU) string {
	switch {
	case p == nil:
		return ""
	case p.Error != nil:
		return fmt.Sprintf("error class %d, code %d", p.Error.Class, p.Error.Code)
	case p.Reject != nil:
		return fmt.Sprintf("reject: PDU type %d, reason %d", p.Reject.Type, p.Reject.Reason)
	}
	return ""
}
