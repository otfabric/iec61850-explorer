# Testing

No hardware: Go tests run against `internal/simied`, a simulated IED that serves `internal/simied/demo.icd` on go-iec61850's server over loopback TCP (with a report control block, a log, two setting groups and a small file store) and counts every MMS service it receives. The frontend tests run against an in-memory mock (`frontend/src/app/service/mock.ts`).

## Tiers

| Tier | Command | What | In CI (`.github/workflows/build.yml`) |
|---|---|---|---|
| Go tests with race | `make test` | Every package; the Wails shell needs its platform's libraries | as `make coverage-pkg-check` |
| Coverage gate | `make coverage-pkg-check` | The `TEST_PKGS` in the Makefile, at least `COVERAGE_MIN` (90 %) in total and `COVER_PKG_MIN` (85 %) per package | yes |
| Frontend | `make frontend-check` | Typecheck and Vitest (adapters, the App against the mock) | yes |
| Fuzz | `make fuzz` (`FUZZTIME`, default 30 s per target) | Every `Fuzz*` target in the `TEST_PKGS` | `fuzz` job in `ci.yml`, 10 s per target; `nightly.yml`, 5 min |
| Build checks | workflow steps | The daemon is static, has no Wails, runs without a display; the image runs; the web bundle is free of the Wails runtime | yes |
| Vulnerabilities | `make vuln` | govulncheck | yes |
| SCL corpus | `make corpus` with `IEC61850_SCL_CORPUS` set to a checkout of the private otfabric/iec61850-scl-corpus | `TestSCLCorpusOffline`: every IED with an MMS server of every corpus file opens offline. The full tree is browsed, then a search, data sets, report control blocks and a snapshot export run, each IED within 5 s; unparsable files are refused | no (the corpus is private) |

`make check` runs fmt, fmt-check, vet, lint, lint-ci, vuln, test, coverage-pkg-check and frontend-check.

## What holds the guarantees

