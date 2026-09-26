/** Russian names of permission keys (catalog: internal/modules/<module>/model). */
const permissionLabels: Record<string, string> = {
  'platform.companies.create': 'Платформа: создание компаний',
  'platform.companies.access': 'Платформа: доступ и удаление компаний',
  'platform.users.manage': 'Платформа: сотрудники платформы',
  'platform.memberships.manage': 'Платформа: членство в компаниях',
  'platform.roles.manage': 'Платформа: роли и разрешения',
  'platform.directory.read': 'Платформа: справочник компаний',
  'platform.audit.read': 'Платформа: журнал действий',

  'company.create': 'Компания: создание',
  'company.edit': 'Компания: изменение реквизитов',
  'company.users.manage': 'Компания: сотрудники и их роли',
  'branches.create': 'Филиалы: создание',
  'branches.edit': 'Филиалы: изменение',

  'inventory.read': 'Склад: просмотр',
  'inventory.models.edit': 'Склад: модели автомобилей',
  'inventory.warehouses.manage': 'Склад: управление складами',
  'inventory.receipts.create': 'Склад: приёмка автомобилей',
  'inventory.vehicles.move': 'Склад: перемещение автомобилей',

  'commerce.read': 'Закупки: просмотр',
  'commerce.partnerships.manage': 'Закупки: партнёрства',
  'commerce.offers.manage': 'Закупки: предложения',
  'commerce.trade': 'Закупки: оптовые сделки',
  'commerce.payments.accept': 'Закупки: подтверждение оплат',

  'retail.read': 'Продажи: просмотр',
  'retail.crm.manage': 'Продажи: CRM (лиды, клиенты, задачи)',
  'retail.listings.manage': 'Продажи: витрина',
  'retail.deals.manage': 'Продажи: сделки',
  'retail.payments.accept': 'Продажи: приём оплат',
  'retail.deals.deliver': 'Продажи: выдача автомобиля',

  'financing.read': 'Финансирование: просмотр',
  'financing.programs.manage': 'Финансирование: программы',
  'financing.applications.manage': 'Финансирование: подача заявок',
  'financing.applications.agree': 'Финансирование: согласие клиента',
  'financing.applications.review': 'Финансирование: рассмотрение заявок',
  'financing.applications.decide': 'Финансирование: решение по заявкам',

  'insurance.read': 'Страхование: просмотр',
  'insurance.applications.manage': 'Страхование: подача заявок',
  'insurance.applications.review': 'Страхование: рассмотрение заявок',
  'insurance.applications.decide': 'Страхование: решение по заявкам',

  'documents.read': 'Документы: просмотр',
  'documents.upload': 'Документы: загрузка',
  'documents.sensitive.download': 'Документы: скачивание персональных данных',
};

/** The Russian name of a permission key; unknown keys are shown as is. */
export const permissionLabel = (key: string) => permissionLabels[key] ?? key;

/** Options for a permission picker, sorted by their Russian names. */
export const permissionOptions = (keys: string[]): [string, string][] =>
  keys.map((k): [string, string] => [k, permissionLabel(k)]).sort((a, b) => a[1].localeCompare(b[1], 'ru'));
