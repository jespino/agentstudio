import { afterEach, expect, it, vi } from 'vitest';
import { createBackend } from './backend';

afterEach(() => vi.unstubAllGlobals());

function respond(status: number, body: unknown, type = 'application/json') {
  return new Response(typeof body === 'string' ? body : JSON.stringify(body), { status, headers: { 'Content-Type': type } });
}

it('calls a method over HTTP with its arguments as JSON', async () => {
  const fetch = vi.fn().mockResolvedValue(respond(200, { result: 3 }));
  vi.stubGlobal('fetch', fetch);
  const b = createBackend({ service: 'x.App' });
  expect(await b.call<number>('Add', 1, 2)).toBe(3);
  const [url, init] = fetch.mock.calls[0];
  expect(url).toBe('/rpc/Add');
  expect(init.method).toBe('POST');
  expect(init.headers['Content-Type']).toBe('application/json');
  expect(JSON.parse(init.body)).toEqual([1, 2]);
});

it('turns an error result into a rejection', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respond(200, { error: 'no project is open' })));
  await expect(createBackend({ service: 'x.App' }).call('SaveSource', 'x')).rejects.toThrow('no project is open');
});

it('explains a missing backend and a refused request', async () => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('connection refused')));
  await expect(createBackend({ service: 'x.App', noBackend: 'start it' }).call('X')).rejects.toThrow('start it');
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respond(403, 'Open the full address', 'text/plain')));
  await expect(createBackend({ service: 'x.App' }).call('X')).rejects.toThrow(/Open the full address .*refused/);
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respond(502, '<html>bad gateway</html>', 'text/html')));
  await expect(createBackend({ service: 'x.App', noBackend: 'start it' }).call('X')).rejects.toThrow('start it');
});
