// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// errIED wraps a failed exchange with the IED for the caller.
func errIED(what string, err error) error {
	if e, ok := errors.AsType[*service.Error](err); ok {
		return e
	}
	op := classify(err, false)
	return &service.Error{Code: service.CodeIEDError, Message: what + ": " + op.Message, Detail: op, Err: err}
}

// live runs one action on a connected session as one operation: the
// session's lock held, its timeout applied (times scale for long
// transfers), the requests recorded and published.
func (e *Engine) live(ctx context.Context, id, action, target string, scale time.Duration, fn func(ctx context.Context, s *session) error) error {
	s, err := e.session(id)
	if err != nil {
		return err
	}
	if err := s.liveErr(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.start()
	start := time.Now()
	tctx, cancel := context.WithTimeout(ctx, s.timeout*scale)
	defer cancel()
	err = fn(tctx, s)
	var opErr *model.OpError
	if err != nil {
		if se, ok := errors.AsType[*service.Error](err); ok && se.Detail != nil {
			opErr = se.Detail
		} else {
			opErr = classify(err, false)
		}
	}
	e.publishOperation(s.rec, s.id, action, target, start, opErr)
	return err
}

// checkLD refuses a logical device the session's model does not have.
func (s *session) checkLD(ld string) error {
	ld = strings.TrimSpace(ld)
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.index[ld]; ld == "" || !ok || !n.Reference.IsLD() {
		return service.Errorf(service.CodeNotFound, "%q is not a logical device of this session", ld)
	}
	return nil
}

// DataSets implements service.Service.
func (e *Engine) DataSets(ctx context.Context, id, ld string) ([]model.DataSet, error) {
	s, err := e.session(id)
	if err != nil {
		return nil, err
	}
	if err := s.checkLD(ld); err != nil {
		return nil, err
	}
	if s.scl != nil && s.snapValues == nil {
		return slices.Clone(s.sclDataSets[ld]), nil
	}
	var out []model.DataSet
	err = e.live(ctx, id, "datasets", ld, 1, func(ctx context.Context, s *session) error {
		names, err := s.client.ListDataSets(ctx, ld)
		if err != nil {
			return errIED("list the data sets", err)
		}
		out = make([]model.DataSet, 0, len(names))
		for _, name := range names {
			ds, err := s.client.GetDataSet(ctx, ld, name)
			if err != nil {
				return errIED("read data set "+name, err)
			}
			out = append(out, dataSetOf(ld, name, ds))
		}
		return nil
	})
	return out, err
}

func dataSetOf(ld, item string, ds *iec61850.DataSet) model.DataSet {
	out := model.DataSet{
		Ref: dataSetRef(ld, item), MMS: model.MMSName{Domain: ld, ItemID: item},
		Deletable: ds.Deletable, Source: model.SourceIED, Members: make([]model.DataSetMember, len(ds.Members)),
	}
	for i, m := range ds.Members {
		out.Members[i] = model.DataSetMember{Ref: m.Ref.String(), MMS: model.MMSName{Domain: m.DomainID, ItemID: m.ItemID}}
	}
	return out
}

// dataSetRef is the object reference of a data set: "LD/LLN0.Meas" for
// item "LLN0$Meas".
func dataSetRef(ld, item string) string {
	return ld + "/" + strings.ReplaceAll(item, "$", ".")
}

// splitDataSetRef is the inverse of dataSetRef.
func splitDataSetRef(ref string) (ld, item string, err error) {
	ld, rest, ok := strings.Cut(strings.TrimSpace(ref), "/")
	if !ok || ld == "" || rest == "" || strings.ContainsAny(rest, "[]$/") || !strings.Contains(rest, ".") {
		return "", "", service.Errorf(service.CodeInvalidArgument, "%q is not a data set reference such as LD/LLN0.Name", ref)
	}
	return ld, strings.ReplaceAll(rest, ".", "$"), nil
}

// ReadDataSet implements service.Service.
func (e *Engine) ReadDataSet(ctx context.Context, id, ref string) (model.ReadResult, error) {
	ld, item, err := splitDataSetRef(ref)
	if err != nil {
		return model.ReadResult{}, err
	}
	res := model.ReadResult{Source: model.SourceIED}
	err = e.live(ctx, id, "read", ref, 1, func(ctx context.Context, s *session) error {
		res.At = time.Now().UTC()
		start := time.Now()
		ds, err := s.client.GetDataSet(ctx, ld, item)
		if err != nil {
			return errIED("read data set "+ref, err)
		}
		for _, m := range ds.Members {
			if isControlFC(m.Ref) {
				return service.Errorf(service.CodeInvalidArgument, "data set %s has the member %s: %s", ref, m.Ref.String(), controlReadMessage)
			}
		}
		values, err := s.client.ReadDataSet(ctx, ld, item)
		if err != nil {
			return errIED("read data set "+ref, err)
		}
		res.Values = make([]model.ReadValue, len(values))
		for i, v := range values {
			out := &res.Values[i]
			out.Ref = v.Member.Ref.String()
			out.MMS = model.MMSName{Domain: v.Member.DomainID, ItemID: v.Member.ItemID}
			if v.Err != nil {
				out.Error = classify(v.Err, false)
				continue
			}
			e.setValue(ctx, s, v.Member.Ref, v.Value.MMS(), out)
		}
		res.DurationMS = since(start)
		return nil
	})
	return res, err
}

// Reports implements service.Service.
func (e *Engine) Reports(ctx context.Context, id, ld string) ([]model.RCB, error) {
	s, err := e.session(id)
	if err != nil {
		return nil, err
	}
	if err := s.checkLD(ld); err != nil {
		return nil, err
	}
	if s.scl != nil && s.snapValues == nil {
		return slices.Clone(s.sclReports[ld]), nil
	}
	var out []model.RCB
	err = e.live(ctx, id, "reports", ld, 1, func(ctx context.Context, s *session) error {
		items, err := s.client.ListReports(ctx, ld)
		if err != nil {
			return errIED("list the report control blocks", err)
		}
		out = make([]model.RCB, 0, len(items))
		for _, item := range items {
			rcb := model.RCB{
				Ref: dataSetRef(ld, item), MMS: model.MMSName{Domain: ld, ItemID: item}, Source: model.SourceIED,
				Buffered: strings.Contains(item, "$BR$"), OptFlds: []string{}, TrgOps: []string{},
			}
			cb, err := s.client.GetReportControlBlock(ctx, ld, item)
			if err != nil {
				rcb.Error = classify(err, false)
				out = append(out, rcb)
				continue
			}
			fillRCB(&rcb, cb)
			out = append(out, rcb)
		}
		return nil
	})
	return out, err
}

func fillRCB(r *model.RCB, cb *iec61850.ReportControlBlock) {
	r.Buffered = cb.Type == iec61850.RCBBuffered
	r.RptID, r.DatSet, r.ConfRev = cb.RptID, cb.DatSet, cb.ConfRev
	r.OptFlds, r.TrgOps = flagList(cb.OptFlds.String()), flagList(cb.TrgOps.String())
	r.BufTmMS, r.IntgPdMS = cb.BufTm, cb.IntgPd
	rptEna, gi, resv, sq, resvTms := cb.RptEna, cb.GI, cb.Resv, cb.SqNum, cb.ResvTms
	r.RptEna, r.GI, r.SqNum = &rptEna, &gi, &sq
	if !r.Buffered {
		r.Resv = &resv
	} else {
		r.ResvTms = &resvTms
	}
	r.EntryID, r.Owner = hex.EncodeToString(cb.EntryID), hex.EncodeToString(cb.Owner)
}

// flagList splits go-iec61850's comma-separated flag names; "none" is
// empty.
func flagList(s string) []string {
	if s == "" || s == "none" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// Files implements service.Service.
func (e *Engine) Files(ctx context.Context, id, dir string) ([]model.FileEntry, error) {
	if strings.ContainsAny(dir, "*?\x00") {
		return nil, service.Errorf(service.CodeInvalidArgument, "a directory name has no wildcards")
	}
	var out []model.FileEntry
	err := e.live(ctx, id, "files", dir, 1, func(ctx context.Context, s *session) error {
		entries, err := s.client.ListFiles(ctx, dir)
		if err != nil {
			return errIED("list the files", err)
		}
		out = make([]model.FileEntry, len(entries))
		for i, f := range entries {
			out[i] = model.FileEntry{Name: f.Name, Size: f.Size, LastModified: f.LastModified.UTC()}
		}
		return nil
	})
	return out, err
}

// errTooLarge stops a download beyond model.MaxFileBytes.
var errTooLarge = fmt.Errorf("the file is larger than %d MiB", model.MaxFileBytes>>20)

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > model.MaxFileBytes {
		return 0, errTooLarge
	}
	return b.Buffer.Write(p)
}

// ReadFile implements service.Service.
func (e *Engine) ReadFile(ctx context.Context, id, name string) (model.FileContent, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "*?\x00") {
		return model.FileContent{}, service.Errorf(service.CodeInvalidArgument, "%q is not a file name", name)
	}
	var out model.FileContent
	err := e.live(ctx, id, "download", name, 30, func(ctx context.Context, s *session) error {
		var buf cappedBuffer
		entry, err := s.client.DownloadFile(ctx, name, &buf)
		if errors.Is(err, errTooLarge) {
			return service.Errorf(service.CodeLimitExceeded, "%v", errTooLarge)
		}
		if err != nil {
			return errIED("download "+name, err)
		}
		out = model.FileContent{Name: name, Size: entry.Size, LastModified: entry.LastModified.UTC(), Data: buf.Bytes()}
		return nil
	})
	return out, err
}

