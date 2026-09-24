import { decodeResponseJSON, responseJSONLimits } from './responseJSON';
import type { ResponseJSONLimits } from './responseJSON';
export type { ResponseJSONLimits } from './responseJSON';

/** Feature adapters must synchronously reject invalid/unknown DTO fields and return typed data. */
export interface Schema<T> {
  parse(value: unknown): T;
}

export const errorStatuses = [400, 401, 403, 404, 409, 412, 422, 428, 429, 503] as const;
export type ErrorStatus = (typeof errorStatuses)[number];
export type SuccessStatus = 200 | 201 | 202 | 204;
export type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export interface ErrorReceipt {
  readonly error: {
    readonly code: string;
    readonly message: string;
    // Field-value schemas belong to the feature; the common contract specifies an object.
    readonly fields: Readonly<Record<string, unknown>>;
    readonly traceId: string;
  };
  readonly operationId?: string;
}

export type HttpFailure = {
  [S in ErrorStatus]: { readonly kind: 'http-error'; readonly status: S; readonly receipt: ErrorReceipt };
}[ErrorStatus];

export type ApiResult<T> =
  | { readonly kind: 'success'; readonly status: SuccessStatus; readonly data: T }
  | HttpFailure
  | { readonly kind: 'unexpected-status'; readonly status: number }
  | { readonly kind: 'invalid-response'; readonly status: number }
  | { readonly kind: 'invalid-request' }
  | { readonly kind: 'transport-error'; readonly outcome: 'unavailable' | 'unknown'; readonly aborted: boolean };

export interface HeaderRule {
  readonly csrf: 'required' | 'forbidden';
  readonly retryAfter: 'required' | 'forbidden';
}
export interface ResponseContract {
  readonly numeric: 'safe-integers';
  readonly errors: Readonly<Partial<Record<ErrorStatus, Schema<ErrorReceipt>>>>;
  readonly metadata: Readonly<Partial<Record<SuccessStatus | ErrorStatus, HeaderRule>>>;
  readonly auth: boolean;
}
export type ValidatedResult<T> = Extract<ApiResult<T>, { kind: 'success' | 'http-error' }>;
export interface ResponseMetadata {
  readonly csrf?: string;
  readonly retryAfterSeconds?: number;
}
/** Trusted synchronous port: check the captured current ticket before any mutation. */
export interface AuthExchange<T> {
  prepare(): { readonly csrf?: string } | undefined;
  accept(result: ValidatedResult<T>, metadata: ResponseMetadata): boolean;
}

