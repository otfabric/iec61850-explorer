# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in the IEC 61850 Explorer, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@otfabric.io**, or open a private advisory on GitHub (the [Security](https://github.com/otfabric/iec61850-explorer/security) tab → **Advisories** → **Report a vulnerability**), with:

- A description of the vulnerability
- Steps to reproduce the issue
- The affected version(s) (`iec61850-explorer --version` output)
- Any potential impact you've identified

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation within 7 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | Best effort |

## Security Considerations

### Observe Mode: The Explorer Does Not Change the IED

A session is in Observe mode unless the user grants it Commission mode (below). In Observe mode the Explorer only discovers and reads. It never sends a Write, a control (Select, Operate, Cancel), a report control block change, a data set definition or deletion, a setting-group change, or a file rename, deletion or transfer. This is enforced, not a convention:

- every MMS client the engine creates is restricted by go-mms's service allow-list (`DialOptions.AllowedServices`) to Identify, Status, GetNameList, Read, GetVariableAccessAttributes, GetNamedVariableListAttributes, FileDirectory, FileOpen, FileRead, FileClose and ReadJournal: the client refuses anything else before it reaches the wire (`internal/readonly`). FileOpen and FileClose only open and release a read state for one download;
- the linter fails the build when non-test code calls a state-changing go-iec61850 or go-mms method or creates an MMS server (`.golangci.yml`);
- a test runs every workflow against a simulated IED that counts the services it receives, and asserts that only those services arrive.

Reading still costs the IED some work (a large log or file download most of all), and it opens one MMS association per session: an IED that accepts few associations may refuse other clients while the Explorer is connected.

No interface of the Explorer accepts a raw MMS PDU, a service name or a file path.

### Commission Mode: Granted, Separate, Audited

Commission mode lets a user change an IED. It is off unless asked for, per session:

- **A grant** names a reason, a duration (at most 8 hours) and capabilities: `write` (data attributes of the functional constraints SP, CF, DC, SV and BL), `reports` (subscribe to report control blocks), `datasets` (create and delete data sets) and `setting-groups` (select the active setting group). Controls are not offered.
- **Reports** are taken only from a control block that is neither enabled nor reserved, so the Explorer does not take over another client's reports. The control blocks it reserved and enabled are disabled and released when the subscription or the grant ends; a step that fails (the IED is gone) is recorded as such.
- **Data sets** the IED marks as not deletable (configured in SCL) are never deleted.
- **A second association.** The Observe association of the session is never widened. The grant opens another association whose go-mms allow-list holds the Observe services plus only the services its capabilities need (`internal/commission`); go-mms refuses anything else before the wire. An IED that accepts few associations may refuse it.
- **Audit trail.** Every grant, end, cleanup and write is appended to `<data dir>/audit/audit-YYYY-MM-DD.jsonl` (mode 0600) and synced before the result is shown, with the target, the reference, the value before, the value written and the value read back. Without a writable trail there is no grant. A write the IED received is recorded even if it failed.
- **Ends by itself.** A grant ends on revoke, at its expiry, and when the session or the engine ends.
- **The daemon** offers Commission mode only when started with `--allow-commission`. Anyone holding the token can then change the IEDs the daemon may reach: keep the token, the target allow-list and that flag as narrow as the work needs.
- **A write is verified** by reading the value back; a value the IED stores differently is shown and recorded as a mismatch.

### The Daemon

`iec61850-explorerd` listens on `127.0.0.1` only, and is still authenticated: other users of the gateway, and other pages in your browser, can reach a loopback port too.

- **Token.** Every API request needs a bearer token, compared in constant time and accepted in the `Authorization` header only. It is created on the first start, stored in a file with mode 0600, never logged, and shown only by `iec61850-explorerd url` and, when the daemon was started from a terminal, in one line at startup. A daemon under systemd or in a container does not write it: its journal and its logs hold no token. A URL with `#token=` is a credential: do not paste it into chats or tickets.
- **In the browser** the token arrives in the URL fragment, which is not sent to any server; the page removes it from the address bar and keeps it in memory only: no cookie, no local storage, no query string.
- **Host check.** Requests that name another host are refused, which stops DNS-rebinding attacks from web pages. No CORS headers are sent; the page's content security policy allows its own origin only.
- **Targets.** The daemon connects only to the networks it is allowed to (`--allow-target`; by default private and link-local ranges), so that a token holder cannot use a gateway to probe other networks. A host name is checked when the session is opened; a name that later resolves elsewhere is not re-checked.
- **Other addresses.** Binding anything but loopback needs `--allow-remote-bind`. The listener is plain HTTP: on such an address the token is readable on the network. Use the SSH port forward instead.
- **Data directory.** Created with mode 0700; a symbolic link, another owner or wider permissions are refused.

### MMS Usually Has No Security

IEC 61850 MMS as most IEDs run it carries no authentication and no encryption: anyone who can reach port 102 of an IED can read it, often write it and operate its controls, and anyone on the path can read or alter the traffic.

- **Use the Explorer only on devices and networks you own or are authorized to access**
- **Keep the station bus isolated**; never expose port 102 of an IED to the Internet

### Files Are Untrusted Input

SCL files and snapshots come from users and other tools. The engine never opens a path a client names: the user interface reads the file the user chose and hands over its bytes, at most 64 MiB. SCL is parsed by go-iec61850's parser (Go's `encoding/xml`, which resolves no external entities); a snapshot is checked for its schema and for a consistent tree before it is used. Strings from either are rendered as text, never as markup; CSV exports prefix text a spreadsheet would take for a formula.

### Profiles

Connection profiles hold addresses, ISO selectors and notes, no passwords. They are stored in the data directory in a file with mode 0600.

### Data Read From Devices Is Untrusted Input

Names, types and values come from the IED. The model tree is built from the names the IED lists; a value's nesting is bounded; the user interface renders device strings as text, never as markup.

### Dependencies

`make check` runs `govulncheck`. The protocol code is in [go-iec61850](https://github.com/otfabric/go-iec61850), [go-mms](https://github.com/otfabric/go-mms) and [go-cotp](https://github.com/otfabric/go-cotp). The desktop shell is [Wails v3](https://v3.wails.io), a prerelease pinned to an exact version in `go.mod`. The user interface bundles its fonts and makes no request to any other host.

### Verifying Downloads

There is no release yet. When there is, releases will not be code-signed or notarized at first: macOS Gatekeeper and Windows SmartScreen will warn before the first start. Each release is published with a SHA-256 checksum file, and every file and the container image carry a build provenance attestation, which proves that the release workflow of this repository built them:

```sh
sha256sum -c --ignore-missing iec61850-explorer-<version>-checksums.txt
gh attestation verify <file> --repo otfabric/iec61850-explorer
gh attestation verify oci://ghcr.io/otfabric/iec61850-explorerd:<version> --repo otfabric/iec61850-explorer
```

Verify before installing, above all on a gateway.
