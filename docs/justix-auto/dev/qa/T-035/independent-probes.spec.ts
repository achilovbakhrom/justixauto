import { test, expect, type Page } from '@playwright/test';
import { createServer } from 'node:http';
import { writeFile } from 'node:fs/promises';
import { http, HttpResponse } from 'msw';
import { checkedFixture, createFixtureServer, jsonHandler, captureComparison } from '../../../../../web/packages/test-kit/src/fixtures';

const identity = { parse: (value: unknown) => value };
const nullSchema = { parse(value: unknown) { if (value !== null) throw Error('PRIVATE'); return null; } };
const url = 'http://127.0.0.1:4196/docs/justix-auto/dev/qa/T-034/developer-fixture.html?scope=admin-shell';
const noOp = async () => {};

test('independent lossy/transforming JSON and descriptor probes', () => {
  let getterCalls = 0;
  const accessor = Object.defineProperty({}, 'x', { enumerable: true, get() { getterCalls++; return 1; } });
  const cases = [accessor, { a: [1, undefined] }, { x: -Infinity }, new Map([['x', 1]]),
    Object.create({ inherited: 1 }), Object.defineProperty([1], 'extra', { value: 2 }),
    Object.assign([1], { [Symbol('x')]: 2 }), { x: -0 }, { x: 1n }];
  for (const value of cases) expect(() => checkedFixture(identity, value)).toThrow(/^Invalid synthetic JSON fixture$/);
  expect(getterCalls).toBe(0);
  for (const parse of [() => ({ n: 1 }), () => ({}), () => Promise.resolve({ n: '1' }),
    () => ({ n: '1', extra: undefined })]) {
    expect(() => checkedFixture({ parse }, { n: '1' })).toThrow(/^Invalid synthetic JSON fixture$/);
  }
  const source = JSON.parse('{"__proto__":{"safe":true},"constructor":"fixture","x":[{"n":"9223372036854775807"}]}');
  const snapshot = checkedFixture(identity, source) as typeof source;
  source.x[0].n = '2';
  expect(snapshot.x[0].n).toBe('9223372036854775807');
  expect(Object.isFrozen(snapshot.x[0])).toBe(true);
  expect(Object.prototype.hasOwnProperty.call(snapshot, '__proto__')).toBe(true);
  expect(({} as { safe?: boolean }).safe).toBeUndefined();
});

test('MSW traps caught errors and prevents real network fallthrough; reset and close expose failures', async () => {
  let networkHits = 0;
  const network = createServer((_req, res) => { networkHits++; res.end('unexpected'); });
  await new Promise<void>(resolve => network.listen(0, '127.0.0.1', resolve));
  const address = network.address();
  if (!address || typeof address === 'string') throw Error('Missing local test port');
  const endpoint = `http://127.0.0.1:${address.port}/synthetic`;
  const response = { nested: { value: 'original' } };
  const fixture = jsonHandler({ method: 'POST', url: endpoint,
    request: { schema: nullSchema, read: request => request.json() },
    response: { status: 201, schema: identity, body: response } });
  response.nested.value = 'mutated';
  const server = createFixtureServer(fixture);
  server.listen();
  try {
    const ok = await fetch(endpoint, { method: 'POST', body: 'null' });
    expect(await ok.json()).toEqual({ nested: { value: 'original' } });
    await fetch(endpoint, { method: 'POST', body: '"bad"' }).catch(() => undefined);
    expect(() => server.assertClean()).toThrow('Fixture transport failed');
    expect(() => server.reset()).toThrow('Fixture transport failed');
    server.assertClean();
    await fetch(`${endpoint}/unregistered`).catch(() => undefined);
    expect(() => server.reset()).toThrow('Fixture transport failed');
    expect(networkHits).toBe(0);
    server.use(http.post(endpoint, () => HttpResponse.json({ override: true }), { once: true }));
    expect(await (await fetch(endpoint, { method: 'POST', body: 'null' })).json()).toEqual({ override: true });
    server.reset();
    expect(await (await fetch(endpoint, { method: 'POST', body: 'null' })).json()).toEqual({ nested: { value: 'original' } });
    server.use(http.post(endpoint, () => { throw Error('Synthetic resolver failure'); }));
    await fetch(endpoint, { method: 'POST', body: 'null' }).catch(() => undefined);
    expect(() => server.close()).toThrow('Fixture transport failed');
    expect(networkHits).toBe(0);
  } finally {
    server.close();
    await new Promise<void>((resolve, reject) => network.close(error => error ? reject(error) : resolve()));
  }
});

