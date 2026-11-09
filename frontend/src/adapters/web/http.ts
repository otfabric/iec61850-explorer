import type { Platform } from '../../app/platform/platform';
import { ServiceError, type ErrorCode, type LinkStatus, type Service } from '../../app/service/service';
import type {
  CreateDataSetRequest,
  SelectSGRequest,
  SubscribeRequest,
  Subscription as ReportSubscription,
  AuditQuery,
  AuditRecord,
  GrantRequest,
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
import { createSseParser } from './sse';

const BASE = '/api/v1';
const RETRY_MIN_MS = 1000;
const RETRY_MAX_MS = 30000;
// The daemon sends a keep-alive comment about every 15 s. A stream that stays
// silent this long is dead even if the socket has not noticed (a dropped
// tunnel, a sleeping laptop).
const SILENCE_MS = 45000;

interface ErrorBody {
  error?: { code?: ErrorCode; message?: string; detail?: OpError };
}

interface Subscription {
  onLink?: (status: LinkStatus) => void;
  /** Drops the current stream connection so that it is opened again. */
  reopen: () => void;
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener('abort', () => {
      clearTimeout(timer);
      resolve();
    });
  });
}

/** Service over the daemon's same-origin HTTP API. The token lives only in this object. */
export class HttpService implements Service {
  private readonly token: string | null;
  private readonly platform: Pick<Platform, 'saveBlob'>;
  private readonly subscriptions = new Set<Subscription>();

  constructor(token: string | null, platform: Pick<Platform, 'saveBlob'>) {
    this.token = token;
    this.platform = platform;
  }

  health = () => this.request<Health>('GET', '/health');
  openSession = (cfg: TargetConfig) => this.request<SessionInfo>('POST', '/sessions', cfg);
  closeSession = (id: string) => this.request<void>('DELETE', `/sessions/${encodeURIComponent(id)}`);
  sessions = () => this.request<SessionInfo[]>('GET', '/sessions');
  browse = (id: string, ref: string) => this.request<ModelNode[]>('GET', `/sessions/${encodeURIComponent(id)}/browse?ref=${encodeURIComponent(ref)}`);
  read = (id: string, req: ReadRequest) => this.request<ReadResult>('POST', `/sessions/${encodeURIComponent(id)}/read`, req);
  search = (id: string, query: SearchQuery) => this.request<SearchResult>('POST', `/sessions/${encodeURIComponent(id)}/search`, query);
  startPolling = (id: string, cfg: PollConfig) => this.request<void>('PUT', `/sessions/${encodeURIComponent(id)}/polling`, cfg);
  stopPolling = (id: string) => this.request<void>('DELETE', `/sessions/${encodeURIComponent(id)}/polling`);
  exportSession = (id: string, format: ExportFormat, refs: string[]) =>
    this.request<ExportFile>('POST', `/sessions/${encodeURIComponent(id)}/export`, { format, refs });
  openSnapshot = (file: FileUpload) => this.upload<SessionInfo>('/snapshots/open', file, {});
  inspectSCL = (file: FileUpload) => this.upload<SCLInfo>('/scl/inspect', file, {});
  openSCL = (file: FileUpload, ied: string, accessPoint: string) => this.upload<SessionInfo>('/scl/open', file, { ied, ap: accessPoint });
  dataSets = (id: string, ld: string) => this.request<DataSet[]>('GET', `/sessions/${encodeURIComponent(id)}/datasets?ld=${encodeURIComponent(ld)}`);
  readDataSet = (id: string, ref: string) => this.request<ReadResult>('POST', `/sessions/${encodeURIComponent(id)}/datasets/read`, { ref });
  reports = (id: string, ld: string) => this.request<RCB[]>('GET', `/sessions/${encodeURIComponent(id)}/reports?ld=${encodeURIComponent(ld)}`);
  files = (id: string, dir: string) => this.request<FileEntry[]>('GET', `/sessions/${encodeURIComponent(id)}/files?dir=${encodeURIComponent(dir)}`);
  journals = (id: string, ld: string) => this.request<string[]>('GET', `/sessions/${encodeURIComponent(id)}/journals?ld=${encodeURIComponent(ld)}`);
  readJournal = (id: string, req: JournalRequest) => this.request<JournalResult>('POST', `/sessions/${encodeURIComponent(id)}/journals/read`, req);
  grantCommission = (id: string, req: GrantRequest) => this.request<SessionInfo>('PUT', `/sessions/${encodeURIComponent(id)}/commission`, req);
  revokeCommission = (id: string) => this.request<SessionInfo>('DELETE', `/sessions/${encodeURIComponent(id)}/commission`);
  write = (id: string, req: WriteRequest) => this.request<WriteResult>('POST', `/sessions/${encodeURIComponent(id)}/write`, req);
  subscribeReport = (id: string, req: SubscribeRequest) => this.request<ReportSubscription>('POST', `/sessions/${encodeURIComponent(id)}/reports/subscriptions`, req);
  unsubscribe = (id: string, sub: string) =>
    this.request<void>('DELETE', `/sessions/${encodeURIComponent(id)}/reports/subscriptions/${encodeURIComponent(sub)}`);
  triggerGI = (id: string, sub: string) =>
    this.request<void>('POST', `/sessions/${encodeURIComponent(id)}/reports/subscriptions/${encodeURIComponent(sub)}/gi`);
  createDataSet = (id: string, req: CreateDataSetRequest) => this.request<DataSet>('POST', `/sessions/${encodeURIComponent(id)}/datasets`, req);
  deleteDataSet = (id: string, ref: string) => this.request<void>('DELETE', `/sessions/${encodeURIComponent(id)}/datasets?ref=${encodeURIComponent(ref)}`);
  selectActiveSG = (id: string, req: SelectSGRequest) => this.request<SettingGroups>('PUT', `/sessions/${encodeURIComponent(id)}/setting-groups/active`, req);
  audit = (query: AuditQuery) =>
    this.request<AuditRecord[]>('GET', `/audit?session=${encodeURIComponent(query.session_id ?? '')}&limit=${query.limit ?? 0}`);
  settingGroups = (id: string, ld: string) =>
    this.request<SettingGroups>('GET', `/sessions/${encodeURIComponent(id)}/setting-groups?ld=${encodeURIComponent(ld)}`);

