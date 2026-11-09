import { useEffect, useState } from 'react';
import { toServiceError, type Service } from '../service/service';
import type { Semantics } from '../service/types';
import { Button, Chip } from './ui';

/**
 * What the IEC 61850 namespace defines for the selected node (M3c): class,
 * data object, CDC, presence, attribute type, measured quantity; for a
 * logical node, the class's data objects and which the node has.
 */
export function NamespaceView({ service, sessionId, nodeRef }: { service: Service; sessionId: string; nodeRef: string }) {
  const [sem, setSem] = useState<Semantics | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [all, setAll] = useState(false);

  useEffect(() => {
    let live = true;
    setSem(null);
    setError(null);
    setAll(false);
    service.semantics(sessionId, nodeRef).then(
      (s) => live && setSem(s),
      (err) => live && setError(toServiceError(err).message),
    );
    return () => {
      live = false;
    };
  }, [service, sessionId, nodeRef]);

  if (error) {
    return <p className="mt-3 text-muted">Namespace: {error}</p>;
  }
  if (!sem) {
    return null;
  }
  const rows: [string, string | undefined][] = [
    ['Class', sem.ln_class && `${sem.ln_class}${sem.base ? ` (from ${sem.base})` : ''}${sem.deprecated ? ', deprecated' : ''}`],
    ['Namespace', sem.namespace],
    ['Data object', sem.data_object && `${sem.data_object}${sem.declared_in && sem.declared_in !== sem.ln_class ? ` (declared in ${sem.declared_in})` : ''}`],
    ['CDC', sem.cdc],
    ['Presence', sem.presence],
    ['Attribute', sem.attribute && `${sem.attribute}: ${sem.type ?? ''}${sem.fc ? ` [${sem.fc}]` : ''}`],
    ['Quantity', sem.quantity && [sem.quantity, sem.phase, sem.unit].filter(Boolean).join(' · ')],
  ];
  const dos = sem.data_objects ?? [];
  const shown = all ? dos : dos.filter((d) => d.present > 0 || d.mandatory);
  return (
    <section aria-label="Namespace" className="mt-4">
      <h3 className="ot-label">Namespace</h3>
      {sem.note && <p className="mt-1 text-muted">{sem.note}</p>}
      <dl className="mt-1 grid grid-cols-[8rem_1fr] gap-x-3 gap-y-0.5 text-[13px]">
        {rows
          .filter(([, v]) => v)
          .map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd className="font-mono">
                {v}
                {k === 'CDC' && sem.instance_cdc && sem.instance_cdc !== sem.cdc && (
                  <span className="ml-2">
                    <Chip tone="wire">SCL says {sem.instance_cdc}</Chip>
                  </span>
                )}
              </dd>
            </div>
          ))}
      </dl>
      {dos.length > 0 && (
        <>
          <table className="mt-2 w-full text-[13px]" aria-label="Data objects of the class">
            <thead>
              <tr className="ot-label text-left">
                <th className="py-1 pr-2">Data object</th>
                <th className="py-1 pr-2">CDC</th>
                <th className="py-1 pr-2">Presence</th>
                <th className="py-1 pr-2">In this node</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((d) => (
                <tr key={d.name} className="border-t border-line">
                  <td className="py-0.5 pr-2 font-mono">{d.name}</td>
                  <td className="py-0.5 pr-2 font-mono">{d.cdc}</td>
                  <td className="py-0.5 pr-2 font-mono">{d.presence}</td>
                  <td className="py-0.5 pr-2">
                    {d.present > 0 ? (
                      <Chip tone="accent">{d.present > 1 ? `${d.present} instances` : 'yes'}</Chip>
                    ) : d.mandatory ? (
                      <Chip tone="wire">missing</Chip>
                    ) : (
                      <span className="text-muted">no</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {shown.length < dos.length && (
            <Button className="mt-1" onClick={() => setAll(true)}>
              Show all {dos.length} data objects of {sem.ln_class}
            </Button>
          )}
        </>
      )}
    </section>
  );
}