test('real browser captures identical synthetic state with isolated cookies/storage and fixed environment', async ({ browser, viewport }) => {
  const count = browser.contexts().length;
  const states: unknown[] = [];
  const target = { url,
    async prepare(page: Page, state: unknown) {
      states.push(state);
      await page.addInitScript(() => {
        if (localStorage.getItem('qa-t035') || sessionStorage.getItem('qa-t035') || document.cookie.includes('qa-t035')) throw Error('State leaked');
        localStorage.setItem('qa-t035', 'synthetic'); sessionStorage.setItem('qa-t035', 'synthetic'); document.cookie = 'qa-t035=synthetic; path=/';
      });
    },
    async ready(page: Page) {
      expect(page.viewportSize()).toEqual(viewport);
      expect(await page.evaluate(() => ({ date: new Date().toISOString(), locale: navigator.language,
        tz: Intl.DateTimeFormat().resolvedOptions().timeZone, scale: devicePixelRatio,
        reduced: matchMedia('(prefers-reduced-motion: reduce)').matches }))).toEqual({
        date: '2026-01-01T00:00:00.000Z', locale: 'ru-RU', tz: 'Asia/Tashkent', scale: 1, reduced: true });
      await page.getByRole('button', { name: 'Открыть форму' }).click();
      await page.getByRole('textbox', { name: 'Название' }).fill('QA synthetic');
      await expect(page.getByRole('dialog')).toBeVisible();
    },
  };
  const result = await captureComparison({ browser, schema: identity, state: { nested: ['QA synthetic'] },
    stateId: 'qa-t035-isolation', viewport: viewport!, reference: target, candidate: target });
  expect(states).toHaveLength(2); expect(states[0]).not.toBe(states[1]);
  expect(result.reference.equals(result.candidate)).toBe(true);
  expect(browser.contexts()).toHaveLength(count);
  expect(JSON.stringify(result.manifest)).not.toContain('QA synthetic');
  const prefix = `docs/justix-auto/dev/qa/T-035/independent-${viewport!.width}`;
  await writeFile(`${prefix}-reference.png`, result.reference);
  await writeFile(`${prefix}-candidate.png`, result.candidate);
  await writeFile(`${prefix}-manifest.json`, JSON.stringify(result.manifest, null, 2));
});

test('browser cleanup on preparation, candidate readiness, HTTP failure and uncaught page errors', async ({ browser, viewport }) => {
  const count = browser.contexts().length;
  const base = { browser, schema: identity, state: null, stateId: 'qa-failures', viewport: viewport!,
    reference: { url, prepare: noOp, ready: noOp }, candidate: { url, prepare: noOp, ready: noOp } };
  await expect(captureComparison({ ...base, reference: { ...base.reference, prepare: async () => { throw Error('prepare failure'); } } })).rejects.toThrow('prepare failure');
  expect(browser.contexts()).toHaveLength(count);
  await expect(captureComparison({ ...base, candidate: { ...base.candidate, ready: async () => { throw Error('candidate failure'); } } })).rejects.toThrow('candidate failure');
  expect(browser.contexts()).toHaveLength(count);
  await expect(captureComparison({ ...base, reference: { ...base.reference, prepare: async (page: Page) => {
    await page.route(url, route => route.fulfill({ status: 503, body: 'Synthetic unavailable' }));
  } } })).rejects.toThrow('did not load successfully');
  expect(browser.contexts()).toHaveLength(count);
  await expect(captureComparison({ ...base, reference: { ...base.reference, prepare: async (page: Page) => {
    await page.addInitScript(() => { throw Error('Synthetic uncaught error'); });
  } } })).rejects.toThrow('uncaught error');
  expect(browser.contexts()).toHaveLength(count);
});

test('runnable HTML mock reference can be captured with explicit readiness, without parity claim', async ({ browser, viewport }) => {
  const mock = { url: 'http://127.0.0.1:4196/docs/justix-auto/mocks/admin/', prepare: noOp,
    async ready(page: Page) { await expect(page.locator('body')).toContainText('Компании'); } };
  const result = await captureComparison({ browser, schema: identity, state: null, stateId: 'qa-admin-mock-default',
    viewport: viewport!, reference: mock, candidate: mock });
  expect(result.reference.length).toBeGreaterThan(1000);
  await writeFile(`docs/justix-auto/dev/qa/T-035/independent-mock-${viewport!.width}.png`, result.reference);
});
