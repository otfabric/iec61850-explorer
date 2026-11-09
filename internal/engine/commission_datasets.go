// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// dataSetName is the name of a created data set: a letter, then letters,
// digits and underscores.
var dataSetName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// CreateDataSet implements service.Service.
func (e *Engine) CreateDataSet(ctx context.Context, id string, req model.CreateDataSetRequest) (model.DataSet, error) {
	ld, item, err := splitDataSetRef(req.Ref)
	if err != nil {
		return model.DataSet{}, err
	}
	ln, name, _ := strings.Cut(item, "$")
	switch {
	case strings.Contains(name, "$") || !dataSetName.MatchString(name) || len(name) > model.MaxDataSetNameLen:
		return model.DataSet{}, service.Errorf(service.CodeInvalidArgument, "a data set name has 1 to %d letters, digits or underscores and starts with a letter", model.MaxDataSetNameLen)
	case len(req.Members) == 0 || len(req.Members) > model.MaxDataSetMembers:
		return model.DataSet{}, service.Errorf(service.CodeInvalidArgument, "a data set has 1 to %d members", model.MaxDataSetMembers)
	}
	members := make([]iec61850.DataSetMember, len(req.Members))
	for i, raw := range req.Members {
		ref, err := iec61850.ParseRef(strings.TrimSpace(raw))
		if err != nil || ref.FC == "" || ref.LN == "" || !ref.HasPath() {
			return model.DataSet{}, service.Errorf(service.CodeInvalidArgument, "member %q is not an FC-qualified data reference such as LD/MMXU1.Hz[MX]", raw)
		}
		members[i] = iec61850.DataSetMember{Ref: ref}
	}
	s, err := e.session(id)
	if err != nil {
		return model.DataSet{}, err
	}
	if err := s.checkLD(ld); err != nil {
		return model.DataSet{}, err
	}
	if !s.hasLN(ld, ln) {
		return model.DataSet{}, service.Errorf(service.CodeNotFound, "%s has no logical node %s", ld, ln)
	}

	var out model.DataSet
	err = e.commissionAction(ctx, id, model.CapDataSets, "create-dataset", req.Ref, func(ctx context.Context, s *session, g *grant) error {
		audit := model.AuditRecord{
			Action: model.AuditCreateDS, Ref: req.Ref,
			Detail: fmt.Sprintf("%d members: %s", len(members), clip(strings.Join(req.Members, ", "), 2000)),
		}
		if err := g.client.CreateDataSet(ctx, ld, item, members); err != nil {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
			e.appendAudit(s, audit)
			return errIED("create data set "+req.Ref, err)
		}
		audit.Outcome = model.OutcomeOK
		// The Observe association caches data set definitions.
		s.client.InvalidateLDCache(ld)
		ds, err := s.client.GetDataSet(ctx, ld, item)
		if err != nil {
			audit.Outcome, audit.Error = model.OutcomeMismatch, "read back: "+err.Error()
			e.appendAudit(s, audit)
			return errIED("read the new data set back", err)
		}
		e.appendAudit(s, audit)
		out = dataSetOf(ld, item, ds)
		return nil
	})
	return out, err
}

// DeleteDataSet implements service.Service.
func (e *Engine) DeleteDataSet(ctx context.Context, id, ref string) error {
	ld, item, err := splitDataSetRef(ref)
	if err != nil {
		return err
	}
	return e.commissionAction(ctx, id, model.CapDataSets, "delete-dataset", ref, func(ctx context.Context, s *session, g *grant) error {
		ds, err := s.client.GetDataSet(ctx, ld, item)
		if err != nil {
			return errIED("read data set "+ref, err)
		}
		if !ds.Deletable {
			return service.Errorf(service.CodeNotAllowed, "the IED marks %s as not deletable (configured, not created by a client)", ref)
		}
		datSet := ld + "/" + item
		g.mu.Lock()
		for _, sub := range g.subs {
			if sub.info.DatSet == datSet {
				g.mu.Unlock()
				return service.Errorf(service.CodeNotAllowed, "the report subscription to %s uses %s; unsubscribe first", sub.info.RCB, ref)
			}
		}
		g.mu.Unlock()
		audit := model.AuditRecord{Action: model.AuditDeleteDS, Ref: ref, Before: fmt.Sprintf("%d members", len(ds.Members)), Outcome: model.OutcomeOK}
		err = g.client.DeleteDataSet(ctx, ld, item)
		s.client.InvalidateLDCache(ld) // the Observe association caches data set definitions
		if err != nil {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
			e.appendAudit(s, audit)
			return errIED("delete data set "+ref, err)
		}
		e.appendAudit(s, audit)
		return nil
	})
}

// SelectActiveSG implements service.Service.
func (e *Engine) SelectActiveSG(ctx context.Context, id string, req model.SelectSGRequest) (model.SettingGroups, error) {
	s, err := e.session(id)
	if err != nil {
		return model.SettingGroups{}, err
	}
	if err := s.checkLD(req.LD); err != nil {
		return model.SettingGroups{}, err
	}
	var out model.SettingGroups
	err = e.commissionAction(ctx, id, model.CapSettingGroups, "select-active-sg", fmt.Sprintf("%s group %d", req.LD, req.SG), func(ctx context.Context, s *session, g *grant) error {
		before, err := s.client.GetSettingGroupInfo(ctx, req.LD)
		if err != nil {
			return errIED("read the setting group control block", err)
		}
		if req.SG < 1 || req.SG > before.NumOfSGs {
			return service.Errorf(service.CodeInvalidArgument, "%s has setting groups 1 to %d", req.LD, before.NumOfSGs)
		}
		audit := model.AuditRecord{
			Action: model.AuditActiveSG, Ref: req.LD + "/LLN0.SGCB.ActSG[SP]",
			Before: fmt.Sprint(before.ActSG), Written: fmt.Sprint(req.SG),
		}
		if err := g.client.SelectActiveSG(ctx, req.LD, req.SG); err != nil {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
			e.appendAudit(s, audit)
			return errIED("select setting group", err)
		}
		after, err := s.client.GetSettingGroupInfo(ctx, req.LD)
		if err != nil {
			audit.Outcome, audit.Error = model.OutcomeMismatch, "read back: "+err.Error()
			e.appendAudit(s, audit)
			return errIED("read the setting group control block back", err)
		}
		audit.After, audit.Outcome = fmt.Sprint(after.ActSG), model.OutcomeOK
		if after.ActSG != req.SG {
			audit.Outcome = model.OutcomeMismatch
		}
		e.appendAudit(s, audit)
		out = model.SettingGroups{LD: req.LD, NumOfSGs: after.NumOfSGs, ActSG: after.ActSG, EditSG: after.EditSG, CnfEdit: after.CnfEdit, ResvTms: after.ResvTms}
		return nil
	})
	return out, err
}

// hasLN reports whether the session's model has logical node ln in ld.
func (s *session) hasLN(ld, ln string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.index[ld+"/"+ln]
	return ok
}

// clip shortens s to at most n bytes.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
