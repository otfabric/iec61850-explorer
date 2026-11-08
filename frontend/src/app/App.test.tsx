import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { cliCommand } from './components/NodePanel';
import type { Platform } from './platform/platform';
import { MockService } from './service/mock';
import type { SessionInfo } from './service/types';

function platform() {
  const host = {
    mode: { kind: 'browser' } as const,
    saveFile: vi.fn(async () => true),
    openExternal: vi.fn(),
    persistTheme: vi.fn(),
    loadTheme: () => 'dark' as const,
    copyText: vi.fn(async (_text: string) => {}),
  };
  return host satisfies Platform;
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
    await user.type(screen.getByLabelText('IED host'), 'unreachable');
    await user.click(screen.getByRole('button', { name: 'Connect' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Could not connect');
    expect(alert).toHaveTextContent('TCP connection');
    expect(alert).toHaveTextContent('Check the address');
  });

  it('connects, browses, reads with quality and time, and logs the requests', async () => {
    const user = userEvent.setup();
    const host = platform();
    render(<App service={new MockService()} platform={host} />);
    await user.type(screen.getByLabelText('IED host'), '10.0.0.5');
    await user.click(screen.getByRole('button', { name: 'Connect' }));

    expect(await screen.findByText(/OT Fabric · Mock IED/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Expand DemoIEDLD0' }));
    await user.click(await screen.findByRole('button', { name: 'Expand MMXU1' }));
    await user.click(await screen.findByText('Hz'));

    const panel = screen.getByRole('region', { name: 'Selected node' });
    expect(within(panel).getByText('DemoIEDLD0/MMXU1.Hz')).toBeInTheDocument();
    await user.click(within(panel).getByRole('button', { name: 'Read' }));
    expect(await within(panel).findByText('50.002')).toBeInTheDocument();
    expect(within(panel).getByText('good')).toBeInTheDocument();
    expect(within(panel).getByText('2026-11-08T14:32:08.417000Z')).toBeInTheDocument();
    expect(within(panel).getByText(/MMXU1\$MX\$Hz/)).toBeInTheDocument();

    await user.click(within(panel).getByRole('button', { name: 'Copy MMS item ID' }));
    expect(host.copyText).toHaveBeenLastCalledWith('MMXU1$MX$Hz');

    const activity = screen.getByRole('region', { name: 'Activity' });
    expect(within(activity).getAllByText('read').length).toBeGreaterThan(0);
    expect(within(activity).getByText('connect')).toBeInTheDocument();
  });

  it('builds the equivalent iec61850ctl command', () => {
    const session = { target: { host: '10.0.0.5', port: 102 } } as SessionInfo;
    expect(cliCommand(session, 'LD0/MMXU1.Hz', 'MX')).toBe('iec61850ctl get object --host 10.0.0.5 --object LD0/MMXU1.Hz --fc MX');
    const other = { target: { host: 'ied', port: 1102 } } as SessionInfo;
    expect(cliCommand(other, 'LD0/X', 'ST')).toContain('--port 1102');
  });
});
