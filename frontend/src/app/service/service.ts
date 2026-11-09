// TypeScript mirror of internal/service/service.go.

import type {
  DataSet,
  ExplorerEvent,
  ExportFile,
  ExportFormat,
  FileUpload,
  FileEntry,
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
  SessionInfo,
  SettingGroups,
  TargetConfig,
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
  profiles(): Promise<Profile[]>;
  /** Creates (empty id) or replaces a profile. */
  saveProfile(profile: Profile): Promise<Profile>;
  deleteProfile(id: string): Promise<void>;
  /**
   * Delivers the event stream. Events above sinceSeq that the engine still
   * holds are replayed first; 0 delivers new events only. onLink, when given,
   * is told about changes of the link to the engine; an in-process engine
   * never calls it. Returns the function that ends the subscription.
   */
  subscribe(sinceSeq: number, onEvent: (event: ExplorerEvent) => void, onLink?: (status: LinkStatus) => void): () => void;
}
