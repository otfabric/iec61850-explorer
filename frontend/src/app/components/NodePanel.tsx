import { CopyIcon, ErrorIcon, PinIcon, ReadIcon } from '../icons';
import type { ModelNode, SessionInfo } from '../service/types';
import type { ReadEntry } from '../state/useExplorer';
import type { ControlRequest, ControlResult, WriteResult } from '../service/types';
import { ControlForm, WriteForm } from './Commission';
import { NamespaceView } from './NamespaceView';
import type { Service } from '../service/service';
import { Button, Chip } from './ui';
import { ValueView } from './ValueView';

interface NodePanelProps {
  service: Service;
  session: SessionInfo;
  node: ModelNode;
  reads: Record<string, ReadEntry>;
  reading: boolean;
  disabled: boolean;
  watched: (ref: string) => boolean;
  onRead: (refs: string[]) => void;
  onWatch: (ref: string, on: boolean) => void;
  onCopy: (text: string) => void;
  /** Writes an attribute; set while the session holds the write capability. */
  onWrite?: (ref: string, value: string, reread: string) => Promise<WriteResult | null>;
  /** Set in Commission mode with the controls capability. */
  onControl?: (req: ControlRequest) => Promise<ControlResult | null>;
}

const withFC = (ref: string, fc: string) => `${ref}[${fc}]`;

/** The functional constraint of the control services; never read (reading SBO selects). */
export const CONTROL_FC = 'CO';

/** The iec61850ctl command that reads the same reference. */
export function cliCommand(session: SessionInfo, ref: string, fc: string): string {
  const port = session.target.port && session.target.port !== 102 ? ` --port ${session.target.port}` : '';
  return `iec61850ctl get object --host ${session.target.host}${port} --object ${ref} --fc ${fc}`;
}

export function NodePanel({ service, session, node, reads, reading, disabled, watched, onRead, onWatch, onCopy, onWrite, onControl }: NodePanelProps) {
  const sclOnly = session.state === 'offline' && !session.snapshot;
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
      {sclOnly && readable && (
        <p className="mt-3 text-muted">This model comes from an SCL file: it has no live values. Connect to the IED to read them.</p>
      )}
      {session.snapshot && readable && <p className="mt-3 text-muted">Values come from the snapshot {session.snapshot.file}, not from the IED.</p>}
      {readable &&
        node.fcs.map((fc) => {
          const ref = withFC(node.ref, fc);
          const entry = reads[ref];
          const control = fc === CONTROL_FC;
          return (
            <div key={fc} className="mt-4 rounded border border-line">
              <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2">
                <span className="ot-label">FC</span>
                <span className="font-mono text-fg-strong">{fc}</span>
                {entry && (
                  <span className="text-[12px] text-muted">
                    {entry.source === 'snapshot' ? 'from snapshot' : 'read'} {new Date(entry.at).toLocaleTimeString()}
                  </span>
                )}
                <span className="ml-auto flex gap-2">
                  {session.target.host && (
                    <Button icon={<CopyIcon size={14} />} onClick={() => onCopy(cliCommand(session, node.ref, fc))} title="Copy the equivalent iec61850ctl command">
                      CLI
                    </Button>
                  )}
                  {session.state === 'connected' && !control && (
                    <Button icon={<PinIcon size={14} />} aria-pressed={watched(ref)} onClick={() => onWatch(ref, !watched(ref))} title="Follow this value in the watch table">
                      {watched(ref) ? 'Watching' : 'Watch'}
                    </Button>
                  )}
                  {!sclOnly && !control && (
                    <Button variant="primary" icon={<ReadIcon size={14} />} disabled={disabled || reading} onClick={() => onRead([ref])}>
                      Read
                    </Button>
                  )}
                </span>
              </div>
              {control && (
                <p className="px-3 py-2 text-muted">
                  Control attributes (Oper, SBO, SBOw, Cancel) are not read: reading SBO selects the control on the IED.
                  {!onControl && ' Grant Commission mode with the controls capability to operate it.'}
                </p>
              )}
              {control && onControl && node.kind === 'do' && <ControlForm key={node.ref} doRef={node.ref} disabled={disabled} onControl={onControl} />}
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
              {onWrite && !node.ref.endsWith('/LLN0.SGCB') && (
                <WriteForm
                  key={`${ref}|${entry?.value.value ? 'read' : 'unread'}`}
                  doRef={node.ref}
                  fc={fc}
                  value={entry?.value.value}
                  disabled={disabled}
                  onWrite={(attr, value) => onWrite(attr, value, ref)}
                />
              )}
            </div>
          );
        })}
      {node.kind !== 'ld' && <NamespaceView service={service} sessionId={session.id} nodeRef={node.ref} />}
    </section>
  );
}
