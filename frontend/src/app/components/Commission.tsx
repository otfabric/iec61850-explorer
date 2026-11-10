import { useState } from 'react';
import { CheckIcon, ErrorIcon, UnlockIcon, WarningIcon, WriteIcon } from '../icons';
import {
  DEFAULT_GRANT_MINUTES,
  MAX_GRANT_MINUTES,
  WRITABLE_FCS,
  type AuditRecord,
  type Capability,
  type GrantRequest,
  type SessionInfo,
  type Value,
  type WriteResult,
  type ControlRequest,
  type ControlResult,
} from '../service/types';
import type { ReportEntryView } from '../state/useExplorer';
import { stateText, summarize } from './summary';
import { Button, Chip, Field, TextInput, Toggle } from './ui';

/** The capabilities this version of the Explorer uses; the engine knows more. */
export const OFFERED_CAPABILITIES: { cap: Capability; label: string }[] = [
  { cap: 'write', label: 'Write data attributes (SP, CF, DC, SV, BL), verified by readback' },
  { cap: 'reports', label: 'Subscribe to report control blocks (reserve, enable, GI; released when the grant ends)' },
  { cap: 'datasets', label: 'Create and delete data sets' },
  { cap: 'setting-groups', label: 'Select the active setting group' },
  { cap: 'controls', label: 'Operate controls (select, operate, CommandTermination)' },
];

interface GrantDialogProps {
  session: SessionInfo;
  onGrant: (req: GrantRequest) => Promise<boolean>;
  onCancel: () => void;
}

/** Asks for Commission mode: a reason, a duration and capabilities. */
export function GrantDialog({ session, onGrant, onCancel }: GrantDialogProps) {
  const [reason, setReason] = useState('');
  const [minutes, setMinutes] = useState(String(DEFAULT_GRANT_MINUTES));
  const [caps, setCaps] = useState<Capability[]>(['write']);
  const [busy, setBusy] = useState(false);
  const mins = Number(minutes);
  const valid = reason.trim() !== '' && Number.isInteger(mins) && mins >= 1 && mins <= MAX_GRANT_MINUTES && caps.length > 0;

  async function submit() {
    setBusy(true);
    const ok = await onGrant({ reason: reason.trim(), minutes: mins, capabilities: caps });
    setBusy(false);
    if (ok) {
      onCancel();
    }
  }

  return (
    <div className="fixed inset-0 z-10 flex items-start justify-center overflow-auto bg-bg/80 p-8">
      <section role="dialog" aria-modal="true" aria-label="Commission mode" className="w-full max-w-xl rounded border border-wire bg-surface p-4">
        <h2 className="flex items-center gap-2 font-display text-lg font-semibold text-fg-strong">
          <UnlockIcon size={18} className="text-wire" /> Commission mode
        </h2>
        <p className="mt-2 text-muted">
          Changes go to {session.target.host}:{session.target.port ?? 102} over a second association that allows only what you grant here. Every change is
          recorded in the audit trail with the value before and after. The grant ends when you revoke it, when it expires or when the session ends.
        </p>
        <form
          className="mt-4 flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (valid && !busy) {
              void submit();
            }
          }}
        >
          <Field label="Reason (recorded, for example a work order)">
            <TextInput value={reason} maxLength={200} autoFocus onChange={(e) => setReason(e.target.value)} placeholder="WO-4711 relay commissioning" />
          </Field>
          <Field label={`Duration in minutes (1 to ${MAX_GRANT_MINUTES})`}>
            <TextInput className="w-28" inputMode="numeric" value={minutes} onChange={(e) => setMinutes(e.target.value)} />
          </Field>
          <fieldset className="flex flex-col gap-1">
            <legend className="ot-label mb-1">Capabilities</legend>
            {OFFERED_CAPABILITIES.map(({ cap, label }) => (
              <Toggle key={cap} label={label} checked={caps.includes(cap)} onChange={(on) => setCaps((c) => (on ? [...c, cap] : c.filter((x) => x !== cap)))} />
            ))}
          </fieldset>
          <div className="flex justify-end gap-2">
            <Button onClick={onCancel}>Cancel</Button>
            <Button type="submit" variant="primary" disabled={!valid || busy}>
              Grant
            </Button>
          </div>
        </form>
      </section>
    </div>
  );
}

