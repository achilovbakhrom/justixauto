import { useState } from 'react';
import {
  ActionButton, Badge, Details, Modal, Page, Panel, Stat, Table, Tabs, date, dateTime, get, list, patch, post, useData, useRefresh,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { modelName, useBranches, useModelName, useModels, useVehicles, useWarehouses } from '../data';
import type { Model, Vehicle, Warehouse } from '../data';

interface Batch { id: string; modelId: string; modelSpecificationVersion: string; confirmedQuantity: string; identifiedCount: string; unidentifiedCount: string; receivedAt: string; revision: string }
interface Stock { warehouse: Warehouse; vehicles: Vehicle[]; unidentifiedBatches: Batch[] }

const warehouseFields = (w?: Warehouse): FieldSpec[] => [
  { name: 'name', label: 'Название', type: 'text', required: true, initial: w?.name ?? '' },
  { name: 'country', label: 'Страна', type: 'text', required: true, initial: w?.country.label ?? '' },
  { name: 'city', label: 'Город', type: 'text', required: true, initial: w?.city ?? '' },
  { name: 'address', label: 'Адрес', type: 'text', required: true, initial: w?.address ?? '' },
];
const warehouseInput = (v: Record<string, unknown>) => ({ name: v.name, country: { label: v.country }, city: v.city, address: v.address });

export function WarehousesPage() {
  const q = useWarehouses();
  const branches = useBranches();
  const branchName = (id: string | null) => id ? branches.data?.find((b) => b.id === id)?.name ?? '—' : 'Общий склад компании';
  const [open, setOpen] = useState<string | null>(null);
  return <Page title="Склады" subtitle="Занято = размещённые автомобили + ожидающие ввода VIN"
    actions={<ActionButton label="Добавить склад" variant="primary" refresh={[['warehouses']]}
      fields={[...warehouseFields(), { name: 'capacity', label: 'Вместимость (машин)', type: 'number', required: true },
        { name: 'branchId', label: 'Основной склад филиала (необязательно)', type: 'select', options: (branches.data ?? []).map((b) => [b.id, b.name]) }]}
      onSubmit={(v) => post('/inventory/warehouses', { ...warehouseInput(v), capacity: v.capacity, branchId: v.branchId || null })} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(w) => w.id} onRowClick={(w) => setOpen(w.id)} columns={[
      { title: 'Склад', render: (w) => <b>{w.name}</b> }, { title: 'Филиал', render: (w) => branchName(w.branchId) },
      { title: 'Город', render: (w) => w.city },
      { title: 'Вместимость', render: (w) => w.capacity }, { title: 'Занято', render: (w) => w.occupied },
      { title: 'Свободно', render: (w) => <Badge tone={w.free === '0' ? 'danger' : 'success'}>{w.free}</Badge> },
    ]} /></Panel>
    {open && <WarehouseDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

function WarehouseDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['stock', id], () => get<Stock>(`/inventory/warehouses/${id}/inventory`));
  const models = useModels();
  const name = useModelName();
  const reload = useRefresh();
  const s = q.data?.data;
  const w = s?.warehouse;
  const refresh = [['stock', id], ['warehouses'], ['vehicles']];
  const modelOptions = (models.data ?? []).map((m): [string, string] => [m.id, modelName(m)]);
  const branches = useBranches();
  return <Modal title={w?.name ?? 'Склад'} onClose={onClose} size="wide" footer={w && <>
    <ActionButton label="Принять автомобили" variant="primary" refresh={refresh} fields={[
      { name: 'modelId', label: 'Модель', type: 'select', required: true, options: modelOptions },
      { name: 'mode', label: 'Приёмка', type: 'select', required: true, options: [['identified', 'С VIN'], ['unidentified', 'Количество, VIN позже']], initial: 'identified' },
      { name: 'vins', label: 'VIN (по одному в строке)', type: 'textarea' },
      { name: 'quantity', label: 'Количество (без VIN)', type: 'number' },
      { name: 'receivedAt', label: 'Дата приёмки', type: 'datetime', required: true },
    ]} onSubmit={(v) => {
      const m = models.data?.find((x) => x.id === v.modelId);
      const stock = v.mode === 'identified'
        ? { mode: 'identified', vins: String(v.vins).split(/\s+/).filter(Boolean) }
        : { mode: 'unidentified', quantity: v.quantity };
      return post(`/inventory/warehouses/${id}/receipt-batches`, { modelId: v.modelId, modelSpecificationVersion: m?.specification.version ?? '1',
        stock, receivedAt: v.receivedAt }, { ifMatch: w.revision });
    }} />
    <ActionButton label="Изменить вместимость" refresh={refresh} fields={[
      { name: 'capacity', label: 'Новая вместимость', type: 'number', required: true, initial: w.capacity },
      { name: 'reason', label: 'Основание', type: 'textarea', required: true },
    ]} onSubmit={(v) => post(`/inventory/warehouses/${id}/capacity-changes`, v, { ifMatch: w.revision })} />
    <ActionButton label="Реквизиты" fields={warehouseFields(w)} refresh={refresh}
      onSubmit={(v) => patch(`/inventory/warehouses/${id}`, warehouseInput(v), { ifMatch: w.revision })} />
    <ActionButton label="Филиал" refresh={refresh}
      intro={<p>У филиала может быть один основной склад. Отвязка сохраняет склад, вместимость и автомобили.</p>}
      fields={[{ name: 'branchId', label: 'Основной склад филиала', type: 'select', initial: w.branchId ?? '',
        options: (branches.data ?? []).map((b) => [b.id, b.name]) }]}
      onSubmit={(v) => post(`/inventory/warehouses/${id}/branch-attachment`, { branchId: v.branchId || null }, { ifMatch: w.revision })} />
  </>}>
    {w && <div className="kit-grid"><Stat label="Вместимость" value={w.capacity} /><Stat label="Занято" value={w.occupied} /><Stat label="Свободно" value={w.free} /></div>}
    <Panel title="Автомобили"><Table rows={s?.vehicles} rowKey={(v) => v.id} columns={[
      { title: 'VIN', render: (v) => <code>{v.vin}</code> }, { title: 'Модель', render: (v) => name(v.modelId) },
      { title: 'Размещён', render: (v) => date(v.placement?.placedAt) },
    ]} empty="На складе нет автомобилей с VIN" /></Panel>
    <Panel title="Ожидают ввода VIN"><Table rows={s?.unidentifiedBatches} rowKey={(b) => b.id} columns={[
      { title: 'Модель', render: (b) => name(b.modelId) }, { title: 'Принято', render: (b) => b.confirmedQuantity },
      { title: 'Без VIN', render: (b) => b.unidentifiedCount }, { title: 'Дата', render: (b) => date(b.receivedAt) },
      { title: '', render: (b) => <div className="kit-row"><ActionButton label="Ввести VIN" fields={[{ name: 'vins', label: 'VIN (по одному в строке)', type: 'textarea', required: true }]}
        onSubmit={async (v) => {
          await post(`/inventory/receipt-batches/${b.id}/identifications`, { atomic: true,
            items: String(v.vins).split(/\s+/).filter(Boolean).map((vin) => ({ vin, modelId: b.modelId })) });
          await reload(...refresh);
        }} />
        <ActionButton label="Исправить количество" refresh={refresh}
          intro={<p>Пересчёт партии фиксируется в истории склада. Нельзя указать меньше, чем уже введено VIN ({b.identifiedCount}).</p>}
          fields={[{ name: 'quantity', label: 'Верное количество', type: 'number', required: true, initial: b.confirmedQuantity },
            { name: 'reason', label: 'Основание', type: 'textarea', required: true }]}
          onSubmit={(v) => post(`/inventory/receipt-batches/${b.id}/quantity-corrections`, v, { ifMatch: b.revision })} /></div> },
    ]} empty="Нет партий без VIN" /></Panel>
  </Modal>;
}

interface VehicleDetail { vehicle: Vehicle; specification: Model['specification']; history: { type: string; occurredAt: string; warehouseId: string | null; reason: string }[] }

const factLabel: Record<string, string> = {
  'vehicle.received': 'Принят на склад', 'vehicle.identified': 'VIN введён', 'vehicle.moved': 'Перемещён',
  'vehicle.handed_over': 'Получен от поставщика', 'vehicle.delivered_to_customer': 'Выдан клиенту',
};

export function VehiclesPage() {
  const [tab, setTab] = useState<'vehicles' | 'models'>('vehicles');
  const [placement, setPlacement] = useState('any');
  const q = useVehicles(placement);
  const warehouses = useWarehouses();
  const name = useModelName();
  const [open, setOpen] = useState<string | null>(null);
  const wh = (id?: string) => warehouses.data?.find((w) => w.id === id)?.name ?? '—';
  return <Page title="Автомобили" subtitle="Один VIN — одна машина; статусы меняются только фактами">
    <Tabs value={tab} onChange={setTab} tabs={[['vehicles', 'Автомобили'], ['models', 'Каталог моделей']]} />
    {tab === 'vehicles' ? <>
      <Tabs value={placement} onChange={setPlacement} tabs={[['any', 'Все'], ['warehouse', 'На складе'], ['outside', 'Вне склада']]} />
      <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(v) => v.id} onRowClick={(v) => setOpen(v.id)} columns={[
        { title: 'VIN', render: (v) => <code>{v.vin}</code> }, { title: 'Модель', render: (v) => name(v.modelId) },
        { title: 'Склад', render: (v) => wh(v.placement?.warehouseId) },
      ]} /></Panel>
    </> : <ModelsPanel />}
    {open && <VehicleDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

function VehicleDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['vehicle', id], () => get<VehicleDetail>(`/inventory/vehicle-units/${id}`));
  const warehouses = useWarehouses();
  const d = q.data?.data;
  const wh = (x?: string | null) => warehouses.data?.find((w) => w.id === x)?.name ?? '—';
  return <Modal title={d ? `VIN ${d.vehicle.vin}` : 'Автомобиль'} onClose={onClose} size="wide" footer={d?.vehicle.placement &&
    <ActionButton label="Переместить" refresh={[['vehicle', id], ['vehicles'], ['warehouses']]} fields={[
      { name: 'toWarehouseId', label: 'На склад', type: 'select', required: true,
        options: (warehouses.data ?? []).filter((w) => w.id !== d.vehicle.placement!.warehouseId).map((w) => [w.id, `${w.name} (свободно ${w.free})`]) },
      { name: 'occurredAt', label: 'Когда', type: 'datetime', required: true },
    ]} onSubmit={(v) => post(`/inventory/vehicle-units/${id}/warehouse-moves`, { fromWarehouseId: d.vehicle.placement!.warehouseId, ...v })} />}>
    {d && <>
      <Details items={[
        ['Модель', `${d.specification.make} ${d.specification.model} ${d.specification.variant} (${d.specification.year})`],
        ['Кузов / цвет', `${d.specification.bodyType}, ${d.specification.exteriorColor} / ${d.specification.interiorColor}`],
        ['Двигатель / привод', `${d.specification.powertrain}, ${d.specification.drivetrain}`],
        ['Склад', wh(d.vehicle.placement?.warehouseId)],
      ]} />
      <Panel title="История" padded><ul className="kit-timeline">{d.history.map((h, i) =>
        <li key={i}>{dateTime(h.occurredAt)} — {factLabel[h.type] ?? h.type}{h.warehouseId ? ` · ${wh(h.warehouseId)}` : ''}{h.reason ? ` · ${h.reason}` : ''}</li>)}</ul></Panel>
    </>}
  </Modal>;
}