- `TestContract` (`internal/api/contract_test.go`): one suite against the engine and against the engine behind the HTTP API, with the simulated IED: health, failed connect with its classified stage, unknown sessions, open and discover, browse, read (float, boolean, unknown object), close, events. M0's exit criterion.
- `TestWireReadOnly` (`internal/engine`): every workflow against the simulated IED; every service the IED received is in `internal/readonly.Services`.
- `TestGuardRefusesWrite` (`internal/engine`): a write through the engine's own MMS client fails with `mms.ErrServiceNotAllowed` and never reaches the IED.
- `TestNoStateChangingService` (`internal/readonly`): the Observe list contains no state-changing service; adding a read service is a deliberate change of this test.
- `TestSimIEDOnlyInTests` (`internal/readonly`) and `TestNoWailsNoCgo` (`cmd/iec61850-explorerd`): `go list` checks that neither program depends on the simulated IED, and that the daemon links no Wails and nothing that needs cgo.
- `.golangci.yml` (forbidigo) bans state-changing go-iec61850 and go-mms calls and MMS servers outside `_test.go` files, `internal/simied` and the engine's `commission*.go` files.
- `TestConnectRefused`, `TestConnectNotMMS`, `TestClassify` (`internal/engine`): a failed connect names its layer only from the error type.
- `TestSessionLost`: an IED that goes away makes the session `lost`; reads are refused, the tree stays browsable.
- `TestReadBatched` (`internal/engine`): several references go out as one MMS Read per batch of 32; a logical-node read is not batched.
- `TestPollingLifecycle`, `TestPollingOverruns`: polling replaces, counts failures and missed starts, and stops with a lost session.
- `TestOpenSCLMatchesDiscovery`: the data model built from the simulated IED's SCL file equals its live discovery, node for node, with the same FCs.
- `TestSnapshotRoundTrip`, `TestExportCSV`: a snapshot reopens with the same tree and answers reads from its values; CSV has one row per leaf value and no spreadsheet formulas.
- `TestDataSetsLiveMatchSCL`, `TestReportsLiveMatchSCL`: the data sets and report control blocks the simulated IED reports are those its SCL defines; the SCL ones claim no IED state.
- `TestFilesJournalsSettingGroups`: file listing and download, log entries in a time range, the setting group control block, and only allowed services on the wire.
- `TestCommissionWriteVerified`, `TestCommissionCapabilities`, `TestCommissionEnds`, `TestCommissionRefused`, `TestAuditAcrossRuns` (`internal/engine`): a grant opens a second association; a write is verified by readback and audited; the Observe association still refuses writes; a grant without `write`, an offline session, an engine without Commission mode or an unwritable audit trail refuse; expiry, close, loss and cleanup end the grant and are audited; the audit sequence continues across runs.
- `TestReportSubscription`, `TestReportSubscriptionRefused`, `TestReportSubscriptionLost`, `TestCreateDeleteDataSet`, `TestDeleteDataSetInUse`, `TestSelectActiveSG` (`internal/engine`): GI and data-change reports named by the data set's members, unbuffered and buffered RCBs, reserve/enable on subscribe and disable/release on unsubscribe and revoke; RCBs another client enabled or reserved are refused; a lost association audits the failed cleanup; data sets are created, read back and deleted, configured ones refused; the active setting group is selected and read back.
- `TestControlModels`, `TestControlNegativeTermination`, `TestControlRefused`, `TestControlInvalid` (`internal/engine`): the sequence of each control model against the simulated IED's GGIO1.SPCSO1 to SPCSO4; XCBR1.Pos ends with a negative termination (blocked by interlocking) while GGIO1.Ind1 is false; a select another connection holds is refused with object-already-selected; one audit record per control. `TestObserveNeverReadsControls` and `TestSBOReadSelects`: Observe mode never reads FC CO.
- `TestControlCheckFlags` (`internal/engine`): the simulated IED checks the interlocking only when asked (an interlock check), and refuses a synchrocheck (blocked by synchrocheck).
- `TestContractM2b3` (`internal/api`): controls through the engine and the HTTP API.
- `TestContractM2b2` (`internal/api`): subscription with its reports as events, data sets and setting groups through the engine and the HTTP API.
- `TestServicesFollowCapabilities` (`internal/commission`): each capability adds only the services it needs; none adds file changes.
- `TestContractCommission` (`internal/api`): grant, write, audit and revoke behave the same through the engine and the HTTP API, and are refused where Commission mode is not offered.
- `TestApplyAddressing`, `TestProfilesStore`, `TestProfileEdges`: ISO addressing is validated before dialling; profiles are stored 0600 and atomically.
- `TestTraceGapsOfSimIED` (`internal/engine`), M4's exit test: a test-only TCP relay (`tracecapture_test.go`) records the Explorer reading the simulated IED, and a test-only writer turns the recording into a classic pcap file (Ethernet, IPv4, TCP with consistent sequence numbers) with the segment that carries a Read response left out. Opened as a trace and linked to a live session of the same IED: the association shows a TCP gap, the `tcp-gap` and `unanswered` findings name their association, frame, time and source, the unanswered Read's objects resolve to `DemoIEDLD0/MMXU1.Hz[MX]` (its data object and data set) and `LLN0.Mod.stVal[ST]`, and a reference the IED does not have is a `not-in-model` finding.
- `TestTraceReportsDecoded` (`internal/engine`): reports captured from a Commission subscription (through the relay) are decoded against the members of the `Events` data set, linked to the SCL file and to the live IED; raw, with the reason, without a link and against a snapshot.
- `TestTraceFixtures`, `TestTraceDamagedAndInvalid`, `TestTraceExports` (`internal/engine`): the captures of `testdata/traces` (see its `NOTICE.md`): statuses, findings, filters and paging, GOOSE and SV, MMS on port 1102 found only when named, a capture captured midstream, `ErrCorrupt` (opens, with a finding) and `ErrFormat` (`invalid_argument`), gzip, the size and session limits; exports with formula-protected cells.
- `TestWireView`, `TestWireRing` (`internal/engine`): an operation's `wire` number returns its units, decoded with go-mms, with the AARQ of the connect and the Read request and response matched by invoke ID; poll cycles keep none; the ring's bounds.
- `TestReportOf`, `TestObjectOf`, `TestGooseModel`, `TestCaptureFindings` (`internal/engine`): a report decoded against its data set and its malformed forms (go-iec61850 `DecodeReport`), the mapping of MMS names, GOOSE control block matching, a finding for every counter, status and issue.
- `TestTraceContract`, `TestTraceHTTP` (`internal/api`): every trace method and the wire view through the engine and the HTTP API.
- `frontend/src/app/Trace.test.tsx`: the trace tab (summary, findings, timeline filter, the correlation picker) and the wire view of the activity panel, against the mock.
- `TestAuthAndHost`, `TestHostCheck`, `TestEventsResume`, `TestEventsEndWhenTokenRotates`, `FuzzRequest` (`internal/api`): token, Host check, SSE replay and resync, rotation.