/** The banner of a session in Commission mode. */
export function CommissionBanner({ session, onRevoke }: { session: SessionInfo; onRevoke: () => void }) {
  const { grant } = session;
  if (!grant) {
    return null;
  }
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-wire bg-surface px-4 py-2 text-wire-text" role="status" aria-label="Commission grant">
      <WarningIcon size={16} />
      <span className="font-medium">Commission mode</span>
      {grant.capabilities.map((c) => (
        <Chip key={c} tone="wire">
          {c}
        </Chip>
      ))}
      <span className="text-muted">
        {grant.reason} · until {new Date(grant.expires_at).toLocaleTimeString()}
      </span>
      <Button className="ml-auto" onClick={onRevoke}>
        Revoke
      </Button>
    </div>
  );
}

/** The scalar attributes of a read value, as paths below the data object. */
export function leaves(value: Value, prefix = ''): { path: string; value: Value }[] {
  if (!value.children || value.children.length === 0) {
    return prefix ? [{ path: prefix, value }] : [];
  }
  // Array elements have no name; they are not written one by one here.
  return value.children.flatMap((child) => (child.name ? leaves(child, prefix ? `${prefix}.${child.name}` : child.name) : []));
}

interface WriteFormProps {
  doRef: string;
  fc: string;
  /** The last read of the data object's FC, which names the attributes. */
  value?: Value;
  disabled: boolean;
  onWrite: (ref: string, value: string) => Promise<WriteResult | null>;
}

/** Writes one attribute of a data object under a writable FC. */
export function WriteForm({ doRef, fc, value, disabled, onWrite }: WriteFormProps) {
  const attrs = value ? leaves(value) : [];
  const [path, setPath] = useState(attrs[0]?.path ?? '');
  const [text, setText] = useState(attrs[0]?.value.text ?? '');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<WriteResult | null>(null);
  if (!WRITABLE_FCS.includes(fc)) {
    return null;
  }
  if (attrs.length === 0) {
    return <p className="px-3 py-2 text-[12px] text-muted">Read this FC to write its attributes.</p>;
  }
  const ref = `${doRef}.${path}[${fc}]`;

  async function submit() {
    setBusy(true);
    setResult(await onWrite(ref, text));
    setBusy(false);
  }

  return (
    <form
      aria-label={`Write ${fc}`}
      className="flex flex-col gap-2 border-t border-wire px-3 py-2"
      onSubmit={(event) => {
        event.preventDefault();
        if (!busy && !disabled) {
          void submit();
        }
      }}
    >
      <div className="flex flex-wrap items-end gap-2">
        <Field label="Attribute">
          <select
            className="h-8 rounded border border-control bg-bg px-2 font-mono text-[13px] text-fg-strong"
            value={path}
            onChange={(e) => {
              setPath(e.target.value);
              setText(attrs.find((a) => a.path === e.target.value)?.value.text ?? '');
              setResult(null);
            }}
          >
            {attrs.map((a) => (
              <option key={a.path} value={a.path}>
                {a.path} ({a.value.type})
              </option>
            ))}
          </select>
        </Field>
        <Field label="New value">
          <TextInput aria-label="New value" value={text} onChange={(e) => setText(e.target.value)} />
        </Field>
        <Button type="submit" icon={<WriteIcon size={14} />} disabled={disabled || busy} title={`Write ${ref} and read it back`}>
          Write
        </Button>
      </div>
      {result && <WriteOutcome result={result} />}
    </form>
  );
}

function WriteOutcome({ result }: { result: WriteResult }) {
  if (result.error) {
    return (
      <p className="flex items-center gap-2 text-danger" role="alert">
        <ErrorIcon size={14} /> The IED refused the write: {result.error.message}
      </p>
    );
  }
  return (
    <p className={`flex flex-wrap items-center gap-2 font-mono text-[12px] ${result.verified ? 'text-accent' : 'text-wire-text'}`} role="status">
      {result.verified ? <CheckIcon size={14} /> : <WarningIcon size={14} />}
      {result.verified ? 'Written and verified' : 'Written, but the value read back differs'}
      <span className="text-muted">
        {result.before?.text ?? '?'} → {result.written?.text ?? '?'} · read back {result.after?.text ?? 'nothing'}
      </span>
    </p>
  );
}

