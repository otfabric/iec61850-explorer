# Architecture

The Explorer's architecture is sunspec-explorer's (ADR 0001), with go-iec61850 and go-mms in place of go-sunspec and go-modbus. This document describes the Explorer as of M4 (trace analysis) and records what the libraries offer for the milestones after it. Library facts were checked against go-iec61850 v1.9.3 and go-mms v1.5.0 on 2026-11-08, and for M4 against go-mms-trace v0.1.1, go-mms v1.7.1 and go-iec61850 v1.15.2 on 2026-11-10.

## 1. Principles

1. One Go engine, two places it runs: in the desktop application's process (Local), or in the daemon, used from a browser (Remote). There is no Go client of the daemon.
2. The user interfaces talk to a `Service` interface only, never to MMS or HTTP directly.
3. The daemon is plain Go: static, no cgo, no Wails.
4. What a mode permits is a property of the code, enforced in the library and tested on the wire, not of the UI.
5. Diagnostics are evidence: a stage or a cause is shown only when an error type proves it.

## 2. Artifacts

| Artifact | Built with | Notes |
|---|---|---|
| `iec61850-explorer` | Wails v3 (Beta), cgo | Local mode only. Linux: GTK4 + WebKitGTK 6.0; Windows: WebView2; macOS: WKWebView. Built on a runner of the target architecture. |
| `iec61850-explorerd` | `CGO_ENABLED=0 go build` | Static; serves `/api/v1` and the embedded web UI (`frontend/dist/web`). Subcommands `url`, `token rotate`, `version`. Release targets: linux/amd64, arm64, armv7; also a container image. |

CI guards: `go list -deps ./cmd/iec61850-explorerd` contains no Wails, no cgo and no `internal/simied`; neither program depends on `internal/simied`.

## 3. Layering

```
cmd/iec61850-explorer  ->  internal/gui          -> service.Service (engine)
cmd/iec61850-explorerd ->  internal/api (server) -> service.Service (engine)   <- browser: fetch + SSE
internal/engine -> go-iec61850 -> go-mms -> go-cotp
internal/engine -> go-mms-trace (captures, offline) -> go-mms, go-cotp, go-iec61850 (GOOSE, SV)
```

