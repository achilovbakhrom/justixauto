import { useState } from 'react';
import {
  ActionButton, Badge, Details, Modal, Page, Panel, Table, Tabs, dateTime, fileUrl, get, list, money, post, useData, useRefresh, useSession,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { TermsButton, TermsView } from '../shared';
import { modelName, routeLabel, useModelName, useModels, usePartners, useVehicles, useWarehouses } from '../data';
import type { Money, Terms } from '../data';

const reason: FieldSpec[] = [{ name: 'reason', label: 'Причина', type: 'textarea', required: true }];

// ---------------- partners ----------------

const partnershipLabel: Record<string, string> = { requested: 'Запрошено', active: 'Активно', declined: 'Отклонено', withdrawn: 'Отозвано', ended: 'Завершено' };

export function PartnersPage() {
  const q = usePartners();
  const dir = useData(['directory'], () => list<{ id: string; name: string; country: string }>('/identity/directory/companies?kind=seller&limit=100'));
  const s = useSession();
  const refresh = [['partnerships']];
  return <Page title="Партнёры" subtitle="Активное партнёрство открывает предложения, цены и новые сделки"
    actions={<ActionButton label="Запросить партнёрство" variant="primary" refresh={refresh} fields={[
      { name: 'counterpartyCompanyId', label: 'Компания', type: 'select', required: true,
        options: (dir.data ?? []).filter((c) => c.id !== s.company?.id).map((c) => [c.id, `${c.name} (${c.country})`]) },
    ]} onSubmit={(v) => post('/commerce/partnerships', v)} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(p) => p.id} columns={[
      { title: 'Компания', render: (p) => <b>{p.counterparty.name}</b> },
      { title: 'Направление', render: (p) => p.direction === 'incoming' ? 'Входящий' : 'Исходящий' },
      { title: 'Статус', render: (p) => <Badge tone={p.status === 'active' ? 'success' : p.status === 'requested' ? 'warning' : undefined}>{partnershipLabel[p.status]}</Badge> },
      { title: 'Причина', render: (p) => p.statusReason || '—' },
      { title: '', render: (p) => <div className="kit-row">{p.allowedActions.map((a) => a === 'accept'
        ? <ActionButton key={a} label="Принять" refresh={refresh} onSubmit={() => post(`/commerce/partnerships/${p.id}/accept`, {}, { ifMatch: p.revision })} />
        : <ActionButton key={a} label={{ decline: 'Отклонить', withdraw: 'Отозвать', end: 'Завершить' }[a] ?? a} fields={reason} refresh={refresh}
          onSubmit={(v) => post(`/commerce/partnerships/${p.id}/${a}`, v, { ifMatch: p.revision })} />)}</div> },
    ]} /></Panel>
  </Page>;
}

// ---------------- offers ----------------

interface OfferVersion { id: string; number: number; terms: Terms; total: Money; audience?: { mode: string; partnerCompanyIds: string[] }; publishedAt: string | null }
interface Offer { id: string; supplier: { id: string; name: string }; status: string; statusReason: string; publishedVersion: OfferVersion | null; versions?: OfferVersion[]; allowedActions: string[]; revision: string }

const offerLabel: Record<string, string> = { draft: 'Черновик', published: 'Опубликовано', withdrawn: 'Снято' };

export function OffersPage() {
  const [scope, setScope] = useState<'own' | 'available'>('available');
  const q = useData(['offers', scope], () => list<Offer>(`/commerce/offers?scope=${scope}`));
  const partners = usePartners();
  const [open, setOpen] = useState<string | null>(null);
  const active = (partners.data ?? []).filter((p) => p.status === 'active');
  return <Page title="Предложения" subtitle="Публикация не резервирует VIN"
    actions={<TermsButton label="Новое предложение" variant="primary" refresh={[['offers']]}
      extra={[{ name: 'partners', label: 'Только для выбранных партнёров (ничего не выбрано — всем активным)', multiple: true,
        options: active.map((p) => [p.counterparty.id, p.counterparty.name]) }]}
      onSubmit={(terms, x) => {
        const ids = (x.partners as string[] | undefined) ?? [];
        return post('/commerce/offers', { terms, audience: ids.length ? { mode: 'selected', partnerCompanyIds: ids } : { mode: 'all-active', partnerCompanyIds: [] } });
      }} />}>
    <Tabs value={scope} onChange={setScope} tabs={[['available', 'От партнёров'], ['own', 'Мои предложения']]} />
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(o) => o.id} onRowClick={(o) => setOpen(o.id)} columns={[
      { title: 'Поставщик', render: (o) => o.supplier.name },
      { title: 'Версия', render: (o) => o.publishedVersion ? `№${o.publishedVersion.number}` : '—' },
      { title: 'Итого', render: (o) => money(o.publishedVersion?.total ?? o.versions?.[o.versions.length - 1]?.total) },
      { title: 'Статус', render: (o) => <Badge tone={o.status === 'published' ? 'success' : undefined}>{offerLabel[o.status]}</Badge> },
    ]} /></Panel>
    {open && <OfferDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

function OfferDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['offer', id], () => get<Offer>(`/commerce/offers/${id}`));
  const name = useModelName();
  const o = q.data?.data;
  const refresh = [['offer', id], ['offers']];
  const own = !!o?.versions;
  const latest = o?.versions?.[o.versions.length - 1];
  const shown = own ? latest : o?.publishedVersion;
  return <Modal title={o ? `Предложение ${o.supplier.name}` : 'Предложение'} onClose={onClose} size="wide" footer={o && (own ? o.status !== 'withdrawn' && <>
    <TermsButton label="Новая версия" initial={latest?.terms} refresh={refresh} onSubmit={async (terms) => {
      await post(`/commerce/offers/${id}/versions`, { terms, audience: latest?.audience ?? { mode: 'all-active', partnerCompanyIds: [] } }, { ifMatch: o.revision });
    }} />
    {latest && <ActionButton label={`Опубликовать №${latest.number}`} variant="primary" refresh={refresh}
      onSubmit={() => post(`/commerce/offers/${id}/publish`, { offerVersionId: latest.id }, { ifMatch: o.revision })} />}
    <ActionButton label="Снять" variant="danger" fields={reason} refresh={refresh} onSubmit={(v) => post(`/commerce/offers/${id}/withdraw`, v, { ifMatch: o.revision })} />
  </> : o.publishedVersion && <ActionButton label="Заказать" variant="primary" refresh={[['orders']]}
    intro={<p>Укажите количество по строкам (0 — не заказывать). Заказ ждёт подтверждения поставщика.</p>}
    fields={o.publishedVersion.terms.lines.map((l) => ({ name: l.lineId!, label: `${name(l.modelId)} (до ${l.quantity}) × ${money(l.unitPrice)}`, type: 'number', initial: '0' }) as FieldSpec)}
    onSubmit={(v) => post('/commerce/orders', { offerVersionId: o.publishedVersion!.id,
      lines: Object.entries(v).filter(([, q]) => Number(q) > 0).map(([offerLineId, quantity]) => ({ offerLineId, quantity })) })} />)}>
    {shown && <><p><b>Версия №{shown.number}</b> · итого {money(shown.total)}{own && o?.publishedVersion ? ` · опубликована №${o.publishedVersion.number}` : ''}</p>
      <TermsView terms={shown.terms} modelNameOf={name} /></>}
  </Modal>;
}

