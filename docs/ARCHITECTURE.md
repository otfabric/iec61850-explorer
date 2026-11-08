# Architecture

The Explorer's architecture is sunspec-explorer's (ADR 0001), with go-iec61850 and go-mms in place of go-sunspec and go-modbus. This document describes M0, the foundation, and records what the libraries offer for the milestones after it. Library facts were checked against go-iec61850 v1.9.3 and go-mms main (v1.5.0 prepared) on 2026-11-08.

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
    Health(ctx context.Context) (model.Health, error)
    OpenSession(ctx context.Context, cfg model.TargetConfig) (model.SessionInfo, error) // connect, Identify, discover
    CloseSession(ctx context.Context, id string) error
    Sessions(ctx context.Context) ([]model.SessionInfo, error)
    Browse(ctx context.Context, id, ref string) ([]model.Node, error) // from the discovered model, no IED traffic
    Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error)
    Subscribe(ctx context.Context, sinceSeq uint64) (<-chan model.Event, error)
}
```

The contract suite (`internal/api/contract_test.go`) runs the same tests against the engine and against the engine behind the HTTP API, with a test-only `net/http` adapter as the second `Service`. This is M0's exit criterion: one service, identical behaviour in the desktop and the daemon form.

## 4. Sessions

A session = target + one go-iec61850 client (one MMS association) + the model discovered at open time + a recorder of MMS requests.

- `OpenSession` dials with the session timeout, asks Identify (an IED without it is normal), and builds the model tree with `TreeWithOptions{IncludeFCs: true}` (GetNameList per logical device). The tree is indexed; `Browse` answers from it without talking to the IED.
- A session's user actions are serialised by a mutex, so the requests the recorder sees during an action belong to it.
- When the association ends without the user closing it, the session becomes `lost`, keeps its tree for browsing and refuses reads. Closing a lost session forgets it; at most 16 lost sessions are kept.
- Reads take FC-qualified references (`LD/LN.DO.DA[FC]`). Each reference is read on its own (`ReadRaw`); its MMS type comes from GetVariableAccessAttributes, cached per reference, and names the components of a structure. A failed reference fails alone.

## 5. Observe mode

Every MMS client the engine creates sets `mms.DialOptions.AllowedServices` to `internal/readonly.Services`: Identify, Status, GetNameList, Read, GetVariableAccessAttributes, GetNamedVariableListAttributes. go-mms checks the list in `sendConfirmed`, the one function every confirmed request passes through, and refuses anything else before the transport (`mms.ErrServiceNotAllowed`); the association stays usable. Association control (Initiate, Conclude, Abort) is not a confirmed service.

Defense in depth:

1. The allow-list in the MMS client (`TestGuardRefusesWrite`).
2. `forbidigo` in `.golangci.yml`: state-changing go-iec61850 and go-mms calls and MMS servers are banned outside tests and `internal/simied`.
3. The wire test: `TestWireReadOnly` runs every workflow against the simulated IED, whose server metrics count the services it received, and asserts that each is in the list.

Subscribing to a report control block writes RCB attributes, so report subscription belongs to Commission mode, not Observe mode. Reading an RCB's configuration does not.

## 6. Diagnostics (M0 groundwork)

- **Operations.** Every user action (`connect`, `discover`, `read`, `close`) is published as an `operation` event with its duration, result and the MMS requests it caused (service, invoke ID, duration, error), from go-mms `Metrics` events. The UI shows them in the activity panel.
- **Connect stages.** `internal/engine/classify.go` places a failure in its layer only from the error type: `*net.OpError` (dial) is TCP; `*cotp.RejectionError` is COTP, with reason 3 explained as the T-selector; `*mms.ProtocolError` gives session, presentation, ACSE or MMS phase; `mms.ErrConnectionRejected` is ACSE; `mms.ErrNegotiationFailed` is MMS Initiate. Anything else gets no stage. Response errors map data-access errors, ConfirmedError and Reject.
- **Values.** Structures are named from the variable's type; a 13-bit bit string named `q` is decoded as an IEC 61850-7-3 quality; a UTC time carries its time quality; integers beyond 2^53 cross JSON as strings; NaN and infinities have no JSON value. Every read shows the MMS domain and item ID next to the IEC 61850 reference.

## 7. HTTP API

All `/api/v1` requests need `Authorization: Bearer <token>` and a Host the daemon answers to. Errors: `{"error":{"code","message","detail"}}`, where `detail` is the classified `OpError` of a failed connect or discovery.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/health` | Versions, API version, mode, uptime, engine time |
| `GET /api/v1/sessions`, `POST /api/v1/sessions` | List, open (`{host, port, timeout_ms}`) |
| `GET /api/v1/sessions/{id}`, `DELETE ...` | Get, close |
| `GET /api/v1/sessions/{id}/browse?ref=` | Children of a node; no `ref` lists the logical devices |
| `POST /api/v1/sessions/{id}/read` | `{refs: [...]}`, at most 200 FC-qualified references |
| `GET /api/v1/events?since=<seq>` | SSE stream (`session`, `operation`, `resync`); replay from `since` or `Last-Event-ID` |
| `GET /` and assets | The web UI, unless `--no-ui` |

