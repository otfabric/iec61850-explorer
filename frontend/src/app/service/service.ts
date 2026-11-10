// TypeScript mirror of internal/service/service.go.

import type {
  AuditQuery,
  AuditRecord,
  CompareRequest,
  Comparison,
  Conformance,
  NamespacePackage,
  Namespaces,
  Semantics,
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
  OpError,
  PollConfig,
  Profile,
  RCB,
  ReadRequest,
  ReadResult,
  SCLInfo,
  SearchQuery,
  SearchResult,
  SelectSGRequest,
  SessionInfo,
  SettingGroups,
  Settings,
  SubscribeRequest,
  Subscription,
  TargetConfig,
  WriteRequest,
  WriteResult,
  ControlRequest,
  ControlResult,
  FindingsQuery,
  Timeline,
  TimelineQuery,
  TraceAssociation,
  TraceFindings,
  TraceReports,
  TraceReportsQuery,
  TraceStreams,
  TraceSummary,
  WireView,
} from './types';

export type ErrorCode =
  | 'invalid_argument'
  | 'not_found'
  | 'limit_exceeded'
  | 'not_allowed'
  | 'connect_failed'
  | 'discovery_failed'
  | 'session_lost'
  | 'unauthorized'
  | 'canceled'
  | 'internal';

/**
 * The link between this window and the engine. Only a transport that can lose
 * the engine reports it: 'down' while it retries, 'up' when the event stream
 * is open again, 'unauthorized' when the engine refuses the token (final).
 */
export type LinkStatus = 'up' | 'down' | 'unauthorized';

/** The error every Service method rejects with. */
export class ServiceError extends Error {
  readonly code: ErrorCode;
  /** The classified protocol error of a failed connect or discovery. */
  readonly detail?: OpError;

  constructor(code: ErrorCode, message: string, detail?: OpError) {
    super(message);
    this.name = 'ServiceError';
    this.code = code;
    this.detail = detail;
  }
}

/** Turns whatever a transport threw into a ServiceError. */
export function toServiceError(err: unknown): ServiceError {
  if (err instanceof ServiceError) {
    return err;
  }
  return new ServiceError('internal', err instanceof Error ? err.message : String(err));
}

