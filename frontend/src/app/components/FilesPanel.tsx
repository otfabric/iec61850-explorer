import { useEffect, useState } from 'react';
import { ErrorIcon, ExportIcon, RefreshIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { FileEntry, SessionInfo } from '../service/types';
import { Button } from './ui';

function size(n: number): string {
  if (n < 1024) {
    return `${n} B`;
  }
  if (n < 1024 * 1024) {
    return `${(n / 1024).toFixed(1)} KiB`;
  }
  return `${(n / 1024 / 1024).toFixed(1)} MiB`;
}

/** The IED's file store: list and download (disturbance records, configuration). */
export function FilesPanel({ service, session, disabled }: { service: Service; session: SessionInfo; disabled: boolean }) {
  const [files, setFiles] = useState<FileEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  async function load() {
    try {
      setError(null);
      setFiles(await service.files(session.id, ''));
    } catch (err) {
      setError(toServiceError(err).message);
    }
  }

  useEffect(() => {
    void load();
  }, [session.id]);

  async function download(name: string) {
    try {
      setBusy(name);
      await service.saveDeviceFile(session.id, name);
    } catch (err) {
      setError(toServiceError(err).message);
    } finally {
      setBusy(null);
    }
  }

  return (
    <section aria-label="Files" className="min-h-0 overflow-auto p-4">
      <div className="flex items-center gap-2">
        <h2 className="font-display text-lg font-semibold text-fg-strong">Files of the IED</h2>
        <Button className="ml-auto" icon={<RefreshIcon size={14} />} onClick={() => void load()} disabled={disabled}>
          Refresh
        </Button>
      </div>
      {error && (
        <p className="mt-2 flex items-center gap-2 text-danger" role="alert">
          <ErrorIcon size={16} />
          {error}
        </p>
      )}
      {files && files.length === 0 && <p className="mt-3 text-muted">The IED lists no files.</p>}
      {files && files.length > 0 && (
        <table className="mt-3 w-full text-[13px]">
          <thead>
            <tr className="ot-label text-left">
              <th className="py-1 pr-2">Name</th>
              <th className="py-1 pr-2">Size</th>
              <th className="py-1 pr-2">Modified</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {files.map((f) => (
              <tr key={f.name} className="border-t border-line">
                <td className="py-1 pr-2 font-mono">{f.name}</td>
                <td className="py-1 pr-2 text-muted">{size(f.size)}</td>
                <td className="py-1 pr-2 text-muted">{f.last_modified ? new Date(f.last_modified).toLocaleString() : ''}</td>
                <td className="py-1 text-right">
                  <Button icon={<ExportIcon size={14} />} disabled={disabled || busy !== null} onClick={() => void download(f.name)} aria-label={`Download ${f.name}`}>
                    {busy === f.name ? 'Downloading…' : 'Download'}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
