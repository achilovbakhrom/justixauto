import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ActionButton, Badge, Button, Details, FinanceDialog, FinanceTable, Modal, Page, Panel, ResultMeta, Stat, Stats, Table, Tabs, Toolbar, date, get,
  list, matches, percent, plural, post, useCompanyNames, useData, useFinanceApplications, useSearchQuery, useSession,
} from '@justixauto/kit';
import type { FinanceApplication, Program } from '@justixauto/kit';
import { programFields, programInput, programLabel } from './programs';

const useOwnPrograms = () => useData(['programs', 'own'], () => list<Program>('/financing/programs?limit=100'));

/** Program name of an application ("Авто 10 · версия 2"). */
function useProgramName() {
  const programs = useOwnPrograms();
  return (a: FinanceApplication) => {
    const p = programs.data?.find((x) => x.id === a.programId);
    const v = p?.versions.find((x) => Number(x.number) === a.programVersion);
    return v ? `${v.name} · версия ${v.number}` : '—';
  };
}

const statusTabs: [string, string][] = [['submitted', 'Новая заявка'], ['review', 'На рассмотрении'], ['needs-info', 'Нужны сведения'],
  ['terms', 'Условия отправлены'], ['agreed', 'Условия согласованы'], ['declined', 'Отказ']];

