/** Seller data: types, query hooks and labels shared by the pages. */
import { canonicalCountry, countries, list, useData, useSession } from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';

export interface Money {
  amountMinor: string;
  currency: string;
}
interface Spec {
  version: string;
  make: string;
  model: string;
  variant: string;
  year: number;
  bodyType: string;
  exteriorColor: string;
  interiorColor: string;
  powertrain: string;
  drivetrain: string;
}
export interface Model {
  id: string;
  specification: Spec;
  versions?: Spec[];
  revision: string;
}
export interface Warehouse {
  id: string;
  branchId: string | null;
  name: string;
  city: string;
  address: string;
  country: { label: string };
  capacity: string;
  occupied: string;
  free: string;
  revision: string;
}
export interface Vehicle {
  id: string;
  vin: string;
  modelId: string;
  modelSpecificationVersion: string;
  placement: { warehouseId: string; placedAt: string } | null;
  reserved: boolean;
  revision: string;
}
export interface Partnership {
  id: string;
  direction: string;
  counterparty: { id: string; name: string; country: string };
  status: string;
  statusReason: string;
  allowedActions: string[];
  revision: string;
}
export interface Branch {
  id: string;
  name: string;
  address: string;
  revision: string;
}
export interface Customer {
  id: string;
  displayName: string;
  phone: string;
  revision: string;
}
interface Line {
  lineId?: string;
  modelId: string;
  quantity: string;
  unitPrice: Money;
}
interface Installment {
  id?: string;
  amount: Money;
  dueDate: string;
}
export interface Terms {
  lines: Line[];
  route: string;
  deliveryTerms: string;
  paymentSchedule: Installment[];
  warrantyTerms: string;
  serviceTerms: string;
}

export const modelName = (m: Model | undefined) =>
  m ? `${m.specification.make} ${m.specification.model} ${m.specification.variant}` : '—';

export const useModels = () => useData(['models'], () => list<Model>('/inventory/vehicle-models?limit=100'));
export const useWarehouses = () => useData(['warehouses'], () => list<Warehouse>('/inventory/warehouses'));
export const useVehicles = (placement = 'any') =>
  useData(['vehicles', placement], () => list<Vehicle>(`/inventory/vehicle-units?placement=${placement}&limit=100`));
export const usePartners = () => useData(['partnerships'], () => list<Partnership>('/commerce/partnerships?limit=100'));
export const useCustomers = () => useData(['customers'], () => list<Customer>('/retail/customers?limit=100'));

export function useModelName() {
  const models = useModels();
  return (id: string) => modelName(models.data?.find((m) => m.id === id));
}

export const routeLabel: Record<string, string> = {
  factory: 'Заводской заказ',
  'foreign-direct': 'Прямая иностранная поставка',
  'in-transit': 'Автомобиль в пути',
  local: 'Локальный остаток',
};

export function useBranches() {
  const s = useSession();
  const id = s.company?.id ?? '';
  return useData(['branches', id], () => list<Branch>(`/identity/companies/${id}/branches`), !!id);
}

interface RetailEvidence {
  id: string;
  amount: Money;
  paidOn: string;
  externalReference: string;
  attachmentIds: string[];
  status: string;
  decisionReason: string;
  revision: string;
}
export interface RetailInvoice {
  id: string;
  purpose: string;
  amount: Money;
  recipientSnapshot: string;
  dueDate: string | null;
  status: string;
  paid: Money;
  pending: Money;
  outstanding: Money;
  paymentEvidence: RetailEvidence[];
  revision: string;
}
export interface Deal {
  id: string;
  branchId: string;
  customer: Customer;
  leadId: string | null;
  vehicleId: string;
  paymentScheme: string;
  price: Money;
  status: string;
  statusReason: string;
  contractSignedOn: string | null;
  contractReference: string;
  contractFileIds: string[];
  registeredOn: string | null;
  plateNumber: string;
  registrationReference: string;
  deliveredAt: string | null;
  invoices?: RetailInvoice[];
  checklist?: {
    contract: boolean;
    vehiclePayment?: boolean;
    insuranceApproved?: boolean;
    firstInstallment?: boolean;
    registrationPaid: boolean;
    registered: boolean;
    policyResolved: boolean;
  };
  allowedActions: string[];
  history?: { type: string; occurredAt: string; reason: string }[];
  revision: string;
  updatedAt: string;
}

