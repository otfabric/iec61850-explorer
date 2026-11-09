import { useEffect, useState } from 'react';
import { CloseIcon, ErrorIcon, PinIcon, PlayIcon, PlusIcon, ReadIcon, RefreshIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import { MAX_DATASET_MEMBERS, type Capability, type DataSet, type JournalResult, type RCB, type ReadResult, type SessionInfo } from '../service/types';
import { summarize } from './summary';
import { Button, Chip, Field, TextInput, Toggle } from './ui';

type Tab = 'datasets' | 'reports' | 'journals' | 'setting-groups';

interface LDPanelProps {
  service: Service;
  session: SessionInfo;
  ld: string;
  disabled: boolean;
  watched: (ref: string) => boolean;
  onWatch: (ref: string, on: boolean) => void;
  /** The watched references of this session, offered as data set members. */
  watchedRefs?: string[];
  /** Shows an error of a Commission action. */
  onError?: (err: unknown) => void;
}

const live = (s: SessionInfo) => s.state === 'connected';

/** Whether the session holds a grant with capability c. */
const can = (s: SessionInfo, c: Capability) => live(s) && (s.grant?.capabilities.includes(c) ?? false);

/** What a logical device defines: data sets, report control blocks, logs, setting groups. */
export function LDPanel({ service, session, ld, disabled, watched, onWatch, watchedRefs = [], onError = () => {} }: LDPanelProps) {
  const [tab, setTab] = useState<Tab>('datasets');
  const tabs: { id: Tab; label: string; live: boolean }[] = [
    { id: 'datasets', label: 'Data sets', live: false },
    { id: 'reports', label: 'Report control blocks', live: false },
    { id: 'journals', label: 'Logs', live: true },
    { id: 'setting-groups', label: 'Setting groups', live: true },
  ];
  const offlineSCL = session.state === 'offline' && session.scl !== undefined && session.snapshot === undefined;
  const available = tabs.filter((t) => (offlineSCL ? !t.live : live(session) || !t.live));
  return (
    <section aria-label="Logical device" className="flex min-h-0 flex-col overflow-auto p-4">
      <h2 className="font-mono text-[15px] text-fg-strong">{ld}</h2>
      {!live(session) && !offlineSCL && <p className="mt-3 text-muted">Connect to the IED to inspect its data sets, reports and logs.</p>}
      {(live(session) || offlineSCL) && (
        <>
          <div role="tablist" className="mt-3 flex gap-4 border-b border-line">
            {available.map((t) => (
              <button
                key={t.id}
                type="button"
                role="tab"
                aria-selected={tab === t.id}
                className={`cursor-pointer pb-1 text-[13px] ${tab === t.id ? 'border-b-2 border-accent text-accent' : 'text-muted hover:text-accent'}`}
                onClick={() => setTab(t.id)}
              >
                {t.label}
              </button>
            ))}
          </div>
          <div className="mt-3">
            {tab === 'datasets' && (
              <DataSets
                key={ld}
                service={service}
                session={session}
                ld={ld}
                disabled={disabled}
                watched={watched}
                onWatch={onWatch}
                watchedRefs={watchedRefs}
                onError={onError}
              />
            )}
            {tab === 'reports' && <Reports key={ld} service={service} session={session} ld={ld} disabled={disabled} onError={onError} />}
            {tab === 'journals' && live(session) && <Journals key={ld} service={service} session={session} ld={ld} disabled={disabled} />}
            {tab === 'setting-groups' && live(session) && (
              <SettingGroupsView key={ld} service={service} session={session} ld={ld} disabled={disabled} onError={onError} />
            )}
          </div>
        </>
      )}
    </section>
  );
}

/** Loads on mount and on reload(); shows errors in place. */
function useLoad<T>(load: () => Promise<T>): { data: T | null; error: string | null; reload: () => void } {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  useEffect(() => {
    let cancelled = false;
    load().then(
      (d) => {
        if (!cancelled) {
          setData(d);
          setError(null);
        }
      },
      (err) => !cancelled && setError(toServiceError(err).message),
    );
    return () => {
      cancelled = true;
    };
  }, [nonce]);
  return { data, error, reload: () => setNonce((n) => n + 1) };
}

/** Runs a Commission action, reports its error and reloads on success. */
async function act(action: () => Promise<unknown>, reload: () => void, onError: (err: unknown) => void): Promise<boolean> {
  try {
    await action();
    reload();
    return true;
  } catch (err) {
    onError(err);
    return false;
  }
}

function Failure({ message }: { message: string }) {
  return (
    <p className="flex items-center gap-2 text-danger" role="alert">
      <ErrorIcon size={16} />
      {message}
    </p>
  );
}

function SourceChip({ source }: { source: string }) {
  return <Chip tone={source === 'scl' ? 'neutral' : 'accent'}>{source === 'scl' ? 'from SCL' : 'from IED'}</Chip>;
}

function DataSets(props: {
  service: Service;
  session: SessionInfo;
  ld: string;
  disabled: boolean;
  watched: (ref: string) => boolean;
  onWatch: (ref: string, on: boolean) => void;
  watchedRefs: string[];
  onError: (err: unknown) => void;
}) {
  const { data, error, reload } = useLoad(() => props.service.dataSets(props.session.id, props.ld));
  const editable = can(props.session, 'datasets');
  const [values, setValues] = useState<Record<string, ReadResult>>({});
  const [readError, setReadError] = useState<string | null>(null);
  if (error) {
    return <Failure message={error} />;
  }
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  async function read(ds: DataSet) {
    try {
      setReadError(null);
      const res = await props.service.readDataSet(props.session.id, ds.ref);
      setValues((v) => ({ ...v, [ds.ref]: res }));
    } catch (err) {
      setReadError(toServiceError(err).message);
    }
  }
  return (
    <div className="flex flex-col gap-3">
      {readError && <Failure message={readError} />}
      {data.length === 0 && <p className="text-muted">No data sets in this logical device.</p>}
      {editable && (
        <CreateDataSet
          ld={props.ld}
          watchedRefs={props.watchedRefs}
          disabled={props.disabled}
          onCreate={(ref, members) => act(() => props.service.createDataSet(props.session.id, { ref, members }), reload, props.onError)}
        />
      )}
      {data.map((ds) => {
        const res = values[ds.ref];
        return (
          <div key={ds.ref} className="rounded border border-line">
            <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2">
              <span className="font-mono text-fg-strong">{ds.ref}</span>
              <SourceChip source={ds.source} />
              <span className="text-[12px] text-muted">
                {ds.members.length} members{ds.desc ? ` · ${ds.desc}` : ''}
              </span>
              {live(props.session) && (
                <span className="ml-auto flex gap-2">
                  <Button
                    icon={<PinIcon size={14} />}
                    onClick={() => ds.members.forEach((m) => props.onWatch(m.ref, true))}
                    disabled={ds.members.every((m) => props.watched(m.ref))}
                  >
                    Watch all
                  </Button>
                  <Button variant="primary" icon={<ReadIcon size={14} />} disabled={props.disabled} onClick={() => void read(ds)}>
                    Read
                  </Button>
                  {editable && ds.deletable && (
                    <Button
                      icon={<CloseIcon size={14} />}
                      disabled={props.disabled}
                      title="Delete this data set from the IED (recorded in the audit trail)"
                      onClick={() => void act(() => props.service.deleteDataSet(props.session.id, ds.ref), reload, props.onError)}
                    >
                      Delete
                    </Button>
                  )}
                </span>
              )}
            </div>
            <table className="w-full text-[13px]">
              <tbody>
                {ds.members.map((m, i) => {
                  const v = res?.values[i];
                  const s = v?.value ? summarize(v.value) : null;
                  return (
                    <tr key={`${m.ref}${i}`} className="border-t border-line first:border-t-0">
                      <td className="px-3 py-1 font-mono">{m.ref}</td>
                      <td className="px-3 py-1 font-mono text-[12px] text-muted">{m.mms.item_id}</td>
                      <td className="px-3 py-1 font-mono text-fg-strong">{v?.error ? <span className="text-danger">{v.error.message}</span> : (s?.text ?? '')}</td>
                      <td className="px-3 py-1 text-muted">{s?.quality ? [s.quality.validity, ...s.quality.flags].join(', ') : ''}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        );
      })}
    </div>
  );
}

function Reports(props: { service: Service; session: SessionInfo; ld: string; disabled: boolean; onError: (err: unknown) => void }) {
  const { data, error, reload } = useLoad(() => props.service.reports(props.session.id, props.ld));
  const [gi, setGI] = useState(true);
  const subscribable = can(props.session, 'reports');
  const subs = props.session.grant?.subscriptions ?? [];
  if (error) {
    return <Failure message={error} />;
  }
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  if (data.length === 0) {
    return <p className="text-muted">No report control blocks in this logical device.</p>;
  }
  const rows: [string, (r: RCB) => string][] = [
    ['RptID', (r) => r.rpt_id],
    ['DatSet', (r) => r.dat_set],
    ['ConfRev', (r) => String(r.conf_rev)],
    ['TrgOps', (r) => r.trg_ops.join(', ') || 'none'],
    ['OptFlds', (r) => r.opt_flds.join(', ') || 'none'],
    ['BufTm', (r) => `${r.buf_tm_ms} ms`],
    ['IntgPd', (r) => `${r.intg_pd_ms} ms`],
    ['RptEna', (r) => (r.rpt_ena === undefined ? '' : r.rpt_ena ? 'enabled' : 'disabled')],
    ['Resv / ResvTms', (r) => (r.resv !== undefined ? (r.resv ? 'reserved' : 'free') : r.resv_tms !== undefined ? String(r.resv_tms) : '')],
    ['SqNum', (r) => (r.sq_num === undefined ? '' : String(r.sq_num))],
    ['Owner', (r) => r.owner ?? ''],
    ['EntryID', (r) => r.entry_id ?? ''],
  ];
  return (
    <div className="flex flex-col gap-3">
      {subscribable ? (
        <div className="flex flex-wrap items-center gap-3 text-[12px] text-muted">
          <span>Subscribing reserves (unbuffered) and enables a control block; unsubscribing or revoking the grant disables and releases it.</span>
          <Toggle label="General interrogation on subscribe" checked={gi} onChange={setGI} />
          <Button icon={<RefreshIcon size={14} />} onClick={reload}>
            Refresh
          </Button>
        </div>
      ) : (
        <p className="text-[12px] text-muted">The configuration as read. Subscribing writes to the control block and needs Commission mode with the reports capability.</p>
      )}
      {data.map((r) => (
        <div key={r.ref} className="rounded border border-line">
          <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2">
            <span className="font-mono text-fg-strong">{r.ref}</span>
            <Chip>{r.buffered ? 'buffered (BR)' : 'unbuffered (RP)'}</Chip>
            <SourceChip source={r.source} />
            {(() => {
              const sub = subs.find((s) => s.rcb === r.ref);
              if (sub) {
                return (
                  <>
                    <Chip tone="accent">subscribed since {new Date(sub.since).toLocaleTimeString()}</Chip>
                    <span className="ml-auto flex gap-2">
                      <Button disabled={props.disabled} onClick={() => void act(() => props.service.triggerGI(props.session.id, sub.id), () => {}, props.onError)}>
                        GI
                      </Button>
                      <Button
                        icon={<CloseIcon size={14} />}
                        disabled={props.disabled}
                        onClick={() => void act(() => props.service.unsubscribe(props.session.id, sub.id), reload, props.onError)}
                      >
                        Unsubscribe
                      </Button>
                    </span>
                  </>
                );
              }
              if (r.rpt_ena) {
                return <Chip tone="wire">enabled: another client may own it</Chip>;
              }
              return (
                subscribable &&
                r.source === 'ied' &&
                !r.error && (
                  <Button
                    className="ml-auto"
                    variant="primary"
                    icon={<PlayIcon size={14} />}
                    disabled={props.disabled}
                    onClick={() => void act(() => props.service.subscribeReport(props.session.id, { rcb: r.ref, gi }), reload, props.onError)}
                  >
                    Subscribe
                  </Button>
                )
              );
            })()}
          </div>
          {r.error ? (
            <div className="px-3 py-2">
              <Failure message={r.error.message} />
            </div>
          ) : (
            <table className="w-full text-[13px]">
              <tbody>
                {rows
                  .filter(([, f]) => f(r) !== '')
                  .map(([name, f]) => (
                    <tr key={name} className="border-t border-line first:border-t-0">
                      <td className="ot-label w-40 px-3 py-1">{name}</td>
                      <td className="px-3 py-1 font-mono">{f(r)}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          )}
        </div>
      ))}
    </div>
  );
}

const RANGES = [
  { label: 'last hour', ms: 3_600_000 },
  { label: 'last 24 hours', ms: 86_400_000 },
  { label: 'last 7 days', ms: 7 * 86_400_000 },
];

function Journals(props: { service: Service; session: SessionInfo; ld: string; disabled: boolean }) {
  const { data, error } = useLoad(() => props.service.journals(props.session.id, props.ld));
  const [journal, setJournal] = useState('');
  const [range, setRange] = useState(RANGES[1]?.ms ?? 86_400_000);
  const [result, setResult] = useState<JournalResult | null>(null);
  const [readError, setReadError] = useState<string | null>(null);
  if (error) {
    return <Failure message={error} />;
  }
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  if (data.length === 0) {
    return <p className="text-muted">No logs in this logical device.</p>;
  }
  const chosen = journal || data[0] || '';
  async function read() {
    try {
      setReadError(null);
      const stop = new Date();
      setResult(await props.service.readJournal(props.session.id, { ld: props.ld, journal: chosen, start: new Date(stop.getTime() - range).toISOString(), stop: stop.toISOString() }));
    } catch (err) {
      setReadError(toServiceError(err).message);
    }
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <select aria-label="Log" value={chosen} onChange={(e) => setJournal(e.target.value)} className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong">
          {data.map((j) => (
            <option key={j} value={j}>
              {j}
            </option>
          ))}
        </select>
        <select aria-label="Time range" value={range} onChange={(e) => setRange(Number(e.target.value))} className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong">
          {RANGES.map((r) => (
            <option key={r.ms} value={r.ms}>
              {r.label}
            </option>
          ))}
        </select>
        <Button variant="primary" icon={<ReadIcon size={14} />} disabled={props.disabled} onClick={() => void read()}>
          Read log
        </Button>
      </div>
      {readError && <Failure message={readError} />}
      {result && (
        <>
          <table className="w-full text-[13px]">
            <thead>
              <tr className="ot-label text-left">
                <th className="py-1 pr-2">Time</th>
                <th className="py-1 pr-2">Entry</th>
                <th className="py-1 pr-2">Variable</th>
                <th className="py-1 pr-2">Value</th>
              </tr>
            </thead>
            <tbody>
              {result.entries.flatMap((en) =>
                (en.variables.length ? en.variables : [null]).map((v, i) => (
                  <tr key={`${en.entry_id}-${i}`} className="border-t border-line">
                    <td className="py-1 pr-2 font-mono text-muted">{i === 0 ? new Date(en.time).toISOString().replace('T', ' ').slice(0, 23) : ''}</td>
                    <td className="py-1 pr-2 font-mono text-[12px] text-muted">{i === 0 ? en.entry_id : ''}</td>
                    <td className="py-1 pr-2 font-mono">{v?.tag ?? ''}</td>
                    <td className="py-1 pr-2 font-mono text-fg-strong">{v ? summarize(v.value).text : ''}</td>
                  </tr>
                )),
              )}
            </tbody>
          </table>
          <p className="text-[12px] text-muted">
            {result.entries.length} entries{result.more_follows ? '; the IED has more in this range (narrow it to see them)' : ''}
          </p>
        </>
      )}
    </div>
  );
}

function SettingGroupsView(props: { service: Service; session: SessionInfo; ld: string; disabled: boolean; onError: (err: unknown) => void }) {
  const { data, error, reload } = useLoad(() => props.service.settingGroups(props.session.id, props.ld));
  const [sg, setSG] = useState(0);
  if (error) {
    return <Failure message={error} />;
  }
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  const rows: [string, string][] = [
    ['Setting groups', String(data.num_of_sgs)],
    ['Active group', String(data.act_sg)],
    ['Edit group', data.edit_sg === 0 ? 'none' : String(data.edit_sg)],
    ['Edit confirmed', data.cnf_edit ? 'yes' : 'no'],
    ['Reservation time', `${data.resv_tms} s`],
  ];
  const chosen = sg || (data.act_sg === 1 && data.num_of_sgs > 1 ? 2 : 1);
  return (
    <div className="flex flex-col gap-3">
      <table className="text-[13px]">
        <tbody>
          {rows.map(([k, v]) => (
            <tr key={k} className="border-t border-line first:border-t-0">
              <td className="ot-label w-40 py-1 pr-3">{k}</td>
              <td className="py-1 font-mono text-fg-strong">{v}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {can(props.session, 'setting-groups') && data.num_of_sgs > 1 && (
        <div className="flex flex-wrap items-end gap-2 border-t border-wire pt-3">
          <Field label="Active setting group">
            <select
              aria-label="Active setting group"
              value={chosen}
              onChange={(e) => setSG(Number(e.target.value))}
              className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
            >
              {Array.from({ length: data.num_of_sgs }, (_, i) => i + 1).map((n) => (
                <option key={n} value={n}>
                  {n}
                  {n === data.act_sg ? ' (active)' : ''}
                </option>
              ))}
            </select>
          </Field>
          <Button
            variant="primary"
            disabled={props.disabled || chosen === data.act_sg}
            title="Switch the IED to this setting group (recorded in the audit trail)"
            onClick={() => void act(() => props.service.selectActiveSG(props.session.id, { ld: props.ld, sg: chosen }), reload, props.onError)}
          >
            Activate
          </Button>
          <span className="text-[12px] text-muted">Switching changes which protection settings the IED uses.</span>
        </div>
      )}
    </div>
  );
}

/** A form that creates a data set from references, one per line. */
function CreateDataSet(props: { ld: string; watchedRefs: string[]; disabled: boolean; onCreate: (ref: string, members: string[]) => Promise<boolean> }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [members, setMembers] = useState('');
  const list = members
    .split('\n')
    .map((m) => m.trim())
    .filter(Boolean);
  const valid = /^[A-Za-z][A-Za-z0-9_]{0,31}$/.test(name) && list.length > 0 && list.length <= MAX_DATASET_MEMBERS;
  if (!open) {
    return (
      <div>
        <Button icon={<PlusIcon size={14} />} onClick={() => setOpen(true)}>
          New data set
        </Button>
      </div>
    );
  }
  return (
    <form
      aria-label="New data set"
      className="flex flex-col gap-2 rounded border border-wire p-3"
      onSubmit={(event) => {
        event.preventDefault();
        if (valid) {
          void props.onCreate(`${props.ld}/LLN0.${name}`, list).then((ok) => ok && setOpen(false));
        }
      }}
    >
      <Field label={`Name (created as ${props.ld}/LLN0.<name>)`}>
        <TextInput aria-label="Data set name" value={name} maxLength={32} onChange={(e) => setName(e.target.value)} placeholder="Explorer1" />
      </Field>
      <Field label="Members: FC-qualified references, one per line">
        <textarea
          aria-label="Members"
          rows={4}
          value={members}
          onChange={(e) => setMembers(e.target.value)}
          className="rounded border border-control bg-bg px-2 py-1 font-mono text-[13px] text-fg-strong"
          placeholder={`${props.ld}/MMXU1.Hz[MX]`}
        />
      </Field>
      <div className="flex flex-wrap gap-2">
        {props.watchedRefs.length > 0 && <Button onClick={() => setMembers(props.watchedRefs.join('\n'))}>Use the watched points</Button>}
        <span className="ml-auto flex gap-2">
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={!valid || props.disabled}>
            Create
          </Button>
        </span>
      </div>
    </form>
  );
}
