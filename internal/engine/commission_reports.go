// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// reportQueue is the number of reports a subscription buffers before it
// drops (and counts) new ones.
const reportQueue = 256

// subscription is a report subscription of a grant. The engine runs the
// RCB's lifecycle itself (reserve, enable, GI; disable, release) so that
// each step's outcome reaches the audit trail; go-iec61850's subscription
// is passive.
type subscription struct {
	info     model.Subscription
	ld, item string
	reserved bool
	sub      *iec61850.ReportSubscription
	// members and types label the entries of a report by inclusion index.
	members []iec61850.DataSetMember
	types   []*mms.TypeSpec

	reports, dropped atomic.Uint64
	done             chan struct{}
}

func (sub *subscription) snapshot() model.Subscription {
	out := sub.info
	out.Reports, out.Dropped = sub.reports.Load(), sub.dropped.Load()
	return out
}

// splitRCBRef turns "LD/LLN0.RP.urcbMeas" into its logical device and MMS
// item ID, and says whether the RCB is buffered.
func splitRCBRef(ref string) (ld, item string, buffered bool, err error) {
	ld, item, err = splitDataSetRef(ref)
	if err != nil {
		return "", "", false, service.Errorf(service.CodeInvalidArgument, "%q is not a report control block reference such as LD/LLN0.RP.urcb01", ref)
	}
	parts := strings.Split(item, "$")
	if len(parts) < 3 || (parts[1] != "RP" && parts[1] != "BR") {
		return "", "", false, service.Errorf(service.CodeInvalidArgument, "%q is not a report control block reference such as LD/LLN0.RP.urcb01", ref)
	}
	return ld, item, parts[1] == "BR", nil
}

// splitDatSet turns an RCB's DatSet ("LD/LLN0$Meas") into its logical
// device and item ID.
func splitDatSet(datSet string) (ld, item string, ok bool) {
	ld, item, ok = strings.Cut(datSet, "/")
	if !ok || ld == "" || item == "" {
		return "", "", false
	}
	return ld, strings.ReplaceAll(item, ".", "$"), true
}

