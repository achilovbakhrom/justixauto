import { useState } from 'react';
import {
  ActionButton, FinanceDialog, FinanceTable, InsuranceDialog, InsuranceTable, Page, Panel, Table, Tabs, list, money, percent, post,
  useCompanyNames, useData, useFinanceApplications, useInsuranceApplications,
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
  const [open, setOpen] = useState<string | null>(null);
  const eligible = (deals.data ?? []).filter((d) => d.status === 'reserved' && d.paymentScheme === 'own-installment');
  return <Page title="Страхование" subtitle="Одобрение страховой обязательно для продаж в собственную рассрочку"
    actions={<ActionButton label="Новая заявка" variant="primary" refresh={[['insurance-applications']]} fields={[
      { name: 'retailDealId', label: 'Сделка (собственная рассрочка)', type: 'select', required: true, options: eligible.map((d) => [d.id, dealLabel(d.id)]) },
      { name: 'insurerCompanyId', label: 'Страховая компания', type: 'select', required: true, options: (insurers.data ?? []).map((c) => [c.id, c.name]) },
      { name: 'note', label: 'Комментарий', type: 'textarea' },
    ]} onSubmit={(v) => post('/insurance/applications', { ...v, note: v.note ?? '' })} />}>
    <Panel><InsuranceTable rows={q.data} loading={q.isLoading} error={q.error} counterparty={(a) => `${name(a.insurerCompanyId)} · ${dealLabel(a.retailDealId)}`} onOpen={setOpen} /></Panel>
    {open && <InsuranceDialog id={open} onClose={() => setOpen(null)} describeDeal={dealLabel} />}
  </Page>;
}

export function FinancingPage() {
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
  return <Page title="Финансирование" subtitle="Заявки в банки и МФО по опубликованным программам"
    actions={<ActionButton label="Новая заявка" variant="primary" refresh={[['finance-applications']]} fields={[
      { name: 'retailDealId', label: 'Сделка (банк / МФО)', type: 'select', required: true, options: eligible.map((d) => [d.id, dealLabel(d.id)]) },
      { name: 'programId', label: 'Программа', type: 'select', required: true, options: published.map((p) => [p.id, `${p.provider.name} — ${current(p)?.name ?? ''}`]) },
      { name: 'downPayment', label: 'Первый взнос', type: 'money', required: true },
      { name: 'termMonths', label: 'Срок, месяцев', type: 'number', required: true },
      { name: 'firstDueDate', label: 'Дата первого платежа', type: 'date', required: true },
    ]} onSubmit={(v) => {
      const p = published.find((x) => x.id === v.programId)!;
      return post('/financing/applications', { retailDealId: v.retailDealId, providerCompanyId: p.provider.id, programId: p.id, programVersion: p.publishedVersion,
        calculationInputs: { downPayment: v.downPayment, termMonths: Number(v.termMonths), firstDueDate: v.firstDueDate } });
    }} />}>
    <Tabs value={tab} onChange={setTab} tabs={[['applications', 'Заявки'], ['programs', 'Программы партнёров']]} />
    {tab === 'applications'
      ? <Panel><FinanceTable rows={q.data} loading={q.isLoading} error={q.error} counterparty={(a) => `${name(a.providerCompanyId)} · ${dealLabel(a.retailDealId)}`} onOpen={setOpen} /></Panel>
      : <Panel><Table rows={published} loading={programs.isLoading} error={programs.error} rowKey={(p) => p.id} columns={[
        { title: 'Банк / МФО', render: (p) => p.provider.name },
        { title: 'Программа', render: (p) => current(p)?.name },
        { title: 'Наценка', render: (p) => current(p) && percent(current(p)!.terms.markupBps) },
        { title: 'Мин. взнос', render: (p) => current(p) && percent(current(p)!.terms.minDownPaymentBps) },
        { title: 'Сроки', render: (p) => current(p)?.terms.termMonths.join(', ') + ' мес.' },
        { title: 'Валюта', render: (p) => current(p)?.currency },
      ]} /></Panel>}
    {open && <FinanceDialog id={open} onClose={() => setOpen(null)} describeDeal={dealLabel} />}
  </Page>;
}
