import logoDark from '../../assets/logo-dark.svg';
import logoLight from '../../assets/logo-light.svg';
import iecIcon from '../../assets/iec61850.svg';
import { BrowserIcon, LocalIcon, MoonIcon, SunIcon } from '../icons';
import type { Mode, Theme } from '../platform/platform';
import { IconButton } from './ui';

function ModeBadge({ mode }: { mode: Mode }) {
  return (
    <span className="ot-label inline-flex items-center gap-1.5 rounded border border-line px-2 py-1.5" title="How this window reaches the engine">
      {mode.kind === 'local' ? <LocalIcon size={14} /> : <BrowserIcon size={14} />}
      {mode.kind}
    </span>
  );
}

/**
 * The mode of the open session. Observe: the engine sends no MMS service that
 * changes the IED. Commission: the session holds a grant.
 */
function SessionModeBadge({ commission }: { commission: boolean }) {
  if (commission) {
    return (
      <span
        className="ot-label inline-flex items-center rounded border border-wire px-2 py-1.5 text-wire-text"
        title="Commission mode: this session may change the IED through a granted, audited second association."
      >
        commission
      </span>
    );
  }
  return (
    <span
      className="ot-label inline-flex items-center rounded border border-accent bg-accent-soft px-2 py-1.5 text-accent-strong"
      title="Observe mode: discovery and reads only. The MMS client refuses every state-changing service before it is sent."
    >
      observe
    </span>
  );
}

interface HeaderProps {
  mode: Mode;
  theme: Theme;
  /** The open session holds a Commission grant. */
  commission?: boolean;
  onToggleTheme: () => void;
}

export function Header({ mode, theme, commission = false, onToggleTheme }: HeaderProps) {
  return (
    <header className="flex h-14 shrink-0 items-center gap-4 border-b border-line bg-header px-4">
      <img src={iecIcon} alt="" width={32} height={32} />
      <span className="font-display text-[1.45rem] font-semibold tracking-[0.01em] text-fg-strong">IEC 61850 Explorer</span>
      <span className="flex items-center gap-1.5 text-[12px] text-muted">
        powered by
        <img src={theme === 'dark' ? logoDark : logoLight} alt="" width={16} height={16} />
        <span className="font-display text-[14px] font-medium tracking-[0.01em] text-fg">
          <b className="font-bold">OT</b> Fabric
        </span>
      </span>
      <span className="ml-auto flex items-center gap-3">
        <SessionModeBadge commission={commission} />
        <ModeBadge mode={mode} />
        <IconButton label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'} onClick={onToggleTheme}>
          {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
        </IconButton>
      </span>
    </header>
  );
}
