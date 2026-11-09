# Releases

Release notes, newest first. Each release has a section headed `## vX.Y.Z` with `**Date:**` and `**Previous release:**` lines, ending with a line `---`; the Release workflow publishes that section.

## v0.6.0

**Date:** 2026-11-09
**Previous release:** v0.5.0

### Added

- **Interlock check and synchrocheck** on controls: the Control form asks the IED to check the interlocking and the synchronism before it executes (the Check attribute), and the confirmation, the result and the audit record show which checks were asked for. API: `interlock_check` and `synchro_check` in `POST /api/v1/sessions/{id}/control`.

### Changed

- Built on go-iec61850 v1.10.1, which sends the two Check bits in the standard order (v1.10.0 swapped them).
- The simulated test IED checks the interlocking of XCBR1.Pos only when an interlock check is asked for, and its single point controls refuse a synchrocheck.

### Dependencies

go-iec61850 v1.10.1, go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.5.0

**Date:** 2026-11-09
**Previous release:** v0.4.0

Commission mode, third step: controls. This release also carries the Observe-mode fix prepared as v0.4.1, which was not released on its own.

### Fixed

- **Observe mode could select a control.** Reading a control's `SBO` attribute is the select of a control with normal security (IEC 61850-8-1), so in v0.4.0 and earlier a read of `…SBO[CO]` (or of a data object or logical node with FC CO) could select a control on the IED. The Read service is in Observe mode's allow-list, so go-mms let it pass. The engine now refuses every read of the functional constraint CO, and reading a data set with a CO member; the node panel offers no read or watch for CO.

### Added

- **Controls** (capability `controls`): operate a controllable data object from its CO section in the node panel, after a confirmation, optionally as a test. The Explorer reads the object's ctlModel and runs its sequence:
  - direct: operate;
  - select before operate: select, then operate;
  - enhanced security: the operate is followed by a wait for the CommandTermination.

  The result shows each step, the IED's reason when it refuses or ends a control negatively (such as object-not-selected or blocked-by-interlocking), and the status before and after. A failed operate after a select is cancelled. Each control is one audit record with all its steps.
- API: `POST /api/v1/sessions/{id}/control`; API version 0.4.

### Changed

- Built on go-iec61850 v1.10.0. Its IED server now reports a data set member that is a whole data object (such as `MMXU1.Hz[MX]`) as the structure of its attributes and reports changes below it; before, the member was sent as `false`. This affects only go-iec61850-based IEDs, such as its example server and the Explorer's simulated test IED.
- The simulated test IED has controls: GGIO1.SPCSO1 to SPCSO4 (one per control model) and XCBR1.Pos (SBO with enhanced security, interlocked by GGIO1.Ind1), and its `Events` data set has a data-object member again.

### Known limitations

- Interlock and synchrocheck flags are not offered yet: go-iec61850 v1.10.0 encodes them in the opposite bit order to libiec61850.
- No time-activated operate.

### Dependencies

go-iec61850 v1.10.0, go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.4.0

**Date:** 2026-11-09
**Previous release:** v0.3.0

Commission mode, second step: reports, data sets and setting groups.

### Added

- **Report subscription** (capability `reports`): subscribe to an unbuffered or buffered report control block, with a general interrogation if wanted. The Explorer reserves and enables the control block; unsubscribing, revoking the grant or ending the session disables and releases it. A control block that another client enabled or reserved is left alone. Reports appear in the new Reports tab with their sequence number, reasons and values, named by the data set's members.
- **Data sets** (capability `datasets`): create a data set in LLN0 from FC-qualified references (the watched points can be used), delete the ones the IED marks deletable.
- **Setting groups** (capability `setting-groups`): switch the active setting group; the control block is read back.
- Every subscription, general interrogation, data set change and setting group switch is in the audit trail, with the step that failed if one did.
- API: `POST`/`DELETE /api/v1/sessions/{id}/reports/subscriptions[/{sub}]`, `POST .../{sub}/gi`, `POST`/`DELETE /api/v1/sessions/{id}/datasets`, `PUT /api/v1/sessions/{id}/setting-groups/active`, the `report` event; API version 0.3.

### Known limitations

