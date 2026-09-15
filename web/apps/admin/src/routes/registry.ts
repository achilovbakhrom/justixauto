/** M-A01 fixed HTML placement. T-590 will intersect this catalog with validated
 * present feature pages and session visibility; a catalog item grants no access.
 * No absent feature imports or contextual document/payment menu items belong here.
 */
export const navigation = Object.freeze([
  { id: 'overview', label: 'Обзор', group: 'main' },
  { id: 'companies', label: 'Компании', group: 'main' },
  { id: 'users', label: 'Пользователи и доступ', group: 'main' },
  { id: 'roles', label: 'Роли и разрешения', group: 'main' },
  { id: 'mfo', label: 'МФО', group: 'integrations' },
  { id: 'banks', label: 'Банки', group: 'integrations' },
  { id: 'insurers', label: 'Страховые компании', group: 'integrations' },
  { id: 'audit', label: 'Журнал действий', group: 'audit' },
] as const);

// This shell fragment registers zero business pages. Feature binding and its
// validation are owned by T-590, not inferred from the reference demo routes.
export const registeredPages: readonly never[] = Object.freeze([]);