export interface ApiRequest<T> {
  readonly path: string;
  readonly method?: Method;
  readonly schema: Schema<T>;
  /** Explicitly select the endpoint's success statuses; 204 passes undefined to the adapter. */
  readonly successStatuses: readonly SuccessStatus[];
  readonly body?: unknown;
  readonly contextRevision?: string;
  readonly ifMatch?: string;
  readonly idempotencyKey?: string;
  /** Feature-owned headers, including the approved session CSRF protocol when available. */
  readonly headers?: HeadersInit;
  readonly signal?: AbortSignal;
  readonly responseContract?: ResponseContract;
  readonly authExchange?: AuthExchange<T>;
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isRevision(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    /^(0|[1-9][0-9]*)$/.test(value) &&
    (value.length < 19 || (value.length === 19 && value <= '9223372036854775807'))
  );
}
export function isId(value: unknown): value is string {
  return typeof value === 'string' && uuid.test(value);
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
function parseError(value: unknown): ErrorReceipt {
  if (
    !record(value) ||
    !record(value.error) ||
    Object.keys(value).some((key) => key !== 'error' && key !== 'operationId') ||
    Object.keys(value.error).some((key) => !['code', 'message', 'fields', 'traceId'].includes(key)) ||
    typeof value.error.code !== 'string' ||
    value.error.code.length === 0 ||
    typeof value.error.message !== 'string' ||
    typeof value.error.traceId !== 'string' ||
    !record(value.error.fields) ||
    ('operationId' in value && !isId(value.operationId))
  ) {
    throw new Error('Invalid error receipt');
  }
  return value as unknown as ErrorReceipt;
}

function requestUrl(path: string, origin: string): URL {
  // Reject ambiguous paths before URL normalization; never fetch another app's HTML.
  if (
    !/^\/api\/v1\/(identity|inventory|commerce|retail|financing|insurance|documents|operations)\//.test(path) ||
    /[\\#\s]/.test(path)
  )
    throw new Error('Invalid API path');
  const pathPart = path.split('?')[0] ?? '';
  for (const segment of pathPart.split('/')) {
    const decoded = decodeURIComponent(segment);
    if (
      decoded === '.' ||
      decoded === '..' ||
      /[\\/%]/.test(decoded) ||
      [...decoded].some((character) => character.charCodeAt(0) <= 32)
    ) {
      throw new Error('Invalid API path segment');
    }
  }
  const url = new URL(path, origin);
  if (url.origin !== origin) throw new Error('Invalid origin');
  return url;
}

function requestHeaders(request: ApiRequest<unknown>): Headers {
  const headers = new Headers(request.headers);
  for (const key of headers.keys()) {
    const normalized = key.toLowerCase().replaceAll('_', '-');
    if (
      /^(authorization|proxy-authorization|cookie|host|origin|accept|content-type|idempotency-key|if-match|x-context-revision)$/.test(
        normalized,
      ) ||
      /^(sec-|x-(actor|user|company|branch|permissions?|internal|justix-internal|forwarded)(-|$))/.test(normalized)
    ) {
      throw new Error('Reserved header');
    }
  }
  headers.set('Accept', 'application/json');
  if (request.body !== undefined) headers.set('Content-Type', 'application/json');
  for (const [name, value] of [
    ['X-Context-Revision', request.contextRevision],
    ['If-Match', request.ifMatch],
  ] as const) {
    if (value !== undefined) {
      if (!isRevision(value)) throw new Error('Invalid revision');
      headers.set(name, name === 'If-Match' ? `"${value}"` : value);
    }
  }
  if (request.idempotencyKey !== undefined) {
    if (!isId(request.idempotencyKey)) throw new Error('Invalid idempotency key');
    headers.set('Idempotency-Key', request.idempotencyKey);
  }
  return headers;
}

const promiseThen = Promise.prototype.then;
const typedArrayTag = Object.getOwnPropertyDescriptor(
  Object.getPrototypeOf(Uint8Array.prototype),
  Symbol.toStringTag,
)!.get!;
const discard = () => undefined;
/** Brand-check and silence supported native Promises without reading then/getters. */
function disposePromise(value: unknown): boolean {
  try {
    Reflect.apply(promiseThen, value, [discard, discard]);
    return true;
  } catch {
    return false;
  }
}
function synchronous<T>(invoke: () => T): T {
  let value: T;
  try {
    value = invoke();
  } catch (error) {
    disposePromise(error);
    // Binding diagnostics may contain secrets and must not survive in a cause chain.
    // eslint-disable-next-line preserve-caught-error
    throw new Error('Invalid binding');
  }
  if (disposePromise(value)) throw new Error('Asynchronous binding');
  // Do not assimilate arbitrary thenables, including accessor-based ones.
  if ((typeof value === 'object' && value !== null) || typeof value === 'function') {
    for (
      let current: object | null = value as object;
      current !== null;
      current = Object.getPrototypeOf(current) as object | null
    ) {
      const then = Object.getOwnPropertyDescriptor(current, 'then');
      if (then && (!('value' in then) || typeof then.value === 'function')) throw new Error('Asynchronous binding');
    }
  }
  return value;
}
function ownRecord(value: unknown, allowed?: readonly string[]): Record<string, unknown> {
  if (!record(value) || (Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null)) {
    throw new Error('Invalid configuration');
  }
  const snapshot: Record<string, unknown> = Object.create(null) as Record<string, unknown>;
  for (const key of Reflect.ownKeys(value)) {
    const descriptor = Object.getOwnPropertyDescriptor(value, key)!;
    if (typeof key !== 'string' || !('value' in descriptor) || (allowed && !allowed.includes(key))) {
      throw new Error('Invalid configuration');
    }
    snapshot[key] = descriptor.value;
  }
  return snapshot;
}
function captureParser<T>(schema: Schema<T>): (value: unknown) => T {
  const parse = captureMethod(schema, 'parse');
  return (value) => synchronous(() => Reflect.apply(parse, schema, [value]) as T);
}
function captureMethod(receiver: unknown, name: string): (...args: never[]) => unknown {
  if ((typeof receiver !== 'object' || receiver === null) && typeof receiver !== 'function')
    throw new Error('Missing binding');
  // Existing structural interfaces permit prototype methods and callable getters.
  // Capture once before dispatch; generated semantic tables impose their own rules.
  const method = Reflect.get(receiver as object, name) as unknown;
  if (typeof method !== 'function') {
    // A callable getter may be misbound to a returned native Promise. Observe its
    // rejection before replacing it with the fixed failure; never assimilate thenables.
    disposePromise(method);
    throw new Error('Missing binding');
  }
  return method as (...args: never[]) => unknown;
}
type CapturedContract = {
  readonly auth: boolean;
  readonly errors: ReadonlyMap<number, (value: unknown) => ErrorReceipt>;
  readonly metadata: ReadonlyMap<number, HeaderRule>;
};
function captureContract(contract: ResponseContract, successes: readonly SuccessStatus[]): CapturedContract {
  const config = ownRecord(contract, ['numeric', 'errors', 'metadata', 'auth']);
  if (config.numeric !== 'safe-integers' || typeof config.auth !== 'boolean') throw new Error('Invalid profile');
  const errors = new Map<number, (value: unknown) => ErrorReceipt>();
  for (const [key, schema] of Object.entries(ownRecord(config.errors))) {
    const status = Number(key);
    if (String(status) !== key || !errorStatuses.includes(status as ErrorStatus)) throw new Error('Invalid status');
    errors.set(status, captureParser(schema as Schema<ErrorReceipt>));
  }
  const declared = new Set<number>([...successes, ...errors.keys()]);
  const metadata = new Map<number, HeaderRule>();
  for (const [key, value] of Object.entries(ownRecord(config.metadata))) {
    const status = Number(key);
    const rule = ownRecord(value, ['csrf', 'retryAfter']);
    if (
      String(status) !== key ||
      !declared.has(status) ||
      !['required', 'forbidden'].includes(rule.csrf as string) ||
      !['required', 'forbidden'].includes(rule.retryAfter as string) ||
      (config.auth &&
        ((status === 429 && (rule.csrf !== 'forbidden' || rule.retryAfter !== 'required')) ||
          (status !== 429 && rule.retryAfter !== 'forbidden') ||
          (status === 204 && rule.csrf !== 'forbidden')))
    ) {
      throw new Error('Invalid header rule');
    }
    metadata.set(status, Object.freeze({ csrf: rule.csrf, retryAfter: rule.retryAfter }) as HeaderRule);
  }
  if (metadata.size !== declared.size) throw new Error('Missing header rule');
  return { auth: config.auth, errors, metadata };
}
const csrfPattern = /^[A-Za-z0-9_-]{1,4096}$/;
function validCSRF(value: unknown): value is string {
  return typeof value === 'string' && csrfPattern.test(value);
}
function responseMetadata(headers: Headers, rule: HeaderRule, auth: boolean): ResponseMetadata {
  const csrf = headers.get('X-CSRF-Token');
  const retryAfter = headers.get('Retry-After');
  if (rule.csrf === 'required' ? !validCSRF(csrf) : csrf !== null) throw new Error('Invalid CSRF metadata');
  if (
    rule.retryAfter === 'required'
      ? retryAfter === null || !/^(0|[1-9][0-9]{0,9})$/.test(retryAfter) || Number(retryAfter) > 2147483647
      : retryAfter !== null
  )
    throw new Error('Invalid retry metadata');
  if (auth && headers.get('Cache-Control')?.toLowerCase() !== 'no-store') throw new Error('Invalid cache metadata');
  return Object.freeze({
    ...(csrf === null ? {} : { csrf }),
    ...(retryAfter === null ? {} : { retryAfterSeconds: Number(retryAfter) }),
  });
}
async function responseBytes(response: Response, maximum: number, signal?: AbortSignal): Promise<Uint8Array> {
  if (signal?.aborted) throw new Error('Aborted response');
  if (response.body === null) return new Uint8Array();
  const reader = response.body.getReader();
  let bytes = new Uint8Array(0);
  let length = 0;
  let abort: (() => void) | undefined;
  const aborted = new Promise<never>((_, reject) => {
    abort = () => reject(new Error('Aborted response'));
    signal?.addEventListener('abort', abort, { once: true });
  });
  try {
    while (true) {
      const { done, value } = await Promise.race([reader.read(), aborted]);
      if (signal?.aborted) throw new Error('Aborted response');
      if (done) break;
      if (
        !ArrayBuffer.isView(value) ||
        Reflect.apply(typedArrayTag, value, []) !== 'Uint8Array' ||
        value.byteLength > maximum - length
      )
        throw new Error('Response size exceeded');
      const nextLength = length + value.byteLength;
      if (nextLength > bytes.length) {
        // Amortized growth bounds storage even for millions of one-byte chunks.
        // Only bytes actually read drive allocation; Content-Length is irrelevant.
        const expanded = new Uint8Array(Math.max(nextLength, Math.min(maximum, Math.max(4096, bytes.length * 2))));
        expanded.set(bytes);
        bytes = expanded;
      }
      bytes.set(value, length);
      length = nextLength;
    }
    return bytes.subarray(0, length);
  } catch {
    // Cancellation is requested immediately; an uncooperative producer cannot delay failure.
    try {
      disposePromise(reader.cancel());
    } catch {
      /* already errored/closed */
    }
    throw new Error('Invalid response body');
  } finally {
    if (abort) signal?.removeEventListener('abort', abort);
    reader.releaseLock();
  }
}

interface PreparedRequest<T> {
  readonly url: URL;
  readonly init: RequestInit;
  readonly contract: CapturedContract | undefined;
  readonly parse: (value: unknown) => T;
  readonly accept: ((result: ValidatedResult<T>, metadata: ResponseMetadata) => boolean) | undefined;
  readonly successes: readonly SuccessStatus[];
}

function prepareAuthExchange<T>(
  exchange: AuthExchange<T>,
  method: Method,
  headers: Headers,
): (result: ValidatedResult<T>, metadata: ResponseMetadata) => boolean {
  const prepare = captureMethod(exchange, 'prepare');
  const acceptResponse = captureMethod(exchange, 'accept');
  const prepared = synchronous(() => Reflect.apply(prepare, exchange, []));
  if (prepared === undefined) throw new Error('Unavailable auth ticket');
  const values = ownRecord(prepared, ['csrf']);
  if (values.csrf !== undefined && !validCSRF(values.csrf)) throw new Error('Invalid request CSRF');
  if (method !== 'GET') {
    if (!validCSRF(values.csrf)) throw new Error('Missing request CSRF');
    headers.set('X-CSRF-Token', values.csrf);
  }
  return (result, metadata) => synchronous(() => Reflect.apply(acceptResponse, exchange, [result, metadata])) === true;
}

function assertValidEnvelope<T>(request: ApiRequest<T>, method: Method, successes: readonly SuccessStatus[]): void {
  if (
    !['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].includes(method) ||
    successes.length === 0 ||
    new Set(successes).size !== successes.length ||
    successes.some((status) => ![200, 201, 202, 204].includes(status)) ||
    request.signal?.aborted ||
    (method === 'GET' && request.body !== undefined)
  )
    throw new Error('Invalid request');
}

function assertNoCompetingCsrf(headers: Headers): void {
  for (const name of headers.keys()) {
    if (name.toLowerCase().replaceAll('_', '-') === 'x-csrf-token') throw new Error('Competing CSRF');
  }
}

/** Validates and normalizes a caller-supplied request; never throws. */
function prepareRequest<T>(request: ApiRequest<T>, origin: string): PreparedRequest<T> | undefined {
  try {
    const method = request.method ?? 'GET';
    const signal = request.signal;
    const successes = [...request.successStatuses];
    assertValidEnvelope(request, method, successes);
    const url = requestUrl(request.path, origin);
    const parse = captureParser(request.schema);
    const contract =
      request.responseContract === undefined ? undefined : captureContract(request.responseContract, successes);
    if (Boolean(contract?.auth) !== (request.authExchange !== undefined)) throw new Error('Invalid auth exchange');
    const headers = requestHeaders(request);
    if (request.authExchange) assertNoCompetingCsrf(headers);
    const init: RequestInit = {
      method,
      headers,
      credentials: 'same-origin',
      mode: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      ...(signal === undefined ? {} : { signal }),
      ...(request.body === undefined ? {} : { body: JSON.stringify(request.body) }),
    };
    const accept = request.authExchange ? prepareAuthExchange(request.authExchange, method, headers) : undefined;
    if (signal?.aborted) throw new Error('Aborted request');
    return { url, init, contract, parse, accept, successes };
  } catch (error) {
    disposePromise(error);
    return undefined;
  }
}

async function decodeResponseBody(
  response: Response,
  contract: CapturedContract | undefined,
  signal: AbortSignal | undefined,
  limits: Readonly<ResponseJSONLimits>,
): Promise<unknown> {
  if (response.status === 204) {
    await responseBytes(response, 0, signal);
    return undefined;
  }
  const media = response.headers.get('Content-Type');
  if (
    contract?.auth
      ? media === null || !/^application\/json(?:\s*;\s*charset=utf-8)?$/i.test(media)
      : media?.split(';')[0]?.trim().toLowerCase() !== 'application/json'
  ) {
    throw new Error('Expected JSON');
  }
  return decodeResponseJSON(
    await responseBytes(response, limits.maxResponseBytes, signal),
    contract ? 'safe-integers' : 'finite-json',
    limits,
  );
}

function buildResult<T>(
  status: number,
  knownError: boolean,
  contract: CapturedContract | undefined,
  parse: (value: unknown) => T,
  value: unknown,
): ValidatedResult<T> {
  if (knownError) {
    const receipt = contract ? contract.errors.get(status)!(value) : parseError(value);
    return { kind: 'http-error', status: status as ErrorStatus, receipt: parseError(receipt) };
  }
  return { kind: 'success', status: status as SuccessStatus, data: parse(value) };
}

async function interpretResponse<T>(
  response: Response,
  successes: readonly SuccessStatus[],
  contract: CapturedContract | undefined,
  parse: (value: unknown) => T,
  accept: ((result: ValidatedResult<T>, metadata: ResponseMetadata) => boolean) | undefined,
  signal: AbortSignal | undefined,
  limits: Readonly<ResponseJSONLimits>,
): Promise<ApiResult<T>> {
  const accepted = successes.includes(response.status as SuccessStatus);
  const knownError = contract
    ? contract.errors.has(response.status)
    : errorStatuses.includes(response.status as ErrorStatus);
  if (!accepted && !knownError) return { kind: 'unexpected-status', status: response.status };
  try {
    const value = await decodeResponseBody(response, contract, signal, limits);
    const result = buildResult(response.status, knownError, contract, parse, value);
    const metadata = contract
      ? responseMetadata(response.headers, contract.metadata.get(response.status)!, contract.auth)
      : undefined;
    if (signal?.aborted || (accept && !accept(result, metadata!))) throw new Error('Response not accepted');
    return result;
  } catch {
    // Never retain raw bodies, schema diagnostics or credentials in transport errors.
    return { kind: 'invalid-response', status: response.status };
  }
}

/** Stateless transport. Dispatched unsafe failures imply unknown outcome; never retry. */
export function createApiClient(
  options: { readonly origin: string; readonly fetch?: typeof fetch } & Partial<ResponseJSONLimits>,
) {
  const origin = new URL(options.origin);
  if (!['https:', 'http:'].includes(origin.protocol) || origin.origin !== options.origin) {
    throw new Error('An HTTP(S) origin without a path or credentials is required');
  }
  const fetcher = options.fetch ?? globalThis.fetch.bind(globalThis);
  const limits = responseJSONLimits(options);
  return Object.freeze({
    responseContractVersion: 1 as const,
    async request<T>(request: ApiRequest<T>): Promise<ApiResult<T>> {
      const signal = request.signal;
      const prepared = prepareRequest(request, origin.origin);
      if (!prepared) return { kind: 'invalid-request' };
      const { url, init, contract, parse, accept, successes } = prepared;
      let response: Response;
      try {
        response = await fetcher(url, init);
      } catch {
        // A stopped/lost write request cannot establish whether the owner committed it.
        return {
          kind: 'transport-error',
          outcome: (request.method ?? 'GET') === 'GET' ? 'unavailable' : 'unknown',
          aborted: signal?.aborted ?? false,
        };
      }
      if (response.redirected || (response.url !== '' && response.url !== url.href)) {
        return { kind: 'invalid-response', status: response.status };
      }
      return interpretResponse(response, successes, contract, parse, accept, signal, limits);
    },
  });
}
