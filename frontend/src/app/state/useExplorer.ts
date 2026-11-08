import { useCallback, useEffect, useReducer, useRef } from 'react';
import { toServiceError, type LinkStatus, type Service, type ServiceError } from '../service/service';
import type {
  ExplorerEvent,
  ExportFormat,
  FileUpload,
  Health,
  ModelNode,
  Operation,
  Profile,
  ReadValue,
  SearchQuery,
  SearchResult,
  SessionInfo,
  TargetConfig,
} from '../service/types';

/** Operations kept in the activity log. */
export const MAX_ACTIVITY = 200;

export interface ActivityEntry {
  seq: number;
  sessionId?: string;
  operation: Operation;
}

export interface ReadEntry {
  at: string;
  source: 'ied' | 'snapshot';
  value: ReadValue;
}

/** What the window holds of one session. */
export interface SessionView {
  info: SessionInfo;
  /** Children per node reference ('' is the list of logical devices). */
  children: Record<string, ModelNode[]>;
  expanded: Record<string, boolean>;
  selected: ModelNode | null;
  /** Last read per FC-qualified reference. */
  reads: Record<string, ReadEntry>;
  search: { query: SearchQuery; result: SearchResult } | null;
}

/** A point pinned to the watch table: an FC-qualified reference of a session. */
export interface WatchEntry {
  sessionId: string;
  ref: string;
}

export const watchKey = (e: WatchEntry) => `${e.sessionId}|${e.ref}`;

export interface ExplorerState {
  health: Health | null;
  link: LinkStatus;
  sessions: Record<string, SessionView>;
  order: string[];
  active: string | null;
  connecting: boolean;
  connectError: ServiceError | null;
  watch: WatchEntry[];
  /** Latest value per watch entry, from polling or a read. */
  latest: Record<string, ReadEntry>;
  reading: boolean;
  profiles: Profile[];
  activity: ActivityEntry[];
  error: string | null;
}

export const initialState: ExplorerState = {
  health: null,
  link: 'up',
  sessions: {},
  order: [],
  active: null,
  connecting: false,
  connectError: null,
  watch: [],
  latest: {},
  reading: false,
  profiles: [],
  activity: [],
  error: null,
};

type Action =
  | { type: 'health'; health: Health }
  | { type: 'link'; link: LinkStatus }
  | { type: 'connecting' }
  | { type: 'opened'; session: SessionInfo; roots: ModelNode[] }
  | { type: 'connectFailed'; error: ServiceError }
  | { type: 'forget'; id: string }
  | { type: 'activate'; id: string | null }
  | { type: 'children'; id: string; ref: string; nodes: ModelNode[] }
  | { type: 'toggle'; id: string; ref: string; open: boolean }
  | { type: 'select'; id: string; node: ModelNode | null }
  | { type: 'search'; id: string; search: SessionView['search'] }
  | { type: 'reading'; on: boolean }
  | { type: 'read'; id: string; at: string; source: ReadEntry['source']; values: ReadValue[] }
  | { type: 'watch'; entry: WatchEntry; on: boolean }
  | { type: 'profiles'; profiles: Profile[] }
  | { type: 'event'; event: ExplorerEvent }
  | { type: 'error'; message: string | null };

function patchView(state: ExplorerState, id: string, patch: (v: SessionView) => Partial<SessionView>): ExplorerState {
  const view = state.sessions[id];
  return view ? { ...state, sessions: { ...state.sessions, [id]: { ...view, ...patch(view) } } } : state;
}