## Fuzz targets

| Package | Target | Input |
|---|---|---|
| `internal/api` | `FuzzRequest` | Method, path, Host, Authorization and body of a request |
| `internal/daemon` | `FuzzReadToken`, `FuzzReadState` | The token file and the state file |
| | `FuzzListenAddress`, `FuzzTargetAllowList` | The flags `--listen`, `--allow-target` |
| `internal/web` | `FuzzHandlerPath` | A path asked of the web UI |

go-mms, go-iec61850 and go-mms-trace fuzz their own decoders (go-iec61850: `FuzzDecodeReport`; go-mms-trace: the capture reader and the pipeline). A failing input lands in `testdata/fuzz/<target>/` of its package: commit it with the fix.

## Coverage policy

`TEST_PKGS` is the engine, the daemon and what they are built from. `cmd/iec61850-explorer` and `internal/gui` are excluded: the Wails shell needs a window. `codecov.yml` ignores `cmd/**`, `internal/gui/**` and `frontend/**`.

## Skips

`go test -short` skips the tests that run `go list`. File-mode tests skip on Windows.

## Large models in the user interface

Measured on 2026-11-09 with the corpus's largest file (an ABB REX640 CID, 2.8 MB: 3 logical devices, 299 logical nodes, 4,502 data objects), daemon and web UI, in a browser:
- **Opening the file:** 82 ms in the daemon.
- **Expanding all logical devices, or searching the whole model:** no main-thread task over 50 ms.
- **Expanding everything at once:** before v0.7.0, expanding all 299 logical nodes and then all 4,502 data objects (about 63,000 DOM nodes) caused one task of 2.2 s. Since v0.7.0 the tree is virtualized: only the rows in view, plus a margin, are in the DOM. With every level expanded (34,465 rows), the page holds about 230 elements, and no main-thread task exceeds 50 ms while expanding. Collapsing or expanding a logical device with 1,349 rows below it takes about 10 ms; jumping to a random scroll position re-renders in about 17 ms.

`frontend/src/app/components/DeviceTree.test.tsx` covers the flattening, the windowing on a 15,001-row tree, the ARIA attributes, and scrolling a row selected through search into view.

Conformance over the corpus (170 IEDs, 2026-11-09, no namespace packages loaded): logical nodes declare IEC 61850-7-4 releases from 2003 to 2007B4, ABB's own namespaces (`REx615:2014`, `REx620:2015`, `1MRS189725`) and others (`IEC 62271-3:2006`); 68 logical devices are Edition 1 (declared, or by the SCL schema). Findings: 338 missing mandatory data objects (errors) and 108 for Edition 1 (warnings), 490 classes and about 25,700 data objects the declared namespace does not define (most of them ABB extension data objects in standard logical nodes, which ABB's namespace package would define), 47 declared namespaces that are not loaded, 503 CDC differences (3,165 more for Edition 1, as information).

The engine handles the whole corpus easily (`make corpus`): 170 IED sessions and about 537,000 nodes. Each IED is also checked for conformance, exported as a point list, and opened a second time and compared with the first, values included (no differences); the slowest session (34,465 nodes) takes about 440 ms with all of that.
