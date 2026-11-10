// The capture of the mock engine: a short exchange between a client and the
// simulated IED with a lost segment, one report and a GOOSE stream. Filtering
// and paging follow the engine (internal/engine/trace.go).

import type {
  FindingsQuery,
  SessionInfo,
  Timeline,
  TimelineQuery,
  TraceAssociation,
  TraceFinding,
  TraceFindings,
  TraceLink,
  TraceObject,
  TraceReport,
  TraceReports,
  TraceReportsQuery,
  TraceStreams,
  TraceSummary,
  TraceTransaction,
  WireView,
} from './types';
import { DEFAULT_TRACE_PAGE, MAX_TRACE_PAGE } from './types';

const LD = 'DemoIEDLD0';
const T0 = Date.parse('2026-11-10T09:00:00.000Z');
const at = (ms: number) => new Date(T0 + ms).toISOString();

/** An MMS variable name of the mock IED, with its IEC 61850 reference. */
function variable(item: string, ref: string, kind: string, missing = false): TraceObject {
  return { mms: { domain: LD, item_id: item }, scope: 'domain', ref, kind, ...(missing ? { missing: true } : {}) } as TraceObject;
}

const HZ = variable('MMXU1$MX$Hz', `${LD}/MMXU1.Hz[MX]`, 'do');
const MOD = variable('LLN0$ST$Mod$stVal', `${LD}/LLN0.Mod.stVal[ST]`, 'da');
const GHOST = variable('GGIO9$ST$Ind1$stVal', `${LD}/GGIO9.Ind1.stVal[ST]`, 'da', true);

const TRANSACTIONS: TraceTransaction[] = [
  {
    index: 1,
    association: 1,
    invoke_id: 1,
    service: 'Identify',
    status: 'ok',
    request_at: at(10),
    response_at: at(12),
    latency_ms: 2,
    request_frame: 6,
    response_frame: 7,
    objects: [],
    request: { kind: 'confirmed-request', invoke_id: 1, service: 'Identify' },
    response: { kind: 'confirmed-response', invoke_id: 1, service: 'Identify', identity: { vendor: 'OT Fabric', model: 'Explorer demo IED', revision: '1' } },
  },
  {
    index: 2,
    association: 1,
    invoke_id: 2,
    service: 'Read',
    status: 'ok',
    request_at: at(20),
    response_at: at(23.5),
    latency_ms: 3.5,
    request_frame: 8,
    response_frame: 9,
    objects: [HZ],
    request: { kind: 'confirmed-request', invoke_id: 2, service: 'Read', objects: [HZ] },
    response: { kind: 'confirmed-response', invoke_id: 2, service: 'Read', values: [{ type: 'float', text: '50.002', json: 50.002 }] },
  },
  {
    index: 3,
    association: 1,
    invoke_id: 3,
    service: 'Read',
    status: 'unanswered',
    request_at: at(40),
    request_frame: 11,
    objects: [MOD],
    request: { kind: 'confirmed-request', invoke_id: 3, service: 'Read', objects: [MOD] },
  },
  {
    index: 4,
    association: 2,
    invoke_id: 1,
    service: 'Read',
    status: 'error',
    request_at: at(60),
    response_at: at(61),
    latency_ms: 1,
    request_frame: 15,
    response_frame: 16,
    objects: [GHOST],
    error: 'confirmed error: class 2 (definition) code 1 (object undefined)',
    request: { kind: 'confirmed-request', invoke_id: 1, service: 'Read', objects: [GHOST] },
    response: { kind: 'confirmed-error', invoke_id: 1, error: 'class 2 (definition) code 1 (object undefined)' },
  },
  {
    index: 5,
    association: 2,
    invoke_id: 2,
    service: 'GetNameList',
    status: 'ok',
    request_at: at(70),
    response_at: at(72),
    latency_ms: 2,
    request_frame: 17,
    response_frame: 18,
    objects: [],
    request: { kind: 'confirmed-request', invoke_id: 2, service: 'GetNameList' },
    response: { kind: 'confirmed-response', invoke_id: 2, service: 'GetNameList', name_list: [LD] },
  },
];

const ASSOCIATIONS: TraceAssociation[] = [
  {
    id: 1,
    client: '10.0.0.20:50211',
    server: '10.0.0.5:102',
    calling_tsap: '0001',
    called_tsap: '0001',
    open: 'associate',
    close: '',
    accepted: true,
    authentication: 'none',
    initiate: { local_detail: 65000, max_serv_outstanding_calling: 5, max_serv_outstanding_called: 5, data_structure_nesting: 10, version: 1 },
    first: at(0),
    last: at(45),
    transactions: 3,
    reports: 1,
    unanswered: 1,
    gaps: 1,
  },
  {
    id: 2,
    client: '10.0.0.21:50300',
    server: '10.0.0.5:102',
    open: 'midstream',
    close: 'conclude',
    first: at(55),
    last: at(80),
    transactions: 2,
    reports: 0,
    unanswered: 0,
    gaps: 0,
  },
];

