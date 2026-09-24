import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  Badge,
  Button,
  Cell,
  FilterSelect,
  Modal,
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
  useSearchQuery,
  useSession,
} from '@justixauto/kit';
import { TermsButton } from '../shared';
import { orderLabel, orderTone, rfqLabel, useLinesLabel, useOffers, useOrders, usePartners, useRFQs } from '../data';
import { OfferDialog, OrderDialog, RFQDialog } from './trade';

interface Row {
  id: string;
  kind: 'rfq' | 'order';
  code: string;
  counterparty: string;
  lines: { modelId: string; quantity: string }[];
  stage: string;
  label: string;
  tone: 'success' | 'warning' | 'danger' | 'info' | undefined;
  updatedAt: string;
  needsMe: boolean;
  total?: string;
}

const rfqTone = (s: string) =>
  s === 'negotiating'
    ? 'warning'
    : s === 'accepted'
      ? 'success'
      : s === 'declined' || s === 'cancelled'
        ? 'danger'
        : 'info';
const rfqStage: Record<string, string> = { ...rfqLabel, negotiating: 'Получены условия', sent: 'Ждём поставщика' };

/** Buyer side of wholesale: quotation requests and orders to suppliers. */
export function PurchasesPage() {
  const rfqs = useRFQs();
  const orders = useOrders();
  const lines = useLinesLabel();
  const partners = usePartners();
  const s = useSession();
  const [params, setParams] = useSearchParams();
  const [tab, setTab] = useState<'all' | 'rfq' | 'order'>('all');
  const [query, setQuery] = useSearchQuery();
  const [stage, setStage] = useState('');
  const [open, setOpen] = useState<Row | null>(null);
  const catalog = params.get('catalog') === '1';
  const setCatalog = (on: boolean) => setParams(on ? { catalog: '1' } : {}, { replace: true });
  const all: Row[] = [
    ...(rfqs.data ?? [])
      .filter((r) => r.buyer.id === s.company?.id)
      .map((r): Row => ({
        id: r.id,
        kind: 'rfq',
        code: `RFQ · ${r.supplier.name}`,
        counterparty: r.supplier.name,
        lines: r.lines,
        stage: r.status,
        label: rfqStage[r.status] ?? r.status,
        tone: rfqTone(r.status),
        updatedAt: r.updatedAt,
        needsMe: r.status === 'negotiating',
      })),
    ...(orders.data ?? [])
      .filter((o) => o.party === 'buyer')
      .map((o): Row => ({
        id: o.id,
        kind: 'order',
        code: `Заказ · ${o.supplier.name}`,
        counterparty: o.supplier.name,
        lines: o.terms.lines,
        stage: o.status,
        label: orderLabel[o.status] ?? o.status,
        tone: orderTone(o.status),
        updatedAt: o.updatedAt,
        total: money(o.total),
        needsMe: o.shipments.some((s) => s.status !== 'received') || o.addenda.some((a) => a.status === 'proposed'),
      })),
  ].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  const rows = all.filter(
    (r) =>
      (tab === 'all' || r.kind === tab) &&
      (!stage || r.label === stage) &&
      matches(query, r.counterparty, lines(r.lines)),
  );
  const qty = rows.reduce((n, r) => n + r.lines.reduce((m, l) => m + Number(l.quantity), 0), 0);
  const suppliers = (partners.data ?? [])
    .filter((p) => p.status === 'active')
    .map((p): [string, string] => [p.counterparty.id, p.counterparty.name]);
  return (
    <Page title="Закупки" subtitle="Запрос условий становится заказом после принятия коммерческих условий">
      <div>
        <Button onClick={() => setCatalog(true)}>Предложения поставщиков</Button>
      </div>
      <Stats>
        <Stat
          label="Нужен ваш ответ"
          value={all.filter((r) => r.needsMe).length}
          note="получены условия или поставка"
        />
        <Stat
          label="Ждём поставщика"
          value={all.filter((r) => r.stage === 'sent' || r.stage === 'awaiting-supplier').length}
          note="запрос или заказ в работе"
        />
        <Stat
          label="Заказы в исполнении"
          value={all.filter((r) => r.kind === 'order' && ['accepted', 'fulfilling'].includes(r.stage)).length}
          note="назначение VIN, путь и приёмка"
        />
        <Stat label="Общий объём" value={`${qty} авто`} note="по текущему фильтру" />
      </Stats>
      <Panel>
        <Tabs
          value={tab}
          onChange={setTab}
          tabs={[
            ['all', 'Все', all.length],
            ['rfq', 'Запросы', all.filter((r) => r.kind === 'rfq').length],
            ['order', 'Заказы', all.filter((r) => r.kind === 'order').length],
          ]}
          actions={
            <>
              <TermsButton
                label="Создать запрос"
                withPrices={false}
                refresh={[['rfqs']]}
                extra={[
                  { name: 'supplier', label: 'Поставщик (активный партнёр)', required: true, options: suppliers },
                ]}
                onSubmit={(terms, x) =>
                  post('/commerce/rfqs', {
                    supplierCompanyId: x.supplier,
                    lines: terms.lines.map((l) => ({ modelId: l.modelId, quantity: l.quantity })),
                  })
                }
              />
              <Button variant="primary" size="sm" onClick={() => setCatalog(true)}>
                Создать заказ
              </Button>
            </>
          }
        />
        <Toolbar query={query} onQuery={setQuery} placeholder="Автомобиль или поставщик" onReset={() => setStage('')}>
          <FilterSelect
            value={stage}
            onChange={setStage}
            all="Все этапы"
            options={[...new Set(all.map((r) => r.label))].map((l) => [l, l])}
          />
        </Toolbar>
        <ResultMeta>
          {plural(rows.length, ['запись', 'записи', 'записей'])} · запрос и созданный из него заказ связаны одной
          историей
        </ResultMeta>
        <Table
          rows={rows}
          loading={rfqs.isLoading || orders.isLoading}
          error={rfqs.error ?? orders.error}
          rowKey={(r) => r.id}
          onRowClick={setOpen}
          empty="Закупок пока нет — откройте предложения поставщиков"
          columns={[
            {
              title: 'Закупка',
              render: (r) => (
                <Cell
                  main={r.kind === 'rfq' ? 'Запрос условий' : 'Заказ'}
                  sub={
                    r.total ??
                    plural(
                      r.lines.reduce((n, l) => n + Number(l.quantity), 0),
                      ['авто', 'авто', 'авто'],
                    )
                  }
                />
              ),
            },
            { title: 'Поставщик', render: (r) => r.counterparty },
            { title: 'Автомобиль', render: (r) => lines(r.lines) },
            { title: 'Этап', render: (r) => <Badge tone={r.tone}>{r.label}</Badge> },
            { title: 'Обновлено', render: (r) => date(r.updatedAt) },
            {
              title: 'Действие',
              render: (r) => (
                <Button size="sm" variant={r.needsMe ? 'primary' : 'secondary'} onClick={() => setOpen(r)}>
                  {r.needsMe ? 'Принять решение' : 'Открыть'}
                </Button>
              ),
            },
          ]}
        />
      </Panel>
      {open?.kind === 'rfq' && <RFQDialog id={open.id} onClose={() => setOpen(null)} />}
      {open?.kind === 'order' && <OrderDialog id={open.id} onClose={() => setOpen(null)} />}
      {catalog && <SupplierCatalog onClose={() => setCatalog(false)} />}
    </Page>
  );
}

