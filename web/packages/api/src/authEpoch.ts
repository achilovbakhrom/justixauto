import type { ApiRequest, ApiResult, ResponseMetadata, ValidatedResult } from './client';

export type AuthPhase =
  'empty' | 'anonymous' | 'restricted' | 'authenticated' | 'context-unresolved' | 'uncertain' | 'exhausted';
export type AuthPurpose = 'session' | 'login' | 'recovery-request' | 'recovery-complete' | 'revoke-all' | 'logout';
export type AuthInvalidation =
  'login-intent' | 'context-change' | 'revocation' | 'navigation' | 'unmount' | 'uncertainty';
export type AuthAcceptance<Session, Restricted> =
  | { readonly kind: 'anonymous' }
  | { readonly kind: 'session'; readonly session: Session }
  | { readonly kind: 'restricted'; readonly restricted: Restricted }
  | { readonly kind: 'clear' | 'unchanged' | 'invalidate-binding' | 'uncertain' };
export interface AuthOperation<T, Session, Restricted> {
  readonly purpose: AuthPurpose;
  /** Trusted typed adapter: a state precondition, never server authorization. */
  readonly expected: readonly AuthPhase[];
  readonly request: Omit<ApiRequest<T>, 'authExchange'>;
  /** Must return synchronously after all feature semantics have passed. */
  classify(result: ValidatedResult<T>): AuthAcceptance<Session, Restricted>;
}
export interface AuthTransport {
  readonly responseContractVersion: 1;
  request<T>(request: ApiRequest<T>): Promise<ApiResult<T>>;
}
/** Deliberately contains no response body, token, error receipt or one-time value. */
export type AuthOutcome =
  | { readonly kind: 'accepted'; readonly epoch: number; readonly status: number }
  | {
      readonly kind:
        'busy' | 'invalid-request' | 'binding-invalid' | 'unavailable' | 'uncertain' | 'stale' | 'exhausted';
    };

const phases: readonly AuthPhase[] = [
  'empty',
  'anonymous',
  'restricted',
  'authenticated',
  'context-unresolved',
  'uncertain',
  'exhausted',
];
const purposes: readonly AuthPurpose[] = [
  'session',
  'login',
  'recovery-request',
  'recovery-complete',
  'revoke-all',
  'logout',
];
const routes: Readonly<Record<AuthPurpose, RegExp>> = {
  session: /^\/api\/v1\/identity\/session$/,
  login: /^\/api\/v1\/identity\/session\/login$/,
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
  try {
    Reflect.apply(promiseThen, value, [() => undefined, () => undefined]);
    return true;
  } catch {
    return false;
  }
}
function fields(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (discardPromise(value)) throw new Error('Invalid auth binding');
  if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('Invalid auth binding');
  const prototype = Object.getPrototypeOf(value) as unknown;
  if (prototype !== Object.prototype && prototype !== null) throw new Error('Invalid auth binding');
  const descriptors = Object.getOwnPropertyDescriptors(value);
  if (Reflect.ownKeys(descriptors).some((key) => typeof key !== 'string' || !keys.includes(key)))
    throw new Error('Invalid auth binding');
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
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean' ||
    (typeof value === 'number' && Number.isSafeInteger(value))
  )
    return value;
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
  } finally {
    seen.delete(value);
  }
}
const csrfValid = (value: unknown): value is string =>
  typeof value === 'string' && value.length > 0 && value.length <= 4096 && !/[^A-Za-z0-9_-]/.test(value);

interface ParsedAuthOperation<T, Session, Restricted> {
  readonly purpose: AuthPurpose;
  readonly expected: readonly AuthPhase[];
  readonly request: Omit<ApiRequest<T>, 'authExchange'>;
  readonly classify: AuthOperation<T, Session, Restricted>['classify'];
  readonly signal: AbortSignal | undefined;
  readonly preAborted: boolean;
}

