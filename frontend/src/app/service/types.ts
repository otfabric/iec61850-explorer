// TypeScript mirror of internal/model. Field names are the JSON names.

/** An RFC 3339 time in UTC. */
export type Timestamp = string;

export type Mode = 'observe' | 'commission';

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
  addressing?: Addressing;
}

/**
 * ISO addressing (the IED's OSI-TSEL, OSI-SSEL, OSI-PSEL, OSI-AP-Title,
 * OSI-AE-Qualifier). Left out: the default. Empty string: omitted on the wire.
 * Selectors are hexadecimal; AP-titles are written 1.3.9999.23.
 */
export interface Addressing {
  local_tsel?: string;
  remote_tsel?: string;
  local_ssel?: string;
  remote_ssel?: string;
  local_psel?: string;
  remote_psel?: string;
  local_ap_title?: string;
  remote_ap_title?: string;
  local_ae_qualifier?: number;
  remote_ae_qualifier?: number;
}

export interface Profile {
  /** Empty when creating. */
  id: string;
  name: string;
  target: TargetConfig;
  notes?: string;
  updated_at?: Timestamp;
}

export type SessionState = 'connected' | 'offline' | 'lost' | 'closed';

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
  polling?: PollState;
  scl?: SCLOrigin;
  snapshot?: SnapshotOrigin;
  /** Set while the session holds a Commission grant (mode 'commission'). */
  grant?: Grant;
}

export interface SCLOrigin {
  file: string;
  ied: string;
  access_point: string;
}

export interface SnapshotOrigin {
  file: string;
  captured_at: Timestamp;
}

export interface PollConfig {
  refs: string[];
  /** 200..3600000; defaults to 1000. */
  interval_ms?: number;
}

export interface PollState {
  refs: string[];
  interval_ms: number;
  started_at: Timestamp;
  samples: number;
  failures: number;
  overruns: number;
}

export interface Sample {
  session_id: string;
  seq: number;
  at: Timestamp;
  duration_ms: number;
  values: ReadValue[];
  overruns: number;
}

export interface SearchQuery {
  text?: string;
  ln_class?: string;
  fc?: string;
  kind?: NodeKind;
  limit?: number;
}

export interface SearchResult {
  hits: ModelNode[];
  truncated: boolean;
}

/** A file the user chose, handed to the engine. */
export interface FileUpload {
  name: string;
  data: Uint8Array;
}

export interface SCLDiagnostic {
  severity: 'error' | 'warning' | 'info';
  code?: string;
  path?: string;
  message: string;
}

export interface SCLAccessPoint {
  name: string;
  logical_devices: number;
  target?: TargetConfig;
}

export interface SCLIED {
  name: string;
  desc?: string;
  manufacturer?: string;
  type?: string;
  access_points: SCLAccessPoint[];
}

export interface SCLInfo {
  name: string;
  kind: string;
  version: string;
  ieds: SCLIED[];
  diagnostics: SCLDiagnostic[];
  diagnostics_dropped?: number;
}

export type ExportFormat = 'snapshot' | 'csv';

export interface ExportFile {
  name: string;
  mime: string;
  text: string;
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
  /** 'ied': read now; 'snapshot': the values a snapshot holds. */
  source: 'ied' | 'snapshot';
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
  action:
    | 'connect'
    | 'discover'
    | 'read'
    | 'close'
    | 'datasets'
    | 'reports'
    | 'files'
    | 'download'
    | 'journals'
    | 'journal'
    | 'setting-groups'
    | 'grant'
    | 'revoke'
    | 'write'
    | 'subscribe'
    | 'unsubscribe'
    | 'gi'
    | 'create-dataset'
    | 'delete-dataset'
    | 'select-active-sg'
    | 'control';
  target?: string;
  started_at: Timestamp;
  duration_ms: number;
  ok: boolean;
  error?: OpError;
  requests: MMSRequest[];
  requests_dropped?: number;
}

export type EventKind = 'session' | 'operation' | 'sample' | 'resync' | 'audit' | 'report';

export interface ExplorerEvent {
  seq: number;
  kind: EventKind;
  session_id?: string;
  at: Timestamp;
  session?: SessionInfo;
  operation?: Operation;
  sample?: Sample;
  audit?: AuditRecord;
  report?: Report;
}

export interface DataSetMember {
  ref: string;
  mms: MMSName;
}

export interface DataSet {
  ref: string;
  mms: MMSName;
  deletable: boolean;
  desc?: string;
  /** 'ied' or 'scl'. */
  source: string;
  members: DataSetMember[];
}

export interface RCB {
  ref: string;
  mms: MMSName;
  buffered: boolean;
  source: string;
  rpt_id: string;
  dat_set: string;
  conf_rev: number;
  opt_flds: string[];
  trg_ops: string[];
  buf_tm_ms: number;
  intg_pd_ms: number;
  rpt_ena?: boolean;
  resv?: boolean;
  resv_tms?: number;
  sq_num?: number;
  gi?: boolean;
  entry_id?: string;
  owner?: string;
  error?: OpError;
}

export interface FileEntry {
  name: string;
  size: number;
  last_modified?: Timestamp;
}

export interface JournalRequest {
  ld: string;
  journal: string;
  start?: Timestamp;
  stop?: Timestamp;
}

export interface JournalVariable {
  tag: string;
  value: Value;
}

export interface JournalEntry {
  entry_id: string;
  time: Timestamp;
  variables: JournalVariable[];
}

export interface JournalResult {
  entries: JournalEntry[];
  more_follows: boolean;
}

