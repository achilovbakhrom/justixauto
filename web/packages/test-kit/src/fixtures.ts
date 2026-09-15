/** Test-only mechanics. Each feature supplies its approved DTO schemas and synthetic state. */
import type { Schema, Method } from '@justixauto/api';
import { http, HttpResponse, type HttpHandler } from 'msw';
import { setupServer } from 'msw/node';
import type { Browser, Page } from '@playwright/test';

function invalid(): never { throw new Error('Invalid synthetic JSON fixture'); }

function copyJson(value: unknown, ancestors = new Set<object>()): unknown {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return value;
  if (typeof value === 'number' && Number.isFinite(value) && !Object.is(value, -0)) return value;
  if (typeof value !== 'object' || ancestors.has(value)) return invalid();
  const array = Array.isArray(value);
  if (!array && Object.getPrototypeOf(value) !== Object.prototype
    && Object.getPrototypeOf(value) !== null) return invalid();
  ancestors.add(value);
  try {
    const keys = Reflect.ownKeys(value);
    if (keys.some((key) => typeof key !== 'string')) return invalid();
    const entries = Object.getOwnPropertyDescriptors(value);
    if (Object.entries(entries).some(([key, descriptor]) =>
      !(array && key === 'length') && (!descriptor.enumerable || !('value' in descriptor)))) return invalid();
    if (array) {
      if (keys.length !== value.length + 1) return invalid();
      const result: unknown[] = [];
      for (let index = 0; index < value.length; index++) {
        const slot = entries[String(index)];
        if (!slot || !('value' in slot)) return invalid();
        result.push(copyJson(slot.value, ancestors));
      }
      return Object.freeze(result);
    }
    return Object.freeze(Object.fromEntries(Object.keys(entries).sort().map((key) =>
      [key, copyJson(entries[key]?.value, ancestors)])));
  } finally { ancestors.delete(value); }
}

/** Reject lossy JSON (undefined, holes, dates, nonfinite numbers, accessors, cycles).
 * The injected synchronous schema must preserve the actual JSON wire value;
 * transforming/coercing schemas are rejected. No schema diagnostics leak.
 */
export function checkedFixture<T>(schema: Schema<T>, value: unknown): Readonly<T> {
  try {
    const snapshot = copyJson(value);
    const parsed = copyJson(schema.parse(snapshot));
    if (JSON.stringify(parsed) !== JSON.stringify(snapshot)) return invalid();
    return snapshot as Readonly<T>;
  } catch { return invalid(); }
}

export interface JsonHandlerOptions<T, R> {
  method: Method;
  url: string;
  /** Explicit request extraction keeps query/header/body contracts feature-owned. */
  request: { schema: Schema<R>; read(request: Request): unknown | Promise<unknown> };
  response: { status: number; schema: Schema<T>; body: unknown };
}

/** A static scenario, not a simulated authorization or command state machine. */
export function jsonHandler<T, R>(options: JsonHandlerOptions<T, R>): HttpHandler {
  const { status } = options.response;
  if (!Number.isInteger(status) || status < 200 || status > 599
    || [204, 205, 304].includes(status)) throw new Error('JSON fixture requires a body-bearing HTTP status');
  const snapshot = checkedFixture(options.response.schema, options.response.body);
  const body = JSON.stringify(snapshot);
  return http[options.method.toLowerCase() as Lowercase<Method>](options.url, async ({ request }) => {
    try { checkedFixture(options.request.schema, await options.request.read(request)); }
    catch { throw new Error('Synthetic request violates its fixture contract'); }
    return new HttpResponse(body, { status, headers: { 'Content-Type': 'application/json' } });
  });
}

/** One harness per test process. Call reset() after each test and close() at end.
 * Both assert failures even if an app has caught fetch errors as an error screen.
 */
