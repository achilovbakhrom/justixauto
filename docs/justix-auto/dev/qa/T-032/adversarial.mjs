import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { stripTypeScriptTypes } from 'node:module';
import { resolve } from 'node:path';
import { test } from 'node:test';

// Run from the exact reviewed worktree. Load its actual sources without editing them.
const asModule = (text) => `data:text/javascript;base64,${Buffer.from(stripTypeScriptTypes(text)).toString('base64')}`;
const clientURL = asModule(await readFile(resolve('web/packages/api/src/client.ts'), 'utf8'));
const { createApiClient } = await import(clientURL);
const keySource = (await readFile(resolve('web/packages/api/src/queryKeys.ts'), 'utf8')).replace("'./client'", JSON.stringify(clientURL));
const { queryKey } = await import(asModule(keySource));
const id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const other = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const origin = 'https://qa.justix.test';
const scope = { app: 'realization', userId: id, companyId: other, contextRevision: '9007199254740993', branchScope: { mode: 'ALL', branchIds: [] } };
const resource = { owner: 'retail', resource: 'leads' };
const req = { path: '/api/v1/retail/leads', schema: { parse: (v) => v }, successStatuses: [200] };
const error = { error: { code: 'DENIED', message: '', fields: {}, traceId: 'qa-synthetic' } };

for (const header of ['X-Permissions', 'X_Permissions', 'X-Justix-Internal-Caller', 'X_Justix_Internal_Caller']) {
  test(`known server-reserved identity header ${header} is rejected before dispatch`, async () => {
    let calls = 0;
    const client = createApiClient({ origin, fetch: async () => { calls++; return Response.json({}); } });
    const result = await client.request({ ...req, headers: { [header]: 'synthetic-spoof' } });
    assert.deepEqual(result, { kind: 'invalid-request' });
    assert.equal(calls, 0);
  });
}

test('SELECTED sparse branch array does not represent a valid nonempty UUID scope', () => {
  assert.throws(() => queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds: new Array(1) } }, resource));
});

test('known receipts require exact structure and never invoke success schema', async () => {
  for (const status of [400, 401, 403, 404, 409, 412, 422, 428, 503]) {
    const client = createApiClient({ origin, fetch: async () => Response.json({ ...error, operationId: id }, { status }) });
    const result = await client.request({ ...req, schema: { parse: () => { throw new Error('must not invoke'); } } });
    assert.deepEqual(result, { kind: 'http-error', status, receipt: { ...error, operationId: id } });
  }
});

test('unknown/malformed receipts never become authoritative HTTP failures', async () => {
  for (const value of [{ ...error, operationId: 1 }, { error: { ...error.error, secret: 'synthetic' } }, { error: { ...error.error, code: null } }]) {
    const client = createApiClient({ origin, fetch: async () => Response.json(value, { status: 403 }) });
    assert.deepEqual(await client.request(req), { kind: 'invalid-response', status: 403 });
  }
});

test('schema diagnostics do not escape through invalid-response', async () => {
  const client = createApiClient({ origin, fetch: async () => Response.json({}) });
  assert.deepEqual(await client.request({ ...req, schema: { parse: () => { throw new Error('synthetic-private-body'); } } }), { kind: 'invalid-response', status: 200 });
});

test('write timeout is unknown, has no retry, and independent identical calls stay explicit', async () => {
  let calls = 0;
  const client = createApiClient({ origin, fetch: async () => { calls++; throw new Error('lost reply after simulated commit'); } });
  for (let i = 0; i < 2; i++) {
    assert.deepEqual(await client.request({ ...req, method: 'POST', idempotencyKey: id, body: {} }), { kind: 'transport-error', outcome: 'unknown', aborted: false });
    assert.equal(calls, i + 1);
  }
});

test('mid-body connection failure on a write does not become a rejection receipt', async () => {
  const stream = new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode('{')); controller.error(new Error('interrupted')); } });
  const client = createApiClient({ origin, fetch: async () => new Response(stream, { headers: { 'Content-Type': 'application/json' } }) });
  assert.deepEqual(await client.request({ ...req, method: 'POST' }), { kind: 'invalid-response', status: 200 });
});

test('path encodings and authority substitution fail closed before dispatch', async () => {
  let calls = 0;
  const client = createApiClient({ origin, fetch: async () => { calls++; return Response.json({}); } });
  for (const path of ['/api/v1/retail/%2E%2e/admin', '/api/v1/retail/%5Cevil', '/api/v1/retail/%252f', '/api/v1/retail/%0a', '/api/v1/retail/%', '/api/v1/retail/a#x', '//evil.test/api/v1/retail/leads']) {
    assert.deepEqual(await client.request({ ...req, path }), { kind: 'invalid-request' });
  }
  assert.equal(calls, 0);
});

test('precise context headers, no-store, same-origin cookies and no redirects', async () => {
  const client = createApiClient({ origin, fetch: async (url, init) => {
    assert.equal(url.origin, origin);
    assert.equal(init.headers.get('X-Context-Revision'), scope.contextRevision);
    assert.equal(init.headers.get('If-Match'), '"9223372036854775807"');
    assert.equal(init.cache, 'no-store'); assert.equal(init.mode, 'same-origin');
    assert.equal(init.credentials, 'same-origin'); assert.equal(init.redirect, 'error');
    return Response.json({});
  } });
  assert.equal((await client.request({ ...req, contextRevision: scope.contextRevision, ifMatch: '9223372036854775807' })).kind, 'success');
});

test('all admitted identity/resource dimensions create distinct serialized keys', () => {
  const keys = [queryKey(scope, resource)];
  for (const change of [{ app: 'admin' }, { userId: other }, { companyId: id }, { contextRevision: '9007199254740994' }, { branchScope: { mode: 'SELECTED', branchIds: [id] } }]) keys.push(queryKey({ ...scope, ...change }, resource));
  for (const change of [{ owner: 'inventory' }, { resource: 'customers' }, { id }, { filters: { state: 'active' } }, { cursor: 'next' }]) keys.push(queryKey(scope, { ...resource, ...change }));
  assert.equal(new Set(keys.map((key) => JSON.stringify(key))).size, keys.length);
});

test('nested filters and branches have no mutable aliases and freeze deeply', () => {
  const filters = { nested: { list: [{ state: 'new' }] } };
  const branches = [id];
  const key = queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds: branches } }, { ...resource, filters });
  filters.nested.list[0].state = 'changed'; branches[0] = other;
  assert.equal(key[8].nested.list[0].state, 'new'); assert.deepEqual(key[4], [id]);
  assert.throws(() => { key[8].nested.list[0].state = 'changed'; }, TypeError);
  assert.ok(Object.isFrozen(key[8].nested.list));
});

test('JSON proto-named filters remain distinct own data without pollution', () => {
  const filters = JSON.parse('{"__proto__":{"flag":"fixture"},"constructor":"fixture"}');
  const key = queryKey(scope, { ...resource, filters });
  assert.ok(Object.hasOwn(key[8], '__proto__')); assert.equal({}.flag, undefined);
  assert.notEqual(JSON.stringify(key), JSON.stringify(queryKey(scope, resource)));
});
