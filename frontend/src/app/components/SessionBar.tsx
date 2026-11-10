import type { ComponentProps, ReactNode } from 'react';
import { CheckIcon, CloseIcon, CompareIcon, DisconnectIcon, FileIcon, UnlockIcon } from '../icons';
import type { SessionInfo } from '../service/types';
import { Button, Chip } from './ui';

/**
 * A button of the session bar. Below 1536 px its label is visually hidden
 * (still its accessible name) and shown as the tooltip, so the bar keeps one
 * line on a 1280 px or 1024 px window.
 */
function BarButton({ label, title, icon, className = '', ...props }: Omit<ComponentProps<typeof Button>, 'children'> & { label: string; icon: ReactNode }) {
  return (
    <Button icon={icon} title={title ?? label} className={`max-2xl:px-2 ${className}`} {...props}>
      <span className="max-2xl:sr-only">{label}</span>
    </Button>
  );
}

const BAR = 'flex shrink-0 items-center gap-3 border-b border-line bg-surface px-4 py-2';
/** The information side: it shrinks and truncates; the actions do not. */
const INFO = 'flex min-w-0 flex-1 items-center gap-3 overflow-hidden whitespace-nowrap';
const ACTIONS = 'flex shrink-0 items-center gap-2';

interface SessionBarProps {
  info: SessionInfo;
  /** The panel shown instead of the selected node. */
  panel: 'files' | 'compare' | 'check' | undefined;
  onToggle: (panel: 'files' | 'compare' | 'check') => void;
  /** Set when the engine offers Commission mode. */
  onCommission?: () => void;
  onClose: () => void;
}

/** What the open tab is: an IED, an SCL model or a snapshot. */
export function SessionBar({ info, panel, onToggle, onCommission, onClose }: SessionBarProps) {
  const { identity, discovery } = info;
  if (info.trace) {
    return (
      <div className={BAR}>
        <div className={INFO}>
          <Chip tone="neutral">capture</Chip>
          <span className="truncate font-mono text-fg-strong" title={info.trace.file}>
            {info.trace.file}
          </span>
          <span className="truncate text-muted">
            {info.trace.format}
            {info.trace.gzip ? ' (gzip)' : ''} · {info.trace.packets} packets · MMS ports {info.trace.ports.join(', ')}
          </span>
          {info.trace.damaged && <Chip tone="wire">damaged: read up to the damage</Chip>}
        </div>
        <div className={ACTIONS} role="toolbar" aria-label="Session actions">
          <BarButton icon={<CloseIcon size={16} />} onClick={onClose} label="Close" title="Close the capture" />
        </div>
      </div>
    );
  }
  const tone = info.state === 'connected' ? 'accent' : info.state === 'offline' ? 'neutral' : 'wire';
  return (
    <div className={BAR}>
      <div className={INFO}>
        {info.target.host && (
          <span className="shrink-0 font-mono text-fg-strong">
            {info.target.host}:{info.target.port ?? 102}
          </span>
        )}
        <Chip tone={tone}>{info.state}</Chip>
        {info.proxy && (
          <span className="truncate font-mono text-muted" title="The proxy this session connected through">
            via {info.proxy}
          </span>
        )}
        {info.scl && (
          <span className="truncate text-muted" title={`Offline: a model from an SCL file. ${info.scl.file} · ${info.scl.ied}/${info.scl.access_point}`}>
            SCL {info.scl.file} · {info.scl.ied}/{info.scl.access_point}
          </span>
        )}
        {info.snapshot && (
          <span className="truncate text-muted" title={`Offline: a snapshot. ${info.snapshot.file}`}>
            Snapshot {info.snapshot.file} · {new Date(info.snapshot.captured_at).toLocaleString()}
          </span>
        )}
        {identity && (
          <span className="truncate text-muted" title={`MMS Identify: ${identity.vendor} · ${identity.model} · ${identity.revision}`}>
            {identity.vendor} · {identity.model} · {identity.revision}
          </span>
        )}
        <span className="truncate text-muted">
          {discovery.logical_devices} LD · {discovery.logical_nodes} LN · {discovery.data_objects} DO
          {info.state === 'connected' && ` · discovered in ${discovery.duration_ms} ms`}
        </span>
        {info.polling && <Chip tone="accent">polling every {info.polling.interval_ms / 1000} s</Chip>}
      </div>
      <div className={ACTIONS} role="toolbar" aria-label="Session actions">
        {panel && (
          <Button onClick={() => onToggle(panel)} title="Back to the selected node">
            Model
          </Button>
        )}
        <BarButton
          icon={<CheckIcon size={16} />}
          aria-pressed={panel === 'check'}
          onClick={() => onToggle('check')}
          label="Check"
          title="Check the model against the IEC 61850 namespace"
        />
        <BarButton
          icon={<CompareIcon size={16} />}
          aria-pressed={panel === 'compare'}
          onClick={() => onToggle('compare')}
          label="Compare"
          title="Compare this session with another: SCL, snapshot or IED"
        />
        {info.state === 'connected' && onCommission && !info.grant && (
          <BarButton icon={<UnlockIcon size={16} />} onClick={onCommission} label="Commission…" title="Commission…: ask for Commission mode, audited changes to this IED" />
        )}
        {info.state === 'connected' && (
          <BarButton icon={<FileIcon size={16} />} aria-pressed={panel === 'files'} onClick={() => onToggle('files')} label="Files" title="Files of the IED" />
        )}
        <BarButton
          icon={info.state === 'connected' ? <DisconnectIcon size={16} /> : <CloseIcon size={16} />}
          onClick={onClose}
          label={info.state === 'connected' ? 'Disconnect' : 'Close'}
          title={info.state === 'connected' ? 'Disconnect from the IED' : 'Close this session'}
        />
      </div>
    </div>
  );
}
