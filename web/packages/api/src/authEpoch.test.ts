import { describe, expect, it, vi } from 'vitest';
import { runInNewContext } from 'node:vm';
import { createApiClient } from './client';
import type { ApiRequest, AuthExchange, ErrorReceipt } from './client';
import { createAuthEpoch } from './authEpoch';
import type { AuthAcceptance, AuthOperation, AuthPhase, AuthPurpose, AuthTransport } from './authEpoch';

type DTO = { value: string };
type Decision = AuthAcceptance<DTO, DTO, DTO>;
type Operation = AuthOperation<DTO, DTO, DTO, DTO>;
const token = 'fixture_csrf';
const body = { value: 'synthetic' };
const phases: readonly AuthPhase[] = ['empty', 'anonymous', 'restricted', 'authenticated', 'context-unresolved', 'uncertain'];
const receipt = (code: string): ErrorReceipt => ({ error: { code, message: 'Unavailable', fields: {}, traceId: 'fixture' } });
function response(status = 200, data: unknown = body, csrf = token): Response {
  return new Response(status === 204 ? null : JSON.stringify(data), {
    status, headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', ...(csrf ? { 'X-CSRF-Token': csrf } : {}) },
  });
}
function operation(purpose: AuthPurpose, decision: Decision, status: 200 | 202 | 204 = 200, csrf = true): Operation {
  const suffix: Record<AuthPurpose, string> = { session: '', login: '/login', 'challenge-create': '/mfa/challenges', 'challenge-verify': '/mfa/verify', 'enrollment-start': '/mfa/enrollment', 'enrollment-confirm': '/mfa/enrollment/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/confirm', 'recovery-request': '/recovery/request', 'recovery-complete': '/recovery/complete', 'revoke-all': '/revoke-all', logout: '/logout' };
  return {
    purpose, expected: phases,
    request: {
      path: '/api/v1/identity/session' + suffix[purpose], method: purpose === 'session' ? 'GET' : 'POST',
      schema: { parse: (value) => value as DTO }, successStatuses: [status],
      responseContract: {
        auth: true, numeric: 'safe-integers',
        errors: { 401: { parse: (value) => value as ErrorReceipt }, 429: { parse: (value) => value as ErrorReceipt }, 503: { parse: (value) => value as ErrorReceipt } },
        metadata: {
          [status]: { csrf: csrf ? 'required' : 'forbidden', retryAfter: 'forbidden' },
          401: { csrf: purpose === 'session' ? 'required' : 'forbidden', retryAfter: 'forbidden' },
          429: { csrf: 'forbidden', retryAfter: 'required' },
          503: { csrf: 'forbidden', retryAfter: 'forbidden' },
        },
      },
    },
    classify: () => decision,
  };
}
function setup() {
  const fetcher = vi.fn<typeof fetch>();
  const controller = createAuthEpoch<DTO, DTO, DTO>(createApiClient({ origin: 'https://justix.test', fetch: fetcher }));
  const acquire = async (kind: 'anonymous' | 'session' | 'restricted' = 'anonymous') => {
    fetcher.mockResolvedValueOnce(kind === 'anonymous' ? response(401, receipt('SESSION_REQUIRED')) : response());
    return controller.execute(operation('session', kind === 'anonymous' ? { kind } : kind === 'session' ? { kind, session: body } : { kind, restricted: body }));
  };
  return { controller, fetcher, acquire };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

describe('memory-only auth epochs', () => {
  it('acquires anonymous CSRF without admitting a user or exposing response data', async () => {
    const { controller, fetcher, acquire } = setup();
    expect(await acquire()).toEqual({ kind: 'accepted', epoch: 0, status: 401 });
    expect(controller.state()).toEqual({ epoch: 0, phase: 'anonymous', busy: false, hasView: false });
    expect(controller.withSession(vi.fn())).toBe(false);
    expect(JSON.stringify(controller.state())).not.toContain(token);
    fetcher.mockResolvedValueOnce(response());
    expect(await controller.execute(operation('login', { kind: 'session', session: body }))).toEqual({ kind: 'accepted', epoch: 0, status: 200 });
    expect(new Headers(fetcher.mock.calls[1]![1]!.headers).get('X-CSRF-Token')).toBe(token);
    const visitor = vi.fn();
    expect(controller.withSession(visitor)).toBe(true);
    expect(visitor).toHaveBeenCalledWith(body);
  });

  it('restricted responses never admit protected content', async () => {
    const { controller, acquire } = setup();
    await acquire('restricted');
    expect(controller.state().phase).toBe('restricted');
    expect(controller.withSession(vi.fn())).toBe(false);
    expect(controller.withRestricted(vi.fn())).toBe(true);
  });

  it('captures and deeply freezes session values independently of callback-owned objects', async () => {
    const { controller, fetcher } = setup();
    const original = { value: 'before', nested: { list: ['kept'] } };
    fetcher.mockResolvedValueOnce(response());
    await controller.execute(operation('session', { kind: 'session', session: original }));
    original.value = 'after'; original.nested.list[0] = 'changed';
    expect(controller.withSession((value) => {
      expect(value).toEqual({ value: 'before', nested: { list: ['kept'] } });
      expect(Object.isFrozen(value)).toBe(true);
      expect(() => { value.value = 'write'; }).toThrow();
    })).toBe(true);
  });

  it('serializes acquisition reads and retains exclusion until an aborted fetch actually settles', async () => {
    const { controller, fetcher } = setup();
    const pending = deferred<Response>();
    fetcher.mockReturnValueOnce(pending.promise);
    const first = controller.execute(operation('session', { kind: 'session', session: body }));
    expect(await controller.execute(operation('session', { kind: 'session', session: body }))).toEqual({ kind: 'busy' });
    controller.invalidate('context-change');
    expect((fetcher.mock.calls[0]![1]!.signal as AbortSignal).aborted).toBe(true);
    expect(controller.state().busy).toBe(true);
    expect(await controller.execute(operation('session', { kind: 'session', session: body }))).toEqual({ kind: 'busy' });
    pending.resolve(response());
    expect(await first).toEqual({ kind: 'stale' });
    expect(controller.state()).toMatchObject({ phase: 'empty', busy: false, epoch: 1 });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('logout clears memory immediately but reports busy while an older exchange is unresolved', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire('session');
    const pending = deferred<Response>(); fetcher.mockReturnValueOnce(pending.promise);
    const read = controller.execute(operation('session', { kind: 'session', session: body }));
    expect(await controller.execute(operation('logout', { kind: 'clear' }, 204, false))).toEqual({ kind: 'busy' });
    expect(controller.state()).toMatchObject({ phase: 'uncertain', epoch: 1, busy: true });
    expect(controller.withSession(vi.fn())).toBe(false);
    pending.resolve(response()); await read;
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(await controller.execute(operation('logout', { kind: 'clear' }, 204, false))).toEqual({ kind: 'invalid-request' });
  });

  it('logout uses the old token only in its private new-epoch exchange and never reacquires automatically', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire('session');
    const pending = deferred<Response>(); fetcher.mockReturnValueOnce(pending.promise);
    const logout = controller.execute(operation('logout', { kind: 'clear' }, 204, false));
    expect(controller.state()).toMatchObject({ epoch: 1, phase: 'uncertain', busy: true });
    expect(controller.withSession(vi.fn())).toBe(false);
    expect(new Headers(fetcher.mock.calls[1]![1]!.headers).get('X-CSRF-Token')).toBe(token);
    pending.resolve(response(204, undefined, ''));
    expect(await logout).toEqual({ kind: 'accepted', epoch: 1, status: 204 });
    expect(controller.state()).toMatchObject({ phase: 'empty', busy: false });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it.each(['transport', 'bad-json', 'bad-header', 'undeclared'] as const)('a dispatched auth write with %s becomes uncertain without replay', async (failure) => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    if (failure === 'transport') fetcher.mockRejectedValueOnce(new Error('private diagnostic'));
    else if (failure === 'bad-json') fetcher.mockResolvedValueOnce(new Response('{', { headers: { 'Content-Type': 'application/json' } }));
    else fetcher.mockResolvedValueOnce(response(failure === 'undeclared' ? 500 : 200, body, ''));
    expect(await controller.execute(operation('login', { kind: 'session', session: body }))).toEqual({ kind: 'uncertain' });
    expect(controller.state()).toMatchObject({ epoch: 1, phase: 'uncertain', hasView: false });
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(await acquire()).toEqual({ kind: 'accepted', epoch: 1, status: 401 });
  });

  it('validated AUTH_OUTCOME_UNKNOWN invalidates and cannot be treated as an ordinary rejection', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    fetcher.mockResolvedValueOnce(response(503, receipt('AUTH_OUTCOME_UNKNOWN'), ''));
    expect(await controller.execute(operation('login', { kind: 'uncertain' }))).toEqual({ kind: 'uncertain' });
    expect(controller.state().phase).toBe('uncertain');
  });

  it('validated throttling preserves binding without queuing or returning the receipt', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    const reply = response(429, receipt('RATE_LIMITED'), ''); reply.headers.set('Retry-After', '5');
    fetcher.mockResolvedValueOnce(reply);
    expect(await controller.execute(operation('login', { kind: 'unchanged' }))).toEqual({ kind: 'accepted', epoch: 0, status: 429 });
    expect(controller.state().phase).toBe('anonymous'); expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it('keeps enrollment values only in the current view and requires explicit session reacquisition after confirmation', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire('restricted');
    fetcher.mockResolvedValueOnce(response(200, body, ''));
    await controller.execute(operation('enrollment-start', { kind: 'enrollment-secret', view: { value: 'one-time-secret' } }, 200, false));
    const secret = vi.fn(); expect(controller.withView(secret)).toBe(true);
    expect(secret).toHaveBeenCalledWith({ value: 'one-time-secret' });
    controller.clearView(); expect(controller.withView(vi.fn())).toBe(false);
    fetcher.mockResolvedValueOnce(response());
    await controller.execute(operation('enrollment-confirm', { kind: 'enrollment-confirmed', view: { value: 'one-time-codes' } }));
    expect(controller.state()).toMatchObject({ phase: 'context-unresolved', hasView: true });
    expect(controller.withSession(vi.fn())).toBe(false);
    expect(fetcher).toHaveBeenCalledTimes(3);
    await acquire('session');
    expect(controller.state()).toMatchObject({ phase: 'authenticated', hasView: false });
  });

  it.each(['login-intent', 'context-change', 'revocation', 'navigation', 'unmount', 'uncertainty'] as const)('%s invalidates binding and all views', async (reason) => {
    const { controller, acquire } = setup(); await acquire('session');
    controller.invalidate(reason);
    expect(controller.state()).toMatchObject({ epoch: 1, phase: reason === 'uncertainty' ? 'uncertain' : 'empty', hasView: false });
    expect(controller.withSession(vi.fn())).toBe(false);
  });

  it('fails closed at safe-integer epoch exhaustion', async () => {
    const fetcher = vi.fn<typeof fetch>();
    const controller = createAuthEpoch(createApiClient({ origin: 'https://justix.test', fetch: fetcher }), { initialEpoch: Number.MAX_SAFE_INTEGER - 1 });
    controller.invalidate('revocation'); controller.invalidate('revocation');
    expect(controller.state()).toMatchObject({ epoch: Number.MAX_SAFE_INTEGER, phase: 'exhausted' });
    expect(await controller.execute(operation('session', { kind: 'anonymous' }))).toEqual({ kind: 'exhausted' });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('rejects wrong state, missing token and pre-aborted work without fetching', async () => {
    const { controller, fetcher } = setup();
    expect(await controller.execute(operation('login', { kind: 'session', session: body }))).toEqual({ kind: 'invalid-request' });
    const read = operation('session', { kind: 'anonymous' });
    expect(await controller.execute({ ...read, expected: ['authenticated'] })).toEqual({ kind: 'invalid-request' });
    expect(await controller.execute({ ...read, request: { ...read.request, signal: AbortSignal.abort() } })).toEqual({ kind: 'invalid-request' });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('does not adopt a reentrant classifier result after invalidation', async () => {
    const { controller, fetcher } = setup(); fetcher.mockResolvedValueOnce(response());
    const read = operation('session', { kind: 'session', session: body });
    expect(await controller.execute({ ...read, classify() { controller.invalidate('revocation'); return { kind: 'session', session: body }; } })).toEqual({ kind: 'stale' });
    expect(controller.state().phase).toBe('empty'); expect(controller.withSession(vi.fn())).toBe(false);
  });

  it.each(['local', 'foreign', 'thenable', 'accessor', 'cycle'] as const)('invalid %s classification cannot install memory', async (kind) => {
    const { controller, fetcher } = setup(); fetcher.mockResolvedValueOnce(response());
    const getter = vi.fn(() => { throw new Error('must not invoke'); });
    const cycle: Record<string, unknown> = {}; cycle.self = cycle;
    const invalid: unknown = kind === 'local' ? Promise.reject(new Error('private'))
      : kind === 'foreign' ? runInNewContext('Promise.reject(new Error("private"))') as unknown
      : kind === 'thenable' ? Object.defineProperty({}, 'then', { get: getter })
      : kind === 'accessor' ? Object.defineProperty({ kind: 'session' }, 'session', { get: getter })
      : { kind: 'session', session: cycle };
    const read = operation('session', { kind: 'session', session: body });
    expect(await controller.execute({ ...read, classify: () => invalid as Decision })).toEqual({ kind: 'uncertain' });
    expect(controller.withSession(vi.fn())).toBe(false); expect(getter).not.toHaveBeenCalled();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  it('tickets prepare and accept at most once and stale finally cannot release a newer slot', async () => {
    let exchange!: AuthExchange<DTO>;
    const pending = deferred<Awaited<ReturnType<AuthTransport['request']>>>();
    const send = vi.fn((request: ApiRequest<unknown>) => { exchange = request.authExchange as AuthExchange<DTO>; return pending.promise; });
    const controller = createAuthEpoch<DTO, DTO, DTO>({ responseContractVersion: 1, request: send as AuthTransport['request'] });
    const run = controller.execute(operation('session', { kind: 'session', session: body }));
    expect(exchange.prepare()).toEqual({}); expect(exchange.prepare()).toBeUndefined();
    expect(exchange.accept({ kind: 'success', status: 200, data: body }, { csrf: token })).toBe(true);
    expect(exchange.accept({ kind: 'success', status: 200, data: body }, { csrf: token })).toBe(false);
    controller.invalidate('revocation');
    expect(await controller.execute(operation('session', { kind: 'anonymous' }))).toEqual({ kind: 'busy' });
    pending.resolve({ kind: 'success', status: 200, data: body }); await run;
    expect(controller.state().busy).toBe(false);
  });

  it('checks transport capability and callback descriptors without invoking getters', () => {
    const getter = vi.fn();
    expect(() => createAuthEpoch(Object.defineProperty({}, 'request', { get: getter }) as AuthTransport)).toThrow('Invalid auth binding');
    expect(getter).not.toHaveBeenCalled();
    expect(() => createAuthEpoch({ request: vi.fn() } as unknown as AuthTransport)).toThrow('Invalid auth transport');
  });

  it('step-up challenge creation retains the old session assurance until verified', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire('session');
    fetcher.mockResolvedValueOnce(response(200, body, ''));
    expect(await controller.execute(operation('challenge-create', { kind: 'challenge', view: { value: 'challenge-id' } }, 200, false))).toMatchObject({ kind: 'accepted' });
    expect(controller.state()).toMatchObject({ phase: 'authenticated', hasView: true });
    const old = vi.fn(); controller.withSession(old); expect(old).toHaveBeenCalledWith(body);
    fetcher.mockResolvedValueOnce(response(200, body, 'rotated_csrf'));
    expect(await controller.execute(operation('challenge-verify', { kind: 'session', session: { value: 'verified' } }))).toMatchObject({ kind: 'accepted' });
    expect(controller.state().hasView).toBe(false);
    const current = vi.fn(); controller.withSession(current); expect(current).toHaveBeenCalledWith({ value: 'verified' });
  });

  it('recovery acceptance grants no session and completion clears the anonymous binding', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    fetcher.mockResolvedValueOnce(response(202, body, ''));
    expect(await controller.execute(operation('recovery-request', { kind: 'unchanged' }, 202, false))).toEqual({ kind: 'accepted', epoch: 0, status: 202 });
    expect(controller.state().phase).toBe('anonymous');
    fetcher.mockResolvedValueOnce(response(204, undefined, ''));
    expect(await controller.execute(operation('recovery-complete', { kind: 'clear' }, 204, false))).toMatchObject({ kind: 'accepted' });
    expect(controller.state().phase).toBe('empty'); expect(fetcher).toHaveBeenCalledTimes(3);
  });

  it.each(['path', 'query', 'method', 'key', 'match', 'signal', 'accessor'] as const)('rejects invalid %s bindings before dispatch', async (invalid) => {
    const { controller, fetcher } = setup();
    const read = operation('session', { kind: 'anonymous' });
    const getter = vi.fn();
    const changes = invalid === 'path' ? { path: '/api/v1/identity/session/login' }
      : invalid === 'query' ? { path: read.request.path + '?x=1' }
      : invalid === 'method' ? { method: 'POST' as const }
      : invalid === 'key' ? { idempotencyKey: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }
      : invalid === 'match' ? { ifMatch: '1' }
      : invalid === 'signal' ? { signal: {} as AbortSignal } : {};
    const request = { ...read.request, ...changes };
    if (invalid === 'accessor') Object.defineProperty(request, 'signal', { get: getter });
    expect(await controller.execute({ ...read, request })).toEqual({ kind: 'invalid-request' });
    expect(fetcher).not.toHaveBeenCalled(); expect(getter).not.toHaveBeenCalled();
  });

  it('an external abort retains its slot and cannot be treated as a known rollback', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    const aborter = new AbortController(); const pending = deferred<Response>();
    fetcher.mockReturnValueOnce(pending.promise);
    const write = operation('login', { kind: 'session', session: body });
    const result = controller.execute({ ...write, request: { ...write.request, signal: aborter.signal } });
    aborter.abort(); expect(controller.state()).toMatchObject({ busy: true, phase: 'uncertain' });
    expect(await controller.execute(operation('session', { kind: 'anonymous' }))).toEqual({ kind: 'busy' });
    pending.reject(new Error('abort'));
    expect(await result).toEqual({ kind: 'uncertain' });
    expect(controller.state()).toMatchObject({ busy: false, phase: 'uncertain' });
  });

  it('a stale dispatched write cannot adopt state after explicit login intent', async () => {
    const { controller, fetcher, acquire } = setup(); await acquire();
    const pending = deferred<Response>(); fetcher.mockReturnValueOnce(pending.promise);
    const write = controller.execute(operation('login', { kind: 'session', session: body }));
    controller.invalidate('login-intent'); pending.resolve(response());
    expect(await write).toEqual({ kind: 'uncertain' });
    expect(controller.state().phase).toBe('uncertain'); expect(controller.withSession(vi.fn())).toBe(false);
  });
});
