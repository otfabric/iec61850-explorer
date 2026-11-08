import { useCallback, useEffect, useReducer, useRef } from 'react';
import { toServiceError, type LinkStatus, type Service, type ServiceError } from '../service/service';
import type { ExplorerEvent, Health, ModelNode, Operation, ReadValue, SessionInfo, TargetConfig } from '../service/types';

/** Operations kept in the activity log. */
export const MAX_ACTIVITY = 200;

export interface ActivityEntry {
  seq: number;
  sessionId?: string;
  operation: Operation;
}

export interface ReadEntry {
  at: string;
  value: ReadValue;
}

export interface ExplorerState {
  health: Health | null;
  link: LinkStatus;
  session: SessionInfo | null;
  connecting: boolean;
  connectError: ServiceError | null;
  /** Children per node reference ('' is the list of logical devices). */
  children: Record<string, ModelNode[]>;
  expanded: Record<string, boolean>;
  selected: ModelNode | null;
  /** Last read per FC-qualified reference. */
  reads: Record<string, ReadEntry>;
  reading: string | null;
  activity: ActivityEntry[];
  error: string | null;
}

const initial: ExplorerState = {
  health: null,
  link: 'up',
  session: null,
  connecting: false,
  connectError: null,
  children: {},
  expanded: {},
  selected: null,
  reads: {},
  reading: null,
  activity: [],
  error: null,
};

type Action =
  | { type: 'health'; health: Health }
  | { type: 'link'; link: LinkStatus }
  | { type: 'connecting' }
  | { type: 'connected'; session: SessionInfo; roots: ModelNode[] }
  | { type: 'connectFailed'; error: ServiceError }
  | { type: 'closed' }
  | { type: 'children'; ref: string; nodes: ModelNode[] }
  | { type: 'toggle'; ref: string; open: boolean }
  | { type: 'select'; node: ModelNode }
  | { type: 'reading'; ref: string | null }
  | { type: 'read'; at: string; values: ReadValue[] }
  | { type: 'event'; event: ExplorerEvent }
  | { type: 'error'; message: string | null };

export function reducer(state: ExplorerState, action: Action): ExplorerState {
  switch (action.type) {
    case 'health':
      return { ...state, health: action.health };
    case 'link':
      return { ...state, link: action.link };
    case 'connecting':
      return { ...state, connecting: true, connectError: null, error: null };
    case 'connected':
      return {
        ...state,
        connecting: false,
        session: action.session,
        children: { '': action.roots },
        expanded: {},
        selected: null,
        reads: {},
      };
    case 'connectFailed':
      return { ...state, connecting: false, connectError: action.error };
    case 'closed':
      return { ...state, session: null, children: {}, expanded: {}, selected: null, reads: {} };
    case 'children':
      return { ...state, children: { ...state.children, [action.ref]: action.nodes } };
    case 'toggle':
      return { ...state, expanded: { ...state.expanded, [action.ref]: action.open } };
    case 'select':
      return { ...state, selected: action.node };
    case 'reading':
      return { ...state, reading: action.ref };
    case 'read': {
      const reads = { ...state.reads };
      for (const value of action.values) {
        reads[value.ref] = { at: action.at, value };
      }
      return { ...state, reads, reading: null };
    }
    case 'event': {
      const { event } = action;
      if (event.kind === 'operation' && event.operation) {
        const entry = { seq: event.seq, sessionId: event.session_id, operation: event.operation };
        return { ...state, activity: [entry, ...state.activity].slice(0, MAX_ACTIVITY) };
      }
      if (event.kind === 'session' && event.session && state.session?.id === event.session.id) {
        return { ...state, session: event.session.state === 'closed' ? state.session : event.session };
      }
      return state;
    }
    case 'error':
      return { ...state, error: action.message, reading: null };
  }
}

/** The Explorer's state and actions, over a Service. */
export function useExplorer(service: Service) {
  const [state, dispatch] = useReducer(reducer, initial);
  const sessionRef = useRef<SessionInfo | null>(null);
  sessionRef.current = state.session;

  useEffect(() => {
    service.health().then(
      (health) => dispatch({ type: 'health', health }),
      () => {},
    );
    // A page that is reloaded adopts the session still open on the engine.
    service.sessions().then(
      async (list) => {
        const open = list.find((s) => s.state === 'connected');
        if (open && sessionRef.current === null) {
          dispatch({ type: 'connected', session: open, roots: await service.browse(open.id, '') });
        }
      },
      () => {},
    );
    return service.subscribe(
      0,
      (event) => dispatch({ type: 'event', event }),
      (link) => dispatch({ type: 'link', link }),
    );
  }, [service]);

  const connect = useCallback(
    async (cfg: TargetConfig) => {
      dispatch({ type: 'connecting' });
      try {
        const session = await service.openSession(cfg);
        dispatch({ type: 'connected', session, roots: await service.browse(session.id, '') });
      } catch (err) {
        dispatch({ type: 'connectFailed', error: toServiceError(err) });
      }
    },
    [service],
  );

  const disconnect = useCallback(async () => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    try {
      await service.closeSession(session.id);
    } catch {
      /* gone already */
    }
    dispatch({ type: 'closed' });
  }, [service]);

  const toggle = useCallback(
    async (node: ModelNode, open: boolean) => {
      const session = sessionRef.current;
      dispatch({ type: 'toggle', ref: node.ref, open });
      if (!open || !session || state.children[node.ref]) {
        return;
      }
      try {
        dispatch({ type: 'children', ref: node.ref, nodes: await service.browse(session.id, node.ref) });
      } catch (err) {
        dispatch({ type: 'error', message: toServiceError(err).message });
      }
    },
    [service, state.children],
  );

  const select = useCallback((node: ModelNode) => dispatch({ type: 'select', node }), []);

  const read = useCallback(
    async (refs: string[]) => {
      const session = sessionRef.current;
      if (!session || refs.length === 0) {
        return;
      }
      dispatch({ type: 'reading', ref: refs[0] ?? null });
      try {
        const result = await service.read(session.id, { refs });
        dispatch({ type: 'read', at: result.at, values: result.values });
      } catch (err) {
        dispatch({ type: 'error', message: toServiceError(err).message });
      }
    },
    [service],
  );

  const clearError = useCallback(() => dispatch({ type: 'error', message: null }), []);

  return { state, connect, disconnect, toggle, select, read, clearError };
}