/** Validates and normalizes a caller-supplied operation; never throws. */
function parseAuthOperation<T, Session, Restricted>(
  operation: AuthOperation<T, Session, Restricted>,
): ParsedAuthOperation<T, Session, Restricted> | undefined {
  try {
    const captured = fields(operation, ['purpose', 'expected', 'request', 'classify']);
    const purpose = captured.purpose as AuthPurpose;
    if (
      !purposes.includes(purpose) ||
      !Array.isArray(captured.expected) ||
      captured.expected.length === 0 ||
      captured.expected.some((item: unknown) => !phases.includes(item as AuthPhase)) ||
      typeof captured.classify !== 'function' ||
      !captured.request
    ) {
      discardPromise(captured.classify);
      throw new Error('Invalid auth operation');
    }
    const expected = [...captured.expected] as AuthPhase[];
    const classify = captured.classify as ParsedAuthOperation<T, Session, Restricted>['classify'];
    const request = fields(captured.request, [
      'path',
      'method',
      'schema',
      'successStatuses',
      'body',
      'contextRevision',
      'ifMatch',
      'headers',
      'signal',
      'responseContract',
    ]) as unknown as ParsedAuthOperation<T, Session, Restricted>['request'];
    const signal = request.signal;
    const preAborted = signal !== undefined ? (Reflect.apply(signalAborted, signal, []) as boolean) : false;
    if (
      request.responseContract?.auth !== true ||
      typeof request.path !== 'string' ||
      !routes[purpose].test(request.path) ||
      request.ifMatch !== undefined ||
      (purpose === 'session' ? (request.method ?? 'GET') !== 'GET' : request.method !== 'POST')
    )
      throw new Error('Invalid auth request');
    return { purpose, expected, request, classify, signal, preAborted };
  } catch (error) {
    discardPromise(error);
    return undefined;
  }
}

interface AcceptanceTransition<Session, Restricted> {
  readonly next: AuthPhase;
  readonly session: Session | undefined;
  readonly restricted: Restricted | undefined;
  readonly token: string | undefined;
}

function exactKeysForKind(kind: unknown): readonly string[] {
  if (kind === 'session') return ['kind', 'session'];
  if (kind === 'restricted') return ['kind', 'restricted'];
  return ['kind'];
}

function isBindingError<T>(
  value: ValidatedResult<T>,
  purpose: AuthPurpose,
  meta: { readonly csrf?: unknown },
): boolean {
  return (
    value.kind === 'http-error' &&
    ((value.status === 403 && value.receipt.error.code === 'CSRF_REJECTED') ||
      (value.status === 401 && ['SESSION_REQUIRED', 'SESSION_EXPIRED'].includes(value.receipt.error.code))) &&
    !(
      purpose === 'session' &&
      value.status === 401 &&
      value.receipt.error.code === 'SESSION_REQUIRED' &&
      csrfValid(meta.csrf)
    )
  );
}

function transitionBindingError<Session, Restricted>(
  kind: unknown,
  meta: { readonly csrf?: unknown },
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (!['unchanged', 'invalidate-binding'].includes(kind as string) || meta.csrf !== undefined) return false;
  return baseline;
}

function transitionAnonymous<T, Session, Restricted>(
  value: ValidatedResult<T>,
  meta: { readonly csrf?: unknown },
  purpose: AuthPurpose,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (
    purpose !== 'session' ||
    value.kind !== 'http-error' ||
    value.status !== 401 ||
    value.receipt.error.code !== 'SESSION_REQUIRED' ||
    !csrfValid(meta.csrf)
  )
    return false;
  return {
    ...baseline,
    next: 'anonymous',
    session: undefined,
    restricted: undefined,
    token: meta.csrf,
  };
}

function transitionSession<T, Session, Restricted>(
  value: ValidatedResult<T>,
  meta: { readonly csrf?: unknown },
  purpose: AuthPurpose,
  decision: Record<string, unknown>,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (
    value.kind !== 'success' ||
    value.status !== 200 ||
    !['session', 'login', 'revoke-all'].includes(purpose) ||
    !csrfValid(meta.csrf)
  )
    return false;
  return {
    ...baseline,
    next: 'authenticated',
    session: snapshot(decision.session as Session),
    restricted: undefined,
    token: meta.csrf,
  };
}