export function useDeals() {
  return useData(['deals'], () => list<Deal>('/retail/deals?limit=100'));
}

// ---- commerce ----

export const partnershipLabel: Record<string, string> = {
  requested: 'Запрошено',
  active: 'Активно',
  declined: 'Отклонено',
  withdrawn: 'Отозвано',
  ended: 'Завершено',
};

interface OfferVersion {
  id: string;
  number: number;
  terms: Terms;
  total: Money;
  audience?: { mode: string; partnerCompanyIds: string[] };
  publishedAt: string | null;
}
export interface Offer {
  id: string;
  supplier: { id: string; name: string };
  status: string;
  statusReason: string;
  publishedVersion: OfferVersion | null;
  versions?: OfferVersion[];
  allowedActions: string[];
  revision: string;
}

export const offerLabel: Record<string, string> = { draft: 'Черновик', published: 'Опубликовано', withdrawn: 'Снято' };

interface Quotation {
  id: string;
  number: number;
  terms: Terms;
  total: Money;
  digest: string;
  createdAt: string;
}
export interface RFQ {
  id: string;
  buyer: { id: string; name: string };
  supplier: { id: string; name: string };
  lines: { modelId: string; quantity: string }[];
  status: string;
  statusReason: string;
  quotations: Quotation[];
  allowedActions: string[];
  revision: string;
  updatedAt: string;
}
interface Allocation {
  orderLineId: string;
  vehicleId: string;
  vin: string;
  status: string;
  shipmentId: string | null;
}
export interface Order {
  id: string;
  party: 'buyer' | 'supplier';
  buyer: { name: string };
  supplier: { name: string };
  source: string;
  terms: Terms;
  total: Money;
  status: string;
  statusReason: string;
  allocations: Allocation[];
  shipments: { id: string; route: string; status: string }[];
  addenda: {
    id: string;
    number: number;
    terms: Terms;
    total: Money;
    reason: string;
    proposedBy: string;
    status: string;
    decisionReason: string;
  }[];
  history?: { type: string; occurredAt: string; reason: string }[];
  allowedActions: string[];
  revision: string;
  updatedAt: string;
}

export const rfqLabel: Record<string, string> = {
  draft: 'Черновик',
  sent: 'Отправлен',
  negotiating: 'Согласование',
  accepted: 'Принят',
  declined: 'Отклонён',
  cancelled: 'Отменён',
};

export const orderLabel: Record<string, string> = {
  'awaiting-supplier': 'Ждёт поставщика',
  accepted: 'Принят',
  fulfilling: 'Исполняется',
  completed: 'Выполнен',
  cancelled: 'Отменён',
};
export const orderTone = (s: string) =>
  s === 'completed' ? 'success' : s === 'cancelled' ? 'danger' : s === 'awaiting-supplier' ? 'warning' : 'info';

export interface Shipment {
  id: string;
  route: string;
  status: string;
  vehicles: Allocation[];
  milestones: { milestoneType: string; occurredAt: string; location: string; note: string }[];
  revision: string;
}

interface Evidence {
  id: string;
  amount: Money;
  paidOn: string;
  externalReference: string;
  attachmentIds: string[];
  status: string;
  decisionReason: string;
  allowedActions: string[];
  revision: string;
}
export interface Invoice {
  id: string;
  orderId: string;
  total: Money;
  status: string;
  paid: Money;
  pending: Money;
  outstanding: Money;
  paymentEvidence: Evidence[];
  allowedActions: string[];
  revision: string;
  schedule: { amount: Money; dueDate: string }[];
}

// ---- retail ----

export interface Listing {
  id: string;
  vehicleId: string;
  text: string;
  askingPrice: Money;
  status: string;
  revision: string;
  updatedAt: string;
}
export const listingLabel: Record<string, string> = {
  draft: 'Черновик',
  published: 'Опубликовано',
  withdrawn: 'Снято',
};

export interface Lead {
  id: string;
  customerId: string;
  customer?: Customer;
  branchId: string;
  source: string;
  stage: string;
  assignedUserId: string | null;
  lostReason: string;
  dealId: string | null;
  contacts?: { channel: string; note: string; occurredAt: string }[];
  history?: { type: string; occurredAt: string; reason: string }[];
  revision: string;
  updatedAt: string;
}
export interface Task {
  id: string;
  customerId: string;
  leadId: string | null;
  dealId: string | null;
  ownerUserId: string;
  dueAt: string;
  title: string;
  status: string;
  revision: string;
}