export interface SettingGroups {
  ld: string;
  num_of_sgs: number;
  act_sg: number;
  edit_sg: number;
  cnf_edit: boolean;
  resv_tms: number;
}

// Commission mode (ADR 0004).

export type Capability = 'write' | 'reports' | 'datasets' | 'setting-groups' | 'controls';

/** The capabilities a grant may carry, in the order the dialog shows them. */
export const CAPABILITIES: Capability[] = ['write', 'reports', 'datasets', 'setting-groups', 'controls'];

/** The functional constraints a write may target. */
export const WRITABLE_FCS = ['SP', 'CF', 'DC', 'SV', 'BL'];

export const DEFAULT_GRANT_MINUTES = 30;
export const MAX_GRANT_MINUTES = 480;

export interface GrantRequest {
  reason: string;
  minutes?: number;
  capabilities: Capability[];
}

export interface Grant {
  reason: string;
  capabilities: Capability[];
  granted_at: Timestamp;
  expires_at: Timestamp;
  subscriptions: Subscription[];
}

export interface WriteRequest {
  ref: string;
  value: string;
}

export interface WriteResult {
  ref: string;
  mms: MMSName;
  at: Timestamp;
  written?: Value;
  before?: Value;
  after?: Value;
  verified: boolean;
  error?: OpError;
  duration_ms: number;
}

export const DEFAULT_CONTROL_TIMEOUT_MS = 10_000;

export interface ControlRequest {
  /** The data object, such as "LD/XCBR1.Pos" (no FC). */
  ref: string;
  /** ctlVal as text. */
  value: string;
  test?: boolean;
  /** Ask the IED to check the interlocking / the synchronism (Check). */
  interlock_check?: boolean;
  synchro_check?: boolean;
  timeout_ms?: number;
}

export type ControlStepName = 'select' | 'operate' | 'termination' | 'cancel';

export interface ControlStep {
  step: ControlStepName;
  ok: boolean;
  add_cause?: string;
  error?: string;
  duration_ms: number;
}

export type ControlOutcome = 'ok' | 'refused' | 'negative' | 'no-termination' | 'failed';

export interface ControlResult {
  ref: string;
  at: Timestamp;
  duration_ms: number;
  ctl_model: string;
  ctl_num: number;
  written?: Value;
  test?: boolean;
  interlock_check?: boolean;
  synchro_check?: boolean;
  steps: ControlStep[];
  outcome: ControlOutcome;
  add_cause?: string;
  /** stVal before and after the sequence. */
  before?: Value;
  after?: Value;
}

export type AuditAction =
  | 'grant'
  | 'revoke'
  | 'expire'
  | 'end'
  | 'write'
  | 'cleanup'
  | 'subscribe'
  | 'unsubscribe'
  | 'gi'
  | 'create-dataset'
  | 'delete-dataset'
  | 'select-active-sg'
  | 'control';

export interface AuditRecord {
  seq: number;
  at: Timestamp;
  session_id: string;
  target: string;
  ied?: string;
  action: AuditAction;
  outcome: 'ok' | 'mismatch' | 'failed';
  reason?: string;
  capabilities?: Capability[];
  ref?: string;
  before?: string;
  written?: string;
  after?: string;
  detail?: string;
  error?: string;
}

export interface AuditQuery {
  session_id?: string;
  limit?: number;
}

export const MAX_SUBSCRIPTIONS = 8;
export const MAX_DATASET_MEMBERS = 100;

export interface SubscribeRequest {
  /** The RCB reference Reports returns, such as "LD/LLN0.RP.urcb01". */
  rcb: string;
  gi?: boolean;
}

export interface Subscription {
  id: string;
  rcb: string;
  mms: MMSName;
  buffered: boolean;
  rpt_id: string;
  dat_set: string;
  since: Timestamp;
  reports: number;
  dropped: number;
}

export interface ReportEntry {
  ref: string;
  reasons?: string[];
  value: Value;
}

export interface Report {
  subscription_id: string;
  rcb: string;
  rpt_id: string;
  received_at: Timestamp;
  seq_num?: number;
  time?: Timestamp;
  conf_rev?: number;
  entry_id?: string;
  buf_ovfl?: boolean;
  entries: ReportEntry[];
}

export interface CreateDataSetRequest {
  /** Such as "LD/LLN0.Explorer1". */
  ref: string;
  /** FC-qualified references. */
  members: string[];
}

export interface SelectSGRequest {
  ld: string;
  sg: number;
}

/** Compares two open sessions: A is the reference, B the subject. */
export interface CompareRequest {
  a: string;
  b: string;
  /** Adds the values of configuration and setting attributes (CF, DC, SP, SG); reads them from a live IED. */
  values?: boolean;
}

export type CompareCategoryName = 'model' | 'datasets' | 'reports' | 'values';
export type DifferenceKind = 'only_a' | 'only_b' | 'changed';

export interface CompareSide {
  session_id: string;
  source: 'ied' | 'scl' | 'snapshot';
  label: string;
}

export interface CompareCategory {
  name: CompareCategoryName;
  compared: boolean;
  /** Why the category was not compared, or compared only in part. */
  reason?: string;
  items: number;
  same: number;
  only_a: number;
  only_b: number;
  changed: number;
}

export interface Difference {
  category: CompareCategoryName;
  kind: DifferenceKind;
  ref: string;
  field?: string;
  a?: string;
  b?: string;
  note?: string;
}

export interface Comparison {
  a: CompareSide;
  b: CompareSide;
  categories: CompareCategory[];
  differences: Difference[];
  truncated: boolean;
  duration_ms: number;
}
