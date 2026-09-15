import { expect, test } from '@playwright/test';
import { captureComparison } from '../../../../../web/packages/test-kit/src/fixtures';
import type { Schema } from '@justixauto/api';
import { writeFile } from 'node:fs/promises';

// Both targets intentionally use the same integrated synthetic React fixture.
// This proves repeatability/isolation, NOT parity with a business HTML page.
const scopes = ['dealer-shell', 'finance-workspace', 'ins-workspace', 'admin-shell'] as const;
interface State { text: string }
const schema: Schema<State> = { parse(value) {
  if (!value || typeof value !== 'object' || !('text' in value)
    || typeof value.text !== 'string' || Object.keys(value).length !== 1) throw new Error('Invalid state');
  return value as State;
} };

for (const scope of scopes) {
  test(`isolated repeatable ${scope}`, async ({ browser, viewport }, testInfo) => {
    const url = `http://127.0.0.1:4195/docs/justix-auto/dev/qa/T-034/developer-fixture.html?scope=${scope}`;
    const contextCount = browser.contexts().length;
    let prepared = 0;
    const target = {
      url,
      async prepare(page: import('@playwright/test').Page, state: Readonly<State>) {
        expect(Object.isFrozen(state)).toBe(true);
        await page.addInitScript(() => {
          if (localStorage.length !== 0 || sessionStorage.length !== 0) throw new Error('Storage was reused');
          sessionStorage.setItem('t035-isolation-probe', 'synthetic-only');
        });
        prepared++;
      },
      async ready(page: import('@playwright/test').Page, state: Readonly<State>) {
        await expect(page.getByTestId('writes')).toHaveText('Synthetic writes: 0');
        await page.getByRole('button', { name: 'Открыть форму' }).click();
        await page.getByRole('textbox', { name: 'Название' }).fill(state.text);
        await expect(page.getByRole('dialog')).toBeVisible();
        await expect(page.getByRole('textbox', { name: 'Название' })).toHaveValue(state.text);
        expect(await page.evaluate(() => new Date().toISOString())).toBe('2026-01-01T00:00:00.000Z');
        expect(page.viewportSize()).toEqual(viewport);
      },
    };
    const result = await captureComparison({ browser, schema, state: { text: 'Synthetic repeatable value' },
      stateId: `t035-${scope}`, viewport: viewport!, reference: target, candidate: target });
    expect(prepared).toBe(2);
    expect(browser.contexts().length).toBe(contextCount);
    expect(result.reference.equals(result.candidate)).toBe(true);
    expect(JSON.stringify(result.manifest)).not.toContain('Synthetic repeatable value');
    const prefix = `docs/justix-auto/dev/qa/T-035/developer-${scope}-${viewport!.width}`;
    await writeFile(`${prefix}-reference.png`, result.reference);
    await writeFile(`${prefix}-candidate.png`, result.candidate);
    await writeFile(`${prefix}-manifest.json`, `${JSON.stringify(result.manifest, null, 2)}\n`);
    await testInfo.attach('same-state-manifest', { body: JSON.stringify(result.manifest), contentType: 'application/json' });
  });
}

test('invalid setup fails before capture and a readiness failure closes its context', async ({ browser, viewport }) => {
  const count = browser.contexts().length;
  const target = { url: 'http://127.0.0.1:4195/docs/justix-auto/dev/qa/T-034/developer-fixture.html',
    prepare: async () => {}, ready: async () => { throw new Error('Expected readiness failure'); } };
  const options = { browser, schema, state: { text: 'Synthetic' }, stateId: 'failure-probe',
    viewport: viewport!, reference: target, candidate: target };
  await expect(captureComparison({ ...options, state: { text: 1 } })).rejects.toThrow('Invalid synthetic JSON fixture');
  await expect(captureComparison({ ...options, viewport: { width: 0, height: 1 } })).rejects.toThrow('explicit viewport');
  await expect(captureComparison({ ...options, reference: { ...target, url: 'https://example.com' } }))
    .rejects.toThrow('loopback HTTP');
  expect(browser.contexts().length).toBe(count);
  await expect(captureComparison(options)).rejects.toThrow('Expected readiness failure');
  expect(browser.contexts().length).toBe(count);
});
