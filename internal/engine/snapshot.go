// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// csvColumns are the columns of a CSV export: one row per leaf value.
var csvColumns = []string{"read_at", "ref", "component", "mms_domain", "mms_item_id", "type", "value", "validity", "error"}

func appInfo() model.AppInfo {
	return model.AppInfo{
		Name: model.AppName, Version: buildinfo.Version,
		GoIEC61850: buildinfo.ModuleVersion(buildinfo.GoIEC61850Path), GoMMS: buildinfo.ModuleVersion(buildinfo.GoMMSPath),
	}
}

// Export implements service.Service.
func (e *Engine) Export(ctx context.Context, id string, req model.ExportRequest) (model.ExportFile, error) {
	switch req.Format {
	case model.ExportPointsCSV, model.ExportPointsJSON:
		s, err := e.session(id)
		if err != nil {
			return model.ExportFile{}, err
		}
		return e.exportPoints(ctx, s, req.Format)
	case model.ExportSnapshot, model.ExportCSV:
	default:
		return model.ExportFile{}, service.Errorf(service.CodeInvalidArgument, "export format %q: use %s, %s, %s or %s", req.Format,
			model.ExportSnapshot, model.ExportCSV, model.ExportPointsCSV, model.ExportPointsJSON)
	}
	if len(req.Refs) > model.MaxReadRefs {
		return model.ExportFile{}, service.Errorf(service.CodeLimitExceeded, "at most %d references", model.MaxReadRefs)
	}
	if req.Format == model.ExportCSV && len(req.Refs) == 0 {
		return model.ExportFile{}, service.Errorf(service.CodeInvalidArgument, "a CSV export needs references to read")
	}
	s, err := e.session(id)
	if err != nil {
		return model.ExportFile{}, err
	}

	var values model.ReadResult
	if len(req.Refs) > 0 {
		if values, err = e.Read(ctx, id, model.ReadRequest{Refs: req.Refs}); err != nil {
			return model.ExportFile{}, err
		}
	}
	info := s.info()
	base := exportBase(info, time.Now().UTC())
	if req.Format == model.ExportCSV {
		text, err := renderCSV(values)
		if err != nil {
			return model.ExportFile{}, service.Errorf(service.CodeInternal, "CSV: %v", err)
		}
		return model.ExportFile{Name: base + ".csv", MIME: "text/csv", Text: text}, nil
	}

	snap := model.Snapshot{
		Schema: model.SnapshotSchema, App: appInfo(), CapturedAt: time.Now().UTC(),
		Target: info.Target, Identity: info.Identity, Discovery: info.Discovery, SCL: info.SCL,
		Nodes: s.allNodes(), Values: values.Values, ValuesAt: values.At,
	}
	if snap.Values == nil {
		snap.Values = []model.ReadValue{}
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return model.ExportFile{}, service.Errorf(service.CodeInternal, "snapshot: %v", err)
	}
	return model.ExportFile{Name: base + ".json", MIME: "application/json", Text: string(b) + "\n"}, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportBase names an export after the IED and the time.
func exportBase(info model.SessionInfo, at time.Time) string {
	who := info.Target.Host
	switch {
	case info.SCL != nil:
		who = info.SCL.IED
	case info.Identity != nil && info.Identity.Model != "":
		who = info.Identity.Model + "-" + info.Target.Host
	}
	who = strings.Trim(unsafeName.ReplaceAllString(who, "_"), "_.")
	if who == "" {
		who = "ied"
	}
	return fmt.Sprintf("iec61850-%s-%s", who, at.Format("20060102T150405Z"))
}

// renderCSV writes one row per leaf value.
func renderCSV(res model.ReadResult) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(csvColumns)
	at := ""
	if !res.At.IsZero() {
		at = res.At.Format(time.RFC3339Nano)
	}
	for _, v := range res.Values {
		if v.Error != nil || v.Value == nil {
			msg := ""
			if v.Error != nil {
				msg = v.Error.Message
			}
			_ = w.Write([]string{at, v.Ref, "", v.MMS.Domain, v.MMS.ItemID, "", "", "", msg})
			continue
		}
		var leaf func(val model.Value, path string)
		leaf = func(val model.Value, path string) {
			if len(val.Children) > 0 {
				for i, c := range val.Children {
					name := c.Name
					if name == "" {
						name = fmt.Sprintf("[%d]", i)
					}
					leaf(c, joinComponent(path, name))
				}
				return
			}
			validity := ""
			if val.Quality != nil {
				validity = val.Quality.Validity
			}
			_ = w.Write([]string{at, v.Ref, path, v.MMS.Domain, v.MMS.ItemID, val.Type, csvSafe(val.Text), validity, ""})
		}
		leaf(*v.Value, "")
	}
	w.Flush()
	return buf.String(), w.Error()
}

