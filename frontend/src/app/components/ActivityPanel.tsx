import { useState } from 'react';
import { CheckIcon, ErrorIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { Operation, WireView } from '../service/types';
import type { ActivityEntry } from '../state/useExplorer';
import { PDUView } from './TracePanel';
import { Button } from './ui';

/** The Explorer's own activity: each action and the MMS requests it sent. */
export function ActivityPanel({ activity, service }: { activity: ActivityEntry[]; service?: Service }) {
  return (
    <section aria-label="Activity" className="flex min-h-0 flex-col border-t border-line">
      <p className="ot-label px-4 py-2">Activity & diagnostics</p>
      <ol className="min-h-0 flex-1 overflow-auto px-4 pb-2 font-mono text-[12px]">
        {activity.length === 0 && <li className="text-muted">Nothing yet.</li>}
        {activity.map(({ seq, sessionId, operation }) => (
          <li key={seq} className="border-b border-line py-1">
            <details>
              <summary className="flex cursor-pointer items-center gap-2">
                {operation.ok ? <CheckIcon size={14} className="text-accent" /> : <ErrorIcon size={14} className="text-danger" />}
                <span className="text-muted">{new Date(operation.started_at).toLocaleTimeString()}</span>
                <span className="text-fg-strong">{operation.action}</span>
                <span className="truncate">{operation.target}</span>
                <span className="ml-auto text-muted">
                  {operation.requests.length} req · {operation.duration_ms} ms
                </span>
              </summary>
              {operation.error && (
                <p className="py-1 pl-6 text-danger">
                  {operation.error.stage && `[${operation.error.stage}] `}
                  {operation.error.message}
                </p>
              )}
              <ul className="pl-6 text-muted">
                {operation.requests.map((r, i) => (
                  <li key={i}>
                    #{r.invoke_id} {r.service} {r.duration_ms.toFixed(1)} ms{r.error && <span className="text-danger"> {r.error}</span>}
                  </li>
                ))}
                {operation.requests_dropped ? <li>… {operation.requests_dropped} more</li> : null}
              </ul>
              {service && sessionId && operation.wire ? <Wire service={service} sessionId={sessionId} operation={operation} /> : null}
            </details>
          </li>
        ))}
      </ol>
    </section>
  );
}

/** The MMS units an operation sent and received, decoded, with their bytes. */
function Wire({ service, sessionId, operation }: { service: Service; sessionId: string; operation: Operation }) {
  const [view, setView] = useState<WireView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const invokeIds = new Set(operation.requests.map((r) => r.invoke_id));

  async function toggle() {
    if (open) {
      setOpen(false);
      return;
    }
    setOpen(true);
    try {
      setView(await service.wire(sessionId, operation.wire ?? 0));
      setError(null);
    } catch (err) {
      setError(toServiceError(err).message);
    }
  }

  return (
    <div className="pl-6 pt-1">
      <Button aria-pressed={open} onClick={() => void toggle()}>
        Wire
      </Button>
      {open && error && <p className="text-danger">{error}</p>}
      {open && view && (
        <ol aria-label="Wire units" className="mt-1 flex flex-col gap-2">
          {view.note && <li className="text-muted">{view.note}</li>}
          {view.units.length === 0 && <li className="text-muted">No units recorded.</li>}
          {view.units.map((u) => {
            const matched = u.pdu?.invoke_id !== undefined && invokeIds.has(u.pdu.invoke_id);
            return (
              <li key={u.seq} data-matched={matched} className={`rounded border p-2 ${matched ? 'border-accent' : 'border-line'}`}>
                <p className="flex flex-wrap gap-2">
                  <span className={u.direction === 'send' ? 'text-accent' : 'text-blue'}>{u.direction === 'send' ? '→ sent' : '← received'}</span>
                  <span className="text-muted">
                    {u.association} · {u.bytes} bytes · {[u.session, u.presentation, u.acse].filter(Boolean).join(' / ')}
                  </span>
                </p>
                {u.pdu && <PDUView title="PDU" pdu={u.pdu} />}
                {u.decode_error && <p className="text-danger">decode error: {u.decode_error}</p>}
                <pre className="mt-1 font-mono text-[11px] break-all whitespace-pre-wrap text-muted">
                  {u.hex}
                  {u.truncated ? ' …' : ''}
                </pre>
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
