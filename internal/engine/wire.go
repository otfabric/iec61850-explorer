// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// The wire view: application-to-wire correlation of the Explorer's own
// traffic. go-mms DialOptions.RawHook hands the recorder every ISO unit
// (session SPDU) the client sends and receives; it only observes, it
// cannot change or send anything. Units are kept while an operation runs
// and, when the operation is published, moved to a bounded ring of the
// session; Wire decodes them with go-mms (mms.DecodeTSDU) when asked.
// Units are never logged. go-mms decodes no credential, and the hex of an
// AARQ that carries authentication is withheld.

// Associations of a wire unit.
const (
	wireObserve    = "observe"
	wireCommission = "commission"
)

// maxWireBytes bounds the bytes a session's ring holds.
const maxWireBytes = 4 << 20

// wireUnit is one unit as captured.
type wireUnit struct {
	seq   uint64
	op    uint64
	at    time.Time
	dir   string
	assoc string
	size  int
	// data is nil for a unit larger than model.MaxWireUnitBytes.
	data []byte
}

// wireRing holds the newest units of a session's published operations.
type wireRing struct {
	mu    sync.Mutex
	seq   uint64
	units []wireUnit
	bytes int
	// evicted is the highest operation number whose units were dropped.
	evicted uint64
}

// hook returns the RawHook of an association.
func (r *recorder) hook(assoc string) func(direction string, raw []byte) {
	if r.wire == nil {
		return nil
	}
	return func(direction string, raw []byte) {
		u := wireUnit{at: time.Now().UTC(), dir: direction, assoc: assoc, size: len(raw)}
		if len(raw) <= model.MaxWireUnitBytes {
			u.data = slices.Clone(raw)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.active || len(r.pending) >= model.MaxWireUnits {
			return
		}
		u.op = r.op
		r.pending = append(r.pending, u)
	}
}

// commit moves the running action's units to the ring and returns its
// number, or 0 for a session without a ring.
func (r *recorder) commit() uint64 {
	r.mu.Lock()
	units, op := r.pending, r.op
	r.pending = nil
	r.mu.Unlock()
	if r.wire == nil {
		return 0
	}
	r.wire.add(units)
	return op
}

func (w *wireRing) add(units []wireUnit) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, u := range units {
		w.seq++
		u.seq = w.seq
		w.units = append(w.units, u)
		w.bytes += len(u.data)
	}
	for len(w.units) > 0 && (len(w.units) > model.MaxWireUnits || w.bytes > maxWireBytes) {
		w.evicted = max(w.evicted, w.units[0].op)
		w.bytes -= len(w.units[0].data)
		w.units = w.units[1:]
	}
}

// of returns the units of an operation and whether some of them may have
// been dropped.
func (w *wireRing) of(op uint64) ([]wireUnit, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []wireUnit
	for _, u := range w.units {
		if u.op == op {
			out = append(out, u)
		}
	}
	return out, op <= w.evicted
}

// Wire implements service.Service.
func (e *Engine) Wire(_ context.Context, id string, operation uint64) (model.WireView, error) {
	s, err := e.lookup(id)
	if err != nil {
		return model.WireView{}, err
	}
	if s.rec.wire == nil {
		return model.WireView{}, service.Errorf(service.CodeInvalidArgument, "session %s has no association: only live sessions record their traffic", id)
	}
	s.rec.mu.Lock()
	current := s.rec.op
	s.rec.mu.Unlock()
	if operation == 0 || operation > current {
		return model.WireView{}, service.Errorf(service.CodeNotFound, "session %s has no operation %d", id, operation)
	}
	units, evicted := s.rec.wire.of(operation)
	out := model.WireView{SessionID: id, Operation: operation, Units: make([]model.WireUnit, 0, len(units))}
	if evicted {
		out.Note = fmt.Sprintf("the session keeps the newest %d units; older ones of this operation are gone", model.MaxWireUnits)
	}
	for _, u := range units {
		out.Units = append(out.Units, wireUnitOf(u))
	}
	return out, nil
}

// wireUnitOf decodes a unit with go-mms.
func wireUnitOf(u wireUnit) model.WireUnit {
	out := model.WireUnit{Seq: u.seq, At: u.at, Direction: u.dir, Association: u.assoc, Bytes: u.size}
	if u.data == nil {
		out.DecodeError = fmt.Sprintf("a unit of %d bytes is not kept (at most %d)", u.size, model.MaxWireUnitBytes)
		return out
	}
	c, err := mms.DecodeTSDU(u.data)
	if c != nil {
		out.Session, out.Presentation = c.Session, c.Presentation
		if c.ACSE != nil {
			out.ACSE = c.ACSE.Kind
		}
		out.PDU = pduOf(c.PDU)
	}
	if err != nil {
		out.DecodeError = err.Error()
		if errors.Is(err, mms.ErrNotMMS) {
			out.DecodeError = "not an MMS session unit"
		}
	}
	if c != nil && c.ACSE != nil && c.ACSE.Kind == "AARQ" && c.ACSE.Authentication != "" && c.ACSE.Authentication != "none" {
		out.DecodeError = "the hex is withheld: this AARQ carries authentication (" + c.ACSE.Authentication + ")"
		return out
	}
	shown := u.data
	if len(shown) > model.MaxWireHexBytes {
		shown, out.Truncated = shown[:model.MaxWireHexBytes], true
	}
	out.Hex = hex.EncodeToString(shown)
	return out
}
