import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { LinkStatus } from '../../app/service/service';
import type { ExplorerEvent } from '../../app/service/types';
import { HttpService } from './http';
import { takeToken } from './token';

const TOKEN = 's3cr3t-token-0123456789';

const platform = { saveBlob: vi.fn(async (_name: string, _blob: Blob) => true) };

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const frame = (event: Partial<ExplorerEvent>) => `id: ${event.seq}\nevent: ${event.kind}\ndata: ${JSON.stringify(event)}\n\n`;

/** An event stream that delivers its text and then ends, or stays open. */
function stream(text: string, open = false): Response {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(text));
      if (!open) {
        controller.close();
      }
    },
  });
  return new Response(body, { status: 200 });
}

const urlsOf = (fetchMock: ReturnType<typeof vi.fn>) => fetchMock.mock.calls.map(([url]) => String(url));

describe('HttpService', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('keeps the token in memory only: not in the address, storage, cookies or request URLs', async () => {
    window.history.replaceState(null, '', `/#token=${TOKEN}`);
    const fetchMock = vi.fn(async (url: string) => (url.includes('/events') ? stream(': keep-alive\n\n', true) : json({ status: 'ok' })));
    vi.stubGlobal('fetch', fetchMock);

    const service = new HttpService(takeToken(window), platform);
    const unsubscribe = service.subscribe(0, () => {});
    await service.health();
    await service.sessions();
    await vi.advanceTimersByTimeAsync(0);
    unsubscribe();

    expect(window.location.href).not.toContain(TOKEN);
    expect(window.location.hash).toBe('');
    expect(JSON.stringify({ ...localStorage })).not.toContain(TOKEN);
    expect(JSON.stringify({ ...sessionStorage })).not.toContain(TOKEN);
    expect(document.cookie).not.toContain(TOKEN);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    for (const [url, init] of fetchMock.mock.calls as unknown as [string, RequestInit][]) {
      expect(url).not.toContain(TOKEN);
      expect(init.headers).toMatchObject({ Authorization: `Bearer ${TOKEN}` });
      expect(init.body ?? '').not.toContain(TOKEN);
    }
  });

  it('reports a lost stream, resumes from the last sequence number and follows a resync down', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(stream(frame({ seq: 4, kind: 'operation' }) + frame({ seq: 5, kind: 'operation' })))
      .mockRejectedValueOnce(new TypeError('network'))
      // A restarted daemon: it cannot replay and numbers its events from 1 again.
      .mockResolvedValueOnce(stream(frame({ seq: 2, kind: 'resync' })))
      .mockResolvedValueOnce(stream('', true));
    vi.stubGlobal('fetch', fetchMock);
    const events: number[] = [];
    const link: LinkStatus[] = [];

    const unsubscribe = new HttpService(TOKEN, platform).subscribe(0, (event) => events.push(event.seq), (status) => link.push(status));
    await vi.advanceTimersByTimeAsync(10_000);
    unsubscribe();

    expect(urlsOf(fetchMock)).toEqual(['/api/v1/events?since=0', '/api/v1/events?since=5', '/api/v1/events?since=5', '/api/v1/events?since=2']);
    expect(events).toEqual([4, 5, 2]);
    expect(link).toEqual(['up', 'down', 'down', 'up', 'down', 'up']);
  });

  it('treats a stream without keep-alives as lost', async () => {
    const fetchMock = vi.fn(async (_url: string, init: RequestInit) => {
      const response = stream(': keep-alive\n\n', true);
      // Like a real fetch, an aborted request fails its pending read.
      init.signal?.addEventListener('abort', () => void response.body?.cancel().catch(() => {}));
      return response;
    });
    vi.stubGlobal('fetch', fetchMock);
    const link: LinkStatus[] = [];
    const abortSpy = vi.spyOn(AbortController.prototype, 'abort');

    const unsubscribe = new HttpService(TOKEN, platform).subscribe(0, () => {}, (status) => link.push(status));
    await vi.advanceTimersByTimeAsync(44_000);
    expect(abortSpy).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(abortSpy).toHaveBeenCalled();
    unsubscribe();
    expect(link[0]).toBe('up');
  });

  it('reports the link as down and reopens the stream when a request cannot reach the daemon', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.includes('/events')) {
        return stream('', true);
      }
      throw new TypeError('network');
    });
    vi.stubGlobal('fetch', fetchMock);
    const link: LinkStatus[] = [];
    const service = new HttpService(TOKEN, platform);
    const unsubscribe = service.subscribe(0, () => {}, (status) => link.push(status));
    await vi.advanceTimersByTimeAsync(0);

    await expect(service.health()).rejects.toMatchObject({ message: 'The daemon cannot be reached.' });
    expect(link).toEqual(['up', 'down']);
    unsubscribe();
  });

  it('reports a rejected token as unauthorized and stops the stream', async () => {
    const fetchMock = vi.fn(async () => json({ error: { code: 'unauthorized', message: 'invalid token' } }, 401));
    vi.stubGlobal('fetch', fetchMock);
    const link: LinkStatus[] = [];
    const service = new HttpService('wrong', platform);
    service.subscribe(0, () => {}, (status) => link.push(status));

    await expect(service.health()).rejects.toMatchObject({ code: 'unauthorized', message: 'invalid token' });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(link).toEqual(['unauthorized', 'unauthorized']);
    expect(urlsOf(fetchMock).filter((url) => url.includes('/events'))).toHaveLength(1);
  });

  it('does not contact the daemon without a token', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const link: LinkStatus[] = [];
    const service = new HttpService(null, platform);
    service.subscribe(0, () => {}, (status) => link.push(status));

    await expect(service.sessions()).rejects.toMatchObject({ code: 'unauthorized' });
    expect(link).toEqual(['unauthorized', 'unauthorized']);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('passes error bodies on, with the classified detail, and accepts an empty 204', async () => {
    const detail = { code: 'connect_failed', message: 'refused', stage: 'tcp', hint: 'check the port' };
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(json({ error: { code: 'connect_failed', message: 'cannot connect', detail } }, 502))
        .mockResolvedValueOnce(new Response(null, { status: 204 })),
    );
    const service = new HttpService(TOKEN, platform);
    await expect(service.openSession({ host: '10.0.0.1' })).rejects.toMatchObject({ code: 'connect_failed', detail });
    await expect(service.closeSession('s1')).resolves.toBeUndefined();
  });

  it('downloads a file of the IED with the token in the header and hands it to the platform', async () => {
    const fetchMock = vi.fn(
      async (_url: string, _init: RequestInit) =>
        new Response('cfg', { status: 200, headers: { 'Content-Disposition': 'attachment; filename="fault1.cfg"' } }),
    );
    vi.stubGlobal('fetch', fetchMock);
    await expect(new HttpService(TOKEN, platform).saveDeviceFile('s1', 'COMTRADE/fault1.cfg')).resolves.toBe(true);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/sessions/s1/files/download?name=COMTRADE%2Ffault1.cfg');
    expect(init.headers).toMatchObject({ Authorization: `Bearer ${TOKEN}` });
    const [name, blob] = platform.saveBlob.mock.calls.at(-1) as [string, Blob];
    expect(name).toBe('fault1.cfg');
    expect(await blob.text()).toBe('cfg');
  });

  it('browses with the reference as a query parameter and reads with a JSON body', async () => {
    const fetchMock = vi.fn(async (url: string, _init: RequestInit) => json(url.includes('/browse') ? [] : { at: '', duration_ms: 1, values: [] }));
    vi.stubGlobal('fetch', fetchMock);
    const service = new HttpService(TOKEN, platform);

    await service.browse('s 1', 'LD0/MMXU1');
    await service.read('s 1', { refs: ['LD0/MMXU1.Hz[MX]'] });

    expect(urlsOf(fetchMock)).toEqual(['/api/v1/sessions/s%201/browse?ref=LD0%2FMMXU1', '/api/v1/sessions/s%201/read']);
    expect((fetchMock.mock.calls[1] as [string, RequestInit])[1]).toMatchObject({ method: 'POST', body: '{"refs":["LD0/MMXU1.Hz[MX]"]}' });
  });

  it('reads and saves the settings at /settings', async () => {
    const fetchMock = vi.fn(async (_url: string, _init: RequestInit) => json({ default_proxy: 'direct', default_proxy_locked: false }));
    vi.stubGlobal('fetch', fetchMock);
    const service = new HttpService(TOKEN, platform);

    expect(await service.settings()).toEqual({ default_proxy: 'direct', default_proxy_locked: false });
    await service.saveSettings({ default_proxy: 'direct', default_proxy_locked: false });

    expect(urlsOf(fetchMock)).toEqual(['/api/v1/settings', '/api/v1/settings']);
    expect((fetchMock.mock.calls[0] as [string, RequestInit])[1]).toMatchObject({ method: 'GET' });
    expect((fetchMock.mock.calls[1] as [string, RequestInit])[1]).toMatchObject({
      method: 'PUT',
      body: '{"default_proxy":"direct","default_proxy_locked":false}',
    });
  });

  it('calls the trace endpoints: capture upload with ports, link, views and the wire view', async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => json({}));
    vi.stubGlobal('fetch', fetchMock);
    const service = new HttpService(TOKEN, platform);
    await service.openTrace({ name: 'cap.pcapng', data: new Uint8Array([1, 2]) }, [102, 1102]);
    await service.openTrace({ name: 'b.pcap', data: new Uint8Array([3]) }, []);
    await service.linkTrace('t1', 's1');
    await service.traceSummary('t1');
    await service.traceAssociations('t1');
    await service.traceTimeline('t1', { status: ['unanswered'], limit: 10 });
    await service.traceReports('t1', { association: 2 });
    await service.traceStreams('t1');
    await service.traceFindings('t1', { severity: 'error' });
    await service.wire('s1', 7);
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][];
    expect(calls.map(([url, init]) => `${init.method} ${url}`)).toEqual([
      'POST /api/v1/traces/open?name=cap.pcapng&ports=102%2C1102',
      'POST /api/v1/traces/open?name=b.pcap',
      'PUT /api/v1/sessions/t1/trace/link',
      'GET /api/v1/sessions/t1/trace/summary',
      'GET /api/v1/sessions/t1/trace/associations',
      'POST /api/v1/sessions/t1/trace/timeline',
      'POST /api/v1/sessions/t1/trace/reports',
      'GET /api/v1/sessions/t1/trace/streams',
      'POST /api/v1/sessions/t1/trace/findings',
      'GET /api/v1/sessions/s1/wire?operation=7',
    ]);
    expect(calls[0]?.[1].body).toBeInstanceOf(Uint8Array);
    expect(calls[2]?.[1].body).toBe(JSON.stringify({ session: 's1' }));
    expect(calls[5]?.[1].body).toBe(JSON.stringify({ status: ['unanswered'], limit: 10 }));
  });
});
