# ADR 0001: Reuse the architecture of sunspec-explorer

- Status: Accepted
- Date: 2026-11-08

## Context

OT Fabric already ships sunspec-explorer: one Go engine behind a `Service` interface, a Wails desktop application that runs it in-process, and a static daemon that serves an HTTP/SSE API and the same React frontend to a browser through an SSH port forward. Its daemon security (token, Host check, bind rules, target allow-list), packaging and native-runner CI are proven.

## Decision

The IEC 61850 Explorer uses the same architecture, layering, daemon, frontend toolchain and CI. The code was copied and adapted, not shared as a library: the two applications have different domain models and will diverge.

## Consequences

Two artifacts from one module (`iec61850-explorer`, `iec61850-explorerd`); the daemon never imports Wails; the contract suite runs against the engine and the HTTP API. A security fix in the shared daemon code (`internal/daemon`, `internal/api`, `internal/web`) is applied to both repositories.
