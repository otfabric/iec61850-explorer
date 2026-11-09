import { useEffect, useState } from 'react';
import { ErrorIcon, PinIcon, ReadIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { DataSet, JournalResult, RCB, ReadResult, SessionInfo } from '../service/types';
import { summarize } from './summary';
import { Button, Chip } from './ui';

type Tab = 'datasets' | 'reports' | 'journals' | 'setting-groups';

interface LDPanelProps {
  service: Service;
  session: SessionInfo;
  ld: string;
  disabled: boolean;
  watched: (ref: string) => boolean;
  onWatch: (ref: string, on: boolean) => void;
}

const live = (s: SessionInfo) => s.state === 'connected';

/** What a logical device defines: data sets, report control blocks, logs, setting groups. */
export function LDPanel({ service, session, ld, disabled, watched, onWatch }: LDPanelProps) {
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
            {tab === 'datasets' && <DataSets key={ld} service={service} session={session} ld={ld} disabled={disabled} watched={watched} onWatch={onWatch} />}
            {tab === 'reports' && <Reports key={ld} service={service} session={session} ld={ld} />}
            {tab === 'journals' && live(session) && <Journals key={ld} service={service} session={session} ld={ld} disabled={disabled} />}
            {tab === 'setting-groups' && live(session) && <SettingGroupsView key={ld} service={service} session={session} ld={ld} />}
          </div>
        </>
      )}
    </section>
  );
}

/** Loads once per mount; shows errors in place. */
function useLoad<T>(load: () => Promise<T>): { data: T | null; error: string | null } {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    load().then(
      (d) => !cancelled && setData(d),
      (err) => !cancelled && setError(toServiceError(err).message),
    );
    return () => {
      cancelled = true;
    };
  }, []);
  return { data, error };
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

function DataSets(props: { service: Service; session: SessionInfo; ld: string; disabled: boolean; watched: (ref: string) => boolean; onWatch: (ref: string, on: boolean) => void }) {
  const { data, error } = useLoad(() => props.service.dataSets(props.session.id, props.ld));
  const [values, setValues] = useState<Record<string, ReadResult>>({});
  const [readError, setReadError] = useState<string | null>(null);
  if (error) {
    return <Failure message={error} />;
  }
  if (!data) {
    return <p className="text-muted">Loading…</p>;
  }
  if (data.length === 0) {
    return <p className="text-muted">No data sets in this logical device.</p>;
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

function Reports(props: { service: Service; session: SessionInfo; ld: string }) {
  const { data, error } = useLoad(() => props.service.reports(props.session.id, props.ld));
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
      <p className="text-[12px] text-muted">
        The configuration as read. Subscribing to a report writes to the control block and needs Commission mode, which this version does not have.
      </p>
      {data.map((r) => (
        <div key={r.ref} className="rounded border border-line">
          <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2">
            <span className="font-mono text-fg-strong">{r.ref}</span>
            <Chip>{r.buffered ? 'buffered (BR)' : 'unbuffered (RP)'}</Chip>
            <SourceChip source={r.source} />
            {r.rpt_ena && <Chip tone="wire">enabled: another client may own it</Chip>}
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

function SettingGroupsView(props: { service: Service; session: SessionInfo; ld: string }) {
  const { data, error } = useLoad(() => props.service.settingGroups(props.session.id, props.ld));
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
  return (
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
  );
}
