// An in-memory engine for development (`?mock` in the web build) and the UI
// tests. It mirrors a small part of the simulated IED of the Go tests
// (internal/simied); nothing here talks to a device.

import { ServiceError, type Service } from './service';
import type {
  ExplorerEvent,
  ExportFile,
  ExportFormat,
  FileUpload,
  Health,
  ModelNode,
  Operation,
  PollConfig,
  Profile,
  ReadRequest,
  ReadResult,
  ReadValue,
  SCLInfo,
  SearchQuery,
  SearchResult,
  SessionInfo,
  TargetConfig,
  Value,
} from './types';

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
  [LD]: [node(`${LD}/LLN0`, 'ln', ['ST'], 1, 'LLN0'), node(`${LD}/MMXU1`, 'ln', ['MX'], 2, 'MMXU')],
  [`${LD}/LLN0`]: [node(`${LD}/LLN0.Mod`, 'do', ['CF', 'ST'], 0)],
  [`${LD}/MMXU1`]: [node(`${LD}/MMXU1.Hz`, 'do', ['MX'], 0), node(`${LD}/MMXU1.TotW`, 'do', ['MX'], 0)],
};

const allNodes = (): ModelNode[] => Object.values(TREE).flat();

const measurement = (f: number, validity: 'good' | 'questionable', flags: string[] = []): Value => ({
  type: 'structure',
  text: '{3 components}',
  children: [
    { name: 'mag', type: 'structure', text: '{1 components}', children: [{ name: 'f', type: 'float', text: String(f), json: f }] },
    {
      name: 'q',
      type: 'bit-string',
      text: flags.length ? `${validity} (${flags.join(', ')})` : validity,
      bits: 13,
      quality: { validity, flags, raw: 0 },
    },
    {
      name: 't',
      type: 'utc-time',
      text: '2026-11-08T14:32:08.417000Z',
      time: { time: '2026-11-08T14:32:08.417Z', leap_second_known: false, clock_failure: false, clock_not_synchronized: false, accuracy: 0 },
    },
  ],
});

