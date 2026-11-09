# ADR 0002: Observe mode is enforced in the MMS client

- Status: Accepted
- Date: 2026-11-08

## Context

The engine works on a go-iec61850 client, which exposes writes, controls, RCB changes, dataset and file operations. Hiding buttons would make "read-only" a convention of the UI. Subscribing to a report writes RCB attributes, so even an innocent-looking feature can change an IED.

## Decision

Every MMS client the engine creates sets go-mms `DialOptions.AllowedServices` to `internal/readonly.Services` (Identify, Status, GetNameList, Read, GetVariableAccessAttributes, GetNamedVariableListAttributes). The option was added to go-mms (v1.5.0, otfabric/go-mms#29) for this project: the check sits in the one function every confirmed request passes through and refuses before the transport. The linter bans state-changing calls and MMS servers outside tests; the wire test counts the services a simulated IED receives.

## Consequences

The Explorer needs go-mms v1.5.0 or later. One state change rides on an allowed service: reading a control's `SBO` attribute selects it (IEC 61850-8-1). The allow-list cannot see that, so the engine also refuses every read of the functional constraint CO (added in 0.5.0). A new read service (file directory, journal read) is added to the list and its test deliberately. Commission and Laboratory modes will be separate, explicit per-session grants enforced in the engine, with their own allow-lists; Observe stays the default.
