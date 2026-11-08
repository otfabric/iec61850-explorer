import { ErrorIcon } from '../icons';
import type { LinkStatus } from '../service/service';

/** Shown instead of the workspace when the engine refuses this page. */
export function NoToken() {
  return (
    <section className="m-4 max-w-2xl rounded border border-danger p-4" role="alert">
      <h2 className="flex items-center gap-2 font-display text-lg font-semibold text-danger">
        <ErrorIcon />
        No valid token
      </h2>
      <p className="mt-2">
        This page has no access token, or the daemon rejected it. The token is kept in memory only, so it is gone after a reload, and a restarted
        daemon issues a new one.
      </p>
      <p className="mt-2">
        Open the URL printed by <code className="font-mono text-fg-strong">iec61850-explorerd url</code> on the machine that runs the daemon.
      </p>
    </section>
  );
}

export function LinkBanner({ link }: { link: LinkStatus }) {
  if (link !== 'down') {
    return null;
  }
  return (
    <div className="flex shrink-0 items-center gap-2 border-b border-danger px-4 py-2 text-danger" role="alert">
      <ErrorIcon />
      <strong className="font-semibold">Connection to the daemon lost — retrying.</strong>
      <span>What is on screen is what was last received; the daemon keeps its sessions.</span>
    </div>
  );
}
