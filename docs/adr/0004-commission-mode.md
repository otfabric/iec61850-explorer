# ADR 0004: Commission mode is a separate, granted, audited association

- Status: Accepted
- Date: 2026-11-09

## Context

M2b lets a user change an IED: write data attributes, configure and enable report control blocks, create and delete data sets, change setting groups and, later, operate controls. ADR 0002 made Observe mode a property of the MMS client (go-mms `DialOptions.AllowedServices`), fixed when the association is opened. Commission mode must keep that guarantee for everything that is not explicitly granted, leave a record of every change, and clean up what it set up for itself (report reservations, enabled report control blocks) when it ends.

## Decision

1. **A session stays in Observe mode.** Its association keeps `internal/readonly.Services` for its whole life; nothing widens it.
2. **Commission is a grant on a session.** `GrantCommission` opens a *second* association to the same IED with the Commission allow-list `internal/commission.Services(capabilities)`: the Observe services plus only what the granted capabilities need (`Write` for writes, report and setting group control blocks; `DefineNamedVariableList` and `DeleteNamedVariableList` for data sets). Every state-changing call goes through that association and nowhere else. A grant has a reason the user types, a duration (at most 8 hours, default 30 minutes) and capabilities chosen from `write`, `reports`, `datasets`, `setting-groups`; the engine checks the capability before each call. Controls (`controls`) are not grantable until go-iec61850's client waits for and reports CommandTermination.
3. **Revoking closes the association.** A grant ends when the user revokes it, when it expires, when the session closes or is lost, or when the engine shuts down. Before the association is concluded the engine undoes what it set up for itself: it disables the report control blocks it enabled and releases the unbuffered ones it reserved. Changes the user made deliberately (written values, created data sets, a selected setting group) are not undone; they are in the audit trail.
4. **No audit, no Commission.** The engine refuses a grant unless it can write the audit trail: JSON Lines, one file per day in `<data dir>/audit/`, mode 0600, appended and synced before the outcome is reported. Every grant, revocation, expiry and change is recorded with the session, the target, the reference, the value before and after (readback) and the outcome; the same records are published as `audit` events.
5. **The daemon does not offer Commission mode unless started with `--allow-commission`.** The desktop application offers it. Health reports whether it is available.
6. **The ban stays in the linter.** forbidigo still bans state-changing calls everywhere except `_test.go`, `internal/simied` and the engine's `commission*.go` files, which are the only code that holds the Commission association.
7. **A write is verified.** The engine converts the user's value with the attribute's type (GetVariableAccessAttributes), writes it and reads it back; the result shows the value before, the value written, the value read back and whether they match.

## Consequences

- A grant costs a second association on the IED. An IED that allows only one client refuses it; the grant fails with the classified connect error and the Observe association is not touched.
- Observe-mode guarantees, tests and the wire test are unchanged. New wire tests count the services the simulated IED receives on each association.
- Cleanup is best effort: if the IED is unreachable when a grant ends, the audit trail records what could not be undone.
- Controls follow in a later step, after the go-iec61850 client handles CommandTermination (`docs/ARCHITECTURE.md` section 8).
