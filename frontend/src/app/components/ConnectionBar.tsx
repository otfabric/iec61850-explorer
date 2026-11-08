import { useState, type FormEvent } from 'react';
import { ConnectIcon, DisconnectIcon } from '../icons';
import type { SessionInfo, TargetConfig } from '../service/types';
import { Button, Chip, Field, TextInput } from './ui';

interface ConnectionBarProps {
  session: SessionInfo | null;
  connecting: boolean;
  disabled: boolean;
  onConnect: (cfg: TargetConfig) => void;
  onDisconnect: () => void;
}

export function ConnectionBar({ session, connecting, disabled, onConnect, onDisconnect }: ConnectionBarProps) {
  const [host, setHost] = useState('');
  const [port, setPort] = useState('102');

  function submit(event: FormEvent) {
    event.preventDefault();
    const p = Number(port);
    onConnect({ host: host.trim(), port: Number.isInteger(p) && p > 0 ? p : undefined });
  }

  if (session) {
    const { identity, discovery } = session;
    return (
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-line px-4 py-2">
        <span className="font-mono text-fg-strong">
          {session.target.host}:{session.target.port}
        </span>
        <Chip tone={session.state === 'connected' ? 'accent' : 'wire'}>{session.state}</Chip>
        {identity && (
          <span className="text-muted" title="MMS Identify">
            {identity.vendor} · {identity.model} · {identity.revision}
          </span>
        )}
        <span className="text-muted">
          {discovery.logical_devices} LD · {discovery.logical_nodes} LN · {discovery.data_objects} DO · discovered in {discovery.duration_ms} ms
        </span>
        <Button className="ml-auto" icon={<DisconnectIcon size={16} />} onClick={onDisconnect}>
          Disconnect
        </Button>
      </div>
    );
  }
  return (
    <form className="flex shrink-0 flex-wrap items-end gap-3 border-b border-line px-4 py-2" onSubmit={submit}>
      <Field label="IED host">
        <TextInput value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.10" className="w-56" aria-label="IED host" />
      </Field>
      <Field label="Port">
        <TextInput value={port} onChange={(e) => setPort(e.target.value)} inputMode="numeric" className="w-20" aria-label="Port" />
      </Field>
      <Button type="submit" variant="primary" icon={<ConnectIcon size={16} />} disabled={disabled || connecting || host.trim() === ''}>
        {connecting ? 'Connecting…' : 'Connect'}
      </Button>
    </form>
  );
}