const WIRE_FINDINGS: TraceFinding[] = [
  { severity: 'error', kind: 'tcp-gap', message: 'Association 1 lost 1 range of its TCP byte stream: a PDU in it was not seen', association: 1, at: at(0), source: 'Association.Gaps', count: 1 },
  { severity: 'warning', kind: 'unanswered', message: 'Read (invoke ID 3) got no response', association: 1, frame: 11, at: at(40), transaction: 3, source: 'Transaction.Status=unanswered' },
  { severity: 'warning', kind: 'open-at-end', message: 'Association 1 was still open at the end of the capture', association: 1, at: at(45), source: 'Association.Close' },
  { severity: 'warning', kind: 'service-error', message: 'Read (invoke ID 1) failed: class 2 (definition) code 1 (object undefined)', association: 2, frame: 16, at: at(61), transaction: 4, source: 'Transaction.Status=error' },
  { severity: 'info', kind: 'midstream', message: 'Association 2 was captured midstream: no AARQ, AARE or TSAPs', association: 2, at: at(55), source: 'Association.Open' },
  { severity: 'warning', kind: 'goose-sqnum-gap', message: 'GOOSE DemoIEDLD0/LLN0$GO$gcb01: sqNum gap (2 times)', stream: `${LD}/LLN0$GO$gcb01`, source: 'GooseStream.Issues[sqNum gap]', count: 2 },
];

const REPORT_VALUES = [
  { type: 'visible-string' as const, text: 'Meas', json: 'Meas' },
  { type: 'float' as const, text: '50.002', json: 50.002 },
];

/** One trace session of the mock: the capture and its link. */
export class MockTrace {
  link: TraceLink | undefined;
  linked: SessionInfo | undefined;

  constructor(readonly file: string, readonly ports: number[]) {}

  private resolve(o: TraceObject): TraceObject {
    const { missing, ...plain } = o as TraceObject & { missing?: boolean };
    if (!this.link) {
      return plain;
    }
    if (missing) {
      return { ...plain, model: 'missing' };
    }
    const concerns = o.ref?.includes('MMXU1.Hz') ? [`data object ${LD}/MMXU1.Hz`, `data set ${LD}/LLN0.Meas`] : [`data object ${LD}/LLN0.Mod`];
    return { ...plain, model: 'found', concerns };
  }

  private transaction(t: TraceTransaction): TraceTransaction {
    const r = (o: TraceObject) => this.resolve(o);
    return {
      ...t,
      objects: t.objects.map(r),
      ...(t.request ? { request: { ...t.request, ...(t.request.objects ? { objects: t.request.objects.map(r) } : {}) } } : {}),
    };
  }

  private findingsAll(): TraceFinding[] {
    const out = [...WIRE_FINDINGS];
    if (this.link) {
      out.push({
        severity: 'warning',
        kind: 'not-in-model',
        message: `${LD}/GGIO9.Ind1.stVal[ST] is not in the model of ${this.link.label}`,
        association: 2,
        frame: 15,
        at: at(60),
        transaction: 4,
        source: 'Transaction.Request.Variables',
      });
    }
    return out;
  }

  summary(): TraceSummary {
    const byStatus: Record<string, number> = {};
    const byService: Record<string, number> = {};
    for (const t of TRANSACTIONS) {
      byStatus[t.status] = (byStatus[t.status] ?? 0) + 1;
      byService[t.service] = (byService[t.service] ?? 0) + 1;
    }
    const findings: Record<string, number> = {};
    for (const f of this.findingsAll()) {
      findings[f.severity] = (findings[f.severity] ?? 0) + 1;
    }
    return {
      file: {
        name: this.file,
        format: 'pcapng',
        sha256: '9f2c6a1e0b7d4c3a8e5f6b2d1c0a9e8f7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a',
        bytes: 6768,
        packets: 42,
        interfaces: 1,
        links: ['ethernet'],
        first: at(0),
        last: at(80),
        duration_ms: 80,
        ports: this.ports,
      },
      counters: { unsupported: { other_tcp: 3 }, tpkt_resync: 12, cotp_errors: 0, tsdu_errors: 0, bare_mms: 0, tso_segments: 0, goose_errors: 0, sv_errors: 0 },
      associations: ASSOCIATIONS.length,
      transactions: TRANSACTIONS.length,
      by_status: byStatus,
      by_service: byService,
      reports: 1,
      goose: 1,
      sv: 0,
      findings,
      ...(this.link ? { link: this.link } : {}),
      engine: 'go-mms-trace v0.1.1',
    };
  }

  associations(): TraceAssociation[] {
    return ASSOCIATIONS;
  }