// ---------------- purchases: RFQs and orders ----------------

interface Quotation { id: string; number: number; terms: Terms; total: Money; digest: string; createdAt: string }
interface RFQ { id: string; buyer: { id: string; name: string }; supplier: { id: string; name: string }; lines: { modelId: string; quantity: string }[]; status: string; statusReason: string; quotations: Quotation[]; allowedActions: string[]; revision: string }
interface Allocation { orderLineId: string; vehicleId: string; vin: string; status: string; shipmentId: string | null }
interface Order {
  id: string; party: 'buyer' | 'supplier'; buyer: { name: string }; supplier: { name: string }; source: string; terms: Terms; total: Money;
  status: string; statusReason: string; allocations: Allocation[]; shipments: { id: string; route: string; status: string }[];
  addenda: { id: string; number: number; terms: Terms; total: Money; reason: string; proposedBy: string; status: string; decisionReason: string }[];
  history?: { type: string; occurredAt: string; reason: string }[]; allowedActions: string[]; revision: string;
}

const rfqLabel: Record<string, string> = { draft: 'Черновик', sent: 'Отправлен', negotiating: 'Согласование', accepted: 'Принят', declined: 'Отклонён', cancelled: 'Отменён' };
const orderLabel: Record<string, string> = { 'awaiting-supplier': 'Ждёт поставщика', accepted: 'Принят', fulfilling: 'Исполняется', completed: 'Выполнен', cancelled: 'Отменён' };
const orderTone = (s: string) => s === 'completed' ? 'success' : s === 'cancelled' ? 'danger' : s === 'awaiting-supplier' ? 'warning' : 'info';

