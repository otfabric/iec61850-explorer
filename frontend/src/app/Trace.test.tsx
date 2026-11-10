import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { parsePorts } from './components/ConnectionPanel';
import type { Platform } from './platform/platform';
import { MockService } from './service/mock';

function platform() {
  const host = {
    mode: { kind: 'browser' } as const,
    saveFile: vi.fn(async (_name: string, _mime: string, _text: string) => true),
    saveBlob: vi.fn(async (_name: string, _blob: Blob) => true),
    openExternal: vi.fn(),
    persistTheme: vi.fn(),
    loadTheme: () => 'dark' as const,
    copyText: vi.fn(async (_text: string) => {}),
  };
  return host satisfies Platform;
}

async function openCapture(user: ReturnType<typeof userEvent.setup>, ports = '102, 1102') {
  const field = screen.getByLabelText('MMS ports');
  await user.clear(field);
  if (ports) {
    await user.type(field, ports);
  }
  await user.upload(screen.getByLabelText('Capture file'), new File([new Uint8Array([0xd4, 0xc3, 0xb2, 0xa1])], 'feeder.pcapng'));
}

describe('trace sessions', () => {
  it('parses the MMS ports', () => {
    expect(parsePorts('102, 1102')).toEqual([102, 1102]);
    expect(parsePorts('')).toEqual([]);
    expect(parsePorts('102,x')).toBeNull();
    expect(parsePorts('70000')).toBeNull();
  });

  it('opens a capture with its ports and shows the summary', async () => {
    const user = userEvent.setup();
    const service = new MockService();
    const open = vi.spyOn(service, 'openTrace');
    render(<App service={service} platform={platform()} />);
    await openCapture(user);
    expect(open).toHaveBeenCalledWith(expect.objectContaining({ name: 'feeder.pcapng' }), [102, 1102]);
    expect(await screen.findByRole('button', { name: 'feeder.pcapng (capture)' })).toBeInTheDocument();
    const summary = await screen.findByRole('region', { name: 'Trace summary' });
    expect(within(summary).getByText('pcapng')).toBeInTheDocument();
    expect(within(summary).getByText(/9f2c6a1e/)).toBeInTheDocument();
    expect(within(summary).getByText('102, 1102')).toBeInTheDocument();
    expect(within(summary).getByText('unanswered: 1')).toBeInTheDocument();
    expect(within(summary).getByText('TPKT resync (bytes): 12')).toBeInTheDocument();
    // A trace session offers no model actions.
    expect(screen.queryByRole('button', { name: 'Compare' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Commission/ })).not.toBeInTheDocument();
  });

  it('gives a trace a smaller dock that collapses to its tabs and opens again from a tab', async () => {
    const user = userEvent.setup();
    localStorage.removeItem('iec61850-explorer.dock');
    const { container } = render(<App service={new MockService()} platform={platform()} />);
    await openCapture(user);
    await screen.findByRole('region', { name: 'Trace summary' });
    const grid = () => container.querySelector('.grid.min-h-0.flex-1') as HTMLElement;
    expect(grid().className).toContain('grid-rows-[1fr_minmax(128px,20%)]');
    const dock = screen.getByRole('region', { name: 'Dock' });
    await user.click(within(dock).getByRole('button', { name: 'Hide the dock' }));
    expect(grid().className).toContain('grid-rows-[1fr_auto]');
    expect(within(dock).getByRole('button', { name: 'Show the dock' })).toHaveAttribute('aria-expanded', 'false');
    expect(within(dock).queryByRole('button', { name: /Read all/ })).not.toBeInTheDocument();
    expect(localStorage.getItem('iec61850-explorer.dock')).toBe('collapsed');
    await user.click(within(dock).getByRole('tab', { name: /Activity/ }));
    expect(grid().className).toContain('grid-rows-[1fr_minmax(128px,20%)]');
    expect(within(dock).getByRole('button', { name: 'Hide the dock' })).toHaveAttribute('aria-expanded', 'true');
    localStorage.removeItem('iec61850-explorer.dock');
  });

  it('keeps the session bar on one line: the information truncates, the actions have names and tooltips', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await openCapture(user);
    const actions = await screen.findByRole('toolbar', { name: 'Session actions' });
    expect(actions.parentElement?.className).not.toContain('flex-wrap');
    const close = within(actions).getByRole('button', { name: 'Close' });
    expect(close).toHaveAttribute('title', 'Close the capture');
    expect(within(close).getByText('Close').className).toContain('max-2xl:sr-only');
  });

  it('lists findings with where they are and their source, filtered by severity', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await openCapture(user, '');
    await user.click(await screen.findByRole('tab', { name: 'findings' }));
    const findings = await screen.findByRole('region', { name: 'Findings' });
    expect(await within(findings).findByText('tcp-gap')).toBeInTheDocument();
    expect(within(findings).getByText('Association.Gaps')).toBeInTheDocument();
    expect(within(findings).getByText('Transaction.Status=unanswered')).toBeInTheDocument();
    expect(within(findings).getByText(/assoc 1 · frame 11 · #3/)).toBeInTheDocument();
    await user.selectOptions(within(findings).getByLabelText('Severity'), 'error');
    await waitFor(() => expect(within(findings).queryByText('unanswered')).not.toBeInTheDocument());
    expect(within(findings).getByText('tcp-gap')).toBeInTheDocument();
  });

  it('filters the timeline by status and association and expands a transaction', async () => {
    const user = userEvent.setup();
    render(<App service={new MockService()} platform={platform()} />);
    await openCapture(user);
    await user.click(await screen.findByRole('tab', { name: 'timeline' }));
    const timeline = await screen.findByRole('region', { name: 'Timeline' });
    await waitFor(() => expect(within(timeline).getAllByTestId('transaction')).toHaveLength(5));
    await user.click(within(timeline).getByLabelText('unanswered'));
    await waitFor(() => expect(within(timeline).getAllByTestId('transaction')).toHaveLength(1));
    const row = within(timeline).getByTestId('transaction');
    expect(within(row).getByText('DemoIEDLD0/LLN0.Mod.stVal[ST]')).toBeInTheDocument();
    await user.click(row);
    expect(await within(timeline).findByText('not captured')).toBeInTheDocument();

    await user.click(within(timeline).getByLabelText('unanswered'));
    await user.selectOptions(within(timeline).getByLabelText('Association'), '2');
    await waitFor(() => expect(within(timeline).getAllByTestId('transaction')).toHaveLength(2));
    await user.type(within(timeline).getByLabelText('Service'), 'GetNameList');
    await waitFor(() => expect(within(timeline).getAllByTestId('transaction')).toHaveLength(1));
  });

  it('links the capture to a model session and shows the resolution', async () => {
    const user = userEvent.setup();
    const service = new MockService();
    render(<App service={service} platform={platform()} />);
    await user.type(screen.getByLabelText('IED host'), '10.0.0.5');
    await user.click(screen.getByRole('button', { name: 'Connect' }));
    await screen.findByText(/OT Fabric · Mock IED/);
    await user.click(screen.getByRole('button', { name: 'New' }));
    await openCapture(user);
    const trace = await screen.findByRole('region', { name: 'Trace' });
    const picker = within(trace).getByLabelText('Correlate with model');
    const live = within(picker).getAllByRole('option')[1];
    expect(live).toHaveTextContent('Mock IED 10.0.0.5 (live)');
    await user.selectOptions(picker, live!);
    await user.click(within(trace).getByRole('button', { name: 'Link' }));
    expect(await within(trace).findByText(/Linked to 10.0.0.5:102 \(live\)/)).toBeInTheDocument();
    expect(within(trace).getByRole('list', { name: 'Link notes' })).toHaveTextContent('no GOOSE control blocks');

    await user.click(within(trace).getByRole('tab', { name: 'timeline' }));
    const timeline = await within(trace).findByRole('region', { name: 'Timeline' });
    expect(await within(timeline).findByText('not in the model')).toBeInTheDocument();
    expect(within(timeline).getAllByText('found').length).toBeGreaterThan(0);
    expect(within(timeline).getByText('data set DemoIEDLD0/LLN0.Meas')).toBeInTheDocument();

    await user.click(within(trace).getByRole('tab', { name: 'reports' }));
    const reports = await within(trace).findByRole('region', { name: 'Captured reports' });
    expect(await within(reports).findByText('decoded')).toBeInTheDocument();
    expect(within(reports).getByText(/DemoIEDLD0\/MMXU1.Hz\[MX\]/)).toBeInTheDocument();

    await user.click(within(trace).getByRole('tab', { name: 'streams' }));
    const streams = await within(trace).findByRole('region', { name: 'Streams' });
    expect(within(streams).getByText('model has no GoCBs')).toBeInTheDocument();
    expect(within(streams).getByText(/sqNum gap ×2/)).toBeInTheDocument();

    await user.click(within(trace).getByRole('tab', { name: 'findings' }));
    expect(await within(trace).findByText('not-in-model')).toBeInTheDocument();

    await user.selectOptions(picker, '');
    await user.click(within(trace).getByRole('button', { name: 'Unlink' }));
    expect(await within(trace).findByText(/Not linked/)).toBeInTheDocument();
  });

  it('exports findings and the timeline', async () => {
    const user = userEvent.setup();
    const host = platform();
    render(<App service={new MockService()} platform={host} />);
    await openCapture(user);
    await user.click(await screen.findByRole('button', { name: 'Findings CSV' }));
    await waitFor(() => expect(host.saveFile).toHaveBeenCalledWith('feeder.pcapng-findings.csv', 'text/csv', expect.stringContaining('tcp-gap')));
    await user.click(screen.getByRole('button', { name: 'Timeline JSON' }));
    await waitFor(() => expect(host.saveFile).toHaveBeenLastCalledWith('feeder.pcapng-timeline.json', 'application/json', expect.stringContaining('Identify')));
  });

  it('shows the wire view of an operation, matched by invoke ID', async () => {
    const user = userEvent.setup();
    const service = new MockService();
    const wire = vi.spyOn(service, 'wire');
    render(<App service={service} platform={platform()} />);
    await user.type(screen.getByLabelText('IED host'), '10.0.0.5');
    await user.click(screen.getByRole('button', { name: 'Connect' }));
    await screen.findByText(/OT Fabric · Mock IED/);
    await user.click(screen.getByRole('tab', { name: 'Activity & diagnostics' }));
    const activity = screen.getByRole('region', { name: 'Activity' });
    await user.click(within(activity).getByText('connect'));
    await user.click(within(activity).getByRole('button', { name: 'Wire' }));
    const units = await within(activity).findByRole('list', { name: 'Wire units' });
    expect(wire).toHaveBeenCalledWith(expect.stringMatching(/^mock-/), 1);
    expect(within(units).getByText('→ sent')).toBeInTheDocument();
    expect(within(units).getByText('← received')).toBeInTheDocument();
    expect(units.querySelectorAll('[data-matched="true"]')).toHaveLength(2);
    expect(within(units).getAllByText(/^0100010061/)).toHaveLength(2);
  });
});