export function reducer(state: ExplorerState, action: Action): ExplorerState {
  switch (action.type) {
    case 'health':
      return { ...state, health: action.health };
    case 'link':
      return { ...state, link: action.link };
    case 'connecting':
      return { ...state, connecting: true, connectError: null, error: null };
    case 'opened': {
      const { session } = action;
      const view: SessionView = { info: session, children: { '': action.roots }, expanded: {}, selected: null, reads: {}, search: null };
      const order = state.order.includes(session.id) ? state.order : [...state.order, session.id];
      // A session that already polls (a reloaded page) brings its points back.
      const known = new Set(state.watch.map(watchKey));
      const adopted = (session.polling?.refs ?? []).map((ref) => ({ sessionId: session.id, ref })).filter((e) => !known.has(watchKey(e)));
      return {
        ...state,
        connecting: false,
        sessions: { ...state.sessions, [session.id]: view },
        order,
        active: session.id,
        watch: [...state.watch, ...adopted],
      };
    }
    case 'connectFailed':
      return { ...state, connecting: false, connectError: action.error };
    case 'forget': {
      const { [action.id]: _, ...sessions } = state.sessions;
      const order = state.order.filter((id) => id !== action.id);
      const watch = state.watch.filter((e) => e.sessionId !== action.id);
      return { ...state, sessions, order, watch, active: state.active === action.id ? (order.at(-1) ?? null) : state.active };
    }
    case 'activate':
      return { ...state, active: action.id, connectError: null };
    case 'children':
      return patchView(state, action.id, (v) => ({ children: { ...v.children, [action.ref]: action.nodes } }));
    case 'toggle':
      return patchView(state, action.id, (v) => ({ expanded: { ...v.expanded, [action.ref]: action.open } }));
    case 'select':
      return patchView(state, action.id, () => ({ selected: action.node }));
    case 'search':
      return patchView(state, action.id, () => ({ search: action.search }));
    case 'reading':
      return { ...state, reading: action.on };
    case 'read': {
      const entries = action.values.map((value) => [value.ref, { at: action.at, source: action.source, value }] as const);
      const next = patchView(state, action.id, (v) => ({ reads: { ...v.reads, ...Object.fromEntries(entries) } }));
      const latest = { ...next.latest };
      for (const [ref, entry] of entries) {
        const key = watchKey({ sessionId: action.id, ref });
        if (state.watch.some((w) => watchKey(w) === key)) {
          latest[key] = entry;
        }
      }
      return { ...next, latest, reading: false };
    }
    case 'watch': {
      const key = watchKey(action.entry);
      const has = state.watch.some((w) => watchKey(w) === key);
      if (action.on === has) {
        return state;
      }
      return { ...state, watch: action.on ? [...state.watch, action.entry] : state.watch.filter((w) => watchKey(w) !== key) };
    }
    case 'profiles':
      return { ...state, profiles: action.profiles };
    case 'event': {
      const { event } = action;
      if (event.kind === 'operation' && event.operation) {
        const entry = { seq: event.seq, sessionId: event.session_id, operation: event.operation };
        return { ...state, activity: [entry, ...state.activity].slice(0, MAX_ACTIVITY) };
      }
      if (event.kind === 'sample' && event.sample) {
        const { sample } = event;
        const latest = { ...state.latest };
        for (const value of sample.values) {
          latest[watchKey({ sessionId: sample.session_id, ref: value.ref })] = { at: sample.at, source: 'ied', value };
        }
        return { ...state, latest };
      }
      if (event.kind === 'session' && event.session && state.sessions[event.session.id] && event.session.state !== 'closed') {
        const info = event.session;
        return patchView(state, info.id, () => ({ info }));
      }
      return state;
    }
    case 'error':
      return { ...state, error: action.message, reading: false };
  }
}

/** The refs of the watch table that belong to one session. */
export function watchedRefs(watch: WatchEntry[], sessionId: string): string[] {
  return watch.filter((w) => w.sessionId === sessionId).map((w) => w.ref);
}

