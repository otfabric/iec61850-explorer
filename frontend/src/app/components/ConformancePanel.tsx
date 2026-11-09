import { useMemo, useState } from 'react';
import { CheckIcon, ErrorIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { Conformance, FindingSeverity, SessionInfo } from '../service/types';
import { Button, Chip, Field } from './ui';

const SEVERITIES: FindingSeverity[] = ['error', 'warning', 'info'];
const PAGE = 200;

/**
 * Checks the session's model against the IEC 61850 namespace (M3c):
 * missing mandatory data objects, logical node classes and data objects
 * the namespace does not define, CDCs that differ from the namespace's.
 */
export function ConformancePanel({ service, session }: { service: Service; session: SessionInfo }) {
  const [result, setResult] = useState<Conformance | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [severity, setSeverity] = useState<FindingSeverity | 'all'>('all');
  const [shown, setShown] = useState(PAGE);

  async function run() {
    try {
      setBusy(true);
      setError(null);
      setResult(await service.conformance(session.id));
      setShown(PAGE);
    } catch (err) {
      setError(toServiceError(err).message);
    } finally {
      setBusy(false);
    }
  }

  const rows = useMemo(() => (result?.findings ?? []).filter((f) => severity === 'all' || f.severity === severity), [result, severity]);
  const count = (s: FindingSeverity) => result?.findings.filter((f) => f.severity === s).length ?? 0;

  return (
    <section aria-label="Conformance" className="min-h-0 overflow-auto p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-display text-lg font-semibold text-fg-strong">Conformance to the namespace</h2>
        <Button className="ml-auto" variant="primary" icon={<CheckIcon size={14} />} disabled={busy} onClick={() => void run()}>
          {busy ? 'Checking…' : 'Check'}
        </Button>
      </div>
      <p className="mt-1 text-muted">
        Mandatory data objects (presence M), logical node classes and data objects the namespace defines, and, for an SCL file, the CDC of each
        data object. Conditional presence is not evaluated.
      </p>
      {error && (
        <p className="mt-2 flex items-center gap-2 text-danger" role="alert">
          <ErrorIcon size={16} />
          {error}
        </p>
      )}
      {result && (
        <>
          <p className="mt-3 text-muted">
            {result.logical_nodes} logical nodes, {result.data_objects} data objects against {result.namespaces.join(', ')} · {result.duration_ms} ms
          </p>
          <div className="mt-2 flex flex-wrap items-end gap-3" aria-label="Summary">
            {result.findings.length === 0 && <Chip tone="accent">no findings</Chip>}
            {SEVERITIES.filter((s) => count(s) > 0).map((s) => (
              <Chip key={s} tone={s === 'info' ? 'neutral' : 'wire'}>
                {count(s)} {s === 'info' || count(s) === 1 ? s : `${s}s`}
              </Chip>
            ))}
            {result.findings.length > 0 && (
              <Field label="Severity">
                <select
                  aria-label="Severity"
                  className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
                  value={severity}
                  onChange={(event) => setSeverity(event.target.value as FindingSeverity | 'all')}
                >
                  <option value="all">All</option>
                  {SEVERITIES.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </Field>
            )}
            {result.truncated && <span className="text-muted">the check stopped at {result.findings.length} findings</span>}
          </div>
          {rows.length > 0 && (
            <table className="mt-2 w-full table-fixed text-[13px]">
              <thead>
                <tr className="ot-label text-left">
                  <th className="w-24 py-1 pr-2">Severity</th>
                  <th className="w-[38%] py-1 pr-2">Reference</th>
                  <th className="py-1 pr-2">Finding</th>
                </tr>
              </thead>
              <tbody>
                {rows.slice(0, shown).map((f, i) => (
                  <tr key={`${f.code}|${f.ref}|${i}`} className="border-t border-line">
                    <td className="py-1 pr-2 align-top">
                      <Chip tone={f.severity === 'info' ? 'neutral' : 'wire'}>{f.severity}</Chip>
                    </td>
                    <td className="py-1 pr-2 align-top font-mono break-all">{f.ref}</td>
                    <td className="py-1 pr-2 align-top">{f.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {rows.length > shown && (
            <Button className="mt-2" onClick={() => setShown((n) => n + PAGE)}>
              Show {Math.min(PAGE, rows.length - shown)} more
            </Button>
          )}
        </>
      )}
    </section>
  );
}
