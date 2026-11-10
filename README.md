# OT Fabric IEC 61850 Explorer

[![CI](https://github.com/otfabric/iec61850-explorer/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/iec61850-explorer/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Connect to an IEC 61850 IED, see the model it exposes, and read its values with their quality and time, without changing anything on the device.**

A cross-platform workbench for IEC 61850 over MMS, built by OT Fabric on [go-iec61850](https://github.com/otfabric/go-iec61850) and [go-mms](https://github.com/otfabric/go-mms). MIT licensed.

**Status: M4, trace analysis** (v0.10.0). Earlier milestones: the read-only browser (M1), inspection (M2a), Commission mode (M2b), comparison, semantics and point lists (M3). Release notes are in [RELEASE.md](RELEASE.md); what comes next (M5, the laboratory) is in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), section 9.

## Two ways to run it

| | Desktop application | Daemon + browser |
|---|---|---|
| Program | `iec61850-explorer` (Windows, macOS, Linux) | `iec61850-explorerd` (static Linux binary: amd64, arm64, armv7; container image) |
| Use it when | The IED is on your network | The IED is reachable only from a gateway or an engineering host |
| How | Start the application | Run the daemon there, forward its port with SSH, open the URL it prints in a browser |

Both run the same engine and show the same interface. There is no "remote mode" in the desktop application.

## What it does

- **Connect** to an IED (host, port 102 by default) with the ISO addressing it expects: T/S/P selectors, AP-title and AE-qualifier, as in its SCL Address. Save connections as **profiles**. Reach IEDs **through a proxy** (SOCKS5 or HTTP CONNECT), per connection or by default.
- **Discover** its model (MMS Identify, then logical devices, logical nodes, data objects and attributes with their functional constraints) and **browse** it in a tree, or **search** it by reference, LN class (`MMXU`, `XCBR`, ...) and FC.
- **Read** a node under one of its functional constraints. Structures show their component names; a quality is decoded (`good`, `questionable (old-data)`, ...); a timestamp shows its time quality. Every value shows the MMS domain and item ID it was read from, and the equivalent `iec61850ctl get object` command can be copied.
- **Watch** points from several IEDs in one table and **poll** them (0.5 s to 1 min); the engine polls, so a daemon keeps polling while the browser is away.
- **Open an SCL file** (SCD, CID, ICD, IID): browse an IED's data model offline, or fill in the connection with the address the file gives it.
- **Inspect** what a logical device defines: data sets (read in one request, or pinned to the watch table), report control blocks with their configuration, logs by time range, setting groups; and the IED's **files** (download disturbance records). From an SCL file, data sets and report control blocks are shown as the file defines them.
- **Export** a snapshot (the model and the watched values, JSON) or the watched values as CSV; **open a snapshot** again later, without the IED.
- **See what happened on the wire**: every action (connect, discover, read, close) is logged with its duration and the MMS requests it sent. A failed connect says where it stopped (TCP, COTP, session, presentation, ACSE or MMS Initiate) when the error proves it, and what to check.
- **Observe mode** by default: the MMS client is built so that it cannot send a Write, a control, a report or data set change, or a file change. See [SECURITY.md](SECURITY.md).
- **Commission mode** on request, per session: a grant with a reason and a duration opens a second, restricted association. **Write** set points and configuration values (FC SP, CF, DC, SV, BL), each verified by reading it back; **subscribe** to report control blocks and watch the reports arrive (the control block is released again when you stop); **create and delete** data sets; **switch** the active setting group; **operate controls** of every control model, with the select, the operate, the CommandTermination and the IED's reasons shown. Everything is recorded in an audit trail. The daemon offers it only with `--allow-commission`.

- **Compare** two sessions (IED, SCL file, snapshot): model, data sets, report control blocks and configured values; **check conformance** against the IEC 61850 namespaces; export **point lists**.
- **Analyse a capture** (pcap or pcapng, plain or gzip, at most 256 MiB) offline, with [go-mms-trace](https://github.com/otfabric/go-mms-trace): the MMS associations, a **timeline** of the confirmed services with their status (ok, error, reject, unanswered, orphan, pending) and latency, the reports, the GOOSE and Sampled Values streams, and the **findings**: TCP gaps, unanswered and failed requests, associations captured midstream or still open, decoding errors, lost or repeated GOOSE and SV messages, each with where it is and the counter it comes from. MMS on another port than 102 (1102 in iec61850-interop captures) is analysed when the port is named. **Link** the capture to an IED, SCL or snapshot session: object names become IEC 61850 references, are looked up in that model, and reports are decoded against their data sets; what the model does not have is a finding. Findings and the timeline export as CSV and JSON.
- **The wire view** of every action on an IED: the MMS PDUs it sent and received, decoded, and their bytes, in the activity panel.

Not yet: editing setting values, time-activated controls, live capture, MMS over TLS in captures.

## Build from source

```bash
make build
```

This builds `bin/iec61850-explorer` (needs a C compiler and the platform's webview; see [CONTRIBUTING.md](CONTRIBUTING.md)) and `bin/iec61850-explorerd`. It needs Go and Node.js 22 or later; the frontend dependencies are installed (`npm ci`) on the first build and again when `frontend/package-lock.json` changes.

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

## Connecting through a proxy

When the IEDs are reachable only through a jump host or a gateway proxy, the Explorer connects through a SOCKS5 proxy or an HTTP proxy's CONNECT method:

- `socks5://[user:password@]host[:1080]`: the Explorer resolves the IED's host name and sends the address; when the name has several addresses, each is tried in turn until the proxy reaches one.
- `socks5h://[user:password@]host[:1080]`: the proxy resolves the IED's host name.
- `http://[user:password@]host[:8080]`: HTTP CONNECT, with `Proxy-Authorization: Basic` when a user is given; the proxy resolves the name.

The **Proxy** field of the connection form (and of a profile) sets the proxy of one IED; empty uses the **default proxy**, `direct` connects without a proxy even when a default is set. The default proxy is set below the connection form; the desktop application keeps it in its data directory (`settings.json`, mode 0600). A session shows the proxy it connected through, always with the password replaced (`xxxxx`): in the session bar, the API, the logs, events and snapshots. A failed connection through a proxy is reported at the stage `proxy`: the proxy not reachable, a malformed answer, authentication refused, or the proxy refusing the IED (with the SOCKS reply code or the HTTP status).

The daemon takes its default proxy from `--proxy URL` (or `--proxy direct`), else from the environment variable `ALL_PROXY` or `all_proxy`; it logs the one in use at start, without its password. A default the operator sets this way cannot be changed through the API, and the web UI shows it read-only. Without one, users of the daemon may set a default. The target allow-list still applies; see [SECURITY.md](SECURITY.md).

```bash
iec61850-explorerd --proxy socks5://127.0.0.1:1080   # for example the end of "ssh -D 1080 jump-host"
```

A password in a proxy URL is stored as given in a profile or the settings file (both mode 0600 in the data directory); use a proxy account for the purpose.

## Documentation

- [Architecture](docs/ARCHITECTURE.md): layering, the `Service` interface, the HTTP API, Observe mode, the capability audit of the libraries, milestones
- [Decisions](docs/adr/)
- [Security](SECURITY.md), [Testing](TESTING.md), [Contributing](CONTRIBUTING.md)

## License

MIT, see [LICENSE](LICENSE).