export function createFixtureServer(...handlers: HttpHandler[]) {
  const server = setupServer(...handlers);
  const failures: string[] = [];
  server.events.on('unhandledException', () => { failures.push('Fixture resolver failed'); });
  function assertClean() {
    if (failures.length) throw new Error(`Fixture transport failed (${failures.length} request(s))`);
  }
  return {
    listen() {
      server.listen({ onUnhandledRequest() {
        failures.push('Unhandled fixture request');
        throw new Error('Unhandled fixture request');
      } });
    },
    use(...overrides: HttpHandler[]) { server.use(...overrides); },
    assertClean,
    reset() {
      try { assertClean(); }
      finally { failures.length = 0; server.resetHandlers(); server.restoreHandlers(); }
    },
    close() { try { assertClean(); } finally { server.close(); failures.length = 0; } },
  };
}

export interface CaptureTarget<T> {
  url: string;
  /** Runs before navigation: install synthetic API routes or explicit fixture adapters. */
  prepare(page: Page, state: Readonly<T>): Promise<void>;
  /** Navigate to the exact interaction and assert its visible state before capture. */
  ready(page: Page, state: Readonly<T>): Promise<void>;
}

export interface ComparisonOptions<T> {
  browser: Browser;
  schema: Schema<T>;
  state: unknown;
  /** Non-sensitive identifier for the approved synthetic scenario; never raw state. */
  stateId: string;
  viewport: { width: number; height: number };
  reference: CaptureTarget<T>;
  candidate: CaptureTarget<T>;
}

function localUrl(input: string): string {
  const url = new URL(input);
  if (url.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)
    || url.username || url.password) throw new Error('Browser fixtures require an explicit loopback HTTP URL');
  return url.href;
}

/** Captures evidence; it does NOT assert visual equivalence. Each side receives
 * isolated storage, the same validated state, viewport, locale and fixed Date.
 * No default demo account, seed database, persistence adapter or navigation exists.
 */
export async function captureComparison<T>(options: ComparisonOptions<T>) {
  const state = checkedFixture(options.schema, options.state);
  const { width, height } = options.viewport;
  if (![width, height].every((size) => Number.isSafeInteger(size) && size > 0)
    || !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$/.test(options.stateId)) {
    throw new Error('Comparison requires an explicit viewport and synthetic state ID');
  }
  const referenceUrl = localUrl(options.reference.url);
  const candidateUrl = localUrl(options.candidate.url);
  const environment = {
    viewport: { width, height }, deviceScaleFactor: 1,
    locale: 'ru-RU', timezoneId: 'Asia/Tashkent',
    colorScheme: 'light' as const, reducedMotion: 'reduce' as const,
    serviceWorkers: 'block' as const,
  };
  const clock = '2026-01-01T00:00:00.000Z';
  async function capture(target: CaptureTarget<T>, url: string) {
    const context = await options.browser.newContext(environment);
    try {
      const page = await context.newPage();
      const pageErrors: string[] = [];
      page.on('pageerror', () => { pageErrors.push('Uncaught page error'); });
      await page.clock.setFixedTime(new Date(clock));
      const isolatedState = checkedFixture(options.schema, state);
      await target.prepare(page, isolatedState);
      const response = await page.goto(url, { waitUntil: 'load' });
      if (!response?.ok()) throw new Error('Comparison page did not load successfully');
      await target.ready(page, isolatedState);
      await page.evaluate(async () => { await document.fonts.ready; });
      const screenshot = await page.screenshot({ fullPage: true, animations: 'disabled', caret: 'hide', scale: 'css' });
      if (pageErrors.length) throw new Error('Comparison page raised an uncaught error');
      return screenshot;
    } finally { await context.close(); }
  }
  // Sequential captures also enforce the single browser-heavy QA convention.
  const reference = await capture(options.reference, referenceUrl);
  const candidate = await capture(options.candidate, candidateUrl);
  return {
    reference, candidate,
    manifest: { stateId: options.stateId, referenceUrl, candidateUrl, ...environment, clock },
  };
}
