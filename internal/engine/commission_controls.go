// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// controlOrigin identifies the Explorer as the originator of its controls.
var controlOrigin = iec61850.Origin{OrCat: iec61850.OrCatRemoteControl, OrIdent: []byte("OT Fabric IEC 61850 Explorer")}

// ctlNums numbers the controls of this process (1 to 255, wrapping): the
// select with value and the operate of one control carry the same number.
var ctlNums atomic.Uint32

func nextCtlNum() uint8 {
	for {
		if n := uint8(ctlNums.Add(1)); n != 0 {
			return n
		}
	}
}

// Control implements service.Service. It reads the control model of the
// data object on the Observe association and runs its sequence on the
// Commission association: every step, the IED's reasons and the stVal
// before and after are in the result and in one audit record.
func (e *Engine) Control(ctx context.Context, id string, req model.ControlRequest) (model.ControlResult, error) {
	s, err := e.session(id)
	if err != nil {
		return model.ControlResult{}, err
	}
	if err := s.liveErr(); err != nil {
		return model.ControlResult{}, err
	}
	ref, err := iec61850.ParseRef(strings.TrimSpace(req.Ref))
	switch {
	case err != nil:
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "%q is not a reference: %v", req.Ref, err)
	case ref.FC != "" || ref.LN == "" || len(ref.Path) == 0:
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "a control needs a data object without a functional constraint, such as LD/XCBR1.Pos")
	case req.TimeoutMS < 0 || req.TimeoutMS > model.MaxControlTimeoutMS:
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "timeout_ms must be 0 to %d", model.MaxControlTimeoutMS)
	}
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = model.DefaultControlTimeoutMS * time.Millisecond
	}
	if _, err := s.requireGrant(model.CapControls); err != nil {
		return model.ControlResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.requireGrant(model.CapControls) // the grant may have ended meanwhile
	if err != nil {
		return model.ControlResult{}, err
	}
	s.rec.start()
	start := time.Now()
	res := model.ControlResult{Ref: ref.String(), At: start.UTC(), Test: req.Test, Steps: []model.ControlStep{}}

	tctx, cancel := s.timeoutCtx(ctx)
	ctlModel, err := s.client.ReadCtlModel(tctx, ref)
	cancel()
	if err != nil {
		opErr := classify(err, false)
		e.publishOperation(s.rec, s.id, "control", res.Ref, start, opErr)
		if opErr.Code == model.ErrCodeNotFound {
			return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "%s has no ctlModel[CF]: it is not a control", res.Ref)
		}
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "%s: read ctlModel: %s", res.Ref, opErr.Message)
	}
	res.CtlModel = ctlModel.String()
	if !ctlModel.IsControllable() || ctlModel > iec61850.CtlModelSBOEnhanced {
		e.publishOperation(s.rec, s.id, "control", res.Ref, start, &model.OpError{Code: model.ErrCodeInvalidRef, Message: "not controllable"})
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "%s has ctlModel %s: it is not operated", res.Ref, res.CtlModel)
	}

	ctlValRef := iec61850.Ref{LD: ref.LD, LN: ref.LN, Path: append(append([]string(nil), ref.Path...), "Oper", "ctlVal"), FC: fcControl}
	tctx, cancel = s.timeoutCtx(ctx)
	ts := e.typeOf(tctx, s, ctlValRef)
	cancel()
	ctlVal, err := parseValue(req.Value, ts)
	if err != nil {
		e.publishOperation(s.rec, s.id, "control", res.Ref, start, &model.OpError{Code: model.ErrCodeInvalidRef, Message: err.Error()})
		return model.ControlResult{}, service.Errorf(service.CodeInvalidArgument, "%s: ctlVal: %v", res.Ref, err)
	}
	written := convertValue(ctlVal, ts, "ctlVal")
	res.Written = &written

	stVal := iec61850.Ref{LD: ref.LD, LN: ref.LN, Path: append(append([]string(nil), ref.Path...), "stVal"), FC: "ST"}
	var before model.ReadValue
	e.readSingle(ctx, s, stVal, &before)
	res.Before = before.Value

	res.CtlNum = nextCtlNum()
	params := iec61850.OperateParams{CtlVal: ctlVal, CtlNum: res.CtlNum, Origin: &controlOrigin, Test: req.Test}
	if req.InterlockCheck {
		params.Check |= iec61850.CheckInterlockCheck
	}
	if req.SynchroCheck {
		params.Check |= iec61850.CheckSynchroCheck
	}
	res.InterlockCheck, res.SynchroCheck = req.InterlockCheck, req.SynchroCheck
	run := controlRun{ctx: ctx, s: s, g: g, ref: ref, params: params, timeout: timeout, res: &res}
	run.sequence(ctlModel)

	var after model.ReadValue
	e.readSingle(ctx, s, stVal, &after)
	res.After = after.Value
	res.DurationMS = since(start)

	rec := model.AuditRecord{
		Action: model.AuditControl, Ref: res.Ref, Written: written.Text,
		Before: stateText(res.Before), After: stateText(res.After), Outcome: model.OutcomeOK,
		Detail: run.detail(),
	}
	var opErr *model.OpError
	if res.Outcome != model.ControlOK {
		rec.Outcome = model.OutcomeFailed
		rec.Error = run.failure()
		opErr = &model.OpError{Code: model.ErrCodeRejected, Message: rec.Error}
	}
	e.appendAudit(s, rec)
	e.publishOperation(s.rec, s.id, "control", res.Ref, start, opErr)
	return res, nil
}

