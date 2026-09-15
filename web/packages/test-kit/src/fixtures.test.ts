import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { createElement, useState } from 'react';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Button } from '@justixauto/ui/Button';
import { createApiClient, isRevision, type Schema } from '@justixauto/api';
import { checkedFixture, createFixtureServer, jsonHandler } from './fixtures';

// Deliberately test-only DTO/endpoint, not a proposed owner contract.
interface SyntheticReceipt { label: string; revision: string }
const receiptSchema: Schema<SyntheticReceipt> = {
  parse(value) {
    if (!value || typeof value !== 'object' || Array.isArray(value)
      || Object.keys(value).sort().join(',') !== 'label,revision'
      || !('label' in value) || typeof value.label !== 'string'
      || !('revision' in value) || !isRevision(value.revision)) throw new Error('Private diagnostic');
    return value as SyntheticReceipt;
  },
};
const emptyRequest: Schema<null> = { parse(value) {
  if (value !== null) throw new Error('Expected no body');
  return null;
} };
const identity: Schema<unknown> = { parse: (value) => value };
const url = 'http://localhost/api/v1/identity/synthetic-test-receipt';
const body = { label: 'Synthetic ready', revision: '9223372036854775807' };
const baseline = () => jsonHandler({
  method: 'GET', url, request: { schema: emptyRequest, read: () => null },
  response: { status: 200, schema: receiptSchema, body },
});
const server = createFixtureServer(baseline());
beforeAll(() => { server.listen(); });
afterEach(() => { cleanup(); server.reset(); });
afterAll(() => { server.close(); });

describe('schema-checked wire fixtures', () => {
  it('copies and freezes nested values without changing revision precision', () => {
    const input = { rows: [body], scope: { branchIds: ['synthetic-branch'] } };
    const fixture = checkedFixture(identity, input) as typeof input;
    input.rows[0] = { label: 'Changed', revision: '1' };
    expect(fixture.rows[0]).toEqual(body);
    expect(Object.isFrozen(fixture.rows)).toBe(true);
    expect(Object.isFrozen(fixture.rows[0])).toBe(true);
    expect(() => { fixture.scope.branchIds.push('other'); }).toThrow();
  });

  it.each([
    undefined, Number.NaN, Number.POSITIVE_INFINITY, -0, 1n, new Date(),
    () => 1, Symbol('fixture'), { missing: undefined }, [undefined],
    Array(1), Object.assign([], { extra: true }),
    Object.defineProperty({}, 'secret', { enumerable: true, get: () => 'value' }),
    Object.defineProperty({}, 'hidden', { value: 1 }), { [Symbol('hidden')]: 1 },
  ])('rejects values that JSON silently loses or changes: %s', (value) => {
    expect(() => checkedFixture(identity, value)).toThrow('Invalid synthetic JSON fixture');
  });

  it('rejects inherited array slots and cycles but permits repeated references', () => {
    const hole = Array(1);
    Object.setPrototypeOf(hole, { 0: 'inherited' });
    expect(() => checkedFixture(identity, hole)).toThrow();
    const cycle: { self?: unknown } = {};
    cycle.self = cycle;
    expect(() => checkedFixture(identity, cycle)).toThrow();
    expect(checkedFixture(identity, [body, body])).toEqual([body, body]);
  });

  it.each([
    { label: 'Synthetic', revision: 1 },
    { label: 'Synthetic', revision: '01' },
    { label: 'Synthetic', revision: '9223372036854775808' },
    { label: 'Synthetic', revision: '1', unknown: true },
    { revision: '1' },
  ])('rejects schema-invalid fixtures before creating handlers', (value) => {
    expect(() => jsonHandler({ method: 'GET', url,
      request: { schema: emptyRequest, read: () => null },
      response: { status: 200, schema: receiptSchema, body: value },
    })).toThrow('Invalid synthetic JSON fixture');
  });

  it('rejects transforming schemas and hides private schema diagnostics', () => {
    expect(() => checkedFixture({ parse: () => ({ altered: true }) }, {})).toThrow('Invalid synthetic JSON fixture');
    expect(() => checkedFixture(receiptSchema, { password: 'never echo this' }))
      .toThrow(/^Invalid synthetic JSON fixture$/);
  });

  it.each([199, 600, 200.5, 204, 205, 304])('rejects incompatible JSON response status %s', (status) => {
    expect(() => jsonHandler({ method: 'GET', url,
      request: { schema: emptyRequest, read: () => null },
      response: { status, schema: receiptSchema, body },
    })).toThrow('JSON fixture requires a body-bearing HTTP status');
  });
});

describe('strict MSW lifecycle with real fetch and shared client', () => {
  it('serves a validated snapshot through the shared API transport', async () => {
    const client = createApiClient({ origin: 'http://localhost' });
    const result = await client.request({ path: '/api/v1/identity/synthetic-test-receipt',
      schema: receiptSchema, successStatuses: [200] });
    expect(result).toEqual({ kind: 'success', status: 200, data: body });
    server.assertClean();
  });

  it('validates requests before response and exposes swallowed resolver failures', async () => {
    server.use(jsonHandler({ method: 'POST', url,
      request: { schema: receiptSchema, read: (request) => request.json() },
      response: { status: 201, schema: receiptSchema, body },
    }));
    const valid = await fetch(url, { method: 'POST', body: JSON.stringify(body) });
    expect(valid.status).toBe(201);
    const invalid = await fetch(url, { method: 'POST', body: '{"revision":1}' });
    expect(invalid.status).toBe(500);
    expect(() => server.assertClean()).toThrow('Fixture transport failed');
    expect(() => server.reset()).toThrow('Fixture transport failed');
    server.assertClean();
  });

  it('blocks unhandled requests instead of contacting an external backend', async () => {
    // MSW turns a thrown resolver/unhandled-request failure into a local 500.
    expect((await fetch('http://unhandled.invalid/synthetic')).status).toBe(500);
    expect(() => server.reset()).toThrow('Fixture transport failed');
  });

  it('resets scenario overrides back to the original valid handlers', async () => {
    server.use(jsonHandler({ method: 'GET', url,
      request: { schema: emptyRequest, read: () => null },
      response: { status: 202, schema: receiptSchema, body: { label: 'Synthetic pending', revision: '0' } },
    }));
    expect((await fetch(url)).status).toBe(202);
    server.reset();
    expect((await fetch(url)).status).toBe(200);
    expect(await (await fetch(url)).json()).toEqual(body);
  });

  it('renders the shared button and accessible async state using RTL and a checked fixture', async () => {
    function SyntheticView() {
      const [label, setLabel] = useState('Synthetic idle');
      const [busy, setBusy] = useState(false);
      async function load() {
        setBusy(true);
        const result = await createApiClient({ origin: 'http://localhost' }).request({
          path: '/api/v1/identity/synthetic-test-receipt', schema: receiptSchema, successStatuses: [200],
        });
        setLabel(result.kind === 'success' ? result.data.label : 'Synthetic error');
        setBusy(false);
      }
      return createElement('div', null,
        createElement(Button, { processing: busy, onClick: () => { void load(); } }, 'Load synthetic fixture'),
        createElement('p', { role: 'status' }, label));
    }
    render(createElement(SyntheticView));
    expect(screen.getByRole('status').textContent).toBe('Synthetic idle');
    const user = userEvent.setup();
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Load synthetic fixture' }));
    await user.keyboard('{Enter}');
    expect((await screen.findByText('Synthetic ready')).getAttribute('role')).toBe('status');
    expect(screen.getByRole('button').hasAttribute('disabled')).toBe(false);
  });
});