/** The MMS item ID of an FC-qualified reference: LN$FC$DO$DA. */
export function itemID(ref: string): { domain: string; item_id: string } {
  const match = /^([^/]+)\/([^[]+)\[([A-Z]{2})\]$/.exec(ref);
  if (!match) {
    return { domain: '', item_id: '' };
  }
  const [, domain = '', path = '', fc = ''] = match;
  const [ln = '', ...rest] = path.split('.');
  return { domain, item_id: [ln, fc, ...rest].join('$') };
}

export const MOCK_SCL = new TextEncoder().encode('<SCL>mock</SCL>');

export class MockService implements Service {
  private sessionsById = new Map<string, SessionInfo>();
  private seq = 0;
  private ids = 0;
  private hz = 50.002;
  private listeners = new Set<(event: ExplorerEvent) => void>();
  private pollers = new Map<string, ReturnType<typeof setInterval>>();
  private profileList: Profile[] = [];

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
      features: ['profiles'],
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
      this.operation(undefined, { action: 'connect', target: `${cfg.host}:102`, ok: false, error: detail });
      throw new ServiceError('connect_failed', 'cannot connect to unreachable:102: connection refused', detail);
    }
    const session = this.add({
      target: { ...cfg, port: cfg.port ?? 102 },
      state: 'connected',
      identity: { vendor: 'OT Fabric', model: 'Mock IED', revision: '1' },
    });
    this.operation(session.id, { action: 'connect', target: `${cfg.host}:${cfg.port ?? 102}`, ok: true });
    return session;
  }

  async closeSession(id: string): Promise<void> {
    await this.stopPolling(id);
    const session = this.sessionsById.get(id);
    this.sessionsById.delete(id);
    if (session) {
      this.emit({ kind: 'session', session_id: id, session: { ...session, state: 'closed' } });
    }
  }

  async sessions(): Promise<SessionInfo[]> {
    return [...this.sessionsById.values()];
  }

  async browse(id: string, ref: string): Promise<ModelNode[]> {
    this.session(id);
    const nodes = TREE[ref];
    if (!nodes) {
      throw new ServiceError('not_found', `${ref} is not in the discovered model`);
    }
    return nodes;
  }

  async read(id: string, req: ReadRequest): Promise<ReadResult> {
    const session = this.session(id);
    if (session.state === 'offline' && !session.snapshot) {
      throw new ServiceError('not_allowed', 'an SCL model has no live values: connect to the IED to read');
    }
    const values = req.refs.map((ref) => this.value(ref));
    if (!session.snapshot) {
      this.operation(id, { action: 'read', target: req.refs.length > 1 ? `${req.refs.length} references` : req.refs[0], ok: values.every((v) => !v.error) });
    }
    return { source: session.snapshot ? 'snapshot' : 'ied', at: new Date().toISOString(), duration_ms: 2, values };
  }

  async search(id: string, query: SearchQuery): Promise<SearchResult> {
    this.session(id);
    const text = query.text?.toLowerCase() ?? '';
    const hits = allNodes().filter(
      (n) =>
        n.ref.toLowerCase().includes(text) &&
        (!query.kind || n.kind === query.kind) &&
        (!query.fc || n.fcs.includes(query.fc)) &&
        (!query.ln_class || n.ref.includes(query.ln_class.toUpperCase())),
    );
    return { hits, truncated: false };
  }

  async startPolling(id: string, cfg: PollConfig): Promise<void> {
    const session = this.session(id);
    if (session.state !== 'connected') {
      throw new ServiceError('not_allowed', 'not connected');
    }
    await this.stopPolling(id);
    let seq = 0;
    const interval = cfg.interval_ms ?? 1000;
    const tick = () => {
      this.hz = Math.round((this.hz + 0.001) * 1000) / 1000;
      seq += 1;
      this.emit({
        kind: 'sample',
        session_id: id,
        sample: { session_id: id, seq, at: new Date().toISOString(), duration_ms: 1, values: cfg.refs.map((r) => this.value(r)), overruns: 0 },
      });
    };
    this.pollers.set(id, setInterval(tick, interval));
    this.sessionsById.set(id, {
      ...session,
      polling: { refs: cfg.refs, interval_ms: interval, started_at: new Date().toISOString(), samples: 0, failures: 0, overruns: 0 },
    });
    this.emit({ kind: 'session', session_id: id, session: this.sessionsById.get(id) });
    queueMicrotask(tick);
  }

  async stopPolling(id: string): Promise<void> {
    const timer = this.pollers.get(id);
    if (timer !== undefined) {
      clearInterval(timer);
      this.pollers.delete(id);
      const session = this.sessionsById.get(id);
      if (session) {
        const { polling: _, ...rest } = session;
        this.sessionsById.set(id, rest);
        this.emit({ kind: 'session', session_id: id, session: rest });
      }
    }
  }

  async exportSession(id: string, format: ExportFormat, refs: string[]): Promise<ExportFile> {
    const session = this.session(id);
    if (format === 'csv') {
      const rows = refs.map((r) => `${r},${this.value(r).value?.text ?? ''}`);
      return { name: 'iec61850-mock.csv', mime: 'text/csv', text: ['ref,value', ...rows].join('\n') };
    }
    const snapshot = {
      schema: 'otfabric.iec61850-explorer.snapshot/1',
      target: session.target,
      nodes: allNodes(),
      values: refs.map((r) => this.value(r)),
    };
    return { name: 'iec61850-mock.json', mime: 'application/json', text: JSON.stringify(snapshot) };
  }

  async openSnapshot(file: FileUpload): Promise<SessionInfo> {
    return this.add({
      target: { host: '10.0.0.5', port: 102 },
      state: 'offline',
      snapshot: { file: file.name, captured_at: '2026-11-08T14:00:00Z' },
    });
  }

  async inspectSCL(file: FileUpload): Promise<SCLInfo> {
    return {
      name: file.name,
      kind: 'scd',
      version: '2007B',
      ieds: [
        { name: 'DemoIED', manufacturer: 'OT Fabric', access_points: [{ name: 'S1', logical_devices: 1, target: { host: '10.0.0.5', port: 102, addressing: { remote_psel: '00000001' } } }] },
        { name: 'Other', access_points: [{ name: 'S1', logical_devices: 1 }] },
      ],
      diagnostics: [{ severity: 'warning', message: 'an unused type template' }],
    };
  }

  async openSCL(file: FileUpload, ied: string, accessPoint: string): Promise<SessionInfo> {
    return this.add({ target: { host: '', port: 102 }, state: 'offline', scl: { file: file.name, ied, access_point: accessPoint } });
  }

  async profiles(): Promise<Profile[]> {
    return [...this.profileList].sort((a, b) => a.name.localeCompare(b.name));
  }

  async saveProfile(profile: Profile): Promise<Profile> {
    if (profile.name.trim() === '' || profile.target.host.trim() === '') {
      throw new ServiceError('invalid_argument', 'a profile needs a name and a host');
    }
    const saved = { ...profile, id: profile.id || `p${++this.ids}`, updated_at: new Date().toISOString() };
    this.profileList = [...this.profileList.filter((p) => p.id !== saved.id), saved];
    return saved;
  }

  async deleteProfile(id: string): Promise<void> {
    this.profileList = this.profileList.filter((p) => p.id !== id);
  }

  subscribe(_since: number, onEvent: (event: ExplorerEvent) => void): () => void {
    this.listeners.add(onEvent);
    return () => this.listeners.delete(onEvent);
  }

  private session(id: string): SessionInfo {
    const session = this.sessionsById.get(id);
    if (!session) {
      throw new ServiceError('not_found', `no session "${id}"`);
    }
    return session;
  }

  private add(parts: Pick<SessionInfo, 'target' | 'state'> & Partial<SessionInfo>): SessionInfo {
    const session: SessionInfo = {
      id: `mock-${++this.ids}`,
      mode: 'observe',
      opened_at: new Date().toISOString(),
      discovery: { logical_devices: 1, logical_nodes: 2, data_objects: 3, nodes: 3, duration_ms: 3 },
      ...parts,
    };
    this.sessionsById.set(session.id, session);
    this.emit({ kind: 'session', session_id: session.id, session });
    return session;
  }

  private value(ref: string): ReadValue {
    const mms = itemID(ref);
    if (ref === `${LD}/MMXU1.Hz[MX]`) {
      return { ref, mms, value: measurement(this.hz, 'good') };
    }
    if (ref === `${LD}/MMXU1.TotW[MX]`) {
      return { ref, mms, value: measurement(1245000, 'questionable', ['old-data']) };
    }
    return { ref, mms, error: { code: 'object_not_found', message: 'object-non-existent', stage: 'response' } };
  }

  private emit(event: Omit<ExplorerEvent, 'seq' | 'at'>): void {
    const full: ExplorerEvent = { ...event, seq: ++this.seq, at: new Date().toISOString() };
    this.listeners.forEach((listener) => listener(full));
  }

  private operation(sessionId: string | undefined, op: Omit<Operation, 'started_at' | 'duration_ms' | 'requests'>): void {
    this.emit({
      kind: 'operation',
      session_id: sessionId,
      operation: {
        ...op,
        started_at: new Date().toISOString(),
        duration_ms: 1,
        requests: [{ service: op.action === 'read' ? 'Read' : 'Identify', invoke_id: this.seq, duration_ms: 0.4 }],
      },
    });
  }
}