// controlRun is one control sequence on the grant's association.
type controlRun struct {
	ctx     context.Context
	s       *session
	g       *grant
	ref     iec61850.Ref
	params  iec61850.OperateParams
	timeout time.Duration
	res     *model.ControlResult
}

// sequence runs the services of the control model (IEC 61850-7-2,
// clause 20) and sets the outcome.
func (r *controlRun) sequence(m iec61850.CtlModel) {
	c := r.g.client
	if m.IsSBO() {
		ok := r.step(model.StepSelect, func(ctx context.Context) error {
			if m == iec61850.CtlModelSBONormal {
				_, err := c.Select(ctx, r.ref)
				return err
			}
			return c.SelectWithValue(ctx, r.ref, r.params)
		})
		if !ok {
			return
		}
	}
	if !m.IsEnhanced() {
		if !r.step(model.StepOperate, func(ctx context.Context) error { return c.Operate(ctx, r.ref, r.params) }) && m.IsSBO() {
			r.cancel()
		}
		if r.res.Outcome == "" {
			r.res.Outcome = model.ControlOK
		}
		return
	}

	// Enhanced security: the operate, then its CommandTermination.
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.ctx, r.timeout)
	defer cancel()
	term, err := c.OperateWait(ctx, r.ref, r.params)
	switch {
	case err == nil:
		r.add(model.StepOperate, start, nil)
		r.add(model.StepTermination, start, nil)
		r.res.Outcome = model.ControlOK
	case errors.Is(err, iec61850.ErrCommandTerminationNegative):
		r.add(model.StepOperate, start, nil)
		r.add(model.StepTermination, start, err)
		r.res.Outcome = model.ControlNegative
		if term != nil && term.LastApplError != nil {
			r.res.AddCause = term.LastApplError.AddCause.String()
		}
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil && r.ctx.Err() == nil:
		// OperateWait returns the deadline both for the operate write and
		// for the wait; the write fails with a ControlError.
		if _, isCtl := errors.AsType[*iec61850.ControlError](err); isCtl {
			r.add(model.StepOperate, start, err)
		} else {
			r.add(model.StepOperate, start, nil)
			r.add(model.StepTermination, start, fmt.Errorf("no CommandTermination within %s", r.timeout))
			r.res.Outcome = model.ControlNoTermination
		}
	default:
		r.add(model.StepOperate, start, err)
		if m.IsSBO() {
			r.cancel()
		}
	}
}

// step runs one service with the session timeout and records it.
func (r *controlRun) step(name string, fn func(context.Context) error) bool {
	start := time.Now()
	ctx, cancel := r.s.timeoutCtx(r.ctx)
	err := fn(ctx)
	cancel()
	r.add(name, start, err)
	return err == nil
}

// cancel withdraws a select whose operate failed; best effort, recorded.
func (r *controlRun) cancel() {
	c := r.g.client
	r.step(model.StepCancel, func(ctx context.Context) error {
		return c.Cancel(ctx, r.ref, iec61850.CancelParams{CtlVal: r.params.CtlVal, CtlNum: r.params.CtlNum, Origin: r.params.Origin})
	})
}

// add records a step; the first failure decides the outcome and the cause.
func (r *controlRun) add(name string, start time.Time, err error) {
	st := model.ControlStep{Step: name, OK: err == nil, DurationMS: since(start)}
	if err != nil {
		st.Error = classify(err, false).Message
		if ce, ok := errors.AsType[*iec61850.ControlError](err); ok && ce.LastApplError != nil {
			st.AddCause = ce.LastApplError.AddCause.String()
		}
		if r.res.Outcome == "" && name != model.StepCancel {
			r.res.Outcome, r.res.AddCause = model.ControlFailed, st.AddCause
			if st.AddCause != "" {
				r.res.Outcome = model.ControlRefused
			}
		}
	}
	r.res.Steps = append(r.res.Steps, st)
}

// dbposNames are the IEC 61850-7-2 names of a double point position.
var dbposNames = map[string]string{"00": "intermediate-state", "01": "off", "10": "on", "11": "bad-state"}

// stateText is a control's stVal for the audit trail. A two-bit bit string
// is a Dbpos (IEC 61850-7-3: the only stVal of that type) and is recorded by
// its name with the bits, "on (10)"; other values as they are shown.
func stateText(v *model.Value) string {
	if v != nil && v.Type == model.TypeBitString && v.Bits == 2 {
		if name, ok := dbposNames[v.Text]; ok {
			return name + " (" + v.Text + ")"
		}
	}
	return textOf(v)
}

// detail summarizes the sequence for the audit trail.
func (r *controlRun) detail() string {
	parts := []string{r.res.CtlModel, fmt.Sprintf("ctlNum %d", r.res.CtlNum)}
	if r.res.Test {
		parts = append(parts, "test")
	}
	if r.res.InterlockCheck {
		parts = append(parts, "interlock-check")
	}
	if r.res.SynchroCheck {
		parts = append(parts, "synchrocheck")
	}
	for _, st := range r.res.Steps {
		state := "ok"
		if !st.OK {
			state = "failed"
			if st.AddCause != "" {
				state = st.AddCause
			}
		}
		parts = append(parts, st.Step+" "+state)
	}
	return strings.Join(parts, "; ")
}

// failure describes why a control did not succeed.
func (r *controlRun) failure() string {
	msg := r.res.Outcome
	if r.res.AddCause != "" {
		msg += ": " + r.res.AddCause
	}
	for _, st := range r.res.Steps {
		if !st.OK && st.Step != model.StepCancel {
			return msg + " (" + st.Step + ": " + st.Error + ")"
		}
	}
	return msg
}
