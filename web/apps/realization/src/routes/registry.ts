/** M-R01 placement only. T-588 owns validated feature discovery and binding.
 * This catalog grants no permissions and currently registers zero pages. */
export const navigation = Object.freeze([
  { id: 'dashboard', label: 'Дашборд', group: 'Обзор', icon: 'dashboard' },
  { id: 'partners', label: 'Партнёры', group: 'Работа', icon: 'people' },
  { id: 'warehouses', label: 'Склады', group: 'Работа', icon: 'warehouse' },
  { id: 'vehicles', label: 'Автомобили', group: 'Работа', icon: 'car' },
  { id: 'purchases', label: 'Закупки', group: 'Работа', icon: 'cart' },
  { id: 'listings', label: 'Предложения', group: 'Работа', icon: 'tag' },
  { id: 'sales', label: 'Продажи', group: 'Работа', icon: 'cart' },
  { id: 'crm', label: 'CRM', group: 'Работа', icon: 'people' },
  { id: 'billing', label: 'Счета и оплаты', group: 'Финансы', icon: 'wallet' },
  { id: 'insurance', label: 'Страхование', group: 'Финансы', icon: 'shield' },
  { id: 'settings', label: 'Настройки', group: 'bottom', icon: 'settings' },
] as const);
export const registeredPages: readonly never[] = Object.freeze([]);
