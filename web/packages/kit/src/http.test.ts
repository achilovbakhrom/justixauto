import { afterEach, describe, expect, it, vi } from 'vitest';
import { post } from './http';

type Reply = { status: number; body?: unknown } | 'network-error';

/** Stubs fetch with the given replies and records the Idempotency-Key of each call. */
function server(...replies: Reply[]) {
  const keys: (string | null)[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init: RequestInit) => {
      keys.push(new Headers(init.headers).get('Idempotency-Key'));
      const reply = replies.shift() ?? { status: 201, body: { data: {}, revision: '1' } };
      if (reply === 'network-error') return Promise.reject(new TypeError('Failed to fetch'));
      return Promise.resolve(new Response(JSON.stringify(reply.body ?? {}), { status: reply.status }));
    }),
  );
  return keys;
}

const created = { status: 201, body: { data: {}, revision: '1' } };
const failure = (status: number, code: string) => ({ status, body: { error: { code, message: code } } });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Idempotency-Key follows the user action', () => {
  it('reuses the key when the same POST is resubmitted after a lost response', async () => {
    const keys = server('network-error', created);
    await expect(post('/retail/leads', { name: 'A' })).rejects.toMatchObject({ code: 'network' });
    await post('/retail/leads', { name: 'A' });
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
  });

  it('reuses the key after a server error or while the first attempt is in progress', async () => {
    const keys = server(failure(503, 'unavailable'), failure(409, 'request_in_progress'), created);
    await expect(post('/retail/tasks', { title: 'T' })).rejects.toMatchObject({ status: 503 });
    await expect(post('/retail/tasks', { title: 'T' })).rejects.toMatchObject({ code: 'request_in_progress' });
    await post('/retail/tasks', { title: 'T' });
    expect(new Set(keys).size).toBe(1);
  });

  it('shares the key between concurrent identical submissions (double click)', async () => {
    const keys = server(created, created);
    await Promise.all([post('/retail/leads', { name: 'A' }), post('/retail/leads', { name: 'A' })]);
    expect(keys[1]).toBe(keys[0]);
  });

  it('uses a new key once the previous action has a definitive outcome', async () => {
    const keys = server(created, failure(422, 'validation_failed'), created);
    await post('/retail/leads', { name: 'A' });
    await expect(post('/retail/leads', { name: 'A' })).rejects.toMatchObject({ status: 422 });
    await post('/retail/leads', { name: 'A' });
    expect(new Set(keys).size).toBe(3);
  });

  it('uses different keys for different requests', async () => {
    const keys = server('network-error', created, created);
    await expect(post('/retail/leads', { name: 'A' })).rejects.toMatchObject({ code: 'network' });
    await post('/retail/leads', { name: 'B' });
    await post('/retail/customers', { name: 'A' });
    expect(new Set(keys).size).toBe(3);
  });

  it('sends an explicit key unchanged', async () => {
    const key = '0b6a3c1e-8f7d-4d2a-9c4b-5e6f7a8b9c0d';
    const keys = server(created);
    await post('/retail/leads', { name: 'A' }, { idempotencyKey: key });
    expect(keys).toEqual([key]);
  });
});