const specFields = (s?: Model['specification']): FieldSpec[] => [
  { name: 'make', label: 'Марка', type: 'text', required: true, initial: s?.make ?? '' },
  { name: 'model', label: 'Модель', type: 'text', required: true, initial: s?.model ?? '' },
  { name: 'variant', label: 'Комплектация', type: 'text', required: true, initial: s?.variant ?? '' },
  { name: 'year', label: 'Год', type: 'number', required: true, initial: s ? String(s.year) : '' },
  { name: 'bodyType', label: 'Кузов', type: 'text', required: true, initial: s?.bodyType ?? '' },
  { name: 'exteriorColor', label: 'Цвет кузова', type: 'text', required: true, initial: s?.exteriorColor ?? '' },
  { name: 'interiorColor', label: 'Цвет салона', type: 'text', required: true, initial: s?.interiorColor ?? '' },
  { name: 'powertrain', label: 'Двигатель', type: 'text', required: true, initial: s?.powertrain ?? '' },
  { name: 'drivetrain', label: 'Привод', type: 'text', required: true, initial: s?.drivetrain ?? '' },
];
const spec = (v: Record<string, unknown>) => ({ specification: { ...v, year: Number(v.year) } });

function ModelsPanel() {
  const q = useModels();
  return <Panel title="Модели" actions={<ActionButton label="Добавить модель" variant="primary" fields={specFields()} refresh={[['models']]}
    onSubmit={(v) => post('/inventory/vehicle-models', spec(v))} />}>
    <Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(m) => m.id} columns={[
      { title: 'Модель', render: (m) => <b>{modelName(m)}</b> }, { title: 'Год', render: (m) => m.specification.year },
      { title: 'Версия', render: (m) => m.specification.version },
      { title: '', render: (m) => <ActionButton label="Новая версия" fields={specFields(m.specification)} refresh={[['models']]}
        onSubmit={(v) => post(`/inventory/vehicle-models/${m.id}/specification-versions`, spec(v), { ifMatch: m.revision })} /> },
    ]} />
  </Panel>;
}

