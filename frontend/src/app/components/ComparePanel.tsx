import { useMemo, useState } from 'react';
import { CompareIcon, ErrorIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { CompareCategoryName, Comparison, Difference, DifferenceKind, SessionInfo } from '../service/types';
import { Button, Chip, Field, Toggle } from './ui';

const CATEGORY_LABEL: Record<CompareCategoryName, string> = {
  model: 'Model',
  datasets: 'Data sets',
  reports: 'Report control blocks',
  values: 'Values',
};
const KIND_LABEL: Record<DifferenceKind, string> = { only_a: 'only in A', only_b: 'only in B', changed: 'differs' };
/** Rows shown before "Show more". */
const PAGE = 200;

interface ComparePanelProps {
  service: Service;
  /** The session of the open tab: B, the subject. */
  session: SessionInfo;
  /** The other open sessions, any of which can be A, the reference. */
  others: SessionInfo[];
  label: (id: string) => string;
}

/**
 * Compares the open session with another one (an SCL file with the IED, a
 * snapshot with the IED, two IEDs): the model, the data sets and the
 * report control blocks. Every difference shows both sides.
 */
export function ComparePanel({ service, session, others, label }: ComparePanelProps) {
  const [reference, setReference] = useState(others[0]?.id ?? '');
  const [result, setResult] = useState<Comparison | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [values, setValues] = useState(false);
  const [category, setCategory] = useState<CompareCategoryName | 'all'>('all');
  const [kind, setKind] = useState<DifferenceKind | 'all'>('all');
  const [shown, setShown] = useState(PAGE);

  async function run() {
    try {
      setBusy(true);
      setError(null);
      setResult(await service.compare({ a: reference, b: session.id, values }));
      setShown(PAGE);
    } catch (err) {
      setError(toServiceError(err).message);
    } finally {
      setBusy(false);
    }
  }

  const rows = useMemo(
    () => (result?.differences ?? []).filter((d) => (category === 'all' || d.category === category) && (kind === 'all' || d.kind === kind)),
    [result, category, kind],
  );

  return (
    <section aria-label="Compare" className="min-h-0 overflow-auto p-4">
      <h2 className="font-display text-lg font-semibold text-fg-strong">Compare</h2>
      {others.length === 0 ? (
        <p className="mt-2 text-muted">Open a second session (an SCL file, a snapshot or another IED) to compare it with this one.</p>
      ) : (
        <div className="mt-2 flex flex-wrap items-end gap-3">
          <Field label="A: reference">
            <select
              className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
              value={reference}
              onChange={(event) => setReference(event.target.value)}
            >
              {others.map((o) => (
                <option key={o.id} value={o.id}>
                  {label(o.id)}
                </option>
              ))}
            </select>
          </Field>
          <Field label="B: this session">
            <span className="flex h-8 items-center font-mono text-[13px] text-fg-strong">{label(session.id)}</span>
          </Field>
          <span className="flex h-8 items-center" title="Configuration, description, set point and setting values (FC CF, DC, SP, SG). From an IED, this reads them.">
            <Toggle label="Values" checked={values} onChange={setValues} />
          </span>
          <Button variant="primary" icon={<CompareIcon size={14} />} disabled={busy || !reference} onClick={() => void run()}>
            {busy ? 'Comparing…' : 'Compare'}
          </Button>
        </div>
      )}
      {error && (
        <p className="mt-2 flex items-center gap-2 text-danger" role="alert">
          <ErrorIcon size={16} />
          {error}
        </p>
      )}
      {result && (
        <>
          <p className="mt-3 text-muted">
            A: {result.a.source} {result.a.label} · B: {result.b.source} {result.b.label} · {result.duration_ms} ms
          </p>
          <ul className="mt-2 flex flex-col gap-1" aria-label="Summary">
            {result.categories.map((c) => (
              <li key={c.name} className="flex flex-wrap items-center gap-2 text-[13px]">
                <span className="w-44 font-medium text-fg-strong">{CATEGORY_LABEL[c.name]}</span>
                {c.compared ? (
                  <>
                    <Chip tone={c.only_a + c.only_b + c.changed === 0 ? 'accent' : 'wire'}>
                      {c.same} of {c.items} equal
                    </Chip>
                    {c.only_a > 0 && <Chip tone="wire">{c.only_a} only in A</Chip>}
                    {c.only_b > 0 && <Chip tone="wire">{c.only_b} only in B</Chip>}
                    {c.changed > 0 && <Chip tone="wire">{c.changed} differ</Chip>}
                    {c.reason && <span className="text-muted">partly: {c.reason}</span>}
                  </>
                ) : (
                  <span className="text-muted">not compared: {c.reason}</span>
                )}
              </li>
            ))}
          </ul>
          {result.differences.length === 0 ? (
            <p className="mt-3 text-fg-strong">No differences in what was compared.</p>
          ) : (
            <>
              <div className="mt-3 flex flex-wrap items-end gap-3">
                <Field label="Category">
                  <select
                    aria-label="Category"
                    className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
                    value={category}
                    onChange={(event) => setCategory(event.target.value as CompareCategoryName | 'all')}
                  >
                    <option value="all">All</option>
                    {(Object.keys(CATEGORY_LABEL) as CompareCategoryName[]).map((c) => (
                      <option key={c} value={c}>
                        {CATEGORY_LABEL[c]}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Difference">
                  <select
                    aria-label="Difference"
                    className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
                    value={kind}
                    onChange={(event) => setKind(event.target.value as DifferenceKind | 'all')}
                  >
                    <option value="all">All</option>
                    {(Object.keys(KIND_LABEL) as DifferenceKind[]).map((k) => (
                      <option key={k} value={k}>
                        {KIND_LABEL[k]}
                      </option>
                    ))}
                  </select>
                </Field>
                <span className="pb-1.5 text-muted">
                  {rows.length} of {result.differences.length} differences
                  {result.truncated && ' (the comparison stopped at this many)'}
                </span>
              </div>
              <table className="mt-2 w-full table-fixed text-[13px]">
                <thead>
                  <tr className="ot-label text-left">
                    <th className="w-28 py-1 pr-2">Difference</th>
                    <th className="w-[34%] py-1 pr-2">Reference</th>
                    <th className="w-24 py-1 pr-2">Field</th>
                    <th className="py-1 pr-2">A</th>
                    <th className="py-1 pr-2">B</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.slice(0, shown).map((d, i) => (
                    <DifferenceRow key={`${d.category}|${d.kind}|${d.ref}|${d.field ?? ''}|${i}`} d={d} />
                  ))}
                </tbody>
              </table>
              {rows.length > shown && (
                <Button className="mt-2" onClick={() => setShown((n) => n + PAGE)}>
                  Show {Math.min(PAGE, rows.length - shown)} more
                </Button>
              )}
            </>
          )}
        </>
      )}
    </section>
  );
}

function DifferenceRow({ d }: { d: Difference }) {
  const cell = 'py-1 pr-2 align-top';
  const value = (v?: string) => (v ? <pre className="max-h-40 overflow-auto font-mono text-[12px] whitespace-pre-wrap break-all">{v}</pre> : null);
  return (
    <tr className="border-t border-line">
      <td className={cell}>
        <Chip tone={d.kind === 'changed' ? 'wire' : 'neutral'}>{KIND_LABEL[d.kind]}</Chip>
      </td>
      <td className={`${cell} font-mono break-all`}>
        {d.ref}
        {d.note && <div className="text-[12px] text-muted">{d.note}</div>}
      </td>
      <td className={`${cell} font-mono`}>{d.field}</td>
      <td className={cell}>{value(d.a)}</td>
      <td className={cell}>{value(d.b)}</td>
    </tr>
  );
}
