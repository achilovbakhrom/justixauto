import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { writeFile, readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { captureComparison } from '../../../../../web/packages/test-kit/src/fixtures';

const fixtureUrl = 'http://127.0.0.1:4196/docs/justix-auto/dev/qa/T-036/developer-fixture.html';
const referenceUrl = 'http://127.0.0.1:4180/admin/';
const schema = { parse(value: unknown) {
  if (!value || typeof value !== 'object' || !('id' in value) || value.id !== 'zero-features'
    || Object.keys(value).length !== 1) throw new Error('Invalid shell state');
  return value as { id: 'zero-features' };
} };

async function metrics(page: Page) {
  return page.evaluate(() => {
    const shell = document.querySelector('.admin-shell')!;
    const sidebar = shell.querySelector('aside')!;
    const main = shell.querySelector('main')!;
    const header = main.querySelector('header')!;
    const nav = sidebar.querySelector('nav')!;
    const rect = (element: Element) => { const r = element.getBoundingClientRect(); return { x:r.x, y:r.y, width:r.width, height:r.height }; };
    return { sidebar: rect(sidebar), header: rect(header), brand: rect(sidebar.firstElementChild!),
      buttons: [...nav.querySelectorAll('button')].map((button) => ({ label: button.textContent, ...rect(button),
        font: getComputedStyle(button).font, color: getComputedStyle(button).color })),
      overflow: document.documentElement.scrollWidth > innerWidth,
    };
  });
}

test('M-A01 shell placement, explicit projected zero-feature comparison', async ({ browser, viewport }) => {
  const prefix = `docs/justix-auto/dev/qa/T-036/developer-${viewport!.width}`;
  let referenceMetrics: Awaited<ReturnType<typeof metrics>>;
  let candidateMetrics: Awaited<ReturnType<typeof metrics>>;
  const result = await captureComparison({ browser, schema, state: { id: 'zero-features' },
    stateId: 't036-zero-features-shell', viewport: viewport!,
    reference: { url: referenceUrl, prepare: async () => {}, ready: async (page) => {
      await expect(page.locator('.admin-sidebar')).toBeVisible();
      await page.screenshot({ path: `${prefix}-reference-raw.png`, fullPage: true });
      // Explicit shell-only projection: preserve original reference separately,
      // hide business content and clear the active page. This is not an observed
      // business empty/auth state, and cannot establish Overview page parity.
      await page.locator('.admin-sidebar button').evaluateAll((buttons) => buttons.forEach((button) => {
        button.classList.remove('active'); (button as HTMLButtonElement).disabled = true;
      }));
      await page.addStyleTag({ content: '.admin-page { visibility: hidden }' });
      referenceMetrics = await metrics(page);
    } },
    candidate: { url: fixtureUrl, prepare: async (page) => {
      await page.addInitScript(() => {
        Storage.prototype.setItem = () => { throw new Error('Unexpected shell persistence'); };
        window.fetch = () => { throw new Error('Unexpected shell API call'); };
      });
    }, ready: async (page) => {
      await expect(page.getByRole('main')).toBeVisible();
      const buttons = page.getByRole('navigation').getByRole('button');
      await expect(buttons).toHaveCount(8);
      for (const button of await buttons.all()) await expect(button).toBeDisabled();
      await expect(page.getByRole('link')).toHaveCount(0);
      await expect(page.getByRole('heading')).toHaveCount(0);
      await expect(page.getByRole('dialog')).toHaveCount(0);
      candidateMetrics = await metrics(page);
      await page.screenshot({ path: `${prefix}-candidate-raw.png`, fullPage: true });
    } },
  });
  expect(candidateMetrics!).toEqual(referenceMetrics!);
  expect(candidateMetrics!.overflow).toBe(false);
  await writeFile(`${prefix}-reference-projected.png`, result.reference);
  await writeFile(`${prefix}-candidate.png`, result.candidate);
  const hashes = Object.fromEntries(await Promise.all(['admin/admin.js', 'admin/management.js', 'admin/admin.css', 'common.css', 'styles.css', 'insurance.css'].map(async (file) =>
    [file, createHash('sha256').update(await readFile(`docs/justix-auto/mocks/${file}`)).digest('hex')])));
  await writeFile(`${prefix}-manifest.json`, JSON.stringify({ ...result.manifest, hashes,
    anchors: ['admin/admin.js::render', 'admin/management.js::page'],
    comparison: 'Shell only; raw reference retained, business content masked and active Overview cleared. Not full page parity.',
    referenceMetrics: referenceMetrics!, candidateMetrics: candidateMetrics!,
  }, null, 2) + '\n');
});

test('entry and deep links stay closed; local dev/preview HTML fallback never captures assets or APIs', async ({ page, request }) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('response', (response) => {
    if (response.request().resourceType() === 'script' && response.status() >= 400) errors.push(`Script failed: ${response.url()}`);
  });
  for (const port of [4197, 4198]) {
    for (const path of ['/admin/', '/admin/companies/synthetic', '/admin/users/one?tab=access']) {
      const response = await page.goto(`http://127.0.0.1:${port}${path}`);
      expect(response?.status()).toBe(200);
      await expect(page).toHaveTitle('JustixAuto — Администрирование');
      await expect(page.getByRole('main')).toHaveCount(0);
      await expect(page.locator('#admin-root')).toBeEmpty();
      await page.reload();
      await expect(page.locator('#admin-root')).toBeEmpty();
      await expect(page.locator('vite-error-overlay')).toHaveCount(0);
    }
    for (const path of ['/', '/finance/', '/insurance/', '/api/session', '/admin/api/session', '/admin/assets/missing.js', '/admin/assets/extensionless', '/admin/missing.svg', '/administer/']) {
      const response = await request.get(`http://127.0.0.1:${port}${path}`, { headers: { Accept: 'text/html' } });
      expect(response.status(), `${port}${path}`).toBe(404);
      expect(response.headers()['content-type']).not.toContain('text/html');
      expect(await response.text()).not.toContain('admin-root');
    }
    const post = await request.post(`http://127.0.0.1:${port}/admin/companies`, { headers: { Accept: 'text/html' } });
    expect(post.status()).toBe(404);
  }
  expect(errors).toEqual([]);
});

test('synthetic admitted shell retains browser navigation and long identity text without storage', async ({ page, viewport }) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(`${fixtureUrl}?long=1`);
  await expect(page.getByRole('main')).toBeVisible();
  await page.evaluate(() => {
    history.pushState(null, '', '/admin/companies/synthetic');
    dispatchEvent(new PopStateEvent('popstate'));
    history.pushState(null, '', '/admin/users/synthetic');
    dispatchEvent(new PopStateEvent('popstate'));
  });
  await page.goBack();
  await expect(page).toHaveURL(/\/admin\/companies\/synthetic$/);
  await page.goForward();
  await expect(page).toHaveURL(/\/admin\/users\/synthetic$/);
  await expect(page.getByRole('main')).toBeVisible();
  expect(await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length,
    overflow: document.documentElement.scrollWidth > innerWidth }))).toEqual({ local: 0, session: 0, overflow: false });
  await page.keyboard.press('Tab');
  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe('BODY');
  expect(errors).toEqual([]);
  await page.screenshot({ path: `docs/justix-auto/dev/qa/T-036/developer-${viewport!.width}-long-label.png`, fullPage: true });
});
