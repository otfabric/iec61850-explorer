// SPDX-License-Identifier: MIT

// Package model holds the JSON data transfer objects of the Explorer: what the
// Service interface returns and what the HTTP API carries. The types are
// independent of go-iec61850 and go-mms so that the wire format stays stable
// when the libraries change.
//
// All timestamps are UTC.
package model

import "time"

// AppName is the application name.
const AppName = "iec61850-explorer"

// APIVersion is the version of the Service contract and of the HTTP API. The
// major part changes on incompatible changes, the minor part on additions.
const APIVersion = "0.1"

// DefaultPort is the MMS port (ISO-on-TCP, RFC 1006).
const DefaultPort = 102

// Operating modes. Only Observe exists in M0; the others are named so that
// clients can show what an engine permits.
const (
	// ModeObserve permits discovery and reads only. The MMS client is
	// created with an allow-list of read services (internal/readonly).
	ModeObserve = "observe"
)

// Health describes the engine.
type Health struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	UptimeS    int64  `json:"uptime_s"`
	GoIEC61850 string `json:"go_iec61850"`
	GoMMS      string `json:"go_mms"`
	// Mode is the operating mode of every session of this engine.
	Mode string `json:"mode"`
	// Time is the engine's clock, for clients to notice a skew.
	Time time.Time `json:"time"`
	// Features lists optional capabilities. None exist in M0; the field is
	// there so that clients need no change when they do.
	Features []string `json:"features"`
}

// TargetConfig names an IED.
type TargetConfig struct {
	// Host is an IP address or host name.
	Host string `json:"host"`
	// Port defaults to 102.
	Port int `json:"port,omitempty"`
	// TimeoutMS bounds connecting and every request. Zero selects the
	// engine default (10 s); at most 120000.
	TimeoutMS int `json:"timeout_ms,omitempty"`
}

// Session states.
const (
	SessionConnected = "connected"
	// SessionLost means the association ended without the user closing it.
	SessionLost   = "lost"
	SessionClosed = "closed"
)

// Identity is the answer to the MMS Identify service.
type Identity struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	Revision string `json:"revision"`
}

// SessionInfo describes an open session.
type SessionInfo struct {
	ID       string       `json:"id"`
	Target   TargetConfig `json:"target"`
	State    string       `json:"state"`
	Mode     string       `json:"mode"`
	OpenedAt time.Time    `json:"opened_at"`
	// Identity is nil when the IED does not support Identify.
	Identity *Identity `json:"identity,omitempty"`
	// Discovery summarises the model the IED exposes.
	Discovery DiscoverySummary `json:"discovery"`
	// Error is set for a lost session.
	Error *OpError `json:"error,omitempty"`
}

// DiscoverySummary counts the discovered model.
type DiscoverySummary struct {
	LogicalDevices int `json:"logical_devices"`
	LogicalNodes   int `json:"logical_nodes"`
	DataObjects    int `json:"data_objects"`
	// Nodes counts every node below the logical nodes, data objects
	// included.
	Nodes      int   `json:"nodes"`
	DurationMS int64 `json:"duration_ms"`
}

// Node kinds. MMS variable names do not say whether a component below a data
// object is a sub-data object or a data attribute, so both are "child".
const (
	NodeLD    = "ld"
	NodeLN    = "ln"
	NodeDO    = "do"
	NodeChild = "child"
)

// Node is one entry of the device tree.
type Node struct {
	Name string `json:"name"`
	// Ref is the object reference without functional constraint, such as
	// "IED1LD0/MMXU1.Hz.mag".
	Ref  string `json:"ref"`
	Kind string `json:"kind"`
	// LNClass is the logical node class for a logical node ("MMXU" of
	// "MMXU1"), without prefix and instance; empty for other kinds.
	LNClass string `json:"ln_class,omitempty"`
	// FCs are the functional constraints under which the node exists.
	// A read needs one of them.
	FCs []string `json:"fcs"`
	// Children is the number of child nodes.
	Children int `json:"children"`
}

// ReadRequest reads values. Each reference must carry a functional
// constraint: "IED1LD0/MMXU1.Hz[MX]".
type ReadRequest struct {
	Refs []string `json:"refs"`
}

// MaxReadRefs bounds the references of one ReadRequest.
const MaxReadRefs = 200

// ReadResult is the answer to a ReadRequest, one value per reference in the
// order of the request.
type ReadResult struct {
	At         time.Time   `json:"at"`
	DurationMS int64       `json:"duration_ms"`
	Values     []ReadValue `json:"values"`
}

// ReadValue is the result of one reference.
type ReadValue struct {
	Ref string `json:"ref"`
	// MMS is the variable that was read, as it appears on the wire.
	MMS MMSName `json:"mms"`
	// Value is nil when Error is set.
	Value *Value   `json:"value,omitempty"`
	Error *OpError `json:"error,omitempty"`
}

// MMSName is a domain-specific MMS variable name.
type MMSName struct {
	Domain string `json:"domain"`
	ItemID string `json:"item_id"`
}