function transitionRestricted<T, Session, Restricted>(
  value: ValidatedResult<T>,
  meta: { readonly csrf?: unknown },
  purpose: AuthPurpose,
  decision: Record<string, unknown>,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (
    value.kind !== 'success' ||
    value.status !== 200 ||
    !['session', 'login'].includes(purpose) ||
    !csrfValid(meta.csrf)
  )
    return false;
  return {
    ...baseline,
    next: 'restricted',
    session: undefined,
    restricted: snapshot(decision.restricted as Restricted),
    token: (meta.csrf as string | undefined) ?? baseline.token,
  };
}

function transitionClear<T, Session, Restricted>(
  value: ValidatedResult<T>,
  purpose: AuthPurpose,
): AcceptanceTransition<Session, Restricted> | false {
  if (value.kind !== 'success' || value.status !== 204 || !['logout', 'recovery-complete'].includes(purpose))
    return false;
  return { next: 'empty', session: undefined, restricted: undefined, token: undefined };
}

function transitionUncertain<T, Session, Restricted>(
  value: ValidatedResult<T>,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (value.kind !== 'http-error' || value.status !== 503 || value.receipt.error.code !== 'AUTH_OUTCOME_UNKNOWN')
    return false;
  return baseline;
}

function transitionUnchanged<T, Session, Restricted>(
  kind: unknown,
  value: ValidatedResult<T>,
  meta: { readonly csrf?: unknown },
  purpose: AuthPurpose,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (
    kind !== 'unchanged' ||
    meta.csrf !== undefined ||
    purpose === 'logout' ||
    (value.kind === 'success' && (purpose !== 'recovery-request' || value.status !== 202))
  )
    return false;
  return baseline;
}

/** Dispatches to the per-kind acceptance rule; returns false for any rejected/invalid shape. */
function computeAcceptanceTransition<T, Session, Restricted>(
  bindingError: boolean,
  kind: unknown,
  value: ValidatedResult<T>,
  meta: { readonly csrf?: unknown },
  purpose: AuthPurpose,
  decision: Record<string, unknown>,
  baseline: AcceptanceTransition<Session, Restricted>,
): AcceptanceTransition<Session, Restricted> | false {
  if (bindingError) return transitionBindingError(kind, meta, baseline);
  if (kind === 'anonymous') return transitionAnonymous(value, meta, purpose, baseline);
  if (kind === 'session') return transitionSession(value, meta, purpose, decision, baseline);
  if (kind === 'restricted') return transitionRestricted(value, meta, purpose, decision, baseline);
  if (kind === 'clear') return transitionClear(value, purpose);
  if (kind === 'uncertain') return transitionUncertain(value, baseline);
  return transitionUnchanged(kind, value, meta, purpose, baseline);
}

/** Pre-dispatch admission gate; assumes any purpose-triggered invalidation already ran. */
function gateExecution(
  purpose: AuthPurpose,
  expected: readonly AuthPhase[],
  entryPhase: AuthPhase,
  phase: AuthPhase,
  slotBusy: boolean,
  preAborted: boolean,
  requestToken: string | undefined,
): AuthOutcome | undefined {
  if (phase === 'exhausted') return { kind: 'exhausted' };
  if (slotBusy) return { kind: 'busy' };
  // The caller's expectation is about the phase it started from: logout has
  // already moved the live phase to 'uncertain' by the time we get here.
  if (!expected.includes(entryPhase) || preAborted) return { kind: 'invalid-request' };
  if (purpose !== 'session' && !csrfValid(requestToken)) return { kind: 'invalid-request' };
  return undefined;
}