  timeline(q: TimelineQuery): Timeline {
    const service = q.service?.trim().toLowerCase() ?? '';
    const from = q.from ? Date.parse(q.from) : -Infinity;
    const to = q.to ? Date.parse(q.to) : Infinity;
    const matching = TRANSACTIONS.filter((t) => {
      const when = Date.parse(t.request_at ?? t.response_at ?? '');
      return (
        (!q.association || t.association === q.association) &&
        (service === '' || t.service.toLowerCase() === service) &&
        (!q.status?.length || q.status.includes(t.status)) &&
        when >= from &&
        when <= to
      );
    });
    const { offset, limit } = page(q.offset, q.limit);
    return { total: matching.length, offset, transactions: matching.slice(offset, offset + limit).map((t) => this.transaction(t)) };
  }

  reports(q: TraceReportsQuery): TraceReports {
    const report: TraceReport = this.link
      ? {
          index: 1,
          association: 1,
          at: at(30),
          frame: 10,
          list_name: 'RPT',
          rpt_id: 'Meas',
          rcb: `${LD}/LLN0.urcbMeas01`,
          dat_set: `${LD}/LLN0.Meas`,
          decoded: true,
          seq_num: 4,
          entries: [{ ref: `${LD}/MMXU1.Hz[MX]`, reasons: ['data-change'], value: { type: 'float', text: '50.002', json: 50.002 } }],
        }
      : { index: 1, association: 1, at: at(30), frame: 10, list_name: 'RPT', rpt_id: 'Meas', decoded: false, values: REPORT_VALUES, note: 'Link a session with the report control blocks to name the entries.' };
    const all = [report].filter((r) => !q.association || r.association === q.association);
    const { offset, limit } = page(q.offset, q.limit);
    return { total: all.length, offset, reports: all.slice(offset, offset + limit) };
  }

  streams(): TraceStreams {
    return {
      goose: [
        {
          src: '00:30:a7:00:00:01',
          dst: '01:0c:cd:01:00:01',
          app_id: 0x1000,
          vlan: -1,
          gocb_ref: `${LD}/LLN0$GO$gcb01`,
          dat_set: `${LD}/LLN0$Events`,
          go_id: 'gcb01',
          conf_rev: 1,
          messages: 20,
          changes: 1,
          first: at(0),
          last: at(80),
          simulation: 0,
          nds_com: 0,
          issues: { 'sqNum gap': 2 },
          max_per_second: 20,
          ...(this.link ? { model: 'unknown', note: 'The linked model defines no GOOSE control blocks.' } : {}),
        },
      ],
      sv: [],
    };
  }

  findings(q: FindingsQuery): TraceFindings {
    const all = this.findingsAll();
    const bySeverity: Record<string, number> = {};
    for (const f of all) {
      bySeverity[f.severity] = (bySeverity[f.severity] ?? 0) + 1;
    }
    const matching = all.filter((f) => (!q.severity || f.severity === q.severity) && (!q.kind || f.kind === q.kind));
    const { offset, limit } = page(q.offset, q.limit);
    return { total: matching.length, offset, by_severity: bySeverity, findings: matching.slice(offset, offset + limit) };
  }

  exportText(what: 'findings' | 'timeline', json: boolean): string {
    if (what === 'findings') {
      const all = this.findingsAll();
      return json ? JSON.stringify({ findings: all }) : ['severity,kind,association,frame,message', ...all.map((f) => `${f.severity},${f.kind},${f.association ?? ''},${f.frame ?? ''},${f.message}`)].join('\n');
    }
    return json
      ? JSON.stringify({ transactions: TRANSACTIONS })
      : ['index,association,invoke_id,service,status', ...TRANSACTIONS.map((t) => `${t.index},${t.association},${t.invoke_id},${t.service},${t.status}`)].join('\n');
  }
}

function page(offset = 0, limit = 0): { offset: number; limit: number } {
  return { offset: Math.max(0, offset), limit: limit <= 0 ? DEFAULT_TRACE_PAGE : Math.min(limit, MAX_TRACE_PAGE) };
}

/** The wire view of an operation: request and response units of its requests. */
export function mockWire(sessionId: string, operation: number, invokeIds: number[]): WireView {
  const units = invokeIds.flatMap((id, i) => [
    {
      seq: 2 * i + 1,
      at: new Date().toISOString(),
      direction: 'send' as const,
      association: 'observe' as const,
      bytes: 24,
      hex: '0100010061133011020101a00ca10a300830068004' + id.toString(16).padStart(2, '0'),
      session: 'data',
      presentation: 'user-data',
      pdu: { kind: 'confirmed-request', invoke_id: id, service: 'Read', objects: [{ ...HZ }] },
    },
    {
      seq: 2 * i + 2,
      at: new Date().toISOString(),
      direction: 'recv' as const,
      association: 'observe' as const,
      bytes: 20,
      hex: '0100010061113a0f020101a10aa108870442484000',
      session: 'data',
      presentation: 'user-data',
      pdu: { kind: 'confirmed-response', invoke_id: id, service: 'Read', values: [{ type: 'float' as const, text: '50.002', json: 50.002 }] },
    },
  ]);
  return { session_id: sessionId, operation, units };
}
