# Architecture

The Explorer's architecture is sunspec-explorer's (ADR 0001), with go-iec61850 and go-mms in place of go-sunspec and go-modbus. This document describes the Explorer as of M1 (the read-only browser) and records what the libraries offer for the milestones after it. Library facts were checked against go-iec61850 v1.9.3 and go-mms v1.5.0 on 2026-11-08.

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
```

- `internal/model`: JSON DTOs, independent of the library types.
- `internal/service`: the `Service` interface and its error codes.
- `internal/engine`: the one implementation. Sessions, discovery, browse, read, value conversion, error classification, the event hub.
- `internal/readonly`: the services of Observe mode.
- `internal/api`: HTTP/SSE handlers wrapping a `Service`.
- `internal/gui`: the Wails binding; forwards events to the window.
- `internal/web`: the embedded web UI handler (SPA fallback, CSP).
- `internal/daemon`: token, state file, data directory, bind rules, target allow-list.
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
    Subscribe(ctx, sinceSeq) (<-chan model.Event, error)
}
```

The contract suite (`internal/api/contract_test.go`) runs the same tests against the engine and against the engine behind the HTTP API, with a test-only `net/http` adapter as the second `Service`. This is M0's exit criterion: one service, identical behaviour in the desktop and the daemon form.

## 4. Sessions

A live session = target + one go-iec61850 client (one MMS association) + the model discovered at open time + a recorder of MMS requests + at most one poller. An offline session has a model and no association: from an SCL file, or from a snapshot (which also holds values). The window shows sessions in tabs.

