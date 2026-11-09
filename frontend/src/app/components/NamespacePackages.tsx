import { useEffect, useRef, useState } from 'react';
import { CloseIcon, ErrorIcon, FileIcon } from '../icons';
import { toServiceError, type Service } from '../service/service';
import type { Namespaces } from '../service/types';
import { readUpload } from './files';
import { Button, Chip } from './ui';

/**
 * The namespace definitions models are checked against: the releases built
 * into the Explorer, and the packages the user loads (official releases not
 * embedded, vendor extension namespaces of ABB, Siemens, Schneider, ...).
 */
export function NamespacePackages({ service, onChange }: { service: Service; onChange: () => void }) {
  const [list, setList] = useState<Namespaces | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      setList(await service.namespaces());
    } catch (err) {
      setError(toServiceError(err).message);
    }
  }

  useEffect(() => {
    void load();
  }, [service]);

  async function act(f: () => Promise<unknown>) {
    try {
      setBusy(true);
      setError(null);
      await f();
      await load();
      onChange();
    } catch (err) {
      setError(toServiceError(err).message);
    } finally {
      setBusy(false);
    }
  }

  async function add() {
    const file = input.current?.files?.[0];
    if (file) {
      const upload = await readUpload(file);
      input.current!.value = '';
      await act(() => service.addNamespaces(upload));
    }
  }

  return (
    <section aria-label="Namespace definitions" className="mt-4 rounded border border-line p-3">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="ot-label">Namespace definitions</h3>
        <Button className="ml-auto" icon={<FileIcon size={14} />} disabled={busy} onClick={() => input.current?.click()}>
          Load NSD package…
        </Button>
        <input ref={input} type="file" accept=".zip,.nsd,.xml" aria-label="NSD package" className="hidden" onChange={() => void add()} />
      </div>
      <p className="mt-1 text-muted">
        {list ? `${list.embedded.length} releases built in. ` : ''}
        Load official releases that are not built in, or a vendor's extension namespaces: logical nodes and data objects declared in them are
        then checked against their definitions.
      </p>
      {error && (
        <p className="mt-2 flex items-center gap-2 text-danger" role="alert">
          <ErrorIcon size={16} />
          {error}
        </p>
      )}
      {list && list.loaded.length > 0 && (
        <ul className="mt-2 flex flex-col gap-1" aria-label="Loaded packages">
          {list.loaded.map((p) => (
            <li key={p.id} className="flex flex-wrap items-center gap-2 text-[13px]">
              <span className="font-mono">{p.file}</span>
              {p.namespaces.map((n) => (
                <Chip key={n} tone="accent">
                  {n}
                </Chip>
              ))}
              {p.docs && <Chip>docs</Chip>}
              <Button className="ml-auto" icon={<CloseIcon size={14} />} disabled={busy} aria-label={`Remove ${p.file}`} onClick={() => void act(() => service.removeNamespaces(p.id))}>
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