export function PurchasesPage() {
  const [tab, setTab] = useState<'orders' | 'rfqs'>('orders');
  return <Page title="Закупки и продажи опт" subtitle="Запросы котировок, заказы, отгрузки и приёмка">
    <Tabs value={tab} onChange={setTab} tabs={[['orders', 'Заказы'], ['rfqs', 'Запросы котировок (RFQ)']]} />
    {tab === 'orders' ? <OrdersPanel /> : <RFQPanel />}
  </Page>;
}

function RFQPanel() {
  const q = useData(['rfqs'], () => list<RFQ>('/commerce/rfqs'));
  const partners = usePartners();
  const [open, setOpen] = useState<string | null>(null);
  const suppliers = (partners.data ?? []).filter((p) => p.status === 'active').map((p): [string, string] => [p.counterparty.id, p.counterparty.name]);
  return <Panel title="RFQ" actions={<TermsButton label="Новый RFQ" withPrices={false} refresh={[['rfqs']]}
    extra={[{ name: 'supplier', label: 'Поставщик (активный партнёр)', required: true, options: suppliers }]}
    onSubmit={(terms, x) => post('/commerce/rfqs', { supplierCompanyId: x.supplier, lines: terms.lines.map((l) => ({ modelId: l.modelId, quantity: l.quantity })) })} />}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(r) => r.id} onRowClick={(r) => setOpen(r.id)} columns={[
      { title: 'Покупатель', render: (r) => r.buyer.name }, { title: 'Поставщик', render: (r) => r.supplier.name },
      { title: 'Позиций', render: (r) => r.lines.length }, { title: 'Котировок', render: (r) => r.quotations.length },
      { title: 'Статус', render: (r) => <Badge>{rfqLabel[r.status]}</Badge> },
    ]} />
    {open && <RFQDialog id={open} onClose={() => setOpen(null)} />}
  </Panel>;
}

function RFQDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['rfq', id], () => get<RFQ>(`/commerce/rfqs/${id}`));
  const name = useModelName();
  const r = q.data?.data;
  const refresh = [['rfq', id], ['rfqs'], ['orders']];
  const latest = r?.quotations[r.quotations.length - 1];
  const act = (a: string) => {
    switch (a) {
      case 'send': return <ActionButton key={a} label="Отправить" variant="primary" refresh={refresh} onSubmit={() => post(`/commerce/rfqs/${id}/send`, {}, { ifMatch: r!.revision })} />;
      case 'quote': return <TermsButton key={a} label="Отправить котировку" variant="primary" refresh={refresh} initial={latest?.terms ?? { lines: r!.lines.map((l) => ({ ...l, unitPrice: { amountMinor: '0', currency: 'USD' } })), route: 'local', deliveryTerms: '', paymentSchedule: [], warrantyTerms: '', serviceTerms: '' }}
        onSubmit={(terms) => post(`/commerce/rfqs/${id}/quotation-versions`, { terms }, { ifMatch: r!.revision })} />;
      case 'accept': return latest && <ActionButton key={a} label={`Принять котировку №${latest.number}`} variant="primary" refresh={refresh}
        intro={<p>Будет создан заказ на условиях котировки №{latest.number} на сумму {money(latest.total)}.</p>}
        onSubmit={() => post(`/commerce/rfqs/${id}/accept`, { quotationVersionId: latest.id, quotationDigest: latest.digest }, { ifMatch: r!.revision })} />;
      default: return <ActionButton key={a} label={a === 'cancel' ? 'Отменить' : 'Отклонить'} fields={reason} refresh={refresh}
        onSubmit={(v) => post(`/commerce/rfqs/${id}/${a}`, v, { ifMatch: r!.revision })} />;
    }
  };
  return <Modal title={r ? `RFQ ${r.buyer.name} → ${r.supplier.name}` : 'RFQ'} onClose={onClose} size="wide" footer={r && <>{r.allowedActions.map(act)}</>}>
    {r && <>
      <Details items={[['Статус', rfqLabel[r.status]], ['Позиции', r.lines.map((l) => `${name(l.modelId)} × ${l.quantity}`).join('; ')]]} />
      {r.quotations.map((qt) => <Panel key={qt.id} title={`Котировка №${qt.number} · ${money(qt.total)}`} padded><TermsView terms={qt.terms} modelNameOf={name} /></Panel>)}
    </>}
  </Modal>;
}

