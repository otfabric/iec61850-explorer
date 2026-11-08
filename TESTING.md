# Testing

No hardware: Go tests run against `internal/simied`, a simulated IED that serves `internal/simied/demo.icd` on go-iec61850's server over loopback TCP and counts every MMS service it receives. The frontend tests run against an in-memory mock (`frontend/src/app/service/mock.ts`).

## Tiers

| Tier | Command | What | In CI (`.github/workflows/build.yml`) |
|---|---|---|---|
| Go tests with race | `make test` | Every package; the Wails shell needs its platform's libraries | as `make coverage-pkg-check` |
| Coverage gate | `make coverage-pkg-check` | The `TEST_PKGS` in the Makefile, at least `COVERAGE_MIN` (90 %) in total and `COVER_PKG_MIN` (85 %) per package | yes |
| Frontend | `make frontend-check` | Typecheck and Vitest (adapters, the App against the mock) | yes |
| Fuzz | `make fuzz` (`FUZZTIME`, default 30 s per target) | Every `Fuzz*` target in the `TEST_PKGS` | `fuzz` job in `ci.yml`, 10 s per target; `nightly.yml`, 5 min |
| Build checks | workflow steps | The daemon is static, has no Wails, runs without a display; the image runs; the web bundle is free of the Wails runtime | yes |
| Vulnerabilities | `make vuln` | govulncheck | yes |

`make check` runs fmt, fmt-check, vet, lint, lint-ci, vuln, test, coverage-pkg-check and frontend-check.

## What holds the guarantees

- `TestContract` (`internal/api/contract_test.go`): one suite against the engine and against the engine behind the HTTP API, with the simulated IED: health, failed connect with its classified stage, unknown sessions, open and discover, browse, read (float, boolean, unknown object), close, events. M0's exit criterion.
- `TestWireReadOnly` (`internal/engine`): every workflow against the simulated IED; every service the IED received is in `internal/readonly.Services`.
- `TestGuardRefusesWrite` (`internal/engine`): a write through the engine's own MMS client fails with `mms.ErrServiceNotAllowed` and never reaches the IED.
- `TestNoStateChangingService` (`internal/readonly`): the Observe list contains no state-changing service; adding a read service is a deliberate change of this test.
- `TestSimIEDOnlyInTests` (`internal/readonly`) and `TestNoWailsNoCgo` (`cmd/iec61850-explorerd`): `go list` checks that neither program depends on the simulated IED, and that the daemon links no Wails and nothing that needs cgo.
- `.golangci.yml` (forbidigo) bans state-changing go-iec61850 and go-mms calls and MMS servers outside `_test.go` files and `internal/simied`.
- `TestConnectRefused`, `TestConnectNotMMS`, `TestClassify` (`internal/engine`): a failed connect names its layer only from the error type.
- `TestSessionLost`: an IED that goes away makes the session `lost`; reads are refused, the tree stays browsable.
- `TestReadBatched` (`internal/engine`): several references go out as one MMS Read per batch of 32; a logical-node read is not batched.
- `TestPollingLifecycle`, `TestPollingOverruns`: polling replaces, counts failures and missed starts, and stops with a lost session.
- `TestOpenSCLMatchesDiscovery`: the data model built from the simulated IED's SCL file equals its live discovery, node for node, with the same FCs.
- `TestSnapshotRoundTrip`, `TestExportCSV`: a snapshot reopens with the same tree and answers reads from its values; CSV has one row per leaf value and no spreadsheet formulas.
- `TestApplyAddressing`, `TestProfilesStore`, `TestProfileEdges`: ISO addressing is validated before dialling; profiles are stored 0600 and atomically.
- `TestAuthAndHost`, `TestHostCheck`, `TestEventsResume`, `TestEventsEndWhenTokenRotates`, `FuzzRequest` (`internal/api`): token, Host check, SSE replay and resync, rotation.

## Fuzz targets

| Package | Target | Input |
|---|---|---|
| `internal/api` | `FuzzRequest` | Method, path, Host, Authorization and body of a request |
| `internal/daemon` | `FuzzReadToken`, `FuzzReadState` | The token file and the state file |
| | `FuzzListenAddress`, `FuzzTargetAllowList` | The flags `--listen`, `--allow-target` |
| `internal/web` | `FuzzHandlerPath` | A path asked of the web UI |

go-mms and go-iec61850 fuzz their own decoders. A failing input lands in `testdata/fuzz/<target>/` of its package: commit it with the fix.

## Coverage policy

`TEST_PKGS` is the engine, the daemon and what they are built from. `cmd/iec61850-explorer` and `internal/gui` are excluded: the Wails shell needs a window. `codecov.yml` ignores `cmd/**`, `internal/gui/**` and `frontend/**`.

## Skips

`go test -short` skips the tests that run `go list`. File-mode tests skip on Windows.
