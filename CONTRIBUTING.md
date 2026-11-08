# Contributing to iec61850-explorer

Thank you for your interest in contributing. Read [`AGENTS.md`](AGENTS.md) first: it lists the settled decisions and the hard rules, and points to the requirements and the architecture in [`docs/`](docs/).

## Development Setup

### Prerequisites

- Go 1.26+ (see [go.mod](go.mod)); Wails v3 needs it. CI and releases build with the current stable Go, so that the binaries carry its security fixes; `go.mod` names the oldest version that works
- Node.js 22+ and npm for the frontend
- [staticcheck](https://staticcheck.dev/), [golangci-lint](https://golangci-lint.run/) v2 and [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) for the checks
- For the desktop application: a C compiler and the platform's webview (macOS: Xcode command line tools; Windows: WebView2 runtime; Linux: GTK4 and WebKitGTK 6.0)
- No hardware: tests run against `internal/simied`, a simulated IED on go-iec61850's server

### Build & Test

```sh
make frontend-install   # once: npm ci
make frontend           # build the UI twice: frontend/dist/wails and frontend/dist/web
make check              # format, vet, lint, vulnerability scan, race tests, coverage gate, frontend checks
make build              # ./bin/iec61850-explorer and ./bin/iec61850-explorerd
make dev                # run the desktop application from source
```

The Go code compiles and tests without a frontend build: a fresh checkout embeds placeholders. A binary needs `make frontend` first.

Run `make` (or `make help`) for the full list of targets.

### Trying it by hand

`iec61850ctl` serves an IED from an SCL file:

```sh
iec61850ctl server start --scl device.cid --host 127.0.0.1 --port 10102
```

Connect the desktop application (`make dev`) to `127.0.0.1`, port `10102`. For the daemon, allow loopback targets, which are off by default:

```sh
./bin/iec61850-explorerd --allow-target 127.0.0.1
./bin/iec61850-explorerd url
```

The program icon is generated: `make appicon` renders `cmd/iec61850-explorer/appicon.png`, and `make winres` builds the Windows resource files (`rsrc_windows_*.syso`: icon and manifest) from it. Both results are committed, so a normal build needs neither.

This is for hands-on checks only: the automated tests use `internal/simied` and must not depend on the `iec61850ctl` binary.

## Making Changes

### Hard rules

- **Observe mode.** No state-changing MMS or IEC 61850 call, and no MMS server outside `_test.go` files and `internal/simied`. The linter, the allow-list in the MMS client (`internal/readonly`) and the wire test enforce it; do not weaken any of the three.
- **The daemon never imports Wails**, and `internal/` uses no cgo outside `internal/gui`.
- **The user interface talks to the `Service` interface only** (`internal/service` in Go, `frontend/src/app/service` in TypeScript), never to MMS or HTTP directly.
- **Diagnostics are evidence.** A stage or a cause is shown only when an error type proves it (`internal/engine/classify.go`).
- **Colors come from the tokens** in `frontend/src/styles/tokens.css` (the OT Fabric tokens, shared with sunspec-explorer); check both themes.

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`); start every Go file with `// SPDX-License-Identifier: MIT`
- Comments end with a period (enforced by the linter)
- `make fmt` formats with the `gofmt` of the Go version in `go.mod`, the version CI uses

### Where a change belongs

The MMS services, the IEC 61850 mapping, SCL and the transports live in [go-iec61850](https://github.com/otfabric/go-iec61850), [go-mms](https://github.com/otfabric/go-mms) and [go-cotp](https://github.com/otfabric/go-cotp). A wrongly decoded value or a missing service is fixed there, with its own tests and interop evidence. This repository holds the engine (sessions, discovery, reads, diagnostics, events), the daemon and the user interface.

### Tests

New behaviour needs a test against the simulated IED (`internal/simied`). API behaviour is tested through plain `net/http` requests (`internal/api/client_test.go`); there is no Go client package and none should be added. The coverage gate is 90 % of the engine and daemon packages in total and 85 % per package.

## Releasing

Releases are made by the **Release** workflow (Actions tab, run by hand), never from a workstation. It follows the conventions of the other otfabric repositories.

1. Write the release notes at the top of [`RELEASE.md`](RELEASE.md): a `## v<version>` heading, the notes, and a line with only `---` after them. The workflow publishes everything between the heading and that line. Without such a section it publishes the list of commits since the previous tag.
2. Merge to `main` and wait for CI: it runs the same checks and builds as a release (`.github/workflows/build.yml`).
3. Run **Release** on `main` and choose the bump:
   - `patch`, `minor` or `major`: the new version is the latest `v*` tag plus the bump (`patch` makes v0.0.1 when there is no tag yet). The workflow builds everything, pushes the annotated tag `v<version>` and publishes the GitHub release.
   - `none`: rebuilds the files of the latest existing tag and replaces them in its release.

The workflow builds every deliverable on a runner of its own architecture (no QEMU; the armv7 daemon and image are Go cross-compilation on the arm64 runner), writes `iec61850-explorer-<version>-checksums.txt`, attests build provenance, and pushes the container image to `ghcr.io/otfabric/iec61850-explorerd` (`<version>` and `latest`). A tag, once pushed, is not moved: a release that went wrong is followed by the next patch.

## License

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE).
