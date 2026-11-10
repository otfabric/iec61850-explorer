import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { ErrorIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import {
  TRACE_SEVERITIES,
  TRACE_STATUSES,
  type ExportFormat,
  type SessionInfo,
  type Timeline,
  type TimelineQuery,
  type TraceAssociation,
  type TraceFindings,
  type TraceObject,
  type TraceReports,
  type TraceSeverity,
  type TraceStreams,
  type TraceSummary,
  type TraceTransaction,
  type WirePDU,
} from '../service/types';
import { Button, Chip, Field, TextInput, Toggle } from './ui';
import { ValueView } from './ValueView';

const PAGE = 100;
const SUBTABS = ['summary', 'findings', 'timeline', 'associations', 'reports', 'streams'] as const;
type SubTab = (typeof SUBTABS)[number];

const SEVERITY_TONE: Record<string, string> = { error: 'text-danger', warning: 'text-wire', info: 'text-blue' };
const STATUS_TONE: Record<string, string> = {
  ok: 'text-accent',
  error: 'text-danger',
  reject: 'text-danger',
  unanswered: 'text-wire',
  orphan: 'text-wire',
  pending: 'text-muted',
};

interface TracePanelProps {
  service: Service;
  session: SessionInfo;
  /** Sessions a trace can be linked to: open live, SCL and snapshot sessions. */
  models: SessionInfo[];
  label: (id: string) => string;
  onLink: (session: string) => Promise<boolean>;
  onExport: (format: ExportFormat) => void;
}

const time = (t?: string) => (t ? new Date(t).toISOString().replace('T', ' ').replace('Z', '') : '');

/** A capture analysed offline (M4): what it holds, its gaps, its timeline, and how it maps to a model. */
export function TracePanel({ service, session, models, label, onLink, onExport }: TracePanelProps) {
  const [tab, setTab] = useState<SubTab>('summary');
  const [error, setError] = useState<string | null>(null);
  const link = session.trace?.link;
  // Views that depend on the link are fetched again when it changes.
  const linkKey = link ? `${link.session_id}@${link.linked_at}` : '';
  const fail = useCallback((err: unknown) => setError(toServiceError(err).message), []);

  return (
    <section aria-label="Trace" className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-end gap-3 border-b border-line px-4 py-2">
        <LinkPicker session={session} models={models} label={label} onLink={onLink} />
        <div className="ml-auto flex flex-wrap gap-2">
          <Button onClick={() => onExport('findings-csv')}>Findings CSV</Button>
          <Button onClick={() => onExport('findings-json')}>Findings JSON</Button>
          <Button onClick={() => onExport('timeline-csv')}>Timeline CSV</Button>
          <Button onClick={() => onExport('timeline-json')}>Timeline JSON</Button>
        </div>
      </div>
      {link && link.notes.length > 0 && (
        <ul aria-label="Link notes" className="shrink-0 border-b border-line px-4 py-1 text-[12px] text-muted">
          {link.notes.map((n) => (
            <li key={n}>{n}</li>
          ))}
        </ul>
      )}
      <div role="tablist" aria-label="Trace views" className="flex shrink-0 gap-4 px-4 pt-2">
        {SUBTABS.map((t) => (
          <button
            key={t}
            type="button"
            role="tab"
            aria-selected={tab === t}
            className={`ot-label cursor-pointer pb-1 ${tab === t ? 'border-b-2 border-accent text-accent' : ''}`}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      {error && (
        <p className="flex items-center gap-2 px-4 py-1 text-danger" role="alert">
          <ErrorIcon size={16} />
          {error}
          <Button className="ml-auto" onClick={() => setError(null)}>
            Dismiss
          </Button>
        </p>
      )}
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {tab === 'summary' && <SummaryView key={linkKey} service={service} id={session.id} onError={fail} />}
        {tab === 'findings' && <FindingsView key={linkKey} service={service} id={session.id} onError={fail} />}
        {tab === 'timeline' && <TimelineView key={linkKey} service={service} id={session.id} onError={fail} />}
        {tab === 'associations' && <AssociationsView service={service} id={session.id} onError={fail} />}
        {tab === 'reports' && <ReportsView key={linkKey} service={service} id={session.id} onError={fail} />}
        {tab === 'streams' && <StreamsView key={linkKey} service={service} id={session.id} onError={fail} />}
      </div>
    </section>
  );
}

function LinkPicker({ session, models, label, onLink }: Pick<TracePanelProps, 'session' | 'models' | 'label' | 'onLink'>) {
  const current = session.trace?.link?.session_id ?? '';
  const [choice, setChoice] = useState(current);
  const [busy, setBusy] = useState(false);
  useEffect(() => setChoice(current), [current]);
  async function apply(value: string) {
    setBusy(true);
    await onLink(value);
    setBusy(false);
  }
  return (
    <div className="flex flex-wrap items-end gap-2">
      <Field label="Correlate with model">
        <select
          aria-label="Correlate with model"
          className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
          value={choice}
          onChange={(e) => setChoice(e.target.value)}
        >
          <option value="">no model</option>
          {models.map((m) => (
            <option key={m.id} value={m.id}>
              {label(m.id)} ({m.kind ?? (m.scl ? 'scl' : m.snapshot ? 'snapshot' : 'live')})
            </option>
          ))}
        </select>
      </Field>
      <Button disabled={busy || choice === current} onClick={() => void apply(choice)}>
        {choice === '' && current !== '' ? 'Unlink' : 'Link'}
      </Button>
      {session.trace?.link ? (
        <span className="text-[12px] text-muted">
          Linked to {session.trace.link.label} ({session.trace.link.kind}): {session.trace.link.data_sets} data sets, {session.trace.link.rcbs} report
          control blocks, {session.trace.link.gocbs} GOOSE control blocks
        </span>
      ) : (
        <span className="text-[12px] text-muted">Not linked: object names are mapped to references, not looked up.</span>
      )}
    </div>
  );
}

interface ViewProps {
  service: Service;
  id: string;
  onError: (err: unknown) => void;
}

/** Loads a view once; fetch changes when its inputs do. */
function useLoad<T>(fetch: () => Promise<T>, onError: (err: unknown) => void): T | null {
  const [data, setData] = useState<T | null>(null);
  useEffect(() => {
    let live = true;
    fetch().then(
      (d) => live && setData(d),
      (err) => live && onError(err),
    );
    return () => {
      live = false;
    };
  }, [fetch, onError]);
  return data;
}

function KV({ k, children }: { k: string; children: ReactNode }) {
  return (
    <>
      <dt className="ot-label self-center">{k}</dt>
      <dd className="font-mono text-[13px] text-fg-strong break-all">{children}</dd>
    </>
  );
}

function Counts({ title, counts }: { title: string; counts: Record<string, number> }) {
  const entries = Object.entries(counts).sort(([a], [b]) => a.localeCompare(b));
  return (
    <div>
      <h3 className="ot-label mb-1">{title}</h3>
      {entries.length === 0 ? (
        <p className="text-muted">none</p>
      ) : (
        <ul className="font-mono text-[13px]">
          {entries.map(([k, v]) => (
            <li key={k}>
              {k}: {v}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function SummaryView({ service, id, onError }: ViewProps) {
  const s = useLoad<TraceSummary>(useCallback(() => service.traceSummary(id), [service, id]), onError);
  if (!s) {
    return <p className="text-muted">Loading…</p>;
  }
  const { file, counters } = s;
  const counterRows: Record<string, number> = {
    'TPKT resync (bytes)': counters.tpkt_resync,
    'COTP errors': counters.cotp_errors,
    'TSDU errors': counters.tsdu_errors,
    'bare MMS': counters.bare_mms,
    'TSO segments': counters.tso_segments,
    'GOOSE errors': counters.goose_errors,
    'SV errors': counters.sv_errors,
  };
  for (const [k, v] of Object.entries(counters.unsupported)) {
    counterRows[`unsupported: ${k}`] = v;
  }
  return (
    <div aria-label="Trace summary" role="region" className="flex flex-col gap-4">
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1">
        <KV k="file">{file.name}</KV>
        <KV k="format">
          {file.format}
          {file.gzip ? ' (gzip)' : ''}
        </KV>
        <KV k="packets">{file.packets}</KV>
        <KV k="bytes">{file.bytes}</KV>
        <KV k="time span">
          {file.first ? `${time(file.first)} – ${time(file.last)} (${file.duration_ms.toFixed(1)} ms)` : 'no timestamps'}
        </KV>
        <KV k="SHA-256">{file.sha256}</KV>
        <KV k="links">{file.links.join(', ')}</KV>
        <KV k="MMS ports">{file.ports.join(', ')}</KV>
        {file.truncated_tail && <KV k="tail">the last record was cut short</KV>}
        {file.damaged && <KV k="damaged">{file.damage ?? 'damage ended the read'}</KV>}
        <KV k="analysed by">{s.engine}</KV>
      </dl>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Counts
          title="Content"
          counts={{ associations: s.associations, transactions: s.transactions, reports: s.reports, 'GOOSE streams': s.goose, 'SV streams': s.sv }}
        />
        <Counts title="By status" counts={s.by_status} />
        <Counts title="By service" counts={s.by_service} />
        <Counts title="Findings" counts={s.findings} />
      </div>
      <Counts title="Counters" counts={counterRows} />
    </div>
  );
}

function FindingsView({ service, id, onError }: ViewProps) {
  const [severity, setSeverity] = useState<TraceSeverity | ''>('');
  const [offset, setOffset] = useState(0);
  const data = useLoad<TraceFindings>(
    useCallback(() => service.traceFindings(id, { severity: severity || undefined, offset, limit: PAGE }), [service, id, severity, offset]),
    onError,
  );
  return (
    <div role="region" aria-label="Findings">
      <div className="mb-2 flex flex-wrap items-end gap-3">
        <Field label="Severity">
          <select
            aria-label="Severity"
            className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
            value={severity}
            onChange={(e) => {
              setSeverity(e.target.value as TraceSeverity | '');
              setOffset(0);
            }}
          >
            <option value="">all</option>
            {TRACE_SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {s} ({data?.by_severity[s] ?? 0})
              </option>
            ))}
          </select>
        </Field>
        {data && <Pager total={data.total} offset={offset} onOffset={setOffset} />}
      </div>
      {data && data.findings.length === 0 && <p className="text-muted">No findings.</p>}
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-left">
            {['severity', 'kind', 'message', 'where', 'source'].map((h) => (
              <th key={h} className="ot-label py-1 pr-3">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data?.findings.map((f, i) => (
            <tr key={`${offset + i}`} className="border-b border-line align-top">
              <td className={`py-1 pr-3 ${SEVERITY_TONE[f.severity] ?? ''}`}>{f.severity}</td>
              <td className="py-1 pr-3 font-mono">{f.kind}</td>
              <td className="py-1 pr-3">
                {f.message}
                {f.count ? <span className="text-muted"> (×{f.count})</span> : null}
              </td>
              <td className="py-1 pr-3 font-mono text-[12px] text-muted">
                {[
                  f.association ? `assoc ${f.association}` : '',
                  f.frame ? `frame ${f.frame}` : '',
                  f.transaction ? `#${f.transaction}` : '',
                  f.stream ?? '',
                  time(f.at),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </td>
              <td className="py-1 font-mono text-[12px] text-muted">{f.source}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Pager({ total, offset, onOffset }: { total: number; offset: number; onOffset: (o: number) => void }) {
  return (
    <div className="flex items-center gap-2 text-muted">
      <span>
        {total === 0 ? 0 : offset + 1}–{Math.min(offset + PAGE, total)} of {total}
      </span>
      <Button disabled={offset === 0} onClick={() => onOffset(Math.max(0, offset - PAGE))}>
        Previous
      </Button>
      <Button disabled={offset + PAGE >= total} onClick={() => onOffset(offset + PAGE)}>
        Next
      </Button>
    </div>
  );
}

/** A datetime-local value (UTC) as an RFC 3339 time, or undefined. */
function rfc3339(local: string): string | undefined {
  if (!local) {
    return undefined;
  }
  const d = new Date(`${local}Z`);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

function TimelineView({ service, id, onError }: ViewProps) {
  const [associations, setAssociations] = useState<TraceAssociation[]>([]);
  const [association, setAssociation] = useState(0);
  const [svc, setSvc] = useState('');
  const [statuses, setStatuses] = useState<string[]>([]);
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [offset, setOffset] = useState(0);
  const [open, setOpen] = useState<number | null>(null);

  useEffect(() => {
    service.traceAssociations(id).then(setAssociations, onError);
  }, [service, id, onError]);

  const query: TimelineQuery = {
    ...(association ? { association } : {}),
    ...(svc.trim() ? { service: svc.trim() } : {}),
    ...(statuses.length ? { status: statuses } : {}),
    ...(rfc3339(from) ? { from: rfc3339(from) } : {}),
    ...(rfc3339(to) ? { to: rfc3339(to) } : {}),
    offset,
    limit: PAGE,
  };
  const key = JSON.stringify(query);
  const data = useLoad<Timeline>(useCallback(() => service.traceTimeline(id, JSON.parse(key) as TimelineQuery), [service, id, key]), onError);
  const reset = () => setOffset(0);

  return (
    <div role="region" aria-label="Timeline">
      <div className="mb-2 flex flex-wrap items-end gap-3">
        <Field label="Association">
          <select
            aria-label="Association"
            className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
            value={association}
            onChange={(e) => {
              setAssociation(Number(e.target.value));
              reset();
            }}
          >
            <option value={0}>all</option>
            {associations.map((a) => (
              <option key={a.id} value={a.id}>
                {a.id}: {a.client} → {a.server}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Service">
          <TextInput
            aria-label="Service"
            value={svc}
            placeholder="Read"
            className="w-36"
            onChange={(e) => {
              setSvc(e.target.value);
              reset();
            }}
          />
        </Field>
        <Field label="From (UTC)">
          <TextInput
            aria-label="From"
            type="datetime-local"
            step="0.001"
            value={from}
            onChange={(e) => {
              setFrom(e.target.value);
              reset();
            }}
          />
        </Field>
        <Field label="To (UTC)">
          <TextInput
            aria-label="To"
            type="datetime-local"
            step="0.001"
            value={to}
            onChange={(e) => {
              setTo(e.target.value);
              reset();
            }}
          />
        </Field>
        <fieldset className="flex flex-wrap items-center gap-2" aria-label="Status">
          {TRACE_STATUSES.map((s) => (
            <Toggle
              key={s}
              label={s}
              checked={statuses.includes(s)}
              onChange={(on) => {
                setStatuses(on ? [...statuses, s] : statuses.filter((x) => x !== s));
                reset();
              }}
            />
          ))}
        </fieldset>
        {data && <Pager total={data.total} offset={offset} onOffset={setOffset} />}
      </div>
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-left">
            {['#', 'time', 'assoc', 'invoke', 'service', 'status', 'latency', 'objects', 'error'].map((h) => (
              <th key={h} className="ot-label py-1 pr-3">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data?.transactions.map((t) => (
            <TransactionRow key={t.index} t={t} open={open === t.index} onToggle={() => setOpen(open === t.index ? null : t.index)} />
          ))}
        </tbody>
      </table>
      {data && data.total === 0 && <p className="text-muted">No transaction matches.</p>}
    </div>
  );
}

function TransactionRow({ t, open, onToggle }: { t: TraceTransaction; open: boolean; onToggle: () => void }) {
  return (
    <>
      <tr className="cursor-pointer border-b border-line align-top hover:bg-surface" onClick={onToggle} aria-expanded={open} data-testid="transaction">
        <td className="py-1 pr-3 font-mono">{t.index}</td>
        <td className="py-1 pr-3 font-mono text-[12px]">{time(t.request_at ?? t.response_at)}</td>
        <td className="py-1 pr-3 font-mono">{t.association}</td>
        <td className="py-1 pr-3 font-mono">{t.invoke_id}</td>
        <td className="py-1 pr-3">
          {t.service}
          {t.server_request && <span className="text-muted"> (server)</span>}
        </td>
        <td className={`py-1 pr-3 ${STATUS_TONE[t.status] ?? ''}`}>{t.status}</td>
        <td className="py-1 pr-3 font-mono">{t.latency_ms !== undefined ? `${t.latency_ms.toFixed(2)} ms` : ''}</td>
        <td className="py-1 pr-3 font-mono text-[12px]">
          {t.objects.map((o, i) => (
            <ObjectName key={i} o={o} />
          ))}
        </td>
        <td className="py-1 text-danger">{t.error}</td>
      </tr>
      {open && (
        <tr className="border-b border-line">
          <td colSpan={9} className="py-2 pl-6">
            <div className="grid gap-4 lg:grid-cols-2">
              <PDUView title={`Request${t.request_frame ? ` (frame ${t.request_frame})` : ''}`} pdu={t.request} />
              <PDUView title={`Response${t.response_frame ? ` (frame ${t.response_frame})` : ''}`} pdu={t.response} />
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

const RESOLUTION_TONE: Record<string, 'accent' | 'wire' | 'neutral'> = { found: 'accent', missing: 'wire', 'not-iec61850': 'neutral' };

/** An object name: its reference (or MMS name), and with a link its resolution and what it concerns. */
export function ObjectName({ o }: { o: TraceObject }) {
  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="text-fg-strong" title={`${o.mms.domain}/${o.mms.item_id}`}>
        {o.ref ?? `${o.mms.domain ? `${o.mms.domain}/` : ''}${o.mms.item_id}`}
      </span>
      {o.list && <Chip>list</Chip>}
      {o.model && (
        <Chip tone={RESOLUTION_TONE[o.model] ?? 'neutral'} title="Resolution against the linked model">
          {o.model === 'missing'
            ? 'not in the model'
            : o.model === 'unknown'
              ? 'no definitions in the linked session'
              : o.model === 'not-iec61850'
                ? 'not an IEC 61850 name'
                : o.model}
        </Chip>
      )}
      {o.concerns?.map((c) => (
        <span key={c} className="text-muted">
          {c}
        </span>
      ))}
    </div>
  );
}

/** A decoded MMS PDU. */
export function PDUView({ title, pdu }: { title: string; pdu?: WirePDU }) {
  return (
    <div className="min-w-0">
      <p className="ot-label mb-1">{title}</p>
      {!pdu ? (
        <p className="text-muted">not captured</p>
      ) : (
        <div className="flex flex-col gap-1 font-mono text-[12px]">
          <span>
            {pdu.kind}
            {pdu.service ? ` · ${pdu.service}` : ''}
            {pdu.invoke_id !== undefined ? ` · invoke ${pdu.invoke_id}` : ''}
          </span>
          {pdu.objects?.map((o, i) => (
            <ObjectName key={i} o={o} />
          ))}
          {pdu.objects_dropped ? <span className="text-muted">… {pdu.objects_dropped} more names</span> : null}
          {pdu.identity && (
            <span>
              {pdu.identity.vendor} · {pdu.identity.model} · {pdu.identity.revision}
            </span>
          )}
          {pdu.name_list && (
            <span className="break-all">
              {pdu.name_list.join(', ')}
              {pdu.name_list_more ? ' …more follows' : ''}
            </span>
          )}
          {pdu.write_results && <span>write results: {pdu.write_results.join(', ')}</span>}
          {pdu.initiate && (
            <span>
              initiate: version {pdu.initiate.version}, local detail {pdu.initiate.local_detail}, outstanding {pdu.initiate.max_serv_outstanding_calling}/
              {pdu.initiate.max_serv_outstanding_called}, nesting {pdu.initiate.data_structure_nesting}
            </span>
          )}
          {pdu.error && <span className="text-danger">{pdu.error}</span>}
          {pdu.decode_error && <span className="text-danger">decode error: {pdu.decode_error}</span>}
          {pdu.raw && <span className="break-all text-muted">raw {pdu.raw}</span>}
          {pdu.values && pdu.values.length > 0 && (
            <table>
              <tbody>
                {pdu.values.map((v, i) => (
                  <ValueView key={i} value={v} />
                ))}
              </tbody>
            </table>
          )}
          {pdu.values_dropped ? <span className="text-muted">… {pdu.values_dropped} more values</span> : null}
        </div>
      )}
    </div>
  );
}

function AssociationsView({ service, id, onError }: ViewProps) {
  const data = useLoad<TraceAssociation[]>(useCallback(() => service.traceAssociations(id), [service, id]), onError);
  return (
    <div role="region" aria-label="Associations">
      {data?.length === 0 && <p className="text-muted">No MMS association in this capture.</p>}
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-left">
            {['id', 'client', 'server', 'TSAPs', 'open', 'close', 'AARE', 'authentication', 'initiate', 'transactions', 'unanswered', 'gaps', 'time'].map(
              (h) => (
                <th key={h} className="ot-label py-1 pr-3">
                  {h}
                </th>
              ),
            )}
          </tr>
        </thead>
        <tbody>
          {data?.map((a) => (
            <tr key={a.id} className="border-b border-line align-top font-mono text-[12px]">
              <td className="py-1 pr-3">{a.id}</td>
              <td className="py-1 pr-3">{a.client}</td>
              <td className="py-1 pr-3">{a.server}</td>
              <td className="py-1 pr-3">{a.calling_tsap || a.called_tsap ? `${a.calling_tsap ?? '-'} → ${a.called_tsap ?? '-'}` : ''}</td>
              <td className={`py-1 pr-3 ${a.open === 'midstream' ? 'text-wire' : ''}`}>{a.open}</td>
              <td className={`py-1 pr-3 ${a.close === '' ? 'text-wire' : ''}`}>{a.close || 'not seen'}</td>
              <td className={`py-1 pr-3 ${a.accepted === false ? 'text-danger' : ''}`}>
                {a.accepted === undefined ? '' : a.accepted ? 'accepted' : 'rejected'}
              </td>
              <td className="py-1 pr-3">{a.authentication}</td>
              <td className="py-1 pr-3">
                {a.initiate &&
                  `v${a.initiate.version} · ${a.initiate.local_detail} B · ${a.initiate.max_serv_outstanding_calling}/${a.initiate.max_serv_outstanding_called} · nest ${a.initiate.data_structure_nesting}`}
              </td>
              <td className="py-1 pr-3">
                {a.transactions} ({a.reports} reports)
              </td>
              <td className={`py-1 pr-3 ${a.unanswered ? 'text-wire' : ''}`}>{a.unanswered}</td>
              <td className={`py-1 pr-3 ${a.gaps ? 'text-danger' : ''}`}>{a.gaps}</td>
              <td className="py-1">
                {time(a.first)} – {time(a.last)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ReportsView({ service, id, onError }: ViewProps) {
  const [offset, setOffset] = useState(0);
  const data = useLoad<TraceReports>(useCallback(() => service.traceReports(id, { offset, limit: PAGE }), [service, id, offset]), onError);
  return (
    <div role="region" aria-label="Captured reports">
      {data && <Pager total={data.total} offset={offset} onOffset={setOffset} />}
      {data?.total === 0 && <p className="text-muted">No InformationReport in this capture.</p>}
      <ol className="flex flex-col gap-3">
        {data?.reports.map((r) => (
          <li key={r.index} className="rounded border border-line p-2">
            <p className="flex flex-wrap items-center gap-2 font-mono text-[12px]">
              <span className="text-fg-strong">#{r.index}</span>
              <span className="text-muted">
                {time(r.at)} · assoc {r.association} · frame {r.frame}
              </span>
              {r.rpt_id && <span>RptID {r.rpt_id}</span>}
              {r.rcb && <span>RCB {r.rcb}</span>}
              {r.dat_set && <span>data set {r.dat_set}</span>}
              {r.seq_num !== undefined && <span>SqNum {r.seq_num}</span>}
              <Chip tone={r.decoded ? 'accent' : 'neutral'}>{r.decoded ? 'decoded' : 'raw'}</Chip>
            </p>
            {r.note && <p className="text-[12px] text-muted">{r.note}</p>}
            {r.decode_error && <p className="text-[12px] text-danger">{r.decode_error}</p>}
            {r.objects?.map((o, i) => (
              <ObjectName key={i} o={o} />
            ))}
            <table className="mt-1">
              <tbody>
                {r.entries?.map((e, i) => (
                  <ValueView key={`e${i}`} value={{ ...e.value, name: `${e.ref}${e.reasons?.length ? ` (${e.reasons.join(', ')})` : ''}` }} />
                ))}
                {r.values?.map((v, i) => (
                  <ValueView key={`v${i}`} value={v} />
                ))}
              </tbody>
            </table>
          </li>
        ))}
      </ol>
    </div>
  );
}

function StreamsView({ service, id, onError }: ViewProps) {
  const data = useLoad<TraceStreams>(useCallback(() => service.traceStreams(id), [service, id]), onError);
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  return (
    <div role="region" aria-label="Streams" className="flex flex-col gap-4">
      <div>
        <h3 className="ot-label mb-1">GOOSE</h3>
        {data.goose.length === 0 && <p className="text-muted">No GOOSE stream.</p>}
        <ul className="flex flex-col gap-2">
          {data.goose.map((g) => (
            <li key={`${g.src}|${g.app_id}|${g.gocb_ref}`} className="rounded border border-line p-2 font-mono text-[12px]">
              <p className="flex flex-wrap items-center gap-2">
                <span className="text-fg-strong">{g.gocb_ref}</span>
                {g.model && (
                  <Chip tone={RESOLUTION_TONE[g.model] ?? 'neutral'}>{g.model === 'missing' ? 'not in the model' : g.model === 'unknown' ? 'model has no GoCBs' : g.model}</Chip>
                )}
              </p>
              <p className="text-muted">
                {g.src} → {g.dst} · APPID 0x{g.app_id.toString(16).padStart(4, '0')} · VLAN {g.vlan < 0 ? 'none' : g.vlan} · data set {g.dat_set} · goID {g.go_id} ·
                confRev {g.conf_rev}
              </p>
              <p>
                {g.messages} messages · {g.changes} state changes · max {g.max_per_second}/s
                {g.simulation ? ` · ${g.simulation} simulation` : ''}
                {g.nds_com ? ` · ${g.nds_com} ndsCom` : ''}
              </p>
              {g.note && <p className="text-muted">{g.note}</p>}
              {Object.keys(g.issues).length > 0 && (
                <p className="text-wire">
                  issues:{' '}
                  {Object.entries(g.issues)
                    .map(([k, v]) => `${k} ×${v}`)
                    .join(', ')}
                </p>
              )}
            </li>
          ))}
        </ul>
      </div>
      <div>
        <h3 className="ot-label mb-1">Sampled Values</h3>
        {data.sv.length === 0 && <p className="text-muted">No Sampled Values stream.</p>}
        <ul className="flex flex-col gap-2">
          {data.sv.map((s) => (
            <li key={`${s.src}|${s.app_id}|${s.sv_id}`} className="rounded border border-line p-2 font-mono text-[12px]">
              <p className="text-fg-strong">{s.sv_id}</p>
              <p className="text-muted">
                {s.src} → {s.dst} · APPID 0x{s.app_id.toString(16).padStart(4, '0')} · data set {s.dat_set} · confRev {s.conf_rev}
              </p>
              <p className={s.lost || s.repeated ? 'text-wire' : ''}>
                {s.asdus} samples in {s.messages} frames · lost {s.lost} · repeated {s.repeated} · wrap {s.wrap}
              </p>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
