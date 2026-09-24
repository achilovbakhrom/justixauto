import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ActionButton,
  Badge,
  Button,
  Cell,
  EmptyState,
  FilterSelect,
  Modal,
  Page,
  Panel,
  Progress,
  ResultMeta,
  Stat,
  Stats,
  Table,
  Toolbar,
  matches,
  money,
  plural,
  post,
  useScopeLabel,
  useSearchQuery,
  useSession,
} from '@justixauto/kit';
import {
  orderLabel,
  orderTone,
  useBranches,
  useDeals,
  useLinesLabel,
  useListings,
  useModels,
  useOrders,
  usePartners,
  useRFQs,
  useVehicles,
  useWarehouses,
  warehouseFields,
  warehouseInput,
} from '../data';
import type { Vehicle } from '../data';
import { ModelsPanel, VehicleDialog, WarehouseDialog } from './inventory';
import { OrderDialog } from './trade';

const active = ['awaiting-supplier', 'accepted', 'fulfilling'];

export function DashboardPage() {
  const navigate = useNavigate();
  const orders = useOrders();
  const rfqs = useRFQs();
  const deals = useDeals();
  const partners = usePartners();
  const lines = useLinesLabel();
  const [open, setOpen] = useState<string | null>(null);
  const buying =
    (orders.data ?? []).filter((o) => o.party === 'buyer' && active.includes(o.status)).length +
    (rfqs.data ?? []).filter((r) => ['sent', 'negotiating'].includes(r.status)).length;
  const selling = (orders.data ?? []).filter((o) => o.party === 'supplier');
  return (
    <Page title="Дашборд" subtitle={<ScopeLine />}>
      <Stats>
        <Stat label="Закупки" value={buying} note="запросы и заказы поставщикам" />
        <Stat
          label="Розничные продажи"
          value={(deals.data ?? []).filter((d) => d.status === 'reserved').length}
          note="активные сделки с клиентами"
        />
        <Stat
          label="Оптовые продажи"
          value={selling.filter((o) => active.includes(o.status)).length}
          note="активные заказы партнёров"
        />
        <Stat
          label="Партнёры"
          value={(partners.data ?? []).filter((p) => p.status === 'active').length}
          note="активные отношения"
        />
      </Stats>
      <Panel title="Быстрые действия" padded>
        <div className="kit-row">
          <Button onClick={() => navigate('/purchases?catalog=1')}>Предложения поставщиков</Button>
          <Button onClick={() => navigate('/warehouses')}>Открыть склады</Button>
          <Button variant="primary" onClick={() => navigate('/sales?new=1')}>
            Продать клиенту
          </Button>
          <Button variant="primary" onClick={() => navigate('/offers?channel=partners')}>
            Продать партнёру
          </Button>
        </div>
      </Panel>
      <Panel
        title="Последние продажи партнёрам"
        actions={
          <Button size="sm" onClick={() => navigate('/sales?channel=partners')}>
            Все оптовые продажи
          </Button>
        }
      >
        {selling.length === 0 && !orders.isLoading ? (
          <EmptyState
            icon="cart"
            title="Пока нет оптовых продаж"
            text="Опубликуйте предложение для партнёров; их заказы появятся здесь."
            action={
              <Button variant="primary" onClick={() => navigate('/offers?channel=partners')}>
                Предложения партнёрам
              </Button>
            }
          />
        ) : (
          <Table
            rows={selling.slice(0, 5)}
            loading={orders.isLoading}
            error={orders.error}
            rowKey={(o) => o.id}
            onRowClick={(o) => setOpen(o.id)}
            columns={[
              {
                title: 'Покупатель',
                render: (o) => <Cell main={o.buyer.name} sub={o.source === 'rfq' ? 'По котировке' : 'Прямой заказ'} />,
              },
              { title: 'Автомобили', render: (o) => lines(o.terms.lines) },
              { title: 'Сумма', render: (o) => money(o.total) },
              { title: 'Статус', render: (o) => <Badge tone={orderTone(o.status)}>{orderLabel[o.status]}</Badge> },
            ]}
          />
        )}
      </Panel>
      {open && <OrderDialog id={open} onClose={() => setOpen(null)} />}
    </Page>
  );
}

/** "Company · branch scope" under page titles. */
function ScopeLine() {
  const s = useSession();
  return (
    <>
      {s.company?.name} · {useScopeLabel()}
    </>
  );
}

