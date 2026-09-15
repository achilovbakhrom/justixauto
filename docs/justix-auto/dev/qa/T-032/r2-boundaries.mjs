import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { stripTypeScriptTypes } from 'node:module';
import { resolve } from 'node:path';
import { test } from 'node:test';

const moduleURL = (source) => `data:text/javascript;base64,${Buffer.from(stripTypeScriptTypes(source)).toString('base64')}`;
const clientURL = moduleURL(await readFile(resolve('web/packages/api/src/client.ts'), 'utf8'));
const { createApiClient } = await import(clientURL);
const { queryKey } = await import(moduleURL((await readFile(resolve('web/packages/api/src/queryKeys.ts'), 'utf8')).replace("'./client'", JSON.stringify(clientURL))));
const id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const other = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const scope = { app: 'realization', userId: id, companyId: other, contextRevision: '1', branchScope: { mode: 'SELECTED', branchIds: [id] } };
const resource = { owner: 'retail', resource: 'leads' };
const request = { path: '/api/v1/retail/leads', schema: { parse: (v) => v }, successStatuses: [200] };

test('all existing Go reserved identity names and normalized duplicate headers are denied', async () => {
  let calls = 0;
  const client = createApiClient({ origin: 'https://qa.justix.test', fetch: async () => { calls++; return Response.json({}); } });
  for (const name of ['x-actor', 'x-actor-id', 'x-actor-kind', 'x-permission', 'x-permissions', 'x-internal-caller', 'x-justix-internal-caller', 'x-justix-internal-purpose']) {
    for (const header of [name, name.toUpperCase(), name.replaceAll('-', '_')]) {
      assert.deepEqual(await client.request({ ...request, headers: [[header, 'first'], [header.toLowerCase(), 'second']] }), { kind: 'invalid-request' });
    }
  }
  assert.equal(calls, 0);
});

test('fix preserves feature-owned headers and exact public context metadata', async () => {
  const client = createApiClient({ origin: 'https://qa.justix.test', fetch: async (_, init) => {
    assert.equal(init.headers.get('X-Feature-Challenge'), 'synthetic-ephemeral');
    assert.equal(init.headers.get('X-Context-Revision'), '7');
    return Response.json({});
  } });
  assert.equal((await client.request({ ...request, contextRevision: '7', headers: { 'X-Feature-Challenge': 'synthetic-ephemeral' } })).kind, 'success');
});

test('holes at either end, in the middle, and inherited array slots fail closed', () => {
  const inherited = new Array(1);
  Object.setPrototypeOf(inherited, Object.assign(Object.create(Array.prototype), { 0: id }));
  for (const branchIds of [new Array(1), [, id], [id, , other], [id, ,], inherited]) {
    assert.throws(() => queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds } }, resource), /Invalid branch scope/);
  }
});

test('valid dense duplicate sets still canonicalize independently from ALL', () => {
  const branches = [other.toUpperCase(), id, other];
  const selected = queryKey({ ...scope, branchScope: { mode: 'SELECTED', branchIds: branches } }, resource);
  assert.deepEqual(selected[4], [id, other]);
  branches.fill(id);
  assert.deepEqual(selected[4], [id, other]);
  const all = queryKey({ ...scope, branchScope: { mode: 'ALL', branchIds: [] } }, resource);
  assert.notEqual(JSON.stringify(selected), JSON.stringify(all));
  assert.ok(Object.isFrozen(selected[4]));
});
