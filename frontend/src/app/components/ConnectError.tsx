import { ErrorIcon } from '../icons';
import type { ServiceError } from '../service/service';

const STAGES: Record<string, string> = {
  tcp: 'TCP connection',
  cotp: 'Transport connection (COTP)',
  session: 'Session layer',
  presentation: 'Presentation layer',
  acse: 'Association (ACSE)',
  'mms-initiate': 'MMS Initiate',
};

/** A failed connect: where it stopped, why, and what to check. */
export function ConnectError({ error }: { error: ServiceError }) {
  const detail = error.detail;
  return (
    <section className="m-4 max-w-3xl rounded border border-danger p-4" role="alert">
      <h2 className="flex items-center gap-2 font-display text-lg font-semibold text-danger">
        <ErrorIcon />
        {error.code === 'discovery_failed' ? 'Connected, but the model could not be listed' : 'Could not connect'}
      </h2>
      <p className="mt-2 text-fg">{error.message}</p>
      {detail?.stage && (
        <p className="mt-2">
          <span className="ot-label">Stopped at</span> <span className="font-mono text-fg-strong">{STAGES[detail.stage] ?? detail.stage}</span>
        </p>
      )}
      {detail?.hint && <p className="mt-2 text-muted">{detail.hint}</p>}
    </section>
  );
}
