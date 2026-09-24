import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  ActionButton,
  Badge,
  Button,
  Cell,
  FilterSelect,
  Page,
  Panel,
  ResultMeta,
  Stat,
  Stats,
  Table,
  Tabs,
  Toolbar,
  date,
  matches,
  money,
  plural,
  post,
  useFinanceApplications,
  useSearchQuery,
} from '@justixauto/kit';
import {
  dealLabel,
  orderLabel,
  orderTone,
  schemeLabel,
  stageLabel,
  useBranches,
  useCustomers,
  useDeals,
  useLeads,
  useLinesLabel,
  useModels,
  useOrders,
  useVehicles,
} from '../data';
import { DealDialog } from './retail';
import { OrderDialog } from './trade';
import { FinancingPanel } from './partners-finance';

type View = 'active' | 'delivered' | 'installments' | 'finance';

export function SalesPage() {
  const [params, setParams] = useSearchParams();
  const channel = params.get('channel') === 'partners' ? 'partners' : 'clients';
  const setChannel = (c: 'clients' | 'partners') =>
    setParams(c === 'partners' ? { channel: c } : {}, { replace: true });
  return (
    <Page
      title="Продажи"
      subtitle="Розничные и оптовые продажи вашей компании"
      actions={channel === 'clients' && <NewSale autoOpen={params.get('new') === '1'} />}
    >
      <Tabs
        channel
        value={channel}
        onChange={setChannel}
        tabs={[
          ['clients', 'Клиентам'],
          ['partners', 'Партнёрам'],
        ]}
      />
      {channel === 'clients' ? <ClientSales /> : <PartnerSales />}
    </Page>
  );
}

function useCarLabel() {
  const vehicles = useVehicles();
  const models = useModels();
  return (id: string) => {
    const v = vehicles.data?.find((x) => x.id === id);
    const m = models.data?.find((x) => x.id === v?.modelId)?.specification;
    return { model: m ? `${m.make} ${m.model} ${m.variant}` : '—', vin: v?.vin ?? '' };
  };
}

function NewSale({ autoOpen }: { autoOpen: boolean }) {
  const customers = useCustomers();
  const vehicles = useVehicles('warehouse');
  const branches = useBranches();
  const leads = useLeads();
  const car = useCarLabel();
  return (
    <ActionButton
      label="Новая продажа"
      variant="primary"
      refresh={[['deals'], ['leads'], ['vehicles']]}
      {...(autoOpen ? { defaultOpen: true } : {})}
      intro={<p>Продажа резервирует выбранный автомобиль; клиент и лид должны уже существовать.</p>}
      fields={[
        {
          name: 'customerId',
          label: 'Клиент',
          type: 'select',
          required: true,
          options: (customers.data ?? []).map((c) => [c.id, `${c.displayName}${c.phone ? ` · ${c.phone}` : ''}`]),
        },
        {
          name: 'leadId',
          label: 'Лид (необязательно)',
          type: 'select',
          options: (leads.data ?? [])
            .filter((l) => !l.dealId && ['qualified', 'test-drive', 'negotiation'].includes(l.stage))
            .map((l) => [l.id, `${l.customer?.displayName ?? ''} · ${stageLabel[l.stage]}`] as [string, string]),
        },
        {
          name: 'vehicleId',
          label: 'Автомобиль',
          type: 'select',
          required: true,
          options: (vehicles.data ?? []).filter((v) => !v.reserved).map((v) => [v.id, `${car(v.id).model} · ${v.vin}`]),
        },
        {
          name: 'branchId',
          label: 'Филиал',
          type: 'select',
          required: true,
          options: (branches.data ?? []).map((b) => [b.id, b.name]),
        },
        {
          name: 'paymentScheme',
          label: 'Способ оформления',
          type: 'select',
          required: true,
          options: Object.entries(schemeLabel),
        },
        { name: 'price', label: 'Цена', type: 'money', required: true },
      ]}
      onSubmit={(v) => post('/retail/deals', { ...v, leadId: v.leadId || null })}
    />
  );
}

