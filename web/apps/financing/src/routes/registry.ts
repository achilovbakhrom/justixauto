/** M-F01 placement only. T-589 owns validated feature discovery and binding.
 * This catalog grants no permissions and currently registers zero pages. */
export const navigation = Object.freeze([
  { id: 'overview', label: 'Обзор', group: 'Работа' },
  { id: 'applications', label: 'Заявки', group: 'Работа' },
  { id: 'programs', label: 'Программы', group: 'Работа' },
  { id: 'partners', label: 'Партнёры', group: 'Работа' },
  { id: 'settings', label: 'Настройки', group: 'Работа' },
] as const);
export const registeredPages: readonly never[] = Object.freeze([]);
