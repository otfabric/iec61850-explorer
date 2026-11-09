import { useState } from 'react';
import { ActivityPanel } from './components/ActivityPanel';
import { AuditPanel, CommissionBanner, GrantDialog, ReportsPanel } from './components/Commission';
import { ConnectError } from './components/ConnectError';
import { ConnectionPanel } from './components/ConnectionPanel';
import { DeviceTree } from './components/DeviceTree';
import { FilesPanel } from './components/FilesPanel';
import { Header } from './components/Header';
import { LDPanel } from './components/LDPanel';
import { LinkBanner, NoToken } from './components/LinkStatus';
import { NodePanel } from './components/NodePanel';
import { SCLDialog } from './components/SCLDialog';
import { SearchBox } from './components/SearchBox';
import { SessionBar } from './components/SessionBar';
import { SessionTabs, sessionLabel } from './components/SessionTabs';
import { Button } from './components/ui';
import { WatchTable } from './components/WatchTable';
import { ErrorIcon } from './icons';
import type { Platform } from './platform/platform';
import type { Service } from './service/service';
import type { ExportFormat, FileUpload, SCLInfo, TargetConfig } from './service/types';
import { useExplorer, watchedRefs, watchKey, type SessionView } from './state/useExplorer';
import { useTheme } from './state/useTheme';

interface AppProps {
  service: Service;
  platform: Platform;
}

type BottomTab = 'watch' | 'activity' | 'reports' | 'audit';