// SubscribeReport implements service.Service.
func (e *Engine) SubscribeReport(ctx context.Context, id string, req model.SubscribeRequest) (model.Subscription, error) {
	ld, item, buffered, err := splitRCBRef(req.RCB)
	if err != nil {
		return model.Subscription{}, err
	}
	var out model.Subscription
	err = e.commissionAction(ctx, id, model.CapReports, "subscribe", req.RCB, func(ctx context.Context, s *session, g *grant) error {
		g.mu.Lock()
		n := len(g.subs)
		g.mu.Unlock()
		if n >= model.MaxSubscriptions {
			return service.Errorf(service.CodeLimitExceeded, "at most %d report subscriptions per grant", model.MaxSubscriptions)
		}
		for _, other := range g.subs {
			if other.ld == ld && other.item == item {
				return service.Errorf(service.CodeNotAllowed, "this grant already subscribes to %s", req.RCB)
			}
		}
		audit := model.AuditRecord{Action: model.AuditSubscribe, Ref: req.RCB}
		fail := func(err error) error {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
			e.appendAudit(s, audit)
			return err
		}

		cb, err := g.client.GetReportControlBlock(ctx, ld, item)
		if err != nil {
			return errIED("read the report control block", err)
		}
		switch {
		case cb.RptEna:
			return service.Errorf(service.CodeNotAllowed, "%s is enabled: another client may own it", req.RCB)
		case !buffered && cb.Resv:
			return service.Errorf(service.CodeNotAllowed, "%s is reserved by another client", req.RCB)
		case buffered && cb.ResvTms != 0:
			return service.Errorf(service.CodeNotAllowed, "%s is reserved (ResvTms %d)", req.RCB, cb.ResvTms)
		}
		sub := &subscription{
			ld: ld, item: item, done: make(chan struct{}),
			info: model.Subscription{
				ID: newID(), RCB: req.RCB, MMS: model.MMSName{Domain: ld, ItemID: item}, Buffered: buffered,
				RptID: cb.RptID, DatSet: cb.DatSet, Since: time.Now().UTC(),
			},
		}
		if sub.info.RptID == "" {
			// An empty RptID reports with the RCB's reference.
			sub.info.RptID = ld + "/" + item
		}
		if dsLD, dsItem, ok := splitDatSet(cb.DatSet); ok {
			if ds, err := s.client.GetDataSet(ctx, dsLD, dsItem); err == nil {
				sub.members = ds.Members
				sub.types = make([]*mms.TypeSpec, len(ds.Members))
				for i, m := range ds.Members {
					sub.types[i] = e.typeOf(ctx, s, m.Ref)
				}
			}
		}

		// Passive first, so that no report of the enable or the GI is
		// missed. go-iec61850 closes a subscription when its context
		// ends: it lives until the engine stops it, not for this request.
		sub.sub, err = g.client.SubscribeReport(context.WithoutCancel(ctx), sub.info.RptID, iec61850.SubscribeReportOptions{
			QueueSize: reportQueue, OverflowPolicy: iec61850.OverflowCallback,
			OnOverflow: func(*iec61850.ReportIndication) { sub.dropped.Add(1) },
		})
		if err != nil {
			return fail(errIED("subscribe", err))
		}
		if !buffered {
			if err := g.client.ReserveURCB(ctx, ld, item); err != nil {
				_ = sub.sub.Close()
				return fail(errIED("reserve "+req.RCB, err))
			}
			sub.reserved = true
		}
		if err := g.client.SetReportControlBlock(ctx, ld, item, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: true}); err != nil {
			_ = sub.sub.Close()
			undoErr := sub.release(context.WithoutCancel(ctx), g.client)
			if undoErr != nil {
				audit.Detail = "not released: " + undoErr.Error()
			}
			return fail(errIED("enable "+req.RCB, err))
		}
		audit.Detail = fmt.Sprintf("RptID %s, data set %s", sub.info.RptID, cb.DatSet)
		if req.GI {
			if err := g.client.TriggerGI(ctx, ld, item); err != nil {
				audit.Detail += "; general interrogation refused: " + classify(err, false).Message
			} else {
				audit.Detail += "; general interrogation"
			}
		}

		g.mu.Lock()
		g.subs[sub.info.ID] = sub
		g.mu.Unlock()
		g.cleanup = append(g.cleanup, cleanupStep{id: sub.info.ID, what: "unsubscribe " + req.RCB, undo: sub.stop})
		go e.deliver(s, sub)
		audit.Outcome = model.OutcomeOK
		e.appendAudit(s, audit)
		out = sub.snapshot()
		return nil
	})
	if err == nil {
		s, _ := e.session(id)
		e.publishSession(s)
	}
	return out, err
}

// release disables the RCB, and releases it when the engine reserved it.
func (sub *subscription) release(ctx context.Context, c *iec61850.Client) error {
	var errs []error
	if err := c.SetReportControlBlock(ctx, sub.ld, sub.item, iec61850.RCBUpdate{Fields: iec61850.RCBFieldRptEna, RptEna: false}); err != nil {
		errs = append(errs, fmt.Errorf("disable: %w", err))
	}
	if sub.reserved {
		if err := c.ReleaseURCB(ctx, sub.ld, sub.item); err != nil {
			errs = append(errs, fmt.Errorf("release: %w", err))
		}
	}
	return errors.Join(errs...)
}

// stop ends the subscription: stop delivering, then disable and release.
func (sub *subscription) stop(ctx context.Context, c *iec61850.Client) error {
	_ = sub.sub.Close() // passive: no remote requests
	<-sub.done
	return sub.release(ctx, c)
}

// deliver publishes the subscription's reports until it is closed.
func (e *Engine) deliver(s *session, sub *subscription) {
	defer close(sub.done)
	for ri := range sub.sub.Reports() {
		sub.reports.Add(1)
		r := sub.convert(ri)
		e.hub.publish(model.Event{Kind: model.EventReport, SessionID: s.id, Report: &r})
	}
}

