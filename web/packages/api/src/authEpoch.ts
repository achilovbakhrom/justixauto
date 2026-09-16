import type { ApiRequest, ApiResult, ResponseMetadata, ValidatedResult } from './client';

export type AuthPhase = 'empty' | 'anonymous' | 'restricted' | 'authenticated' | 'context-unresolved' | 'uncertain' | 'exhausted';
export type AuthPurpose = 'session' | 'login' | 'challenge-create' | 'challenge-verify' | 'enrollment-start' | 'enrollment-confirm' | 'recovery-request' | 'recovery-complete' | 'revoke-all' | 'logout';
export type AuthInvalidation = 'login-intent' | 'context-change' | 'revocation' | 'navigation' | 'unmount' | 'uncertainty';
export type AuthAcceptance<Session, Restricted, View> =
  | { readonly kind: 'anonymous' }
  | { readonly kind: 'session'; readonly session: Session }
  | { readonly kind: 'restricted'; readonly restricted: Restricted }
  | { readonly kind: 'challenge'; readonly view: View }
  | { readonly kind: 'enrollment-secret'; readonly view: View }
  | { readonly kind: 'enrollment-confirmed'; readonly view: View }
  | { readonly kind: 'clear' | 'unchanged' | 'invalidate-binding' | 'uncertain' };
export interface AuthOperation<T, Session, Restricted, View> {
  readonly purpose: AuthPurpose;
  /** Trusted typed adapter: a state precondition, never server authorization. */
  readonly expected: readonly AuthPhase[];
  readonly request: Omit<ApiRequest<T>, 'authExchange'>;
  /** Must return synchronously after all feature semantics have passed. */
  classify(result: ValidatedResult<T>): AuthAcceptance<Session, Restricted, View>;
}
export interface AuthTransport {
  readonly responseContractVersion: 1;
  request<T>(request: ApiRequest<T>): Promise<ApiResult<T>>;
}
/** Deliberately contains no response body, token, error receipt or one-time value. */
export type AuthOutcome =
  | { readonly kind: 'accepted'; readonly epoch: number; readonly status: number }
  | { readonly kind: 'busy' | 'invalid-request' | 'binding-invalid' | 'unavailable' | 'uncertain' | 'stale' | 'exhausted' };