func joinComponent(path, name string) string {
	switch {
	case path == "":
		return name
	case strings.HasPrefix(name, "["):
		return path + name
	}
	return path + "." + name
}

// csvSafe keeps a device string from being taken for a formula by a
// spreadsheet (CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		if _, err := fmt.Sscanf(s, "%g", new(float64)); err == nil {
			return s // a number such as -7 or +1e3
		}
		return "'" + s
	}
	return s
}

// allNodes returns the session's model depth first.
func (s *session) allNodes() []model.Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []model.Node{}
	var walk func(n *iec61850.ModelNode)
	walk = func(n *iec61850.ModelNode) {
		for _, c := range n.Children {
			out = append(out, nodeOf(c))
			walk(c)
		}
	}
	walk(s.index[""])
	return out
}

// OpenSnapshot implements service.Service.
func (e *Engine) OpenSnapshot(_ context.Context, f model.File) (model.SessionInfo, error) {
	switch {
	case len(f.Data) == 0:
		return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "the snapshot file is empty")
	case len(f.Data) > model.MaxSnapshotBytes:
		return model.SessionInfo{}, service.Errorf(service.CodeLimitExceeded, "the snapshot is larger than %d MiB", model.MaxSnapshotBytes>>20)
	}
	var snap model.Snapshot
	if err := json.Unmarshal(f.Data, &snap); err != nil {
		return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "%s is not a snapshot: %v", displayName(f.Name), err)
	}
	if snap.Schema != model.SnapshotSchema {
		return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "%s has schema %q, want %q", displayName(f.Name), snap.Schema, model.SnapshotSchema)
	}
	root, err := treeFromNodes(snap.Nodes)
	if err != nil {
		return model.SessionInfo{}, service.Errorf(service.CodeInvalidArgument, "%s: %v", displayName(f.Name), err)
	}
	values := make(map[string]model.ReadValue, len(snap.Values))
	for _, v := range snap.Values {
		values[v.Ref] = v
	}

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
		target: snap.Target, identity: snap.Identity, scl: snap.SCL,
		snapshot:   &model.SnapshotOrigin{File: displayName(f.Name), CapturedAt: snap.CapturedAt},
		snapValues: values, snapAt: snap.ValuesAt,
	}
	s.target.Proxy = redactProxySetting(s.target.Proxy)
	s.setTree(root, 0)
	e.sessions[s.id] = s
	e.order = append(e.order, s.id)
	info := s.info()
	e.hub.publish(model.Event{Kind: model.EventSession, SessionID: s.id, Session: &info})
	return info, nil
}

// treeFromNodes rebuilds a model tree from a snapshot's depth-first nodes.
// A node's parent must come before it.
func treeFromNodes(nodes []model.Node) (*iec61850.ModelNode, error) {
	root := &iec61850.ModelNode{Name: "root"}
	byRef := map[string]*iec61850.ModelNode{"": root}
	for _, n := range nodes {
		ref, err := iec61850.ParseRef(n.Ref)
		if err != nil || ref.FC != "" {
			return nil, fmt.Errorf("node %q is not an object reference", n.Ref)
		}
		parentKey := ""
		if p, ok := ref.Parent(); ok {
			parentKey = refKey(p)
		}
		parent, ok := byRef[parentKey]
		if !ok {
			return nil, fmt.Errorf("node %q comes before its parent", n.Ref)
		}
		key := refKey(ref)
		if _, dup := byRef[key]; dup {
			return nil, fmt.Errorf("node %q appears twice", n.Ref)
		}
		mn := &iec61850.ModelNode{Name: n.Name, Reference: ref}
		for _, fc := range n.FCs {
			mn.FCs = append(mn.FCs, iec61850.FunctionalConstraint(fc))
		}
		parent.Children = append(parent.Children, mn)
		byRef[key] = mn
	}
	return root, nil
}

// readSnapshot answers a read on a snapshot session from its values.
func readSnapshot(s *session, refs []string) model.ReadResult {
	res := model.ReadResult{Source: model.SourceSnapshot, At: s.snapAt, Values: make([]model.ReadValue, len(refs))}
	for i, raw := range refs {
		key := strings.TrimSpace(raw)
		if ref, err := iec61850.ParseRef(key); err == nil {
			key = ref.String()
		}
		if v, ok := s.snapValues[key]; ok {
			res.Values[i] = v
			continue
		}
		res.Values[i] = model.ReadValue{Ref: raw, Error: &model.OpError{
			Code: model.ErrCodeNotFound, Message: "the snapshot holds no value for this reference",
		}}
	}
	return res
}
