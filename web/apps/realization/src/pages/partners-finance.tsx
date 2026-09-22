import { useState } from 'react';
import {
  ActionButton, Button, Cell, FilterSelect, FinanceDialog, FinanceTable, InsuranceDialog, Page, Panel, ResultMeta, StatusBadge, Table, Tabs,
  Toolbar, applicationStatusLabel, list, matches, money, percent, plural, post, useCompanyNames, useData, useFinanceApplications,
  useInsuranceApplications, useSearchQuery,
} from '@justixauto/kit';
import type { Program } from '@justixauto/kit';
import { useDeals } from '../data';

function useDealLabel() {
  const deals = useDeals();
  return (id: string) => {
    const d = deals.data?.find((x) => x.id === id);
    return d ? `${d.customer.displayName} · ${money(d.price)}` : id.slice(0, 8);
  };
}

export function InsurancePage() {
  const q = useInsuranceApplications();
  const insurers = useData(['directory', 'insurance'], () => list<{ id: string; name: string }>('/identity/directory/companies?kind=insurance&limit=100'));
  const deals = useDeals();
  const name = useCompanyNames();
  const dealLabel = useDealLabel();
  const [query, setQuery] = useSearchQuery();
  const [status, setStatus] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const eligible = (deals.data ?? []).filter((d) => d.status === 'reserved' && d.paymentScheme === 'own-installment');
  const rows = (q.data ?? []).filter((a) => (!status || a.status === status) &&
    matches(query, a.snapshot?.customer?.name, a.snapshot?.vehicle?.vin, a.snapshot?.vehicle?.model, dealLabel(a.retailDealId), name(a.insurerCompanyId)));
  return <Page title="Страхование рассрочки" subtitle="Заявки и решения страховых компаний по продажам в собственную рассрочку"
    actions={<ActionButton label="Создать заявку" variant="primary" refresh={[['insurance-applications']]} fields={[
      { name: 'retailDealId', label: 'Продажа в собственную рассрочку', type: 'select', required: true, options: eligible.map((d) => [d.id, dealLabel(d.id)]) },
      { name: 'insurerCompanyId', label: 'Страховая компания', type: 'select', required: true, options: (insurers.data ?? []).map((c) => [c.id, c.name]) },
      { name: 'note', label: 'Комментарий для страховой', type: 'textarea' },
    ]} intro={<p>Черновик виден только вам; отправка — отдельное действие со снимком данных продажи.</p>}
      onSubmit={(v) => post('/insurance/applications', { ...v, note: v.note ?? '' })} />}>
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Клиент, VIN или страховая" onReset={() => setStatus('')}>
        <FilterSelect value={status} onChange={setStatus} all="Все статусы" options={Object.entries(applicationStatusLabel).filter(([k]) => !['terms', 'agreed'].includes(k))} />
      </Toolbar>
      <ResultMeta>{plural(rows.length, ['заявка', 'заявки', 'заявок'])}</ResultMeta>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(a) => a.id} onRowClick={(a) => setOpen(a.id)} empty="Заявок на страхование пока нет" columns={[
        { title: 'Заявка / продажа', render: (a) => <Cell main={dealLabel(a.retailDealId)} sub={a.submittedAt ? `Отправлена ${new Date(a.submittedAt).toLocaleDateString('ru-RU')}` : 'Черновик'} /> },
        { title: 'Клиент / автомобиль', render: (a) => <Cell main={a.snapshot?.customer?.name ?? '—'} sub={a.snapshot?.vehicle?.model} /> },
        { title: 'Страховая', render: (a) => name(a.insurerCompanyId) },
        { title: 'Цена авто', render: (a) => money(a.snapshot?.price) },
        { title: 'Статус', render: (a) => <StatusBadge status={a.status} /> },
        { title: 'Действие', render: (a) => <Button size="sm" onClick={() => setOpen(a.id)}>Открыть</Button> },
      ]} />
    </Panel>
    {open && <InsuranceDialog id={open} onClose={() => setOpen(null)} describeDeal={dealLabel} />}
  </Page>;
}

/** Seller's applications to banks and MFOs (Продажи → Клиентам → Банк / МФО). */
export function FinancingPanel() {
  const [tab, setTab] = useState<'applications' | 'programs'>('applications');
  const q = useFinanceApplications();
  const programs = useData(['programs', 'published'], () => list<Program>('/financing/programs?limit=100'));
  const deals = useDeals();
  const name = useCompanyNames();
  const dealLabel = useDealLabel();
  const [open, setOpen] = useState<string | null>(null);
  const eligible = (deals.data ?? []).filter((d) => d.status === 'reserved' && d.paymentScheme === 'partner-finance');
  const published = (programs.data ?? []).filter((p) => p.publishedVersion !== null);
  const current = (p: Program) => p.versions.find((v) => Number(v.number) === p.publishedVersion);
  return <>
    <Tabs value={tab} onChange={setTab} tabs={[['applications', 'Заявки', q.data?.length], ['programs', 'Программы партнёров', published.length]]}
      actions={<ActionButton label="Заявка в банк / МФО" variant="primary" refresh={[['finance-applications']]} fields={[
        { name: 'retailDealId', label: 'Продажа (банк / МФО)', type: 'select', required: true, options: eligible.map((d) => [d.id, dealLabel(d.id)]) },
        { name: 'programId', label: 'Программа', type: 'select', required: true, options: published.map((p) => [p.id, `${p.provider.name} — ${current(p)?.name ?? ''}`]) },
        { name: 'downPayment', label: 'Первый взнос', type: 'money', required: true },
        { name: 'termMonths', label: 'Срок, месяцев', type: 'number', required: true },
        { name: 'firstDueDate', label: 'Дата первого платежа', type: 'date', required: true },
      ]} onSubmit={(v) => {
        const p = published.find((x) => x.id === v.programId)!;
        return post('/financing/applications', { retailDealId: v.retailDealId, providerCompanyId: p.provider.id, programId: p.id, programVersion: p.publishedVersion,
          calculationInputs: { downPayment: v.downPayment, termMonths: Number(v.termMonths), firstDueDate: v.firstDueDate } });
      }} />} />
    {tab === 'applications'
      ? <FinanceTable rows={q.data} loading={q.isLoading} error={q.error} counterparty={(a) => `${name(a.providerCompanyId)} · ${dealLabel(a.retailDealId)}`} onOpen={setOpen} />
      : <Table rows={published} loading={programs.isLoading} error={programs.error} rowKey={(p) => p.id} empty="Банки и МФО пока не опубликовали программ" columns={[
        { title: 'Банк / МФО', render: (p) => <Cell main={p.provider.name} sub={p.provider.kind === 'bank' ? 'Банк' : 'МФО'} /> },
        { title: 'Программа', render: (p) => current(p)?.name },
        { title: 'Наценка', render: (p) => current(p) && percent(current(p)!.terms.markupBps) },
        { title: 'Мин. взнос', render: (p) => current(p) && percent(current(p)!.terms.minDownPaymentBps) },
        { title: 'Сроки', render: (p) => `${current(p)?.terms.termMonths.join(', ')} мес.` },
        { title: 'Валюта', render: (p) => current(p)?.currency },
      ]} />}
    {open && <FinanceDialog id={open} onClose={() => setOpen(null)} describeDeal={dealLabel} />}
  </>;
}
