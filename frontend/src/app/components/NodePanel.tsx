import { CopyIcon, ErrorIcon, ReadIcon } from '../icons';
import type { ModelNode, SessionInfo } from '../service/types';
import type { ReadEntry } from '../state/useExplorer';
import { Button, Chip } from './ui';
import { ValueView } from './ValueView';

interface NodePanelProps {
  session: SessionInfo;
  node: ModelNode;
  reads: Record<string, ReadEntry>;
  reading: string | null;
  disabled: boolean;
  onRead: (refs: string[]) => void;
  onCopy: (text: string) => void;
}

const withFC = (ref: string, fc: string) => `${ref}[${fc}]`;

/** The iec61850ctl command that reads the same reference. */
export function cliCommand(session: SessionInfo, ref: string, fc: string): string {
  const port = session.target.port && session.target.port !== 102 ? ` --port ${session.target.port}` : '';
  return `iec61850ctl get object --host ${session.target.host}${port} --object ${ref} --fc ${fc}`;
}

export function NodePanel({ session, node, reads, reading, disabled, onRead, onCopy }: NodePanelProps) {
  const readable = node.kind !== 'ld' && node.fcs.length > 0;
  return (
    <section aria-label="Selected node" className="min-h-0 overflow-auto p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-mono text-[15px] text-fg-strong">{node.ref}</h2>
        <Chip>{node.kind.toUpperCase()}</Chip>
        {node.ln_class && <Chip tone="accent">{node.ln_class}</Chip>}
        <Button className="ml-auto" icon={<CopyIcon size={14} />} onClick={() => onCopy(node.ref)}>
          Copy reference
        </Button>
      </div>
      {node.kind === 'ld' && <p className="mt-3 text-muted">A logical device. Open it in the tree to see its logical nodes.</p>}
      {node.kind !== 'ld' && node.fcs.length === 0 && <p className="mt-3 text-muted">No functional constraint was discovered for this node.</p>}
      {readable &&
        node.fcs.map((fc) => {
          const ref = withFC(node.ref, fc);
          const entry = reads[ref];
          return (
            <div key={fc} className="mt-4 rounded border border-line">
              <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2">
                <span className="ot-label">FC</span>
                <span className="font-mono text-fg-strong">{fc}</span>
                {entry && <span className="text-[12px] text-muted">read {new Date(entry.at).toLocaleTimeString()}</span>}
                <span className="ml-auto flex gap-2">
                  <Button icon={<CopyIcon size={14} />} onClick={() => onCopy(cliCommand(session, node.ref, fc))} title="Copy the equivalent iec61850ctl command">
                    CLI
                  </Button>
                  <Button variant="primary" icon={<ReadIcon size={14} />} disabled={disabled || reading !== null} onClick={() => onRead([ref])}>
                    Read
                  </Button>
                </span>
              </div>
              {entry?.value.error && (
                <div className="flex gap-2 px-3 py-2 text-danger" role="alert">
                  <ErrorIcon size={16} />
                  <span>
                    {entry.value.error.message}
                    {entry.value.error.hint && <span className="block text-muted">{entry.value.error.hint}</span>}
                  </span>
                </div>
              )}
              {entry?.value.value && (
                <table className="w-full px-3">
                  <tbody>
                    <ValueView value={entry.value.value} />
                  </tbody>
                </table>
              )}
              {entry && (
                <p className="flex items-center gap-2 px-3 py-2 text-[12px] text-muted">
                  <span className="ot-label">MMS</span>
                  <span className="font-mono">
                    {entry.value.mms.domain} / {entry.value.mms.item_id}
                  </span>
                  <button
                    type="button"
                    className="cursor-pointer hover:text-accent"
                    aria-label="Copy MMS item ID"
                    title="Copy MMS item ID"
                    onClick={() => onCopy(entry.value.mms.item_id)}
                  >
                    <CopyIcon size={14} />
                  </button>
                </p>
              )}
            </div>
          );
        })}
    </section>
  );
}