interface ControlFormProps {
  doRef: string;
  disabled: boolean;
  onControl: (req: ControlRequest) => Promise<ControlResult | null>;
}

/**
 * Operates a controllable data object. The engine reads its ctlModel and
 * runs the sequence; the user confirms before anything is sent.
 */
export function ControlForm({ doRef, disabled, onControl }: ControlFormProps) {
  const [value, setValue] = useState('true');
  const [test, setTest] = useState(false);
  const [interlock, setInterlock] = useState(false);
  const [synchro, setSynchro] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ControlResult | null>(null);

  async function operate() {
    setBusy(true);
    setResult(await onControl({ ref: doRef, value, test, interlock_check: interlock, synchro_check: synchro }));
    setBusy(false);
    setConfirming(false);
  }

  return (
    <form
      aria-label="Control"
      className="flex flex-col gap-2 border-t border-wire px-3 py-2"
      onSubmit={(event) => {
        event.preventDefault();
        if (!busy && !disabled) {
          if (confirming) {
            void operate();
          } else {
            setResult(null);
            setConfirming(true);
          }
        }
      }}
    >
      <div className="flex flex-wrap items-end gap-2">
        <Field label="ctlVal">
          <TextInput aria-label="ctlVal" value={value} disabled={confirming} onChange={(e) => setValue(e.target.value)} />
        </Field>
        <Toggle label="Test" checked={test} onChange={setTest} />
        <Toggle label="Interlock check" checked={interlock} onChange={setInterlock} />
        <Toggle label="Synchrocheck" checked={synchro} onChange={setSynchro} />
        {!confirming && (
          <Button type="submit" icon={<WriteIcon size={14} />} disabled={disabled || busy || value.trim() === ''} title={`Operate ${doRef}`}>
            Operate…
          </Button>
        )}
      </div>
      {confirming && (
        <div className="flex flex-wrap items-center gap-2 rounded border border-wire px-2 py-1.5" role="group" aria-label="Confirm the control">
          <WarningIcon size={14} />
          <span>
            Operate <span className="font-mono">{doRef}</span> with ctlVal <span className="font-mono">{value}</span>
            {test ? ' as a test' : ''}
            {checks(interlock, synchro)} on the IED?
          </span>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? 'Operating…' : 'Operate'}
          </Button>
          <Button type="button" disabled={busy} onClick={() => setConfirming(false)}>
            Back
          </Button>
        </div>
      )}
      {result && <ControlOutcome result={result} />}
    </form>
  );
}

/** The Check flags as words for the confirmation. */
function checks(interlock: boolean, synchro: boolean): string {
  const asked = [interlock && 'interlock', synchro && 'synchronism'].filter(Boolean);
  return asked.length ? `, checking ${asked.join(' and ')}` : '';
}

const OUTCOME_TEXT: Record<ControlResult['outcome'], string> = {
  ok: 'Operated',
  refused: 'The IED refused the control',
  negative: 'Accepted, then ended negatively',
  'no-termination': 'Accepted; no CommandTermination arrived',
  failed: 'The control failed',
};

/** The outcome of a control: every step and the IED's reason. */
export function ControlOutcome({ result }: { result: ControlResult }) {
  const ok = result.outcome === 'ok';
  return (
    <div className={`flex flex-col gap-1 font-mono text-[12px] ${ok ? 'text-accent' : 'text-danger'}`} role={ok ? 'status' : 'alert'} aria-label="Control outcome">
      <p className="flex flex-wrap items-center gap-2">
        {ok ? <CheckIcon size={14} /> : <ErrorIcon size={14} />}
        {OUTCOME_TEXT[result.outcome]}
        {result.add_cause && <Chip tone="wire">{result.add_cause}</Chip>}
        <span className="text-muted">
          {result.ctl_model} · ctlNum {result.ctl_num}
          {result.test ? ' · test' : ''}
          {result.interlock_check ? ' · interlock check' : ''}
          {result.synchro_check ? ' · synchrocheck' : ''}
        </span>
      </p>
      <ol className="flex flex-wrap gap-2 text-muted">
        {result.steps.map((st, i) => (
          <li key={`${st.step}${i}`} className={st.ok ? '' : 'text-danger'}>
            {st.step} {st.ok ? 'ok' : (st.add_cause ?? st.error ?? 'failed')}
          </li>
        ))}
      </ol>
      <p className="text-muted" title={`stVal ${result.before?.text ?? '?'} → ${result.after?.text ?? '?'}`}>
        stVal {stateText(result.before)} → {stateText(result.after)}
      </p>
    </div>
  );
}

