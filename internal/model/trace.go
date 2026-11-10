// SPDX-License-Identifier: MIT

package model

import "time"

// Limits of trace analysis (M4).
const (
	// MaxTraceBytes bounds a capture the engine analyses (the file as the
	// user chose it, compressed or not).
	MaxTraceBytes = 256 << 20
	// MaxTraceSessions bounds the trace sessions open at once: a result
	// stays in memory while its session is open.
	MaxTraceSessions = 4
	// MaxTracePorts bounds the MMS ports of OpenTraceRequest.
	MaxTracePorts = 16
	// DefaultTracePage and MaxTracePage bound a page of a timeline, of
	// reports and of findings.
	DefaultTracePage = 200
	MaxTracePage     = 1000
	// MaxTraceObjects bounds the object names listed per PDU.
	MaxTraceObjects = 64
	// MaxTraceValues bounds the values shown per PDU.
	MaxTraceValues = 64
)

// OpenTraceRequest opens a capture (pcap or pcapng, plain or gzip) as a
// trace session.
type OpenTraceRequest struct {
	File File `json:"file"`
	// Ports are the TCP server ports of MMS; empty means 102.
	Ports []int `json:"ports,omitempty"`
}

// Session kinds of SessionInfo.Kind.
const (
	SessionKindLive     = "live"
	SessionKindSCL      = "scl"
	SessionKindSnapshot = "snapshot"
	SessionKindTrace    = "trace"
)

// TraceOrigin describes the capture of a trace session.
type TraceOrigin struct {
	File    string `json:"file"`
	Format  string `json:"format"`
	Gzip    bool   `json:"gzip,omitempty"`
	Packets uint64 `json:"packets"`
	SHA256  string `json:"sha256"`
	Ports   []int  `json:"ports"`
	// Damaged is set when damage ended the read: the session holds what
	// was read before it (a finding says where).
	Damaged bool `json:"damaged,omitempty"`
	// Link is the model session the capture is correlated with, nil
	// without one.
	Link *TraceLink `json:"link,omitempty"`
}

// TraceLink is the session a trace is correlated with.
type TraceLink struct {
	SessionID string `json:"session_id"`
	// Kind is SessionKindLive, SessionKindSCL or SessionKindSnapshot.
	Kind     string    `json:"kind"`
	Label    string    `json:"label"`
	LinkedAt time.Time `json:"linked_at"`
	// DataSets, RCBs and GoCBs count the definitions the link knows.
	DataSets int `json:"data_sets"`
	RCBs     int `json:"rcbs"`
	GoCBs    int `json:"gocbs"`
	// Notes say what the linked session cannot tell (a snapshot holds no
	// data set definitions, an SCL session's control blocks come from the
	// file, ...).
	Notes []string `json:"notes"`
}

// LinkTraceRequest links a trace session to a model session; an empty
// Session removes the link.
type LinkTraceRequest struct {
	Session string `json:"session"`
}

// TraceSummary is what a capture holds.
type TraceSummary struct {
	File         TraceFile      `json:"file"`
	Counters     TraceCounters  `json:"counters"`
	Associations int            `json:"associations"`
	Transactions int            `json:"transactions"`
	ByStatus     map[string]int `json:"by_status"`
	ByService    map[string]int `json:"by_service"`
	Reports      int            `json:"reports"`
	GOOSE        int            `json:"goose"`
	SV           int            `json:"sv"`
	// Findings counts the findings by severity.
	Findings map[string]int `json:"findings"`
	Link     *TraceLink     `json:"link,omitempty"`
	// Engine names the analyser, "go-mms-trace vX.Y.Z".
	Engine string `json:"engine"`
}

