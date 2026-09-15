/** M-I01 placement only. T-590 owns validated feature discovery and binding.
 * This catalog grants no permissions and currently registers zero pages. */
export const navigation = Object.freeze([
  { id: 'overview', label: 'Обзор' },
  { id: 'applications', label: 'Заявки' },
  { id: 'settings', label: 'Настройки' },
] as const);
export const registeredPages: readonly never[] = Object.freeze([]);