function ClientSales() {
  const q = useDeals();
  const branches = useBranches();
  const finance = useFinanceApplications();
  const car = useCarLabel();
  const [view, setView] = useState<View>('active');
  const [query, setQuery] = useSearchQuery();
  const [scheme, setScheme] = useState('');
  const [branch, setBranch] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const all = q.data ?? [];
  const inView = (d: (typeof all)[number]) =>
    view === 'active'
      ? d.status === 'reserved'
      : view === 'delivered'
        ? d.status === 'delivered'
        : view === 'installments'
          ? d.paymentScheme === 'own-installment' && d.status !== 'cancelled'
          : false;
  const rows = all.filter(
    (d) =>
      inView(d) &&
      (!scheme || d.paymentScheme === scheme) &&
      (!branch || d.branchId === branch) &&
      matches(query, d.customer.displayName, d.customer.phone, car(d.vehicleId).vin, car(d.vehicleId).model),
  );
  const count = (v: View) =>
    all.filter((d) =>
      v === 'active'
        ? d.status === 'reserved'
        : v === 'delivered'
          ? d.status === 'delivered'
          : d.paymentScheme === 'own-installment' && d.status !== 'cancelled',
    ).length;
  return (
    <>
      <Stats>
        <Stat label="Активные продажи" value={count('active')} note="до выдачи автомобиля" />
        <Stat label="Рассрочки" value={count('installments')} note="собственная рассрочка продавца" />
        <Stat
          label="Заявки в банк / МФО"
          value={(finance.data ?? []).filter((a) => !['declined', 'agreed'].includes(a.status)).length}
          note="на рассмотрении у партнёров"
        />
        <Stat label="Выдано" value={count('delivered')} note="автомобилей клиентам" />
      </Stats>
      <Panel>
        <Tabs
          value={view}
          onChange={setView}
          tabs={[
            ['active', 'Активные продажи', count('active')],
            ['delivered', 'Выданные', count('delivered')],
            ['installments', 'Рассрочки', count('installments')],
            ['finance', 'Банк / МФО', finance.data?.length],
          ]}
        />
        {view === 'finance' ? (
          <FinancingPanel />
        ) : (
          <>
            <Toolbar
              query={query}
              onQuery={setQuery}
              placeholder="Клиент, телефон или VIN"
              onReset={() => {
                setScheme('');
                setBranch('');
              }}
            >
              <FilterSelect
                value={scheme}
                onChange={setScheme}
                all="Все способы оформления"
                options={Object.entries(schemeLabel)}
              />
              <FilterSelect
                value={branch}
                onChange={setBranch}
                all="Все филиалы"
                options={(branches.data ?? []).map((b) => [b.id, b.name])}
              />
            </Toolbar>
            <ResultMeta>{plural(rows.length, ['продажа', 'продажи', 'продаж'])}</ResultMeta>
            <Table
              rows={rows}
              loading={q.isLoading}
              error={q.error}
              rowKey={(d) => d.id}
              onRowClick={(d) => setOpen(d.id)}
              empty="Продаж в этом разделе нет"
              columns={[
                {
                  title: 'Сделка',
                  render: (d) => <Cell main={date(d.updatedAt)} sub={schemeLabel[d.paymentScheme]} />,
                },
                { title: 'Клиент', render: (d) => <Cell main={d.customer.displayName} sub={d.customer.phone} /> },
                {
                  title: 'Автомобиль',
                  render: (d) => <Cell main={car(d.vehicleId).model} sub={car(d.vehicleId).vin} />,
                },
                { title: 'Филиал', render: (d) => branches.data?.find((b) => b.id === d.branchId)?.name ?? '—' },
                {
                  title: 'Этап',
                  render: (d) => (
                    <Badge tone={d.status === 'delivered' ? 'success' : d.status === 'cancelled' ? 'danger' : 'info'}>
                      {dealLabel[d.status]}
                    </Badge>
                  ),
                },
                { title: 'Сумма', render: (d) => money(d.price) },
                {
                  title: 'Действие',
                  render: (d) => (
                    <Button
                      size="sm"
                      variant={d.status === 'reserved' ? 'primary' : 'secondary'}
                      onClick={() => setOpen(d.id)}
                    >
                      {d.status === 'reserved' ? 'Продолжить' : 'Открыть'}
                    </Button>
                  ),
                },
              ]}
            />
          </>
        )}
      </Panel>
      {open && <DealDialog id={open} onClose={() => setOpen(null)} />}
    </>
  );
}

