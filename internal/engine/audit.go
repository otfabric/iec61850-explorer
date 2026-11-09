// SPDX-License-Identifier: MIT

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// auditLog is the audit trail of Commission mode (ADR 0004): JSON Lines, one
// file per UTC day in dir, mode 0600. A record is written and synced before
// the outcome it records is reported; the recent records stay in memory and
// are published as events. The sequence continues across engine runs: the
// recent records are loaded from the files on first use.
type auditLog struct {
	dir string
	hub *hub
	now func() time.Time
	log func(msg string, args ...any)

	loaded sync.Once
	mu     sync.Mutex
	seq    uint64
	recent []model.AuditRecord // oldest first, at most MaxAuditRecords
}

func newAuditLog(dir string, h *hub, log func(string, ...any)) *auditLog {
	return &auditLog{dir: dir, hub: h, now: time.Now, log: log}
}

// maxAuditLine bounds a line of an audit file the engine reads back.
const maxAuditLine = 64 << 10

// load reads the recent records of the newest files once. A file that
// cannot be read is logged; its records stay on disk.
func (a *auditLog) load() {
	a.loaded.Do(func() {
		files, _ := filepath.Glob(filepath.Join(a.dir, "audit-*.jsonl"))
		slices.Sort(files)
		var recs []model.AuditRecord
		for i := len(files) - 1; i >= 0 && len(recs) < model.MaxAuditRecords; i-- {
			got, err := readAuditFile(files[i])
			if err != nil {
				a.log("audit file not read", "file", files[i], "error", err)
			}
			recs = append(got, recs...)
		}
		if len(recs) > model.MaxAuditRecords {
			recs = recs[len(recs)-model.MaxAuditRecords:]
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, r := range recs {
			a.seq = max(a.seq, r.Seq)
		}
		a.recent = append(recs, a.recent...)
	})
}

func readAuditFile(path string) ([]model.AuditRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []model.AuditRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), maxAuditLine)
	for sc.Scan() {
		var r model.AuditRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return out, fmt.Errorf("line %d: %w", len(out)+1, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// writable checks that the trail can be written, without writing a record.
func (a *auditLog) writable() error {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path(a.now()), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func (a *auditLog) path(t time.Time) string {
	return filepath.Join(a.dir, "audit-"+t.UTC().Format("2006-01-02")+".jsonl")
}

// append writes rec to the file of the day and syncs it. Only a record that
// reached the disk is kept and published.
func (a *auditLog) append(rec model.AuditRecord) (model.AuditRecord, error) {
	a.load()
	a.mu.Lock()
	defer a.mu.Unlock()
	rec.Seq = a.seq + 1
	rec.At = a.now().UTC()
	b, err := json.Marshal(rec)
	if err != nil {
		return rec, err
	}
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return rec, err
	}
	f, err := os.OpenFile(a.path(rec.At), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return rec, err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return rec, fmt.Errorf("audit trail: %w", err)
	}
	a.seq = rec.Seq
	a.recent = append(a.recent, rec)
	if len(a.recent) > model.MaxAuditRecords {
		a.recent = a.recent[len(a.recent)-model.MaxAuditRecords:]
	}
	r := rec
	a.hub.publish(model.Event{Kind: model.EventAudit, SessionID: rec.SessionID, Audit: &r})
	return rec, nil
}

// query returns recent records, newest first.
func (a *auditLog) query(q model.AuditQuery) []model.AuditRecord {
	limit := q.Limit
	if limit <= 0 || limit > model.MaxAuditRecords {
		limit = model.MaxAuditRecords
	}
	a.load()
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []model.AuditRecord{}
	for i := len(a.recent) - 1; i >= 0 && len(out) < limit; i-- {
		if q.SessionID == "" || a.recent[i].SessionID == q.SessionID {
			out = append(out, a.recent[i])
		}
	}
	return out
}

// Audit implements service.Service.
func (e *Engine) Audit(_ context.Context, q model.AuditQuery) ([]model.AuditRecord, error) {
	if err := e.commissionErr(); err != nil {
		return nil, err
	}
	return e.audit.query(q), nil
}
