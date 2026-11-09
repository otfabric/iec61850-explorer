# OT Fabric IEC 61850 Explorer

[![CI](https://github.com/otfabric/iec61850-explorer/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/iec61850-explorer/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Connect to an IEC 61850 IED, see the model it exposes, and read its values with their quality and time, without changing anything on the device.**

A cross-platform workbench for IEC 61850 over MMS, built by OT Fabric on [go-iec61850](https://github.com/otfabric/go-iec61850) and [go-mms](https://github.com/otfabric/go-mms). MIT licensed.

**Status: M2a, inspection.** Releases: v0.1.0 (the read-only browser), v0.2.0 (inspection). What comes after it is in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), section 9.

## Two ways to run it

| | Desktop application | Daemon + browser |
|---|---|---|
| Program | `iec61850-explorer` (Windows, macOS, Linux) | `iec61850-explorerd` (static Linux binary: amd64, arm64, armv7; container image) |
| Use it when | The IED is on your network | The IED is reachable only from a gateway or an engineering host |
| How | Start the application | Run the daemon there, forward its port with SSH, open the URL it prints in a browser |

Both run the same engine and show the same interface. There is no "remote mode" in the desktop application.

## What it does

- **Connect** to an IED (host, port 102 by default) with the ISO addressing it expects: T/S/P selectors, AP-title and AE-qualifier, as in its SCL Address. Save connections as **profiles**.
- **Discover** its model (MMS Identify, then logical devices, logical nodes, data objects and attributes with their functional constraints) and **browse** it in a tree, or **search** it by reference, LN class (`MMXU`, `XCBR`, ...) and FC.
- **Read** a node under one of its functional constraints. Structures show their component names; a quality is decoded (`good`, `questionable (old-data)`, ...); a timestamp shows its time quality. Every value shows the MMS domain and item ID it was read from, and the equivalent `iec61850ctl get object` command can be copied.
- **Watch** points from several IEDs in one table and **poll** them (0.5 s to 1 min); the engine polls, so a daemon keeps polling while the browser is away.
- **Open an SCL file** (SCD, CID, ICD, IID): browse an IED's data model offline, or fill in the connection with the address the file gives it.
- **Inspect** what a logical device defines: data sets (read in one request, or pinned to the watch table), report control blocks with their configuration, logs by time range, setting groups; and the IED's **files** (download disturbance records). From an SCL file, data sets and report control blocks are shown as the file defines them.
- **Export** a snapshot (the model and the watched values, JSON) or the watched values as CSV; **open a snapshot** again later, without the IED.
- **See what happened on the wire**: every action (connect, discover, read, close) is logged with its duration and the MMS requests it sent. A failed connect says where it stopped (TCP, COTP, session, presentation, ACSE or MMS Initiate) when the error proves it, and what to check.
- **Observe mode** by default: the MMS client is built so that it cannot send a Write, a control, a report or data set change, or a file change. See [SECURITY.md](SECURITY.md).
- **Commission mode** on request, per session: a grant with a reason and a duration opens a second, restricted association; **write** set points and configuration values (FC SP, CF, DC, SV, BL), each verified by reading it back and recorded in an audit trail. The daemon offers it only with `--allow-commission`.

Not yet: report subscription, data set and setting group changes, controls, comparison of SCL with the live model, semantic search, trace analysis.

## Build from source

```bash
make frontend-install
```

```bash
make build
```

This builds `bin/iec61850-explorer` (needs a C compiler and the platform's webview; see [CONTRIBUTING.md](CONTRIBUTING.md)) and `bin/iec61850-explorerd`.

## Try it without hardware

`iec61850ctl` serves an IED from an SCL file:

```bash
iec61850ctl server start --scl device.cid --host 127.0.0.1 --port 10102
```

Then start the desktop application (`make dev`) and connect to `127.0.0.1`, port `10102`. With the daemon, allow the loopback target:

```bash
bin/iec61850-explorerd --allow-target 127.0.0.1
```

and open the URL that `bin/iec61850-explorerd url` prints.

## Remote use

On the gateway:

```bash
iec61850-explorerd
```

On the workstation, forward the port and open the URL that `iec61850-explorerd url` prints on the gateway (the part after `#` is the access token: treat the URL like a password):

```bash
ssh -N -L 8765:127.0.0.1:8765 user@gateway
```

The daemon listens on loopback, requires its token even there, checks the Host header and connects only to private and link-local addresses unless `--allow-target` says otherwise. `iec61850-explorerd token rotate` replaces the token. More in [docs/REMOTE-USE.md](docs/REMOTE-USE.md); a systemd unit is in [`packaging/`](packaging/).

## Documentation

- [Architecture](docs/ARCHITECTURE.md): layering, the `Service` interface, the HTTP API, Observe mode, the capability audit of the libraries, milestones
- [Decisions](docs/adr/)
- [Security](SECURITY.md), [Testing](TESTING.md), [Contributing](CONTRIBUTING.md)

## License

MIT, see [LICENSE](LICENSE).