/** Resolves the post-dispatch outcome once the transport promise has settled. */
function resolveSettledOutcome<T>(
  bindingRejected: boolean,
  ticketStillCurrent: boolean,
  epoch: number,
  ticketEpoch: number,
  currentValid: boolean,
  phase: AuthPhase,
  accepted: boolean,
  result: ApiResult<T>,
  prepared: boolean,
  markUncertain: () => void,
): AuthOutcome {
  if (bindingRejected && ticketStillCurrent && epoch === ticketEpoch + 1) {
    return { kind: epoch === Number.MAX_SAFE_INTEGER ? 'exhausted' : 'binding-invalid' };
  }
  if (!currentValid) return { kind: phase === 'uncertain' ? 'uncertain' : 'stale' };
  if (accepted && (result.kind === 'success' || result.kind === 'http-error'))
    return { kind: 'accepted', epoch, status: result.status };
  if (result.kind === 'invalid-request' && !accepted) return { kind: 'invalid-request' };
  if (prepared) {
    markUncertain();
    return { kind: 'uncertain' };
  }
  return { kind: 'unavailable' };
}

function isAcceptEligible(
  prepared: boolean,
  acceptanceAttempted: boolean,
  currentValid: boolean,
  phase: AuthPhase,
  ticketPhase: AuthPhase,
  aborted: boolean,
): boolean {
  return prepared && !acceptanceAttempted && currentValid && phase === ticketPhase && !aborted;
}

function validateAcceptanceMeta(
  metadata: ResponseMetadata,
): { readonly csrf?: unknown; readonly retryAfterSeconds?: unknown } | undefined {
  const meta = fields(metadata, ['csrf', 'retryAfterSeconds']);
  if (meta.csrf !== undefined && !csrfValid(meta.csrf)) return undefined;
  if (
    meta.retryAfterSeconds !== undefined &&
    (!Number.isInteger(meta.retryAfterSeconds) ||
      (meta.retryAfterSeconds as number) < 0 ||
      (meta.retryAfterSeconds as number) > 2147483647)
  )
    return undefined;
  return meta;
}

function isValidDecisionShape<T>(decision: Record<string, unknown>, value: ValidatedResult<T>): boolean {
  const kind = decision.kind;
  const exactKeys = exactKeysForKind(kind);
  if (
    Object.keys(decision).length !== exactKeys.length ||
    Object.keys(decision).some((key) => !exactKeys.includes(key))
  )
    return false;
  return !(
    value.kind === 'http-error' &&
    value.status === 503 &&
    value.receipt.error.code === 'AUTH_OUTCOME_UNKNOWN' &&
    kind !== 'uncertain'
  );
}