- Editing setting values (setting groups' SE values) and controls are not offered yet.
- go-iec61850's own IED server sends data set members that are whole data objects (such as `MMXU1.Hz[MX]`) as `false` in reports and does not report changes below them; members that are attributes work. Real IEDs are not affected by this server gap.

---

## v0.3.0

**Date:** 2026-11-09
**Previous release:** v0.2.0

Commission mode, first step: an IED can be changed, on purpose, with a record of every change.

### Added

- **Commission mode** per session: a grant with a reason, a duration (default 30 minutes, at most 8 hours) and capabilities. It opens a second MMS association whose allow-list holds only the services the capabilities need; the session's own association stays read-only. The grant ends on revoke, at expiry, and when the session or the application ends.
- **Writes** of data attributes with FC SP, CF, DC, SV or BL, with the value as text in the form a read shows it. Each write reads the value before and after and says whether the IED stored what was written.
- **Audit trail**: every grant, end and write with the target, reference, value before, value written and value read back, in `audit/audit-YYYY-MM-DD.jsonl` of the data directory (mode 0600) and in the new Audit tab.
- Daemon flag `--allow-commission`; without it the daemon offers no Commission mode.
- API: `PUT`/`DELETE /api/v1/sessions/{id}/commission`, `POST /api/v1/sessions/{id}/write`, `GET /api/v1/audit`, the `audit` event; API version 0.2.

### Changed

- The header shows the mode of the open session (observe or commission).

### Known limitations

- Report subscription, data set and setting group changes and controls are not offered yet.
- A grant needs a second association: an IED that accepts only one client refuses it.

---

## v0.2.0

**Date:** 2026-11-08
**Previous release:** v0.1.0

Inspection: everything a logical device defines can be seen, still without changing the IED.

### Added

- **Data sets** of a logical device with their members (FC-qualified references and MMS names); read a data set with one request, or pin all its members to the watch table.
- **Report control blocks** with their configuration: RptID, DatSet, ConfRev, TrgOps, OptFlds, BufTm, IntgPd, and from the IED RptEna, Resv or ResvTms, SqNum, GI, Owner, EntryID. An enabled one is marked as possibly owned by another client.
- **Logs** (journals): entries in the last hour, day or week.
- **Setting groups**: the setting group control block (number of groups, active and edit group).
- **Files** of the IED: list and download, at most 64 MiB (desktop: native Save dialog; browser: a download).
- From an **SCL file**, data sets and report control blocks as the file defines them, marked as such and without IED-only state.

### Changed

- Observe mode now allows the read services FileDirectory, FileOpen, FileRead, FileClose and ReadJournal. FileOpen and FileClose open and release a read state for one download; FileRename, FileDelete and ObtainFile stay refused.

### Known limitations

- No Commission mode yet (M2b): no report subscription, writes, controls or setting group changes.

---

## v0.1.0

**Date:** 2026-11-08
**Previous release:** none (first release)

The first release: a read-only IEC 61850 browser, as a desktop application (Windows, macOS, Linux) and as a daemon for gateways with a web UI (Linux amd64, arm64, armv7; container image).

### Added

- **Connect** to an IED over MMS with configurable ISO addressing (T/S/P selectors, AP-titles, AE-qualifiers; omitted with `-`), ask its identity and discover its model.
- **Browse** the model in a tree, **search** it by reference, LN class, FC and kind.
- **Read** values in batches (32 references per MMS Read); structures with their component names, decoded quality and time quality, the MMS domain and item ID of every value, the equivalent `iec61850ctl` command.
- **Watch and poll** points from several IEDs in one table (0.5 s to 1 min), polled by the engine; missed cycles are counted, not caught up.
- **SCL import**: list the IEDs and access points of an SCD/CID/ICD/IID file with their addresses and the parser's findings; browse a data model offline, or connect with the address the file gives.
- **Snapshots** (`otfabric.iec61850-explorer.snapshot/1`): the model and the watched values as JSON; open a snapshot again as an offline session. **CSV** export of the watched values.
- **Connection profiles**, stored by the engine (desktop: the user's configuration directory; daemon: `--data-dir`).
- **Diagnostics**: every action with the MMS requests it caused; a failed connect names its layer (TCP, COTP, session, presentation, ACSE, MMS Initiate) when the error type proves it, with what to check.
- **Observe mode**, enforced in the MMS client (go-mms `DialOptions.AllowedServices`), by the linter and by a wire test: no state-changing service can be sent.
- **Daemon**: bearer token (0600, rotatable), Host check, loopback bind, target allow-list (private and link-local ranges by default), `url` and `token rotate` commands, systemd unit, container image.

### Known limitations

- Control blocks (reports, logs, GOOSE, setting groups) are not built from SCL; the live tree shows them.
- No datasets, reports, controls, setting groups, files or journals yet (M2); no SCL-to-live comparison (M3).
- Releases are not code-signed: macOS Gatekeeper and Windows SmartScreen warn before the first start.

### Dependencies

go-iec61850 v1.9.3, go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---
