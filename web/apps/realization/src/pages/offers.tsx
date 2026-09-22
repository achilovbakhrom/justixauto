import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  ActionButton, Badge, Button, Cell, Details, Modal, Page, Panel, ResultMeta, Stat, Stats, Table, Tabs, Toolbar, get, matches, minorToMajor, money,
  patch, plural, post, useData, useSearchQuery,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { TermsButton } from '../shared';
import { listingLabel, offerLabel, useLinesLabel, useListings, useModels, useOffers, useOrders, usePartners, useVehicles, useWarehouses } from '../data';
import type { Listing } from '../data';
import { OfferDialog } from './trade';

type Filter = 'all' | 'published' | 'draft';

export function OffersPage() {
  const [params, setParams] = useSearchParams();
  const channel = params.get('channel') === 'partners' ? 'partners' : 'clients';
  const setChannel = (c: 'clients' | 'partners') => setParams(c === 'partners' ? { channel: c } : {}, { replace: true });
  const vehicles = useVehicles('warehouse');
  const partners = usePartners();
  const active = (partners.data ?? []).filter((p) => p.status === 'active');
  const vehicleLabel = useVehicleLabel();
  return <Page title="Предложения" subtitle="Ваши предложения для покупателей и компаний-партнёров"
    actions={channel === 'clients'
      ? <ActionButton label="Новое предложение" variant="primary" refresh={[['listings']]} fields={listingFields(undefined,
        (vehicles.data ?? []).filter((v) => !v.reserved).map((v) => [v.id, vehicleLabel(v.id)]))}
        onSubmit={(v) => post('/retail/listings', v)} />
      : <TermsButton label="Новое предложение" variant="primary" refresh={[['offers']]}
        extra={[{ name: 'partners', label: 'Только для выбранных партнёров (ничего не выбрано — всем активным)', multiple: true,
          options: active.map((p) => [p.counterparty.id, p.counterparty.name]) }]}
        onSubmit={(terms, x) => {
          const ids = (x.partners as string[] | undefined) ?? [];
          return post('/commerce/offers', { terms, audience: ids.length ? { mode: 'selected', partnerCompanyIds: ids } : { mode: 'all-active', partnerCompanyIds: [] } });
        }} />}>
    <Tabs channel value={channel} onChange={setChannel} tabs={[['clients', 'Клиентам'], ['partners', 'Партнёрам']]} />
    {channel === 'clients' ? <ClientOffers /> : <PartnerOffers />}
  </Page>;
}

function useVehicleLabel() {
  const vehicles = useVehicles();
  const models = useModels();
  return (id: string) => {
    const v = vehicles.data?.find((x) => x.id === id);
    const m = models.data?.find((x) => x.id === v?.modelId)?.specification;
    return v ? `${m ? `${m.make} ${m.model} ${m.variant}` : ''} · ${v.vin}` : '—';
  };
}

function listingFields(l: Listing | undefined, vehicles: [string, string][]): FieldSpec[] {
  return [
    ...(l ? [] : [{ name: 'vehicleId', label: 'Автомобиль на складе', type: 'select', required: true, options: vehicles } as FieldSpec]),
    { name: 'askingPrice', label: 'Цена', type: 'money', required: true, ...(l ? { initial: minorToMajor(l.askingPrice.amountMinor), currency: l.askingPrice.currency } : {}) },
    { name: 'text', label: 'Описание для покупателя', type: 'textarea', required: true, ...(l ? { initial: l.text } : {}) },
  ];
}

function ClientOffers() {
  const q = useListings();
  const vehicles = useVehicles();
  const models = useModels();
  const warehouses = useWarehouses();
  const [tab, setTab] = useState<Filter>('all');
  const [query, setQuery] = useSearchQuery();
  const [open, setOpen] = useState<string | null>(null);
  const all = (q.data ?? []).filter((l) => l.status !== 'withdrawn' || tab === 'all');
  const car = (l: Listing) => {
    const v = vehicles.data?.find((x) => x.id === l.vehicleId);
    return { v, m: models.data?.find((x) => x.id === v?.modelId)?.specification };
  };
  const rows = all.filter((l) => (tab === 'all' || l.status === tab) && matches(query, car(l).v?.vin, car(l).m?.make, car(l).m?.model, l.text));
  const n = (s: string) => (q.data ?? []).filter((l) => l.status === s).length;
  return <>
    <Stats columns={3}>
      <Stat label="Опубликовано" value={n('published')} note="видны покупателям" />
      <Stat label="Черновики" value={n('draft')} note="не опубликованы" />
      <Stat label="Сняты" value={n('withdrawn')} note="больше не показываются" />
    </Stats>
    <Panel>
      <Tabs value={tab} onChange={setTab} tabs={[['all', 'Все', (q.data ?? []).length], ['published', 'Опубликованы', n('published')], ['draft', 'Черновики', n('draft')]]} />
      <Toolbar query={query} onQuery={setQuery} placeholder="VIN или автомобиль" />
      <ResultMeta>{plural(rows.length, ['предложение', 'предложения', 'предложений'])}</ResultMeta>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(l) => l.id} onRowClick={(l) => setOpen(l.id)} empty="Предложений клиентам пока нет" columns={[
        { title: 'Предложение', render: (l) => { const { v, m } = car(l); return <Cell main={m ? `${m.make} ${m.model} ${m.variant}` : '—'} sub={v?.vin} />; } },
        { title: 'Склад', render: (l) => warehouses.data?.find((w) => w.id === car(l).v?.placement?.warehouseId)?.name ?? '—' },
        { title: 'Цена', render: (l) => money(l.askingPrice) },
        { title: 'Статус', render: (l) => <Badge tone={l.status === 'published' ? 'success' : undefined}>{listingLabel[l.status]}</Badge> },
        { title: 'Действие', render: (l) => <Button size="sm" onClick={() => setOpen(l.id)}>{l.status === 'draft' ? 'Редактировать' : 'Открыть'}</Button> },
      ]} />
    </Panel>
    {open && <ListingDialog id={open} onClose={() => setOpen(null)} />}
  </>;
}

function ListingDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['listing', id], () => get<Listing>(`/retail/listings/${id}`));
  const label = useVehicleLabel();
  const l = q.data?.data;
  const refresh = [['listing', id], ['listings']];
  return <Modal title="Предложение клиентам" icon="tag" onClose={onClose} footer={l && l.status !== 'withdrawn' && <>
    <ActionButton label="Изменить" fields={listingFields(l, [])} refresh={refresh} onSubmit={(v) => patch(`/retail/listings/${id}`, v, { ifMatch: l.revision })} />
    {l.status === 'draft'
      ? <ActionButton label="Опубликовать" variant="primary" refresh={refresh} onSubmit={() => post(`/retail/listings/${id}/publish`, {}, { ifMatch: l.revision })} />
      : <ActionButton label="Снять" variant="danger" refresh={refresh} onSubmit={() => post(`/retail/listings/${id}/withdraw`, {}, { ifMatch: l.revision })} />}
  </>}>
    {l && <Details items={[['Автомобиль', label(l.vehicleId)], ['Цена', money(l.askingPrice)], ['Статус', listingLabel[l.status]], ['Описание', l.text]]} />}
  </Modal>;
}

function PartnerOffers() {
  const q = useOffers('own');
  const orders = useOrders();
  const lines = useLinesLabel();
  const partners = usePartners();
  const [tab, setTab] = useState<Filter>('all');
  const [query, setQuery] = useSearchQuery();
  const [open, setOpen] = useState<string | null>(null);
  const latest = (o: NonNullable<typeof q.data>[number]) => o.versions?.[o.versions.length - 1];
  const rows = (q.data ?? []).filter((o) => (tab === 'all' || o.status === tab) && matches(query, lines(latest(o)?.terms.lines ?? [])));
  const n = (s: string) => (q.data ?? []).filter((o) => o.status === s).length;
  const partnerName = (id: string) => partners.data?.find((p) => p.counterparty.id === id)?.counterparty.name ?? id.slice(0, 8);
  return <>
    <Stats columns={3}>
      <Stat label="Опубликовано" value={n('published')} note="видны партнёрам" />
      <Stat label="Черновики" value={n('draft')} note="не опубликованы" />
      <Stat label="Заказы партнёров" value={(orders.data ?? []).filter((o) => o.party === 'supplier' && o.status !== 'cancelled').length} note="по вашим предложениям" />
    </Stats>
    <Panel>
      <Tabs value={tab} onChange={setTab} tabs={[['all', 'Все', (q.data ?? []).length], ['published', 'Опубликованы', n('published')], ['draft', 'Черновики', n('draft')]]} />
      <Toolbar query={query} onQuery={setQuery} placeholder="Автомобиль" />
      <ResultMeta>{plural(rows.length, ['предложение', 'предложения', 'предложений'])} · публикация не резервирует VIN</ResultMeta>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(o) => o.id} onRowClick={(o) => setOpen(o.id)} empty="Предложений партнёрам пока нет" columns={[
        { title: 'Предложение', render: (o) => <Cell main={lines(latest(o)?.terms.lines ?? [])} sub={`Версия №${latest(o)?.number ?? '—'}${o.publishedVersion ? ` · опубликована №${o.publishedVersion.number}` : ''}`} /> },
        { title: 'Для кого', render: (o) => { const a = latest(o)?.audience; return a?.mode === 'selected' ? a.partnerCompanyIds.map(partnerName).join(', ') : 'Все активные партнёры'; } },
        { title: 'Итого', render: (o) => money(latest(o)?.total) },
        { title: 'Статус', render: (o) => <Badge tone={o.status === 'published' ? 'success' : undefined}>{offerLabel[o.status]}</Badge> },
        { title: 'Действие', render: (o) => <Button size="sm" onClick={() => setOpen(o.id)}>Открыть</Button> },
      ]} />
    </Panel>
    {open && <OfferDialog id={open} onClose={() => setOpen(null)} />}
  </>;
}
