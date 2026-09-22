import { useState } from 'react';
import {
  ActionButton, Badge, Details, Modal, Page, Panel, Table, Tabs, date, dateTime, fileUrl, get, list, money, patch, post, useData, useSession,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { minorToMajor } from '@justixauto/kit';
import { modelName, useBranches, useCustomers, useDeals, useModels, useVehicles } from '../data';
import type { Customer, Deal, Money, RetailInvoice } from '../data';

const reason: FieldSpec[] = [{ name: 'reason', label: 'Причина', type: 'textarea', required: true }];


/** VIN plus model name for the seller's own vehicles. */
function useVehicleLabel() {
  const vehicles = useVehicles();
  const models = useModels();
  return (id: string) => {
    const v = vehicles.data?.find((x) => x.id === id);
    return v ? `${v.vin} · ${modelName(models.data?.find((m) => m.id === v.modelId))}` : id.slice(0, 8);
  };
}

// ---------------- listings ----------------

interface Listing { id: string; vehicleId: string; text: string; askingPrice: Money; status: string; revision: string; updatedAt: string }
const listingLabel: Record<string, string> = { draft: 'Черновик', published: 'Опубликовано', withdrawn: 'Снято' };

export function ListingsPage() {
  const q = useData(['listings'], () => list<Listing>('/retail/listings?limit=100'));
  const vehicles = useVehicles('warehouse');
  const label = useVehicleLabel();
  const refresh = [['listings']];
  const fields = (l?: Listing): FieldSpec[] => [
    ...(l ? [] : [{ name: 'vehicleId', label: 'Автомобиль', type: 'select', required: true,
      options: (vehicles.data ?? []).map((v) => [v.id, label(v.id)]) } as FieldSpec]),
    { name: 'askingPrice', label: 'Цена', type: 'money', required: true, ...(l ? { initial: minorToMajor(l.askingPrice.amountMinor), currency: l.askingPrice.currency } : {}) },
    { name: 'text', label: 'Описание', type: 'textarea', required: true, ...(l ? { initial: l.text } : {}) },
  ];
  return <Page title="Витрина" subtitle="Розничные объявления о продаже автомобилей со склада"
    actions={<ActionButton label="Новое объявление" variant="primary" fields={fields()} refresh={refresh} onSubmit={(v) => post('/retail/listings', v)} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(l) => l.id} columns={[
      { title: 'Автомобиль', render: (l) => label(l.vehicleId) },
      { title: 'Цена', render: (l) => money(l.askingPrice) },
      { title: 'Описание', render: (l) => l.text },
      { title: 'Статус', render: (l) => <Badge tone={l.status === 'published' ? 'success' : undefined}>{listingLabel[l.status]}</Badge> },
      { title: '', render: (l) => l.status !== 'withdrawn' && <div className="kit-row">
        <ActionButton label="Изменить" fields={fields(l)} refresh={refresh} onSubmit={(v) => patch(`/retail/listings/${l.id}`, v, { ifMatch: l.revision })} />
        {l.status === 'draft'
          ? <ActionButton label="Опубликовать" variant="primary" refresh={refresh} onSubmit={() => post(`/retail/listings/${l.id}/publish`, {}, { ifMatch: l.revision })} />
          : <ActionButton label="Снять" refresh={refresh} onSubmit={() => post(`/retail/listings/${l.id}/withdraw`, {}, { ifMatch: l.revision })} />}
      </div> },
    ]} /></Panel>
  </Page>;
}

// ---------------- CRM ----------------

interface Lead {
  id: string; customerId: string; customer?: Customer; branchId: string; source: string; stage: string; assignedUserId: string | null;
  lostReason: string; dealId: string | null; contacts?: { channel: string; note: string; occurredAt: string }[];
  history?: { type: string; occurredAt: string; reason: string }[]; revision: string; updatedAt: string;
}
interface Task { id: string; customerId: string; leadId: string | null; dealId: string | null; ownerUserId: string; dueAt: string; title: string; status: string; revision: string }

const stageLabel: Record<string, string> = { new: 'Новый', contacted: 'Контакт', qualified: 'Квалифицирован', 'test-drive': 'Тест-драйв', negotiation: 'Переговоры', won: 'Продажа', lost: 'Потерян' };
const stageOrder = ['new', 'contacted', 'qualified', 'test-drive', 'negotiation'];
const eventLabel: Record<string, string> = {
  'lead.created': 'Лид создан', 'lead.assigned': 'Назначен ответственный', 'lead.stage_changed': 'Смена этапа', 'lead.won': 'Продажа',
  'deal.reserved': 'Сделка создана, автомобиль зарезервирован', 'deal.contract_recorded': 'Договор подписан', 'deal.invoice_issued': 'Выставлен счёт',
  'deal.payment_submitted': 'Внесена оплата', 'deal.payment_accepted': 'Оплата принята', 'deal.payment_rejected': 'Оплата отклонена',
  'deal.registered': 'Регистрация', 'deal.delivered': 'Автомобиль выдан', 'deal.cancelled': 'Сделка отменена',
};
const channelLabel: Record<string, string> = { phone: 'Звонок', telegram: 'Telegram', visit: 'Визит', email: 'Письмо', other: 'Другое' };
const sourceLabel: Record<string, string> = { website: 'Сайт', telegram: 'Telegram', phone: 'Звонок', manual: 'Вручную' };

export function CRMPage() {
  const [tab, setTab] = useState<'leads' | 'customers' | 'tasks'>('leads');
  return <Page title="Клиенты и лиды" subtitle="Воронка розничных продаж">
    <Tabs value={tab} onChange={setTab} tabs={[['leads', 'Лиды'], ['customers', 'Клиенты'], ['tasks', 'Задачи']]} />
    {tab === 'leads' && <LeadsPanel />}
    {tab === 'customers' && <CustomersPanel />}
    {tab === 'tasks' && <TasksPanel />}
  </Page>;
}

const customerFields = (c?: Customer): FieldSpec[] => [
  { name: 'displayName', label: 'Имя', type: 'text', required: true, ...(c ? { initial: c.displayName } : {}) },
  { name: 'phone', label: 'Телефон', type: 'text', ...(c ? { initial: c.phone } : {}) },
];

function CustomersPanel() {
  const q = useCustomers();
  const refresh = [['customers']];
  return <Panel title="Клиенты" actions={<ActionButton label="Новый клиент" variant="primary" fields={customerFields()} refresh={refresh}
    onSubmit={(v) => post('/retail/customers', { profile: v })} />}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(c) => c.id} columns={[
      { title: 'Имя', render: (c) => c.displayName }, { title: 'Телефон', render: (c) => c.phone || '—' },
      { title: '', render: (c) => <ActionButton label="Изменить" fields={customerFields(c)} refresh={refresh}
        onSubmit={(v) => patch(`/retail/customers/${c.id}`, { profile: v }, { ifMatch: c.revision })} /> },
    ]} />
  </Panel>;
}

