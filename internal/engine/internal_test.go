// SPDX-License-Identifier: MIT

package engine

import (
	"errors"
	"testing"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

func TestRecorder(t *testing.T) {
	r := &recorder{}
	r.ObserveRequest(mms.RequestEvent{Service: mms.ServiceRead}) // not active: ignored
	r.start()
	for i := range model.MaxOperationRequests + 3 {
		ev := mms.RequestEvent{Service: mms.ServiceRead, InvokeID: mms.InvokeID(i)}
		if i == 0 {
			ev.Err = errors.New("boom")
		}
		r.ObserveRequest(ev)
	}
	reqs, dropped := r.stop()
	if len(reqs) != model.MaxOperationRequests || dropped != 3 || reqs[0].Error != "boom" || reqs[0].Service != "Read" {
		t.Errorf("requests = %d, dropped %d, first %+v", len(reqs), dropped, reqs[0])
	}
	r.start()
	if reqs, _ := r.stop(); reqs == nil || len(reqs) != 0 {
		t.Errorf("empty action = %#v", reqs)
	}
}

// Lost sessions are kept for the user to see, but not without bound.
func TestPruneLost(t *testing.T) {
	e := New(Options{})
	defer e.Close()
	for i := range maxLostSessions + 3 {
		id := string(rune('a' + i))
		e.sessions[id] = &session{id: id, state: model.SessionLost}
		e.order = append(e.order, id)
	}
	e.pruneLost()
	if len(e.sessions) != maxLostSessions || len(e.order) != maxLostSessions || e.order[0] != "d" {
		t.Errorf("kept %d sessions, order %v", len(e.sessions), e.order)
	}
	list, _ := e.Sessions(t.Context())
	if len(list) != maxLostSessions {
		t.Errorf("Sessions = %d", len(list))
	}
}