/** Supplier side of wholesale: partners' orders to us. */
function PartnerSales() {
  const q = useOrders();
  const lines = useLinesLabel();
  const [query, setQuery] = useSearchQuery();
  const [view, setView] = useState<'open' | 'done'>('open');
  const [open, setOpen] = useState<string | null>(null);
  const mine = (q.data ?? []).filter((o) => o.party === 'supplier');
  const isOpen = (s: string) => ['awaiting-supplier', 'accepted', 'fulfilling'].includes(s);
  const rows = mine.filter(
    (o) => (view === 'open') === isOpen(o.status) && matches(query, o.buyer.name, lines(o.terms.lines)),
  );
  return (
    <>
      <Stats>
        <Stat
          label="Ждут подтверждения"
          value={mine.filter((o) => o.status === 'awaiting-supplier').length}
          note="нужно принять решение"
        />
        <Stat
          label="В исполнении"
          value={mine.filter((o) => ['accepted', 'fulfilling'].includes(o.status)).length}
          note="VIN, отгрузка, счёт"
        />
        <Stat label="Выполнено" value={mine.filter((o) => o.status === 'completed').length} note="закрытые заказы" />
        <Stat label="Партнёров" value={new Set(mine.map((o) => o.buyer.name)).size} note="покупали у вас" />
      </Stats>
      <Panel>
        <Tabs
          value={view}
          onChange={setView}
          tabs={[
            ['open', 'В работе', mine.filter((o) => isOpen(o.status)).length],
            ['done', 'Завершённые', mine.filter((o) => !isOpen(o.status)).length],
          ]}
        />
        <Toolbar query={query} onQuery={setQuery} placeholder="Покупатель или автомобиль" />
        <ResultMeta>{plural(rows.length, ['заказ', 'заказа', 'заказов'])}</ResultMeta>
        <Table
          rows={rows}
          loading={q.isLoading}
          error={q.error}
          rowKey={(o) => o.id}
          onRowClick={(o) => setOpen(o.id)}
          empty="Оптовых продаж пока нет"
          columns={[
            {
              title: 'Заказ',
              render: (o) => (
                <Cell main={date(o.updatedAt)} sub={o.source === 'rfq' ? 'По котировке' : 'По предложению'} />
              ),
            },
            { title: 'Покупатель', render: (o) => o.buyer.name },
            { title: 'Автомобили', render: (o) => lines(o.terms.lines) },
            { title: 'Этап', render: (o) => <Badge tone={orderTone(o.status)}>{orderLabel[o.status]}</Badge> },
            { title: 'Сумма', render: (o) => money(o.total) },
            {
              title: 'Действие',
              render: (o) => (
                <Button
                  size="sm"
                  variant={o.status === 'awaiting-supplier' ? 'primary' : 'secondary'}
                  onClick={() => setOpen(o.id)}
                >
                  {o.status === 'awaiting-supplier' ? 'Принять решение' : 'Открыть'}
                </Button>
              ),
            },
          ]}
        />
      </Panel>
      {open && <OrderDialog id={open} onClose={() => setOpen(null)} />}
    </>
  );
}
