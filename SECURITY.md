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

In this version the Explorer only discovers and reads. It never sends a Write, a control (Select, Operate, Cancel), a report control block change, a dataset definition or deletion, a setting-group change or a file operation. This is enforced, not a convention:

- every MMS client the engine creates is restricted by go-mms's service allow-list (`DialOptions.AllowedServices`) to Identify, Status, GetNameList, Read, GetVariableAccessAttributes and GetNamedVariableListAttributes: the client refuses anything else before it reaches the wire (`internal/readonly`);
- the linter fails the build when non-test code calls a state-changing go-iec61850 or go-mms method or creates an MMS server (`.golangci.yml`);
- a test runs every workflow against a simulated IED that counts the services it receives, and asserts that only those services arrive.

Reading still costs the IED some work, and it opens one MMS association per session: an IED that accepts few associations may refuse other clients while the Explorer is connected.

No interface of the Explorer accepts a raw MMS PDU, a service name, a value to write or a file path.

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
