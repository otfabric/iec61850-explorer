// An in-memory engine for development (`?mock` in the web build) and the UI
// tests. It mirrors a small part of the simulated IED of the Go tests
// (internal/simied); nothing here talks to a device.

import { ServiceError, type Service } from './service';
import type {
  AuditQuery,
  AuditRecord,
  CompareRequest,
  Comparison,
  CreateDataSetRequest,
  DataSet,
  ExplorerEvent,
  ExportFile,
  ExportFormat,
  FileUpload,
  FileEntry,
  GrantRequest,
  Health,
  JournalRequest,
  JournalResult,
  ModelNode,
  Operation,
  PollConfig,
  Profile,
  RCB,
  ReadRequest,
  ReadResult,
  ReadValue,
  SCLInfo,
  SearchQuery,
  SearchResult,
  SelectSGRequest,
  SessionInfo,
  SettingGroups,
  SubscribeRequest,
  Subscription,
  TargetConfig,
  Value,
  WriteRequest,
  WriteResult,
  ControlRequest,
  ControlResult,
  ControlStep,
} from './types';
import { WRITABLE_FCS } from './types';

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
  [`${LD}/LLN0`]: [node(`${LD}/LLN0.Mod`, 'do', ['CF', 'CO', 'ST'], 0)],
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
  private auditLog: AuditRecord[] = [];
  private createdSets: DataSet[] = [];
  private actSG = 1;
  private ctlNum = 0;
  /** Attributes the mock can write, with their values. */
  private attrs = new Map<string, Value>([
    [`${LD}/LLN0.Mod.ctlModel[CF]`, { name: 'ctlModel', type: 'integer', text: '0', json: 0 }],
    [`${LD}/LLN0.NamPlt.vendor[DC]`, { name: 'vendor', type: 'visible-string', text: 'OT Fabric', json: 'OT Fabric' }],
  ]);

  async health(): Promise<Health> {
    return {
      status: 'ok',
      version: 'mock',
      api_version: '0.2',
      uptime_s: 0,
      go_iec61850: 'mock',
      go_mms: 'mock',
      mode: 'observe',
      time: new Date().toISOString(),
      features: ['profiles', 'commission'],
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

  async dataSets(id: string, ld: string): Promise<DataSet[]> {
    const session = this.session(id);
    return [
      {
        ref: `${ld}/LLN0.Meas`,
        mms: { domain: ld, item_id: 'LLN0$Meas' },
        deletable: false,
        source: session.scl ? 'scl' : 'ied',
        members: [`${LD}/MMXU1.Hz[MX]`, `${LD}/MMXU1.TotW[MX]`].map((ref) => ({ ref, mms: itemID(ref) })),
      },
      ...(session.scl ? [] : this.createdSets.filter((d) => d.mms.domain === ld)),
    ];
  }

  async readDataSet(id: string, ref: string): Promise<ReadResult> {
    this.session(id);
    void ref;
    return { source: 'ied', at: new Date().toISOString(), duration_ms: 1, values: [`${LD}/MMXU1.Hz[MX]`, `${LD}/MMXU1.TotW[MX]`].map((r) => this.value(r)) };
  }

  async reports(id: string, ld: string): Promise<RCB[]> {
    const session = this.session(id);
    const live = !session.scl;
    return [
      {
        ref: `${ld}/LLN0.RP.urcbMeas`,
        mms: { domain: ld, item_id: 'LLN0$RP$urcbMeas' },
        buffered: false,
        source: live ? 'ied' : 'scl',
        rpt_id: 'Meas',
        dat_set: `${ld}/LLN0$Meas`,
        conf_rev: 1,
        opt_flds: ['seq-num', 'timestamp'],
        trg_ops: ['data-changed', 'GI'],
        buf_tm_ms: 0,
        intg_pd_ms: 0,
        ...(live ? { rpt_ena: true, resv: false, sq_num: 4 } : {}),
      },
      {
        ref: `${ld}/LLN0.RP.urcbEvents`,
        mms: { domain: ld, item_id: 'LLN0$RP$urcbEvents' },
        buffered: false,
        source: live ? 'ied' : 'scl',
        rpt_id: 'Events',
        dat_set: `${ld}/LLN0$Events`,
        conf_rev: 1,
        opt_flds: ['seq-num', 'reason-code'],
        trg_ops: ['data-changed', 'GI'],
        buf_tm_ms: 0,
        intg_pd_ms: 0,
        ...(live ? { rpt_ena: (session.grant?.subscriptions ?? []).some((s) => s.rcb === `${ld}/LLN0.RP.urcbEvents`), resv: false, sq_num: 0 } : {}),
      },
    ];
  }

  async files(id: string, dir: string): Promise<FileEntry[]> {
    this.session(id);
    const all = [
      { name: 'COMTRADE/fault1.cfg', size: 34, last_modified: '2026-11-08T12:00:00Z' },
      { name: 'COMTRADE/fault1.dat', size: 20, last_modified: '2026-11-08T12:00:00Z' },
      { name: 'demo.icd', size: 6000, last_modified: '2026-11-08T12:00:00Z' },
    ];
    return all.filter((f) => f.name.startsWith(dir));
  }

  async saveDeviceFile(id: string, _name: string): Promise<boolean> {
    this.session(id);
    return true;
  }

  async journals(id: string, _ld: string): Promise<string[]> {
    this.session(id);
    return ['LLN0$EventLog'];
  }

  async readJournal(id: string, _req: JournalRequest): Promise<JournalResult> {
    this.session(id);
    return {
      entries: [{ entry_id: '0001', time: '2026-11-08T14:30:00Z', variables: [{ tag: `${LD}/MMXU1$MX$Hz`, value: { type: 'float', text: '49.998', json: 49.998 } }] }],
      more_follows: false,
    };
  }

  async settingGroups(id: string, ld: string): Promise<SettingGroups> {
    this.session(id);
    return { ld, num_of_sgs: 2, act_sg: this.actSG, edit_sg: 0, cnf_edit: false, resv_tms: 0 };
  }

  async grantCommission(id: string, req: GrantRequest): Promise<SessionInfo> {
    const session = this.session(id);
    if (session.state !== 'connected') {
      throw new ServiceError('not_allowed', 'an SCL model has no live values: connect to the IED to read');
    }
    if (req.reason.trim() === '') {
      throw new ServiceError('invalid_argument', 'a grant needs a reason, for example a work order');
    }
    if (req.capabilities.length === 0) {
      throw new ServiceError('invalid_argument', 'a grant needs at least one capability');
    }
    if (session.grant) {
      throw new ServiceError('not_allowed', 'the session already holds a grant; revoke it first');
    }
    const now = new Date();
    const minutes = req.minutes || 30;
    const next: SessionInfo = {
      ...session,
      mode: 'commission',
      grant: {
        reason: req.reason.trim(),
        capabilities: [...new Set(req.capabilities)].sort(),
        granted_at: now.toISOString(),
        expires_at: new Date(now.getTime() + minutes * 60_000).toISOString(),
        subscriptions: [],
      },
    };
    this.sessionsById.set(id, next);
    this.record(next, { action: 'grant', outcome: 'ok', reason: next.grant?.reason, capabilities: next.grant?.capabilities });
    this.operation(id, { action: 'grant', target: next.grant?.capabilities.join(', ') ?? '', ok: true });
    this.emit({ kind: 'session', session_id: id, session: next });
    return next;
  }

  async revokeCommission(id: string): Promise<SessionInfo> {
    const session = this.session(id);
    if (!session.grant) {
      return session;
    }
    for (const sub of session.grant.subscriptions) {
      this.record(session, { action: 'cleanup', outcome: 'ok', ref: `unsubscribe ${sub.rcb}` });
    }
    const { grant, ...rest } = session;
    const next: SessionInfo = { ...rest, mode: 'observe' };
    this.sessionsById.set(id, next);
    this.record(next, { action: 'revoke', outcome: 'ok', capabilities: grant.capabilities });
    this.emit({ kind: 'session', session_id: id, session: next });
    return next;
  }

  async write(id: string, req: WriteRequest): Promise<WriteResult> {
    const session = this.session(id);
    if (!session.grant?.capabilities.includes('write')) {
      throw new ServiceError('not_allowed', 'this needs Commission mode with the write capability');
    }
    const fc = /\[([A-Z]{2})\]$/.exec(req.ref)?.[1] ?? '';
    if (!WRITABLE_FCS.includes(fc)) {
      throw new ServiceError('invalid_argument', `functional constraint "${fc}" is not written`);
    }
    const before = this.attrs.get(req.ref);
    if (!before) {
      throw new ServiceError('invalid_argument', `${req.ref}: the type of the attribute is unknown`);
    }
    if (before.type === 'integer' && !/^-?\d+$/.test(req.value.trim())) {
      throw new ServiceError('invalid_argument', `${req.ref}: "${req.value}" is not an INT32`);
    }
    const text = before.type === 'integer' ? String(Number(req.value)) : req.value;
    const written: Value = { ...before, text, json: before.type === 'integer' ? Number(text) : text };
    this.attrs.set(req.ref, written);
    this.record(session, { action: 'write', outcome: 'ok', ref: req.ref, before: before.text, written: text, after: text });
    this.operation(id, { action: 'write', target: req.ref, ok: true });
    return { ref: req.ref, mms: itemID(req.ref), at: new Date().toISOString(), written, before, after: written, verified: true, duration_ms: 3 };
  }

  async control(id: string, req: ControlRequest): Promise<ControlResult> {
    const session = this.granted(id, 'controls');
    const models = ['status-only', 'direct-with-normal-security', 'sbo-with-normal-security', 'direct-with-enhanced-security', 'sbo-with-enhanced-security'];
    const m = Number(this.attrs.get(`${req.ref}.ctlModel[CF]`)?.text ?? NaN);
    if (!(m >= 1 && m <= 4)) {
      throw new ServiceError('invalid_argument', `${req.ref} has ctlModel ${models[m] ?? 'unknown'}: it is not operated`);
    }
    if (!/^-?\d+$/.test(req.value.trim())) {
      throw new ServiceError('invalid_argument', `${req.ref}: ctlVal: "${req.value}" is not an INT32`);
    }
    // The mock's Mod refuses to be switched off (5) at execution.
    const negative = m >= 3 && req.value.trim() === '5';
    const steps: ControlStep[] = [];
    if (m === 2 || m === 4) steps.push({ step: 'select', ok: true, duration_ms: 2 });
    steps.push({ step: 'operate', ok: true, duration_ms: 3 });
    if (m >= 3) steps.push({ step: 'termination', ok: !negative, duration_ms: 100, ...(negative ? { add_cause: 'blocked-by-mode', error: 'negative command termination' } : {}) });
    const stValRef = `${req.ref}.stVal[ST]`;
    const before = this.attrs.get(stValRef) ?? { name: 'stVal', type: 'integer', text: '1', json: 1 };
    const written: Value = { name: 'ctlVal', type: 'integer', text: String(Number(req.value)), json: Number(req.value) };
    const after = negative ? before : { ...before, text: written.text, json: written.json };
    this.attrs.set(stValRef, after);
    this.ctlNum = (this.ctlNum % 255) + 1;
    const outcome = negative ? 'negative' : 'ok';
    const detail = [models[m], `ctlNum ${this.ctlNum}`, ...steps.map((st) => `${st.step} ${st.ok ? 'ok' : st.add_cause}`)].join('; ');
    this.record(session, {
      action: 'control', outcome: negative ? 'failed' : 'ok', ref: req.ref, before: before.text, written: written.text, after: after.text, detail,
      ...(negative ? { error: 'negative: blocked-by-mode' } : {}),
    });
    this.operation(id, { action: 'control', target: req.ref, ok: !negative });
    return {
      ref: req.ref, at: new Date().toISOString(), duration_ms: 110, ctl_model: models[m]!, ctl_num: this.ctlNum, written, test: req.test ?? false,
      interlock_check: req.interlock_check ?? false, synchro_check: req.synchro_check ?? false,
      steps, outcome, ...(negative ? { add_cause: 'blocked-by-mode' } : {}), before, after,
    };
  }

  private granted(id: string, cap: string): SessionInfo {
    const session = this.session(id);
    if (!session.grant?.capabilities.some((c) => c === cap)) {
      throw new ServiceError('not_allowed', `this needs Commission mode with the ${cap} capability`);
    }
    return session;
  }

  private setGrant(session: SessionInfo, subscriptions: Subscription[]): void {
    if (!session.grant) {
      return;
    }
    const next = { ...session, grant: { ...session.grant, subscriptions } };
    this.sessionsById.set(session.id, next);
    this.emit({ kind: 'session', session_id: session.id, session: next });
  }

  async subscribeReport(id: string, req: SubscribeRequest): Promise<Subscription> {
    const session = this.granted(id, 'reports');
    const rcb = (await this.reports(id, LD)).find((r) => r.ref === req.rcb);
    if (!rcb) {
      throw new ServiceError('not_found', `no report control block ${req.rcb}`);
    }
    if (rcb.rpt_ena) {
      throw new ServiceError('not_allowed', `${req.rcb} is enabled: another client may own it`);
    }
    const sub: Subscription = {
      id: `sub-${++this.ids}`,
      rcb: rcb.ref,
      mms: rcb.mms,
      buffered: rcb.buffered,
      rpt_id: rcb.rpt_id,
      dat_set: rcb.dat_set,
      since: new Date().toISOString(),
      reports: 0,
      dropped: 0,
    };
    this.setGrant(session, [...(session.grant?.subscriptions ?? []), sub]);
    this.record(session, { action: 'subscribe', outcome: 'ok', ref: rcb.ref, detail: `RptID ${rcb.rpt_id}` });
    if (req.gi) {
      queueMicrotask(() => this.report(session.id, sub, 'GI'));
    }
    return sub;
  }

  private report(sessionId: string, sub: Subscription, reason: string): void {
    this.emit({
      kind: 'report',
      session_id: sessionId,
      report: {
        subscription_id: sub.id,
        rcb: sub.rcb,
        rpt_id: sub.rpt_id,
        received_at: new Date().toISOString(),
        seq_num: 1,
        entries: [
          { ref: `${LD}/GGIO1.Ind1.stVal[ST]`, reasons: [reason], value: { name: 'stVal', type: 'boolean', text: 'true', json: true } },
          { ref: `${LD}/MMXU1.Hz.mag.f[MX]`, reasons: [reason], value: { name: 'f', type: 'float', text: String(this.hz), json: this.hz } },
        ],
      },
    });
  }

  private findSub(session: SessionInfo, subscriptionId: string): Subscription {
    const sub = session.grant?.subscriptions.find((s) => s.id === subscriptionId);
    if (!sub) {
      throw new ServiceError('not_found', `no report subscription "${subscriptionId}"`);
    }
    return sub;
  }

  async unsubscribe(id: string, subscriptionId: string): Promise<void> {
    const session = this.granted(id, 'reports');
    const sub = this.findSub(session, subscriptionId);
    this.setGrant(session, session.grant?.subscriptions.filter((s) => s.id !== subscriptionId) ?? []);
    this.record(session, { action: 'unsubscribe', outcome: 'ok', ref: sub.rcb });
  }

  async triggerGI(id: string, subscriptionId: string): Promise<void> {
    const session = this.granted(id, 'reports');
    const sub = this.findSub(session, subscriptionId);
    this.record(session, { action: 'gi', outcome: 'ok', ref: sub.rcb });
    queueMicrotask(() => this.report(session.id, sub, 'GI'));
  }

  async createDataSet(id: string, req: CreateDataSetRequest): Promise<DataSet> {
    const session = this.granted(id, 'datasets');
    const match = /^([^/]+)\/(LLN0)\.([A-Za-z][A-Za-z0-9_]*)$/.exec(req.ref.trim());
    if (!match || req.members.length === 0) {
      throw new ServiceError('invalid_argument', 'a data set needs a name such as LD/LLN0.Name and at least one member');
    }
    if (this.createdSets.some((d) => d.ref === req.ref)) {
      throw new ServiceError('invalid_argument', `create data set ${req.ref}: object-exists`);
    }
    const [, ld = '', ln = '', name = ''] = match;
    const ds: DataSet = {
      ref: req.ref,
      mms: { domain: ld, item_id: `${ln}$${name}` },
      deletable: true,
      source: 'ied',
      members: req.members.map((ref) => ({ ref, mms: itemID(ref) })),
    };
    this.createdSets.push(ds);
    this.record(session, { action: 'create-dataset', outcome: 'ok', ref: req.ref, detail: `${req.members.length} members` });
    return ds;
  }

  async deleteDataSet(id: string, ref: string): Promise<void> {
    const session = this.granted(id, 'datasets');
    if (!this.createdSets.some((d) => d.ref === ref)) {
      throw new ServiceError('not_allowed', `the IED marks ${ref} as not deletable`);
    }
    this.createdSets = this.createdSets.filter((d) => d.ref !== ref);
    this.record(session, { action: 'delete-dataset', outcome: 'ok', ref });
  }

  async selectActiveSG(id: string, req: SelectSGRequest): Promise<SettingGroups> {
    const session = this.granted(id, 'setting-groups');
    if (req.sg < 1 || req.sg > 2) {
      throw new ServiceError('invalid_argument', `${req.ld} has setting groups 1 to 2`);
    }
    const before = this.actSG;
    this.actSG = req.sg;
    this.record(session, {
      action: 'select-active-sg',
      outcome: 'ok',
      ref: `${req.ld}/LLN0.SGCB.ActSG[SP]`,
      before: String(before),
      written: String(req.sg),
      after: String(req.sg),
    });
    return this.settingGroups(id, req.ld);
  }

  async audit(query: AuditQuery): Promise<AuditRecord[]> {
    return this.auditLog
      .filter((r) => !query.session_id || r.session_id === query.session_id)
      .slice(0, query.limit || 500);
  }

  async compare(req: CompareRequest): Promise<Comparison> {
    if (!req.a || !req.b || req.a === req.b) {
      throw new ServiceError('invalid_argument', 'compare needs two different sessions');
    }
    const side = (id: string) => {
      const s = this.session(id);
      return s.snapshot
        ? { session_id: id, source: 'snapshot' as const, label: s.snapshot.file }
        : s.scl
          ? { session_id: id, source: 'scl' as const, label: `${s.scl.file} (${s.scl.ied}/${s.scl.access_point})` }
          : { session_id: id, source: 'ied' as const, label: `${s.target.host}:${s.target.port ?? 102}` };
    };
    return {
      a: side(req.a),
      b: side(req.b),
      categories: [
        { name: 'model', compared: true, items: 40, same: 38, only_a: 1, only_b: 0, changed: 1 },
        { name: 'datasets', compared: true, items: 2, same: 1, only_a: 0, only_b: 0, changed: 1 },
        { name: 'reports', compared: true, items: 3, same: 3, only_a: 0, only_b: 0, changed: 0 },
        ...(req.values ? [{ name: 'values' as const, compared: true, items: 12, same: 11, only_a: 0, only_b: 0, changed: 1 }] : []),
      ],
      differences: [
        { category: 'model', kind: 'only_a', ref: `${LD}/LPHD9`, note: 'ln' },
        { category: 'model', kind: 'changed', ref: `${LD}/MMXU1.Hz`, field: 'fcs', a: 'CF,MX', b: 'MX' },
        {
          category: 'datasets',
          kind: 'changed',
          ref: `${LD}/LLN0.Meas`,
          field: 'members',
          a: `${LD}/MMXU1.Hz[MX]`,
          b: `${LD}/MMXU1.Hz[MX]\n${LD}/MMXU1.TotW[MX]`,
          note: '1 members / 2 members',
        },
        ...(req.values
          ? [
              {
                category: 'values' as const,
                kind: 'changed' as const,
                ref: `${LD}/GGIO1.SPCSO2.ctlModel[CF]`,
                field: 'value',
                a: 'direct-with-normal-security (1)',
                b: 'sbo-with-normal-security (2)',
              },
            ]
          : []),
      ],
      truncated: false,
      duration_ms: 3,
    };
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

  private record(session: SessionInfo, rec: Omit<AuditRecord, 'seq' | 'at' | 'session_id' | 'target'>): void {
    const full: AuditRecord = {
      ...rec,
      seq: this.auditLog.length + 1,
      at: new Date().toISOString(),
      session_id: session.id,
      target: `${session.target.host}:${session.target.port ?? 102}`,
    };
    this.auditLog.unshift(full);
    this.emit({ kind: 'audit', session_id: session.id, audit: full });
  }

  private value(ref: string): ReadValue {
    const mms = itemID(ref);
    const attr = this.attrs.get(ref);
    if (attr) {
      return { ref, mms, value: attr };
    }
    // A data object under an FC: the writable attributes the mock holds.
    const fc = /\[([A-Z]{2})\]$/.exec(ref)?.[1];
    const base = ref.replace(/\[[A-Z]{2}\]$/, '');
    const children = [...this.attrs].filter(([r]) => r.startsWith(`${base}.`) && r.endsWith(`[${fc}]`)).map(([, v]) => v);
    if (children.length > 0) {
      return { ref, mms, value: { type: 'structure', text: `{${children.length} components}`, children } };
    }
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
