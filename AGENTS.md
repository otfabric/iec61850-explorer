# OT Fabric IEC 61850 Explorer: agent handoff

Status (2026-11-10): **M4 (trace analysis) is implemented** (v0.10.0, ADR 0005): captures (pcap, pcapng, plain or gzip, at most 256 MiB, as bytes) open as trace sessions analysed offline by [go-mms-trace](https://github.com/otfabric/go-mms-trace) v0.1.1, with a summary, associations, a filtered timeline, reports, GOOSE and SV streams, and findings that name the go-mms-trace field they come from; a trace links to a live, SCL or snapshot session, which resolves MMS names to IEC 61850 references and decodes captured reports against data sets; findings and the timeline export as CSV and JSON; every operation of a live session links to its MMS units (go-mms `RawHook`, passive; `mms.DecodeTSDU`), the wire view. M5 (laboratory) follows. v0.9.0 added proxy support: IEDs are reached through SOCKS5 (`socks5`, `socks5h`) or HTTP CONNECT proxies with optional username and password (`internal/proxy`, go-iec61850 v1.15 `DialOptions.Dialer`), per target or by default (a setting, or the daemon's `--proxy`/`ALL_PROXY`, which users cannot change); the daemon's allow-list is checked when connecting and applies to users' proxies. **M3 is implemented** (v0.8.0): the namespace view, conformance check (against what each logical device or node declares, Edition 1 marked, user and vendor NSD packages) and search by quantity from go-iec61850-model (M3c), point-list exports (M3d), and the comparison of configured values (M3b, FC CF, DC, SP, SG; settings per group). M3a (v0.7.0): any two sessions (IED, SCL, snapshot) compare their model, data sets and report control blocks, read-only. Before that, M2b3: controls of every control model in Commission mode, with the CommandTermination and the IED's reasons audited, and Observe mode never reads the functional constraint CO. Before that, M2b2 (v0.4.0): report subscription, data set creation and deletion, active setting group selection. M2b1 (v0.3.0): Commission mode, a per-session grant on a second, restricted association, an audit trail, writes verified by readback (ADR 0004). Before it: M2a (inspection, v0.2.0), M1 (the read-only browser, v0.1.0) and M0 (the foundation). It shows a logical device's data sets, report control blocks, logs and setting groups and the IED's files, from the IED or, for an offline session, from the SCL file. One Go engine behind a `Service` interface runs in the desktop application (Wails, in-process) and in the daemon (`iec61850-explorerd`, HTTP/SSE plus the embedded web UI). It connects with configurable ISO addressing, discovers, browses, searches, reads (batched) and polls IEDs, imports SCL files and snapshots as offline sessions, exports snapshots and CSV, and keeps connection profiles, by default in **Observe mode**: the MMS client refuses every state-changing service before it is sent. Commission mode is a granted, audited second association. The contract suite proves the same behaviour through the engine and through the HTTP API against a simulated IED. Read this file fully, then `docs/ARCHITECTURE.md`, before writing code.

## Organisation standards

This repository follows **otfabric/engineering policy 1.2**, profile **application** (`.otfabric/policy.yaml`); local exceptions are in `.otfabric/exceptions.yaml`. Where this file and the organisation standards disagree, this file wins for this repository; say so in your report.

## What this is

A cross-platform IEC 61850 workbench (Wails v3 + React/TypeScript) on [otfabric/go-iec61850](https://github.com/otfabric/go-iec61850), [otfabric/go-mms](https://github.com/otfabric/go-mms), [go-iec61850-model](https://github.com/otfabric/go-iec61850-model) (semantics) and [go-mms-trace](https://github.com/otfabric/go-mms-trace) (captures). The next milestone (`docs/ARCHITECTURE.md` section 9) is M5, the laboratory: evidence bundles and constrained replay.

## Read these, in this order

1. `docs/ARCHITECTURE.md`: artifacts, layering, the `Service` interface, the API, Observe mode, security, the capability audit of the libraries, milestones.
2. `docs/adr/`: the foundational decisions and why.
3. `TESTING.md`: the test tiers. `SECURITY.md`: what Observe mode and Commission mode guarantee.
4. `docs/BRANDING.md` of [sunspec-explorer](https://github.com/otfabric/sunspec-explorer): the shared visual identity; the tokens are in `frontend/src/styles/tokens.css`.

## Decisions that are settled (do not relitigate without asking the user)

- **The architecture of sunspec-explorer is reused** (ADR 0001): two artifacts from one module, `iec61850-explorer` (Wails GUI, cgo, Local mode only) and `iec61850-explorerd` (static daemon, `CGO_ENABLED=0`, linux/amd64, arm64, armv7). The daemon never imports Wails (`TestNoWailsNoCgo`).
- **One `Service` interface, one Go implementation** (the engine). The Wails binding and the HTTP handlers wrap it; the contract suite (`internal/api/contract_test.go`) runs against both. The TypeScript `Service` has two implementations (Wails bindings, `fetch` to `/api/v1`).
- **Remote = daemon + user-managed SSH port forward + browser.** No GUI remote mode, no Go client of the daemon, no token storage in the GUI (ADR 0003).
- **Observe mode is enforced in the library, not implied by the UI** (ADR 0002): every MMS client is created with go-mms `DialOptions.AllowedServices` = `internal/readonly.Services` (go-mms v1.5.0 or later; the option was added for this project), the linter bans state-changing calls and MMS servers outside tests, and the wire test counts what the IED receives. Commission mode (ADR 0004) is a per-session capability grant enforced in the engine on a second association, never a hidden button; Laboratory mode will follow the same pattern.
- **The CLI is not the backend.** The engine uses the libraries directly; `iec61850ctl` is a reference for workflows (the UI offers the equivalent `iec61850ctl` command).
- **Daemon is authenticated even on loopback**: bearer token in a 0600 file, rotatable; Host header check; target allow-list (default: private and link-local ranges); loopback bind unless `--allow-remote-bind`.
- **CI and release builds run on native-architecture runners, never under QEMU.** One reusable workflow (`build.yml`) holds all checks and builds; `ci.yml` and `release.yml` call it.
- **Releases are unsigned for now**: SHA-256 checksums and build provenance.
- **Branding**: dark theme by default, OT Fabric tokens, the IEC 61850 icon of the website (`frontend/src/assets/iec61850.svg`, `packaging/icons/render_appicon.py`), fonts bundled (the app works offline).

## Hard rules

- No state-changing MMS or IEC 61850 call outside `_test.go` files, `internal/simied` and the engine's `commission*.go` files (`.golangci.yml` forbidigo). A new read service is added to `internal/readonly.Services` and its test deliberately; a new Commission service to `internal/commission` under a capability, with its test.
- A Commission change goes through the grant's association only, after `requireGrant(capability)`, and is audited (ADR 0004). No grant without a writable audit trail.
- `internal/simied` (the simulated IED) is imported by tests only (`TestSimIEDOnlyInTests`).
- No cgo in `internal/` except what the Wails GUI strictly needs (`internal/gui`, `cmd/iec61850-explorer`).
- Never log the bearer token. The web UI receives it only in the URL fragment.
- The engine never opens a path a client names: SCL files, snapshots and captures arrive as bytes the user interface read.
- The engine decodes no protocol itself: captures are go-mms-trace's, PDUs go-mms's (`DecodeTSDU`), the fields of a captured IEC 61850 report go-iec61850's (`DecodeReport`, v1.16.0). A decoding gap is reported to those libraries, not patched around. Raw MMS units are never logged.
- An SCL-derived model claims nothing the SCL does not say: no invented control-block attributes.
- A diagnosis names a cause only when an error type proves it (`internal/engine/classify.go`); never infer a stage from a message text.
- Pin the Wails v3 beta version in `go.mod`.
- Do not commit unless the user asks. When committing, follow the user's commit conventions.

## Definition of done for any change

`make check` passes (format, vet, staticcheck, golangci-lint with the Observe-mode ban, govulncheck, race tests, coverage gate 90 % total and 85 % per package, frontend typecheck and tests); new behaviour has a test against `internal/simied`; no new state-changing call path; UI changes use tokens, not hard-coded colours; both themes checked.