function LeadsPanel() {
  const [stage, setStage] = useState('');
  const q = useData(['leads', stage], () => list<Lead>(`/retail/leads?limit=100${stage ? `&stage=${stage}` : ''}`));
  const customers = useCustomers();
  const branches = useBranches();
  const s = useSession();
  const [open, setOpen] = useState<string | null>(null);
  return <Panel title="Лиды" actions={<>
    <select value={stage} onChange={(e) => setStage(e.target.value)} aria-label="Этап">
      <option value="">Все этапы</option>{Object.entries(stageLabel).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
    </select>
    <ActionButton label="Новый лид" variant="primary" refresh={[['leads']]} fields={[
      { name: 'customerId', label: 'Клиент', type: 'select', required: true, options: (customers.data ?? []).map((c) => [c.id, c.displayName]) },
      { name: 'branchId', label: 'Филиал', type: 'select', required: true, options: (branches.data ?? []).map((b) => [b.id, b.name]) },
      { name: 'source', label: 'Источник', type: 'select', required: true, options: Object.entries(sourceLabel) },
      { name: 'mine', label: 'Назначить на меня', type: 'checkbox' },
    ]} onSubmit={(v) => post('/retail/leads', { customerId: v.customerId, branchId: v.branchId, source: v.source, assignedUserId: v.mine ? s.view.user.id : null })} />
  </>}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(l) => l.id} onRowClick={(l) => setOpen(l.id)} columns={[
      { title: 'Клиент', render: (l) => l.customer?.displayName ?? customers.data?.find((c) => c.id === l.customerId)?.displayName ?? '—' },
      { title: 'Источник', render: (l) => sourceLabel[l.source] ?? l.source },
      { title: 'Этап', render: (l) => <Badge tone={l.stage === 'won' ? 'success' : l.stage === 'lost' ? 'danger' : 'info'}>{stageLabel[l.stage]}</Badge> },
      { title: 'Филиал', render: (l) => branches.data?.find((b) => b.id === l.branchId)?.name ?? '—' },
      { title: 'Обновлён', render: (l) => dateTime(l.updatedAt) },
    ]} />
    {open && <LeadDialog id={open} onClose={() => setOpen(null)} />}
  </Panel>;
}

function LeadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['lead', id], () => get<Lead>(`/retail/leads/${id}`));
  const s = useSession();
  const l = q.data?.data;
  const refresh = [['lead', id], ['leads']];
  const open = l && l.stage !== 'won' && l.stage !== 'lost';
  const next = l ? stageOrder[stageOrder.indexOf(l.stage) + 1] : undefined;
  return <Modal title={l ? `Лид: ${l.customer?.displayName ?? ''}` : 'Лид'} onClose={onClose} size="wide" footer={l && open && <>
    <ActionButton label="Контакт" refresh={refresh} fields={[
      { name: 'channel', label: 'Канал', type: 'select', required: true, options: Object.entries(channelLabel) },
      { name: 'note', label: 'Итог', type: 'textarea', required: true },
    ]} onSubmit={(v) => post(`/retail/leads/${id}/contacts`, v)} />
    {l.assignedUserId !== s.view.user.id && <ActionButton label="Взять себе" refresh={refresh}
      onSubmit={() => post(`/retail/leads/${id}/assign`, { assignedUserId: s.view.user.id }, { ifMatch: l.revision })} />}
    {next && <ActionButton label={`→ ${stageLabel[next]}`} variant="primary" refresh={refresh}
      onSubmit={() => post(`/retail/leads/${id}/stage`, { stage: next }, { ifMatch: l.revision })} />}
    <ActionButton label="Потерян" variant="danger" fields={reason} refresh={refresh}
      onSubmit={(v) => post(`/retail/leads/${id}/stage`, { stage: 'lost', reason: v.reason }, { ifMatch: l.revision })} />
  </>}>
    {l && <>
      <Details items={[['Этап', stageLabel[l.stage]], ['Источник', sourceLabel[l.source] ?? l.source], ['Телефон', l.customer?.phone || '—'],
        ['Ответственный', l.assignedUserId === s.view.user.id ? 'Я' : l.assignedUserId ? 'Назначен' : 'Не назначен'],
        ['Причина потери', l.lostReason || '—'], ['Сделка', l.dealId ? 'Создана' : '—']]} />
      <Panel title="Контакты" padded><ul className="kit-timeline">{(l.contacts ?? []).map((c, i) =>
        <li key={i}>{dateTime(c.occurredAt)} — {channelLabel[c.channel] ?? c.channel}: {c.note}</li>)}</ul></Panel>
      <Panel title="История" padded><ul className="kit-timeline">{(l.history ?? []).map((h, i) =>
        <li key={i}>{dateTime(h.occurredAt)} — {eventLabel[h.type] ?? h.type}{h.reason ? ` · ${h.reason}` : ''}</li>)}</ul></Panel>
    </>}
  </Modal>;
}

