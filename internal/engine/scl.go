// SPDX-License-Identifier: MIT

package engine

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// parseSCL parses an SCL document the user chose.
func parseSCL(f model.File) (*scl.Result, error) {
	switch {
	case len(f.Data) == 0:
		return nil, service.Errorf(service.CodeInvalidArgument, "the SCL file is empty")
	case len(f.Data) > model.MaxSCLBytes:
		return nil, service.Errorf(service.CodeLimitExceeded, "the SCL file is larger than %d MiB", model.MaxSCLBytes>>20)
	}
	res, err := scl.ParseBytes(f.Data, scl.ParseOptions{ValidateSemantic: true, Kind: kindOf(f.Name)})
	if err != nil {
		return nil, &service.Error{Code: service.CodeInvalidArgument, Message: fmt.Sprintf("%s is not an SCL document this version reads: %v", displayName(f.Name), err), Err: err}
	}
	return res, nil
}

// kindOf names the document kind by the file extension (IEC 61850-6
// clause 7.3); an unknown extension lets the parser decide from the content.
func kindOf(name string) scl.DocumentKind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".scd":
		return scl.KindSCD
	case ".cid":
		return scl.KindCID
	case ".icd":
		return scl.KindICD
	case ".iid":
		return scl.KindIID
	case ".ssd":
		return scl.KindSSD
	}
	return ""
}

func displayName(name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return "the file"
	}
	return name
}

// InspectSCL implements service.Service.
func (e *Engine) InspectSCL(_ context.Context, f model.File) (model.SCLInfo, error) {
	res, err := parseSCL(f)
	if err != nil {
		return model.SCLInfo{}, err
	}
	doc := res.Document
	info := model.SCLInfo{
		Name: displayName(f.Name), Kind: string(res.Kind), IEDs: []model.SCLIED{}, Diagnostics: []model.SCLDiagnostic{},
		Version: strings.TrimSpace(res.Version.Version + res.Version.Revision + res.Version.Release),
	}
	for _, d := range res.Diagnostics {
		if len(info.Diagnostics) == model.MaxSCLDiagnostics {
			info.DiagnosticsDropped++
			continue
		}
		info.Diagnostics = append(info.Diagnostics, model.SCLDiagnostic{Severity: string(d.Severity), Code: d.Code, Path: d.Path, Message: d.Message})
	}
	for _, ied := range doc.IEDs {
		out := model.SCLIED{Name: ied.Name, Desc: ied.Desc, Manufacturer: ied.Manufacturer, Type: ied.Type, AccessPoints: []model.SCLAccessPoint{}}
		for _, ap := range ied.AccessPoints {
			if ap.Server == nil {
				continue
			}
			out.AccessPoints = append(out.AccessPoints, model.SCLAccessPoint{
				Name: ap.Name, LogicalDevices: len(ap.Server.LDevices), Target: targetFromSCL(doc, ied.Name, ap.Name),
			})
		}
		info.IEDs = append(info.IEDs, out)
	}
	return info, nil
}

// targetFromSCL reads the MMS address of an access point from the
// Communication section (IEC 61850-6 Table "P" types): IP and the OSI
// parameters. Nil without an IP address.
func targetFromSCL(doc *scl.SCL, iedName, apName string) *model.TargetConfig {
	if doc.Communication == nil {
		return nil
	}
	for _, sn := range doc.Communication.SubNetworks {
		for _, cap := range sn.ConnectedAPs {
			if cap.IEDName != iedName || cap.APName != apName {
				continue
			}
			t := &model.TargetConfig{Port: model.DefaultPort}
			a := &model.Addressing{}
			set := false
			for _, p := range cap.Address {
				v := strings.TrimSpace(p.Value)
				switch p.Type {
				case "IP":
					t.Host = v
				case "OSI-TSEL":
					a.RemoteTSelector, set = &v, true
				case "OSI-SSEL":
					a.RemoteSSelector, set = &v, true
				case "OSI-PSEL":
					a.RemotePSelector, set = &v, true
				case "OSI-AP-Title":
					a.RemoteAPTitle, set = &v, true
				case "OSI-AE-Qualifier":
					if n, err := strconv.Atoi(v); err == nil {
						a.RemoteAEQualifier, set = &n, true
					}
				}
			}
			if t.Host == "" {
				return nil
			}
			if set {
				t.Addressing = a
			}
			return t
		}
	}
	return nil
}