function OrdersPanel() {
  const q = useData(['orders'], () => list<Order>('/commerce/orders?limit=100'));
  const [open, setOpen] = useState<string | null>(null);
  return <Panel title="Заказы"><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(o) => o.id} onRowClick={(o) => setOpen(o.id)} columns={[
    { title: 'Роль', render: (o) => o.party === 'buyer' ? 'Покупка' : 'Продажа' },
    { title: 'Контрагент', render: (o) => o.party === 'buyer' ? o.supplier.name : o.buyer.name },
    { title: 'Сумма', render: (o) => money(o.total) },
    { title: 'Статус', render: (o) => <Badge tone={orderTone(o.status)}>{orderLabel[o.status]}</Badge> },
  ]} />
    {open && <OrderDialog id={open} onClose={() => setOpen(null)} />}
  </Panel>;
}

function OrderDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['order', id], () => get<Order>(`/commerce/orders/${id}`));
  const invoices = useData(['order-invoices', id], () => list<Invoice>(`/commerce/orders/${id}/invoices`));
  const vehicles = useVehicles('any');
  const name = useModelName();
  const models = useModels();
  const o = q.data?.data;
  const refresh = [['order', id], ['orders'], ['order-invoices', id], ['vehicles']];
  const [shipment, setShipment] = useState<string | null>(null);
  if (!o) return <Modal title="Заказ" onClose={onClose}><p>Загрузка…</p></Modal>;
  const openAddendum = o.addenda.find((a) => a.status === 'proposed');
  const allocatedFree = o.allocations.filter((a) => a.status === 'allocated');
  const freeVehicles = (vehicles.data ?? []).filter((v) => o.terms.lines.some((l) => l.modelId === v.modelId) && !o.allocations.some((a) => a.vehicleId === v.id && a.status !== 'rejected' && a.status !== 'released'));
  const actions = o.allowedActions;
  return <Modal title={`${o.party === 'buyer' ? 'Покупка у' : 'Продажа'} ${o.party === 'buyer' ? o.supplier.name : o.buyer.name}`} onClose={onClose} size="wide" footer={<>
    {actions.includes('confirm') && <ActionButton label="Подтвердить" variant="primary" refresh={refresh} onSubmit={() => post(`/commerce/orders/${id}/supplier-confirmations`, {}, { ifMatch: o.revision })} />}
    {actions.includes('reject') && <ActionButton label="Отклонить" fields={reason} refresh={refresh} onSubmit={(v) => post(`/commerce/orders/${id}/supplier-rejections`, v, { ifMatch: o.revision })} />}
    {actions.includes('allocate') && <ActionButton label="Назначить VIN" refresh={refresh} fields={[
      { name: 'orderLineId', label: 'Строка заказа', type: 'select', required: true, options: o.terms.lines.map((l) => [l.lineId!, `${name(l.modelId)} × ${l.quantity}`]) },
      { name: 'vehicleIds', label: 'Автомобили', type: 'multiselect', options: freeVehicles.map((v) => [v.id, `${v.vin} — ${modelName(models.data?.find((m) => m.id === v.modelId))}`]) },
    ]} onSubmit={(v) => post(`/commerce/orders/${id}/allocations`, { items: (v.vehicleIds as string[]).map((vehicleId) => ({ orderLineId: v.orderLineId, vehicleId })) }, { ifMatch: o.revision })} />}
    {actions.includes('ship') && <ActionButton label="Отгрузить" refresh={refresh} fields={[
      { name: 'vehicleIds', label: 'Автомобили', type: 'multiselect', options: allocatedFree.map((a) => [a.vehicleId, a.vin]) },
      { name: 'route', label: 'Маршрут', type: 'select', required: true, options: Object.entries(routeLabel), initial: o.terms.route },
    ]} onSubmit={(v) => post(`/commerce/orders/${id}/shipments`, v, { ifMatch: o.revision })} />}
    {actions.includes('propose-addendum') && <TermsButton label="Предложить изменение" initial={o.terms} refresh={refresh} extra={[{ name: 'reason', label: 'Причина изменения', required: true }]}
      onSubmit={(terms, x) => post(`/commerce/orders/${id}/addenda`, { terms, reason: x.reason }, { ifMatch: o.revision })} />}
    {actions.includes('accept-addendum') && openAddendum && <ActionButton label={`Принять изменение №${openAddendum.number}`} variant="primary" refresh={refresh}
      onSubmit={() => post(`/commerce/orders/${id}/addenda/${openAddendum.id}/accept`, {}, { ifMatch: o.revision })} />}
    {actions.includes('reject-addendum') && openAddendum && <ActionButton label="Отклонить изменение" fields={reason} refresh={refresh}
      onSubmit={(v) => post(`/commerce/orders/${id}/addenda/${openAddendum.id}/reject`, v, { ifMatch: o.revision })} />}
    {o.party === 'supplier' && ['accepted', 'fulfilling', 'completed'].includes(o.status) && !(invoices.data ?? []).some((i) => i.status === 'issued') &&
      <ActionButton label="Выставить счёт" refresh={refresh} fields={o.terms.paymentSchedule.length ? [] : [{ name: 'dueDate', label: 'Срок оплаты', type: 'date', required: true }]}
        onSubmit={(v) => post(`/commerce/orders/${id}/invoices`, v, { ifMatch: o.revision })} />}
    {actions.includes('cancel') && <ActionButton label="Отменить заказ" variant="danger" fields={reason} refresh={refresh} onSubmit={(v) => post(`/commerce/orders/${id}/cancellations`, v, { ifMatch: o.revision })} />}
  </>}>
    <Details items={[['Статус', <Badge tone={orderTone(o.status)}>{orderLabel[o.status]}</Badge>], ['Сумма', money(o.total)],
      ['Источник', o.source === 'rfq' ? 'Котировка' : 'Прямой заказ'], ['Причина', o.statusReason || '—']]} />
    <Panel title="Условия" padded><TermsView terms={o.terms} modelNameOf={name} /></Panel>
    {o.allocations.length > 0 && <Panel title="Назначенные VIN"><Table rows={o.allocations} rowKey={(a) => a.vehicleId + a.status} columns={[
      { title: 'VIN', render: (a) => <code>{a.vin}</code> }, { title: 'Статус', render: (a) => ({ allocated: 'Назначен', shipped: 'Отгружен', delivered: 'Принят', rejected: 'Отклонён при приёмке', released: 'Освобождён' }[a.status] ?? a.status) },
    ]} /></Panel>}
    {o.shipments.length > 0 && <Panel title="Отгрузки"><Table rows={o.shipments} rowKey={(s) => s.id} onRowClick={(s) => setShipment(s.id)} columns={[
      { title: 'Маршрут', render: (s) => routeLabel[s.route] }, { title: 'Статус', render: (s) => s.status === 'received' ? 'Принята' : 'В пути' },
    ]} /></Panel>}
    {o.addenda.length > 0 && <Panel title="Изменения условий"><Table rows={o.addenda} rowKey={(a) => a.id} columns={[
      { title: '№', render: (a) => a.number }, { title: 'Сумма', render: (a) => money(a.total) }, { title: 'Причина', render: (a) => a.reason },
      { title: 'Статус', render: (a) => ({ proposed: 'Предложено', accepted: 'Принято', rejected: 'Отклонено' }[a.status]) },
    ]} /></Panel>}
    {(invoices.data ?? []).map((i) => <InvoicePanel key={i.id} invoice={i} party={o.party} refresh={refresh} />)}
    {o.history && <Panel title="История" padded><ul className="kit-timeline">{o.history.map((h, i) => <li key={i}>{dateTime(h.occurredAt)} — {h.type}{h.reason ? ` · ${h.reason}` : ''}</li>)}</ul></Panel>}
    {shipment && <ShipmentDialog id={shipment} party={o.party} onClose={() => setShipment(null)} refreshOrder={refresh} />}
  </Modal>;
}