  /**
   * Downloads with the token in a header, as every request, and hands the
   * bytes to the browser as a download: a plain link would need the token in
   * its URL.
   */
  async saveDeviceFile(id: string, name: string): Promise<boolean> {
    const response = await this.send('GET', `/sessions/${encodeURIComponent(id)}/files/download?name=${encodeURIComponent(name)}`);
    if (!response.ok) {
      throw this.failure(response, await response.text());
    }
    const saveAs = /filename="([^"]+)"/.exec(response.headers.get('Content-Disposition') ?? '')?.[1] ?? 'file';
    return this.platform.saveBlob(saveAs, await response.blob());
  }

  profiles = () => this.request<Profile[]>('GET', '/profiles');
  saveProfile = (profile: Profile) =>
    profile.id === ''
      ? this.request<Profile>('POST', '/profiles', profile)
      : this.request<Profile>('PUT', `/profiles/${encodeURIComponent(profile.id)}`, profile);
  deleteProfile = (id: string) => this.request<void>('DELETE', `/profiles/${encodeURIComponent(id)}`);

  subscribe(sinceSeq: number, onEvent: (event: ExplorerEvent) => void, onLink?: (status: LinkStatus) => void): () => void {
    const abort = new AbortController();
    const subscription: Subscription = { onLink, reopen: () => {} };
    this.subscriptions.add(subscription);
    void this.stream(sinceSeq, onEvent, subscription, abort.signal);
    return () => {
      this.subscriptions.delete(subscription);
      abort.abort();
    };
  }

  private headers(): Record<string, string> {
    return { Authorization: `Bearer ${this.token}` };
  }

  private unauthorized(message: string): ServiceError {
    this.subscriptions.forEach((subscription) => subscription.onLink?.('unauthorized'));
    return new ServiceError('unauthorized', message);
  }

  private async send(method: string, path: string, body?: unknown): Promise<Response> {
    if (this.token === null) {
      throw this.unauthorized('This page has no access token.');
    }
    try {
      const raw = body instanceof Uint8Array;
      return await fetch(BASE + path, {
        method,
        headers:
          body === undefined ? this.headers() : { ...this.headers(), 'Content-Type': raw ? 'application/octet-stream' : 'application/json' },
        body: body === undefined ? undefined : raw ? (body as BodyInit) : JSON.stringify(body),
      });
    } catch {
      // The stream may not have noticed yet: report the loss now and have the
      // stream reconnect, which is also what reports the link as up again.
      this.subscriptions.forEach((subscription) => {
        subscription.onLink?.('down');
        subscription.reopen();
      });
      throw new ServiceError('internal', 'The daemon cannot be reached.');
    }
  }

  /** The error an unsuccessful response stands for. */
  private failure(response: Response, text: string): ServiceError {
    let error: ErrorBody['error'];
    try {
      error = (JSON.parse(text) as ErrorBody).error;
    } catch {
      /* not the daemon's error format */
    }
    const message = error?.message ?? `HTTP ${response.status} ${response.statusText}`;
    return response.status === 401 ? this.unauthorized(message) : new ServiceError(error?.code ?? 'internal', message, error?.detail);
  }

  /** Sends a file as the raw request body; the query names it. */
  private async upload<T>(path: string, file: FileUpload, query: Record<string, string>): Promise<T> {
    const params = new URLSearchParams({ name: file.name, ...query });
    return this.parse<T>(await this.send('POST', `${path}?${params}`, file.data));
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.parse<T>(await this.send(method, path, body));
  }

  private async parse<T>(response: Response): Promise<T> {
    const text = await response.text();
    if (!response.ok) {
      throw this.failure(response, text);
    }
    try {
      return (text === '' ? undefined : JSON.parse(text)) as T;
    } catch {
      throw new ServiceError('internal', `HTTP ${response.status}: the answer is not JSON. Is this the daemon?`);
    }
  }

  /**
   * Reads the event stream until aborted. A broken stream is reopened with
   * backoff from the last sequence number seen, so the daemon replays what
   * was missed, or says with a `resync` event that it no longer can.
   */
  private async stream(sinceSeq: number, onEvent: (event: ExplorerEvent) => void, subscription: Subscription, signal: AbortSignal): Promise<void> {
    if (this.token === null) {
      subscription.onLink?.('unauthorized');
      return;
    }
    let lastSeq = sinceSeq;
    let delay = RETRY_MIN_MS;
    while (!signal.aborted) {
      const attempt = new AbortController();
      const stop = () => attempt.abort();
      signal.addEventListener('abort', stop);
      subscription.reopen = stop;
      let silence = setTimeout(stop, SILENCE_MS);
      try {
        const response = await fetch(`${BASE}/events?since=${lastSeq}`, { headers: { ...this.headers(), Accept: 'text/event-stream' }, signal: attempt.signal });
        if (response.status === 401) {
          subscription.onLink?.('unauthorized');
          return;
        }
        if (!response.ok || !response.body) {
          throw new Error(`HTTP ${response.status}`);
        }
        subscription.onLink?.('up');
        delay = RETRY_MIN_MS;
        const parse = createSseParser((frame) => {
          const event = JSON.parse(frame.data) as ExplorerEvent;
          // After a resync the daemon's numbering is what counts, even when it
          // is lower than before (a restarted daemon starts again at 1).
          lastSeq = event.kind === 'resync' ? event.seq : Math.max(lastSeq, event.seq);
          onEvent(event);
        });
        const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
        for (let read = await reader.read(); !read.done; read = await reader.read()) {
          clearTimeout(silence);
          silence = setTimeout(stop, SILENCE_MS);
          parse(read.value);
        }
      } catch {
        // Network failure, silence or a broken frame: reconnect below.
      } finally {
        clearTimeout(silence);
        signal.removeEventListener('abort', stop);
      }
      if (signal.aborted) {
        return;
      }
      subscription.onLink?.('down');
      await sleep(delay, signal);
      delay = Math.min(delay * 2, RETRY_MAX_MS);
    }
  }
}