/** Published offers of active partners; ordering happens from the offer. */
function SupplierCatalog({ onClose }: { onClose: () => void }) {
  const q = useOffers('available');
  const lines = useLinesLabel();
  const [open, setOpen] = useState<string | null>(null);
  return (
    <Modal
      title="Предложения поставщиков"
      icon="tag"
      size="wide"
      onClose={onClose}
      help="Опубликованные предложения активных партнёров. Заказ ждёт подтверждения поставщика и не резервирует VIN сразу."
    >
      <Table
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(o) => o.id}
        onRowClick={(o) => setOpen(o.id)}
        empty="Партнёры пока ничего не опубликовали"
        columns={[
          {
            title: 'Поставщик',
            render: (o) => (
              <Cell
                main={o.supplier.name}
                sub={o.publishedVersion ? `Версия №${o.publishedVersion.number}` : undefined}
              />
            ),
          },
          { title: 'Автомобили', render: (o) => (o.publishedVersion ? lines(o.publishedVersion.terms.lines) : '—') },
          { title: 'Итого', render: (o) => money(o.publishedVersion?.total) },
          {
            title: '',
            render: (o) => (
              <Button size="sm" onClick={() => setOpen(o.id)}>
                Открыть
              </Button>
            ),
          },
        ]}
      />
      {open && <OfferDialog id={open} onClose={() => setOpen(null)} />}
    </Modal>
  );
}
