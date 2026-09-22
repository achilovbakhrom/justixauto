import { useState } from 'react';
import {
  Badge, Button, Cell, FilterSelect, Modal, Page, Panel, ResultMeta, Stat, Stats, Table, Tabs, Toolbar, date, get, list, matches, money,
  plural, useData, useSearchQuery,
} from '@justixauto/kit';
import { purposeLabel, sumMoney, useLinesLabel } from '../data';
import type { Deal, Invoice, Money, Order, RetailInvoice } from '../data';
import { RetailInvoicePanel } from './retail';
import { InvoicePanel } from './trade';

type State = 'due' | 'review' | 'paid' | 'void';
const stateLabel: Record<State, [string, 'warning' | 'info' | 'success' | undefined]> = {
  due: ['К оплате', 'warning'], review: ['На сверке', 'info'], paid: ['Оплачен', 'success'], void: ['Аннулирован', undefined],
};
const stateOf = (i: { status: string; outstanding: Money; pending: Money }): State =>
  i.status === 'void' ? 'void' : i.outstanding.amountMinor === '0' ? 'paid' : i.pending.amountMinor !== '0' ? 'review' : 'due';
const moneyList = (ms: Money[]) => ms.length ? ms.map(money).join(' + ') : '0';
const today = new Date().toISOString().slice(0, 10);

export function BillingPage() {
  const [channel, setChannel] = useState<'suppliers' | 'clients'>('suppliers');
  return <Page title="Счета и оплаты" subtitle="Расчёты по закупкам и розничным продажам. Деньги через платформу не проходят — фиксируются подтверждения.">
    <Tabs channel value={channel} onChange={setChannel} tabs={[['suppliers', 'Поставщикам'], ['clients', 'От клиентов']]} />
    {channel === 'suppliers' ? <SupplierInvoices /> : <ClientInvoices />}
  </Page>;
}

function Summary({ rows, overdue }: { rows: { status: string; total: Money; paid: Money; pending: Money; outstanding: Money }[]; overdue: Money[] }) {
  const live = rows.filter((r) => r.status !== 'void');
  return <Stats>
    <Stat label="К оплате" value={moneyList(sumMoney(live.map((r) => r.outstanding)))} note="остаток по выставленным счетам" />
    <Stat label="На сверке" value={moneyList(sumMoney(live.map((r) => r.pending)))} note="подтверждение отправлено" />
    <Stat label="Оплачено" value={moneyList(sumMoney(live.map((r) => r.paid)))} note="принятые оплаты" />
    <Stat label="Просрочено" value={moneyList(sumMoney(overdue))} note={overdue.length ? 'срок оплаты прошёл' : 'нет просрочки'} />
  </Stats>;
}

type SupplierRow = Invoice & { order: Order };

function SupplierInvoices() {
  const q = useData(['billing', 'suppliers'], async () => {
    const orders = (await list<Order>('/commerce/orders?limit=100')).filter((o) => o.party === 'buyer');
    const per = await Promise.all(orders.map((o) => list<Invoice>(`/commerce/orders/${o.id}/invoices`).then((is) => is.map((i): SupplierRow => ({ ...i, order: o })))));
    return per.flat();
  });
  const lines = useLinesLabel();
  const [query, setQuery] = useSearchQuery();
  const [state, setState] = useState('');
  const [open, setOpen] = useState<SupplierRow | null>(null);
  const all = q.data ?? [];
  const late = (i: SupplierRow) => stateOf(i) === 'due' && i.schedule.some((p) => p.dueDate < today);
  const rows = all.filter((i) => (!state || stateOf(i) === state) && matches(query, i.order.supplier.name, lines(i.order.terms.lines)));
  return <>
    <Summary rows={all} overdue={all.filter(late).map((i) => i.outstanding)} />
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Поставщик или автомобиль" onReset={() => setState('')}>
        <FilterSelect value={state} onChange={setState} all="Все статусы" options={Object.entries(stateLabel).map(([k, [l]]) => [k, l])} />
      </Toolbar>
      <ResultMeta>{plural(rows.length, ['счёт', 'счёта', 'счетов'])} · оплаты оптовых продаж партнёрам — в карточке заказа</ResultMeta>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(i) => i.id} onRowClick={setOpen} empty="Счетов от поставщиков пока нет" columns={[
        { title: 'Счёт', render: (i) => <Cell main={`Счёт · ${i.order.supplier.name}`} sub={i.schedule.length ? `до ${date(i.schedule[i.schedule.length - 1]!.dueDate)}` : undefined} /> },
        { title: 'Заказ', render: (i) => lines(i.order.terms.lines) },
        { title: 'Поставщик', render: (i) => i.order.supplier.name },
        { title: 'Сумма', render: (i) => <Cell main={money(i.total)} sub={`остаток ${money(i.outstanding)}`} /> },
        { title: 'Статус', render: (i) => { const [l, t] = stateLabel[stateOf(i)]; return <Badge tone={late(i) ? 'danger' : t}>{late(i) ? 'Просрочен' : l}</Badge>; } },
        { title: 'Действие', render: (i) => <Button size="sm" variant={stateOf(i) === 'due' ? 'primary' : 'secondary'} onClick={() => setOpen(i)}>{stateOf(i) === 'due' ? 'Добавить оплату' : 'Открыть'}</Button> },
      ]} />
    </Panel>
    {open && <Modal title={`Счёт · ${open.order.supplier.name}`} icon="wallet" size="wide" onClose={() => setOpen(null)} help={lines(open.order.terms.lines)}>
      <InvoiceById kind="supplier" id={open.id} orderId={open.order.id} /></Modal>}
  </>;
}

