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
const APIVersion = "0.5"

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
	// Mode is the mode a session starts in; Commission mode is granted per
	// session (FeatureCommission).
	Mode string `json:"mode"`
	// Time is the engine's clock, for clients to notice a skew.
	Time time.Time `json:"time"`
	// Features lists optional capabilities, see the Feature constants.
	Features []string `json:"features"`
	// Quantities are the measured quantities a search can ask for
	// (SearchQuery.Quantity).
	Quantities []string `json:"quantities"`
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
	// Addressing overrides the ISO addressing; nil sends the defaults.
	Addressing *Addressing `json:"addressing,omitempty"`
	// Proxy is the proxy to reach the IED through: "" uses the default
	// proxy (Settings.DefaultProxy), ProxyDirect connects without a proxy
	// even when a default is set, anything else is a proxy URL
	// (socks5://, socks5h:// or http://, with an optional user:password).
	// A session's target holds it with the password replaced.
	Proxy string `json:"proxy,omitempty"`
}

// ProxyDirect is the value of TargetConfig.Proxy and Settings.DefaultProxy
// that asks for no proxy.
const ProxyDirect = "direct"

// MaxProxyLen bounds a proxy setting.
const MaxProxyLen = 1024

// Settings are the engine's settings.
type Settings struct {
	// DefaultProxy is the proxy for a target that names none: "" or
	// ProxyDirect for none, else a proxy URL.
	DefaultProxy string `json:"default_proxy"`
	// DefaultProxyLocked is set when the daemon's operator chose the default
	// proxy (--proxy or ALL_PROXY): DefaultProxy is then shown with the
	// password replaced and cannot be changed through the Service.
	DefaultProxyLocked bool `json:"default_proxy_locked"`
}

// Addressing is the ISO addressing of an association: the values of the
// IED's SCL Address (OSI-TSEL, OSI-SSEL, OSI-PSEL, OSI-AP-Title,
// OSI-AE-Qualifier). Each field left out (nil) takes the default go-mms
// sends, the values libiec61850 sends: selectors 0001 (T, S) and 00000001
// (P), AP-titles 1.1.1.999 (calling) and 1.1.1.999.1 (called), AE-qualifier
// 12. A selector or AP-title set to "" is omitted from the request.
// Selectors are hexadecimal; AP-titles are object identifiers written
// 1.3.9999.23 or 1,3,9999,23.
type Addressing struct {
	LocalTSelector    *string `json:"local_tsel,omitempty"`
	RemoteTSelector   *string `json:"remote_tsel,omitempty"`
	LocalSSelector    *string `json:"local_ssel,omitempty"`
	RemoteSSelector   *string `json:"remote_ssel,omitempty"`
	LocalPSelector    *string `json:"local_psel,omitempty"`
	RemotePSelector   *string `json:"remote_psel,omitempty"`
	LocalAPTitle      *string `json:"local_ap_title,omitempty"`
	RemoteAPTitle     *string `json:"remote_ap_title,omitempty"`
	LocalAEQualifier  *int    `json:"local_ae_qualifier,omitempty"`
	RemoteAEQualifier *int    `json:"remote_ae_qualifier,omitempty"`
}