interface Shipment { id: string; route: string; status: string; vehicles: Allocation[]; milestones: { milestoneType: string; occurredAt: string; location: string; note: string }[]; revision: string }
const milestoneLabel: Record<string, string> = { departed: 'Отправлено', 'border-crossed': 'Граница пройдена', 'customs-cleared': 'Таможня пройдена', arrived: 'Прибыло', 'damage-reported': 'Повреждение' };

function ShipmentDialog({ id, party, onClose, refreshOrder }: { id: string; party: string; onClose: () => void; refreshOrder: unknown[][] }) {
  const q = useData(['shipment', id], () => get<Shipment>(`/commerce/shipments/${id}`));
  const warehouses = useWarehouses();
  const s = q.data?.data;
  const refresh = [['shipment', id], ...refreshOrder, ['warehouses']];
  const waiting = s?.vehicles.filter((v) => v.status === 'shipped') ?? [];
  return <Modal title="Отгрузка" onClose={onClose} size="wide" footer={s && <>
    <ActionButton label="Отметить этап" refresh={refresh} fields={[
      { name: 'milestoneType', label: 'Этап', type: 'select', required: true, options: Object.entries(milestoneLabel) },
      { name: 'occurredAt', label: 'Когда', type: 'datetime', required: true },
      { name: 'location', label: 'Где', type: 'text', required: true }, { name: 'note', label: 'Примечание', type: 'textarea' },
    ]} onSubmit={(v) => post(`/commerce/shipments/${id}/milestones`, v)} />
    {party === 'buyer' && waiting.length > 0 && <>
      <ActionButton label="Принять на склад" variant="primary" refresh={refresh} fields={[
        { name: 'vehicleIds', label: 'Автомобили', type: 'multiselect', options: waiting.map((v) => [v.vehicleId, v.vin]) },
        { name: 'warehouseId', label: 'Склад', type: 'select', required: true, options: (warehouses.data ?? []).map((w) => [w.id, `${w.name} (свободно ${w.free})`]) },
      ]} onSubmit={(v) => post(`/commerce/shipments/${id}/receipt-decisions`, { decision: 'accept', ...v }, { ifMatch: s.revision })} />
      <ActionButton label="Отказать в приёмке" variant="danger" refresh={refresh} fields={[
        { name: 'vehicleIds', label: 'Автомобили', type: 'multiselect', options: waiting.map((v) => [v.vehicleId, v.vin]) }, ...reason,
      ]} onSubmit={(v) => post(`/commerce/shipments/${id}/receipt-decisions`, { decision: 'reject', ...v }, { ifMatch: s.revision })} />
    </>}
  </>}>
    {s && <>
      <Details items={[['Маршрут', routeLabel[s.route]], ['Статус', s.status === 'received' ? 'Принята' : 'В пути'], ['VIN', s.vehicles.map((v) => `${v.vin} (${v.status})`).join(', ')]]} />
      <Panel title="Этапы" padded><ul className="kit-timeline">{s.milestones.map((m, i) =>
        <li key={i}>{dateTime(m.occurredAt)} — {milestoneLabel[m.milestoneType]} · {m.location}{m.note ? ` · ${m.note}` : ''}</li>)}</ul></Panel>
    </>}
  </Modal>;
}

