import { CloseIcon, PlusIcon } from '../icons';
import type { SessionView } from '../state/useExplorer';

/** A short name for a session: its IED, file or host. */
export function sessionLabel(view: SessionView): string {
  const { info } = view;
  if (info.snapshot) {
    return info.snapshot.file;
  }
  if (info.scl) {
    return `${info.scl.ied} (${info.scl.file})`;
  }
  return info.identity?.model ? `${info.identity.model} ${info.target.host}` : `${info.target.host}:${info.target.port ?? 102}`;
}

const STATE_TONE: Record<string, string> = { connected: 'bg-accent', offline: 'bg-blue', lost: 'bg-danger' };

interface SessionTabsProps {
  sessions: SessionView[];
  active: string | null;
  onActivate: (id: string | null) => void;
  onClose: (id: string) => void;
}

export function SessionTabs({ sessions, active, onActivate, onClose }: SessionTabsProps) {
  return (
    <nav aria-label="Sessions" className="flex shrink-0 items-end gap-1 overflow-x-auto border-b border-line px-2 pt-2">
      {sessions.map((view) => {
        const id = view.info.id;
        const selected = id === active;
        return (
          <div
            key={id}
            className={`flex h-8 items-center gap-2 rounded-t border border-b-0 px-3 text-[13px] ${selected ? 'border-line bg-surface text-fg-strong' : 'border-transparent text-muted'}`}
          >
            <span className={`inline-block size-2 rounded-full ${STATE_TONE[view.info.state] ?? 'bg-muted'}`} title={view.info.state} />
            <button type="button" className="cursor-pointer whitespace-nowrap" aria-current={selected} onClick={() => onActivate(id)}>
              {sessionLabel(view)}
            </button>
            <button type="button" aria-label={`Close ${sessionLabel(view)}`} title="Close" className="cursor-pointer hover:text-accent" onClick={() => onClose(id)}>
              <CloseIcon size={14} />
            </button>
          </div>
        );
      })}
      <button
        type="button"
        className={`flex h-8 cursor-pointer items-center gap-1 rounded-t border border-b-0 px-3 text-[13px] ${active === null ? 'border-line bg-surface text-fg-strong' : 'border-transparent text-muted hover:text-accent'}`}
        onClick={() => onActivate(null)}
      >
        <PlusIcon size={14} />
        New
      </button>
    </nav>
  );
}
