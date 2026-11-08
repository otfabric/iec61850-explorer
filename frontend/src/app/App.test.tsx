import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { cliCommand } from './components/NodePanel';
import { summarize } from './components/summary';
import type { Platform } from './platform/platform';
import { MOCK_SCL, MockService } from './service/mock';
import type { SessionInfo } from './service/types';

function platform() {
  const host = {
    mode: { kind: 'browser' } as const,
    saveFile: vi.fn(async (_name: string, _mime: string, _text: string) => true),
    openExternal: vi.fn(),
    persistTheme: vi.fn(),
    loadTheme: () => 'dark' as const,
    copyText: vi.fn(async (_text: string) => {}),
  };
  return host satisfies Platform;
}

async function connect(user: ReturnType<typeof userEvent.setup>, host = '10.0.0.5') {
  await user.type(screen.getByLabelText('IED host'), host);
  await user.click(screen.getByRole('button', { name: 'Connect' }));
}

async function openHz(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Expand DemoIEDLD0' }));
  await user.click(await screen.findByRole('button', { name: 'Expand MMXU1' }));
  await user.click(await screen.findByText('Hz'));
  return screen.getByRole('region', { name: 'Selected node' });
}

describe('App', () => {
  it('says it is in Observe mode before anything is connected', () => {
    render(<App service={new MockService()} platform={platform()} />);
    expect(screen.getByTitle(/Observe mode/)).toHaveTextContent('observe');
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled();
  });

  it('shows where a failed connect stopped and what to check', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await connect(user, 'unreachable');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Could not connect');
    expect(alert).toHaveTextContent('TCP connection');
    expect(alert).toHaveTextContent('Check the address');
  });

  it('connects with ISO addressing, browses, reads, and logs the requests', async () => {
    const user = userEvent.setup();
    const host = platform();
    const service = new MockService();
    const open = vi.spyOn(service, 'openSession');
    render(<App service={service} platform={host} />);
    await user.click(screen.getByText(/ISO addressing/));
    await user.type(screen.getByLabelText('Remote P-selector'), '00000001');
    await user.type(screen.getByLabelText('Local AP-title'), '-');
    await user.type(screen.getByLabelText('Remote AE-qualifier'), '23');
    await connect(user);
    expect(open).toHaveBeenCalledWith({ host: '10.0.0.5', port: 102, addressing: { remote_psel: '00000001', local_ap_title: '', remote_ae_qualifier: 23 } });

    expect(await screen.findByText(/OT Fabric · Mock IED/)).toBeInTheDocument();
    const panel = await openHz(user);
    await user.click(within(panel).getByRole('button', { name: 'Read' }));
    expect(await within(panel).findByText('50.002')).toBeInTheDocument();
    expect(within(panel).getByText('good')).toBeInTheDocument();
    expect(within(panel).getByText(/MMXU1\$MX\$Hz/)).toBeInTheDocument();
    await user.click(within(panel).getByRole('button', { name: 'Copy MMS item ID' }));
    expect(host.copyText).toHaveBeenLastCalledWith('MMXU1$MX$Hz');

    await user.click(screen.getByRole('tab', { name: 'Activity & diagnostics' }));
    const activity = screen.getByRole('region', { name: 'Activity' });
    expect(within(activity).getAllByText('read').length).toBeGreaterThan(0);
    expect(within(activity).getByText('connect')).toBeInTheDocument();
  });

  it('watches points, polls them, and exports the open tab', async () => {
    const user = userEvent.setup();
    const host = platform();
    render(<App service={new MockService()} platform={host} />);
    await connect(user);
    const panel = await openHz(user);
    await user.click(within(panel).getByRole('button', { name: 'Watch' }));
    expect(within(panel).getByRole('button', { name: 'Watching' })).toHaveAttribute('aria-pressed', 'true');

    const watch = screen.getByRole('region', { name: 'Watch' });
    expect(within(watch).getByText('DemoIEDLD0/MMXU1.Hz[MX]')).toBeInTheDocument();
    await user.selectOptions(within(watch).getByLabelText('Poll interval'), '500');
    await user.click(within(watch).getByRole('button', { name: 'Poll' }));
    expect(await within(watch).findByText(/^50\.00\d$/)).toBeInTheDocument();
    expect(within(watch).getByText('good')).toBeInTheDocument();
    expect(await screen.findByText('polling every 0.5 s')).toBeInTheDocument();
    await user.click(within(watch).getByRole('button', { name: 'Stop polling' }));
    await waitFor(() => expect(screen.queryByText(/polling every/)).not.toBeInTheDocument());

    await user.click(within(watch).getByRole('button', { name: 'CSV' }));
    await waitFor(() => expect(host.saveFile).toHaveBeenCalled());
    const [name, mime, text] = host.saveFile.mock.calls[0] ?? [];
    expect(name).toBe('iec61850-mock.csv');
    expect(mime).toBe('text/csv');
    expect(text).toContain('DemoIEDLD0/MMXU1.Hz[MX]');

    await user.click(within(watch).getByRole('button', { name: 'Stop watching DemoIEDLD0/MMXU1.Hz[MX]' }));
    expect(within(watch).queryByText('DemoIEDLD0/MMXU1.Hz[MX]')).not.toBeInTheDocument();
  });

  it('searches the model and opens a hit', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await connect(user);
    await screen.findByText(/OT Fabric · Mock IED/);
    await user.type(screen.getByLabelText('Search references'), 'totw');
    await user.click(screen.getByRole('button', { name: 'Search' }));
    const results = await screen.findByRole('list', { name: 'Search results' });
    await user.click(within(results).getByText('DemoIEDLD0/MMXU1.TotW'));
    const panel = screen.getByRole('region', { name: 'Selected node' });
    await user.click(within(panel).getByRole('button', { name: 'Read' }));
    expect(await within(panel).findByText('questionable (old-data)')).toBeInTheDocument();
  });

  it('saves a profile and fills the form from it', async () => {
    const user = userEvent.setup();
    const service = new MockService();
    render(<App service={service} platform={platform()} />);
    await waitFor(() => expect(screen.getByLabelText('Profile')).toBeInTheDocument());
    await user.type(screen.getByLabelText('IED host'), '10.1.1.1');
    await user.type(screen.getByLabelText('Profile name'), 'Feeder 7');
    await user.click(screen.getByRole('button', { name: 'Save as profile' }));
    expect(await screen.findByRole('option', { name: 'Feeder 7 (10.1.1.1)' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Update profile' })).toBeInTheDocument();
    expect(await service.profiles()).toHaveLength(1);
  });

  it('opens an SCL file offline: browse yes, read no', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await user.upload(screen.getByLabelText('SCL file'), new File([MOCK_SCL], 'station.scd'));
    const dialog = await screen.findByRole('dialog', { name: 'SCL file' });
    expect(within(dialog).getByText(/2 IEDs/)).toBeInTheDocument();
    await user.click(within(dialog).getAllByRole('button', { name: 'Browse offline' })[0] as HTMLElement);
    expect(await screen.findByText(/SCL station\.scd · DemoIED\/S1/)).toBeInTheDocument();
    const panel = await openHz(user);
    expect(within(panel).getByText(/has no live values/)).toBeInTheDocument();
    expect(within(panel).queryByRole('button', { name: 'Read' })).not.toBeInTheDocument();
  });

  it('fills the connection from an SCL address', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await user.upload(screen.getByLabelText('SCL file'), new File([MOCK_SCL], 'station.scd'));
    const dialog = await screen.findByRole('dialog', { name: 'SCL file' });
    await user.click(within(dialog).getByRole('button', { name: 'Use address' }));
    expect(screen.getByLabelText('IED host')).toHaveValue('10.0.0.5');
    expect(screen.getByLabelText('Remote P-selector')).toHaveValue('00000001');
  });

  it('opens a snapshot whose reads say where they come from', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await user.upload(screen.getByLabelText('Snapshot file'), new File(['{}'], 'feeder.json'));
    expect(await screen.findByText(/Snapshot feeder\.json/)).toBeInTheDocument();
    const panel = await openHz(user);
    await user.click(within(panel).getByRole('button', { name: 'Read' }));
    expect(await within(panel).findByText(/from snapshot/)).toBeInTheDocument();
  });

  it('keeps several sessions in tabs and closes them', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await connect(user, '10.0.0.5');
    await screen.findByText(/OT Fabric · Mock IED/);
    await user.click(screen.getByRole('button', { name: 'New' }));
    await connect(user, '10.0.0.6');
    const tabs = screen.getByRole('navigation', { name: 'Sessions' });
    await waitFor(() => expect(within(tabs).getAllByRole('button', { name: /^Close / })).toHaveLength(2));
    await user.click(within(tabs).getByRole('button', { name: 'Close Mock IED 10.0.0.6' }));
    expect(within(tabs).getAllByRole('button', { name: /^Close / })).toHaveLength(1);
  });

  it('brings back the watched points of a session that already polls', async () => {
    const service = new MockService();
    const session = await service.openSession({ host: '10.0.0.5' });
    await service.startPolling(session.id, { refs: ['DemoIEDLD0/MMXU1.Hz[MX]'], interval_ms: 500 });
    render(<App service={service} platform={platform()} />);
    const watch = screen.getByRole('region', { name: 'Watch' });
    expect(await within(watch).findByText('DemoIEDLD0/MMXU1.Hz[MX]')).toBeInTheDocument();
    expect(within(watch).getByRole('button', { name: 'Stop polling' })).toBeInTheDocument();
    await service.stopPolling(session.id);
  });

  it('builds the equivalent iec61850ctl command', () => {
    const session = { target: { host: '10.0.0.5', port: 102 } } as SessionInfo;
    expect(cliCommand(session, 'LD0/MMXU1.Hz', 'MX')).toBe('iec61850ctl get object --host 10.0.0.5 --object LD0/MMXU1.Hz --fc MX');
    const other = { target: { host: 'ied', port: 1102 } } as SessionInfo;
    expect(cliCommand(other, 'LD0/X', 'ST')).toContain('--port 1102');
  });

  it('summarizes a data object by its primary attribute, quality and time', () => {
    const s = summarize({
      type: 'structure',
      text: '{3}',
      children: [
        { name: 'stVal', type: 'bit-string', text: '10', bits: 2 },
        { name: 'q', type: 'bit-string', text: 'good', quality: { validity: 'good', flags: [], raw: 0 } },
      ],
    });
    expect(s.text).toBe('10');
    expect(s.quality?.validity).toBe('good');
    expect(summarize({ type: 'float', text: '1.5' }).text).toBe('1.5');
  });
});