export function WarehousesPage() {
  const q = useWarehouses();
  const branches = useBranches();
  const vehicles = useVehicles();
  const [query, setQuery] = useSearchQuery();
  const [branch, setBranch] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const branchName = (id: string | null) =>
    id ? (branches.data?.find((b) => b.id === id)?.name ?? '—') : 'Общий склад компании';
  const withVin = (id: string) => (vehicles.data ?? []).filter((v) => v.placement?.warehouseId === id).length;
  const rows = (q.data ?? []).filter(
    (w) => matches(query, w.name, w.city, w.address, branchName(w.branchId)) && (!branch || w.branchId === branch),
  );
  const total = (k: 'capacity' | 'occupied' | 'free') => (q.data ?? []).reduce((n, w) => n + Number(w[k]), 0);
  const cars = (vehicles.data ?? []).filter((v) => v.placement).length;
  return (
    <Page
      title="Склады"
      subtitle={`${plural(q.data?.length ?? 0, ['склад', 'склада', 'складов'])} · ${plural(total('occupied'), ['автомобиль', 'автомобиля', 'автомобилей'])} на хранении`}
      actions={
        <ActionButton
          label="Добавить склад"
          variant="primary"
          refresh={[['warehouses']]}
          fields={[
            ...warehouseFields(),
            { name: 'capacity', label: 'Вместимость, автомобилей', type: 'number', required: true },
            {
              name: 'branchId',
              label: 'Принадлежность (пусто — общий склад компании)',
              type: 'select',
              options: (branches.data ?? []).map((b) => [b.id, b.name]),
            },
          ]}
          onSubmit={(v) =>
            post('/inventory/warehouses', { ...warehouseInput(v), capacity: v.capacity, branchId: v.branchId || null })
          }
        />
      }
    >
      <Stats>
        <Stat label="Активные склады" value={q.data?.length ?? '…'} note={`из ${q.data?.length ?? 0} складов`} />
        <Stat label="Автомобили" value={total('occupied')} note={`${total('occupied') - cars} без внесённого VIN`} />
        <Stat label="Занято" value={total('occupied')} note={`из ${total('capacity')} мест`} />
        <Stat label="Свободно" value={total('free')} note="по всем складам" />
      </Stats>
      <Panel>
        <Toolbar
          query={query}
          onQuery={setQuery}
          placeholder="Название, филиал, город или адрес"
          onReset={() => setBranch('')}
        >
          <FilterSelect
            value={branch}
            onChange={setBranch}
            all="Все филиалы"
            options={(branches.data ?? []).map((b) => [b.id, b.name])}
          />
        </Toolbar>
        <ResultMeta>
          {plural(rows.length, ['склад', 'склада', 'складов'])} · выберите строку, чтобы открыть склад
        </ResultMeta>
        <Table
          rows={rows}
          loading={q.isLoading}
          error={q.error}
          rowKey={(w) => w.id}
          onRowClick={(w) => setOpen(w.id)}
          empty="Складов пока нет"
          columns={[
            { title: 'Склад', render: (w) => <Cell main={w.name} /> },
            {
              title: 'Филиал',
              render: (w) => (
                <Cell main={branchName(w.branchId)} sub={w.branchId ? 'Основной склад филиала' : undefined} />
              ),
            },
            {
              title: 'Местоположение',
              render: (w) => <Cell main={w.city} sub={`${w.country.label} · ${w.address}`} />,
            },
            { title: 'С VIN', render: (w) => withVin(w.id) },
            { title: 'Без VIN', render: (w) => Number(w.occupied) - withVin(w.id) },
            {
              title: 'Занято / всего',
              render: (w) => (
                <>
                  <div>
                    {w.occupied} / {w.capacity}
                  </div>
                  <Progress value={Number(w.occupied)} max={Number(w.capacity)} />
                </>
              ),
            },
            {
              title: 'Статус',
              render: (w) => (
                <Badge tone={w.free === '0' ? 'warning' : 'success'}>{w.free === '0' ? 'Заполнен' : 'Активен'}</Badge>
              ),
            },
          ]}
        />
      </Panel>
      {open && <WarehouseDialog id={open} onClose={() => setOpen(null)} />}
    </Page>
  );
}

