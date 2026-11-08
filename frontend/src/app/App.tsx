import { ActivityPanel } from './components/ActivityPanel';
import { ConnectError } from './components/ConnectError';
import { ConnectionBar } from './components/ConnectionBar';
import { DeviceTree } from './components/DeviceTree';
import { Header } from './components/Header';
import { LinkBanner, NoToken } from './components/LinkStatus';
import { NodePanel } from './components/NodePanel';
import { Button } from './components/ui';
import { ErrorIcon } from './icons';
import type { Platform } from './platform/platform';
import type { Service } from './service/service';
import { useExplorer } from './state/useExplorer';
import { useTheme } from './state/useTheme';

interface AppProps {
  service: Service;
  platform: Platform;
}

export function App({ service, platform }: AppProps) {
  const explorer = useExplorer(service);
  const { state } = explorer;
  const [theme, toggleTheme] = useTheme(platform);
  const offline = state.link !== 'up';
  const lost = state.session?.state === 'lost';

  if (state.link === 'unauthorized') {
    return (
      <div className="flex h-full flex-col">
        <Header mode={platform.mode} theme={theme} onToggleTheme={toggleTheme} />
        <NoToken />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <Header mode={platform.mode} theme={theme} onToggleTheme={toggleTheme} />
      <LinkBanner link={state.link} />
      <ConnectionBar
        session={state.session}
        connecting={state.connecting}
        disabled={offline}
        onConnect={(cfg) => void explorer.connect(cfg)}
        onDisconnect={() => void explorer.disconnect()}
      />
      {lost && state.session?.error && (
        <div className="flex shrink-0 items-center gap-2 border-b border-wire px-4 py-2 text-wire" role="alert">
          <ErrorIcon />
          <span>The association with the IED ended: {state.session.error.message}. The tree stays browsable; reads need a new connection.</span>
        </div>
      )}
      {state.error && (
        <div className="flex shrink-0 items-center gap-2 border-b border-danger px-4 py-2 text-danger" role="alert">
          <ErrorIcon />
          <span className="flex-1">{state.error}</span>
          <Button onClick={explorer.clearError}>Dismiss</Button>
        </div>
      )}
      {!state.session && state.connectError && <ConnectError error={state.connectError} />}
      {!state.session && !state.connectError && (
        <p className="m-4 max-w-2xl text-muted">
          Connect to an IED to discover its IEC 61850 model. The Explorer is in Observe mode: it sends discovery and read services only.
        </p>
      )}
      {state.session && (
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(240px,320px)_1fr] grid-rows-[1fr_minmax(140px,30%)]">
          <div className="row-span-1 min-h-0 border-r border-line">
            <DeviceTree
              childrenOf={state.children}
              expanded={state.expanded}
              selected={state.selected}
              onToggle={(node, open) => void explorer.toggle(node, open)}
              onSelect={explorer.select}
            />
          </div>
          {state.selected ? (
            <NodePanel
              session={state.session}
              node={state.selected}
              reads={state.reads}
              reading={state.reading}
              disabled={offline || lost}
              onRead={(refs) => void explorer.read(refs)}
              onCopy={(text) => void platform.copyText(text)}
            />
          ) : (
            <p className="p-4 text-muted">Select a node in the tree.</p>
          )}
          <div className="col-span-2 min-h-0">
            <ActivityPanel activity={state.activity} />
          </div>
        </div>
      )}
    </div>
  );
}