export function App({ service, platform }: AppProps) {
  const explorer = useExplorer(service);
  const { state } = explorer;
  const [theme, toggleTheme] = useTheme(platform);
  const [scl, setSCL] = useState<{ file: FileUpload; info: SCLInfo } | null>(null);
  const [initialTarget, setInitialTarget] = useState<TargetConfig | null>(null);
  const [formKey, setFormKey] = useState(0);
  const [bottom, setBottom] = useState<BottomTab>('watch');
  const [filesOpen, setFilesOpen] = useState<Record<string, boolean>>({});
  const [granting, setGranting] = useState<string | null>(null);
  const commission = state.health?.features.includes('commission') ?? false;
  const offline = state.link !== 'up';

  if (state.link === 'unauthorized') {
    return (
      <div className="flex h-full flex-col">
        <Header mode={platform.mode} theme={theme} onToggleTheme={toggleTheme} />
        <NoToken />
      </div>
    );
  }

  const views = state.order.map((id) => state.sessions[id]).filter((v): v is SessionView => v !== undefined);
  const view = state.active ? state.sessions[state.active] : undefined;
  const label = (id: string) => {
    const v = state.sessions[id];
    return v ? sessionLabel(v) : id;
  };
  const polling = views.some((v) => v.info.polling);
  const live = views.some((v) => v.info.state === 'connected' && watchedRefs(state.watch, v.info.id).length > 0);

  async function inspect(file: FileUpload) {
    try {
      setSCL({ file, info: await explorer.inspectSCL(file) });
    } catch (err) {
      setSCL(null);
      explorer.report(err);
    }
  }

  async function exportActive(format: ExportFormat) {
    if (!state.active) {
      return;
    }
    const file = await explorer.exportSession(state.active, format, watchedRefs(state.watch, state.active));
    if (file) {
      await platform.saveFile(file.name, file.mime, file.text);
    }
  }

  async function readAll() {
    for (const v of views) {
      const refs = watchedRefs(state.watch, v.info.id);
      if (refs.length > 0 && (v.info.state === 'connected' || v.info.snapshot)) {
        await explorer.read(v.info.id, refs);
      }
    }
  }

  return (
    <div className="flex h-full flex-col">
      <Header mode={platform.mode} theme={theme} commission={view?.info.mode === 'commission'} onToggleTheme={toggleTheme} />
      <LinkBanner link={state.link} />
      <SessionTabs sessions={views} active={state.active} onActivate={explorer.activate} onClose={(id) => void explorer.close(id)} />
      {state.error && (
        <div className="flex shrink-0 items-center gap-2 border-b border-danger px-4 py-2 text-danger" role="alert">
          <ErrorIcon />
          <span className="flex-1">{state.error}</span>
          <Button onClick={explorer.clearError}>Dismiss</Button>
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-rows-[1fr_minmax(160px,32%)]">
        <div className="min-h-0 overflow-auto">
          {!view && (
            <>
              {state.connectError && <ConnectError error={state.connectError} />}
              <ConnectionPanel
                key={formKey}
                initial={initialTarget}
                profiles={state.profiles}
                profilesAvailable={state.health?.features.includes('profiles') ?? false}
                connecting={state.connecting}
                disabled={offline}
                onConnect={(cfg) => void explorer.connect(cfg)}
                onSaveProfile={explorer.saveProfile}
                onDeleteProfile={(id) => void explorer.deleteProfile(id)}
                onOpenSCL={(file) => void inspect(file)}
                onOpenSnapshot={(file) => void explorer.openSnapshot(file)}
              />
            </>
          )}
          {view && (
            <div className="flex h-full min-h-0 flex-col">
              <SessionBar
                info={view.info}
                filesOpen={filesOpen[view.info.id] === true}
                onToggleFiles={() => setFilesOpen((f) => ({ ...f, [view.info.id]: !f[view.info.id] }))}
                onCommission={commission && !offline ? () => setGranting(view.info.id) : undefined}
                onClose={() => void explorer.close(view.info.id)}
              />
              <CommissionBanner session={view.info} onRevoke={() => void explorer.revokeCommission(view.info.id)} />
              {view.info.state === 'lost' && view.info.error && (
                <div className="flex shrink-0 items-center gap-2 border-b border-wire px-4 py-2 text-wire" role="alert">
                  <ErrorIcon />
                  <span>The association with the IED ended: {view.info.error.message}. The tree stays browsable; reads need a new connection.</span>
                </div>
              )}
              <div className="grid min-h-0 flex-1 grid-cols-[minmax(260px,340px)_1fr]">
                <div className="flex min-h-0 flex-col border-r border-line">
                  <SearchBox
                    key={view.info.id}
                    search={view.search}
                    selected={view.selected}
                    onSearch={(q) => void explorer.search(view.info.id, q)}
                    onPick={(node) => explorer.select(view.info.id, node)}
                  />
                  <div className="min-h-0 flex-1 overflow-auto">
                    <DeviceTree
                      childrenOf={view.children}
                      expanded={view.expanded}
                      selected={view.selected}
                      onToggle={(node, open) => void explorer.toggle(view.info.id, node, open)}
                      onSelect={(node) => explorer.select(view.info.id, node)}
                    />
                  </div>
                </div>
                {filesOpen[view.info.id] && view.info.state === 'connected' ? (
                  <FilesPanel service={service} session={view.info} disabled={offline} />
                ) : view.selected?.kind === 'ld' ? (
                  <LDPanel
                    key={`${view.info.id}|${view.selected.ref}`}
                    service={service}
                    session={view.info}
                    ld={view.selected.ref}
                    disabled={offline || view.info.state === 'lost'}
                    watched={(ref) => state.watch.some((w) => watchKey(w) === watchKey({ sessionId: view.info.id, ref }))}
                    onWatch={(ref, on) => void explorer.watch({ sessionId: view.info.id, ref }, on)}
                    watchedRefs={watchedRefs(state.watch, view.info.id)}
                    onError={explorer.report}
                  />
                ) : view.selected ? (
                  <NodePanel
                    session={view.info}
                    node={view.selected}
                    reads={view.reads}
                    reading={state.reading}
                    disabled={offline || view.info.state === 'lost'}
                    watched={(ref) => state.watch.some((w) => watchKey(w) === watchKey({ sessionId: view.info.id, ref }))}
                    onRead={(refs) => void explorer.read(view.info.id, refs)}
                    onWatch={(ref, on) => void explorer.watch({ sessionId: view.info.id, ref }, on)}
                    onCopy={(text) => void platform.copyText(text)}
                    onWrite={
                      view.info.grant?.capabilities.includes('write') && view.info.state === 'connected'
                        ? (ref, value, reread) => explorer.write(view.info.id, ref, value, reread)
                        : undefined
                    }
                    onControl={
                      view.info.grant?.capabilities.includes('controls') && view.info.state === 'connected'
                        ? (req) => explorer.control(view.info.id, req)
                        : undefined
                    }
                  />
                ) : (
                  <p className="p-4 text-muted">Select a node in the tree, or search.</p>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="flex min-h-0 flex-col border-t border-line">
          <div role="tablist" className="flex shrink-0 gap-4 px-4 pt-2">
            {(commission ? (['watch', 'activity', 'reports', 'audit'] as const) : (['watch', 'activity'] as const)).map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={bottom === tab}
                className={`ot-label cursor-pointer pb-1 ${bottom === tab ? 'border-b-2 border-accent text-accent' : ''}`}
                onClick={() => setBottom(tab)}
              >
                {tab === 'watch'
                  ? `Watch (${state.watch.length})`
                  : tab === 'activity'
                    ? 'Activity & diagnostics'
                    : tab === 'reports'
                      ? `Reports (${state.reports.length})`
                      : `Audit (${state.audit.length})`}
              </button>
            ))}
          </div>
          <div className="min-h-0 flex-1">
            {bottom === 'watch' ? (
              <WatchTable
                entries={state.watch}
                latest={state.latest}
                label={label}
                polling={polling}
                live={live}
                disabled={offline}
                activeSessionId={state.active}
                onPoll={(on, ms) => void explorer.setPolling(on, ms)}
                onReadAll={() => void readAll()}
                onRemove={(entry) => void explorer.watch(entry, false)}
                onExport={(format) => void exportActive(format)}
              />
            ) : bottom === 'activity' ? (
              <ActivityPanel activity={state.activity} />
            ) : bottom === 'reports' ? (
              <ReportsPanel reports={state.reports} label={label} />
            ) : (
              <AuditPanel records={state.audit} label={label} />
            )}
          </div>
        </div>
      </div>

      {granting && state.sessions[granting] && (
        <GrantDialog
          session={state.sessions[granting].info}
          onGrant={(req) => explorer.grantCommission(granting, req)}
          onCancel={() => setGranting(null)}
        />
      )}
      {scl && (
        <SCLDialog
          info={scl.info}
          onCancel={() => setSCL(null)}
          onBrowse={(ied, ap) => {
            const file = scl.file;
            setSCL(null);
            void explorer.openSCL(file, ied, ap);
          }}
          onUseAddress={(target) => {
            setSCL(null);
            setInitialTarget(target);
            setFormKey((k) => k + 1);
          }}
        />
      )}
    </div>
  );
}