function TasksPanel() {
  const q = useData(['tasks'], () => list<Task>('/retail/tasks?limit=100'));
  const customers = useCustomers();
  const s = useSession();
  const refresh = [['tasks']];
  return <Panel title="Задачи" actions={<ActionButton label="Новая задача" variant="primary" refresh={refresh} fields={[
    { name: 'title', label: 'Что сделать', type: 'text', required: true },
    { name: 'customerId', label: 'Клиент', type: 'select', required: true, options: (customers.data ?? []).map((c) => [c.id, c.displayName]) },
    { name: 'dueAt', label: 'Срок', type: 'datetime', required: true },
  ]} onSubmit={(v) => post('/retail/tasks', { ...v, ownerUserId: s.view.user.id, leadId: null, dealId: null })} />}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(t) => t.id} columns={[
      { title: 'Задача', render: (t) => t.title },
      { title: 'Клиент', render: (t) => customers.data?.find((c) => c.id === t.customerId)?.displayName ?? '—' },
      { title: 'Срок', render: (t) => <span style={t.status === 'open' && new Date(t.dueAt) < new Date() ? { color: 'var(--danger)' } : undefined}>{dateTime(t.dueAt)}</span> },
      { title: 'Статус', render: (t) => t.status === 'open' ? 'Открыта' : 'Выполнена' },
      { title: '', render: (t) => t.status === 'open' && <ActionButton label="Выполнено" refresh={refresh}
        onSubmit={() => post(`/retail/tasks/${t.id}/complete`, {}, { ifMatch: t.revision })} /> },
    ]} />
  </Panel>;
}

// ---------------- deals ----------------


const schemeLabel: Record<string, string> = { cash: 'Наличные / перевод', 'own-installment': 'Собственная рассрочка', 'partner-finance': 'Банк / МФО' };
const dealLabel: Record<string, string> = { reserved: 'В работе', delivered: 'Выдан', cancelled: 'Отменена' };
const purposeLabel: Record<string, string> = { 'vehicle-payment': 'Оплата автомобиля', 'first-installment': 'Первый взнос', registration: 'Регистрация' };
const purposes: Record<string, string[]> = { cash: ['vehicle-payment', 'registration'], 'own-installment': ['first-installment', 'registration'], 'partner-finance': ['registration'] };


