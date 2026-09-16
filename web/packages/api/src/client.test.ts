import { describe, expect, it, vi } from 'vitest';
import { runInNewContext } from 'node:vm';
import { createApiClient, errorStatuses, isRevision } from './client';
import type { ApiRequest, AuthExchange, ErrorReceipt, ErrorStatus, HeaderRule, ResponseContract, Schema, SuccessStatus } from './client';
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

  it('an already aborted write is rejected locally before dispatch', async () => {
    const { client, fetcher } = setup();
    const controller = new AbortController();
    controller.abort();
    fetcher.mockRejectedValue(new Error('aborted'));
    expect(await client.request({ ...request, method: 'POST', signal: controller.signal })).toEqual({ kind: 'invalid-request' });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('does not retain responses between requests', async () => {
    const { client, fetcher } = setup();
    fetcher.mockResolvedValueOnce(Response.json({ revision: '1' })).mockResolvedValueOnce(Response.json({ revision: '2' }));
    expect(await client.request(request)).toMatchObject({ data: { revision: '1' } });
    expect(await client.request(request)).toMatchObject({ data: { revision: '2' } });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});

const token = 'synthetic_CSRF-123';
const forbidden: HeaderRule = { csrf: 'forbidden', retryAfter: 'forbidden' };
const rotated: HeaderRule = { csrf: 'required', retryAfter: 'forbidden' };
const limited: HeaderRule = { csrf: 'forbidden', retryAfter: 'required' };
function endpointError(code: string): Schema<ErrorReceipt> {
  return { parse(value) {
    const receipt = value as ErrorReceipt;
    if (receipt?.error?.code !== code || 'operationId' in receipt) throw new Error('Synthetic endpoint mismatch');
    return receipt;
  } };
}
function authCase(status: SuccessStatus | ErrorStatus = 200, rule: HeaderRule = rotated) {
  const success = status < 400;
  const code = status === 429 ? 'RATE_LIMITED' : status === 401 ? 'SESSION_REQUIRED' : 'DENIED';
  const body = success ? { revision: '1' } : { error: { ...errorReceipt.error, code } };
  const contract: ResponseContract = {
    numeric: 'safe-integers', auth: true,
    errors: success ? {} : { [status]: endpointError(code) },
    metadata: { [status]: rule, ...(!success ? { 200: rotated } : {}) },
  };
  const prepare = vi.fn(() => ({ csrf: token }));
  const accept = vi.fn<AuthExchange<{ revision: string } | undefined>['accept']>(() => true);
  const input: ApiRequest<{ revision: string } | undefined> = {
    path: '/api/v1/identity/session', method: 'GET', successStatuses: success ? [status as SuccessStatus] : [200],
    schema: status === 204 ? { parse(value) { if (value !== undefined) throw new Error('Expected no body'); return undefined; } } : schema,
    responseContract: contract, authExchange: { prepare, accept },
  };
  const headers = new Headers({ 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
  if (rule.csrf === 'required') headers.set('X-CSRF-Token', token);
  if (rule.retryAfter === 'required') headers.set('Retry-After', '12');
  const response = () => new Response(status === 204 ? null : JSON.stringify(body), { status, headers });
  return { input, contract, body, headers, response, prepare, accept };
}

describe('exact response contracts and ephemeral auth acceptance', () => {
  it('advertises a captured immutable capability and resource settings', async () => {
    const options = { origin, maxResponseBytes: 16, maxJSONDepth: 1, fetch: vi.fn<typeof fetch>().mockResolvedValue(Response.json('longer than sixteen bytes')) };
    const client = createApiClient(options);
    options.maxResponseBytes = 1000;
    options.maxJSONDepth = 1000;
    expect(client.responseContractVersion).toBe(1);
    expect(Object.isFrozen(client)).toBe(true);
    expect(await client.request({ ...request, schema: { parse: (v) => v } })).toEqual({ kind: 'invalid-response', status: 200 });
    for (const maxResponseBytes of [0, -1, 1.2, Infinity, NaN]) {
      expect(() => createApiClient({ origin, maxResponseBytes })).toThrow();
      expect(() => createApiClient({ origin, maxJSONDepth: maxResponseBytes })).toThrow();
    }
  });

  it.each([
    ['session full/restricted', 200, rotated], ['session anonymous', 401, rotated],
    ['login', 200, rotated], ['MFA verify', 200, rotated], ['revoke all', 200, rotated],
    ['enrollment confirm', 200, rotated], ['logout', 204, forbidden], ['recovery complete', 204, forbidden],
    ['MFA challenge', 200, forbidden], ['MFA enrollment', 200, forbidden], ['recovery request', 202, forbidden],
    ['rate limit', 429, limited], ['other declared error', 403, forbidden],
  ] as const)('accepts the declared %s metadata cell only after its body', async (_, status, rule) => {
    const test = authCase(status, rule);
    const { client, fetcher } = setup(test.response());
    const result = await client.request(test.input);
    expect(result.kind).toBe(status < 400 ? 'success' : 'http-error');
    expect(test.prepare).toHaveBeenCalledTimes(1);
    expect(test.accept).toHaveBeenCalledExactlyOnceWith(result, {
      ...(rule.csrf === 'required' ? { csrf: token } : {}),
      ...(rule.retryAfter === 'required' ? { retryAfterSeconds: 12 } : {}),
    });
    expect(JSON.stringify(result)).not.toContain(token);
    expect(JSON.stringify(result)).not.toContain('retryAfter');
    expect(new Headers(fetcher.mock.calls[0]![1]?.headers).has('X-CSRF-Token')).toBe(false);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('sends only the prepared unsafe token and validates errors before metadata acceptance', async () => {
    const test = authCase(401);
    const { client, fetcher } = setup(Response.json(errorReceipt, { status: 401, headers: test.headers }));
    expect(await client.request({ ...test.input, method: 'POST', body: {} })).toEqual({ kind: 'invalid-response', status: 401 });
    expect(new Headers(fetcher.mock.calls[0]![1]?.headers).get('X-CSRF-Token')).toBe(token);
    expect(test.accept).not.toHaveBeenCalled();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it.each(['', 'a,b', 'a b', 'a=', 'é', 'a'.repeat(4097)])('rejects observable malformed CSRF %j', async (csrf) => {
    const test = authCase();
    test.headers.set('X-CSRF-Token', csrf);
    expect(await setup(test.response()).client.request(test.input)).toEqual({ kind: 'invalid-response', status: 200 });
    expect(test.accept).not.toHaveBeenCalled();
  });

  it.each(['csrf absent', 'csrf forbidden', 'retry forbidden', 'cache absent', 'cache list', 'cache duplicate', 'media extra', 'media duplicate', 'media nonutf8'])('rejects %s without accepting metadata', async (fault) => {
    const test = authCase(200, fault === 'csrf forbidden' ? forbidden : rotated);
    switch (fault) {
      case 'csrf absent': test.headers.delete('X-CSRF-Token'); break;
      case 'csrf forbidden': test.headers.set('X-CSRF-Token', token); break;
      case 'retry forbidden': test.headers.set('Retry-After', '0'); break;
      case 'cache absent': test.headers.delete('Cache-Control'); break;
      case 'cache list': test.headers.set('Cache-Control', 'no-store, private'); break;
      case 'cache duplicate': test.headers.append('Cache-Control', 'no-store'); break;
      case 'media extra': test.headers.set('Content-Type', 'application/json; profile=auth'); break;
      case 'media duplicate': test.headers.set('Content-Type', 'application/json; charset=utf-8; charset=utf-8'); break;
      case 'media nonutf8': test.headers.set('Content-Type', 'application/json; charset=ascii'); break;
    }
    expect(await setup(test.response()).client.request(test.input)).toEqual({ kind: 'invalid-response', status: 200 });
    expect(test.accept).not.toHaveBeenCalled();
  });

  it.each([null, '', '01', '-1', '+1', '1.0', '1, 2', '2147483648', '12345678901', 'Wed, 21 Oct 2015 07:28:00 GMT'])('rejects malformed Retry-After %j', async (value) => {
    const test = authCase(429, limited);
    if (value === null) test.headers.delete('Retry-After'); else test.headers.set('Retry-After', value);
    expect(await setup(test.response()).client.request(test.input)).toEqual({ kind: 'invalid-response', status: 429 });
    expect(test.accept).not.toHaveBeenCalled();
  });

  it.each(['0', '2147483647'])('accepts delay boundary %s with normalized media/cache casing', async (delay) => {
    const test = authCase(429, limited);
    test.headers.set('Retry-After', delay);
    test.headers.set('Content-Type', 'Application/JSON; Charset=UTF-8');
    test.headers.set('Cache-Control', 'No-Store');
    expect((await setup(test.response()).client.request(test.input)).kind).toBe('http-error');
    expect(test.accept.mock.calls[0]?.[1]).toEqual({ retryAfterSeconds: Number(delay) });
  });

  it.each(['no exchange', 'no contract', 'non-auth exchange', 'numeric', 'missing metadata', 'extra metadata', 'unknown directive', 'unknown status', 'unknown contract field', 'missing error parser', 'duplicate success', '429 rules', '204 rules'])('rejects invalid binding/configuration %s before prepare/fetch', async (fault) => {
    const test = authCase();
    let input = test.input;
    switch (fault) {
      case 'no exchange': { const { authExchange, ...rest } = input; void authExchange; input = rest; break; }
      case 'no contract': { const { responseContract, ...rest } = input; void responseContract; input = rest; break; }
      case 'non-auth exchange': input = { ...input, responseContract: { ...test.contract, auth: false } }; break;
      case 'numeric': input = { ...input, responseContract: { ...test.contract, numeric: 'finite-json' } as unknown as ResponseContract }; break;
      case 'missing metadata': input = { ...input, responseContract: { ...test.contract, metadata: {} } }; break;
      case 'extra metadata': input = { ...input, responseContract: { ...test.contract, metadata: { 200: rotated, 201: rotated } } }; break;
      case 'unknown directive': input = { ...input, responseContract: { ...test.contract, metadata: { 200: { ...rotated, cookie: 'required' } } } as ResponseContract }; break;
      case 'unknown status': input = { ...input, responseContract: { ...test.contract, errors: { 500: endpointError('DENIED') } } as ResponseContract }; break;
      case 'unknown contract field': input = { ...input, responseContract: { ...test.contract, extra: true } as ResponseContract }; break;
      case 'missing error parser': input = { ...input, responseContract: { ...test.contract, errors: { 401: {} as Schema<ErrorReceipt> }, metadata: { 200: rotated, 401: rotated } } }; break;
      case 'duplicate success': input = { ...input, successStatuses: [200, 200] }; break;
      case '429 rules': input = { ...input, responseContract: { ...test.contract, errors: { 429: endpointError('RATE_LIMITED') }, metadata: { 200: rotated, 429: rotated } } }; break;
      case '204 rules': input = { ...input, successStatuses: [204], responseContract: { ...test.contract, metadata: { 204: rotated } } }; break;
    }
    const { client, fetcher } = setup(test.response());
    expect(await client.request(input)).toEqual({ kind: 'invalid-request' });
    expect(test.prepare).not.toHaveBeenCalled();
    expect(test.accept).not.toHaveBeenCalled();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it.each(['X-CSRF-Token', 'x_csrf_token', 'X_Csrf-Token'])('rejects a competing token alias %s before prepare', async (name) => {
    const test = authCase();
    const { client, fetcher } = setup(test.response());
    expect(await client.request({ ...test.input, headers: { [name]: token }, method: 'POST' })).toEqual({ kind: 'invalid-request' });
    expect(test.prepare).not.toHaveBeenCalled();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it.each([undefined, {}, { csrf: '' }, { csrf: 'a,b' }, { csrf: token, extra: true }])('rejects unavailable or invalid unsafe preparation %j', async (prepared) => {
    const test = authCase();
    const { client, fetcher } = setup(test.response());
    expect(await client.request({ ...test.input, method: 'POST', authExchange: { prepare: () => prepared, accept: test.accept } })).toEqual({ kind: 'invalid-request' });
    expect(fetcher).not.toHaveBeenCalled();
    expect(test.accept).not.toHaveBeenCalled();
  });

  it('permits safe empty preparation and never falls back to a generic undeclared error', async () => {
    const test = authCase();
    const { client, fetcher } = setup(Response.json(errorReceipt, { status: 403, headers: test.headers }));
    expect(await client.request({ ...test.input, authExchange: { prepare: () => ({}), accept: test.accept } })).toEqual({ kind: 'unexpected-status', status: 403 });
    expect(test.accept).not.toHaveBeenCalled();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('captures schema/status/header rules before asynchronous IO', async () => {
    const test = authCase();
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => {
      (test.contract.metadata as Record<number, HeaderRule>)[200] = forbidden;
      (test.input.successStatuses as number[])[0] = 201;
      test.input.schema.parse = () => { throw new Error('replaced'); };
      return test.response();
    });
    const original = schema.parse;
    try {
      expect((await createApiClient({ origin, fetch: fetcher }).request(test.input)).kind).toBe('success');
      expect(test.accept).toHaveBeenCalledTimes(1);
    } finally { schema.parse = original; }
  });

  it.each([false, undefined, 1, {}, 'true'])('requires exactly true from current-ticket sink, rejecting %j', async (outcome) => {
    const test = authCase();
    const accept = vi.fn(() => outcome) as unknown as AuthExchange<{ revision: string } | undefined>['accept'];
    expect(await setup(test.response()).client.request({ ...test.input, authExchange: { prepare: test.prepare, accept } })).toEqual({ kind: 'invalid-response', status: 200 });
    expect(accept).toHaveBeenCalledTimes(1);
  });

  it('rejects a stale captured ticket and a throwing sink without a transport-owned reset', async () => {
    let epoch = 1;
    let acceptedRevision = 'newer';
    const test = authCase();
    const captured = epoch;
    const accept = vi.fn(() => { if (captured !== epoch) return false; acceptedRevision = 'stale'; return true; });
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => { epoch++; return test.response(); });
    expect(await createApiClient({ origin, fetch: fetcher }).request({ ...test.input, authExchange: { prepare: test.prepare, accept } })).toEqual({ kind: 'invalid-response', status: 200 });
    expect(acceptedRevision).toBe('newer');
    const throwing = () => { throw new Error(token); };
    expect(await setup(test.response()).client.request({ ...test.input, authExchange: { prepare: test.prepare, accept: throwing } })).toEqual({ kind: 'invalid-response', status: 200 });
  });

  it('supports class method receivers and ordinary then/constructor DTO properties', async () => {
    class Parser implements Schema<unknown> {
      expected = 'data';
      parse(value: unknown) {
        expect((value as Record<string, unknown>).then).toBe(this.expected);
        return value;
      }
    }
    class Exchange implements AuthExchange<unknown> {
      calls = 0;
      prepare() { this.calls++; return {}; }
      accept() { this.calls++; return true; }
    }
    const test = authCase();
    const exchange = new Exchange();
    const response = new Response('{"then":"data","constructor":{"then":1},"__proto__":false}', { headers: test.headers });
    const result = await setup(response).client.request({ ...test.input, schema: new Parser(), authExchange: exchange });
    expect(result).toEqual({ kind: 'success', status: 200, data: { then: 'data', constructor: { then: 1 }, ['__proto__']: false } });
    expect(exchange.calls).toBe(2);
  });

  it.each(['prepare', 'accept', 'success schema', 'error schema'] as const)('rejects returned/thrown async values in %s without invoking thenable accessors or leaking rejections', async (hook) => {
    const unhandled = vi.fn();
    process.on('unhandledRejection', unhandled);
    let getterCalls = 0;
    let thenCalls = 0;
    const factories = [
      () => Promise.resolve(true), () => Promise.reject(new Error(token)),
      () => (async () => true)(),
      () => runInNewContext('Promise.reject(new Error("fixture-only"))') as unknown,
      () => ({ then() { thenCalls++; } }),
      () => Object.defineProperty({}, 'then', { get() { getterCalls++; throw new Error(token); } }),
    ];
    try {
      for (const make of factories) {
        for (const thrown of [false, true]) {
          const invalid = () => { const result = make(); if (thrown) throw result; return result; };
          const test = authCase(hook === 'error schema' ? 401 : 200);
          let input = test.input;
          if (hook === 'prepare') input = { ...input, authExchange: { prepare: invalid as AuthExchange<undefined>['prepare'], accept: test.accept } };
          if (hook === 'accept') input = { ...input, authExchange: { prepare: test.prepare, accept: invalid as AuthExchange<undefined>['accept'] } };
          if (hook === 'success schema') input = { ...input, schema: { parse: invalid } as Schema<{ revision: string }> };
          if (hook === 'error schema') input = { ...input, responseContract: { ...test.contract, errors: { 401: { parse: invalid } as Schema<ErrorReceipt> } } };
          const { client, fetcher } = setup(test.response());
          expect(await client.request(input)).toEqual(hook === 'prepare'
            ? { kind: 'invalid-request' } : { kind: 'invalid-response', status: hook === 'error schema' ? 401 : 200 });
          expect(fetcher).toHaveBeenCalledTimes(hook === 'prepare' ? 0 : 1);
          expect(test.accept).not.toHaveBeenCalled();
        }
      }
      await new Promise((resolve) => setTimeout(resolve, 0));
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(unhandled).not.toHaveBeenCalled();
      expect(getterCalls).toBe(0);
      expect(thenCalls).toBe(0);
    } finally { process.off('unhandledRejection', unhandled); }
  });

  it.each(['success parse', 'error parse', 'prepare', 'accept'] as const)('disposes native Promise returns from the %s getter before fixed failure', async (hook) => {
    const unhandled = vi.fn();
    const thenGetter = vi.fn(() => { throw new Error(token); });
    const thenCall = vi.fn();
    const returns = [
      () => Promise.reject(new Error(token)),
      () => runInNewContext('Promise.reject(new Error("fixture-only"))') as unknown,
      () => Promise.resolve(() => true),
      () => runInNewContext('Promise.resolve(() => true)') as unknown,
      () => Object.defineProperty({}, 'then', { get: thenGetter }),
      () => ({ then: thenCall }),
    ];
    process.on('unhandledRejection', unhandled);
    try {
      for (const result of returns) {
        const test = authCase(401);
        const getter = vi.fn(result);
        const receiver = hook === 'success parse' ? { parse: schema.parse }
          : hook === 'error parse' ? { parse: endpointError('SESSION_REQUIRED').parse }
            : test.input.authExchange!;
        Object.defineProperty(receiver, hook.endsWith('parse') ? 'parse' : hook, { get: getter });
        const input = hook === 'success parse' ? { ...test.input, schema: receiver as Schema<{ revision: string }> }
          : hook === 'error parse' ? { ...test.input, responseContract: { ...test.contract, errors: { 401: receiver as Schema<ErrorReceipt> } } }
            : test.input;
        const { client, fetcher } = setup(test.response());
        expect(await client.request(input)).toEqual({ kind: 'invalid-request' });
        expect(getter).toHaveBeenCalledTimes(1);
        expect(fetcher).not.toHaveBeenCalled();
        expect(test.prepare).not.toHaveBeenCalled();
        expect(test.accept).not.toHaveBeenCalled();
        await new Promise((resolve) => setTimeout(resolve, 0));
        await new Promise((resolve) => setTimeout(resolve, 0));
        expect(unhandled).not.toHaveBeenCalled();
      }
      expect(thenGetter).not.toHaveBeenCalled();
      expect(thenCall).not.toHaveBeenCalled();
    } finally { process.off('unhandledRejection', unhandled); }
  });

  it('captures callable getters once with receivers; a throwing getter fails before dispatch', async () => {
    const test = authCase();
    let calls = 0;
    const parser = { revision: '1', get parse() { calls++; return function (this: { revision: string }, value: unknown) { expect(value).toEqual({ revision: this.revision }); return value; }; } };
    const exchange = { token, get prepare() { calls++; return function (this: { token: string }) { return { csrf: this.token }; }; }, get accept() { calls++; return () => true; } };
    expect((await setup(test.response()).client.request({ ...test.input, schema: parser, authExchange: exchange })).kind).toBe('success');
    expect(calls).toBe(3);
  });

  it('rejects throwing method getters and untrusted configuration accessors before fetch', async () => {
    const getter = vi.fn(() => { throw new Error(token); });
    const test = authCase();
    for (const input of [
      { ...test.input, authExchange: Object.defineProperty({ accept: test.accept }, 'prepare', { get: getter }) as unknown as AuthExchange<undefined> },
      { ...test.input, schema: Object.defineProperty({}, 'parse', { get: getter }) as Schema<undefined> },
      { ...test.input, responseContract: Object.defineProperty({ ...test.contract }, 'metadata', { get: getter }) },
    ]) {
      const { client, fetcher } = setup(test.response());
      expect(await client.request(input)).toEqual({ kind: 'invalid-request' });
      expect(fetcher).not.toHaveBeenCalled();
    }
    expect(getter).toHaveBeenCalledTimes(2);
  });
});

describe('one bounded raw-byte Fetch response path', () => {
  const encoder = new TextEncoder();
  function streamed(chunks: Uint8Array[], headers: HeadersInit = { 'Content-Type': 'application/json' }) {
    const cancel = vi.fn();
    let index = 0;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) { if (index === chunks.length) controller.close(); else controller.enqueue(chunks[index++]!); }, cancel,
    });
    return { response: new Response(body, { headers }), cancel };
  }
  it('counts decoded bytes across Unicode splits without trusting Content-Length', async () => {
    const bytes = encoder.encode('{"text":"😀"}');
    const chunks = [...bytes].map((value) => new Uint8Array([value]));
    const { response } = streamed(chunks, { 'Content-Type': 'application/json; legacy=permitted', 'Content-Length': '9999999999999999999999' });
    const json = vi.spyOn(response, 'json');
    const client = createApiClient({ origin, maxResponseBytes: bytes.length, fetch: vi.fn<typeof fetch>().mockResolvedValue(response) });
    expect(await client.request({ ...request, schema: { parse: (value) => value } })).toEqual({ kind: 'success', status: 200, data: { text: '😀' } });
    expect(json).not.toHaveBeenCalled();
  });

  it('cancels immediately on streamed overflow even when cancellation never settles', async () => {
    const test = authCase();
    const cancel = vi.fn(() => new Promise<void>(() => { /* deliberately noncooperative producer */ }));
    const response = new Response(new ReadableStream({
      start(controller) { controller.enqueue(encoder.encode('12345')); }, cancel,
    }), { headers: { ...Object.fromEntries(test.headers), 'Content-Length': '1' } });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response);
    expect(await createApiClient({ origin, fetch: fetcher, maxResponseBytes: 4 }).request(test.input)).toEqual({ kind: 'invalid-response', status: 200 });
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(test.accept).not.toHaveBeenCalled();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('accepts actual cross-realm byte chunks but rejects other typed arrays and spoofed byte tags', async () => {
    const foreign = runInNewContext('new Uint8Array([49])') as Uint8Array;
    expect(foreign instanceof Uint8Array).toBe(false);
    for (const [chunk, valid] of [[foreign, true], [new Uint16Array([49]), false],
      [{ [Symbol.toStringTag]: 'Uint8Array', byteLength: 1, length: 1, 0: 49 }, false]] as const) {
      const { response } = streamed([chunk as Uint8Array]);
      const result = await setup(response).client.request({ ...request, schema: { parse: (value) => value } });
      expect(result.kind).toBe(valid ? 'success' : 'invalid-response');
    }
  });

  it.each([
    encoder.encode('{"revision":"0","revision":"1"}'),
    encoder.encode('{"nested":{"x":1,"\\u0078":2}}'),
    new Uint8Array([0x22, 0xff, 0x22]), encoder.encode('\ufeff{}'),
    encoder.encode('"\\ud800"'), encoder.encode('{"\\udfff":1}'), encoder.encode('{} false'),
  ])('rejects malformed raw bytes before the schema or sink', async (bytes) => {
    const test = authCase();
    const parse = vi.fn((value: unknown) => value);
    const { response } = streamed([bytes], test.headers);
    expect(await setup(response).client.request({ ...test.input, schema: { parse } })).toEqual({ kind: 'invalid-response', status: 200 });
    expect(parse).not.toHaveBeenCalled();
    expect(test.accept).not.toHaveBeenCalled();
  });

  it.each(['1.25', '1e-324', '1.0000000000000001', '9007199254740992'])('preserves finite legacy behavior but applies requested exact safe-integer profile to %s', async (source) => {
    const plain = () => new Response(source, { headers: { 'Content-Type': 'application/json' } });
    const identity = { parse: (value: unknown) => value };
    expect((await setup(plain()).client.request({ ...request, schema: identity })).kind).toBe('success');
    const contract: ResponseContract = { auth: false, numeric: 'safe-integers', errors: {}, metadata: { 200: forbidden } };
    expect(await setup(plain()).client.request({ ...request, schema: identity, responseContract: contract })).toEqual({ kind: 'invalid-response', status: 200 });
  });

  it.each(['1.0', '1e0', '10e-1', '-0', '0e9999999', 'false', 'null', '"9007199254740992"'])('accepts exact profile values without schema coercion: %s', async (source) => {
    const test = authCase();
    const { response } = streamed([encoder.encode(source)], test.headers);
    const result = await setup(response).client.request({ ...test.input, schema: { parse: (value) => value } });
    expect(result.kind).toBe('success');
    expect(test.accept).toHaveBeenCalledTimes(1);
    if (result.kind === 'success' && source.startsWith('"')) expect(typeof result.data).toBe('string');
  });

  it('checks depth and does not deliver a partial response on stream failure', async () => {
    for (const body of [new ReadableStream<Uint8Array>({ start(c) { c.enqueue(encoder.encode('{}')); c.error(new Error(token)); } }),
      encoder.encode('[[0]]')]) {
      const test = authCase();
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(body, { headers: test.headers }));
      expect(await createApiClient({ origin, fetch: fetcher, maxJSONDepth: 1 }).request(test.input)).toEqual({ kind: 'invalid-response', status: 200 });
      expect(test.accept).not.toHaveBeenCalled();
    }
  });

  it('rejects observable bytes on 204 before an empty schema/sink', async () => {
    const test = authCase(204, forbidden);
    const response = new Response('illegal', { headers: test.headers });
    Object.defineProperty(response, 'status', { value: 204 });
    const parse = vi.fn(() => undefined);
    expect(await setup(response).client.request({ ...test.input, schema: { parse } })).toEqual({ kind: 'invalid-response', status: 204 });
    expect(parse).not.toHaveBeenCalled();
    expect(test.accept).not.toHaveBeenCalled();
  });

  it('cancels an in-flight body on abort and leaves a dispatched unsafe outcome unknown', async () => {
    const test = authCase();
    const controller = new AbortController();
    const cancel = vi.fn();
    const response = new Response(new ReadableStream({ cancel }), { headers: test.headers });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response);
    const pending = createApiClient({ origin, fetch: fetcher }).request({ ...test.input, method: 'POST', signal: controller.signal });
    await new Promise((resolve) => setTimeout(resolve, 0));
    controller.abort();
    expect(await pending).toEqual({ kind: 'invalid-response', status: 200 });
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(test.accept).not.toHaveBeenCalled();
  });

  it('reports a dispatched fetch rejection after abort as an unknown write, never a local rejection', async () => {
    const test = authCase();
    const controller = new AbortController();
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => { controller.abort(); throw new Error(token); });
    expect(await createApiClient({ origin, fetch: fetcher }).request({ ...test.input, method: 'POST', signal: controller.signal })).toEqual({ kind: 'transport-error', outcome: 'unknown', aborted: true });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(test.accept).not.toHaveBeenCalled();
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
