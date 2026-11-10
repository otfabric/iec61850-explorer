# Releases

Release notes, newest first. Each release has a section headed `## vX.Y.Z` with `**Date:**` and `**Previous release:**` lines, ending with a line `---`; the Release workflow publishes that section.

## v0.11.0

**Date:** 2026-11-10
**Previous release:** v0.10.1

### Added

- **A demo set** (`demo/`, its own Go module): AA1 Northfield, a 110/20 kV substation of five IEDs (two feeders, one of them Edition 1, a transformer with a tap changer and a made-up vendor namespace, busbar protection with breaker failure, a 9-2LE merging unit) and an HMI. An SCD, a CID per IED and the vendor's NSD; eight pcapng captures of real traffic between go-iec61850 clients and servers built from those CIDs, with GOOSE and Sampled Values from go-iec61850's encoders: browse and poll, reports, SBO controls with an interlock refusal, settings, files and logs, GOOSE trip and breaker failure, SV around a fault, the whole fault across MMS, GOOSE and SV, and a capture of problems for the findings. `make demo-ieds` serves the IEDs live from their CIDs on 127.0.0.1:10201-10205. `demo/README.md` describes each file and a demo script. `make demo` regenerates the set byte for byte; `make demo-check` (also in CI) checks that and analyses every capture with go-mms-trace.

### Changed

- **Start view.** Connect to an IED, Open without an IED and Analyse a capture are side by side, each saying what it offers; proxy, ISO addressing, profile and the default proxy are under their options. The watch and activity dock appears once a session, a watched point or activity exists.

### Fixed

- **macOS: the desktop application is linked for macOS 13.0**, the minimum its webview code and `Info.plist` declare. It was linked for 11.0, Go's default, with a linker warning for every object file (`make build-gui`, `make bundle-macos`).

---

## v0.10.1

**Date:** 2026-11-10
**Previous release:** v0.10.0

### Changed

- **Captured reports are decoded by go-iec61850.** The fields of a report in a capture (RptID, OptFlds, the optional fields, inclusion, data references, values, reasons) are decoded by go-iec61850 `DecodeReport` (v1.16.0), which is strict about the IEC 61850-8-1 report format. v0.10.0 walked them in the engine because go-iec61850's decoder was not exported. A report that does not follow the format is still shown with its values as captured, and the reason. The engine no longer decodes any protocol itself.

### Dependencies

go-iec61850 v1.16.0 (new: `DecodeReport`), go-mms-trace v0.1.1, go-mms v1.7.1.

---

## v0.10.0

**Date:** 2026-11-10
**Previous release:** v0.9.0

### Added

