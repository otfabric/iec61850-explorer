const KEY = 'iec61850-explorer.dock';

// A per-viewer convenience: without storage the dock simply starts open
// every time.
export function persistDockCollapsed(collapsed: boolean): void {
  try {
    localStorage.setItem(KEY, collapsed ? 'collapsed' : 'open');
  } catch {
    /* not remembered */
  }
}

export function loadDockCollapsed(): boolean {
  try {
    return localStorage.getItem(KEY) === 'collapsed';
  } catch {
    return false;
  }
}