- `internal/model`: JSON DTOs, independent of the library types.
- `internal/service`: the `Service` interface and its error codes.
- `internal/engine`: the one implementation. Sessions, discovery, browse, read, value conversion, error classification, the event hub.
- `internal/readonly`: the services of Observe mode.
- `internal/api`: HTTP/SSE handlers wrapping a `Service`.
- `internal/gui`: the Wails binding; forwards events to the window.
- `internal/web`: the embedded web UI handler (SPA fallback, CSP).
- `internal/daemon`: token, state file, data directory, bind rules, target allow-list.
- `internal/proxy`: SOCKS5 and HTTP CONNECT dialing (no dependency beyond the standard library); `internal/proxy/proxytest` holds test proxies, imported by tests only.
- `internal/simied`: a simulated IED (go-iec61850's server on `demo.icd`) that counts the services it receives. Tests only.

```go
type Service interface {
    Health(ctx) (model.Health, error)
    OpenSession(ctx, model.TargetConfig) (model.SessionInfo, error) // connect, Identify, discover
    CloseSession(ctx, id) error
    Sessions(ctx) ([]model.SessionInfo, error)                      // connected, offline and lost
    Browse(ctx, id, ref) ([]model.Node, error)                      // from the model, no IED traffic
    Read(ctx, id, model.ReadRequest) (model.ReadResult, error)      // batched; a snapshot session answers from its values
    Search(ctx, id, model.SearchQuery) (model.SearchResult, error)  // reference text, LN class, FC, kind
    StartPolling(ctx, id, model.PollConfig) error                   // samples arrive as events
    StopPolling(ctx, id) error
    Export(ctx, id, model.ExportRequest) (model.ExportFile, error)  // snapshot JSON or CSV
    OpenSnapshot(ctx, model.File) (model.SessionInfo, error)        // offline session from a snapshot
    InspectSCL(ctx, model.File) (model.SCLInfo, error)              // IEDs, access points with their address, findings
    OpenSCL(ctx, model.OpenSCLRequest) (model.SessionInfo, error)   // offline session from an SCL data model
    DataSets(ctx, id, ld) ([]model.DataSet, error)                  // IED, or the SCL of an offline session
    ReadDataSet(ctx, id, ref) (model.ReadResult, error)             // one MMS Read of the named variable list
    Reports(ctx, id, ld) ([]model.RCB, error)                       // RCB configuration, IED or SCL
    Files(ctx, id, dir) ([]model.FileEntry, error)
    ReadFile(ctx, id, name) (model.FileContent, error)              // at most 64 MiB
    Journals(ctx, id, ld) ([]string, error)
    ReadJournal(ctx, id, model.JournalRequest) (model.JournalResult, error)
    SettingGroups(ctx, id, ld) (model.SettingGroups, error)
    Profiles(ctx) ([]model.Profile, error)
    SaveProfile(ctx, model.Profile) (model.Profile, error)
    DeleteProfile(ctx, id) error
    OpenTrace(ctx, model.OpenTraceRequest) (model.SessionInfo, error) // a capture as a trace session (M4)
    LinkTrace(ctx, id, model.LinkTraceRequest) (model.SessionInfo, error) // correlate with a model session
    TraceSummary(ctx, id) (model.TraceSummary, error)
    TraceAssociations(ctx, id) ([]model.TraceAssociation, error)
    TraceTimeline(ctx, id, model.TimelineQuery) (model.Timeline, error) // filters and paging
    TraceReports(ctx, id, model.TraceReportsQuery) (model.TraceReports, error)
    TraceStreams(ctx, id) (model.TraceStreams, error)               // GOOSE and SV
    TraceFindings(ctx, id, model.FindingsQuery) (model.TraceFindings, error)
    Wire(ctx, id, operation) (model.WireView, error)                // the MMS units of one operation
    Settings(ctx) (model.Settings, error)                           // the default proxy
    SaveSettings(ctx, model.Settings) (model.Settings, error)       // refused when the daemon's operator set it
    Subscribe(ctx, sinceSeq) (<-chan model.Event, error)
}
```

The contract suite (`internal/api/contract_test.go`) runs the same tests against the engine and against the engine behind the HTTP API, with a test-only `net/http` adapter as the second `Service`. This is M0's exit criterion: one service, identical behaviour in the desktop and the daemon form.

## 4. Sessions

A live session = target + one go-iec61850 client (one MMS association) + the model discovered at open time + a recorder of MMS requests and of the association's ISO units + at most one poller. An offline session has a model and no association: from an SCL file, or from a snapshot (which also holds values). A trace session (M4) is a capture analysed offline; it has no model. `SessionInfo.kind` says which (`live`, `scl`, `snapshot`, `trace`). The window shows sessions in tabs.

- **Proxy.** `TargetConfig.Proxy` names how to reach the IED: empty uses the default proxy, `direct` none, else `socks5://`, `socks5h://` or `http://[user:password@]host[:port]` (ports 1080 and 8080 by default). The default is the daemon operator's (`engine.Options.DefaultProxy`, from `--proxy` or `ALL_PROXY`), else the user's setting (`Settings.DefaultProxy`, `<data-dir>/settings.json`, 0600). The engine parses it before dialling (a bad URL is `invalid_argument`) and hands go-iec61850 a dial function (`DialOptions.Dialer`, go-iec61850 v1.15) from `internal/proxy`, used for every attempt of the Observe association and of a Commission association. `internal/proxy` implements SOCKS5 CONNECT (RFC 1928) with username/password authentication (RFC 1929), IPv4, IPv6 and domain-name destinations (`socks5` resolves locally, `socks5h` sends the name), and HTTP CONNECT (RFC 9110, 9.3.6) with `Proxy-Authorization: Basic`, any 2xx accepted, the response header bounded to 8 KiB and bytes after it kept for the MMS connection. The dial context's deadline is set on the connection for the handshake and cleared afterwards; a cancelled context closes it. Failures are `*proxy.Error` with a stage (connect, resolve, not-allowed, handshake, auth, refused) and the SOCKS reply code or HTTP status; `classify` names the stage `proxy` from that type. A proxy URL's password never leaves the engine except in the profile or settings it was saved in: sessions (`SessionInfo.Proxy`, `Target.Proxy`), events, logs, errors and snapshots carry the redacted URL (`user:xxxxx@`).
- **Allow-list hooks.** `Options.AllowTarget` (the daemon's allow-list) is asked for the IED's host before dialling, and for the host of a proxy that is not the operator's; with a proxy that resolves names (`socks5h`, `http`) the IED must be an IP address. `Options.AllowAddr` is asked when a connection is opened (`net.Dialer.Control`) for the address actually connected to: the IED's on a direct connection, a user's proxy, and the IED address a `socks5` proxy is sent. A refusal is `AddrRefusedError` (code `not_allowed`).
- **Addressing.** `TargetConfig.Addressing` sets the T/S/P selectors, AP-titles and AE-qualifiers (the IED's SCL Address); a field left out takes the go-mms default, `""` omits the parameter. Selectors and AP-titles are validated before dialling.
- **Reads** of several object references go out 32 per MMS Read (`ReadMultiple`), one operation per call; a batch that fails as a whole, short of a lost association, is read again one reference at a time. Logical-node reads are not batched.
- **Polling** reads a session's references on an absolute start-to-start schedule (200 ms to 1 h). A cycle that ends after the next start skips the starts it missed and counts them as overruns; there is no catching up. Samples are events. A lost or closed session stops its poller. The window's watch table is the union of the polled references of all sessions; a reloaded page takes the references back from the engine's pollers.
- **Search** walks the session's model: reference text (case-insensitive substring), LN class, FC, kind; at most 500 hits.
- **SCL.** `InspectSCL` lists the IEDs and access points of a file the user chose, with the address the Communication section gives (IP, OSI selectors, AP-title, AE-qualifier) as a ready `TargetConfig`, and the parser's findings. `OpenSCL` builds an offline session from one access point's data model (go-iec61850 `scl.Flatten`): logical devices by their MMS domain names (`LDevice@ldName` or IED name + inst), logical nodes, data objects and attributes with their FCs. Control blocks (reports, logs, GOOSE, setting groups) are not built from SCL: their attributes depend on edition and server. `TestOpenSCLMatchesDiscovery` checks that the data model equals the live discovery of the simulated IED. The engine never reads a path: the window reads the file the user chose and hands over its bytes (at most 64 MiB).
- **Snapshots** (`otfabric.iec61850-explorer.snapshot/1`): target, identity, discovery summary, the whole model tree and the values read for the export, each with its MMS name. `OpenSnapshot` opens one as an offline session; reads answer from the stored values and say `source: "snapshot"`.
- **CSV** export: one row per leaf value (`read_at, ref, component, mms_domain, mms_item_id, type, value, validity, error`); text that a spreadsheet would take for a formula is prefixed with `'`.
- **Inspection** (M2a) of a logical device: data sets with their members, report control blocks with their configuration (RptID, DatSet, ConfRev, TrgOps, OptFlds, BufTm, IntgPd; from the IED also RptEna, Resv or ResvTms, SqNum, GI, Owner, EntryID), logs (journals) read by time range, the setting group control block, and the IED's file store with downloads. Each is one operation. For an offline SCL session, data sets and report control blocks come from the file (`source: "scl"`) and claim nothing only the IED knows (no RptEna, no Owner); `TestDataSetsLiveMatchSCL` and `TestReportsLiveMatchSCL` check that the simulated IED reports what its SCL defines. An enabled RCB is shown as possibly owned by another client: the Explorer does not subscribe in Observe mode.
- **Trace sessions** (M4, ADR 0005). `OpenTrace` analyses a capture the user chose (pcap or pcapng, plain or gzip; the bytes the window read, at most 256 MiB, else `limit_exceeded`) with go-mms-trace v0.1.1 `trace.Analyze` and keeps the result while the session is open; at most 4 trace sessions at once (`limit_exceeded`). `ports` names the TCP ports MMS runs on (default 102; 1 to 16 ports), so that a capture on another port (1102 in iec61850-interop's) is decoded. A capture that is not one is `invalid_argument` (`trace.ErrFormat`); a damaged one (`trace.ErrCorrupt`) opens with what was read before the damage, marked `damaged`, with a finding. The model views (browse, search, read, data sets, compare, conformance, exports other than the trace's) refuse a trace session with `invalid_argument`; closing it forgets it.
- **Trace views.** The summary (file: format, packets, interfaces and link types, time span, SHA-256 of the input as read, the ports; the counters; counts by status, by service and of findings by severity), the associations (client and server, TSAPs, how each was opened and closed, the AARE result, the AARQ's authentication mechanism (never a credential), the Initiate response's parameters, transactions, reports, unanswered requests and TCP gaps), the timeline of transactions in capture order (filters: association, service, status, a time window on the request time; offset and limit, at most 1000), the InformationReports and the GOOSE and SV streams. A transaction carries the decoded request and response (`WirePDU`: kind, invoke ID, service, objects, at most 64 values, write results, name lists, identity, errors, the raw content of a service go-mms does not model, its decode error) and the objects it names, mapped to IEC 61850 references.
- **Findings** are what go-mms-trace counted or decided, each with a severity, where it is (association, frame, time, transaction, stream) and the field it comes from (`source`): TCP gaps (`Association.Gaps`), associations captured midstream, opened without an AARQ, rejected, aborted, reset or still open at the end; unanswered, rejected, failed, orphan and pending transactions; PDUs with a decode error; the counters (TPKT resyncs, COTP and TSDU errors, GOOSE and SV decode errors; bare MMS, segmentation-offload segments and unsupported packets as information); a truncated tail, skipped pcapng blocks and a damaged file; GOOSE issues (a sequence or state number skipped, repeated, decreased, configuration revision or entry count changed, simulation) and SV samples lost or repeated. At most 20,000 per capture. Exports: findings and timeline as CSV (formula-protected cells) or JSON (`otfabric.iec61850-explorer.trace/1`), whole.
- **Correlation (application).** `LinkTrace` links a trace to a live, SCL or snapshot session and copies what that session knows at that moment: its model (references and FCs), and as far as the session can tell them its data sets, report control blocks and GOOSE control blocks (a live session's read in Observe mode: data sets and RCBs per logical device, and `DatSet`, `GoID` of the GoCBs its model lists; an SCL session's from the file, `GSEControl` included). Objects then carry `model`: `found`, `missing`, `not-iec61850` (a name outside the mapping of IEC 61850-8-1) or `unknown` (the session holds no definitions of that kind, such as a snapshot's data sets), and what they concern (the data object, the data sets with a member covering it and their report control blocks; for a control block its RptID or data set). An indexed RCB instance (`urcbEvents01`) matches its SCL definition. Every object of a transaction or report and every GoCB the model does not have is a `not-in-model` finding (one per object, counted). A captured IEC 61850 report (InformationReport on `RPT`) is matched to a report control block by its RptID (and data set, when the report names one; an RptID left empty in SCL is the RCB's reference) and decoded against that data set's members by the inclusion bit string, with data references and reasons; without the definitions, or when the bit string does not fit the data set, its values are shown as captured with the reason. The field layout of a report (IEC 61850-8-1) is walked by the engine (go-iec61850's decoder is not exported); every value was decoded by go-mms. A GOOSE stream's gocbRef is matched to the model's GoCBs, and a data set that differs is noted; a model without GoCBs says so. Unlinking removes the resolution and the model's findings.
- **Profiles** are kept by the engine in `<data-dir>/profiles.json` (0600, written atomically), the settings in `<data-dir>/settings.json` the same way. The desktop application's data directory is the user's configuration directory (`iec61850-explorer`); the daemon's is its `--data-dir`.

Connecting:

- `OpenSession` dials with the session timeout, asks Identify (an IED without it is normal), and builds the model tree with `TreeWithOptions{IncludeFCs: true}` (GetNameList per logical device). The tree is indexed; `Browse` answers from it without talking to the IED.
- A session's user actions are serialised by a mutex, so the requests the recorder sees during an action belong to it.
- When the association ends without the user closing it, the session becomes `lost`, keeps its tree for browsing and refuses reads. Closing a lost session forgets it; at most 16 lost sessions are kept.
- Reads take FC-qualified references (`LD/LN.DO.DA[FC]`). Each reference is read on its own (`ReadRaw`); its MMS type comes from GetVariableAccessAttributes, cached per reference, and names the components of a structure. A failed reference fails alone.

## 5. Observe mode

Every MMS client the engine creates sets `mms.DialOptions.AllowedServices` to `internal/readonly.Services`: Identify, Status, GetNameList, Read, GetVariableAccessAttributes, GetNamedVariableListAttributes, FileDirectory, FileOpen, FileRead, FileClose, ReadJournal. FileOpen and FileClose open and release a file read state machine for one download (ISO 9506-2 annex D); they change nothing the IED stores. FileRename, FileDelete and ObtainFile stay out. go-mms checks the list in `sendConfirmed`, the one function every confirmed request passes through, and refuses anything else before the transport (`mms.ErrServiceNotAllowed`); the association stays usable. Association control (Initiate, Conclude, Abort) is not a confirmed service.

Defense in depth:

1. The allow-list in the MMS client (`TestGuardRefusesWrite`).
2. `forbidigo` in `.golangci.yml`: state-changing go-iec61850 and go-mms calls and MMS servers are banned outside tests and `internal/simied`.
3. The wire test: `TestWireReadOnly` runs every workflow against the simulated IED, whose server metrics count the services it received, and asserts that each is in the list.
4. No read of FC CO (`internal/engine/observe_co.go`): reading `SBO` selects a control with normal security (IEC 61850-8-1), a state change the Read service carries, so the allow-list cannot catch it. The engine refuses CO references (`service_not_allowed`) and data sets with a CO member; the UI offers no read for CO (`TestObserveNeverReadsControls`, which checks from a second connection that no select is held; `TestSBOReadSelects` shows that the read does select).

Subscribing to a report control block writes RCB attributes, so report subscription belongs to Commission mode, not Observe mode. Reading an RCB's configuration does not.

## 5a. Commission mode (ADR 0004)

A session stays in Observe mode for its whole life. `GrantCommission` adds a grant: a reason, a duration (1 to 480 minutes, default 30) and capabilities (`write`, `reports`, `datasets`, `setting-groups`, `controls`). The engine opens a *second* association to the same IED whose allow-list is `internal/commission.Services(capabilities)`: the Observe services plus only what the capabilities need (Write; DefineNamedVariableList and DeleteNamedVariableList for data sets). Every state-changing call uses that association; the engine checks the capability before each call, and go-mms refuses a service outside the list.

- **Ends.** Revoke, expiry, a lost Commission association, the session closing or being lost, the engine stopping. Before concluding the association the engine runs its cleanup steps (what it set up for itself, newest first); each is audited, a failed one too.
- **Audit trail.** `<data dir>/audit/audit-YYYY-MM-DD.jsonl`, mode 0600, appended and synced before the outcome is reported. The sequence continues across runs; the recent 500 records are kept in memory, returned by `Audit` and published as `audit` events. Without a data directory, or when the trail cannot be written, no grant is given.
- **Writes.** `Write` takes a data attribute with FC SP, CF, DC, SV or BL and the value as text. The engine converts it with the attribute's MMS type (GetVariableAccessAttributes; a FLOAT32 is sent as a single float), reads the value before, writes, reads it back and reports `verified`. The setting group control block (`LLN0.SGCB`, FC SP) is refused: setting groups have their own capability.
- **Where.** The desktop application offers Commission mode; the daemon only with `--allow-commission`. `Health.features` contains `commission` when it is offered.
- **Reports** (`reports`). The engine runs the control block's lifecycle itself so that each step reaches the audit trail: it refuses an RCB that is enabled or reserved (another client may own it), registers a passive go-iec61850 subscription, reserves an unbuffered RCB, enables it and, if asked, triggers a general interrogation. Reports are published as `report` events; entries are named by the data set's members (or by the report's data references, converted from MMS form) and converted with their types. Unsubscribing, revoking or losing the grant disables and releases the RCB. At most 8 subscriptions per grant; a queue of 256 reports per subscription, overflow counted.
- **Data sets** (`datasets`). Create a persistent data set in a logical node (`LD/LLN0.Name`, 1 to 100 FC-qualified members) and read it back; delete only what the IED marks deletable and no subscription of the grant uses. The Observe association's cached definitions are invalidated after each change.
- **Controls** (`controls`). `Control` takes a data object and a ctlVal (text, parsed with the type of `Oper.ctlVal`) and reads the object's `ctlModel` on the Observe association. It then runs that model's sequence on the grant's association (IEC 61850-7-2, clause 20):
  - direct, normal security: Operate;
  - SBO, normal security: Select (a Read of SBO), then Operate;
  - direct, enhanced security: Operate, then wait for the CommandTermination (`OperateWait`, default 10 s, at most 60 s);
  - SBO, enhanced security: SelectWithValue, then Operate and wait.

  A failed operate after a select sends Cancel. The result lists every step with the IED's AddCause (from LastApplError) and the stVal before and after. The outcome is ok, refused, negative (accepted, then a negative termination), no-termination or failed. One audit record holds the whole sequence. The originator is remote-control, "OT Fabric IEC 61850 Explorer", with a process-wide ctlNum. The request may ask the IED for an interlock check and a synchrocheck (the Check attribute); this needs go-iec61850 v1.10.1, which sends the two bits in the standard order.
- **Setting groups** (`setting-groups`). Select the active setting group (1 to NumOfSGs) and read the control block back. Editing setting values (SelectEditSG, SE values, ConfirmEditSG) is not offered: go-iec61850's server keeps no SE values, so there is no peer to test it against.
- **Lint.** forbidigo exempts the engine's `commission*.go` files, the only code that holds the Commission association.

## 6. Diagnostics

- **Comparison (M3a).** `Compare` reads both sessions as the user sees them: the model as discovered or imported, data sets and report control blocks as the session lists them (from the IED, or from the SCL file). Control blocks are left out of the model comparison, because an IED's MMS model lists them as nodes and an SCL file defines them apart; report control blocks are compared as such, an SCL `ReportControl` against the indexed instances the IED serves (`brcbA` against `brcbA01`, `brcbA02`). Run-time attributes (enabled, reserved, sequence number, owner) are not configuration and are not compared; an SCL report ID left empty is assigned by the IED. A node missing on one side is reported once, not with everything below it. A snapshot holds no data set or report control block definitions, so those categories say so instead of reporting differences. Logical devices are matched by name: two IEDs with different IED names compare as different devices.
- **Semantics and conformance (M3c).** go-iec61850-model embeds every NSD release IEC publishes (28 namespaces: IEC 61850-7-2, 7-3 and 7-4 from 2007A2 to 2007B6, 7-410, 7-420, the technical reports), gzip-compressed, loaded once on first use (about 65 ms). Each logical node is checked against the namespace it declares: its own `NamPlt.lnNs`, else its logical device's `LLN0.NamPlt.ldNs`; an extension data object against its `dataNs`. A live session reads these (FC EX) in Observe mode; an SCL file gives them, and without a declaration a file of schema 1.7 is taken as Edition 1. A declaration selects a loaded package's namespace first, then the embedded profile go-iec61850-model chooses for it (`official.ForNamespace`). IEC publishes no NSD for Edition 1 of IEC 61850-7-4 (2003): such a device is checked against the default profile (2007B6) and a mandatory data object it lacks is a warning ("since Edition 2"), not an error. A declared namespace that is neither embedded nor loaded is a warning, and the node is checked against the default profile. For a node the engine gives the class (and base), the data object as the namespace names it (`Ind3` is the multi-instance `Ind`), its CDC and presence, the attribute's type and FC, the quantity, phase and unit hint of a measurement, what was declared and checked against, and, from a loaded package with NSDoc, the title and description (shown, never exported: IEC and vendor texts are not redistributable). Conformance findings: a class the namespace does not define (warning), a mandatory data object (presence M) the node lacks (error; warning for Edition 1), a data object the class does not define (information; not listed one by one under a declared namespace that is not loaded, where one finding says so), for an SCL session a data object whose type declares another CDC (warning; information for Edition 1). A data object the node's namespace does not define is also looked up in the other namespaces its logical device declares: vendors add extension data objects to standard logical nodes without a `dataNs`, and once the vendor's package is loaded they are found there. Conditional presence is not evaluated. Search by quantity uses the semantic rules (an SCL file's declarations; the default profile for a live session, since the rules do not change between editions).
- **Namespace packages.** The user loads NSD packages (a ZIP archive with NSD and NSDoc files, or one NSD document) through the user interface: official releases that are not embedded, and vendor extension namespaces (ABB, Siemens, Schneider declare their own logical nodes and data objects in `lnNs`, `dataNs`). Like SCL files they arrive as bytes; go-iec61850-model's bounded importer parses them; a package defining a release that is embedded or loaded already is refused. They are kept as files in `<data-dir>/namespaces` (mode 0600) and loaded again at start; at most 64 packages of 32 MiB each.
- **Point lists (M3d).** `points-csv` and `points-json` list every basic attribute with each FC (control blocks left out): the LD, LN, class, data object and attribute, the namespace's CDC, type and presence, the quantity and unit, the data sets that carry it (as an attribute or through its data object or structure) and the report control blocks of those data sets. A live session lists data sets and report control blocks from the IED; a snapshot holds none, which the JSON says in its `note`. CSV cells are protected against formula injection.
- **Values (M3b).** With `values`, the comparison adds the basic attributes of FC CF, DC, SP and SG that both sides have a value for. An SCL session gives the configured value: the instance's `DAI` values or the type's default, through `SDI`, structures and sub data objects (arrays are left out). A live session reads its data objects per FC (`LD/LN.DO[CF]`, at most 200 per read, in Observe mode) and a snapshot gives what it holds. Values are compared as booleans, numbers or strings: an enumeration's SCL literal by its ordinal (shown as `literal (ordinal)`), a float at FLOAT32 precision. Quality, time stamps and other types are counted as not compared. A setting (SG) has a value per setting group in SCL (`sGroup`, go-iec61850 v1.14.0): two SCL files compare every group, an IED gives the values of its active group, and that group is compared. Settings being edited (SE) are not compared: reading them needs an edit group, which is a Commission action.

- **Operations.** Every user action (`connect`, `discover`, `read`, `close`) is published as an `operation` event with its duration, result and the MMS requests it caused (service, invoke ID, duration, error), from go-mms `Metrics` events. The UI shows them in the activity panel.
- **Wire view (application-to-wire correlation, M4).** Every association of a live session (Observe and Commission) is dialled with go-mms `DialOptions.RawHook`, which hands the session's recorder each ISO unit (session SPDU) sent and received; it only observes. The units of a running action are kept with it and, when its operation is published, moved to a ring of the session (the newest 256 units, at most 4 MiB, a unit above 64 KiB kept as its size); `Operation.wire` numbers the operation. Poll cycles are not operations and keep none. `Wire(id, operation)` decodes them with `mms.DecodeTSDU`: direction, association, session and presentation kinds, the ACSE APDU, the MMS PDU (as in a trace) and the first 4 KiB as hex; the activity panel shows them under the operation and marks the units whose invoke ID is one of its requests. Units are never logged; the hex of an AARQ that carries authentication is withheld (the Explorer sends none).
- **Connect stages.** `internal/engine/classify.go` places a failure in its layer only from the error type: `*proxy.Error` is the proxy (with its own stage in the hint); `*engine.AddrRefusedError` is the allow-list at TCP; `*net.OpError` (dial) is TCP; `*cotp.RejectionError` is COTP, with reason 3 explained as the T-selector; `*mms.ProtocolError` gives session, presentation, ACSE or MMS phase; `mms.ErrConnectionRejected` is ACSE; `mms.ErrNegotiationFailed` is MMS Initiate. Anything else gets no stage. Response errors map data-access errors, ConfirmedError and Reject.
- **Values.** Structures are named from the variable's type; a 13-bit bit string named `q` is decoded as an IEC 61850-7-3 quality; a UTC time carries its time quality; integers beyond 2^53 cross JSON as strings; NaN and infinities have no JSON value. Every read shows the MMS domain and item ID next to the IEC 61850 reference.

## 7. HTTP API

All `/api/v1` requests need `Authorization: Bearer <token>` and a Host the daemon answers to. Errors: `{"error":{"code","message","detail"}}`, where `detail` is the classified `OpError` of a failed connect or discovery.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/health` | Versions, API version, mode, features (`profiles`, `commission`), uptime, engine time |
| `GET /api/v1/sessions`, `POST /api/v1/sessions` | List, open (`{host, port, timeout_ms, addressing, proxy}`) |
| `GET /api/v1/sessions/{id}`, `DELETE ...` | Get, close |
| `GET /api/v1/sessions/{id}/browse?ref=` | Children of a node; no `ref` lists the logical devices |
| `POST /api/v1/sessions/{id}/read` | `{refs: [...]}`, at most 200 FC-qualified references |
| `POST /api/v1/sessions/{id}/search` | `{text, ln_class, fc, kind, limit}` |
| `PUT /api/v1/sessions/{id}/polling`, `DELETE ...` | Start (`{refs, interval_ms}`, replaces) or stop polling |
| `POST /api/v1/sessions/{id}/export` | `{format: snapshot, csv, points-csv or points-json, refs}` -> `{name, mime, text}`; the client saves the text. A trace session: `findings-csv`, `findings-json`, `timeline-csv`, `timeline-json` |
| `POST /api/v1/snapshots/open?name=` | Raw snapshot body -> offline session |
| `GET /api/v1/sessions/{id}/datasets?ld=`, `POST .../datasets/read` (`{ref}`) | Data sets with members; read one |
| `GET /api/v1/sessions/{id}/reports?ld=` | Report control blocks with their configuration |
| `GET /api/v1/sessions/{id}/files?dir=`, `GET .../files/download?name=` | The IED's files; a download as `application/octet-stream` |
| `GET /api/v1/sessions/{id}/journals?ld=`, `POST .../journals/read` | Logs; entries in a time range |
| `GET /api/v1/sessions/{id}/setting-groups?ld=` | The setting group control block |
| `PUT /api/v1/sessions/{id}/commission`, `DELETE ...` | Grant Commission mode (`{reason, minutes, capabilities}`), revoke it |
| `POST /api/v1/sessions/{id}/write` | `{ref, value}` -> the write with the value before and read back; needs the `write` capability |
| `POST /api/v1/sessions/{id}/control` | `{ref, value, test, timeout_ms}` -> every step of the control, its outcome and AddCause, stVal before and after; needs the `controls` capability |
| `POST /api/v1/sessions/{id}/reports/subscriptions` (`{rcb, gi}`), `DELETE .../{sub}`, `POST .../{sub}/gi` | Subscribe to an RCB, unsubscribe, general interrogation; needs `reports` |
| `POST /api/v1/sessions/{id}/datasets` (`{ref, members}`), `DELETE .../datasets?ref=` | Create, delete a data set; needs `datasets` |
| `PUT /api/v1/sessions/{id}/setting-groups/active` (`{ld, sg}`) | Select the active setting group; needs `setting-groups` |
| `GET /api/v1/audit?session=&limit=` | Recent audit records, newest first |
| `GET /api/v1/sessions/{id}/semantics?ref=` | What the namespace defines for a node: class, data object, CDC, presence, attribute type, quantity; for a logical node its class's data objects |
| `GET /api/v1/sessions/{id}/conformance` | The model against the namespace each logical device or node declares: what was declared and checked, findings with severity and evidence |
| `GET /api/v1/namespaces`, `POST /api/v1/namespaces?name=`, `DELETE /api/v1/namespaces/{id}` | The embedded namespace releases and the loaded packages; load a package (raw NSD ZIP or document body); remove one |
| `POST /api/v1/compare` | `{a, b, values}` -> the comparison of two open sessions: model, data sets, report control blocks and, with `values`, the values of CF, DC, SP and SG attributes (reads only) |
| `POST /api/v1/scl/inspect?name=`, `POST /api/v1/scl/open?name=&ied=&ap=` | Raw SCL body -> description, or an offline session |
| `POST /api/v1/traces/open?name=&ports=` | Raw capture body (at most 256 MiB) -> a trace session; `ports` comma separated, default 102 |
| `PUT /api/v1/sessions/{id}/trace/link` | `{session}` -> the trace session, correlated with that session; `""` unlinks |
| `GET /api/v1/sessions/{id}/trace/summary`, `.../trace/associations`, `.../trace/streams` | The capture, its associations, its GOOSE and SV streams |
| `POST /api/v1/sessions/{id}/trace/timeline`, `.../trace/reports`, `.../trace/findings` | `{association, service, status, from, to, offset, limit}`, `{association, offset, limit}`, `{severity, kind, offset, limit}` -> a page |
| `GET /api/v1/sessions/{id}/wire?operation=` | The decoded MMS units of one operation of a live session |
| `GET /api/v1/profiles`, `POST`, `PUT /profiles/{id}`, `DELETE /profiles/{id}` | Connection profiles |
| `GET /api/v1/settings`, `PUT /api/v1/settings` | `{default_proxy, default_proxy_locked}`; `PUT` is `not_allowed` when the operator set the default (`--proxy`, `ALL_PROXY`) |
| `GET /api/v1/events?since=<seq>` | SSE stream (`session`, `operation`, `sample`, `resync`, `audit`, `report`); replay from `since` or `Last-Event-ID` |
| `GET /` and assets | The web UI, unless `--no-ui` |

No endpoint accepts a file path, an MMS PDU to send or a service name; a capture is analysed, never replayed. The only value to write goes through `/write`, which needs a Commission grant. Files arrive as request bodies.

The daemon's security (token lifecycle, Host check, bind rules, target allow-list, data directory checks) is sunspec-explorer's; the target allow-list is also checked when a connection is opened, and applies to proxies a user names (see section 4 and `SECURITY.md`).

## 8. Capability audit of the libraries

What the Explorer's later milestones need, against go-iec61850 v1.9.3, go-mms v1.5.0 and go-iec61850-model v0.6.0. "Gap" lines are the libraries' own documented limitations (go-iec61850 `KNOWN_LIMITATIONS.md`, go-mms `KNOWN_LIMITATIONS.md`, `docs/FEATURE-STATUS.md`) or found while building M0.

| Need | Library state | Consequence |
|---|---|---|
| Read-only enforcement | go-mms v1.5.0 `DialOptions.AllowedServices` (added for M0) | Done |
| Per-request trace of own traffic | go-mms `Metrics` (request and connection events), client `RawHook` (raw ISO payloads) and `DecodeTSDU` (go-mms v1.7) | Operations in M0; since M4 the wire view of each operation |
| Connect failure stages | Typed errors per layer (COTP rejection with reason, `ProtocolError.Phase`, ACSE rejection, negotiation) | Done for what the types tell; an undifferentiated error gets no stage |
| Self-description, browse, typed reads | `TreeWithOptions{IncludeFCs}`, `ReadRaw`, `GetVariableType`, `ReadMultiple` | Batched reads since M1 |
| Model cache across runs | Gap: in-memory only | Snapshots (M1) are the Explorer's own versioned files |
| Datasets | `ListDataSets`, `GetDataSet`, `ReadDataSet`; create/delete | Inspection since M2a; create/delete in Commission mode (M2b) |
| Reports | RCB read/write, subscription; gap: single-client ownership assumed. Data-object members (FCD, such as `MMXU1.Hz[MX]`) are reported as structures and trigger on changes below them since go-iec61850 v1.9.4 | Since M2b2: the engine runs the RCB lifecycle itself, refuses RCBs another client enabled or reserved, audits each step. The simulated IED's `Events` data set mixes attribute members and a data-object member |
| Controls | `OperateWait` waits for the CommandTermination and `ControlError.LastApplError` carries the IED's reason (go-iec61850 v1.10.0, checked against libiec61850, dscsystems, iec61850-rust and cbsigma). The Check bits follow IEC 61850-8-1 since v1.10.1 (v1.10.0 swapped them). Gaps: no time-activated operate; Cancel always sends Test false | Since M2b3: all four control models, with steps and causes audited; interlock check and synchrocheck since 0.6.0 |
| Setting groups, files, journals | Client APIs present; server gap: no SE (edit) values | Reading since M2a (file and journal reads joined `readonly.Services`); active setting group selection since M2b2; SE editing waits for a peer that supports it |
| SCL | Parser, `Flatten`, semantic validation; gaps: `Inputs`, `ExtRef`, `ClientLN` not normalised, no XSD validation | Import and offline browsing since M1; flow analysis waits for the normalisation |
| GOOSE/SV live capture | Linux and macOS raw Ethernet; gap: Windows `ErrUnsupportedPlatform` | PCAP import first |
| Semantics | go-iec61850-model v0.8.2: every published NSD release, profiles per declaration, NSD package import, reference resolution, semantic rules | Since M3c: the namespace view, conformance, search by quantity, point lists |
| Passive MMS trace analysis | go-mms-trace v0.1.1: pcap and pcapng (plain or gzip), IPv4 and IPv6 with fragments, TCP reassembly, TPKT, COTP, session, presentation, ACSE and MMS (go-mms `DecodeTSDU`), GOOSE and SV (go-iec61850). Gaps: MMS over TLS not decodable; MMS found on the given ports only; GOOSE and SV on the Ethernet link type only; a TCP gap is counted per association, not located; the result is not bounded by the input (LIMITATIONS.md). Evidence: generated captures, captures between independent stacks and public captures in CI; the Wireshark comparison is manual and compares PDUs by kind, invoke ID and request service only. go-iec61850 does not export its decoder of a report's fields | Since M4: trace sessions, findings, timelines, correlation with a model session; the engine walks a report's field layout itself |
| TLS | go-mms optional TLS transport | Exposed later with verified configuration, no IEC 62351 claim |

## 9. Milestones

| M | Deliverable | Exit criterion |
|---|---|---|
| M0 (done) | The foundation: engine, Observe mode in the library, daemon and desktop, contract suite, diagnostics groundwork | One service against a simulated IED, identical in desktop and daemon (contract suite) |
| M1 (done) | Read-only browser: connection profiles and ISO addressing, batched reads, polling and a watch table across IEDs, search, SCL import, snapshots, CSV, the first release | A user connects, finds meaningful points, monitors and exports them without the CLI |
| M2a (done) | Inspection in Observe mode: data sets, report control blocks, logs, setting groups, the file store; SCL definitions alongside | Everything an IED defines can be seen without changing it |
| M2b1 (done) | Commission mode foundation: per-session grant on a second association, audit trail, writes with readback | A user changes a set point or configuration value with a verified, recorded write |
| M2b2 (done) | Report subscription (reserve, enable, GI, cleanup on revoke), dynamic data sets, active setting group selection | Practical MMS workflows with predictable safety and cleanup |
| M2b3 (done) | Controls (direct, SBO, enhanced security), after go-iec61850's client waits for CommandTermination | A control's outcome, including its termination, is shown and audited |
| M3a (done) | Comparison of definitions: any two sessions (IED, SCL, snapshot); the model (logical devices, nodes, data, FCs, LN classes), data sets (members in order), report control blocks (configuration; indexed instances matched to their SCL definition) | A mismatch introduced into an SCL file shows up against the IED serving the original, with both sides (`TestCompareSCLWithIED`) |
| M3b (done) | Comparison of values: the configured values of an SCL file, a snapshot's values and the IED's, for configuration and setting FCs (CF, DC, SP, SG) | A changed configuration value is found with its two values and where each came from (`TestCompareValuesSCLWithIED`) |
| M3c (done) | Semantics from go-iec61850-model: the NSD definition of a logical node class, data object and CDC beside the node; conformance findings (a mandatory data object missing, a data object or CDC the namespace does not define); search by semantics | A non-conformant model is diagnosed against the namespace it claims |
| M3d (done) | Integration exports: point lists (reference, FC, type, CDC, description, data set and report membership) as CSV and JSON for SCADA and gateway engineering | A point list for a feeder IED is produced without the CLI |
| M4 (done) | Trace analysis: PCAP/PCAPNG (go-mms-trace), timelines, findings, correlation of a capture with a model session (references, data sets, report and GOOSE control blocks), the wire view of the Explorer's own operations | A capture is analysed offline, gaps identified: a capture of the Explorer reading the simulated IED, with one segment dropped, shows the TCP gap and the request it left unanswered as findings, and its object names resolve to the IED's model (`TestTraceGapsOfSimIED`) |
| M5 | Laboratory: evidence bundles, constrained replay on the go-iec61850 server | Repeatable scenarios; unsupported behaviour is not fabricated |