// OpenSCL implements service.Service.
func (e *Engine) OpenSCL(_ context.Context, req model.OpenSCLRequest) (model.SessionInfo, error) {
	res, err := parseSCL(req.File)
	if err != nil {
		return model.SessionInfo{}, err
	}
	doc := res.Document
	ied, ap, err := pickAccessPoint(doc, req.IED, req.AccessPoint)
	if err != nil {
		return model.SessionInfo{}, err
	}
	start := time.Now()
	root := sclTree(doc, ied, ap)

	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case e.closed:
		return model.SessionInfo{}, service.Errorf(service.CodeNotAllowed, "the engine is shutting down")
	case len(e.sessions) >= e.opts.MaxSessions+maxLostSessions:
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "too many sessions; close some first")
	}
	s := &session{
		id: newID(), openedAt: time.Now().UTC(), state: model.SessionOffline, rec: &recorder{},
		scl: &model.SCLOrigin{File: displayName(req.File.Name), IED: ied.Name, AccessPoint: ap.Name},
	}
	if t := targetFromSCL(doc, ied.Name, ap.Name); t != nil {
		s.target = *t
	}
	s.setTree(root, time.Since(start))
	s.sclDataSets, s.sclReports = sclControls(ied, ap)
	s.sclGoCBs = sclGoCBs(ied, ap)
	m := sclValues(doc, ied, ap)
	s.sclValues, s.sclActSG, s.sclCDC, s.sclDecls, s.sclSchema = m.values, m.actSG, m.cdcs, m.decls, string(res.Version.Schema)
	e.sessions[s.id] = s
	e.order = append(e.order, s.id)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	return info, nil
}

// pickAccessPoint finds the IED and access point; an empty name is fine
// when there is only one candidate.
func pickAccessPoint(doc *scl.SCL, iedName, apName string) (*scl.IED, *scl.AccessPoint, error) {
	var ieds []*scl.IED
	for i := range doc.IEDs {
		if iedName == "" || doc.IEDs[i].Name == iedName {
			ieds = append(ieds, &doc.IEDs[i])
		}
	}
	switch {
	case len(ieds) == 0 && iedName != "":
		return nil, nil, service.Errorf(service.CodeNotFound, "no IED %q in the SCL file", iedName)
	case len(ieds) == 0:
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "the SCL file describes no IED")
	case len(ieds) > 1:
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "the SCL file describes %d IEDs: choose one", len(ieds))
	}
	ied := ieds[0]
	var aps []*scl.AccessPoint
	for i := range ied.AccessPoints {
		a := &ied.AccessPoints[i]
		if a.Server != nil && (apName == "" || a.Name == apName) {
			aps = append(aps, a)
		}
	}
	switch {
	case len(aps) == 0 && apName != "":
		return nil, nil, service.Errorf(service.CodeNotFound, "IED %s has no access point %q with a server", ied.Name, apName)
	case len(aps) == 0:
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "IED %s has no access point with an MMS server", ied.Name)
	case len(aps) > 1:
		return nil, nil, service.Errorf(service.CodeInvalidArgument, "IED %s has %d access points with a server: choose one", ied.Name, len(aps))
	}
	return ied, aps[0], nil
}

// sclTree builds the model tree of one access point, in the shape of a
// discovered tree: logical devices by their MMS domain names (LDevice@ldName
// or IED name + inst, IEC 61850-8-1), logical nodes, then the data objects
// and attributes the data type templates give, each with its functional
// constraints.
func sclTree(doc *scl.SCL, ied *scl.IED, ap *scl.AccessPoint) *iec61850.ModelNode {
	root := &iec61850.ModelNode{Name: "root"}
	nodes := map[string]*iec61850.ModelNode{}
	child := func(parent *iec61850.ModelNode, ref iec61850.Ref, name string) *iec61850.ModelNode {
		key := refKey(ref)
		if n, ok := nodes[key]; ok {
			return n
		}
		n := &iec61850.ModelNode{Name: name, Reference: ref}
		parent.Children = append(parent.Children, n)
		nodes[key] = n
		return n
	}
	addFC := func(n *iec61850.ModelNode, fc string) {
		f := iec61850.FunctionalConstraint(fc)
		if fc != "" && !slices.Contains(n.FCs, f) {
			n.FCs = append(n.FCs, f)
		}
	}

	domains := map[string]string{} // LDevice inst -> domain
	for _, ld := range ap.Server.LDevices {
		domain := ld.LdName
		if domain == "" {
			domain = ied.Name + ld.Inst
		}
		domains[ld.Inst] = domain
		ldNode := child(root, iec61850.Ref{LD: domain}, domain)
		if ld.LN0 != nil {
			child(ldNode, iec61850.Ref{LD: domain, LN: "LLN0"}, "LLN0")
		}
		for _, ln := range ld.LNs {
			name := ln.Prefix + ln.LNClass + ln.Inst
			child(ldNode, iec61850.Ref{LD: domain, LN: name}, name)
		}
	}
	for _, row := range scl.Flatten(doc) {
		domain, ok := domains[row.LD]
		if row.IED != ied.Name || row.AccessPoint != ap.Name || !ok || row.Path == "" {
			continue
		}
		lnNode := child(nodes[domain], iec61850.Ref{LD: domain, LN: row.LN}, row.LN)
		addFC(lnNode, row.FC)
		parent := lnNode
		parts := strings.Split(row.Path, ".")
		for k := range parts {
			ref := iec61850.Ref{LD: domain, LN: row.LN, Path: slices.Clone(parts[:k+1])}
			parent = child(parent, ref, parts[k])
			addFC(parent, row.FC)
		}
	}
	var sortTree func(n *iec61850.ModelNode)
	sortTree = func(n *iec61850.ModelNode) {
		slices.SortFunc(n.Children, func(a, b *iec61850.ModelNode) int { return cmp.Compare(a.Name, b.Name) })
		slices.Sort(n.FCs)
		for _, c := range n.Children {
			sortTree(c)
		}
	}
	sortTree(root)
	return root
}

