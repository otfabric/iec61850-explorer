// The only file that touches the Wails runtime. The Go service `App` in
// internal/gui has methods that mirror service.Service, plus SaveFile.

import { Browser, Call, Clipboard, Events } from '@wailsio/runtime';
import type { Mode, Platform } from '../../app/platform/platform';
import { loadTheme, persistTheme } from '../../app/platform/themeStorage';
import { ServiceError, toServiceError, type ErrorCode, type Service } from '../../app/service/service';
import type {
  CreateDataSetRequest,
  SelectSGRequest,
  SubscribeRequest,
  Subscription,
  AuditQuery,
  AuditRecord,
  CompareRequest,
  Comparison,
  Conformance,
  Semantics,
  GrantRequest,
  ControlRequest,
  ControlResult,
  WriteRequest,
  WriteResult,
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
} from '../../app/service/types';

const BINDING = 'github.com/otfabric/iec61850-explorer/internal/gui.App';
const EVENT_NAME = 'explorer:event';

interface ErrorBody {
  code: ErrorCode;
  message: string;
  detail?: OpError;
}

const isErrorBody = (value: unknown): value is ErrorBody =>
  typeof value === 'object' && value !== null && 'code' in value && 'message' in value;

/** A Go service.Error arrives as the rejection's cause or as JSON in its message. */
function fromGo(err: unknown): ServiceError {
  const candidates: unknown[] = [];
  if (err instanceof Error) {
    candidates.push(err.cause);
    try {
      candidates.push(JSON.parse(err.message));
    } catch {
      /* a plain message */
    }
  }
  const body = candidates.find(isErrorBody);
  return body ? new ServiceError(body.code, body.message, body.detail) : toServiceError(err);
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  try {
    return (await Call.ByName(`${BINDING}.${method}`, ...args)) as T;
  } catch (err) {
    throw fromGo(err);
  }
}

/** Go's []byte crosses the binding as base64. */
function toGoFile(file: FileUpload): { name: string; data: string } {
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < file.data.length; i += chunk) {
    binary += String.fromCharCode(...file.data.subarray(i, i + chunk));
  }
  return { name: file.name, data: btoa(binary) };
}

export class WailsService implements Service {
  health = () => call<Health>('Health');
  openSession = (cfg: TargetConfig) => call<SessionInfo>('OpenSession', cfg);
  closeSession = (id: string) => call<void>('CloseSession', id);
  sessions = () => call<SessionInfo[]>('Sessions');
  browse = (id: string, ref: string) => call<ModelNode[]>('Browse', id, ref);
  read = (id: string, req: ReadRequest) => call<ReadResult>('Read', id, req);
  search = (id: string, query: SearchQuery) => call<SearchResult>('Search', id, query);
  startPolling = (id: string, cfg: PollConfig) => call<void>('StartPolling', id, cfg);
  stopPolling = (id: string) => call<void>('StopPolling', id);
  exportSession = (id: string, format: ExportFormat, refs: string[]) => call<ExportFile>('Export', id, { format, refs });
  openSnapshot = (file: FileUpload) => call<SessionInfo>('OpenSnapshot', toGoFile(file));
  inspectSCL = (file: FileUpload) => call<SCLInfo>('InspectSCL', toGoFile(file));
  openSCL = (file: FileUpload, ied: string, accessPoint: string) =>
    call<SessionInfo>('OpenSCL', { file: toGoFile(file), ied, access_point: accessPoint });
  dataSets = (id: string, ld: string) => call<DataSet[]>('DataSets', id, ld);
  readDataSet = (id: string, ref: string) => call<ReadResult>('ReadDataSet', id, ref);
  reports = (id: string, ld: string) => call<RCB[]>('Reports', id, ld);
  files = (id: string, dir: string) => call<FileEntry[]>('Files', id, dir);
  // The Go side downloads, shows the native Save dialog and writes the file.
  saveDeviceFile = async (id: string, name: string) => (await call<string>('SaveDeviceFile', id, name)) !== '';
  journals = (id: string, ld: string) => call<string[]>('Journals', id, ld);
  readJournal = (id: string, req: JournalRequest) => call<JournalResult>('ReadJournal', id, req);
  grantCommission = (id: string, req: GrantRequest) => call<SessionInfo>('GrantCommission', id, req);
  revokeCommission = (id: string) => call<SessionInfo>('RevokeCommission', id);
  write = (id: string, req: WriteRequest) => call<WriteResult>('Write', id, req);
  control = (id: string, req: ControlRequest) => call<ControlResult>('Control', id, req);
  audit = (query: AuditQuery) => call<AuditRecord[]>('Audit', query);
  compare = (req: CompareRequest) => call<Comparison>('Compare', req);
  semantics = (id: string, ref: string) => call<Semantics>('Semantics', id, ref);
  conformance = (id: string) => call<Conformance>('Conformance', id);
  subscribeReport = (id: string, req: SubscribeRequest) => call<Subscription>('SubscribeReport', id, req);
  unsubscribe = (id: string, sub: string) => call<void>('Unsubscribe', id, sub);
  triggerGI = (id: string, sub: string) => call<void>('TriggerGI', id, sub);
  createDataSet = (id: string, req: CreateDataSetRequest) => call<DataSet>('CreateDataSet', id, req);
  deleteDataSet = (id: string, ref: string) => call<void>('DeleteDataSet', id, ref);
  selectActiveSG = (id: string, req: SelectSGRequest) => call<SettingGroups>('SelectActiveSG', id, req);
  settingGroups = (id: string, ld: string) => call<SettingGroups>('SettingGroups', id, ld);
  profiles = () => call<Profile[]>('Profiles');
  saveProfile = (profile: Profile) => call<Profile>('SaveProfile', profile);
  deleteProfile = (id: string) => call<void>('DeleteProfile', id);

  // The Go side forwards every engine event to the window as it happens, so
  // there is nothing to replay: sinceSeq is not used.
  subscribe(_sinceSeq: number, onEvent: (event: ExplorerEvent) => void): () => void {
    return Events.On(EVENT_NAME, (event) => onEvent(event.data as ExplorerEvent));
  }
}

export class WailsPlatform implements Platform {
  // The desktop application is always local: its engine runs in-process.
  readonly mode: Mode = { kind: 'local' };
  persistTheme = persistTheme;
  loadTheme = loadTheme;

  /** The Go side shows the native Save dialog and returns the path, or "" when cancelled. */
  async saveFile(name: string, mime: string, text: string): Promise<boolean> {
    return (await call<string>('SaveFile', name, mime, text)) !== '';
  }

  async saveBlob(name: string, blob: Blob): Promise<boolean> {
    return this.saveFile(name, blob.type, await blob.text());
  }

  openExternal(url: string): void {
    void Browser.OpenURL(url);
  }

  copyText(text: string): Promise<void> {
    return Clipboard.SetText(text);
  }
}