- **Trace analysis (M4).** Open a capture (pcap or pcapng, plain or gzip, at most 256 MiB) from the connection screen: it opens as a trace session in its own tab, analysed offline by [go-mms-trace](https://github.com/otfabric/go-mms-trace) v0.1.1. MMS ports default to 102 and can be set (1102 for iec61850-interop captures). A damaged capture opens with what was read before the damage; a file that is not a capture is refused. At most 4 trace sessions are open at once. API: `POST /api/v1/traces/open?name=&ports=`.
- **Summary, associations, timeline, reports, streams.** The summary shows the file (format, packets, time span, SHA-256, link types), counts by status and service and the analyser's counters. Associations show their endpoints, TSAPs, AARE result, authentication mechanism (never a credential), Initiate parameters, how they opened and closed, and TCP gaps. The timeline lists the confirmed services with time, association, invoke ID, service, status (ok, error, reject, unanswered, orphan, pending), latency, the objects they name and the error, with filters (association, service, status, time window) and paging; a row opens the decoded request and response. Reports, GOOSE streams (with their issues) and Sampled Values streams (lost, repeated samples) have their own views. API: `GET .../trace/summary`, `.../trace/associations`, `.../trace/streams`; `POST .../trace/timeline`, `.../trace/reports`.
- **Findings: the gaps of a capture.** TCP gaps, associations captured midstream, rejected, aborted, reset or still open at the end, unanswered, rejected, failed, orphan and pending transactions, PDUs that did not decode, TPKT resyncs, COTP and TSDU errors, unsupported packets, a truncated or damaged file, GOOSE sequence issues and lost or repeated SV samples; each with a severity, where it is (association, frame, time, transaction or stream) and the go-mms-trace field it comes from. API: `POST .../trace/findings`.
- **Correlation with a model.** Link a trace to an IED, SCL or snapshot session: MMS names become IEC 61850 references (`LLN0$ST$Mod$stVal` in domain `LD` is `LD/LLN0.Mod.stVal[ST]`) and are looked up in that model, with the data object, data sets and control blocks they concern; what the model does not have is a "not in the model" finding. Captured IEC 61850 reports are matched to their report control block by RptID and decoded against the data set's members (inclusion bit string, data references, reasons) when the linked session defines them; otherwise they are shown as captured, with the reason. GOOSE streams are matched to the model's GOOSE control blocks, or the view says the model has none. A live session's definitions are read in Observe mode when it is linked. API: `PUT .../trace/link`.
- **Exports.** Findings and the timeline as CSV (cells protected against spreadsheet formulas) or JSON (`otfabric.iec61850-explorer.trace/1`). API: export formats `findings-csv`, `findings-json`, `timeline-csv`, `timeline-json`.
- **Wire view.** In the activity panel, an operation of a live session shows the MMS units it sent and received: direction, association (Observe or Commission), session, presentation and ACSE kinds, the decoded PDU and the bytes as hex (first 4 KiB), with the units of its requests marked by invoke ID. The engine keeps the units of the newest operations (256 units, 4 MiB per session) and never logs them. API: `GET /api/v1/sessions/{id}/wire?operation=`; operations carry `wire`.

### Changed

- API version 0.5: sessions carry `kind` (`live`, `scl`, `snapshot`, `trace`) and, for a trace, `trace`. The model views refuse a trace session (`invalid_argument`).

### Dependencies

go-mms-trace v0.1.1 (new), go-iec61850 v1.15.2, go-mms v1.7.1 (`DecodeTSDU`, used for the wire view), go-iec61850-model v0.8.2, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.9.0

**Date:** 2026-11-10
**Previous release:** v0.8.0

### Added

- **Connecting through a proxy.** An IED can be reached through a SOCKS5 proxy (`socks5://`, the Explorer resolves the IED's name; `socks5h://`, the proxy resolves it) or an HTTP proxy's CONNECT method (`http://`), each with an optional `user:password@`. The connection form and profiles have a Proxy field: empty uses the default proxy, `direct` connects without one. The default proxy is a setting below the connection form, kept in the data directory (`settings.json`, mode 0600). A session shows the proxy it connected through with the password replaced; the password never appears in sessions, events, logs, errors or snapshots. A failure through a proxy is reported at the stage `proxy`, with the SOCKS reply code or HTTP status when the proxy refused. API: `proxy` in `POST /api/v1/sessions` and in profiles; `GET`, `PUT /api/v1/settings`.
- **Daemon: `--proxy`.** `iec61850-explorerd --proxy URL` (or `--proxy direct`) sets the default proxy; without the flag, `ALL_PROXY` or `all_proxy` does. The daemon logs the proxy in use at start, without its password. A default the operator sets cannot be changed through the API; the web UI shows it read-only.

### Changed

- **Daemon: the target allow-list is checked when connecting.** Besides the check of the host name when a session is opened, the address actually connected to is checked when the TCP connection is opened, so a name that resolves elsewhere in between (DNS rebinding) is refused.
- **Daemon: the allow-list applies to proxies.** A proxy a user of the daemon names must itself be inside the allow-list; the operator's `--proxy` is trusted. With `socks5h://` and `http://`, which resolve the IED's name at the proxy, the IED must be given as an IP address.

### Dependencies

go-iec61850 v1.15.2 (new: `DialOptions.Dialer`, the hook the proxy uses), go-mms v1.7.1 (new: `Close` bounded by its context; a GetVariableAccessAttributes response whose optional address is encoded primitive, as some IEDs send it, is accepted), go-iec61850-model v0.8.2, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.8.0

**Date:** 2026-11-09
**Previous release:** v0.7.0

### Added

- **Namespace view (M3c).** The node panel shows what the IEC 61850 namespace defines for the selected node: its logical node class and base class, the data object's CDC and presence, the attribute's type and FC, and for a measurement the quantity, phase and unit. For a logical node it lists the class's data objects, which the node has, and which mandatory ones are missing. From go-iec61850-model's embedded namespaces (every release IEC publishes); identifiers and structure only, no documentation texts unless a loaded package brings them. API: `GET /api/v1/sessions/{id}/semantics?ref=`.
- **Conformance check (M3c).** Check, in the session bar, lists missing mandatory data objects (error), logical node classes the namespace does not define (warning), data objects a class does not define (information), and, for an SCL file, data objects whose CDC differs from the namespace's (warning). API: `GET /api/v1/sessions/{id}/conformance`.
- **Checked against what the device declares.** Each logical node is checked against the namespace its logical device declares (`LLN0.NamPlt.ldNs`) or it declares itself (`lnNs`), and an extension data object against its `dataNs`; the check shows what was declared and checked against. A device of IEC 61850-7-4 Edition 1 (declared as 2003, or an SCL file of schema 1.7 without a declaration), for which IEC publishes no namespace definitions, is checked against Edition 2, and a mandatory data object it lacks is a warning ("since Edition 2"). A declared namespace that is neither built in nor loaded is reported.
- **Namespace packages, vendor extensions.** Load NSD packages (a ZIP archive or one NSD document) in the Check view: official releases not built in, or a vendor's extension namespaces, so that logical nodes and data objects declared in them are checked against their definitions. An extension data object a vendor adds to a standard logical node without its own `dataNs` is found in the vendor namespace the device declares elsewhere. Their documentation texts, when the package has them, show in the Namespace view (never in exports). Packages are kept in the data directory. API: `GET`, `POST /api/v1/namespaces`, `DELETE /api/v1/namespaces/{id}`.
- **Search by quantity (M3c).** The search can ask for a measured quantity (frequency, voltage, current, power, ...): it finds the data objects that measure it by the namespace's semantic rules. The engine lists the quantities in its health (`quantities`).
- **Point lists (M3d).** Points CSV and Points JSON export every attribute of the open tab with its FC, the namespace's CDC, type and presence, the quantity and unit, and the data sets and report control blocks that carry it, for SCADA and gateway engineering. API: `points-csv` and `points-json` export formats.
- **Compare values (M3b).** The Compare view has a Values option: it adds the values of configuration, description, set point and setting attributes (FC CF, DC, SP, SG) to the comparison. An SCL file gives its configured values (instance values, or the type's defaults), an IED the values read now, a snapshot the values it holds. Enumerations compare by ordinal and show the SCL literal; floats compare at FLOAT32 precision. Settings compare per setting group between two SCL files, and in the IED's active group against an IED. API: `values` in `POST /api/v1/compare`.

### Dependencies

go-iec61850 v1.14.0, go-iec61850-model v0.8.2 (new: every published NSD release, embedded compressed), go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.7.0

**Date:** 2026-11-09
**Previous release:** v0.6.1

### Added

- **Compare (M3a).** Any two open sessions can be compared: an SCL file with the IED, a snapshot with the IED, two IEDs, two SCL files. The comparison covers the model (logical devices, logical nodes, data objects and attributes, their functional constraints and LN classes), the data sets (members, in order) and the report control blocks (buffered, report ID, data set, configuration revision, optional fields, trigger options, buffer time, integrity period). An SCL report control block is matched to the indexed instances the IED serves. Every difference shows the reference and both values; a missing node is listed once, not with everything below it. The comparison only reads: in Observe mode it lists the data sets and report control blocks of a live session from the IED. API: `POST /api/v1/compare`.

### Changed

- The device tree is virtualized: only the rows in view are rendered. Expanding a large model completely (34,465 rows for a 2.8 MB vendor CID) no longer blocks the user interface; before, expanding all data objects of that file took one task of 2.2 s.
- A node picked in the search results is scrolled into view in the tree.
- Tree rows carry `aria-level`, `aria-setsize` and `aria-posinset` (the flat form of the ARIA tree pattern).

### Dependencies

go-iec61850 v1.12.1, go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

## v0.6.1

**Date:** 2026-11-09
**Previous release:** v0.6.0

### Changed

- Built on go-iec61850 v1.12.1. More SCL files open as offline sessions:
  - files that declare a schema version the library has no bindings for (2007A, 2007B5, a non-2007 version) open with a `schema-fallback` warning;
  - files declared as `encoding="utf8"` open.

  The SCL check also reports missing required attributes, a missing `Header` element, and file-kind content (an ICD, IID or CID without an IED).

### Dependencies

go-iec61850 v1.12.1, go-mms v1.5.0, go-cotp v1.3.0, Wails v3.0.0-beta.29.

---

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
