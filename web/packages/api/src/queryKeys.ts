import { isId, isRevision } from './client';

export type App = 'realization' | 'financing' | 'insurance' | 'admin';
export type Owner = 'identity' | 'inventory' | 'commerce' | 'retail' | 'financing' | 'insurance' | 'documents';
export type FilterValue =
  null | boolean | number | string | readonly FilterValue[] | { readonly [key: string]: FilterValue };
export interface QueryScope {
  readonly app: App;
  readonly userId: string;
  readonly contextRevision: string;
  readonly companyId: string | null;
  readonly branchScope: { readonly mode: 'ALL' | 'SELECTED'; readonly branchIds: readonly string[] };
}
export interface QueryResource {
  readonly owner: Owner;
  readonly resource: string;
  readonly id?: string;
  readonly filters?: Readonly<Record<string, FilterValue>>;
  readonly cursor?: string | null;
}

function copyFilter(value: FilterValue, ancestors = new Set<object>()): FilterValue {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return value;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value !== 'object' || ancestors.has(value)) throw new Error('Filters must be acyclic JSON values');
  ancestors.add(value);
  let result: FilterValue;
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index++) {
      if (!Object.hasOwn(value, index)) throw new Error('Filters must not contain sparse arrays');
    }
    result = Object.freeze(value.map((item) => copyFilter(item, ancestors)));
  } else {
    if (Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null) {
      throw new Error('Filters must be plain JSON objects');
    }
    result = Object.freeze(
      Object.fromEntries(
        Object.entries(value)
          .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
          .map(([key, item]) => [key, copyFilter(item, ancestors)]),
      ),
    );
  }
  ancestors.delete(value);
  return result;
}

function assertValidScopeAndResource(scope: QueryScope, resource: QueryResource) {
  if (
    !['realization', 'financing', 'insurance', 'admin'].includes(scope.app) ||
    !isId(scope.userId) ||
    !isRevision(scope.contextRevision) ||
    (scope.companyId !== null && !isId(scope.companyId)) ||
    !['identity', 'inventory', 'commerce', 'retail', 'financing', 'insurance', 'documents'].includes(resource.owner) ||
    !resource.resource ||
    (resource.id !== undefined && !isId(resource.id)) ||
    (resource.cursor !== undefined && resource.cursor !== null && typeof resource.cursor !== 'string')
  ) {
    throw new Error('Invalid query scope or resource');
  }
}

function assertValidBranchScope(scope: QueryScope) {
  const { mode, branchIds } = scope.branchScope;
  if (
    !['ALL', 'SELECTED'].includes(mode) ||
    !Array.isArray(branchIds) ||
    (mode === 'ALL' && branchIds.length !== 0) ||
    (mode === 'SELECTED' && (branchIds.length === 0 || scope.companyId === null))
  ) {
    throw new Error('Invalid branch scope');
  }
  // Array.every skips holes; SELECTED must contain actual UUID entries at every index.
  for (let index = 0; index < branchIds.length; index++) {
    if (!Object.hasOwn(branchIds, index) || !isId(branchIds[index])) {
      throw new Error('Invalid branch scope');
    }
  }
}

/** In-memory identity only, never authorization. Session consumers own epoch cancellation. */
export function queryKey(scope: QueryScope, resource: QueryResource) {
  assertValidScopeAndResource(scope, resource);
  assertValidBranchScope(scope);
  const branches = Object.freeze([...new Set(scope.branchScope.branchIds.map((id) => id.toLowerCase()))].sort());
  return Object.freeze([
    scope.app,
    scope.userId.toLowerCase(),
    scope.contextRevision,
    scope.companyId?.toLowerCase() ?? null,
    branches,
    resource.owner,
    resource.resource,
    resource.id?.toLowerCase() ?? null,
    copyFilter(resource.filters ?? {}),
    resource.cursor ?? null,
  ] as const);
}
