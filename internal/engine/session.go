// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"strings"
	"sync"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// session is one association with an IED and the model discovered on it.
//
// mu serialises the user actions of a session, so that the MMS requests a
// recorder sees during an action belong to that action.
type session struct {
	id       string
	target   model.TargetConfig
	timeout  time.Duration
	openedAt time.Time

	mu     sync.Mutex
	client *iec61850.Client
	rec    *recorder
	// index maps an object reference without FC to its tree node; "" is the
	// synthetic root.
	index    map[string]*iec61850.ModelNode
	summary  model.DiscoverySummary
	identity *model.Identity
	// types caches GetVariableAccessAttributes per FC-qualified reference.
	types map[string]*mms.TypeSpec

	stateMu sync.Mutex
	state   string
	lostErr *model.OpError
	closing bool
	poller  *poller
	// scl is set for an offline session.
	scl *model.SCLOrigin
}

func (s *session) info() model.SessionInfo {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	info := model.SessionInfo{
		ID: s.id, Target: s.target, State: s.state, Mode: model.ModeObserve,
		OpenedAt: s.openedAt, Identity: s.identity, Discovery: s.summary, Error: s.lostErr, SCL: s.scl,
	}
	if s.poller != nil {
		info.Polling = s.poller.snapshot()
	}
	return info
}

// setTree indexes a discovered tree and counts it.
func (s *session) setTree(root *iec61850.ModelNode, took time.Duration) {
	s.index = map[string]*iec61850.ModelNode{"": root}
	sum := model.DiscoverySummary{DurationMS: took.Milliseconds()}
	var walk func(n *iec61850.ModelNode, depth int)
	walk = func(n *iec61850.ModelNode, depth int) {
		for _, c := range n.Children {
			s.index[refKey(c.Reference)] = c
			switch depth {
			case 1:
				sum.LogicalDevices++
			case 2:
				sum.LogicalNodes++
			case 3:
				sum.DataObjects++
				sum.Nodes++
			default:
				sum.Nodes++
			}
			walk(c, depth+1)
		}
	}
	walk(root, 1)
	s.summary = sum
}

// refKey is the index key of a reference: its string form without FC.
func refKey(r iec61850.Ref) string {
	r.FC = ""
	return r.String()
}

// nodeOf converts a tree node to its DTO.
func nodeOf(n *iec61850.ModelNode) model.Node {
	out := model.Node{Name: n.Name, Ref: refKey(n.Reference), FCs: []string{}, Children: len(n.Children)}
	switch {
	case n.Reference.IsLD():
		out.Kind = model.NodeLD
	case n.Reference.IsLN():
		out.Kind, out.LNClass = model.NodeLN, lnClass(n.Name)
	case len(n.Reference.Path) == 1:
		out.Kind = model.NodeDO
	default:
		out.Kind = model.NodeChild
	}
	fcs := n.FCs
	if len(fcs) == 0 && n.FC != "" {
		fcs = []iec61850.FunctionalConstraint{n.FC}
	}
	for _, fc := range fcs {
		out.FCs = append(out.FCs, string(fc))
	}
	return out
}

// lnClass returns the class of a logical node name: the four upper-case
// letters after an optional prefix and before the instance number, such as
// "MMXU" of "MMXU1" or of "Q0MMXU1". LLN0 is its own class.
func lnClass(name string) string {
	if name == "LLN0" {
		return name
	}
	end := len(name)
	for end > 0 && name[end-1] >= '0' && name[end-1] <= '9' {
		end--
	}
	if end < 4 {
		return ""
	}
	class := name[end-4 : end]
	if strings.ToUpper(class) != class {
		return ""
	}
	return class
}

// recorder collects the MMS requests of the action that is running. It is
// the go-mms Metrics of the session's client; its methods are called on the
// client's goroutines and must not block.
type recorder struct {
	mms.NopMetrics
	mu       sync.Mutex
	active   bool
	requests []model.MMSRequest
	dropped  int
}

func (r *recorder) ObserveRequest(ev mms.RequestEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return
	}
	if len(r.requests) >= model.MaxOperationRequests {
		r.dropped++
		return
	}
	req := model.MMSRequest{
		Service:    ev.Service.String(),
		InvokeID:   uint32(ev.InvokeID),
		DurationMS: float64(ev.Duration.Microseconds()) / 1000,
	}
	if ev.Err != nil {
		req.Error = ev.Err.Error()
	}
	r.requests = append(r.requests, req)
}

// start begins collecting for a new action.
func (r *recorder) start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active, r.requests, r.dropped = true, nil, 0
}

// stop ends the action and returns its requests.
func (r *recorder) stop() ([]model.MMSRequest, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = false
	reqs := r.requests
	if reqs == nil {
		reqs = []model.MMSRequest{}
	}
	return reqs, r.dropped
}

// liveErr refuses an operation that needs the IED when the session has
// none, or lost it.
func (s *session) liveErr() error {
	switch st := s.info().State; st {
	case model.SessionConnected:
		return nil
	case model.SessionOffline:
		return service.Errorf(service.CodeNotAllowed, "an SCL model has no live values: connect to the IED to read")
	default:
		return service.Errorf(service.CodeSessionLost, "the session is %s", st)
	}
}

// timeoutCtx bounds ctx by the session's timeout.
func (s *session) timeoutCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.timeout)
}
