import { describe, expect, it, vi } from 'vitest';
import { createApiClient, errorStatuses, isRevision } from './client';
import type { ApiRequest, Schema } from './client';
import { queryKey } from './queryKeys';
import type { QueryResource, QueryScope } from './queryKeys';

const userId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const companyId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const branchA = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc';
const branchB = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';
const origin = 'https://justix.test';
const path = '/api/v1/retail/leads';
const schema: Schema<{ revision: string }> = {
  parse(value) {
    if (typeof value !== 'object' || value === null || !('revision' in value)
      || !isRevision(value.revision) || Object.keys(value).length !== 1) throw new Error('Invalid DTO');
    return { revision: value.revision };
  },
};
const request: ApiRequest<{ revision: string }> = { path, schema, successStatuses: [200] };
const errorReceipt = { error: { code: 'DENIED', message: 'Unavailable', fields: {}, traceId: 'trace-fixture' } };
function setup(response: Response = Response.json({ revision: '0' })) {
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response);
  return { fetcher, client: createApiClient({ origin, fetch: fetcher }) };
}

describe('same-origin validated transport', () => {
  it('uses cookies without browser caching or redirects and validates a feature DTO', async () => {
    const { client, fetcher } = setup();
    expect(await client.request(request)).toEqual({ kind: 'success', status: 200, data: { revision: '0' } });
    expect(fetcher).toHaveBeenCalledTimes(1);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(String(url)).toBe(`${origin}${path}`);
    expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error' });
    expect(new Headers(init?.headers).get('Accept')).toBe('application/json');
  });

  it('sends exact revision/key headers without inventing body authority', async () => {
    const { client, fetcher } = setup();
    const controller = new AbortController();
    await client.request({ ...request, method: 'POST', body: { label: 'synthetic' }, contextRevision: '7', ifMatch: '9', idempotencyKey: userId,
      headers: { 'X-Feature-Challenge': 'ephemeral' }, signal: controller.signal });
    const init = fetcher.mock.calls[0]![1]!;
    const headers = new Headers(init.headers);
    expect(headers.get('X-Context-Revision')).toBe('7');
    expect(headers.get('If-Match')).toBe('"9"');
    expect(headers.get('Idempotency-Key')).toBe(userId);
    expect(headers.get('X-Feature-Challenge')).toBe('ephemeral');
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(init.signal).toBe(controller.signal);
    expect(init.body).toBe('{"label":"synthetic"}');
  });

  it.each(errorStatuses)('validates and preserves HTTP %s error receipt', async (status) => {
    const receipt = { ...errorReceipt, operationId: userId };
    const { client } = setup(Response.json(receipt, { status }));
    expect(await client.request(request)).toEqual({ kind: 'http-error', status, receipt });
  });

  it.each([201, 202] as const)('passes only the selected %s success to its schema', async (status) => {
    const pending = { operationId: userId, status: 'pending' };
    const adapter = { parse: vi.fn((value: unknown) => value) };
    const { client } = setup(Response.json(pending, { status }));
    expect(await client.request({ ...request, schema: adapter, successStatuses: [status] })).toEqual({ kind: 'success', status, data: pending });
    expect(adapter.parse).toHaveBeenCalledWith(pending);
  });

  it('supports the explicit logout no-body exception through its adapter', async () => {
    const { client } = setup(new Response(null, { status: 204 }));
    const adapter = { parse: vi.fn((value: unknown) => { expect(value).toBeUndefined(); return undefined; }) };
    expect(await client.request({ path: '/api/v1/identity/session/logout', method: 'POST', body: {}, schema: adapter, successStatuses: [204] }))
      .toEqual({ kind: 'success', status: 204, data: undefined });
  });

  it.each([
    'https://evil.test/api/v1/retail/leads', '//evil.test/api/v1/retail/leads', '/finance/',
    '/internal/v1/retail/leads', '/api/v1/finance/leads', '/api/v1/retail/../../admin/',
    '/api/v1/retail/%2e%2e/leads', '/api/v1/retail/%252e%252e/leads', '/api/v1/retail/%2fadmin',
    '/api/v1/retail/\\evil', '/api/v1/retail/leads#secret', '/api/v1/retail/%00',
  ])('rejects unsafe or non-API path %s before fetch', async (unsafePath) => {
    const { client, fetcher } = setup();
    expect(await client.request({ ...request, path: unsafePath })).toEqual({ kind: 'invalid-request' });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('permits encoded query values and owner operation receipt lookup', async () => {
    const { client, fetcher } = setup();
    await client.request({ ...request, path: `/api/v1/operations/retail/${userId}?cursor=a%2Fb` });
    expect(String(fetcher.mock.calls[0]![0])).toBe(`${origin}/api/v1/operations/retail/${userId}?cursor=a%2Fb`);
  });

  it.each([
    'Authorization', 'Cookie', 'X-Actor-Id', 'X_User_Id', 'X-Company-Id', 'X-Internal-Caller', 'If-Match', 'X-Context-Revision',
    'X-Actor', 'X_Actor_Kind', 'X-Permission', 'X-Permissions', 'X_Permissions',
    'X-Justix-Internal-Caller', 'X_Justix_Internal_Caller',
  ])(
    'rejects caller-supplied reserved header %s', async (name) => {
      const { client, fetcher } = setup();
      expect(await client.request({ ...request, headers: { [name]: 'spoof' } })).toEqual({ kind: 'invalid-request' });
      expect(fetcher).not.toHaveBeenCalled();
    });

  it.each([{ ifMatch: '01' }, { contextRevision: '9223372036854775808' }, { idempotencyKey: 'not-a-uuid' }, { body: {} }, { successStatuses: [] }])(
    'rejects invalid transport metadata %j', async (invalid) => {
      const { client, fetcher } = setup();
      expect(await client.request({ ...request, ...invalid })).toEqual({ kind: 'invalid-request' });
      expect(fetcher).not.toHaveBeenCalled();
    });

  it.each([
    new Response('<html>another app</html>', { headers: { 'Content-Type': 'text/html' } }),
    new Response('{broken', { headers: { 'Content-Type': 'application/json' } }),
    Response.json({ revision: 1 }), Response.json({ revision: '1', password: 'synthetic-secret' }),
  ])('rejects HTML, malformed JSON and incompatible schemas without copying response diagnostics', async (response) => {
    const { client } = setup(response);
    expect(await client.request(request)).toEqual({ kind: 'invalid-response', status: 200 });
  });

  it.each([
    {}, { error: { code: 'DENIED' } }, { ...errorReceipt, privateCredential: 'synthetic' },
    { error: { ...errorReceipt.error, fields: [] } }, { ...errorReceipt, operationId: 'invalid' },
  ])('rejects malformed error receipts %j', async (receipt) => {
    const { client } = setup(Response.json(receipt, { status: 403 }));
    expect(await client.request(request)).toEqual({ kind: 'invalid-response', status: 403 });
  });

  it('does not interpret unexpected status as a successful receipt', async () => {
    const { client } = setup(Response.json({ revision: '1' }, { status: 202 }));
    expect(await client.request(request)).toEqual({ kind: 'unexpected-status', status: 202 });
  });

  it('rejects a redirected or substituted response from an injected transport', async () => {
    for (const properties of [{ redirected: { value: true } }, { url: { value: 'https://evil.test/api/v1/retail/leads' } }]) {
      const response = Response.json({ revision: '1' });
      Object.defineProperties(response, properties);
      expect(await setup(response).client.request(request)).toEqual({ kind: 'invalid-response', status: 200 });
    }
  });

  it.each([502, 504, 500, 302])('returns unexpected status %s without automatic retry', async (status) => {
    const { client, fetcher } = setup(new Response('untrusted upstream', { status }));
    expect(await client.request(request)).toEqual({ kind: 'unexpected-status', status });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it.each(['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const)('never retries %s after an unknown network outcome', async (method) => {
    const { client, fetcher } = setup();
    fetcher.mockRejectedValue(new Error('synthetic secret in network diagnostic'));
    expect(await client.request({ ...request, method })).toEqual({ kind: 'transport-error', outcome: method === 'GET' ? 'unavailable' : 'unknown', aborted: false });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('aborting a write does not imply the owner cancelled or rolled back', async () => {
    const { client, fetcher } = setup();
    const controller = new AbortController();
    controller.abort();
    fetcher.mockRejectedValue(new Error('aborted'));
    expect(await client.request({ ...request, method: 'POST', signal: controller.signal })).toEqual({ kind: 'transport-error', outcome: 'unknown', aborted: true });
  });

  it('does not retain responses between requests', async () => {
    const { client, fetcher } = setup();
    fetcher.mockResolvedValueOnce(Response.json({ revision: '1' })).mockResolvedValueOnce(Response.json({ revision: '2' }));
    expect(await client.request(request)).toMatchObject({ data: { revision: '1' } });
    expect(await client.request(request)).toMatchObject({ data: { revision: '2' } });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});

const scope: QueryScope = { app: 'realization', userId, companyId, contextRevision: '1', branchScope: { mode: 'ALL', branchIds: [] } };
const resource: QueryResource = { owner: 'retail', resource: 'leads' };
describe('scope-isolated query keys', () => {
  it('uses the approved tuple including anonymous company context and pagination', () => {
    expect(queryKey(scope, resource)).toEqual(['realization', userId, '1', companyId, [], 'retail', 'leads', null, {}, null]);
    expect(queryKey({ ...scope, companyId: null }, resource)[3]).toBeNull();
  });

  it.each([
    { app: 'admin' }, { userId: branchA }, { companyId: branchA }, { contextRevision: '2' },
    { branchScope: { mode: 'SELECTED', branchIds: [branchA] } },
  ] satisfies Partial<QueryScope>[])('separates different admitted context %j', (change) => {
    expect(queryKey({ ...scope, ...change }, resource)).not.toEqual(queryKey(scope, resource));
  });

  it.each([{ owner: 'commerce' }, { resource: 'deals' }, { id: branchA }, { filters: { state: 'new' } }, { cursor: 'next' }] satisfies Partial<QueryResource>[])(
    'separates resource, state filters and pagination %j', (change) => {
      expect(queryKey(scope, { ...resource, ...change })).not.toEqual(queryKey(scope, resource));
    });

  it('canonicalizes branch sets and filter order without retaining mutable input', () => {
    const branches = [branchB, branchA, branchA.toUpperCase()];
    const filters = { states: ['new'], nested: { z: 2, a: 1 } };
    const key = queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds: branches } }, { ...resource, filters });
    expect(key).toEqual(queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds: [branchA, branchB] } },
      { ...resource, filters: { nested: { a: 1, z: 2 }, states: ['new'] } }));
    branches.push(userId);
    filters.states.push('lost');
    expect(key[4]).toEqual([branchA, branchB]);
    expect(key[8]).toEqual({ nested: { a: 1, z: 2 }, states: ['new'] });
    expect(Object.isFrozen(key)).toBe(true);
    expect(Object.isFrozen(key[4])).toBe(true);
    expect(Object.isFrozen(key[8])).toBe(true);
  });

  it.each([
    { userId: 'demo-user' }, { companyId: 'demo-company' }, { contextRevision: '-1' },
    { branchScope: { mode: 'ALL', branchIds: [branchA] } }, { branchScope: { mode: 'SELECTED', branchIds: [] } },
    { companyId: null, branchScope: { mode: 'SELECTED', branchIds: [branchA] } },
    { branchScope: { mode: 'SELECTED', branchIds: ['demo-branch'] } },
  ] satisfies Partial<QueryScope>[])('rejects malformed scope %j', (change) => {
    expect(() => queryKey({ ...scope, ...change }, resource)).toThrow();
  });

  it('rejects non-JSON filters instead of producing colliding cache keys', () => {
    for (const value of [NaN, Infinity, undefined, new Date(), new Array(1)]) {
      expect(() => queryKey(scope, { ...resource, filters: { value } } as unknown as QueryResource)).toThrow();
    }
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    expect(() => queryKey(scope, { ...resource, filters: cyclic } as unknown as QueryResource)).toThrow();
  });

  it('rejects wholly sparse, mixed and inherited branch slots before key creation', () => {
    const whollySparse = new Array<string>(1);
    const mixed = [branchA, branchB];
    delete mixed[1];
    const inherited = new Array<string>(1);
    Object.setPrototypeOf(inherited, Object.assign(Object.create(Array.prototype) as object, { 0: branchA }));
    for (const branchIds of [whollySparse, mixed, inherited]) {
      expect(() => queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds } }, resource)).toThrow('Invalid branch scope');
    }
  });

  it.each(['0', '1', '9223372036854775807'])('preserves exact string revision %s', (revision) => {
    expect(queryKey({ ...scope, contextRevision: revision }, resource)[2]).toBe(revision);
  });
});
