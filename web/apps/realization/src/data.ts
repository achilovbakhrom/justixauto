/** Seller data: types, query hooks and labels shared by the pages. */
import { list, useData, useSession } from '@justixauto/kit';

export interface Money { amountMinor: string; currency: string }
export interface Spec { version: string; make: string; model: string; variant: string; year: number; bodyType: string; exteriorColor: string; interiorColor: string; powertrain: string; drivetrain: string }
export interface Model { id: string; specification: Spec; versions?: Spec[]; revision: string }
export interface Warehouse { id: string; branchId: string | null; name: string; city: string; address: string; country: { label: string }; capacity: string; occupied: string; free: string; revision: string }
export interface Vehicle { id: string; vin: string; modelId: string; modelSpecificationVersion: string; placement: { warehouseId: string; placedAt: string } | null; revision: string }
export interface Partnership { id: string; direction: string; counterparty: { id: string; name: string; country: string }; status: string; statusReason: string; allowedActions: string[]; revision: string }
export interface Branch { id: string; name: string; address: string; revision: string }
export interface Customer { id: string; displayName: string; phone: string; revision: string }
export interface Line { lineId?: string; modelId: string; quantity: string; unitPrice: Money }
export interface Installment { id?: string; amount: Money; dueDate: string }
export interface Terms { lines: Line[]; route: string; deliveryTerms: string; paymentSchedule: Installment[]; warrantyTerms: string; serviceTerms: string }

export const modelName = (m: Model | undefined) => m ? `${m.specification.make} ${m.specification.model} ${m.specification.variant}` : '—';

export const useModels = () => useData(['models'], () => list<Model>('/inventory/vehicle-models?limit=100'));
export const useWarehouses = () => useData(['warehouses'], () => list<Warehouse>('/inventory/warehouses'));
export const useVehicles = (placement = 'any') => useData(['vehicles', placement], () => list<Vehicle>(`/inventory/vehicle-units?placement=${placement}&limit=100`));
export const usePartners = () => useData(['partnerships'], () => list<Partnership>('/commerce/partnerships?limit=100'));
export const useCustomers = () => useData(['customers'], () => list<Customer>('/retail/customers?limit=100'));

export function useModelName() {
  const models = useModels();
  return (id: string) => modelName(models.data?.find((m) => m.id === id));
}

export function useVin() {
  const vehicles = useVehicles();
  return (id: string) => vehicles.data?.find((v) => v.id === id)?.vin ?? id.slice(0, 8);
}

export const routeLabel: Record<string, string> = { factory: 'Заводской заказ', 'foreign-direct': 'Прямая иностранная поставка', 'in-transit': 'Автомобиль в пути', local: 'Локальный остаток' };



export function useBranches() {
  const s = useSession();
  const id = s.company?.id ?? '';
  return useData(['branches', id], () => list<Branch>(`/identity/companies/${id}/branches`), !!id);
}

export interface RetailEvidence { id: string; amount: Money; paidOn: string; externalReference: string; attachmentIds: string[]; status: string; decisionReason: string; revision: string }
export interface RetailInvoice { id: string; purpose: string; amount: Money; recipientSnapshot: string; dueDate: string | null; status: string; paid: Money; pending: Money; outstanding: Money; paymentEvidence: RetailEvidence[]; revision: string }
export interface Deal {
  id: string; branchId: string; customer: Customer; leadId: string | null; vehicleId: string; paymentScheme: string; price: Money;
  status: string; statusReason: string; contractSignedOn: string | null; contractReference: string; contractFileIds: string[]; registeredOn: string | null;
  plateNumber: string; registrationReference: string; deliveredAt: string | null; invoices?: RetailInvoice[];
  checklist?: { contract: boolean; vehiclePayment?: boolean; insuranceApproved?: boolean; firstInstallment?: boolean; registrationPaid: boolean; registered: boolean; policyResolved: boolean };
  allowedActions: string[]; history?: { type: string; occurredAt: string; reason: string }[]; revision: string; updatedAt: string;
}

export function useDeals() { return useData(['deals'], () => list<Deal>('/retail/deals?limit=100')); }