type VehicleState = 'available' | 'reserved' | 'outside';
const vehicleState = (v: Vehicle): VehicleState => (v.reserved ? 'reserved' : v.placement ? 'available' : 'outside');
const stateLabel: Record<VehicleState, [string, 'success' | 'warning' | undefined]> = {
  available: ['Доступен', 'success'],
  reserved: ['Зарезервирован', 'warning'],
  outside: ['Вне склада', undefined],
};

export function VehiclesPage() {
  const q = useVehicles();
  const models = useModels();
  const warehouses = useWarehouses();
  const listings = useListings();
  const [query, setQuery] = useSearchQuery();
  const [warehouse, setWarehouse] = useState('');
  const [state, setState] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const [catalog, setCatalog] = useState(false);
  const model = (id: string) => models.data?.find((m) => m.id === id);
  const listed = new Set((listings.data ?? []).filter((l) => l.status === 'published').map((l) => l.vehicleId));
  const all = q.data ?? [];
  const rows = all.filter((v) => {
    const m = model(v.modelId)?.specification;
    return (
      matches(query, v.vin, m?.make, m?.model, m?.variant) &&
      (!warehouse || v.placement?.warehouseId === warehouse) &&
      (!state || vehicleState(v) === state)
    );
  });
  return (
    <Page
      title="Автомобили"
      subtitle="Принятые автомобили: наличие, резерв и готовность к продаже"
      actions={
        <Button icon="car" onClick={() => setCatalog(true)}>
          Каталог моделей
        </Button>
      }
    >
      <Stats>
        <Stat
          label="Доступны"
          value={all.filter((v) => vehicleState(v) === 'available').length}
          note="можно предложить клиенту"
        />
        <Stat label="Зарезервированы" value={all.filter((v) => v.reserved).length} note="активный резерв" />
        <Stat label="С предложением" value={all.filter((v) => listed.has(v.id)).length} note="опубликовано клиентам" />
        <Stat label="Всего" value={all.length} note="в реестре компании" />
      </Stats>
      <Panel>
        <Toolbar
          query={query}
          onQuery={setQuery}
          placeholder="VIN, бренд или модель"
          onReset={() => {
            setWarehouse('');
            setState('');
          }}
        >
          <FilterSelect
            value={warehouse}
            onChange={setWarehouse}
            all="Все склады"
            options={(warehouses.data ?? []).map((w) => [w.id, w.name])}
          />
          <FilterSelect
            value={state}
            onChange={setState}
            all="Все статусы"
            options={Object.entries(stateLabel).map(([k, [l]]) => [k, l])}
          />
        </Toolbar>
        <ResultMeta>
          {plural(rows.length, ['автомобиль', 'автомобиля', 'автомобилей'])} · нажмите строку, чтобы открыть карточку
        </ResultMeta>
        <Table
          rows={rows}
          loading={q.isLoading}
          error={q.error}
          rowKey={(v) => v.id}
          onRowClick={(v) => setOpen(v.id)}
          empty="Автомобилей пока нет"
          columns={[
            {
              title: 'Бренд',
              render: (v) => (
                <Cell
                  main={model(v.modelId)?.specification.make ?? '—'}
                  sub={model(v.modelId)?.specification.bodyType}
                />
              ),
            },
            {
              title: 'Модель / комплектация',
              render: (v) => (
                <Cell
                  main={model(v.modelId)?.specification.model ?? '—'}
                  sub={model(v.modelId)?.specification.variant}
                />
              ),
            },
            {
              title: 'Год / цвета',
              render: (v) => {
                const m = model(v.modelId)?.specification;
                return <Cell main={m?.year ?? '—'} sub={m ? `${m.exteriorColor} / ${m.interiorColor}` : undefined} />;
              },
            },
            { title: 'VIN', render: (v) => v.vin },
            {
              title: 'Склад',
              render: (v) => warehouses.data?.find((w) => w.id === v.placement?.warehouseId)?.name ?? 'Вне склада',
            },
            {
              title: 'Статус',
              render: (v) => {
                const [l, t] = stateLabel[vehicleState(v)];
                return <Badge tone={t}>{l}</Badge>;
              },
            },
          ]}
        />
      </Panel>
      {open && <VehicleDialog id={open} onClose={() => setOpen(null)} />}
      {catalog && (
        <Modal
          title="Каталог моделей"
          icon="car"
          size="wide"
          onClose={() => setCatalog(false)}
          help="Общий каталог платформы: спецификации версионируются, VIN ссылается на точную версию."
        >
          <ModelsPanel />
        </Modal>
      )}
    </Page>
  );
}