// Profile is a saved connection.
type Profile struct {
	// ID is assigned by the engine; empty in a request creates a profile.
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Target TargetConfig `json:"target"`
	// Notes is free text, for example where the IED is.
	Notes     string    `json:"notes,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Limits of profiles.
const (
	MaxProfiles        = 200
	MaxProfileNameLen  = 80
	MaxProfileNotesLen = 1000
)

// Features of an engine.
const (
	// FeatureProfiles: the engine stores connection profiles.
	FeatureProfiles = "profiles"
)

// Session states.
const (
	SessionConnected = "connected"
	// SessionOffline is a model imported from SCL; nothing to read.
	SessionOffline = "offline"
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
	// Polling is set while the session polls.
	Polling *PollState `json:"polling,omitempty"`
	// SCL is set for an offline session: a model imported from an SCL
	// file, with no IED behind it.
	SCL *SCLOrigin `json:"scl,omitempty"`
	// Grant is set while the session holds a Commission grant; Mode is
	// then ModeCommission.
	Grant *Grant `json:"grant,omitempty"`
	// Snapshot is set for an offline session opened from a snapshot.
	Snapshot *SnapshotOrigin `json:"snapshot,omitempty"`
	// Proxy is the proxy the session connected through, with the password
	// replaced; empty for a direct connection or an offline session.
	Proxy string `json:"proxy,omitempty"`
	// Kind is live, scl, snapshot or trace (SessionKind*).
	Kind string `json:"kind"`
	// Trace is set for a trace session: a capture analysed offline. It
	// has no IED model of its own; its State is offline.
	Trace *TraceOrigin `json:"trace,omitempty"`
}

// SCLOrigin names where an offline session's model came from.
type SCLOrigin struct {
	File        string `json:"file"`
	IED         string `json:"ied"`
	AccessPoint string `json:"access_point"`
}

// MaxSCLBytes bounds an SCL file the engine parses.
const MaxSCLBytes = 64 << 20

// File is a document handed to the engine: an SCL file or a snapshot. The
// engine never reads a path: the user interface reads the file the user
// chose.
type File struct {
	// Name is the file name, for display.
	Name string `json:"name"`
	Data []byte `json:"data"`
}

// SCLInfo describes an SCL document.
type SCLInfo struct {
	Name string `json:"name"`
	// Kind is scd, cid, icd, iid, ssd or unknown.
	Kind    string   `json:"kind"`
	Version string   `json:"version"`
	IEDs    []SCLIED `json:"ieds"`
	// Diagnostics are the parser's findings, at most MaxSCLDiagnostics.
	Diagnostics []SCLDiagnostic `json:"diagnostics"`
	// DiagnosticsDropped counts findings beyond the ones listed.
	DiagnosticsDropped int `json:"diagnostics_dropped,omitempty"`
}

// MaxSCLDiagnostics bounds SCLInfo.Diagnostics.
const MaxSCLDiagnostics = 100

// SCLIED is an IED of an SCL document.
type SCLIED struct {
	Name         string           `json:"name"`
	Desc         string           `json:"desc,omitempty"`
	Manufacturer string           `json:"manufacturer,omitempty"`
	Type         string           `json:"type,omitempty"`
	AccessPoints []SCLAccessPoint `json:"access_points"`
}

// SCLAccessPoint is an access point with an MMS server.
type SCLAccessPoint struct {
	Name           string `json:"name"`
	LogicalDevices int    `json:"logical_devices"`
	// Target is the address the Communication section gives it, ready to
	// connect with; nil when the SCL has no IP address for it.
	Target *TargetConfig `json:"target,omitempty"`
}

// SCLDiagnostic is a finding of the SCL parser.
type SCLDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// OpenSCLRequest opens an offline session on one IED of an SCL document.
// IED and AccessPoint may be empty when the document has only one.
type OpenSCLRequest struct {
	File        File   `json:"file"`
	IED         string `json:"ied,omitempty"`
	AccessPoint string `json:"access_point,omitempty"`
}

// Polling limits.
const (
	MinPollIntervalMS     = 200
	MaxPollIntervalMS     = 3_600_000
	DefaultPollIntervalMS = 1000
)

// PollConfig starts polling: the references are read every interval, start
// to start. A cycle that is still running when the next is due skips it.
type PollConfig struct {
	Refs       []string `json:"refs"`
	IntervalMS int      `json:"interval_ms,omitempty"`
}

// PollState describes a running poller.
type PollState struct {
	Refs       []string  `json:"refs"`
	IntervalMS int       `json:"interval_ms"`
	StartedAt  time.Time `json:"started_at"`
	Samples    uint64    `json:"samples"`
	// Failures counts cycles in which at least one reference failed.
	Failures uint64 `json:"failures"`
	// Overruns counts skipped cycles.
	Overruns uint64 `json:"overruns"`
}

// Sample is one poll cycle.
type Sample struct {
	SessionID string `json:"session_id"`
	// Seq numbers the samples of a poller, from 1.
	Seq        uint64      `json:"seq"`
	At         time.Time   `json:"at"`
	DurationMS int64       `json:"duration_ms"`
	Values     []ReadValue `json:"values"`
	Overruns   uint64      `json:"overruns"`
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

// SearchQuery searches the discovered model of a session. Every field that
// is set must match.
type SearchQuery struct {
	// Text matches the reference, case-insensitively, as a substring.
	Text string `json:"text,omitempty"`
	// LNClass matches the class of the logical node the node is in.
	LNClass string `json:"ln_class,omitempty"`
	// FC matches one of the node's functional constraints.
	FC string `json:"fc,omitempty"`
	// Kind matches the node kind.
	Kind string `json:"kind,omitempty"`
	// Quantity matches data objects that measure this quantity by the
	// namespace's semantic rules, such as "voltage" or "frequency"
	// (go-iec61850-model semantic.Quantities).
	Quantity string `json:"quantity,omitempty"`
	// Limit bounds the hits; zero is MaxSearchHits.
	Limit int `json:"limit,omitempty"`
}

// MaxSearchHits bounds the hits of one search.
const MaxSearchHits = 500

// SearchResult is the answer to a SearchQuery.
type SearchResult struct {
	Hits []Node `json:"hits"`
	// Truncated says that more nodes matched than Limit.
	Truncated bool `json:"truncated"`
}

// Sources of read values.
const (
	SourceIED      = "ied"
	SourceSnapshot = "snapshot"
)

// SnapshotSchema identifies the snapshot format.
const SnapshotSchema = "otfabric.iec61850-explorer.snapshot/1"

// MaxSnapshotBytes bounds a snapshot the engine opens.
const MaxSnapshotBytes = 64 << 20

// Snapshot is a session's model and values at one moment, as a file.
type Snapshot struct {
	Schema     string           `json:"schema"`
	App        AppInfo          `json:"app"`
	CapturedAt time.Time        `json:"captured_at"`
	Target     TargetConfig     `json:"target"`
	Identity   *Identity        `json:"identity,omitempty"`
	Discovery  DiscoverySummary `json:"discovery"`
	SCL        *SCLOrigin       `json:"scl,omitempty"`
	// Nodes is the whole model tree, depth first.
	Nodes []Node `json:"nodes"`
	// Values were read at ValuesAt; empty for an offline session.
	Values   []ReadValue `json:"values"`
	ValuesAt time.Time   `json:"values_at,omitempty"`
}

// AppInfo names the program and libraries that wrote a file.
type AppInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	GoIEC61850 string `json:"go_iec61850"`
	GoMMS      string `json:"go_mms"`
}

// Export formats.
const (
	ExportSnapshot = "snapshot"
	ExportCSV      = "csv"
)

// ExportRequest reads the references once (none for an offline session) and
// renders the session.
type ExportRequest struct {
	Format string   `json:"format"`
	Refs   []string `json:"refs"`
}

// ExportFile is a rendered export; the user interface saves it.
type ExportFile struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Text string `json:"text"`
}

// SnapshotOrigin names the snapshot an offline session was opened from.
type SnapshotOrigin struct {
	File       string    `json:"file"`
	CapturedAt time.Time `json:"captured_at"`
}

// ReadResult is the answer to a ReadRequest, one value per reference in the
// order of the request.
type ReadResult struct {
	// Source is "ied" for values read now, "snapshot" for the values a
	// snapshot holds.
	Source     string      `json:"source"`
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
	// Action is "connect", "discover", "read", "close"; the inspections
	// "datasets", "reports", "files", "download", "journals", "journal",
	// "setting-groups"; and in Commission mode "grant", "revoke", "write",
	// "subscribe", "unsubscribe", "gi", "create-dataset", "delete-dataset",
	// "select-active-sg". A
	// read of several references is one operation.
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
	// Wire numbers the operation for Service.Wire, which returns the MMS
	// units it sent and received; 0 when none were recorded (an offline
	// session).
	Wire uint64 `json:"wire,omitempty"`
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
	// EventSample carries a poll Sample.
	EventSample = "sample"
	// EventResync tells a subscriber that it missed events.
	EventResync = "resync"
	// EventAudit carries an AuditRecord.
	EventAudit = "audit"
	// EventReport carries a Report of a subscription.
	EventReport = "report"
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
	Sample    *Sample      `json:"sample,omitempty"`
	Audit     *AuditRecord `json:"audit,omitempty"`
	Report    *Report      `json:"report,omitempty"`
}