/** The reports of the subscriptions, newest first. */
export function ReportsPanel({ reports, label }: { reports: ReportEntryView[]; label: (sessionId: string) => string }) {
  return (
    <section aria-label="Reports" className="flex h-full min-h-0 flex-col">
      <ol className="min-h-0 flex-1 overflow-auto px-4 py-2 font-mono text-[12px]">
        {reports.length === 0 && <li className="text-muted">No reports. Subscribe to a report control block in Commission mode to receive them here.</li>}
        {reports.map(({ seq, sessionId, report }) => (
          <li key={seq} className="border-b border-line py-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-muted">{new Date(report.received_at).toLocaleTimeString()}</span>
              <span className="text-fg-strong">{report.rpt_id}</span>
              <span className="text-muted">{sessionId ? label(sessionId) : ''}</span>
              {report.seq_num !== undefined && <span className="text-muted">#{report.seq_num}</span>}
              {report.buf_ovfl && <Chip tone="wire">buffer overflow</Chip>}
              <span className="ml-auto text-muted">{report.entries.length} entries</span>
            </div>
            <ul className="pl-4">
              {report.entries.map((en, i) => (
                <li key={`${en.ref}${i}`} className="flex flex-wrap gap-2">
                  <span>{en.ref}</span>
                  <span className="text-fg-strong">{summarize(en.value).text}</span>
                  {en.reasons && <span className="text-muted">{en.reasons.join(', ')}</span>}
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ol>
    </section>
  );
}

/** The Commission audit trail of this engine run, newest first. */
export function AuditPanel({ records, label }: { records: AuditRecord[]; label: (sessionId: string) => string }) {
  return (
    <section aria-label="Audit trail" className="flex h-full min-h-0 flex-col">
      <ol className="min-h-0 flex-1 overflow-auto px-4 py-2 font-mono text-[12px]">
        {records.length === 0 && <li className="text-muted">No changes. Commission mode records every grant and change here and in the audit files.</li>}
        {records.map((r) => (
          <li key={r.seq} className="flex flex-wrap items-center gap-2 border-b border-line py-1">
            {r.outcome === 'ok' ? (
              <CheckIcon size={14} className="text-accent" />
            ) : r.outcome === 'mismatch' ? (
              <WarningIcon size={14} className="text-wire" />
            ) : (
              <ErrorIcon size={14} className="text-danger" />
            )}
            <span className="text-muted">{new Date(r.at).toLocaleTimeString()}</span>
            <span className="text-fg-strong">{r.action}</span>
            <span className="text-muted">{label(r.session_id)}</span>
            {r.ref && <span className="truncate">{r.ref}</span>}
            {r.action === 'write' && (
              <span>
                {r.before ?? '?'} → {r.written} {r.after !== undefined && `(read back ${r.after})`}
              </span>
            )}
            {r.action === 'control' && (r.before !== undefined || r.after !== undefined) && (
              <span>
                stVal {r.before ?? '?'} → {r.after ?? '?'}
              </span>
            )}
            {r.reason && <span className="text-muted">“{r.reason}”</span>}
            {r.detail && <span className="text-muted">{r.detail}</span>}
            {r.action === 'select-active-sg' && (
              <span>
                group {r.before} → {r.written} {r.after !== undefined && `(read back ${r.after})`}
              </span>
            )}
            {r.capabilities && r.action === 'grant' && <span className="text-muted">{r.capabilities.join(', ')}</span>}
            {r.error && <span className="text-danger">{r.error}</span>}
          </li>
        ))}
      </ol>
    </section>
  );
}