const phases: readonly AuthPhase[] = ['empty', 'anonymous', 'restricted', 'authenticated', 'context-unresolved', 'uncertain', 'exhausted'];
const purposes: readonly AuthPurpose[] = ['session', 'login', 'challenge-create', 'challenge-verify', 'enrollment-start', 'enrollment-confirm', 'recovery-request', 'recovery-complete', 'revoke-all', 'logout'];
const routes: Readonly<Record<AuthPurpose, RegExp>> = {
  session: /^\/api\/v1\/identity\/session$/,
  login: /^\/api\/v1\/identity\/session\/login$/,
  'challenge-create': /^\/api\/v1\/identity\/session\/mfa\/challenges$/,
  'challenge-verify': /^\/api\/v1\/identity\/session\/mfa\/verify$/,
  'enrollment-start': /^\/api\/v1\/identity\/session\/mfa\/enrollment$/,
  'enrollment-confirm': /^\/api\/v1\/identity\/session\/mfa\/enrollment\/(?!00000000-0000-0000-0000-000000000000\/)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\/confirm$/,
  'recovery-request': /^\/api\/v1\/identity\/session\/recovery\/request$/,
  'recovery-complete': /^\/api\/v1\/identity\/session\/recovery\/complete$/,
  'revoke-all': /^\/api\/v1\/identity\/session\/revoke-all$/,
  logout: /^\/api\/v1\/identity\/session\/logout$/,
};
const promiseThen = Promise.prototype.then;
const signalAborted = Object.getOwnPropertyDescriptor(AbortSignal.prototype, 'aborted')!.get!;
const signalAdd = AbortSignal.prototype.addEventListener;
const signalRemove = AbortSignal.prototype.removeEventListener;
function discardPromise(value: unknown): boolean {
  // Native cross-realm Promise brand check; never inspect/assimilate thenables.
  try { Reflect.apply(promiseThen, value, [() => undefined, () => undefined]); return true; } catch { return false; }
}
function fields(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (discardPromise(value)) throw new Error('Invalid auth binding');
  if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('Invalid auth binding');
  const prototype = Object.getPrototypeOf(value) as unknown;
  if (prototype !== Object.prototype && prototype !== null) throw new Error('Invalid auth binding');
  const descriptors = Object.getOwnPropertyDescriptors(value);
  if (Reflect.ownKeys(descriptors).some((key) => typeof key !== 'string' || !keys.includes(key))) throw new Error('Invalid auth binding');
  const output: Record<string, unknown> = Object.create(null) as Record<string, unknown>;
  for (const key of Object.keys(descriptors)) {
    const descriptor = descriptors[key]!;
    if (!('value' in descriptor)) throw new Error('Invalid auth binding');
    output[key] = descriptor.value;
  }
  return output;
}
/** Snapshot JSON DTOs without invoking accessors, preserving no caller-owned mutable references. */
function snapshot<T>(value: T, depth = 0, seen = new Set<object>()): T {
  if (discardPromise(value)) throw new Error('Invalid auth data');
  if (value === null || typeof value === 'string' || typeof value === 'boolean'
    || (typeof value === 'number' && Number.isSafeInteger(value))) return value;
  if (typeof value !== 'object' || depth >= 128 || seen.has(value)) throw new Error('Invalid auth data');
  seen.add(value);
  try {
    if (Array.isArray(value)) {
      const descriptors = Object.getOwnPropertyDescriptors(value);
      if (Reflect.ownKeys(descriptors).length !== value.length + 1) throw new Error('Invalid auth data');
      const copy = Array.from({ length: value.length }, (_, index) => {
        const descriptor = descriptors[String(index)];
        if (!descriptor || !('value' in descriptor)) throw new Error('Invalid auth data');
        return snapshot(descriptor.value as unknown, depth + 1, seen);
      });
      return Object.freeze(copy) as T;
    }
    const prototype = Object.getPrototypeOf(value) as unknown;
    if (prototype !== Object.prototype && prototype !== null) throw new Error('Invalid auth data');
    const descriptors = Object.getOwnPropertyDescriptors(value);
    const copy: Record<string, unknown> = Object.create(null) as Record<string, unknown>;
    for (const key of Reflect.ownKeys(descriptors)) {
      if (typeof key !== 'string') throw new Error('Invalid auth data');
      const descriptor = descriptors[key]!;
      if (!('value' in descriptor)) throw new Error('Invalid auth data');
      copy[key] = snapshot(descriptor.value as unknown, depth + 1, seen);
    }
    return Object.freeze(copy) as T;
  } finally { seen.delete(value); }
}
const csrfValid = (value: unknown): value is string => typeof value === 'string' && value.length > 0 && value.length <= 4096 && !/[^A-Za-z0-9_-]/.test(value);