export function OverviewPage() {
  const q = useFinanceApplications();
  const name = useCompanyNames();
  const program = useProgramName();
  const navigate = useNavigate();
  const s = useSession();
  const [open, setOpen] = useState<string | null>(null);
  const all = q.data ?? [];
  const n = (st: string) => all.filter((a) => a.status === st).length;
  const attention = all.filter((a) => ['submitted', 'review'].includes(a.status));
  return <Page title="Обзор" subtitle={`${s.company?.name ?? ''} · работа с продавцами`} actions={<Button onClick={() => navigate('/applications')}>Все заявки</Button>}>
    <Stats>
      <Stat label="Новые" value={n('submitted')} note="ждут, пока их возьмут в работу" />
      <Stat label="На рассмотрении" value={n('review')} note="решение за вами" />
      <Stat label="Ожидаем сведения" value={n('needs-info')} note="ответ продавца" />
      <Stat label="Условия согласованы" value={n('agreed')} note="обмен документами" />
    </Stats>
    <Panel title="Требуют внимания">
      <FinanceTable rows={attention} loading={q.isLoading} error={q.error} counterparty={(a) => name(a.sellerCompanyId)} programName={program} onOpen={setOpen} />
    </Panel>
    <Panel title="Начать работу" padded><div className="kit-row">
      <Button onClick={() => navigate('/programs')}>Программы финансирования</Button>
      <Button variant="primary" onClick={() => navigate('/applications')}>Открыть заявки</Button>
    </div></Panel>
    {open && <FinanceDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

export function ApplicationsPage() {
  const q = useFinanceApplications();
  const name = useCompanyNames();
  const program = useProgramName();
  const [tab, setTab] = useState('all');
  const [query, setQuery] = useSearchQuery();
  const [open, setOpen] = useState<string | null>(null);
  const all = q.data ?? [];
  const rows = all.filter((a) => (tab === 'all' || a.status === tab)
    && matches(query, name(a.sellerCompanyId), a.snapshot?.customer?.name, a.snapshot?.vehicle?.vin, a.snapshot?.vehicle?.model));
  return <Page title="Заявки" subtitle="Заявки продавцов на финансирование автомобилей">
    <Panel>
      <Tabs value={tab} onChange={setTab} tabs={[['all', 'Все', all.length], ...statusTabs.map(([k, l]): [string, string, number] => [k, l, all.filter((a) => a.status === k).length])]} />
      <Toolbar query={query} onQuery={setQuery} placeholder="Клиент, VIN или продавец" />
      <ResultMeta>{plural(rows.length, ['заявка', 'заявки', 'заявок'])} · черновики продавцов вам не видны</ResultMeta>
      <FinanceTable rows={rows} loading={q.isLoading} error={q.error} counterparty={(a) => name(a.sellerCompanyId)} programName={program} onOpen={setOpen} />
    </Panel>
    {open && <FinanceDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

export function ProgramsPage() {
  const s = useSession();
  const q = useOwnPrograms();
  const [open, setOpen] = useState<string | null>(null);
  const manage = s.can('financing.programs.manage');
  const latest = (p: Program) => p.versions[p.versions.length - 1];
  const shown = (p: Program) => p.versions.find((v) => Number(v.number) === p.publishedVersion) ?? latest(p);
  return <Page title="Программы финансирования" subtitle="Условия, доступные продавцам"
    actions={manage && <ActionButton label="Создать программу" variant="primary" fields={programFields()} refresh={[['programs']]}
      intro={<p>Расчёт: фиксированная наценка на цену автомобиля, равные ежемесячные платежи. Политика расчёта ожидает утверждения бизнесом.</p>}
      onSubmit={(v) => post('/financing/programs', programInput(v))} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(p) => p.id} onRowClick={(p) => setOpen(p.id)} empty="Программ пока нет" columns={[
      { title: 'Программа', render: (p) => <div className="cell-main">{shown(p)?.name}</div> },
      { title: 'Взнос от', render: (p) => shown(p) && percent(shown(p)!.terms.minDownPaymentBps) },
      { title: 'Сроки', render: (p) => shown(p)?.terms.termMonths.map((m) => `${m} мес.`).join(' / ') },
      { title: 'Наценка', render: (p) => shown(p) && percent(shown(p)!.terms.markupBps) },
      { title: 'Версия', render: (p) => p.publishedVersion ?? `${latest(p)?.number} (черновик)` },
      { title: 'Статус', render: (p) => <Badge tone={p.status === 'published' ? 'success' : p.status === 'withdrawn' ? 'danger' : undefined}>{programLabel[p.status] ?? p.status}</Badge> },
      { title: 'Действия', render: (p) => <Button size="sm" onClick={() => setOpen(p.id)}>Открыть</Button> },
    ]} /></Panel>
    <div className="kit-notice" data-kind="info">Публикация делает программу доступной продавцам. Новая версия не меняет уже отправленные заявки: они сохраняют свои условия.</div>
    {open && <ProgramDialog id={open} manage={manage} onClose={() => setOpen(null)} />}
  </Page>;
}

export function PartnersPage() {
  const q = useFinanceApplications();
  const name = useCompanyNames();
  const sellers = [...new Set((q.data ?? []).map((a) => a.sellerCompanyId))];
  return <Page title="Партнёры" subtitle="Продавцы, которые отправляли заявки по вашим опубликованным программам">
    <Panel title="Продавцы"><Table rows={sellers} loading={q.isLoading} error={q.error} rowKey={(id) => id} empty="Заявок от продавцов пока не было" columns={[
      { title: 'Компания', render: (id) => name(id) },
      { title: 'Доступ', render: () => <Badge tone="success">Программы и заявки</Badge> },
      { title: 'Заявки', render: (id) => (q.data ?? []).filter((a) => a.sellerCompanyId === id).length },
      { title: 'Согласовано', render: (id) => (q.data ?? []).filter((a) => a.sellerCompanyId === id && a.status === 'agreed').length },
    ]} /></Panel>
  </Page>;
}

function ProgramDialog({ id, manage, onClose }: { id: string; manage: boolean; onClose: () => void }) {
  const q = useData(['program', id], () => get<Program>(`/financing/programs/${id}`));
  const p = q.data?.data;
  const refresh = [['program', id], ['programs']];
  const latest = p?.versions[p.versions.length - 1];
  return <Modal title={latest?.name ?? 'Программа'} onClose={onClose} size="wide" footer={p && manage && p.status !== 'withdrawn' && <>
    <ActionButton label="Новая версия" fields={programFields(latest)} refresh={refresh} onSubmit={(v) => post(`/financing/programs/${id}/versions`, programInput(v), { ifMatch: q.data!.revision })} />
    {latest && Number(latest.number) !== p.publishedVersion && <ActionButton label={`Опубликовать версию ${latest.number}`} variant="primary" refresh={refresh}
      onSubmit={() => post(`/financing/programs/${id}/publish`, { programVersion: Number(latest.number) }, { ifMatch: q.data!.revision })} />}
    <ActionButton label="Снять с публикации" variant="danger" refresh={refresh} fields={[{ name: 'reason', label: 'Причина', type: 'textarea', required: true }]}
      onSubmit={(v) => post(`/financing/programs/${id}/withdraw`, v, { ifMatch: q.data!.revision })} />
  </>}>
    {p && <>
      <Details items={[['Статус', programLabel[p.status] ?? p.status], ['Опубликована', p.publishedVersion ? `версия ${p.publishedVersion}` : '—'], ['Причина', p.statusReason || '—']]} />
      <Table rows={[...p.versions].reverse()} rowKey={(v) => v.number} columns={[
        { title: 'Версия', render: (v) => v.number }, { title: 'Название', render: (v) => v.name },
        { title: 'Наценка', render: (v) => percent(v.terms.markupBps) }, { title: 'Мин. взнос', render: (v) => percent(v.terms.minDownPaymentBps) },
        { title: 'Сроки', render: (v) => `${v.terms.termMonths.join(', ')} мес.` }, { title: 'Валюта', render: (v) => v.currency },
        { title: 'Создана', render: (v) => date(v.createdAt) },
      ]} />
    </>}
  </Modal>;
}
