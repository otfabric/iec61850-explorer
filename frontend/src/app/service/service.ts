// TypeScript mirror of internal/service/service.go.

import type { ExplorerEvent, Health, ModelNode, OpError, ReadRequest, ReadResult, SessionInfo, TargetConfig } from './types';

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
  /**
   * Delivers the event stream. Events above sinceSeq that the engine still
   * holds are replayed first; 0 delivers new events only. onLink, when given,
   * is told about changes of the link to the engine; an in-process engine
   * never calls it. Returns the function that ends the subscription.
   */
  subscribe(sinceSeq: number, onEvent: (event: ExplorerEvent) => void, onLink?: (status: LinkStatus) => void): () => void;
}