/** Re-reads the invoice so the dialog shows fresh payments after each action. */
function InvoiceById({ id, orderId }: { kind: 'supplier'; id: string; orderId: string }) {
  const q = useData(['order-invoices', orderId], () => list<Invoice>(`/commerce/orders/${orderId}/invoices`));
  const i = q.data?.find((x) => x.id === id);
  return i ? <InvoicePanel invoice={i} party="buyer" refresh={[['order-invoices', orderId], ['billing']]} /> : <p className="cell-sub">Загрузка…</p>;
}

type ClientRow = RetailInvoice & { deal: Deal };

function ClientInvoices() {
  const q = useData(['billing', 'clients'], async () => {
    const deals = await list<Deal>('/retail/deals?limit=100');
    const full = await Promise.all(deals.map((d) => get<Deal>(`/retail/deals/${d.id}`).then((r) => r.data)));
    return full.flatMap((d) => (d.invoices ?? []).map((i): ClientRow => ({ ...i, deal: d })));
  });
  const [query, setQuery] = useSearchQuery();
  const [state, setState] = useState('');
  const [open, setOpen] = useState<ClientRow | null>(null);
  const all = (q.data ?? []).map((i) => ({ ...i, total: i.amount }));
  const late = (i: ClientRow) => stateOf(i) === 'due' && !!i.dueDate && i.dueDate < today;
  const rows = all.filter((i) => (!state || stateOf(i) === state) && matches(query, i.deal.customer.displayName, i.recipientSnapshot, purposeLabel[i.purpose]));
  return <>
    <Summary rows={all} overdue={all.filter(late).map((i) => i.outstanding)} />
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Клиент или назначение" onReset={() => setState('')}>
        <FilterSelect value={state} onChange={setState} all="Все статусы" options={Object.entries(stateLabel).map(([k, [l]]) => [k, l])} />
      </Toolbar>
      <ResultMeta>{plural(rows.length, ['счёт', 'счёта', 'счетов'])} · платежи клиента банку / МФО сюда не попадают</ResultMeta>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(i) => i.id} onRowClick={setOpen} empty="Счетов клиентам пока нет" columns={[
        { title: 'Счёт', render: (i) => <Cell main={purposeLabel[i.purpose] ?? i.purpose} sub={i.dueDate ? `до ${date(i.dueDate)}` : undefined} /> },
        { title: 'Клиент', render: (i) => <Cell main={i.deal.customer.displayName} sub={i.recipientSnapshot} /> },
        { title: 'Сумма', render: (i) => <Cell main={money(i.amount)} sub={`остаток ${money(i.outstanding)}`} /> },
        { title: 'Статус', render: (i) => { const [l, t] = stateLabel[stateOf(i)]; return <Badge tone={late(i) ? 'danger' : t}>{late(i) ? 'Просрочен' : l}</Badge>; } },
        { title: 'Действие', render: (i) => <Button size="sm" onClick={() => setOpen(i)}>Открыть</Button> },
      ]} />
    </Panel>
    {open && <Modal title={`${purposeLabel[open.purpose] ?? 'Счёт'} · ${open.deal.customer.displayName}`} icon="wallet" size="wide" onClose={() => setOpen(null)}>
      <RetailInvoiceById dealId={open.deal.id} id={open.id} /></Modal>}
  </>;
}

function RetailInvoiceById({ dealId, id }: { dealId: string; id: string }) {
  const q = useData(['deal', dealId], () => get<Deal>(`/retail/deals/${dealId}`));
  const i = q.data?.data.invoices?.find((x) => x.id === id);
  return i ? <RetailInvoicePanel invoice={i} refresh={[['deal', dealId], ['billing']]} /> : <p className="cell-sub">Загрузка…</p>;
}
