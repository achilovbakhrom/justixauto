/** Feature adapters must reject invalid/unknown DTO fields and return typed data. */
export interface Schema<T> { parse(value: unknown): T }

export const errorStatuses = [400, 401, 403, 404, 409, 412, 422, 428, 503] as const;
export type ErrorStatus = typeof errorStatuses[number];
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
  [S in ErrorStatus]: { readonly kind: 'http-error'; readonly status: S; readonly receipt: ErrorReceipt }
}[ErrorStatus];

export type ApiResult<T> =
  | { readonly kind: 'success'; readonly status: SuccessStatus; readonly data: T }
  | HttpFailure
  | { readonly kind: 'unexpected-status'; readonly status: number }
  | { readonly kind: 'invalid-response'; readonly status: number }
  | { readonly kind: 'invalid-request' }
  | { readonly kind: 'transport-error'; readonly outcome: 'unavailable' | 'unknown'; readonly aborted: boolean };

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
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isRevision(value: unknown): value is string {
  return typeof value === 'string' && /^(0|[1-9][0-9]*)$/.test(value)
    && (value.length < 19 || (value.length === 19 && value <= '9223372036854775807'));
}
export function isId(value: unknown): value is string {
  return typeof value === 'string' && uuid.test(value);
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
function parseError(value: unknown): ErrorReceipt {
  if (!record(value) || !record(value.error)
    || Object.keys(value).some((key) => key !== 'error' && key !== 'operationId')
    || Object.keys(value.error).some((key) => !['code', 'message', 'fields', 'traceId'].includes(key))
    || typeof value.error.code !== 'string' || value.error.code.length === 0
    || typeof value.error.message !== 'string' || typeof value.error.traceId !== 'string'
    || !record(value.error.fields)
    || ('operationId' in value && !isId(value.operationId))) {
    throw new Error('Invalid error receipt');
  }
  return value as unknown as ErrorReceipt;
}

function requestUrl(path: string, origin: string): URL {
  // Reject ambiguous paths before URL normalization; never fetch another app's HTML.
  if (!/^\/api\/v1\/(identity|inventory|commerce|retail|financing|insurance|documents|operations)\//.test(path)
    || /[\\#\s]/.test(path)) throw new Error('Invalid API path');
  const pathPart = path.split('?')[0] ?? '';
  for (const segment of pathPart.split('/')) {
    const decoded = decodeURIComponent(segment);
    if (decoded === '.' || decoded === '..' || /[\\/%]/.test(decoded)
      || [...decoded].some((character) => character.charCodeAt(0) <= 32)) {
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
    if (/^(authorization|proxy-authorization|cookie|host|origin|accept|content-type|idempotency-key|if-match|x-context-revision)$/.test(normalized)
      || /^(sec-|x-(actor|user|company|branch|permissions?|internal|justix-internal|forwarded)(-|$))/.test(normalized)) {
      throw new Error('Reserved header');
    }
  }
  headers.set('Accept', 'application/json');
  if (request.body !== undefined) headers.set('Content-Type', 'application/json');
  for (const [name, value] of [
    ['X-Context-Revision', request.contextRevision], ['If-Match', request.ifMatch],
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

/** Stateless transport: no cache, retries, token store, logging or receipt persistence. */
export function createApiClient(options: { readonly origin: string; readonly fetch?: typeof fetch }) {
  const origin = new URL(options.origin);
  if (!['https:', 'http:'].includes(origin.protocol) || origin.origin !== options.origin) {
    throw new Error('An HTTP(S) origin without a path or credentials is required');
  }
  const fetcher = options.fetch ?? globalThis.fetch.bind(globalThis);
  return {
    async request<T>(request: ApiRequest<T>): Promise<ApiResult<T>> {
      const method = request.method ?? 'GET';
      let url: URL;
      let init: RequestInit;
      try {
        if (!['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].includes(method)
          || request.successStatuses.length === 0
          || request.successStatuses.some((status) => ![200, 201, 202, 204].includes(status))
          || (method === 'GET' && request.body !== undefined)) throw new Error('Invalid request');
        url = requestUrl(request.path, origin.origin);
        init = {
          method, headers: requestHeaders(request), credentials: 'same-origin',
          mode: 'same-origin', redirect: 'error', cache: 'no-store',
          ...(request.signal === undefined ? {} : { signal: request.signal }),
          ...(request.body === undefined ? {} : { body: JSON.stringify(request.body) }),
        };
      } catch {
        return { kind: 'invalid-request' };
      }
      let response: Response;
      try {
        response = await fetcher(url, init);
      } catch {
        // A stopped/lost write request cannot establish whether the owner committed it.
        return { kind: 'transport-error', outcome: method === 'GET' ? 'unavailable' : 'unknown', aborted: request.signal?.aborted ?? false };
      }
      if (response.redirected || (response.url !== '' && response.url !== url.href)) {
        return { kind: 'invalid-response', status: response.status };
      }
      const accepted = request.successStatuses.includes(response.status as SuccessStatus);
      const knownError = errorStatuses.includes(response.status as ErrorStatus);
      if (!accepted && !knownError) return { kind: 'unexpected-status', status: response.status };
      try {
        let value: unknown;
        if (response.status !== 204) {
          if (response.headers.get('Content-Type')?.split(';')[0]?.trim().toLowerCase() !== 'application/json') {
            throw new Error('Expected JSON');
          }
          value = await response.json();
        }
        if (knownError) {
          return { kind: 'http-error', status: response.status as ErrorStatus, receipt: parseError(value) };
        }
        return { kind: 'success', status: response.status as SuccessStatus, data: request.schema.parse(value) };
      } catch {
        // Never retain raw bodies, schema diagnostics or credentials in transport errors.
        return { kind: 'invalid-response', status: response.status };
      }
    },
  };
}
