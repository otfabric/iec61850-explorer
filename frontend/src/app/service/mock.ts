// An in-memory engine for development (`?mock` in the web build) and the UI
// tests. It mirrors a small part of the simulated IED of the Go tests
// (internal/simied); nothing here talks to a device.

import { ServiceError, type Service } from './service';
import type { ExplorerEvent, Health, ModelNode, Operation, ReadRequest, ReadResult, ReadValue, SessionInfo, TargetConfig, Value } from './types';

const LD = 'DemoIEDLD0';

const node = (ref: string, kind: ModelNode['kind'], fcs: string[], children: number, ln_class?: string): ModelNode => ({
  name: ref.split(/[/.]/).at(-1) ?? ref,
  ref,
  kind,
  fcs,
  children,
  ln_class,
});

const TREE: Record<string, ModelNode[]> = {
  '': [node(LD, 'ld', [], 2)],
  [LD]: [node(`${LD}/LLN0`, 'ln', ['ST'], 1, 'LLN0'), node(`${LD}/MMXU1`, 'ln', ['MX'], 1, 'MMXU')],
  [`${LD}/LLN0`]: [node(`${LD}/LLN0.Mod`, 'do', ['ST', 'CF'], 0)],
  [`${LD}/MMXU1`]: [node(`${LD}/MMXU1.Hz`, 'do', ['MX'], 0)],
};

const HZ: Value = {
  type: 'structure',
  text: '{3 components}',
  children: [
    { name: 'mag', type: 'structure', text: '{1 components}', children: [{ name: 'f', type: 'float', text: '50.002', json: 50.002 }] },
    { name: 'q', type: 'bit-string', text: 'good', bits: 13, quality: { validity: 'good', flags: [], raw: 0 } },
    {
      name: 't',
      type: 'utc-time',
      text: '2026-11-08T14:32:08.417000Z',
      time: { time: '2026-11-08T14:32:08.417Z', leap_second_known: false, clock_failure: false, clock_not_synchronized: false, accuracy: 0 },
    },
  ],
};

/** The MMS item ID of an FC-qualified reference: LN$FC$DO$DA. */
function itemID(ref: string): { domain: string; item_id: string } {
  const match = /^([^/]+)\/([^[]+)\[([A-Z]{2})\]$/.exec(ref);
  if (!match) {
    return { domain: '', item_id: '' };
  }
  const [, domain = '', path = '', fc = ''] = match;
  const [ln = '', ...rest] = path.split('.');
  return { domain, item_id: [ln, fc, ...rest].join('$') };
}

export class MockService implements Service {
  private session: SessionInfo | null = null;
  private seq = 0;
  private listeners = new Set<(event: ExplorerEvent) => void>();

  async health(): Promise<Health> {
    return {
      status: 'ok',
      version: 'mock',
      api_version: '0.1',
      uptime_s: 0,
      go_iec61850: 'mock',
      go_mms: 'mock',
      mode: 'observe',
      time: new Date().toISOString(),
      features: [],
    };
  }

  async openSession(cfg: TargetConfig): Promise<SessionInfo> {
    if (cfg.host === 'unreachable') {
      const detail = {
        code: 'connect_failed',
        message: 'connection refused',
        stage: 'tcp',
        hint: 'No TCP connection to the IED. Check the address, the port (102 by default), routing and firewalls.',
      };
      this.operation({ action: 'connect', target: `${cfg.host}:102`, ok: false, error: detail });
      throw new ServiceError('connect_failed', 'cannot connect to unreachable:102: connection refused', detail);
    }
    this.session = {
      id: 'mock-1',
      target: { host: cfg.host, port: cfg.port ?? 102 },
      state: 'connected',
      mode: 'observe',
      opened_at: new Date().toISOString(),
      identity: { vendor: 'OT Fabric', model: 'Mock IED', revision: '1' },
      discovery: { logical_devices: 1, logical_nodes: 2, data_objects: 2, nodes: 2, duration_ms: 3 },
    };
    this.operation({ action: 'connect', target: `${cfg.host}:${cfg.port ?? 102}`, ok: true });
    return this.session;
  }

  async closeSession(): Promise<void> {
    this.session = null;
  }

  async sessions(): Promise<SessionInfo[]> {
    return this.session ? [this.session] : [];
  }

  async browse(_id: string, ref: string): Promise<ModelNode[]> {
    const nodes = TREE[ref];
    if (!nodes) {
      throw new ServiceError('not_found', `${ref} is not in the discovered model`);
    }
    return nodes;
  }

  async read(_id: string, req: ReadRequest): Promise<ReadResult> {
    const values: ReadValue[] = req.refs.map((ref) =>
      ref === `${LD}/MMXU1.Hz[MX]`
        ? { ref, mms: itemID(ref), value: HZ }
        : { ref, mms: itemID(ref), error: { code: 'object_not_found', message: 'object-non-existent', stage: 'response' } },
    );
    this.operation({ action: 'read', target: req.refs[0], ok: values.every((v) => !v.error) });
    return { at: new Date().toISOString(), duration_ms: 2, values };
  }

  subscribe(_since: number, onEvent: (event: ExplorerEvent) => void): () => void {
    this.listeners.add(onEvent);
    return () => this.listeners.delete(onEvent);
  }

  private operation(op: Omit<Operation, 'started_at' | 'duration_ms' | 'requests'>): void {
    const seq = ++this.seq;
    const event: ExplorerEvent = {
      seq,
      kind: 'operation',
      at: new Date().toISOString(),
      session_id: this.session?.id,
      operation: {
        ...op,
        started_at: new Date().toISOString(),
        duration_ms: 1,
        requests: [{ service: op.action === 'read' ? 'Read' : 'Identify', invoke_id: seq, duration_ms: 0.4 }],
      },
    };
    this.listeners.forEach((listener) => listener(event));
  }
}
