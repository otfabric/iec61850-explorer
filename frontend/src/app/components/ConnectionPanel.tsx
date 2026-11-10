import { useRef, useState, type FormEvent, type ReactNode } from 'react';
import { ConnectIcon, FileIcon } from '../icons';
import { PROXY_DIRECT, type Addressing, type FileUpload, type Profile, type Settings, type TargetConfig } from '../service/types';
import { readUpload } from './files';
import { Button, Field, TextInput } from './ui';

interface ConnectionPanelProps {
  /** A target to start from, for example the address an SCL file gives. */
  initial: TargetConfig | null;
  profiles: Profile[];
  profilesAvailable: boolean;
  connecting: boolean;
  disabled: boolean;
  onConnect: (cfg: TargetConfig) => void;
  onSaveProfile: (profile: Profile) => Promise<Profile | null>;
  onDeleteProfile: (id: string) => void;
  onOpenSCL: (file: FileUpload) => void;
  onOpenSnapshot: (file: FileUpload) => void;
  /** Opens a capture as a trace session; ports are the MMS server ports. */
  onOpenTrace?: (file: FileUpload, ports: number[]) => void;
  /** The engine's settings; null while unknown. */
  settings: Settings | null;
  /** Saves the default proxy; resolves to whether it was saved. */
  onSaveSettings: (settings: Settings) => Promise<boolean>;
}

/** The MMS ports of a capture: comma-separated numbers; empty is the default (102). Null when invalid. */
export function parsePorts(text: string): number[] | null {
  const parts = text
    .split(',')
    .map((p) => p.trim())
    .filter((p) => p !== '');
  const ports = parts.map(Number);
  return ports.every((p) => Number.isInteger(p) && p >= 1 && p <= 65535) ? ports : null;
}

/** What an empty Proxy field means, for its placeholder. */
export function proxyPlaceholder(settings: Settings | null): string {
  const def = settings?.default_proxy ?? '';
  if (def === '' || def === PROXY_DIRECT) {
    return 'direct (no default proxy)';
  }
  return `default: ${def}`;
}

/** The default proxy: editable, or read-only when the daemon's operator set it. */
function DefaultProxy({ settings, onSave }: { settings: Settings; onSave: (s: Settings) => Promise<boolean> }) {
  const [value, setValue] = useState(settings.default_proxy);
  const [saved, setSaved] = useState(false);
  if (settings.default_proxy_locked) {
    return (
      <div className="flex flex-wrap items-center gap-3">
        <span className="ot-label">Default proxy</span>
        <span className="font-mono text-fg-strong" aria-label="Default proxy">
          {settings.default_proxy}
        </span>
        <span className="text-[12px] text-muted">Set by the daemon&apos;s operator (--proxy); it cannot be changed here.</span>
      </div>
    );
  }
  return (
    <form
      className="flex flex-wrap items-end gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        void onSave({ default_proxy: value.trim(), default_proxy_locked: false }).then(setSaved);
      }}
    >
      <Field label="Default proxy">
        <TextInput
          aria-label="Default proxy"
          value={value}
          placeholder="none, or socks5://host:1080"
          className="w-72"
          onChange={(e) => {
            setValue(e.target.value);
            setSaved(false);
          }}
        />
      </Field>
      <Button type="submit" disabled={value.trim() === settings.default_proxy}>
        Save default
      </Button>
      {saved && <span className="text-[12px] text-accent">Saved</span>}
      <p className="w-full text-[12px] text-muted">
        For every IED whose Proxy field is empty. socks5:// resolves the IED&apos;s name here, socks5h:// and http:// (CONNECT) let the proxy resolve
        it. A user:password in the URL is stored in the settings file of the data directory.
      </p>
    </form>
  );
}

/** One way into the Explorer on the start view: a title, what it offers, its controls. */
function Card({ title, hint, children }: { title: string; hint: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-3 rounded border border-line p-4">
      <div>
        <h2 className="font-display text-lg font-semibold text-fg-strong">{title}</h2>
        <p className="mt-1 text-[12px] text-muted">{hint}</p>
      </div>
      {children}
    </section>
  );
}

const ADDRESS_FIELDS: { key: keyof Addressing; label: string; placeholder: string; numeric?: boolean }[] = [
  { key: 'remote_tsel', label: 'Remote T-selector', placeholder: '0001' },
  { key: 'remote_ssel', label: 'Remote S-selector', placeholder: '0001' },
  { key: 'remote_psel', label: 'Remote P-selector', placeholder: '00000001' },
  { key: 'remote_ap_title', label: 'Remote AP-title', placeholder: '1.1.1.999.1' },
  { key: 'remote_ae_qualifier', label: 'Remote AE-qualifier', placeholder: '12', numeric: true },
  { key: 'local_tsel', label: 'Local T-selector', placeholder: '0001' },
  { key: 'local_ap_title', label: 'Local AP-title', placeholder: '1.1.1.999' },
  { key: 'local_ae_qualifier', label: 'Local AE-qualifier', placeholder: '12', numeric: true },
];

