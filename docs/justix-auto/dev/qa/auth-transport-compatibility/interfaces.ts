// Independent QA feasibility probe; not application implementation.
import type { ApiRequest, ApiResult, Schema } from '../../../../../web/packages/api/src/client.ts';
type Session = { revision: string; data: {context: {revision: string}} };
interface ProposedSemantics { validateSession(value: Session): void }
const invalid: Session = {revision:'2',data:{context:{revision:'1'}}};
let settled = false;
let promise: Promise<void> | undefined;
const asyncBinding: ProposedSemantics = {
  async validateSession(value) {
    await Promise.resolve();
    settled = true;
    if (value.revision !== value.data.context.revision) throw new Error('semantic mismatch');
  },
};
// No cast is required: an ordinary function returning Promise<void> is also legal.
const ordinaryPromiseBinding: ProposedSemantics = {
  validateSession(value) {
    promise = Promise.resolve().then(() => {
      settled = true;
      if (value.revision !== value.data.context.revision) throw new Error('semantic mismatch');
    });
    void promise.catch(() => undefined); // retain deterministic rejected evidence
    return promise;
  },
};
let sinks = 0;
function proposedFactory(binding: ProposedSemantics): Schema<Session> {
  if (typeof binding?.validateSession !== 'function') throw new Error('missing semantics');
  return {parse(value: unknown) {
    // Structural fixture is deliberately valid. Its cross-field relation is not.
    const v = value as Session;
    binding.validateSession(v);
    return v;
  }};
}
export async function exposePrematureSink() {
  proposedFactory(ordinaryPromiseBinding).parse(invalid);
  sinks++;
  if (settled || sinks !== 1) throw new Error('probe failed to expose premature sink');
  await promise;
}
// This await is expected to reject; runner catches it through a separate import.
void asyncBinding;

type RequestV1<T> = ApiRequest<T> & {responseContract?: {numeric:'safe-integers'}};
declare const oldTransport: { request<T>(request: ApiRequest<T>): Promise<ApiResult<T>> };
// Put assignments in an uncalled function to test actual types without runtime globals.
function markerChecks() {
  const acceptsIgnoredOptions: {request<T>(request: RequestV1<T>):Promise<ApiResult<T>>} = oldTransport;
  // @ts-expect-error Required capability marker excludes the legacy transport.
  const upgraded: {responseContractVersion:1;request<T>(request:RequestV1<T>):Promise<ApiResult<T>>} = oldTransport;
  void acceptsIgnoredOptions; void upgraded;
}
void markerChecks;

interface SynchronousSemantics { validateSession(value: Session): undefined }
// @ts-expect-error A non-void return constraint rejects async at compile time.
const cannotAssign: SynchronousSemantics = asyncBinding;
void cannotAssign;