- **Addressing.** `TargetConfig.Addressing` sets the T/S/P selectors, AP-titles and AE-qualifiers (the IED's SCL Address); a field left out takes the go-mms default, `""` omits the parameter. Selectors and AP-titles are validated before dialling.
- **Reads** of several object references go out 32 per MMS Read (`ReadMultiple`), one operation per call; a batch that fails as a whole, short of a lost association, is read again one reference at a time. Logical-node reads are not batched.
- **Polling** reads a session's references on an absolute start-to-start schedule (200 ms to 1 h). A cycle that ends after the next start skips the starts it missed and counts them as overruns; there is no catching up. Samples are events. A lost or closed session stops its poller. The window's watch table is the union of the polled references of all sessions; a reloaded page takes the references back from the engine's pollers.
- **Search** walks the session's model: reference text (case-insensitive substring), LN class, FC, kind; at most 500 hits.
- **SCL.** `InspectSCL` lists the IEDs and access points of a file the user chose, with the address the Communication section gives (IP, OSI selectors, AP-title, AE-qualifier) as a ready `TargetConfig`, and the parser's findings. `OpenSCL` builds an offline session from one access point's data model (go-iec61850 `scl.Flatten`): logical devices by their MMS domain names (`LDevice@ldName` or IED name + inst), logical nodes, data objects and attributes with their FCs. Control blocks (reports, logs, GOOSE, setting groups) are not built from SCL: their attributes depend on edition and server. `TestOpenSCLMatchesDiscovery` checks that the data model equals the live discovery of the simulated IED. The engine never reads a path: the window reads the file the user chose and hands over its bytes (at most 64 MiB).
- **Snapshots** (`otfabric.iec61850-explorer.snapshot/1`): target, identity, discovery summary, the whole model tree and the values read for the export, each with its MMS name. `OpenSnapshot` opens one as an offline session; reads answer from the stored values and say `source: "snapshot"`.
- **CSV** export: one row per leaf value (`read_at, ref, component, mms_domain, mms_item_id, type, value, validity, error`); text that a spreadsheet would take for a formula is prefixed with `'`.
- **Inspection** (M2a) of a logical device: data sets with their members, report control blocks with their configuration (RptID, DatSet, ConfRev, TrgOps, OptFlds, BufTm, IntgPd; from the IED also RptEna, Resv or ResvTms, SqNum, GI, Owner, EntryID), logs (journals) read by time range, the setting group control block, and the IED's file store with downloads. Each is one operation. For an offline SCL session, data sets and report control blocks come from the file (`source: "scl"`) and claim nothing only the IED knows (no RptEna, no Owner); `TestDataSetsLiveMatchSCL` and `TestReportsLiveMatchSCL` check that the simulated IED reports what its SCL defines. An enabled RCB is shown as possibly owned by another client: the Explorer does not subscribe in Observe mode.
- **Profiles** are kept by the engine in `<data-dir>/profiles.json` (0600, written atomically). The desktop application's data directory is the user's configuration directory (`iec61850-explorer`); the daemon's is its `--data-dir`.

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

- **Operations.** Every user action (`connect`, `discover`, `read`, `close`) is published as an `operation` event with its duration, result and the MMS requests it caused (service, invoke ID, duration, error), from go-mms `Metrics` events. The UI shows them in the activity panel.
- **Connect stages.** `internal/engine/classify.go` places a failure in its layer only from the error type: `*net.OpError` (dial) is TCP; `*cotp.RejectionError` is COTP, with reason 3 explained as the T-selector; `*mms.ProtocolError` gives session, presentation, ACSE or MMS phase; `mms.ErrConnectionRejected` is ACSE; `mms.ErrNegotiationFailed` is MMS Initiate. Anything else gets no stage. Response errors map data-access errors, ConfirmedError and Reject.
- **Values.** Structures are named from the variable's type; a 13-bit bit string named `q` is decoded as an IEC 61850-7-3 quality; a UTC time carries its time quality; integers beyond 2^53 cross JSON as strings; NaN and infinities have no JSON value. Every read shows the MMS domain and item ID next to the IEC 61850 reference.

## 7. HTTP API

All `/api/v1` requests need `Authorization: Bearer <token>` and a Host the daemon answers to. Errors: `{"error":{"code","message","detail"}}`, where `detail` is the classified `OpError` of a failed connect or discovery.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/health` | Versions, API version, mode, features (`profiles`, `commission`), uptime, engine time |
| `GET /api/v1/sessions`, `POST /api/v1/sessions` | List, open (`{host, port, timeout_ms, addressing}`) |
| `GET /api/v1/sessions/{id}`, `DELETE ...` | Get, close |
| `GET /api/v1/sessions/{id}/browse?ref=` | Children of a node; no `ref` lists the logical devices |
| `POST /api/v1/sessions/{id}/read` | `{refs: [...]}`, at most 200 FC-qualified references |
| `POST /api/v1/sessions/{id}/search` | `{text, ln_class, fc, kind, limit}` |
| `PUT /api/v1/sessions/{id}/polling`, `DELETE ...` | Start (`{refs, interval_ms}`, replaces) or stop polling |
| `POST /api/v1/sessions/{id}/export` | `{format: snapshot or csv, refs}` -> `{name, mime, text}`; the client saves the text |
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
| `POST /api/v1/scl/inspect?name=`, `POST /api/v1/scl/open?name=&ied=&ap=` | Raw SCL body -> description, or an offline session |
| `GET /api/v1/profiles`, `POST`, `PUT /profiles/{id}`, `DELETE /profiles/{id}` | Connection profiles |
| `GET /api/v1/events?since=<seq>` | SSE stream (`session`, `operation`, `sample`, `resync`, `audit`, `report`); replay from `since` or `Last-Event-ID` |
| `GET /` and assets | The web UI, unless `--no-ui` |

No endpoint accepts a file path, an MMS PDU or a service name. The only value to write goes through `/write`, which needs a Commission grant. Files arrive as request bodies.

The daemon's security (token lifecycle, Host check, bind rules, target allow-list, data directory checks) is sunspec-explorer's, unchanged; see `SECURITY.md`.

## 8. Capability audit of the libraries

What the Explorer's later milestones need, against go-iec61850 v1.9.3, go-mms v1.5.0 and go-iec61850-model v0.6.0. "Gap" lines are the libraries' own documented limitations (go-iec61850 `KNOWN_LIMITATIONS.md`, go-mms `KNOWN_LIMITATIONS.md`, `docs/FEATURE-STATUS.md`) or found while building M0.

| Need | Library state | Consequence |
|---|---|---|
| Read-only enforcement | go-mms v1.5.0 `DialOptions.AllowedServices` (added for M0) | Done |
| Per-request trace of own traffic | go-mms `Metrics` (request and connection events) and client `RawHook` (raw ISO payloads) | Operations in M0; a wire view can use `RawHook` |
| Connect failure stages | Typed errors per layer (COTP rejection with reason, `ProtocolError.Phase`, ACSE rejection, negotiation) | Done for what the types tell; an undifferentiated error gets no stage |
| Self-description, browse, typed reads | `TreeWithOptions{IncludeFCs}`, `ReadRaw`, `GetVariableType`, `ReadMultiple` | Batched reads since M1 |
| Model cache across runs | Gap: in-memory only | Snapshots (M1) are the Explorer's own versioned files |
| Datasets | `ListDataSets`, `GetDataSet`, `ReadDataSet`; create/delete | Inspection since M2a; create/delete in Commission mode (M2b) |
| Reports | RCB read/write, subscription; gap: single-client ownership assumed. Data-object members (FCD, such as `MMXU1.Hz[MX]`) are reported as structures and trigger on changes below them since go-iec61850 v1.9.4 | Since M2b2: the engine runs the RCB lifecycle itself, refuses RCBs another client enabled or reserved, audits each step. The simulated IED's `Events` data set mixes attribute members and a data-object member |
| Controls | `OperateWait` waits for the CommandTermination and `ControlError.LastApplError` carries the IED's reason (go-iec61850 v1.10.0, checked against libiec61850, dscsystems, iec61850-rust and cbsigma). The Check bits follow IEC 61850-8-1 since v1.10.1 (v1.10.0 swapped them). Gaps: no time-activated operate; Cancel always sends Test false | Since M2b3: all four control models, with steps and causes audited; interlock check and synchrocheck since 0.6.0 |
| Setting groups, files, journals | Client APIs present; server gap: no SE (edit) values | Reading since M2a (file and journal reads joined `readonly.Services`); active setting group selection since M2b2; SE editing waits for a peer that supports it |
| SCL | Parser, `Flatten`, semantic validation; gaps: `Inputs`, `ExtRef`, `ClientLN` not normalised, no XSD validation | Import and offline browsing since M1; flow analysis waits for the normalisation |
| GOOSE/SV live capture | Linux and macOS raw Ethernet; gap: Windows `ErrUnsupportedPlatform` | PCAP import first |
| Semantics | go-iec61850-model: NSD catalog, reference resolution, search | M3 semantic view and search |
| Passive MMS trace analysis | No trace engine yet | A separate trace package when M4 is committed |
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
| M3 | Diagnostics and semantics: SCL/live/snapshot comparison, NSD descriptions, semantic search, integration exports | Introduced mismatches can be diagnosed from visible evidence |
| M4 | Trace analysis: PCAP/PCAPNG, timelines, application-to-wire correlation | A capture is analysed offline, gaps identified |
| M5 | Laboratory: evidence bundles, constrained replay on the go-iec61850 server | Repeatable scenarios; unsupported behaviour is not fabricated |