// TraceFile describes the capture file.
type TraceFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Gzip   bool   `json:"gzip,omitempty"`
	// SHA256 and Bytes cover the input as read (compressed for gzip).
	SHA256        string     `json:"sha256"`
	Bytes         int64      `json:"bytes"`
	Packets       uint64     `json:"packets"`
	Interfaces    int        `json:"interfaces"`
	Links         []string   `json:"links"`
	TruncatedTail bool       `json:"truncated_tail,omitempty"`
	SkippedBlocks uint64     `json:"skipped_blocks,omitempty"`
	First         *time.Time `json:"first,omitempty"`
	Last          *time.Time `json:"last,omitempty"`
	// DurationMS is the time from the first to the last packet.
	DurationMS float64 `json:"duration_ms"`
	// Damaged is set when damage ended the read; Damage says why.
	Damaged bool   `json:"damaged,omitempty"`
	Damage  string `json:"damage,omitempty"`
	Ports   []int  `json:"ports"`
}

// TraceCounters are what the analyser could not use (go-mms-trace
// Counters).
type TraceCounters struct {
	Unsupported map[string]uint64 `json:"unsupported"`
	TPKTResync  uint64            `json:"tpkt_resync"`
	COTPErrors  uint64            `json:"cotp_errors"`
	TSDUErrors  uint64            `json:"tsdu_errors"`
	BareMMS     uint64            `json:"bare_mms"`
	TSOSegments uint64            `json:"tso_segments"`
	GOOSEErrors uint64            `json:"goose_errors"`
	SVErrors    uint64            `json:"sv_errors"`
}

// TraceAssociation is one MMS association of a capture (one TCP
// connection).
type TraceAssociation struct {
	ID     int    `json:"id"`
	Client string `json:"client"`
	Server string `json:"server"`
	// CallingTSAP and CalledTSAP are the COTP CR's selectors in hex; empty
	// without a CR.
	CallingTSAP string `json:"calling_tsap,omitempty"`
	CalledTSAP  string `json:"called_tsap,omitempty"`
	// Open is associate, connect or midstream; Close is conclude,
	// release, abort, disconnect, reset or empty when the end was not
	// captured.
	Open  string `json:"open"`
	Close string `json:"close"`
	// Accepted is the AARE result; nil without an AARE.
	Accepted *bool `json:"accepted,omitempty"`
	// Authentication is the AARQ's mechanism (none, password, other);
	// credentials are never decoded.
	Authentication string         `json:"authentication,omitempty"`
	Initiate       *TraceInitiate `json:"initiate,omitempty"`
	First          *time.Time     `json:"first,omitempty"`
	Last           *time.Time     `json:"last,omitempty"`
	Transactions   int            `json:"transactions"`
	Reports        int            `json:"reports"`
	Unanswered     int            `json:"unanswered"`
	// Gaps counts skipped ranges of the TCP byte stream.
	Gaps int `json:"gaps"`
}

// TraceInitiate is the MMS Initiate response of an association.
type TraceInitiate struct {
	LocalDetail                  int `json:"local_detail"`
	MaxServicesOutstandingCall   int `json:"max_serv_outstanding_calling"`
	MaxServicesOutstandingCalled int `json:"max_serv_outstanding_called"`
	DataStructureNesting         int `json:"data_structure_nesting"`
	Version                      int `json:"version"`
}

// Transaction statuses (go-mms-trace).
const (
	TraceStatusOK         = "ok"
	TraceStatusError      = "error"
	TraceStatusReject     = "reject"
	TraceStatusUnanswered = "unanswered"
	TraceStatusOrphan     = "orphan"
	TraceStatusPending    = "pending"
)