No endpoint accepts a file path, an MMS PDU, a service name or a value to write.

The daemon's security (token lifecycle, Host check, bind rules, target allow-list, data directory checks) is sunspec-explorer's, unchanged; see `SECURITY.md`.

## 8. Capability audit of the libraries (M0)

What the Explorer's later milestones need, against go-iec61850 v1.9.3, go-mms v1.5.0 and go-iec61850-model v0.6.0. "Gap" lines are the libraries' own documented limitations (go-iec61850 `KNOWN_LIMITATIONS.md`, go-mms `KNOWN_LIMITATIONS.md`, `docs/FEATURE-STATUS.md`) or found while building M0.

| Need | Library state | Consequence |
|---|---|---|
| Read-only enforcement | go-mms v1.5.0 `DialOptions.AllowedServices` (added for M0) | Done |
| Per-request trace of own traffic | go-mms `Metrics` (request and connection events) and client `RawHook` (raw ISO payloads) | Operations in M0; a wire view can use `RawHook` |
| Connect failure stages | Typed errors per layer (COTP rejection with reason, `ProtocolError.Phase`, ACSE rejection, negotiation) | Done for what the types tell; an undifferentiated error gets no stage |
| Self-description, browse, typed reads | `TreeWithOptions{IncludeFCs}`, `ReadRaw`, `GetVariableType`, `ReadMultiple` | M0 reads one reference per request; bulk reads with `ReadMultiple` in M1 |
| Model cache across runs | Gap: in-memory only | M1 snapshots are the Explorer's own versioned files |
| Datasets | `ListDataSets`, `GetDataSet`, `ReadDataSet`; create/delete | M2 dataset explorer; create/delete in Commission mode |
| Reports | RCB read/write, subscription; gap: single-client ownership assumed | M2: ownership-aware, conservative RCB workflows |
| Enhanced-security controls | Gap: the client does not wait for or surface CommandTermination; no time-activated operate | Close in go-iec61850 before advanced controls are offered |
| Setting groups, files, journals | Client APIs present | M2; file and journal reads join `readonly.Services` deliberately |
| SCL | Parser and normalised model; gaps: `Inputs`, `ExtRef`, `ClientLN` not normalised, no XSD validation | M1 import; flow analysis waits for the normalisation |
| GOOSE/SV live capture | Linux and macOS raw Ethernet; gap: Windows `ErrUnsupportedPlatform` | PCAP import first |
| Semantics | go-iec61850-model: NSD catalog, reference resolution, search | M3 semantic view and search |
| Passive MMS trace analysis | No trace engine yet | A separate trace package when M4 is committed |
| TLS | go-mms optional TLS transport | Exposed later with verified configuration, no IEC 62351 claim |

## 9. Milestones

| M | Deliverable | Exit criterion |
|---|---|---|
| M0 | This foundation: engine, Observe mode in the library, daemon and desktop, contract suite, diagnostics groundwork | One service against a simulated IED, identical in desktop and daemon (contract suite) |
| M1 | Read-only browser: connection profiles and selectors, bulk reads, polling, search, favourites, SCL import, snapshots, exports, releases | A user connects, finds meaningful points, monitors and exports them without the CLI |
| M2 | Commission tools: datasets, RCBs, files, journals, controls, setting groups, explicit Commission mode, audit trail | Practical MMS workflows with predictable safety and cleanup |
| M3 | Diagnostics and semantics: SCL/live/snapshot comparison, NSD descriptions, semantic search, integration exports | Introduced mismatches can be diagnosed from visible evidence |
| M4 | Trace analysis: PCAP/PCAPNG, timelines, application-to-wire correlation | A capture is analysed offline, gaps identified |
| M5 | Laboratory: evidence bundles, constrained replay on the go-iec61850 server | Repeatable scenarios; unsupported behaviour is not fabricated |
