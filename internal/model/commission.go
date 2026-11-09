// SPDX-License-Identifier: MIT

package model

import "time"

// ModeCommission: the session holds a Commission grant (ADR 0004). Its
// changes go through a second association whose allow-list follows the
// granted capabilities; reads still use the Observe association.
const ModeCommission = "commission"

// FeatureCommission: the engine grants Commission mode. The daemon offers it
// only when started with --allow-commission.
const FeatureCommission = "commission"

// Capabilities a Commission grant may carry.
const (
	// CapWrite: write data attributes of the writable functional
	// constraints (WritableFCs).
	CapWrite = "write"
	// CapReports: configure, enable and subscribe to report control blocks.
	CapReports = "reports"
	// CapDataSets: create and delete data sets.
	CapDataSets = "datasets"
	// CapSettingGroups: select the active and the edit setting group.
	CapSettingGroups = "setting-groups"
)

// WritableFCs are the functional constraints a write may target: set
// points, configuration, description, substitution and blocking (IEC
// 61850-7-2). Controls (CO) and setting group values (SE) have their own
// services.
var WritableFCs = []string{"SP", "CF", "DC", "SV", "BL"}

// Limits of a grant.
const (
	DefaultGrantMinutes = 30
	MaxGrantMinutes     = 480
	MaxGrantReasonLen   = 200
)

// GrantRequest asks for Commission mode on a session.
type GrantRequest struct {
	// Reason is recorded in the audit trail, for example a work order.
	Reason string `json:"reason"`
	// Minutes is the duration of the grant; zero selects
	// DefaultGrantMinutes, at most MaxGrantMinutes.
	Minutes      int      `json:"minutes,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// Grant is a Commission grant in force.
type Grant struct {
	Reason       string    `json:"reason"`
	Capabilities []string  `json:"capabilities"`
	GrantedAt    time.Time `json:"granted_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// WriteRequest writes one data attribute. Value is text in the form a read
// shows it: true or false, a number, a string, bits as 0101, octets as hex,
// a time as RFC 3339.
type WriteRequest struct {
	Ref   string `json:"ref"`
	Value string `json:"value"`
}

// WriteResult is a verified write: the value before, the value written and
// the value read back.
type WriteResult struct {
	Ref string    `json:"ref"`
	MMS MMSName   `json:"mms"`
	At  time.Time `json:"at"`
	// Written is the value sent, as the IED's type renders it.
	Written *Value `json:"written,omitempty"`
	Before  *Value `json:"before,omitempty"`
	After   *Value `json:"after,omitempty"`
	// Verified is true when the value read back equals the value written.
	Verified bool `json:"verified"`
	// Error is set when the IED refused the write; nothing was changed.
	Error      *OpError `json:"error,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// Audit actions.
const (
	AuditGrant   = "grant"
	AuditRevoke  = "revoke"
	AuditExpire  = "expire"
	AuditEnd     = "end" // the session closed, was lost, or the engine stopped
	AuditWrite   = "write"
	AuditCleanup = "cleanup"
)

// Audit outcomes.
const (
	OutcomeOK = "ok"
	// OutcomeMismatch: the IED accepted the write, the value read back
	// differs.
	OutcomeMismatch = "mismatch"
	OutcomeFailed   = "failed"
)

// AuditRecord is one entry of the audit trail.
type AuditRecord struct {
	// Seq increases by one per record of an engine run.
	Seq       uint64    `json:"seq"`
	At        time.Time `json:"at"`
	SessionID string    `json:"session_id"`
	// Target is host:port of the IED; IED its identity when known.
	Target  string `json:"target"`
	IED     string `json:"ied,omitempty"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	// Reason and Capabilities are set on a grant.
	Reason       string   `json:"reason,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Ref          string   `json:"ref,omitempty"`
	Before       string   `json:"before,omitempty"`
	Written      string   `json:"written,omitempty"`
	After        string   `json:"after,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// MaxAuditRecords is the number of recent records an engine keeps in memory
// and returns; the files in the data directory hold all of them.
const MaxAuditRecords = 500

// AuditQuery selects recent audit records, newest first.
type AuditQuery struct {
	// SessionID, when set, selects one session's records.
	SessionID string `json:"session_id,omitempty"`
	// Limit bounds the answer; zero or more than MaxAuditRecords selects
	// MaxAuditRecords.
	Limit int `json:"limit,omitempty"`
}
