// Existing transport characterization only; demonstrates a downstream handoff.
import assert from 'node:assert/strict';
import { createApiClient } from '../../../../../web/packages/api/src/client.ts';

const results = [];
for (const status of [200, 401, 429]) {
  const receipt = { error: { code: status === 429 ? 'RATE_LIMITED' : 'SESSION_REQUIRED',
    message: 'Synthetic error', fields: {}, traceId: '10000000-0000-4000-8000-000000000001' } };
  const client = createApiClient({ origin: 'https://fixture.invalid', fetch: async () => new Response(
    JSON.stringify(status === 200 ? { fixture: true } : receipt),
    { status, headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': 'fixture-only-csrf', 'Retry-After': '1' } },
  ) });
  const result = await client.request({ path: '/api/v1/identity/session', successStatuses: [200], schema: { parse: v => v } });
  assert.equal(result.status, status);
  assert.equal(JSON.stringify(result).includes('fixture-only-csrf'), false);
  assert.equal('headers' in result, false);
  assert.equal(result.kind, status === 200 ? 'success' : status === 401 ? 'http-error' : 'unexpected-status');
  results.push({ status, kind: result.kind, csrfAvailable: false, responseHeadersAvailable: false });
}
console.log(JSON.stringify({ outcome: 'PASS: reproduced existing transport limitation',
  reviewedSHA: 'fe3ffbfbe37d05261296a0ef5ba91cbec8edcead', results,
  requiredHandoff: 'Assign a narrow approved auth transport/header and 429 contract before browser auth wiring; no source change made.' }, null, 2));
