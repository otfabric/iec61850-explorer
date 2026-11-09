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
	// Subscriptions are the report subscriptions of the grant.
	Subscriptions []Subscription `json:"subscriptions"`
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
	// Detail adds what the other fields do not hold, such as the members
	// of a created data set.
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
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

// More audit actions (M2b2).
const (
	AuditSubscribe   = "subscribe"
	AuditUnsubscribe = "unsubscribe"
	AuditGI          = "gi"
	AuditCreateDS    = "create-dataset"
	AuditDeleteDS    = "delete-dataset"
	AuditActiveSG    = "select-active-sg"
)

// MaxSubscriptions bounds the report subscriptions of one grant.
const MaxSubscriptions = 8

// SubscribeRequest subscribes to a report control block: the engine
// reserves it (unbuffered), enables it and, with GI, asks for a general
// interrogation. Revoking the grant or unsubscribing disables and releases
// it again.
type SubscribeRequest struct {
	// RCB is the reference Reports returns, such as "LD/LLN0.RP.urcbMeas".
	RCB string `json:"rcb"`
	GI  bool   `json:"gi,omitempty"`
}

// Subscription is a report subscription of a grant.
type Subscription struct {
	ID       string    `json:"id"`
	RCB      string    `json:"rcb"`
	MMS      MMSName   `json:"mms"`
	Buffered bool      `json:"buffered"`
	RptID    string    `json:"rpt_id"`
	DatSet   string    `json:"dat_set"`
	Since    time.Time `json:"since"`
	Reports  uint64    `json:"reports"`
	// Dropped counts reports lost because the engine's queue was full.
	Dropped uint64 `json:"dropped"`
}

// Report is one report the IED sent to a subscription.
type Report struct {
	SubscriptionID string    `json:"subscription_id"`
	RCB            string    `json:"rcb"`
	RptID          string    `json:"rpt_id"`
	ReceivedAt     time.Time `json:"received_at"`
	// The fields below are present when the RCB's OptFlds include them.
	SeqNum  *uint32       `json:"seq_num,omitempty"`
	Time    *time.Time    `json:"time,omitempty"`
	ConfRev *uint32       `json:"conf_rev,omitempty"`
	EntryID string        `json:"entry_id,omitempty"`
	BufOvfl bool          `json:"buf_ovfl,omitempty"`
	Entries []ReportEntry `json:"entries"`
}

// ReportEntry is one included data set member.
type ReportEntry struct {
	// Ref is the member's FC-qualified reference.
	Ref string `json:"ref"`
	// Reasons are the inclusion reasons, when reported.
	Reasons []string `json:"reasons,omitempty"`
	Value   Value    `json:"value"`
}

// CreateDataSetRequest creates a persistent data set in a logical node of
// the IED.
type CreateDataSetRequest struct {
	// Ref names the data set, such as "LD/LLN0.Explorer1".
	Ref string `json:"ref"`
	// Members are FC-qualified references of data objects or attributes.
	Members []string `json:"members"`
}

// Limits of a created data set.
const (
	MaxDataSetMembers = 100
	MaxDataSetNameLen = 32
)

// SelectSGRequest selects the active setting group of a logical device.
type SelectSGRequest struct {
	LD string `json:"ld"`
	SG uint8  `json:"sg"`
}
