import { useRef, useState, type FormEvent } from 'react';
import { ConnectIcon, FileIcon } from '../icons';
import type { Addressing, FileUpload, Profile, TargetConfig } from '../service/types';
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
  const [profileId, setProfileId] = useState('');
  const [profileName, setProfileName] = useState('');
  const sclInput = useRef<HTMLInputElement>(null);
  const snapshotInput = useRef<HTMLInputElement>(null);

  function target(): TargetConfig {
    const p = Number(port);
    return { host: host.trim(), port: Number.isInteger(p) && p > 0 ? p : undefined, addressing: fromForm(address) };
  }

  function choose(id: string) {
    setProfileId(id);
    const profile = props.profiles.find((p) => p.id === id);
    if (profile) {
      setHost(profile.target.host);
      setPort(String(profile.target.port ?? 102));
      setAddress(toForm(profile.target.addressing));
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

  return (
    <section aria-label="Connect" className="m-4 flex max-w-4xl flex-col gap-4">
      <form className="flex flex-col gap-3 rounded border border-line p-4" onSubmit={submit}>
        <h2 className="font-display text-lg font-semibold text-fg-strong">Connect to an IED</h2>
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
        <div className="flex flex-wrap items-end gap-3">
          <Field label="IED host">
            <TextInput value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.10" className="w-56" aria-label="IED host" />
          </Field>
          <Field label="Port">
            <TextInput value={port} onChange={(e) => setPort(e.target.value)} inputMode="numeric" className="w-20" aria-label="Port" />
          </Field>
          <Button type="submit" variant="primary" icon={<ConnectIcon size={16} />} disabled={props.disabled || props.connecting || host.trim() === ''}>
            {props.connecting ? 'Connecting…' : 'Connect'}
          </Button>
        </div>
        <details>
          <summary className="cursor-pointer text-[13px] text-muted">ISO addressing (selectors, AP-title)</summary>
          <p className="mt-2 text-[12px] text-muted">
            The values of the IED&apos;s SCL Address. Empty: the default shown. A dash (-) leaves the parameter out of the request.
          </p>
          <div className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-4">
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
              <TextInput aria-label="Profile name" value={profileName} onChange={(e) => setProfileName(e.target.value)} className="w-56" />
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
      </form>

      <div className="flex flex-wrap items-center gap-3 rounded border border-line p-4">
        <span className="text-muted">Without an IED:</span>
        <Button icon={<FileIcon size={16} />} onClick={() => sclInput.current?.click()} disabled={props.disabled}>
          Open SCL file…
        </Button>
        <Button icon={<FileIcon size={16} />} onClick={() => snapshotInput.current?.click()} disabled={props.disabled}>
          Open snapshot…
        </Button>
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
      </div>
    </section>
  );
}