// ldDomain is the MMS domain of a logical device (IEC 61850-8-1): its
// ldName, or the IED name followed by its inst.
func ldDomain(ied *scl.IED, ld scl.LDevice) string {
	if ld.LdName != "" {
		return ld.LdName
	}
	return ied.Name + ld.Inst
}

// sclControls reads the data sets and report control blocks an access
// point's SCL defines, by logical device. An FCDA names its logical device
// by inst; one of another logical device of the same IED is resolved
// through the access point.
func sclControls(ied *scl.IED, ap *scl.AccessPoint) (map[string][]model.DataSet, map[string][]model.RCB) {
	domains := map[string]string{}
	for _, ld := range ap.Server.LDevices {
		domains[ld.Inst] = ldDomain(ied, ld)
	}
	dataSets := map[string][]model.DataSet{}
	reports := map[string][]model.RCB{}
	for _, ld := range ap.Server.LDevices {
		domain := ldDomain(ied, ld)
		lns := []struct {
			name string
			ln   *scl.LN
		}{}
		if ld.LN0 != nil {
			lns = append(lns, struct {
				name string
				ln   *scl.LN
			}{"LLN0", ld.LN0})
		}
		for i := range ld.LNs {
			ln := &ld.LNs[i]
			lns = append(lns, struct {
				name string
				ln   *scl.LN
			}{ln.Prefix + ln.LNClass + ln.Inst, ln})
		}
		for _, l := range lns {
			for _, ds := range l.ln.DataSets {
				item := l.name + "$" + ds.Name
				out := model.DataSet{
					Ref: dataSetRef(domain, item), MMS: model.MMSName{Domain: domain, ItemID: item},
					Desc: ds.Desc, Source: model.SourceSCL, Members: []model.DataSetMember{},
				}
				for _, f := range ds.FCDAs {
					memberDomain, ok := domains[f.LDInst]
					if !ok {
						memberDomain = ied.Name + f.LDInst
					}
					path := f.DOName
					if f.DAName != "" {
						path += "." + f.DAName
					}
					lnName := f.Prefix + f.LNClass + f.LNInst
					ref := fmt.Sprintf("%s/%s.%s[%s]", memberDomain, lnName, path, f.FC)
					mmsItem := lnName + "$" + f.FC + "$" + strings.ReplaceAll(path, ".", "$")
					out.Members = append(out.Members, model.DataSetMember{Ref: ref, MMS: model.MMSName{Domain: memberDomain, ItemID: mmsItem}})
				}
				dataSets[domain] = append(dataSets[domain], out)
			}
			for _, rc := range l.ln.Reports {
				fc := "RP"
				if rc.Buffered {
					fc = "BR"
				}
				item := l.name + "$" + fc + "$" + rc.Name
				datSet := ""
				if rc.DatSet != "" {
					datSet = domain + "/" + l.name + "$" + rc.DatSet
				}
				reports[domain] = append(reports[domain], model.RCB{
					Ref: dataSetRef(domain, item), MMS: model.MMSName{Domain: domain, ItemID: item}, Source: model.SourceSCL,
					Buffered: rc.Buffered, RptID: rc.RptID, DatSet: datSet, ConfRev: rc.ConfRev,
					BufTmMS: rc.BufTime, IntgPdMS: rc.IntgPd, OptFlds: sclOptFields(rc.OptFields), TrgOps: sclTrgOps(rc.TrgOps),
				})
			}
		}
	}
	return dataSets, reports
}

// sclOptFields names the optional fields as go-iec61850 names those it
// reads from an IED, in the same order, so that SCL and IED compare.
func sclOptFields(o scl.OptFields) []string {
	out := []string{}
	for _, f := range []struct {
		on   bool
		name string
	}{
		{o.SeqNum, "seq-num"}, {o.TimeStamp, "timestamp"}, {o.ReasonCode, "reason-code"}, {o.DataSet, "data-set"},
		{o.DataRef, "data-ref"}, {o.BufOvfl, "buf-ovfl"}, {o.EntryID, "entry-id"}, {o.ConfigRef, "conf-rev"},
	} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}

// sclTrgOps names the trigger options as go-iec61850 does.
func sclTrgOps(t scl.TrgOps) []string {
	out := []string{}
	for _, f := range []struct {
		on   bool
		name string
	}{
		{t.Dchg, "data-changed"}, {t.Qchg, "quality-changed"}, {t.Dupd, "data-update"}, {t.Period, "integrity"}, {t.GI, "GI"},
	} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}