// Journals implements service.Service.
func (e *Engine) Journals(ctx context.Context, id, ld string) ([]string, error) {
	s, err := e.session(id)
	if err != nil {
		return nil, err
	}
	if err := s.checkLD(ld); err != nil {
		return nil, err
	}
	var out []string
	err = e.live(ctx, id, "journals", ld, 1, func(ctx context.Context, s *session) error {
		names, err := s.client.ListJournals(ctx, ld)
		if err != nil {
			return errIED("list the journals", err)
		}
		out = append([]string{}, names...)
		return nil
	})
	return out, err
}

// ReadJournal implements service.Service.
func (e *Engine) ReadJournal(ctx context.Context, id string, req model.JournalRequest) (model.JournalResult, error) {
	if req.Stop.IsZero() {
		req.Stop = time.Now().UTC()
	}
	if req.Start.IsZero() {
		req.Start = req.Stop.Add(-24 * time.Hour)
	}
	switch {
	case strings.TrimSpace(req.Journal) == "":
		return model.JournalResult{}, service.Errorf(service.CodeInvalidArgument, "no journal")
	case !req.Start.Before(req.Stop):
		return model.JournalResult{}, service.Errorf(service.CodeInvalidArgument, "the start must be before the stop")
	}
	s, err := e.session(id)
	if err != nil {
		return model.JournalResult{}, err
	}
	if err := s.checkLD(req.LD); err != nil {
		return model.JournalResult{}, err
	}
	out := model.JournalResult{Entries: []model.JournalEntry{}}
	err = e.live(ctx, id, "journal", req.LD+"/"+req.Journal, 3, func(ctx context.Context, s *session) error {
		res, err := s.client.ReadJournal(ctx, req.LD, req.Journal, req.Start, req.Stop)
		if err != nil {
			return errIED("read journal "+req.Journal, err)
		}
		out.MoreFollows = res.MoreFollows
		for _, en := range res.Entries {
			entry := model.JournalEntry{EntryID: hex.EncodeToString(en.EntryID), Time: en.OccurrenceTime.UTC(), Variables: []model.JournalVariable{}}
			for _, v := range en.Variables {
				entry.Variables = append(entry.Variables, model.JournalVariable{Tag: v.Tag, Value: convertValue(v.Value.MMS(), nil, "")})
			}
			out.Entries = append(out.Entries, entry)
		}
		return nil
	})
	return out, err
}

// SettingGroups implements service.Service.
func (e *Engine) SettingGroups(ctx context.Context, id, ld string) (model.SettingGroups, error) {
	s, err := e.session(id)
	if err != nil {
		return model.SettingGroups{}, err
	}
	if err := s.checkLD(ld); err != nil {
		return model.SettingGroups{}, err
	}
	var out model.SettingGroups
	err = e.live(ctx, id, "setting-groups", ld, 1, func(ctx context.Context, s *session) error {
		info, err := s.client.GetSettingGroupInfo(ctx, ld)
		if err != nil {
			return errIED("read the setting group control block", err)
		}
		out = model.SettingGroups{LD: ld, NumOfSGs: info.NumOfSGs, ActSG: info.ActSG, EditSG: info.EditSG, CnfEdit: info.CnfEdit, ResvTms: info.ResvTms}
		return nil
	})
	return out, err
}