/** Cooperative local ordering only: HttpOnly cookies and other tabs remain owner responsibilities. */
export function createAuthEpoch<Session, Restricted, View>(transport: AuthTransport, options: { readonly initialEpoch?: number } = {}) {
  let epoch = options.initialEpoch ?? 0;
  if (!Number.isSafeInteger(epoch) || epoch < 0) throw new Error('Invalid auth epoch');
  const transportFields = fields(transport, ['responseContractVersion', 'request']);
  if (transportFields.responseContractVersion !== 1 || typeof transportFields.request !== 'function') {
    discardPromise(transportFields.request);
    throw new Error('Invalid auth transport');
  }
  const send = transportFields.request as AuthTransport['request'];
  let phase: AuthPhase = epoch === Number.MAX_SAFE_INTEGER ? 'exhausted' : 'empty';
  let csrf: string | undefined;
  let session: Session | undefined;
  let restricted: Restricted | undefined;
  let view: View | undefined;
  let slot: { readonly id: symbol; readonly abort: AbortController; readonly unsafe: boolean; prepared: boolean } | undefined;
  function clear(next: AuthPhase): void {
    csrf = undefined; session = undefined; restricted = undefined; view = undefined; phase = next;
  }
  function invalidate(next: 'empty' | 'uncertain'): void {
    if (epoch < Number.MAX_SAFE_INTEGER) epoch += 1;
    clear(epoch === Number.MAX_SAFE_INTEGER ? 'exhausted' : slot?.unsafe && slot.prepared ? 'uncertain' : next);
    slot?.abort.abort();
  }
  function visit<T>(value: T | undefined, visitor: (value: T) => void): boolean {
    if (value === undefined || typeof visitor !== 'function') return false;
    try { discardPromise(visitor(value)); return true; } catch (error) { discardPromise(error); return false; }
  }
  return Object.freeze({
    state: () => Object.freeze({ epoch, phase, busy: slot !== undefined, hasView: view !== undefined }),
    /** Explicit intent/notification; navigation and unmount also discard one-time views. */
    invalidate(reason: AuthInvalidation): void { invalidate(reason === 'uncertainty' ? 'uncertain' : 'empty'); },
    clearView(): void { view = undefined; },
    withSession(visitor: (value: Session) => void): boolean { return phase === 'authenticated' && visit(session, visitor); },
    withRestricted(visitor: (value: Restricted) => void): boolean { return phase === 'restricted' && visit(restricted, visitor); },
    withView(visitor: (value: View) => void): boolean { return visit(view, visitor); },
    async execute<T>(operation: AuthOperation<T, Session, Restricted, View>): Promise<AuthOutcome> {
      let purpose: AuthPurpose;
      let expected: readonly AuthPhase[];
      let request: Omit<ApiRequest<T>, 'authExchange'>;
      let signal: AbortSignal | undefined;
      let preAborted = false;
      let classify: AuthOperation<T, Session, Restricted, View>['classify'];
      try {
        const captured = fields(operation, ['purpose', 'expected', 'request', 'classify']);
        purpose = captured.purpose as AuthPurpose;
        if (!purposes.includes(purpose) || !Array.isArray(captured.expected)
          || captured.expected.length === 0 || captured.expected.some((item: unknown) => !phases.includes(item as AuthPhase))
          || typeof captured.classify !== 'function' || !captured.request) {
          discardPromise(captured.classify); throw new Error('Invalid auth operation');
        }
        expected = [...captured.expected] as AuthPhase[];
        classify = captured.classify as typeof classify;
        request = fields(captured.request, ['path', 'method', 'schema', 'successStatuses', 'body', 'contextRevision', 'ifMatch', 'idempotencyKey', 'headers', 'signal', 'responseContract']) as unknown as typeof request;
        signal = request.signal;
        if (signal !== undefined) preAborted = Reflect.apply(signalAborted, signal, []) as boolean;
        if (request.responseContract?.auth !== true || typeof request.path !== 'string' || !routes[purpose].test(request.path)
          || request.ifMatch !== undefined || request.idempotencyKey !== undefined
          || (purpose === 'session' ? (request.method ?? 'GET') !== 'GET' : request.method !== 'POST')) throw new Error('Invalid auth request');
      } catch (error) { discardPromise(error); return { kind: 'invalid-request' }; }
      // Logout clears visible memory even while an older fetch keeps the exclusion slot.
      let requestToken = csrf;
      const expectedState = expected.includes(phase);
      if (purpose === 'logout') invalidate('uncertain');
      if (phase === 'exhausted') return { kind: 'exhausted' };
      if (slot) return { kind: 'busy' };
      if (!expectedState || preAborted) return { kind: 'invalid-request' };
      if (purpose !== 'session' && !csrfValid(requestToken)) return { kind: 'invalid-request' };
      const ticketEpoch = epoch;
      const ticketPhase = phase;
      const ticket = { id: Symbol('auth-ticket'), abort: new AbortController(), unsafe: purpose !== 'session', prepared: false };
      slot = ticket;
      const current = () => slot === ticket && epoch === ticketEpoch && phase !== 'exhausted';
      let prepared = false;
      let acceptanceAttempted = false;
      let accepted = false;
      let bindingRejected = false;
      const abort = () => { if (current()) invalidate('uncertain'); };
      try {
        if (signal) Reflect.apply(signalAdd, signal, ['abort', abort, { once: true }]);
        const result = await Reflect.apply(send, transport, [{ ...request, signal: ticket.abort.signal, authExchange: {
          prepare() {
            if (prepared || !current() || phase !== ticketPhase || ticket.abort.signal.aborted) return undefined;
            prepared = true;
            ticket.prepared = true;
            const value = purpose === 'session' ? {} : { csrf: requestToken };
            requestToken = undefined;
            return value;
          },
          accept(value: ValidatedResult<T>, metadata: ResponseMetadata) {
            if (!prepared || acceptanceAttempted || !current() || phase !== ticketPhase || ticket.abort.signal.aborted) return false;
            // Burn acceptance before invoking injected code; reentrant/stale callbacks cannot adopt.
            acceptanceAttempted = true;
            try {
              const decision = fields(Reflect.apply(classify, operation, [value]), ['kind', 'session', 'restricted', 'view']);
              const meta = fields(metadata, ['csrf', 'retryAfterSeconds']);
              if (meta.csrf !== undefined && !csrfValid(meta.csrf)) return false;
              if (meta.retryAfterSeconds !== undefined && (!Number.isInteger(meta.retryAfterSeconds)
                || (meta.retryAfterSeconds as number) < 0 || (meta.retryAfterSeconds as number) > 2147483647)) return false;
              let next = phase;
              let nextSession = session;
              let nextRestricted = restricted;
              let nextView = view;
              let nextToken = csrf;
              const kind = decision.kind;
              const bindingError = value.kind === 'http-error'
                && ((value.status === 403 && value.receipt.error.code === 'CSRF_REJECTED')
                  || (value.status === 401 && ['SESSION_REQUIRED', 'SESSION_EXPIRED'].includes(value.receipt.error.code)))
                && !(purpose === 'session' && value.status === 401 && value.receipt.error.code === 'SESSION_REQUIRED' && csrfValid(meta.csrf));
              const exactKeys = kind === 'session' ? ['kind', 'session'] : kind === 'restricted' ? ['kind', 'restricted']
                : kind === 'challenge' || kind === 'enrollment-secret' || kind === 'enrollment-confirmed' ? ['kind', 'view'] : ['kind'];
              if (Object.keys(decision).length !== exactKeys.length || Object.keys(decision).some((key) => !exactKeys.includes(key))) return false;
              if (value.kind === 'http-error' && value.status === 503 && value.receipt.error.code === 'AUTH_OUTCOME_UNKNOWN' && kind !== 'uncertain') return false;
              if (bindingError) {
                if (!['unchanged', 'invalidate-binding'].includes(kind as string) || meta.csrf !== undefined) return false;
              } else if (kind === 'anonymous') {
                if (purpose !== 'session' || value.kind !== 'http-error' || value.status !== 401 || value.receipt.error.code !== 'SESSION_REQUIRED' || !csrfValid(meta.csrf)) return false;
                next = 'anonymous'; nextSession = undefined; nextRestricted = undefined; nextView = undefined; nextToken = meta.csrf;
              } else if (kind === 'session') {
                if (value.kind !== 'success' || value.status !== 200 || !['session', 'login', 'challenge-verify', 'revoke-all'].includes(purpose) || !csrfValid(meta.csrf)) return false;
                nextSession = snapshot(decision.session as Session); nextRestricted = undefined; nextView = undefined; nextToken = meta.csrf; next = 'authenticated';
              } else if (kind === 'restricted') {
                if (value.kind !== 'success' || value.status !== 200 || !['session', 'login'].includes(purpose) || !csrfValid(meta.csrf)) return false;
                nextRestricted = snapshot(decision.restricted as Restricted); nextSession = undefined; nextView = undefined; nextToken = meta.csrf ?? csrf; next = 'restricted';
              } else if (kind === 'challenge') {
                if (value.kind !== 'success' || value.status !== 200 || purpose !== 'challenge-create' || phase !== 'authenticated' || meta.csrf !== undefined) return false;
                nextView = snapshot(decision.view as View);
              } else if (kind === 'enrollment-secret' || kind === 'enrollment-confirmed') {
                if (value.kind !== 'success' || value.status !== 200 || purpose !== (kind === 'enrollment-secret' ? 'enrollment-start' : 'enrollment-confirm')) return false;
                nextView = snapshot(decision.view as View);
                if (kind === 'enrollment-confirmed') {
                  if (!csrfValid(meta.csrf)) return false;
                  next = 'context-unresolved'; nextSession = undefined; nextRestricted = undefined; nextToken = meta.csrf;
                }
              } else if (kind === 'clear') {
                if (value.kind !== 'success' || value.status !== 204 || !['logout', 'recovery-complete'].includes(purpose)) return false;
                next = 'empty'; nextSession = undefined; nextRestricted = undefined; nextView = undefined; nextToken = undefined;
              } else if (kind === 'uncertain') {
                if (value.kind !== 'http-error' || value.status !== 503 || value.receipt.error.code !== 'AUTH_OUTCOME_UNKNOWN') return false;
              } else if (kind !== 'unchanged' || meta.csrf !== undefined || purpose === 'logout'
                || (value.kind === 'success' && (purpose !== 'recovery-request' || value.status !== 202))) return false;
              if (!current() || phase !== ticketPhase || ticket.abort.signal.aborted) return false;
              if (bindingError) {
                // The validated error establishes rejection, not an unknown write.
                // Keep the ticket's slot until settlement but retire its binding.
                ticket.prepared = false;
                bindingRejected = true;
                invalidate('empty');
              } else if (kind === 'uncertain') invalidate('uncertain');
              else { phase = next; session = nextSession; restricted = nextRestricted; view = nextView; csrf = nextToken; }
              accepted = true;
              return true;
            } catch (error) { discardPromise(error); return false; }
          },
        } }]) as ApiResult<T>;
        if (bindingRejected && slot === ticket && epoch === ticketEpoch + 1) {
          return { kind: epoch === Number.MAX_SAFE_INTEGER ? 'exhausted' : 'binding-invalid' };
        }
        if (!current()) return { kind: phase === 'uncertain' ? 'uncertain' : 'stale' };
        if (accepted && (result.kind === 'success' || result.kind === 'http-error')) return { kind: 'accepted', epoch, status: result.status };
        if (result.kind === 'invalid-request' && !accepted) return { kind: 'invalid-request' };
        if (prepared) { invalidate('uncertain'); return { kind: 'uncertain' }; }
        return { kind: 'unavailable' };
      } catch (error) {
        discardPromise(error);
        if (!current()) return { kind: phase === 'uncertain' ? 'uncertain' : 'stale' };
        if (prepared) { invalidate('uncertain'); return { kind: 'uncertain' }; }
        return { kind: 'invalid-request' };
      } finally {
        requestToken = undefined;
        if (signal) Reflect.apply(signalRemove, signal, ['abort', abort]);
        if (slot === ticket) slot = undefined;
      }
    },
  });
}
