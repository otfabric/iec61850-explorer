import type { SCLInfo, TargetConfig } from '../service/types';
import { Button, Chip } from './ui';

interface SCLDialogProps {
  info: SCLInfo;
  onBrowse: (ied: string, accessPoint: string) => void;
  onUseAddress: (target: TargetConfig) => void;
  onCancel: () => void;
}

/** The IEDs of an SCL file: browse one offline, or connect to its address. */
export function SCLDialog({ info, onBrowse, onUseAddress, onCancel }: SCLDialogProps) {
  const errors = info.diagnostics.filter((d) => d.severity === 'error').length;
  const warnings = info.diagnostics.filter((d) => d.severity === 'warning').length;
  return (
    <div className="fixed inset-0 z-10 flex items-start justify-center overflow-auto bg-bg/80 p-8">
      <section role="dialog" aria-modal="true" aria-label="SCL file" className="w-full max-w-3xl rounded border border-line bg-surface p-4">
        <h2 className="font-display text-lg font-semibold text-fg-strong">{info.name}</h2>
        <p className="mt-1 text-muted">
          {info.kind.toUpperCase()} · SCL {info.version || 'version unknown'} · {info.ieds.length} IED{info.ieds.length === 1 ? '' : 's'}
          {errors + warnings > 0 && ` · ${errors} errors, ${warnings} warnings`}
        </p>
        <ul className="mt-3 flex flex-col gap-2">
          {info.ieds.map((ied) =>
            ied.access_points.map((ap) => (
              <li key={`${ied.name}/${ap.name}`} className="flex flex-wrap items-center gap-2 rounded border border-line p-2">
                <span className="font-mono text-fg-strong">{ied.name}</span>
                <Chip>{ap.name}</Chip>
                <span className="text-[12px] text-muted">
                  {[ied.manufacturer, ied.type].filter(Boolean).join(' · ')} · {ap.logical_devices} LD
                  {ap.target && ` · ${ap.target.host}`}
                </span>
                <span className="ml-auto flex gap-2">
                  {ap.target && (
                    <Button onClick={() => ap.target && onUseAddress(ap.target)} title="Fill in the connection with the address of this access point">
                      Use address
                    </Button>
                  )}
                  <Button variant="primary" onClick={() => onBrowse(ied.name, ap.name)}>
                    Browse offline
                  </Button>
                </span>
              </li>
            )),
          )}
          {info.ieds.every((ied) => ied.access_points.length === 0) && <li className="text-muted">No IED with an MMS server in this file.</li>}
        </ul>
        {info.diagnostics.length > 0 && (
          <details className="mt-3">
            <summary className="cursor-pointer text-[13px] text-muted">Parser findings ({info.diagnostics.length + (info.diagnostics_dropped ?? 0)})</summary>
            <ul className="mt-2 max-h-48 overflow-auto font-mono text-[12px]">
              {info.diagnostics.map((d, i) => (
                <li key={i} className={d.severity === 'error' ? 'text-danger' : d.severity === 'warning' ? 'text-wire' : 'text-muted'}>
                  {d.severity}: {d.message}
                  {d.path && <span className="text-muted"> ({d.path})</span>}
                </li>
              ))}
            </ul>
          </details>
        )}
        <div className="mt-4 flex justify-end">
          <Button onClick={onCancel}>Cancel</Button>
        </div>
      </section>
    </div>
  );
}
