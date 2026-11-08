import { CheckIcon, ErrorIcon } from '../icons';
import type { ActivityEntry } from '../state/useExplorer';

/** The Explorer's own activity: each action and the MMS requests it sent. */
export function ActivityPanel({ activity }: { activity: ActivityEntry[] }) {
  return (
    <section aria-label="Activity" className="flex min-h-0 flex-col border-t border-line">
      <p className="ot-label px-4 py-2">Activity & diagnostics</p>
      <ol className="min-h-0 flex-1 overflow-auto px-4 pb-2 font-mono text-[12px]">
        {activity.length === 0 && <li className="text-muted">Nothing yet.</li>}
        {activity.map(({ seq, operation }) => (
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
            </details>
          </li>
        ))}
      </ol>
    </section>
  );
}