// convert turns a report into its DTO. The members of the RCB's data set
// name the entries; a report's own data references take precedence.
func (sub *subscription) convert(ri *iec61850.ReportIndication) model.Report {
	out := model.Report{
		SubscriptionID: sub.info.ID, RCB: sub.info.RCB, RptID: ri.RptID, ReceivedAt: time.Now().UTC(),
		BufOvfl: ri.BufOvfl, Entries: []model.ReportEntry{},
	}
	if ri.OptFlds&iec61850.OptFldSeqNum != 0 {
		n := ri.SeqNum
		out.SeqNum = &n
	}
	if ri.OptFlds&iec61850.OptFldTimeStamp != 0 && !ri.Timestamp.IsZero() {
		t := ri.Timestamp.UTC()
		out.Time = &t
	}
	if ri.OptFlds&iec61850.OptFldConfRev != 0 {
		n := ri.ConfRev
		out.ConfRev = &n
	}
	if len(ri.EntryID) > 0 {
		out.EntryID = hex.EncodeToString(ri.EntryID)
	}
	k := 0
	for i, included := range ri.Inclusion {
		if !included {
			continue
		}
		if k >= len(ri.Values) {
			break
		}
		entry := model.ReportEntry{}
		var ts *mms.TypeSpec
		name := ""
		if i < len(sub.members) {
			entry.Ref = sub.members[i].Ref.String()
			ts = sub.types[i]
			if p := sub.members[i].Ref.Path; len(p) > 0 {
				name = p[len(p)-1]
			}
		}
		if k < len(ri.DataReferences) && ri.DataReferences[k] != "" {
			entry.Ref = refOfMMS(ri.DataReferences[k])
		}
		if entry.Ref == "" {
			entry.Ref = fmt.Sprintf("member %d", i+1)
		}
		if k < len(ri.ReasonCodes) {
			entry.Reasons = flagList(ri.ReasonCodes[k].String())
		}
		var v *mms.Value
		if ri.Values[k] != nil {
			v = ri.Values[k].MMS()
		}
		entry.Value = convertValue(v, ts, name)
		out.Entries = append(out.Entries, entry)
		k++
	}
	return out
}

// refOfMMS turns a data reference in MMS form ("LD/LN$FC$DO$DA") into an
// FC-qualified object reference ("LD/LN.DO.DA[FC]"); anything else is
// returned as it is.
func refOfMMS(s string) string {
	ld, item, ok := strings.Cut(s, "/")
	parts := strings.Split(item, "$")
	if !ok || ld == "" || len(parts) < 3 || len(parts[1]) != 2 {
		return s
	}
	ref := ld + "/" + parts[0] + "." + strings.Join(parts[2:], ".") + "[" + parts[1] + "]"
	if _, err := iec61850.ParseRef(ref); err != nil {
		return s
	}
	return ref
}

// findSub returns a subscription of the grant.
func (g *grant) findSub(id string) (*subscription, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	sub, ok := g.subs[id]
	if !ok {
		return nil, service.Errorf(service.CodeNotFound, "no report subscription %q", id)
	}
	return sub, nil
}

// Unsubscribe implements service.Service.
func (e *Engine) Unsubscribe(ctx context.Context, id, subscriptionID string) error {
	err := e.commissionAction(ctx, id, model.CapReports, "unsubscribe", subscriptionID, func(ctx context.Context, s *session, g *grant) error {
		sub, err := g.findSub(subscriptionID)
		if err != nil {
			return err
		}
		g.mu.Lock()
		delete(g.subs, subscriptionID)
		g.mu.Unlock()
		g.removeCleanup(subscriptionID)
		audit := model.AuditRecord{Action: model.AuditUnsubscribe, Ref: sub.info.RCB, Outcome: model.OutcomeOK}
		err = sub.stop(ctx, g.client)
		if err != nil {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
		}
		e.appendAudit(s, audit)
		if err != nil {
			return errIED("unsubscribe "+sub.info.RCB, err)
		}
		return nil
	})
	if s, serr := e.session(id); serr == nil {
		e.publishSession(s)
	}
	return err
}

// TriggerGI implements service.Service.
func (e *Engine) TriggerGI(ctx context.Context, id, subscriptionID string) error {
	return e.commissionAction(ctx, id, model.CapReports, "gi", subscriptionID, func(ctx context.Context, s *session, g *grant) error {
		sub, err := g.findSub(subscriptionID)
		if err != nil {
			return err
		}
		audit := model.AuditRecord{Action: model.AuditGI, Ref: sub.info.RCB, Outcome: model.OutcomeOK}
		if err := g.client.TriggerGI(ctx, sub.ld, sub.item); err != nil {
			audit.Outcome, audit.Error = model.OutcomeFailed, err.Error()
			e.appendAudit(s, audit)
			return errIED("general interrogation", err)
		}
		e.appendAudit(s, audit)
		return nil
	})
}