// ---------------- invoices ----------------

interface Evidence { id: string; amount: Money; paidOn: string; externalReference: string; attachmentIds: string[]; status: string; decisionReason: string; allowedActions: string[]; revision: string }
export interface Invoice { id: string; orderId: string; total: Money; status: string; paid: Money; pending: Money; outstanding: Money; paymentEvidence: Evidence[]; allowedActions: string[]; revision: string; schedule: { amount: Money; dueDate: string }[] }

const evidenceLabel: Record<string, string> = { submitted: 'На проверке', accepted: 'Принято', rejected: 'Отклонено' };

export function InvoicePanel({ invoice: i, party, refresh }: { invoice: Invoice; party: string; refresh: unknown[][] }) {
  const reload = useRefresh();
  return <Panel title={`Счёт ${money(i.total)} · оплачено ${money(i.paid)} · остаток ${money(i.outstanding)}${i.status === 'void' ? ' · аннулирован' : ''}`} actions={<>
    {i.allowedActions.includes('submit-payment') && <ActionButton label="Сообщить об оплате" refresh={refresh} fields={[
      { name: 'claimedAmount', label: 'Сумма', type: 'money', required: true, currency: i.total.currency },
      { name: 'paidOn', label: 'Дата оплаты', type: 'date', required: true },
      { name: 'externalReference', label: 'Номер платёжки', type: 'text', required: true },
      { name: 'file', label: 'Подтверждение (файл)', type: 'file', purpose: 'payment-evidence' },
    ]} onSubmit={(v) => post(`/commerce/invoices/${i.id}/payment-evidence`, { claimedAmount: v.claimedAmount, paidOn: v.paidOn,
      externalReference: v.externalReference, attachmentBindingIds: v.file ? [v.file] : [] })} />}
    {i.allowedActions.includes('void') && <ActionButton label="Аннулировать" fields={reason} refresh={refresh} onSubmit={(v) => post(`/commerce/invoices/${i.id}/void`, v, { ifMatch: i.revision })} />}
  </>}>
    <Table rows={i.paymentEvidence} rowKey={(e) => e.id} empty="Оплат пока нет" columns={[
      { title: 'Сумма', render: (e) => money(e.amount) }, { title: 'Дата', render: (e) => e.paidOn },
      { title: 'Документ', render: (e) => <>{e.externalReference} {e.attachmentIds.map((f) => <a key={f} href={fileUrl(f)} target="_blank" rel="noreferrer">файл</a>)}</> },
      { title: 'Статус', render: (e) => <Badge tone={e.status === 'accepted' ? 'success' : e.status === 'rejected' ? 'danger' : 'warning'}>{evidenceLabel[e.status]}</Badge> },
      { title: '', render: (e) => party === 'supplier' && e.allowedActions.length > 0 && <div className="kit-row">
        <ActionButton label="Принять" variant="primary" fields={[{ name: 'confirmation', label: 'Деньги поступили на наш счёт', type: 'checkbox' }]}
          onSubmit={async (v) => { await post(`/commerce/payment-evidence/${e.id}/accept`, v, { ifMatch: e.revision }); await reload(...refresh); }} />
        <ActionButton label="Отклонить" fields={reason} onSubmit={async (v) => { await post(`/commerce/payment-evidence/${e.id}/reject`, v, { ifMatch: e.revision }); await reload(...refresh); }} />
      </div> },
    ]} />
  </Panel>;
}

export function BillingPage() {
  const orders = useData(['orders'], () => list<Order>('/commerce/orders?limit=100'));
  const [tab, setTab] = useState<'buyer' | 'supplier'>('buyer');
  const rows = (orders.data ?? []).filter((o) => o.party === tab);
  return <Page title="Счета и оплаты" subtitle="Оплаты фиксируются как факты; деньги через платформу не проходят">
    <Tabs value={tab} onChange={setTab} tabs={[['buyer', 'Поставщикам'], ['supplier', 'От покупателей']]} />
    {rows.map((o) => <OrderInvoices key={o.id} order={o} />)}
    {rows.length === 0 && <p className="kit-muted">Нет заказов.</p>}
  </Page>;
}

function OrderInvoices({ order }: { order: Order }) {
  const q = useData(['order-invoices', order.id], () => list<Invoice>(`/commerce/orders/${order.id}/invoices`));
  if (!q.data?.length) return null;
  return <div className="kit-stack"><b>{order.party === 'buyer' ? order.supplier.name : order.buyer.name} · заказ {money(order.total)}</b>
    {q.data.map((i) => <InvoicePanel key={i.id} invoice={i} party={order.party} refresh={[['order-invoices', order.id], ['orders']]} />)}</div>;
}