export function SalesPage() {
  const q = useDeals();
  const customers = useCustomers();
  const vehicles = useVehicles('warehouse');
  const branches = useBranches();
  const leads = useData(['leads', ''], () => list<Lead>('/retail/leads?limit=100'));
  const label = useVehicleLabel();
  const [open, setOpen] = useState<string | null>(null);
  const busy = new Set((q.data ?? []).filter((d) => d.status === 'reserved').map((d) => d.vehicleId));
  return <Page title="Продажи" subtitle="Розничные сделки: договор, оплаты, регистрация и выдача"
    actions={<ActionButton label="Новая сделка" variant="primary" refresh={[['deals'], ['leads']]} fields={[
      { name: 'customerId', label: 'Клиент', type: 'select', required: true, options: (customers.data ?? []).map((c) => [c.id, c.displayName]) },
      { name: 'leadId', label: 'Лид (необязательно)', type: 'select', options: [['', '—'], ...(leads.data ?? []).filter((l) => !l.dealId && ['qualified', 'test-drive', 'negotiation'].includes(l.stage)).map((l) => [l.id, `${l.customer?.displayName ?? ''} · ${stageLabel[l.stage]}`] as [string, string])] },
      { name: 'vehicleId', label: 'Автомобиль', type: 'select', required: true, options: (vehicles.data ?? []).filter((v) => !busy.has(v.id)).map((v) => [v.id, label(v.id)]) },
      { name: 'branchId', label: 'Филиал', type: 'select', required: true, options: (branches.data ?? []).map((b) => [b.id, b.name]) },
      { name: 'paymentScheme', label: 'Схема оплаты', type: 'select', required: true, options: Object.entries(schemeLabel) },
      { name: 'price', label: 'Цена', type: 'money', required: true },
    ]} onSubmit={(v) => post('/retail/deals', { ...v, leadId: v.leadId || null })} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(d) => d.id} onRowClick={(d) => setOpen(d.id)} columns={[
      { title: 'Клиент', render: (d) => d.customer.displayName },
      { title: 'Автомобиль', render: (d) => label(d.vehicleId) },
      { title: 'Схема', render: (d) => schemeLabel[d.paymentScheme] },
      { title: 'Цена', render: (d) => money(d.price) },
      { title: 'Статус', render: (d) => <Badge tone={d.status === 'delivered' ? 'success' : d.status === 'cancelled' ? 'danger' : 'info'}>{dealLabel[d.status]}</Badge> },
    ]} /></Panel>
    {open && <DealDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

const check = (ok: boolean | undefined) => ok === undefined ? null : ok ? '✓' : '✗';

function DealDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['deal', id], () => get<Deal>(`/retail/deals/${id}`));
  const label = useVehicleLabel();
  const d = q.data?.data;
  const refresh = [['deal', id], ['deals'], ['vehicles'], ['leads']];
  const can = (a: string) => d?.allowedActions.includes(a) ?? false;
  const c = d?.checklist;
  return <Modal title={d ? `Сделка: ${d.customer.displayName}` : 'Сделка'} onClose={onClose} size="wide" footer={d && <>
    {can('record-contract') && <ActionButton label="Договор подписан" refresh={refresh} fields={[
      { name: 'signedOn', label: 'Дата подписания', type: 'date', required: true },
      { name: 'reference', label: 'Номер договора', type: 'text', required: true },
      { name: 'file', label: 'Скан договора', type: 'file', purpose: 'deal-document' },
    ]} onSubmit={(v) => post(`/retail/deals/${id}/contract-records`, { signedOn: v.signedOn, reference: v.reference, bindingIds: v.file ? [v.file] : [] }, { ifMatch: d.revision })} />}
    {can('issue-invoice') && <ActionButton label="Выставить счёт" refresh={refresh} fields={[
      { name: 'purpose', label: 'Назначение', type: 'select', required: true, options: (purposes[d.paymentScheme] ?? []).map((p) => [p, purposeLabel[p] ?? p]) },
      { name: 'amount', label: 'Сумма', type: 'money', required: true, currency: d.price.currency },
      { name: 'recipientSnapshot', label: 'Плательщик (как в счёте)', type: 'text', required: true, initial: d.customer.displayName },
      { name: 'dueDate', label: 'Оплатить до', type: 'date', required: true },
    ]} onSubmit={(v) => post(`/retail/deals/${id}/invoices`, v, { ifMatch: d.revision })} />}
    {can('record-registration') && <ActionButton label="Регистрация" refresh={refresh} fields={[
      { name: 'registeredOn', label: 'Дата регистрации', type: 'date', required: true },
      { name: 'plateNumber', label: 'Госномер', type: 'text', required: true },
      { name: 'reference', label: 'Номер свидетельства', type: 'text', required: true },
    ]} onSubmit={(v) => post(`/retail/deals/${id}/registration`, v, { ifMatch: d.revision })} />}
    {can('deliver') && <ActionButton label="Выдать автомобиль" variant="primary" refresh={refresh}
      fields={[{ name: 'occurredAt', label: 'Когда выдан', type: 'datetime', required: true }]}
      intro={<p>Автомобиль будет списан со склада. Действие необратимо.</p>}
      onSubmit={(v) => post(`/retail/deals/${id}/deliveries`, v, { ifMatch: d.revision })} />}
    {can('cancel') && <ActionButton label="Отменить сделку" variant="danger" fields={reason} refresh={refresh}
      onSubmit={(v) => post(`/retail/deals/${id}/cancel`, v, { ifMatch: d.revision })} />}
  </>}>
    {d && <>
      <Details items={[['Автомобиль', label(d.vehicleId)], ['Схема', schemeLabel[d.paymentScheme]], ['Цена', money(d.price)],
        ['Статус', dealLabel[d.status]], ['Договор', d.contractSignedOn ? <>{d.contractReference} от {date(d.contractSignedOn)}{d.contractFileIds.map((f, i) =>
          <span key={f}> · <a href={fileUrl(f)} target="_blank" rel="noreferrer">скан {i + 1}</a></span>)}</> : '—'],
        ['Регистрация', d.registeredOn ? `${d.plateNumber} (${d.registrationReference}) от ${date(d.registeredOn)}` : '—'],
        ['Выдан', dateTime(d.deliveredAt)], ['Причина отмены', d.statusReason || '—']]} />
      {c && d.status === 'reserved' && <Panel title="Готовность к выдаче" padded><ul className="kit-timeline">
        <li>{check(c.contract)} Договор</li>
        {c.vehiclePayment !== undefined && <li>{check(c.vehiclePayment)} Оплата автомобиля</li>}
        {c.firstInstallment !== undefined && <li>{check(c.firstInstallment)} Первый взнос</li>}
        {c.insuranceApproved !== undefined && <li>{check(c.insuranceApproved)} Одобрение страховой</li>}
        <li>{check(c.registrationPaid)} Оплата регистрации</li>
        <li>{check(c.registered)} Регистрация</li>
        {!c.policyResolved && <li>✗ Правила выдачи при финансировании банком/МФО ещё не утверждены (OD-01)</li>}
      </ul></Panel>}
      {(d.invoices ?? []).map((i) => <RetailInvoicePanel key={i.id} invoice={i} refresh={refresh} />)}
      <Panel title="История" padded><ul className="kit-timeline">{(d.history ?? []).map((h, i) =>
        <li key={i}>{dateTime(h.occurredAt)} — {eventLabel[h.type] ?? h.type}{h.reason ? ` · ${h.reason}` : ''}</li>)}</ul></Panel>
    </>}
  </Modal>;
}