export const stageLabel: Record<string, string> = {
  new: 'Новый',
  contacted: 'Контакт',
  qualified: 'Квалифицирован',
  'test-drive': 'Тест-драйв',
  negotiation: 'Переговоры',
  won: 'Продажа',
  lost: 'Потерян',
};
export const stageOrder = ['new', 'contacted', 'qualified', 'test-drive', 'negotiation'];

/** VIN plus model name for the seller's own vehicles. */
export function useVehicleLabel() {
  const vehicles = useVehicles();
  const models = useModels();
  return (id: string) => {
    const v = vehicles.data?.find((x) => x.id === id);
    return v ? `${v.vin} · ${modelName(models.data?.find((m) => m.id === v.modelId))}` : id.slice(0, 8);
  };
}

export const channelLabel: Record<string, string> = {
  phone: 'Звонок',
  telegram: 'Telegram',
  visit: 'Визит',
  email: 'Письмо',
  other: 'Другое',
};
export const sourceLabel: Record<string, string> = {
  website: 'Сайт',
  telegram: 'Telegram',
  phone: 'Звонок',
  manual: 'Вручную',
};

export const schemeLabel: Record<string, string> = {
  cash: 'Наличные / перевод',
  'own-installment': 'Собственная рассрочка',
  'partner-finance': 'Банк / МФО',
};
export const dealLabel: Record<string, string> = { reserved: 'В работе', delivered: 'Выдан', cancelled: 'Отменена' };

export const purposeLabel: Record<string, string> = {
  'vehicle-payment': 'Оплата автомобиля',
  'first-installment': 'Первый взнос',
  registration: 'Регистрация',
};

// ---- list hooks shared by the pages ----
export const useOrders = () => useData(['orders'], () => list<Order>('/commerce/orders?limit=100'));
export const useRFQs = () => useData(['rfqs'], () => list<RFQ>('/commerce/rfqs?limit=100'));
export const useOffers = (scope: 'own' | 'available') =>
  useData(['offers', scope], () => list<Offer>(`/commerce/offers?scope=${scope}&limit=100`));
export const useListings = () => useData(['listings'], () => list<Listing>('/retail/listings?limit=100'));
export const useLeads = () => useData(['leads', ''], () => list<Lead>('/retail/leads?limit=100'));
export const useTasks = () => useData(['tasks'], () => list<Task>('/retail/tasks?limit=100'));

/** Model names of a set of terms lines, e.g. "Chevrolet Tracker Premier × 2". */
export function useLinesLabel() {
  const name = useModelName();
  return (lines: { modelId: string; quantity: string }[]) =>
    lines.map((l) => `${name(l.modelId)} × ${l.quantity}`).join(', ');
}

/** Exact sum of minor-unit amounts per currency ("150 000.00 USD + 3 000 000.00 UZS"). */
export function sumMoney(items: Money[]): Money[] {
  const by = new Map<string, bigint>();
  for (const m of items) by.set(m.currency, (by.get(m.currency) ?? 0n) + BigInt(m.amountMinor));
  return [...by].map(([currency, v]) => ({ amountMinor: v.toString(), currency }));
}

// ---- warehouse form ----
export const warehouseFields = (w?: Warehouse): FieldSpec[] => [
  { name: 'name', label: 'Название', type: 'text', required: true, initial: w?.name ?? '' },
  {
    name: 'country',
    label: 'Страна',
    type: 'combobox',
    required: true,
    initial: w?.country.label ?? '',
    options: countries(),
    canonicalize: canonicalCountry,
    placeholder: 'Выберите или найдите страну',
    ariaLabel: 'Показать страны',
  },
  { name: 'city', label: 'Город', type: 'text', required: true, initial: w?.city ?? '' },
  { name: 'address', label: 'Адрес', type: 'text', required: true, initial: w?.address ?? '' },
];
export const warehouseInput = (v: Record<string, unknown>) => ({
  name: v.name,
  country: { label: canonicalCountry(String(v.country)) ?? v.country },
  city: v.city,
  address: v.address,
});
