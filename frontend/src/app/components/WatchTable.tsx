import { useState } from 'react';
import { CloseIcon, ExportIcon, PlayIcon, ReadIcon, StopIcon } from '../icons';
import type { ExportFormat } from '../service/types';
import { watchKey, type ReadEntry, type WatchEntry } from '../state/useExplorer';
import { summarize } from './summary';
import { Button } from './ui';

const INTERVALS = [
  { ms: 500, label: '0.5 s' },
  { ms: 1000, label: '1 s' },
  { ms: 2000, label: '2 s' },
  { ms: 5000, label: '5 s' },
  { ms: 10000, label: '10 s' },
  { ms: 60000, label: '1 min' },
];

const QUALITY_TONE: Record<string, string> = { good: 'text-accent', questionable: 'text-wire', invalid: 'text-danger', reserved: 'text-danger' };

interface WatchTableProps {
  entries: WatchEntry[];
  latest: Record<string, ReadEntry>;
  label: (sessionId: string) => string;
  polling: boolean;
  /** Some watched session can be read (is connected). */
  live: boolean;
  disabled: boolean;
  activeSessionId: string | null;
  onPoll: (on: boolean, intervalMs: number) => void;
  onReadAll: () => void;
  onRemove: (entry: WatchEntry) => void;
  onExport: (format: ExportFormat) => void;
}

/** Pinned points of every open session, updated by polling or a read. */
export function WatchTable(props: WatchTableProps) {
  const [interval, setInterval] = useState(1000);
  const activeCount = props.entries.filter((e) => e.sessionId === props.activeSessionId).length;
  return (
    <section aria-label="Watch" className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 px-4 py-2">
        <select
          aria-label="Poll interval"
          className="h-8 rounded border border-control bg-bg px-1 text-[13px] text-fg-strong"
          value={interval}
          onChange={(e) => setInterval(Number(e.target.value))}
        >
          {INTERVALS.map((i) => (
            <option key={i.ms} value={i.ms}>
              every {i.label}
            </option>
          ))}
        </select>
        {props.polling ? (
          <Button icon={<StopIcon size={14} />} onClick={() => props.onPoll(false, interval)} disabled={props.disabled}>
            Stop polling
          </Button>
        ) : (
          <Button variant="primary" icon={<PlayIcon size={14} />} onClick={() => props.onPoll(true, interval)} disabled={props.disabled || !props.live || props.entries.length === 0}>
            Poll
          </Button>
        )}
        <Button icon={<ReadIcon size={14} />} onClick={props.onReadAll} disabled={props.disabled || props.entries.length === 0}>
          Read all
        </Button>
        <span className="ml-auto flex gap-2">
          <Button icon={<ExportIcon size={14} />} onClick={() => props.onExport('csv')} disabled={props.disabled || activeCount === 0} title="The watched points of the open tab, read now, as CSV">
            CSV
          </Button>
          <Button icon={<ExportIcon size={14} />} onClick={() => props.onExport('snapshot')} disabled={props.disabled || props.activeSessionId === null} title="The model of the open tab and its watched points, read now">
            Snapshot
          </Button>
          <Button
            icon={<ExportIcon size={14} />}
            onClick={() => props.onExport('points-csv')}
            disabled={props.disabled || props.activeSessionId === null}
            title="Point list of the open tab: every attribute with FC, CDC, type, quantity, data sets and reports (CSV)"
          >
            Points CSV
          </Button>
          <Button
            icon={<ExportIcon size={14} />}
            onClick={() => props.onExport('points-json')}
            disabled={props.disabled || props.activeSessionId === null}
            title="Point list of the open tab as JSON"
          >
            Points JSON
          </Button>
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4">
        {props.entries.length === 0 ? (
          <p className="text-muted">Pin points with “Watch” in the node panel to follow them here, across IEDs.</p>
        ) : (
          <table className="w-full text-[13px]">
            <thead>
              <tr className="ot-label text-left">
                <th className="py-1 pr-2">IED</th>
                <th className="py-1 pr-2">Reference</th>
                <th className="py-1 pr-2">Value</th>
                <th className="py-1 pr-2">Quality</th>
                <th className="py-1 pr-2">Time</th>
                <th className="py-1 pr-2">Updated</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {props.entries.map((entry) => {
                const latest = props.latest[watchKey(entry)];
                const summary = latest?.value.value ? summarize(latest.value.value) : null;
                return (
                  <tr key={watchKey(entry)} className="border-t border-line">
                    <td className="py-1 pr-2 text-muted">{props.label(entry.sessionId)}</td>
                    <td className="py-1 pr-2 font-mono">{entry.ref}</td>
                    <td className="py-1 pr-2 font-mono text-fg-strong">
                      {latest?.value.error ? <span className="text-danger">{latest.value.error.message}</span> : (summary?.text ?? '—')}
                    </td>
                    <td className={`py-1 pr-2 ${summary?.quality ? QUALITY_TONE[summary.quality.validity] : 'text-muted'}`}>
                      {summary?.quality ? [summary.quality.validity, ...summary.quality.flags].join(', ') : '—'}
                    </td>
                    <td className="py-1 pr-2 font-mono text-muted">{summary?.time ? new Date(summary.time.time).toISOString().slice(11, 23) : '—'}</td>
                    <td className="py-1 pr-2 text-muted">
                      {latest ? new Date(latest.at).toLocaleTimeString() : '—'}
                      {latest?.source === 'snapshot' && ' (snapshot)'}
                    </td>
                    <td className="py-1">
                      <button type="button" aria-label={`Stop watching ${entry.ref}`} className="cursor-pointer hover:text-accent" onClick={() => props.onRemove(entry)}>
                        <CloseIcon size={14} />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}
