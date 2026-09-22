import { errorMessages } from './messages';

/**
 * Same-origin JSON calls to /api/v1 following the backend HTTP contract:
 * session cookie + X-CSRF-Token, Idempotency-Key on POST, If-Match for
 * existing resources, {data,revision} / {items} envelopes and
 * {error:{code,message,fields,traceId}} errors.
 */

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: Record<string, string>;
  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

let csrf = '';
let onUnauthenticated: (() => void) | undefined;
let stepUpHandler: (() => Promise<boolean>) | undefined;

/** Registers the prompt that re-confirms the second factor (returns true when confirmed). */
export function setStepUpHandler(handler: (() => Promise<boolean>) | undefined) { stepUpHandler = handler; }

/** The session layer keeps the CSRF token current and reacts to 401. */
export function configureHttp(options: { onUnauthenticated: () => void }) {
  onUnauthenticated = options.onUnauthenticated;
}
export function setCsrf(token: string) { csrf = token; }

export interface Envelope<T> { data: T; revision: string }

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export interface CallOptions {
  ifMatch?: string;
  contextRevision?: string;
  /** Reuse the same key when retrying the same POST. */
  idempotencyKey?: string;
  retried?: boolean;
}

async function send(method: Method, path: string, body: BodyInit | undefined, contentType: string | undefined, options: CallOptions = {}): Promise<{ json: unknown; response: Response }> {
  const headers = new Headers({ Accept: 'application/json' });
  if (contentType) headers.set('Content-Type', contentType);
  if (method !== 'GET' && csrf) headers.set('X-CSRF-Token', csrf);
  if (method === 'POST') headers.set('Idempotency-Key', options.idempotencyKey ?? crypto.randomUUID());
  if (options.ifMatch !== undefined) headers.set('If-Match', `"${options.ifMatch}"`);
  if (options.contextRevision !== undefined) headers.set('X-Context-Revision', options.contextRevision);
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, { method, headers, body: body ?? null, credentials: 'same-origin', cache: 'no-store', redirect: 'error' });
  } catch {
    throw new ApiError(0, 'network', method === 'GET'
      ? 'Сервер недоступен. Проверьте соединение.'
      : 'Нет ответа от сервера: результат неизвестен. Обновите данные перед повтором.');
  }
  const token = response.headers.get('X-CSRF-Token');
  if (token) csrf = token;
  if (response.status === 204) return { json: null as unknown, response };
  let json: unknown = null;
  try { json = await response.json(); } catch { /* non-JSON error */ }
  if (!response.ok) {
    const e = (json as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | null)?.error;
    // A stale second factor: ask for a code once, then repeat the same request.
    if (e?.code === 'mfa_required' && stepUpHandler && !options.retried && await stepUpHandler()) {
      return send(method, path, body, contentType, { ...options, retried: true });
    }
    if (response.status === 401 && !path.startsWith('/identity/session')) onUnauthenticated?.();
    const code = e?.code ?? 'http_' + response.status;
    throw new ApiError(response.status, code, errorMessages[code] ?? e?.message ?? response.statusText, e?.fields ?? {});
  }
  return { json, response };
}

export async function call<T>(method: Method, path: string, body?: unknown, options?: CallOptions): Promise<Envelope<T>> {
  const { json } = await send(method, path, body === undefined ? undefined : JSON.stringify(body),
    body === undefined ? undefined : 'application/json', options);
  return (json ?? { data: null, revision: '' }) as Envelope<T>;
}

export const get = <T>(path: string) => call<T>('GET', path);
export const post = <T>(path: string, body: unknown = {}, options?: CallOptions) => call<T>('POST', path, body, options);
export const patch = <T>(path: string, body: unknown, options?: CallOptions) => call<T>('PATCH', path, body, options);
export const put = <T>(path: string, body: unknown, options?: CallOptions) => call<T>('PUT', path, body, options);

export async function list<T>(path: string): Promise<T[]> {
  const { json } = await send('GET', path, undefined, undefined);
  return ((json as { items?: T[] } | null)?.items ?? []);
}

/** Uploads a file (multipart) to the documents module; returns its metadata. */
export async function upload(file: File, purpose: string): Promise<FileInfo> {
  const form = new FormData();
  form.set('purpose', purpose);
  form.set('file', file);
  const { json } = await send('POST', '/documents/files', form, undefined);
  return (json as Envelope<FileInfo>).data;
}

export interface FileInfo { id: string; fileName: string; mime: string; byteLength: number; sensitive: boolean }

export const fileUrl = (id: string) => `/api/v1/documents/files/${id}/content`;

export function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    const fields = Object.entries(error.fields).map(([k, v]) => `${k}: ${v}`).join('; ');
    return fields ? `${error.message} — ${fields}` : error.message;
  }
  return error instanceof Error ? error.message : String(error);
}
