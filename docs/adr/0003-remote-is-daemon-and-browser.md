# ADR 0003: Remote access is the daemon and a browser

- Status: Accepted
- Date: 2026-11-08 (carried over from sunspec-explorer ADR 0003)

## Context

IEDs are often reachable only from a gateway or an engineering host in the substation network.

## Decision

Remote use is `iec61850-explorerd` on that host, a user-managed SSH port forward, and a browser. The daemon embeds the web UI. The desktop application is Local only.

## Consequences

There is no Go client of the daemon, no remote target UI and no token storage in the GUI. The frontend is built twice against the TypeScript `Service` and `Platform` interfaces. The daemon binds loopback by default, requires its token and checks the Host header; it connects only to targets in its allow-list (default: private and link-local ranges).