/** The Explorer's state and actions, over a Service. */
export function useExplorer(service: Service) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const stateRef = useRef(state);
  stateRef.current = state;

  const fail = useCallback((err: unknown) => dispatch({ type: 'error', message: toServiceError(err).message }), []);

  const adopt = useCallback(
    async (session: SessionInfo) => {
      dispatch({ type: 'opened', session, roots: await service.browse(session.id, '') });
    },
    [service],
  );

  const loadProfiles = useCallback(async () => {
    try {
      dispatch({ type: 'profiles', profiles: await service.profiles() });
    } catch {
      /* an engine without profiles */
    }
  }, [service]);

  useEffect(() => {
    service.health().then(
      (health) => {
        dispatch({ type: 'health', health });
        if (health.features.includes('profiles')) {
          void loadProfiles();
        }
      },
      () => {},
    );
    // A page that is reloaded adopts the sessions still open on the engine.
    service.sessions().then(
      async (list) => {
        for (const session of list) {
          if (!stateRef.current.sessions[session.id]) {
            await adopt(session).catch(() => {});
          }
        }
      },
      () => {},
    );
    return service.subscribe(
      0,
      (event) => dispatch({ type: 'event', event }),
      (link) => dispatch({ type: 'link', link }),
    );
  }, [service, adopt, loadProfiles]);

  const open = useCallback(
    async (opener: () => Promise<SessionInfo>) => {
      dispatch({ type: 'connecting' });
      try {
        await adopt(await opener());
      } catch (err) {
        dispatch({ type: 'connectFailed', error: toServiceError(err) });
      }
    },
    [adopt],
  );

  const connect = useCallback((cfg: TargetConfig) => open(() => service.openSession(cfg)), [open, service]);
  const openSCL = useCallback((file: FileUpload, ied: string, ap: string) => open(() => service.openSCL(file, ied, ap)), [open, service]);
  const openSnapshot = useCallback((file: FileUpload) => open(() => service.openSnapshot(file)), [open, service]);

  const close = useCallback(
    async (id: string) => {
      try {
        await service.closeSession(id);
      } catch {
        /* gone already */
      }
      dispatch({ type: 'forget', id });
    },
    [service],
  );

  const activate = useCallback((id: string | null) => dispatch({ type: 'activate', id }), []);

  const toggle = useCallback(
    async (id: string, node: ModelNode, isOpen: boolean) => {
      dispatch({ type: 'toggle', id, ref: node.ref, open: isOpen });
      if (!isOpen || stateRef.current.sessions[id]?.children[node.ref]) {
        return;
      }
      try {
        dispatch({ type: 'children', id, ref: node.ref, nodes: await service.browse(id, node.ref) });
      } catch (err) {
        fail(err);
      }
    },
    [service, fail],
  );

  const select = useCallback((id: string, node: ModelNode | null) => dispatch({ type: 'select', id, node }), []);

  const search = useCallback(
    async (id: string, query: SearchQuery | null) => {
      if (query === null) {
        dispatch({ type: 'search', id, search: null });
        return;
      }
      try {
        dispatch({ type: 'search', id, search: { query, result: await service.search(id, query) } });
      } catch (err) {
        fail(err);
      }
    },
    [service, fail],
  );

  const read = useCallback(
    async (id: string, refs: string[]) => {
      if (refs.length === 0) {
        return;
      }
      dispatch({ type: 'reading', on: true });
      try {
        const result = await service.read(id, { refs });
        dispatch({ type: 'read', id, at: result.at, source: result.source, values: result.values });
      } catch (err) {
        fail(err);
      }
    },
    [service, fail],
  );

  /** Pins or unpins a point; a session that polls polls the new set. */
  const watch = useCallback(
    async (entry: WatchEntry, on: boolean) => {
      dispatch({ type: 'watch', entry, on });
      const polling = stateRef.current.sessions[entry.sessionId]?.info.polling;
      if (!polling) {
        return;
      }
      const refs = new Set(polling.refs);
      if (on) {
        refs.add(entry.ref);
      } else {
        refs.delete(entry.ref);
      }
      try {
        if (refs.size === 0) {
          await service.stopPolling(entry.sessionId);
        } else {
          await service.startPolling(entry.sessionId, { refs: [...refs], interval_ms: polling.interval_ms });
        }
      } catch (err) {
        fail(err);
      }
    },
    [service, fail],
  );

  /** Starts polling the watched points of every connected session, or stops it. */
  const setPolling = useCallback(
    async (on: boolean, intervalMs: number) => {
      const { sessions, watch: entries } = stateRef.current;
      for (const view of Object.values(sessions)) {
        const id = view.info.id;
        const refs = watchedRefs(entries, id);
        try {
          if (on && view.info.state === 'connected' && refs.length > 0) {
            await service.startPolling(id, { refs, interval_ms: intervalMs });
          } else if (!on && view.info.polling) {
            await service.stopPolling(id);
          }
        } catch (err) {
          fail(err);
        }
      }
    },
    [service, fail],
  );

  const exportSession = useCallback(
    async (id: string, format: ExportFormat, refs: string[]) => {
      try {
        return await service.exportSession(id, format, refs);
      } catch (err) {
        fail(err);
        return null;
      }
    },
    [service, fail],
  );

  const saveProfile = useCallback(
    async (profile: Profile) => {
      try {
        const saved = await service.saveProfile(profile);
        await loadProfiles();
        return saved;
      } catch (err) {
        fail(err);
        return null;
      }
    },
    [service, loadProfiles, fail],
  );

  const deleteProfile = useCallback(
    async (id: string) => {
      try {
        await service.deleteProfile(id);
        await loadProfiles();
      } catch (err) {
        fail(err);
      }
    },
    [service, loadProfiles, fail],
  );

  const clearError = useCallback(() => dispatch({ type: 'error', message: null }), []);

  return {
    state,
    connect,
    openSCL,
    openSnapshot,
    close,
    activate,
    toggle,
    select,
    search,
    read,
    watch,
    setPolling,
    exportSession,
    saveProfile,
    deleteProfile,
    clearError,
    /** Shows an error that happened outside the hook's own actions. */
    report: fail,
    inspectSCL: service.inspectSCL.bind(service),
  };
}
