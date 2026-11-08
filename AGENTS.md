# OT Fabric IEC 61850 Explorer: agent handoff

Status (2026-11-08): **M2a (inspection) is implemented**, on top of M1 (the read-only browser, released as v0.1.0) and M0 (the foundation). It shows a logical device's data sets, report control blocks, logs and setting groups and the IED's files, from the IED or, for an offline session, from the SCL file. One Go engine behind a `Service` interface runs in the desktop application (Wails, in-process) and in the daemon (`iec61850-explorerd`, HTTP/SSE plus the embedded web UI). It connects with configurable ISO addressing, discovers, browses, searches, reads (batched) and polls IEDs, imports SCL files and snapshots as offline sessions, exports snapshots and CSV, and keeps connection profiles, all in **Observe mode**: the MMS client refuses every state-changing service before it is sent. The contract suite proves the same behaviour through the engine and through the HTTP API against a simulated IED. Read this file fully, then `docs/ARCHITECTURE.md`, before writing code.

## Organisation standards

This repository follows **otfabric/engineering policy 1.2**, profile **application** (`.otfabric/policy.yaml`); local exceptions are in `.otfabric/exceptions.yaml`. Where this file and the organisation standards disagree, this file wins for this repository; say so in your report.

## What this is

A cross-platform IEC 61850 workbench (Wails v3 + React/TypeScript) on [otfabric/go-iec61850](https://github.com/otfabric/go-iec61850) and [otfabric/go-mms](https://github.com/otfabric/go-mms). The milestones after M1 (`docs/ARCHITECTURE.md` section 9) add datasets and reports, explicit Commission mode, diagnostics, semantics from [go-iec61850-model](https://github.com/otfabric/go-iec61850-model), trace analysis and replay.

## Read these, in this order

1. `docs/ARCHITECTURE.md`: artifacts, layering, the `Service` interface, the API, Observe mode, security, the capability audit of the libraries, milestones.
2. `docs/adr/`: the foundational decisions and why.
3. `TESTING.md`: the test tiers. `SECURITY.md`: what Observe mode guarantees.
4. `docs/BRANDING.md` of [sunspec-explorer](https://github.com/otfabric/sunspec-explorer): the shared visual identity; the tokens are in `frontend/src/styles/tokens.css`.

## Decisions that are settled (do not relitigate without asking the user)

- **The architecture of sunspec-explorer is reused** (ADR 0001): two artifacts from one module, `iec61850-explorer` (Wails GUI, cgo, Local mode only) and `iec61850-explorerd` (static daemon, `CGO_ENABLED=0`, linux/amd64, arm64, armv7). The daemon never imports Wails (`TestNoWailsNoCgo`).
- **One `Service` interface, one Go implementation** (the engine). The Wails binding and the HTTP handlers wrap it; the contract suite (`internal/api/contract_test.go`) runs against both. The TypeScript `Service` has two implementations (Wails bindings, `fetch` to `/api/v1`).
- **Remote = daemon + user-managed SSH port forward + browser.** No GUI remote mode, no Go client of the daemon, no token storage in the GUI (ADR 0003).
- **Observe mode is enforced in the library, not implied by the UI** (ADR 0002): every MMS client is created with go-mms `DialOptions.AllowedServices` = `internal/readonly.Services` (go-mms v1.5.0 or later; the option was added for this project), the linter bans state-changing calls and MMS servers outside tests, and the wire test counts what the IED receives. Commission and Laboratory modes are later milestones; they will be per-session capability grants enforced in the engine, never hidden buttons.
- **The CLI is not the backend.** The engine uses the libraries directly; `iec61850ctl` is a reference for workflows (the UI offers the equivalent `iec61850ctl` command).
- **Daemon is authenticated even on loopback**: bearer token in a 0600 file, rotatable; Host header check; target allow-list (default: private and link-local ranges); loopback bind unless `--allow-remote-bind`.
- **CI and release builds run on native-architecture runners, never under QEMU.** One reusable workflow (`build.yml`) holds all checks and builds; `ci.yml` and `release.yml` call it.
- **Releases are unsigned for now**: SHA-256 checksums and build provenance.
- **Branding**: dark theme by default, OT Fabric tokens, the IEC 61850 icon of the website (`frontend/src/assets/iec61850.svg`, `packaging/icons/render_appicon.py`), fonts bundled (the app works offline).

## Hard rules

- No state-changing MMS or IEC 61850 call outside `_test.go` files and `internal/simied` (`.golangci.yml` forbidigo). A new read service is added to `internal/readonly.Services` and its test deliberately.
- `internal/simied` (the simulated IED) is imported by tests only (`TestSimIEDOnlyInTests`).
- No cgo in `internal/` except what the Wails GUI strictly needs (`internal/gui`, `cmd/iec61850-explorer`).
- Never log the bearer token. The web UI receives it only in the URL fragment.
- The engine never opens a path a client names: SCL files and snapshots arrive as bytes the user interface read.
- An SCL-derived model claims nothing the SCL does not say: no invented control-block attributes.
- A diagnosis names a cause only when an error type proves it (`internal/engine/classify.go`); never infer a stage from a message text.
- Pin the Wails v3 beta version in `go.mod`.
- Do not commit unless the user asks. When committing, follow the user's commit conventions.

## Definition of done for any change

`make check` passes (format, vet, staticcheck, golangci-lint with the Observe-mode ban, govulncheck, race tests, coverage gate 90 % total and 85 % per package, frontend typecheck and tests); new behaviour has a test against `internal/simied`; no new state-changing call path; UI changes use tokens, not hard-coded colours; both themes checked.