export function DashboardPage() {
  const w = useWarehouses();
  const v = useVehicles();
  const deals = useData(['deals', 'reserved'], () => list<{ id: string }>('/retail/deals?status=reserved'));
  const leads = useData(['leads'], () => list<{ id: string; stage: string }>('/retail/leads?limit=100'));
  const orders = useData(['orders'], () => list<{ id: string; status: string }>('/commerce/orders?limit=100'));
  const sum = (k: 'capacity' | 'occupied') => (w.data ?? []).reduce((n, x) => n + Number(x[k]), 0);
  return <Page title="Дашборд">
    <div className="kit-grid">
      <Stat label="Складов" value={w.data?.length ?? '…'} />
      <Stat label="Занято мест" value={w.data ? `${sum('occupied')} / ${sum('capacity')}` : '…'} />
      <Stat label="Автомобилей" value={v.data?.length ?? '…'} />
      <Stat label="Активных продаж" value={deals.data?.length ?? '…'} />
      <Stat label="Открытых лидов" value={leads.data?.filter((l) => !['won', 'lost'].includes(l.stage)).length ?? '…'} />
      <Stat label="Заказов в работе" value={orders.data?.filter((o) => ['awaiting-supplier', 'accepted', 'fulfilling'].includes(o.status)).length ?? '…'} />
    </div>
  </Page>;
}