// TimelineQuery selects transactions of a trace. Every field that is set
// must match.
type TimelineQuery struct {
	// Association is an association ID; 0 matches all.
	Association int `json:"association,omitempty"`
	// Service matches the service name, case-insensitively.
	Service string `json:"service,omitempty"`
	// Status lists the statuses to keep; empty keeps all.
	Status []string `json:"status,omitempty"`
	// From and To bound the request time (or the response time of an
	// orphan), inclusive.
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
	// Offset and Limit page the result; Limit 0 is DefaultTracePage.
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

// Timeline is a page of transactions in capture order.
type Timeline struct {
	// Total counts the transactions that match.
	Total        int                `json:"total"`
	Offset       int                `json:"offset"`
	Transactions []TraceTransaction `json:"transactions"`
}

// TraceTransaction is one confirmed service: request and response paired
// by invoke ID.
type TraceTransaction struct {
	// Index numbers the capture's transactions from 1, in capture order.
	Index         int        `json:"index"`
	Association   int        `json:"association"`
	InvokeID      uint32     `json:"invoke_id"`
	Service       string     `json:"service"`
	Status        string     `json:"status"`
	ServerRequest bool       `json:"server_request,omitempty"`
	RequestAt     *time.Time `json:"request_at,omitempty"`
	ResponseAt    *time.Time `json:"response_at,omitempty"`
	// LatencyMS is nil when either side was not captured.
	LatencyMS     *float64 `json:"latency_ms,omitempty"`
	RequestFrame  uint64   `json:"request_frame,omitempty"`
	ResponseFrame uint64   `json:"response_frame,omitempty"`
	// Objects are what the request names, mapped to IEC 61850 references
	// and, with a link, resolved against the model.
	Objects []TraceObject `json:"objects"`
	// Error is the confirmed error, reject or decode error, as text.
	Error    string   `json:"error,omitempty"`
	Request  *WirePDU `json:"request,omitempty"`
	Response *WirePDU `json:"response,omitempty"`
}

// Model resolutions of a TraceObject.
const (
	// ResolveFound: the linked model has the object.
	ResolveFound = "found"
	// ResolveMissing: the linked model does not have it.
	ResolveMissing = "missing"
	// ResolveNotIEC61850: the MMS name does not follow the IEC 61850
	// mapping (IEC 61850-8-1), so there is nothing to look up.
	ResolveNotIEC61850 = "not-iec61850"
	// ResolveUnknown: the linked session holds no definitions of this kind
	// (a snapshot has no data sets or control blocks; an IED's model may
	// list no GOOSE control blocks).
	ResolveUnknown = "unknown"
)

// TraceObject is an MMS object a PDU names.
type TraceObject struct {
	// MMS is the name on the wire; Scope is "domain", "vmd" or "aa".
	MMS   MMSName `json:"mms"`
	Scope string  `json:"scope"`
	// List says the name is a named variable list (a data set).
	List bool `json:"list,omitempty"`
	// Ref is the IEC 61850 reference ("LD/LLN0.Mod.stVal[ST]", a data set
	// "LD/LLN0.Meas"); empty when the name does not follow the mapping.
	Ref string `json:"ref,omitempty"`
	// Kind is what the reference names: ld, ln, do, da, data-set, rcb,
	// gocb, sgcb, log, or "" when unknown.
	Kind string `json:"kind,omitempty"`
	// Model is the resolution against the linked session (Resolve*); empty
	// without a link.
	Model string `json:"model,omitempty"`
	// Concerns names the data object, data sets and control blocks of the
	// linked model the object belongs to.
	Concerns []string `json:"concerns,omitempty"`
}

// WirePDU is a decoded MMS PDU (go-mms CapturedPDU).
type WirePDU struct {
	// Kind is the PDU kind: confirmed-request, confirmed-response,
	// confirmed-error, unconfirmed, reject, initiate-request, ...
	Kind     string        `json:"kind"`
	InvokeID *uint32       `json:"invoke_id,omitempty"`
	Service  string        `json:"service,omitempty"`
	Objects  []TraceObject `json:"objects,omitempty"`
	// ObjectsDropped counts names beyond MaxTraceObjects.
	ObjectsDropped int `json:"objects_dropped,omitempty"`
	// Values are a Read response's, Write request's or report's values,
	// at most MaxTraceValues.
	Values        []Value `json:"values,omitempty"`
	ValuesDropped int     `json:"values_dropped,omitempty"`
	// WriteResults are a Write response's results ("ok" or the error).
	WriteResults []string `json:"write_results,omitempty"`
	// NameList is a GetNameList response's names (at most MaxTraceObjects).
	NameList     []string  `json:"name_list,omitempty"`
	NameListMore bool      `json:"name_list_more,omitempty"`
	Identity     *Identity `json:"identity,omitempty"`
	// Initiate holds the parameters of an initiate PDU.
	Initiate *TraceInitiate `json:"initiate,omitempty"`
	// Error is a confirmed error ("class 2 code 1") or reject.
	Error string `json:"error,omitempty"`
	// Raw is the hex service content of a service go-mms does not model
	// (at most 256 bytes shown), DecodeError why a modelled one did not
	// decode.
	Raw         string `json:"raw,omitempty"`
	DecodeError string `json:"decode_error,omitempty"`
}

// TraceReportsQuery selects the reports of a trace.
type TraceReportsQuery struct {
	Association int `json:"association,omitempty"`
	Offset      int `json:"offset,omitempty"`
	Limit       int `json:"limit,omitempty"`
}

// TraceReports is a page of a capture's InformationReports.
type TraceReports struct {
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
	Reports []TraceReport `json:"reports"`
}

// TraceReport is one captured InformationReport.
type TraceReport struct {
	Index       int        `json:"index"`
	Association int        `json:"association"`
	At          *time.Time `json:"at,omitempty"`
	Frame       uint64     `json:"frame"`
	// ListName is the variable list it reports ("RPT" for an IEC 61850
	// report); Objects the variables when it names them instead.
	ListName string        `json:"list_name,omitempty"`
	Objects  []TraceObject `json:"objects,omitempty"`
	// RptID is the first value of an IEC 61850 report.
	RptID string `json:"rpt_id,omitempty"`
	// RCB and DatSet are the report control block and data set of the
	// linked model the report was matched to.
	RCB    string `json:"rcb,omitempty"`
	DatSet string `json:"dat_set,omitempty"`
	// Decoded is set when the entries were named by the data set's
	// members; otherwise Values holds the values as captured.
	Decoded bool          `json:"decoded"`
	SeqNum  *uint32       `json:"seq_num,omitempty"`
	ConfRev *uint32       `json:"conf_rev,omitempty"`
	Entries []ReportEntry `json:"entries,omitempty"`
	Values  []Value       `json:"values,omitempty"`
	// Note says why the report was not decoded.
	Note        string `json:"note,omitempty"`
	DecodeError string `json:"decode_error,omitempty"`
}

// TraceStreams are the GOOSE and Sampled Values streams of a capture.
type TraceStreams struct {
	GOOSE []TraceGoose `json:"goose"`
	SV    []TraceSV    `json:"sv"`
}

// TraceGoose is the GOOSE stream of one publisher.
type TraceGoose struct {
	Src          string         `json:"src"`
	Dst          string         `json:"dst"`
	AppID        uint16         `json:"app_id"`
	VLAN         int            `json:"vlan"`
	GocbRef      string         `json:"gocb_ref"`
	DatSet       string         `json:"dat_set"`
	GoID         string         `json:"go_id"`
	ConfRev      uint32         `json:"conf_rev"`
	Messages     int            `json:"messages"`
	Changes      int            `json:"changes"`
	First        *time.Time     `json:"first,omitempty"`
	Last         *time.Time     `json:"last,omitempty"`
	Simulation   int            `json:"simulation"`
	NdsCom       int            `json:"nds_com"`
	Issues       map[string]int `json:"issues"`
	MaxPerSecond int            `json:"max_per_second"`
	// Model is the GoCB's resolution against the linked model (Resolve*,
	// or "unknown" when the model defines no GOOSE control blocks); Note
	// explains it.
	Model string `json:"model,omitempty"`
	Note  string `json:"note,omitempty"`
}

// TraceSV is the Sampled Values stream of one publisher.
type TraceSV struct {
	Src      string     `json:"src"`
	Dst      string     `json:"dst"`
	AppID    uint16     `json:"app_id"`
	VLAN     int        `json:"vlan"`
	SvID     string     `json:"sv_id"`
	DatSet   string     `json:"dat_set"`
	ConfRev  uint32     `json:"conf_rev"`
	Messages int        `json:"messages"`
	ASDUs    int        `json:"asdus"`
	Lost     int        `json:"lost"`
	Repeated int        `json:"repeated"`
	First    *time.Time `json:"first,omitempty"`
	Last     *time.Time `json:"last,omitempty"`
	Wrap     int        `json:"wrap"`
}

// TraceFinding is a gap or anomaly of a capture, with where it is and the
// counter or status it comes from.
type TraceFinding struct {
	// Severity is SeverityError, SeverityWarning or SeverityInfo.
	Severity string `json:"severity"`
	// Kind is a stable identifier, such as tcp-gap, unanswered,
	// not-in-model, goose-sqnum-gap.
	Kind    string `json:"kind"`
	Message string `json:"message"`
	// Association, Frame, At, Transaction and Stream locate it; each is
	// empty when it does not apply or is not known.
	Association int        `json:"association,omitempty"`
	Frame       uint64     `json:"frame,omitempty"`
	At          *time.Time `json:"at,omitempty"`
	Transaction int        `json:"transaction,omitempty"`
	Stream      string     `json:"stream,omitempty"`
	// Source is the go-mms-trace field, counter or status it is read from,
	// such as "Association.Gaps" or "Transaction.Status=unanswered".
	Source string `json:"source"`
	// Count is how often it happened, for counters.
	Count uint64 `json:"count,omitempty"`
}

// FindingsQuery selects findings.
type FindingsQuery struct {
	Severity string `json:"severity,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// TraceFindings is a page of findings: the capture's, then the link's.
type TraceFindings struct {
	Total      int            `json:"total"`
	Offset     int            `json:"offset"`
	BySeverity map[string]int `json:"by_severity"`
	Findings   []TraceFinding `json:"findings"`
}

// Export formats of a trace session.
const (
	ExportFindingsCSV  = "findings-csv"
	ExportFindingsJSON = "findings-json"
	ExportTimelineCSV  = "timeline-csv"
	ExportTimelineJSON = "timeline-json"
)

// Wire view of a live session's own traffic.
const (
	// MaxWireUnits bounds the units kept per session (the newest).
	MaxWireUnits = 256
	// MaxWireUnitBytes bounds a unit kept whole; a larger one is kept as
	// its size only.
	MaxWireUnitBytes = 64 << 10
	// MaxWireHexBytes bounds the bytes shown as hex per unit.
	MaxWireHexBytes = 4096
)

// WireView is the MMS units of one operation, as sent and received.
type WireView struct {
	SessionID string `json:"session_id"`
	// Operation is Operation.Wire of the operation.
	Operation uint64     `json:"operation"`
	Units     []WireUnit `json:"units"`
	// Note says when units are no longer held (the ring keeps the newest
	// MaxWireUnits of a session).
	Note string `json:"note,omitempty"`
}

// WireUnit is one ISO upper-layer unit (session SPDU) on the association.
type WireUnit struct {
	Seq uint64    `json:"seq"`
	At  time.Time `json:"at"`
	// Direction is "send" or "recv"; Association "observe" or
	// "commission".
	Direction   string `json:"direction"`
	Association string `json:"association"`
	Bytes       int    `json:"bytes"`
	// Hex is the unit, at most MaxWireHexBytes; Truncated says it was cut.
	Hex       string `json:"hex"`
	Truncated bool   `json:"truncated,omitempty"`
	// Session and Presentation are the SPDU and PPDU kinds; ACSE the
	// association control APDU (AARQ, AARE, RLRQ, RLRE, ABRT).
	Session      string   `json:"session,omitempty"`
	Presentation string   `json:"presentation,omitempty"`
	ACSE         string   `json:"acse,omitempty"`
	PDU          *WirePDU `json:"pdu,omitempty"`
	DecodeError  string   `json:"decode_error,omitempty"`
}