/** Cooperative local ordering only: HttpOnly cookies and other tabs remain owner responsibilities. */
export function createAuthEpoch<Session, Restricted>(
  transport: AuthTransport,
  options: { readonly initialEpoch?: number } = {},
) {
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
  let slot:
    { readonly id: symbol; readonly abort: AbortController; readonly unsafe: boolean; prepared: boolean } | undefined;
  function clear(next: AuthPhase): void {
    csrf = undefined;
    session = undefined;
    restricted = undefined;
    phase = next;
  }
  function invalidate(next: 'empty' | 'uncertain'): void {
    if (epoch < Number.MAX_SAFE_INTEGER) epoch += 1;
    clear(epoch === Number.MAX_SAFE_INTEGER ? 'exhausted' : slot?.unsafe && slot.prepared ? 'uncertain' : next);
    slot?.abort.abort();
  }
  function visit<T>(value: T | undefined, visitor: (value: T) => void): boolean {
    if (value === undefined || typeof visitor !== 'function') return false;
    try {
      discardPromise(visitor(value));
      return true;
    } catch (error) {
      discardPromise(error);
      return false;
    }
  }
  return Object.freeze({
    state: () => Object.freeze({ epoch, phase, busy: slot !== undefined }),
    /** Explicit intent/notification; navigation and unmount also discard binding state. */
    invalidate(reason: AuthInvalidation): void {
      invalidate(reason === 'uncertainty' ? 'uncertain' : 'empty');
    },
    withSession(visitor: (value: Session) => void): boolean {
      return phase === 'authenticated' && visit(session, visitor);
    },
    withRestricted(visitor: (value: Restricted) => void): boolean {
      return phase === 'restricted' && visit(restricted, visitor);
    },
    async execute<T>(operation: AuthOperation<T, Session, Restricted>): Promise<AuthOutcome> {
      const parsed = parseAuthOperation(operation);
      if (!parsed) return { kind: 'invalid-request' };
      const { purpose, expected, request, classify, signal, preAborted } = parsed;
      // Logout clears visible memory even while an older fetch keeps the exclusion slot.
      let requestToken = csrf;
      const entryPhase = phase;
      if (purpose === 'logout') invalidate('uncertain');
      const gated = gateExecution(purpose, expected, entryPhase, phase, slot !== undefined, preAborted, requestToken);
      if (gated) return gated;
      const ticketEpoch = epoch;
      const ticketPhase = phase;
      const ticket = {
        id: Symbol('auth-ticket'),
        abort: new AbortController(),
        unsafe: purpose !== 'session',
        prepared: false,
      };
      slot = ticket;
      const current = () => slot === ticket && epoch === ticketEpoch && phase !== 'exhausted';
      let prepared = false;
      let acceptanceAttempted = false;
      let accepted = false;
      let bindingRejected = false;
      const abort = () => {
        if (current()) invalidate('uncertain');
      };
      try {
        if (signal) Reflect.apply(signalAdd, signal, ['abort', abort, { once: true }]);
        const result = (await Reflect.apply(send, transport, [
          {
            ...request,
            signal: ticket.abort.signal,
            authExchange: {
              prepare() {
                if (prepared || !current() || phase !== ticketPhase || ticket.abort.signal.aborted) return undefined;
                prepared = true;
                ticket.prepared = true;
                const value = purpose === 'session' ? {} : { csrf: requestToken };
                requestToken = undefined;
                return value;
              },
              accept(value: ValidatedResult<T>, metadata: ResponseMetadata) {
                if (
                  !isAcceptEligible(
                    prepared,
                    acceptanceAttempted,
                    current(),
                    phase,
                    ticketPhase,
                    ticket.abort.signal.aborted,
                  )
                )
                  return false;
                // Burn acceptance before invoking injected code; reentrant/stale callbacks cannot adopt.
                acceptanceAttempted = true;
                try {
                  const decision = fields(Reflect.apply(classify, operation, [value]), [
                    'kind',
                    'session',
                    'restricted',
                  ]);
                  const meta = validateAcceptanceMeta(metadata);
                  if (!meta) return false;
                  if (!isValidDecisionShape(decision, value)) return false;
                  const kind = decision.kind;
                  const bindingError = isBindingError(value, purpose, meta);
                  const baseline: AcceptanceTransition<Session, Restricted> = {
                    next: phase,
                    session,
                    restricted,
                    token: csrf,
                  };
                  const transition = computeAcceptanceTransition(
                    bindingError,
                    kind,
                    value,
                    meta,
                    purpose,
                    decision,
                    baseline,
                  );
                  if (!transition) return false;
                  if (!current() || phase !== ticketPhase || ticket.abort.signal.aborted) return false;
                  if (bindingError) {
                    // The validated error establishes rejection, not an unknown write.
                    // Keep the ticket's slot until settlement but retire its binding.
                    ticket.prepared = false;
                    bindingRejected = true;
                    invalidate('empty');
                  } else if (kind === 'uncertain') invalidate('uncertain');
                  else {
                    phase = transition.next;
                    session = transition.session;
                    restricted = transition.restricted;
                    csrf = transition.token;
                  }
                  accepted = true;
                  return true;
                } catch (error) {
                  discardPromise(error);
                  return false;
                }
              },
            },
          },
        ])) as ApiResult<T>;
        return resolveSettledOutcome(
          bindingRejected,
          slot === ticket,
          epoch,
          ticketEpoch,
          current(),
          phase,
          accepted,
          result,
          prepared,
          () => invalidate('uncertain'),
        );
      } catch (error) {
        discardPromise(error);
        if (!current()) return { kind: phase === 'uncertain' ? 'uncertain' : 'stale' };
        if (prepared) {
          invalidate('uncertain');
          return { kind: 'uncertain' };
        }
        return { kind: 'invalid-request' };
      } finally {
        requestToken = undefined;
        if (signal) Reflect.apply(signalRemove, signal, ['abort', abort]);
        if (slot === ticket) slot = undefined;
      }
    },
  });
}