type AddressForm = Partial<Record<keyof Addressing, string>>;

function toForm(a?: Addressing): AddressForm {
  const out: AddressForm = {};
  for (const { key } of ADDRESS_FIELDS) {
    const v = a?.[key];
    if (v !== undefined) {
      out[key] = String(v);
    }
  }
  return out;
}

/** Fields left empty take the default; "-" omits the parameter. */
function fromForm(form: AddressForm): Addressing | undefined {
  const out: Record<string, string | number> = {};
  for (const { key, numeric } of ADDRESS_FIELDS) {
    const v = form[key]?.trim();
    if (v === undefined || v === '') {
      continue;
    }
    out[key] = numeric ? Number(v) : v === '-' ? '' : v;
  }
  return Object.keys(out).length > 0 ? (out as Addressing) : undefined;
}

export function ConnectionPanel(props: ConnectionPanelProps) {
  const [host, setHost] = useState(props.initial?.host ?? '');
  const [port, setPort] = useState(String(props.initial?.port ?? 102));
  const [address, setAddress] = useState<AddressForm>(toForm(props.initial?.addressing));
  const [proxy, setProxy] = useState(props.initial?.proxy ?? '');
  const [profileId, setProfileId] = useState('');
  const [profileName, setProfileName] = useState('');
  const sclInput = useRef<HTMLInputElement>(null);
  const snapshotInput = useRef<HTMLInputElement>(null);
  const traceInput = useRef<HTMLInputElement>(null);
  const [tracePorts, setTracePorts] = useState('102');
  const ports = parsePorts(tracePorts);

  function target(): TargetConfig {
    const p = Number(port);
    const via = proxy.trim();
    return {
      host: host.trim(),
      port: Number.isInteger(p) && p > 0 ? p : undefined,
      addressing: fromForm(address),
      ...(via !== '' ? { proxy: via } : {}),
    };
  }

  function choose(id: string) {
    setProfileId(id);
    const profile = props.profiles.find((p) => p.id === id);
    if (profile) {
      setHost(profile.target.host);
      setPort(String(profile.target.port ?? 102));
      setAddress(toForm(profile.target.addressing));
      setProxy(profile.target.proxy ?? '');
      setProfileName(profile.name);
    }
  }

  async function save() {
    const saved = await props.onSaveProfile({ id: profileId, name: profileName.trim() || host.trim(), target: target() });
    if (saved) {
      setProfileId(saved.id);
      setProfileName(saved.name);
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    props.onConnect(target());
  }

  async function pick(input: HTMLInputElement | null, then: (f: FileUpload) => void) {
    const file = input?.files?.[0];
    if (file) {
      then(await readUpload(file));
      input.value = '';
    }
  }

  const via = proxy.trim();
  const viaNote = via !== '' ? `proxy ${via}` : props.settings?.default_proxy ? `default proxy ${props.settings.default_proxy}` : '';

  return (
    <section aria-label="Connect" className="mx-auto flex w-full max-w-7xl flex-col gap-4 p-4">
      <p className="text-[13px] text-muted">
        Inspect IEDs live, read-only unless you grant Commission mode; open SCL files and snapshots without an IED; analyse captures of MMS, GOOSE and
        Sampled Values offline.
      </p>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <Card
          title="Connect to an IED"
          hint="Browse, search, read and poll; data sets, reports, logs, setting groups and files; compare with SCL and check conformance."
        >
          <form className="flex flex-col gap-3" onSubmit={submit}>
            {props.profilesAvailable && (
              <Field label="Profile">
                <select
                  aria-label="Profile"
                  className="h-8 rounded border border-control bg-bg px-2 text-[13px] text-fg-strong"
                  value={profileId}
                  onChange={(e) => choose(e.target.value)}
                >
                  <option value="">New connection</option>
                  {props.profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.target.host})
                    </option>
                  ))}
                </select>
              </Field>
            )}
            <div className="flex items-end gap-3">
              <div className="min-w-0 flex-1">
                <Field label="IED host">
                  <TextInput value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.10" className="w-full" aria-label="IED host" />
                </Field>
              </div>
              <Field label="Port">
                <TextInput value={port} onChange={(e) => setPort(e.target.value)} inputMode="numeric" className="w-20" aria-label="Port" />
              </Field>
              <Button type="submit" variant="primary" icon={<ConnectIcon size={16} />} disabled={props.disabled || props.connecting || host.trim() === ''}>
                {props.connecting ? 'Connecting…' : 'Connect'}
              </Button>
            </div>
            <details>
              <summary className="cursor-pointer text-[13px] text-muted">
                Connection options: proxy, selectors, profile
                {viaNote !== '' && <span className="ml-2 font-mono text-[12px] text-fg">({viaNote})</span>}
              </summary>
              <div className="mt-3 flex flex-col gap-3">
                <Field label="Proxy">
                  <TextInput
                    aria-label="Proxy"
                    value={proxy}
                    onChange={(e) => setProxy(e.target.value)}
                    placeholder={proxyPlaceholder(props.settings)}
                    className="w-full"
                    list="proxy-options"
                  />
                </Field>
                <datalist id="proxy-options">
                  <option value={PROXY_DIRECT}>No proxy, even when a default is set</option>
                </datalist>
                <span className="text-[12px] text-muted">Empty: the default proxy. direct: none. socks5://, socks5h:// or http://[user:password@]host:port.</span>
                <details>
                  <summary className="cursor-pointer text-[13px] text-muted">ISO addressing (selectors, AP-title)</summary>
                  <p className="mt-2 text-[12px] text-muted">
                    The values of the IED&apos;s SCL Address. Empty: the default shown. A dash (-) leaves the parameter out of the request.
                  </p>
                  <div className="mt-2 grid grid-cols-2 gap-3">
                    {ADDRESS_FIELDS.map(({ key, label, placeholder }) => (
                      <Field key={key} label={label}>
                        <TextInput
                          aria-label={label}
                          value={address[key] ?? ''}
                          placeholder={placeholder}
                          onChange={(e) => setAddress({ ...address, [key]: e.target.value })}
                        />
                      </Field>
                    ))}
                  </div>
                </details>
                {props.profilesAvailable && (
                  <div className="flex flex-wrap items-end gap-3 border-t border-line pt-3">
                    <Field label="Profile name">
                      <TextInput aria-label="Profile name" value={profileName} onChange={(e) => setProfileName(e.target.value)} className="w-48" />
                    </Field>
                    <Button onClick={() => void save()} disabled={host.trim() === ''}>
                      {profileId ? 'Update profile' : 'Save as profile'}
                    </Button>
                    {profileId && (
                      <Button
                        onClick={() => {
                          props.onDeleteProfile(profileId);
                          setProfileId('');
                        }}
                      >
                        Delete profile
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </details>
          </form>
          {props.settings && (
            <details>
              <summary className="cursor-pointer text-[13px] text-muted">Default proxy for every IED</summary>
              <div className="mt-3">
                <DefaultProxy settings={props.settings} onSave={props.onSaveSettings} />
              </div>
            </details>
          )}
        </Card>

        <Card
          title="Open without an IED"
          hint="An SCL file (SCD, CID, ICD, IID, SSD) or a saved snapshot: browse its model, compare it with an IED or another file, check conformance, export point lists."
        >
          <div className="flex flex-wrap gap-3">
            <Button icon={<FileIcon size={16} />} onClick={() => sclInput.current?.click()} disabled={props.disabled}>
              Open SCL file…
            </Button>
            <Button icon={<FileIcon size={16} />} onClick={() => snapshotInput.current?.click()} disabled={props.disabled}>
              Open snapshot…
            </Button>
          </div>
          <input
            ref={sclInput}
            type="file"
            accept=".scd,.cid,.icd,.iid,.ssd,.xml"
            aria-label="SCL file"
            className="hidden"
            onChange={() => void pick(sclInput.current, props.onOpenSCL)}
          />
          <input
            ref={snapshotInput}
            type="file"
            accept=".json"
            aria-label="Snapshot file"
            className="hidden"
            onChange={() => void pick(snapshotInput.current, props.onOpenSnapshot)}
          />
        </Card>

        {props.onOpenTrace && (
          <Card
            title="Analyse a capture"
            hint="A pcap or pcapng file, analysed offline: MMS associations and a timeline of services, reports, GOOSE and Sampled Values streams, and the gaps found. Link it to a model to name what it carries."
          >
            <div className="flex flex-wrap items-end gap-3">
              <Field label="MMS ports">
                <TextInput
                  aria-label="MMS ports"
                  value={tracePorts}
                  onChange={(e) => setTracePorts(e.target.value)}
                  placeholder="102"
                  className="w-32"
                  aria-invalid={ports === null}
                />
              </Field>
              <Button icon={<FileIcon size={16} />} onClick={() => traceInput.current?.click()} disabled={props.disabled || ports === null}>
                Open capture…
              </Button>
            </div>
            {ports === null && <span className="text-[12px] text-danger">Ports are numbers from 1 to 65535, separated by commas.</span>}
            <input
              ref={traceInput}
              type="file"
              accept=".pcap,.pcapng,.cap,.gz"
              aria-label="Capture file"
              className="hidden"
              onChange={() => void pick(traceInput.current, (f) => props.onOpenTrace?.(f, ports ?? []))}
            />
          </Card>
        )}
      </div>
    </section>
  );
}
