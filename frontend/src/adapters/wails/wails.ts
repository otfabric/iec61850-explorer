// The only file that touches the Wails runtime. The Go service `App` in
// internal/gui has methods that mirror service.Service, plus SaveFile.

import { Browser, Call, Clipboard, Events } from '@wailsio/runtime';
import type { Mode, Platform } from '../../app/platform/platform';
import { loadTheme, persistTheme } from '../../app/platform/themeStorage';
import { ServiceError, toServiceError, type ErrorCode, type Service } from '../../app/service/service';
import type { ExplorerEvent, Health, ModelNode, OpError, ReadRequest, ReadResult, SessionInfo, TargetConfig } from '../../app/service/types';

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

export class WailsService implements Service {
  health = () => call<Health>('Health');
  openSession = (cfg: TargetConfig) => call<SessionInfo>('OpenSession', cfg);
  closeSession = (id: string) => call<void>('CloseSession', id);
  sessions = () => call<SessionInfo[]>('Sessions');
  browse = (id: string, ref: string) => call<ModelNode[]>('Browse', id, ref);
  read = (id: string, req: ReadRequest) => call<ReadResult>('Read', id, req);

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

  openExternal(url: string): void {
    void Browser.OpenURL(url);
  }

  copyText(text: string): Promise<void> {
    return Clipboard.SetText(text);
  }
}