/** The IEC 61850 engine as the user interface sees it. */
export interface Service {
  health(): Promise<Health>;
  openSession(cfg: TargetConfig): Promise<SessionInfo>;
  closeSession(id: string): Promise<void>;
  sessions(): Promise<SessionInfo[]>;
  /** Children of a node of the discovered model; '' lists the logical devices. */
  browse(id: string, ref: string): Promise<ModelNode[]>;
  read(id: string, req: ReadRequest): Promise<ReadResult>;
  /** Finds nodes of the discovered model; no IED traffic. */
  search(id: string, query: SearchQuery): Promise<SearchResult>;
  /** Polls references; samples arrive as events. Replaces a running poller. */
  startPolling(id: string, cfg: PollConfig): Promise<void>;
  stopPolling(id: string): Promise<void>;
  /** Reads the references once and renders the session. */
  exportSession(id: string, format: ExportFormat, refs: string[]): Promise<ExportFile>;
  openSnapshot(file: FileUpload): Promise<SessionInfo>;
  inspectSCL(file: FileUpload): Promise<SCLInfo>;
  openSCL(file: FileUpload, ied: string, accessPoint: string): Promise<SessionInfo>;
  /** Data sets of a logical device: from the IED, or from the SCL of an offline session. */
  dataSets(id: string, ld: string): Promise<DataSet[]>;
  readDataSet(id: string, ref: string): Promise<ReadResult>;
  reports(id: string, ld: string): Promise<RCB[]>;
  files(id: string, dir: string): Promise<FileEntry[]>;
  /** Downloads a file of the IED and saves it where the user wants. Resolves to false when cancelled. */
  saveDeviceFile(id: string, name: string): Promise<boolean>;
  journals(id: string, ld: string): Promise<string[]>;
  readJournal(id: string, req: JournalRequest): Promise<JournalResult>;
  settingGroups(id: string, ld: string): Promise<SettingGroups>;
  /** Puts a session in Commission mode: a second, audited association (ADR 0004). */
  grantCommission(id: string, req: GrantRequest): Promise<SessionInfo>;
  /** Ends a grant; a session without one is not an error. */
  revokeCommission(id: string): Promise<SessionInfo>;
  /** Writes one data attribute and reads it back; needs the write capability. */
  write(id: string, req: WriteRequest): Promise<WriteResult>;
  /** Operates a controllable data object (select, operate, termination); needs the controls capability. */
  control(id: string, req: ControlRequest): Promise<ControlResult>;
  /** Subscribes to a report control block (reserve, enable, GI); reports arrive as events. */
  subscribeReport(id: string, req: SubscribeRequest): Promise<Subscription>;
  /** Disables and releases the RCB of a subscription. */
  unsubscribe(id: string, subscriptionId: string): Promise<void>;
  triggerGI(id: string, subscriptionId: string): Promise<void>;
  createDataSet(id: string, req: CreateDataSetRequest): Promise<DataSet>;
  /** Deletes a data set the IED marks deletable. */
  deleteDataSet(id: string, ref: string): Promise<void>;
  /** Selects the active setting group and reads the control block back. */
  selectActiveSG(id: string, req: SelectSGRequest): Promise<SettingGroups>;
  /** Recent audit records, newest first. */
  audit(query: AuditQuery): Promise<AuditRecord[]>;
  /** Compares two open sessions: the model, the data sets and the report control blocks. Reads only. */
  compare(req: CompareRequest): Promise<Comparison>;
  /** What the IEC 61850 namespace defines for a node (reference without FC). */
  semantics(id: string, ref: string): Promise<Semantics>;
  /** The session's model against the namespace: missing mandatory data objects, unknown classes, CDC mismatches. */
  conformance(id: string): Promise<Conformance>;
  /** The embedded namespace releases and the packages the user loaded. */
  namespaces(): Promise<Namespaces>;
  /** Loads a package of namespace definitions (an NSD ZIP archive or one NSD document) and keeps it. */
  addNamespaces(file: FileUpload): Promise<NamespacePackage>;
  removeNamespaces(id: string): Promise<void>;
  /** Analyses a capture (pcap, pcapng, plain or gzip) and opens it as a trace session; ports are the MMS server ports (empty: 102). */
  openTrace(file: FileUpload, ports: number[]): Promise<SessionInfo>;
  /** Correlates a trace session with a model session (live, SCL or snapshot); '' removes the link. */
  linkTrace(id: string, session: string): Promise<SessionInfo>;
  traceSummary(id: string): Promise<TraceSummary>;
  traceAssociations(id: string): Promise<TraceAssociation[]>;
  /** The confirmed services of a capture in time order, filtered and paged. */
  traceTimeline(id: string, query: TimelineQuery): Promise<Timeline>;
  /** The InformationReports, decoded against the linked model's data sets when it has them. */
  traceReports(id: string, query: TraceReportsQuery): Promise<TraceReports>;
  traceStreams(id: string): Promise<TraceStreams>;
  /** Gaps and anomalies of the capture, and what the linked model does not have. */
  traceFindings(id: string, query: FindingsQuery): Promise<TraceFindings>;
  /** The MMS units a live session sent and received during one operation (Operation.wire). */
  wire(id: string, operation: number): Promise<WireView>;
  profiles(): Promise<Profile[]>;
  /** Creates (empty id) or replaces a profile. */
  saveProfile(profile: Profile): Promise<Profile>;
  deleteProfile(id: string): Promise<void>;
  /** The engine's settings: the default proxy. */
  settings(): Promise<Settings>;
  /** Changes the settings; refused (not_allowed) when the daemon's operator set the default proxy. */
  saveSettings(settings: Settings): Promise<Settings>;
  /**
   * Delivers the event stream. Events above sinceSeq that the engine still
   * holds are replayed first; 0 delivers new events only. onLink, when given,
   * is told about changes of the link to the engine; an in-process engine
   * never calls it. Returns the function that ends the subscription.
   */
  subscribe(sinceSeq: number, onEvent: (event: ExplorerEvent) => void, onLink?: (status: LinkStatus) => void): () => void;
}