function RetailInvoicePanel({ invoice: i, refresh }: { invoice: RetailInvoice; refresh: unknown[][] }) {
  return <Panel title={`${purposeLabel[i.purpose] ?? i.purpose}: ${money(i.amount)} · оплачено ${money(i.paid)} · остаток ${money(i.outstanding)}`}
    actions={i.status === 'issued' && i.outstanding.amountMinor !== '0' && <ActionButton label="Внести оплату" refresh={refresh} fields={[
      { name: 'claimedAmount', label: 'Сумма', type: 'money', required: true, currency: i.amount.currency },
      { name: 'paidOn', label: 'Дата оплаты', type: 'date', required: true },
      { name: 'externalReference', label: 'Номер платёжки / чека', type: 'text', required: true },
      { name: 'file', label: 'Подтверждение (файл)', type: 'file', purpose: 'payment-evidence' },
    ]} onSubmit={(v) => post(`/retail/invoices/${i.id}/evidence`, { claimedAmount: v.claimedAmount, paidOn: v.paidOn,
      externalReference: v.externalReference, attachmentBindingIds: v.file ? [v.file] : [] })} />}>
    <Table rows={i.paymentEvidence} rowKey={(e) => e.id} empty="Оплат пока нет" columns={[
      { title: 'Сумма', render: (e) => money(e.amount) }, { title: 'Дата', render: (e) => e.paidOn },
      { title: 'Документ', render: (e) => <>{e.externalReference} {e.attachmentIds.map((f) => <a key={f} href={fileUrl(f)} target="_blank" rel="noreferrer">файл</a>)}</> },
      { title: 'Статус', render: (e) => <Badge tone={e.status === 'accepted' ? 'success' : e.status === 'rejected' ? 'danger' : 'warning'}>
        {({ submitted: 'На проверке', accepted: 'Принято', rejected: `Отклонено: ${e.decisionReason}` } as Record<string, string>)[e.status]}</Badge> },
      { title: '', render: (e) => e.status === 'submitted' && <div className="kit-row">
        <ActionButton label="Принять" variant="primary" refresh={refresh} fields={[{ name: 'confirmation', label: 'Деньги поступили', type: 'checkbox' }]}
          onSubmit={(v) => post(`/retail/evidence/${e.id}/accept`, v, { ifMatch: e.revision })} />
        <ActionButton label="Отклонить" fields={reason} refresh={refresh} onSubmit={(v) => post(`/retail/evidence/${e.id}/reject`, v, { ifMatch: e.revision })} />
      </div> },
    ]} />
  </Panel>;
}
