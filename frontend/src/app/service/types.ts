// TypeScript mirror of internal/model. Field names are the JSON names.

/** An RFC 3339 time in UTC. */
export type Timestamp = string;

export type Mode = 'observe';

export interface Health {
  status: string;
  version: string;
  api_version: string;
  uptime_s: number;
  go_iec61850: string;
  go_mms: string;
  mode: Mode;
  time: Timestamp;
  features: string[];
}

export interface TargetConfig {
  host: string;
  /** Defaults to 102. */
  port?: number;
  /** Defaults to 10000; 100..120000. */
  timeout_ms?: number;
}

export type SessionState = 'connected' | 'lost' | 'closed';

export interface Identity {
  vendor: string;
  model: string;
  revision: string;
}

export interface DiscoverySummary {
  logical_devices: number;
  logical_nodes: number;
  data_objects: number;
  nodes: number;
  duration_ms: number;
}

export interface SessionInfo {
  id: string;
  target: TargetConfig;
  state: SessionState;
  mode: Mode;
  opened_at: Timestamp;
  identity?: Identity;
  discovery: DiscoverySummary;
  error?: OpError;
}

export type NodeKind = 'ld' | 'ln' | 'do' | 'child';

export interface ModelNode {
  name: string;
  /** Object reference without functional constraint. */
  ref: string;
  kind: NodeKind;
  ln_class?: string;
  fcs: string[];
  children: number;
}

export interface ReadRequest {
  refs: string[];
}

export interface ReadResult {
  at: Timestamp;
  duration_ms: number;
  values: ReadValue[];
}

export interface MMSName {
  domain: string;
  item_id: string;
}

export interface ReadValue {
  ref: string;
  mms: MMSName;
  value?: Value;
  error?: OpError;
}

export type ValueType =
  | 'boolean'
  | 'integer'
  | 'unsigned'
  | 'float'
  | 'bit-string'
  | 'octet-string'
  | 'visible-string'
  | 'mms-string'
  | 'utc-time'
  | 'binary-time'
  | 'structure'
  | 'array'
  | 'data-access-error'
  | 'other';

export interface Quality {
  validity: 'good' | 'invalid' | 'reserved' | 'questionable';
  flags: string[];
  raw: number;
}

export interface TimeValue {
  time: Timestamp;
  leap_second_known: boolean;
  clock_failure: boolean;
  clock_not_synchronized: boolean;
  accuracy: number;
}

export interface Value {
  name?: string;
  type: ValueType;
  text: string;
  json?: boolean | number | string;
  bits?: number;
  quality?: Quality;
  time?: TimeValue;
  children?: Value[];
}

export interface OpError {
  code: string;
  message: string;
  stage?: string;
  hint?: string;
}

export interface MMSRequest {
  service: string;
  invoke_id: number;
  duration_ms: number;
  error?: string;
}

export interface Operation {
  action: 'connect' | 'discover' | 'read' | 'close';
  target?: string;
  started_at: Timestamp;
  duration_ms: number;
  ok: boolean;
  error?: OpError;
  requests: MMSRequest[];
  requests_dropped?: number;
}

export type EventKind = 'session' | 'operation' | 'resync';

export interface ExplorerEvent {
  seq: number;
  kind: EventKind;
  session_id?: string;
  at: Timestamp;
  session?: SessionInfo;
  operation?: Operation;
}
