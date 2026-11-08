// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// readBatch is the number of references in one MMS Read request. Larger
// batches risk answers beyond the negotiated PDU size; a batch that fails as
// a whole is read again one reference at a time.
const readBatch = 32

// Read implements service.Service. A read of several references is one
// operation; object references are read in batches with one MMS Read each.
func (e *Engine) Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error) {
	switch {
	case len(req.Refs) == 0:
		return model.ReadResult{}, service.Errorf(service.CodeInvalidArgument, "no reference to read")
	case len(req.Refs) > model.MaxReadRefs:
		return model.ReadResult{}, service.Errorf(service.CodeLimitExceeded, "at most %d references per read", model.MaxReadRefs)
	}
	s, err := e.session(id)
	if err != nil {
		return model.ReadResult{}, err
	}
	if st := s.info().State; st != model.SessionConnected {
		return model.ReadResult{}, service.Errorf(service.CodeSessionLost, "the session is %s", st)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	res, opErr := e.readLocked(ctx, s, req.Refs)
	target := res.Values[0].Ref
	if len(req.Refs) > 1 {
		target = fmt.Sprintf("%d references", len(req.Refs))
	}
	e.publishOperation(s.rec, s.id, "read", target, res.At, opErr)
	return res, nil
}

// readLocked reads refs; s.mu is held. The returned error describes a read
// that failed as a whole (the association failed), for the operation.
func (e *Engine) readLocked(ctx context.Context, s *session, raw []string) (model.ReadResult, *model.OpError) {
	start := time.Now()
	s.rec.start()
	res := model.ReadResult{At: start.UTC(), Values: make([]model.ReadValue, len(raw))}

	// Parse; collect the object references for batched reads.
	refs := make([]iec61850.Ref, len(raw))
	var batch []int
	for i, r := range raw {
		res.Values[i].Ref = r
		ref, err := iec61850.ParseRef(strings.TrimSpace(r))
		if err == nil && ref.FC == "" {
			err = &iec61850.ReferenceError{Input: r, Reason: "functional constraint required, such as [MX]"}
		}
		if err == nil && ref.LN == "" {
			err = &iec61850.ReferenceError{Input: r, Reason: "logical node required"}
		}
		if err != nil {
			res.Values[i].Error = classify(err, false)
			res.Values[i].Error.Code = model.ErrCodeInvalidRef
			continue
		}
		refs[i] = ref
		res.Values[i].Ref = ref.String()
		if dom, item, err := ref.ToMMS(); err == nil {
			res.Values[i].MMS = model.MMSName{Domain: string(dom), ItemID: string(item)}
		}
		if ref.IsObject() {
			batch = append(batch, i)
		} else {
			e.readSingle(ctx, s, ref, &res.Values[i])
		}
	}

	var whole *model.OpError
	for len(batch) > 0 {
		n := min(readBatch, len(batch))
		if err := e.readMany(ctx, s, refs, batch[:n], res.Values); err != nil {
			whole = err
		}
		batch = batch[n:]
	}
	res.DurationMS = since(start)
	return res, whole
}

// readMany reads the references at idx with one MMS Read. A request that
// fails as a whole, other than by a lost association, is retried one
// reference at a time. It returns the error of a lost association.
func (e *Engine) readMany(ctx context.Context, s *session, refs []iec61850.Ref, idx []int, out []model.ReadValue) *model.OpError {
	if len(idx) == 1 {
		e.readSingle(ctx, s, refs[idx[0]], &out[idx[0]])
		return nil
	}
	group := make([]iec61850.Ref, len(idx))
	for k, i := range idx {
		group[k] = refs[i]
	}
	tctx, cancel := s.timeoutCtx(ctx)
	results, err := s.client.ReadMultiple(tctx, group)
	cancel()
	if err != nil {
		if errors.Is(err, mms.ErrClosed) || errors.Is(err, iec61850.ErrClosed) {
			op := classify(err, false)
			for _, i := range idx {
				out[i].Error = op
			}
			return op
		}
		e.log.Debug("batched read failed, reading one by one", "session", s.id, "error", err)
		for _, i := range idx {
			e.readSingle(ctx, s, refs[i], &out[i])
		}
		return nil
	}
	for k, i := range idx {
		if results[k].Err != nil {
			out[i].Error = classify(results[k].Err, false)
			continue
		}
		e.setValue(ctx, s, refs[i], results[k].Value.MMS(), &out[i])
	}
	return nil
}

// readSingle reads one reference with its own MMS Read.
func (e *Engine) readSingle(ctx context.Context, s *session, ref iec61850.Ref, out *model.ReadValue) {
	tctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	v, err := s.client.ReadRaw(tctx, ref)
	if err != nil {
		out.Error = classify(err, false)
		return
	}
	e.setValue(ctx, s, ref, v, out)
}

// setValue converts a read value with the type of its variable.
func (e *Engine) setValue(ctx context.Context, s *session, ref iec61850.Ref, v *mms.Value, out *model.ReadValue) {
	tctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	name := ""
	if len(ref.Path) > 0 {
		name = ref.Path[len(ref.Path)-1]
	}
	val := convertValue(v, e.typeOf(tctx, s, ref), name)
	out.Value = &val
}

// typeOf returns the MMS type of ref, from the cache or the IED. A failure
// leaves components unnamed; it does not fail the read. s.mu must be held.
func (e *Engine) typeOf(ctx context.Context, s *session, ref iec61850.Ref) *mms.TypeSpec {
	key := ref.String()
	if ts, ok := s.types[key]; ok {
		return ts
	}
	if !ref.IsObject() {
		return nil
	}
	ts, err := s.client.GetVariableType(ctx, ref)
	if err != nil {
		e.log.Debug("no type", "ref", key, "error", err)
		ts = nil
	}
	s.types[key] = ts
	return ts
}

// Search implements service.Service. It walks the discovered model.
func (e *Engine) Search(_ context.Context, id string, q model.SearchQuery) (model.SearchResult, error) {
	s, err := e.session(id)
	if err != nil {
		return model.SearchResult{}, err
	}
	limit := q.Limit
	if limit <= 0 || limit > model.MaxSearchHits {
		limit = model.MaxSearchHits
	}
	text := strings.ToLower(strings.TrimSpace(q.Text))
	class := strings.ToUpper(strings.TrimSpace(q.LNClass))
	fc := strings.ToUpper(strings.TrimSpace(q.FC))
	if text == "" && class == "" && fc == "" && q.Kind == "" {
		return model.SearchResult{}, service.Errorf(service.CodeInvalidArgument, "an empty search")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	res := model.SearchResult{Hits: []model.Node{}}
	var walk func(n *iec61850.ModelNode, lnClassOf string) bool
	walk = func(n *iec61850.ModelNode, lnClassOf string) bool {
		for _, c := range n.Children {
			node := nodeOf(c)
			cls := lnClassOf
			if node.Kind == model.NodeLN {
				cls = node.LNClass
			}
			if matches(node, cls, text, class, fc, q.Kind) {
				if len(res.Hits) == limit {
					res.Truncated = true
					return false
				}
				res.Hits = append(res.Hits, node)
			}
			if !walk(c, cls) {
				return false
			}
		}
		return true
	}
	walk(s.index[""], "")
	return res, nil
}

func matches(n model.Node, lnClass, text, class, fc, kind string) bool {
	switch {
	case text != "" && !strings.Contains(strings.ToLower(n.Ref), text):
		return false
	case class != "" && lnClass != class:
		return false
	case kind != "" && n.Kind != kind:
		return false
	case fc != "":
		for _, f := range n.FCs {
			if f == fc {
				return true
			}
		}
		return false
	}
	return true
}