// Value types, the MMS Data CHOICE names.
const (
	TypeBoolean     = "boolean"
	TypeInteger     = "integer"
	TypeUnsigned    = "unsigned"
	TypeFloat       = "float"
	TypeBitString   = "bit-string"
	TypeOctetString = "octet-string"
	TypeVisible     = "visible-string"
	TypeMMSString   = "mms-string"
	TypeUTCTime     = "utc-time"
	TypeBinaryTime  = "binary-time"
	TypeStructure   = "structure"
	TypeArray       = "array"
	TypeAccessError = "data-access-error"
	TypeOther       = "other"
)

// Value is a decoded MMS value.
type Value struct {
	// Name is the component name inside a structure, empty at the top and
	// for array elements.
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
	// Text is the value as it is shown.
	Text string `json:"text"`
	// JSON is the value as a JSON scalar: a bool, a number (integers that do
	// not fit a float64 exactly as strings), a string. Nil for structures,
	// arrays, NaN and infinities.
	JSON any `json:"json,omitempty"`
	// Bits is the length of a bit string.
	Bits int `json:"bits,omitempty"`
	// Quality decodes a 13-bit bit string named "q".
	Quality *Quality `json:"quality,omitempty"`
	// Time decodes a UTC time.
	Time *Timestamp `json:"time,omitempty"`
	// Children are the components of a structure or the elements of an
	// array.
	Children []Value `json:"children,omitempty"`
}

// Quality is an IEC 61850-7-3 quality.
type Quality struct {
	// Validity is good, invalid, reserved or questionable.
	Validity string `json:"validity"`
	// Flags lists the detail, source, test and operator-blocked flags that
	// are set.
	Flags []string `json:"flags"`
	Raw   uint16   `json:"raw"`
}

// Timestamp is an IEC 61850 UTC time with its time quality.
type Timestamp struct {
	Time                 time.Time `json:"time"`
	LeapSecondKnown      bool      `json:"leap_second_known"`
	ClockFailure         bool      `json:"clock_failure"`
	ClockNotSynchronized bool      `json:"clock_not_synchronized"`
	// Accuracy is the number of significant bits of the fraction of a
	// second; 0 is unspecified.
	Accuracy int `json:"accuracy"`
}

// OpError is a failed operation, as the user sees it.
type OpError struct {
	// Code is a stable identifier, see the Err* constants.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Stage is where the operation stopped: "tcp", "iso" (COTP, session,
	// presentation, ACSE or MMS Initiate), "request" or "response". Empty
	// when the library does not tell.
	Stage string `json:"stage,omitempty"`
	// Hint says what to check. Only set when the evidence supports it.
	Hint string `json:"hint,omitempty"`
}

// Error codes of OpError.
const (
	ErrCodeConnect        = "connect_failed"
	ErrCodeAssociation    = "association_failed"
	ErrCodeTimeout        = "timeout"
	ErrCodeConnectionLost = "connection_lost"
	ErrCodeAccessDenied   = "access_denied"
	ErrCodeNotFound       = "object_not_found"
	ErrCodeServiceError   = "service_error"
	ErrCodeRejected       = "rejected"
	ErrCodeNotAllowed     = "service_not_allowed"
	ErrCodeDecode         = "decode_failed"
	ErrCodeInvalidRef     = "invalid_reference"
	ErrCodeOther          = "error"
)

// Operation records one user action on a session and the MMS requests it
// caused, in order. It is the Explorer's own activity log.
type Operation struct {
	// Action is "connect", "discover", "read" or "close".
	Action string `json:"action"`
	// Target is the object reference or address the action was about.
	Target     string       `json:"target,omitempty"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMS int64        `json:"duration_ms"`
	OK         bool         `json:"ok"`
	Error      *OpError     `json:"error,omitempty"`
	Requests   []MMSRequest `json:"requests"`
	// RequestsDropped counts requests beyond the ones listed.
	RequestsDropped int `json:"requests_dropped,omitempty"`
}

// MaxOperationRequests bounds Operation.Requests.
const MaxOperationRequests = 64

// MMSRequest is one confirmed MMS request.
type MMSRequest struct {
	Service    string  `json:"service"`
	InvokeID   uint32  `json:"invoke_id"`
	DurationMS float64 `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

// Event kinds.
const (
	// EventSession carries a SessionInfo whose state changed.
	EventSession = "session"
	// EventOperation carries an Operation.
	EventOperation = "operation"
	// EventResync tells a subscriber that it missed events.
	EventResync = "resync"
)

// Event is one entry of the event stream.
type Event struct {
	// Seq increases by one per event of an engine.
	Seq       uint64       `json:"seq"`
	Kind      string       `json:"kind"`
	SessionID string       `json:"session_id,omitempty"`
	At        time.Time    `json:"at"`
	Session   *SessionInfo `json:"session,omitempty"`
	Operation *Operation   `json:"operation,omitempty"`
}
