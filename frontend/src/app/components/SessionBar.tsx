import { CheckIcon, CloseIcon, CompareIcon, DisconnectIcon, FileIcon, UnlockIcon } from '../icons';
import type { SessionInfo } from '../service/types';
import { Button, Chip } from './ui';

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
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-line bg-surface px-4 py-2">
        <Chip tone="neutral">capture</Chip>
        <span className="font-mono text-fg-strong">{info.trace.file}</span>
        <span className="text-muted">
          {info.trace.format}
          {info.trace.gzip ? ' (gzip)' : ''} · {info.trace.packets} packets · MMS ports {info.trace.ports.join(', ')}
        </span>
        {info.trace.damaged && <Chip tone="wire">damaged: read up to the damage</Chip>}
        <Button className="ml-auto" icon={<CloseIcon size={16} />} onClick={onClose}>
          Close
        </Button>
      </div>
    );
  }
  const tone = info.state === 'connected' ? 'accent' : info.state === 'offline' ? 'neutral' : 'wire';
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-line bg-surface px-4 py-2">
      {info.target.host && (
        <span className="font-mono text-fg-strong">
          {info.target.host}:{info.target.port ?? 102}
        </span>
      )}
      <Chip tone={tone}>{info.state}</Chip>
      {info.proxy && (
        <span className="font-mono text-muted" title="The proxy this session connected through">
          via {info.proxy}
        </span>
      )}
      {info.scl && (
        <span className="text-muted" title="Offline: a model from an SCL file">
          SCL {info.scl.file} · {info.scl.ied}/{info.scl.access_point}
        </span>
      )}
      {info.snapshot && (
        <span className="text-muted" title="Offline: a snapshot">
          Snapshot {info.snapshot.file} · {new Date(info.snapshot.captured_at).toLocaleString()}
        </span>
      )}
      {identity && (
        <span className="text-muted" title="MMS Identify">
          {identity.vendor} · {identity.model} · {identity.revision}
        </span>
      )}
      <span className="text-muted">
        {discovery.logical_devices} LD · {discovery.logical_nodes} LN · {discovery.data_objects} DO
        {info.state === 'connected' && ` · discovered in ${discovery.duration_ms} ms`}
      </span>
      {info.polling && <Chip tone="accent">polling every {info.polling.interval_ms / 1000} s</Chip>}
      {panel && (
        <Button className="ml-auto" onClick={() => onToggle(panel)} title="Back to the selected node">
          Model
        </Button>
      )}
      <Button
        className={panel ? '' : 'ml-auto'}
        icon={<CheckIcon size={16} />}
        aria-pressed={panel === 'check'}
        onClick={() => onToggle('check')}
        title="Check the model against the IEC 61850 namespace"
      >
        Check
      </Button>
      <Button icon={<CompareIcon size={16} />} aria-pressed={panel === 'compare'} onClick={() => onToggle('compare')} title="Compare this session with another: SCL, snapshot or IED">
        Compare
      </Button>
      {info.state === 'connected' && onCommission && !info.grant && (
        <Button className="" icon={<UnlockIcon size={16} />} onClick={onCommission} title="Ask for Commission mode: audited changes to this IED">
          Commission…
        </Button>
      )}
      {info.state === 'connected' && (
        <Button icon={<FileIcon size={16} />} aria-pressed={panel === 'files'} onClick={() => onToggle('files')}>
          Files
        </Button>
      )}
      <Button icon={info.state === 'connected' ? <DisconnectIcon size={16} /> : <CloseIcon size={16} />} onClick={onClose}>
        {info.state === 'connected' ? 'Disconnect' : 'Close'}
      </Button>
    </div>
  );
}
