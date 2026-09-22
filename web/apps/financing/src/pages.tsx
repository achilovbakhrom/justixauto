import { useState } from 'react';
import {
  ActionButton, Badge, Details, FinanceDialog, FinanceTable, Modal, Page, Panel, Table, Tabs, date, get, list, percent, post,
  useCompanyNames, useData, useFinanceApplications, useSession,
} from '@justixauto/kit';
import type { Program } from '@justixauto/kit';
import { programFields, programInput, programLabel } from './programs';

const queues: [string, string][] = [['open', 'В работе'], ['submitted', 'Новые'], ['terms', 'Ждут продавца'], ['agreed', 'Согласованы'], ['closed', 'Закрыты']];
const inQueue = (queue: string, status: string) => ({
  open: ['submitted', 'review', 'needs-info'], submitted: ['submitted'], terms: ['terms'], agreed: ['agreed'], closed: ['declined'],
} as Record<string, string[]>)[queue]?.includes(status) ?? false;

export function ApplicationsPage() {
  const q = useFinanceApplications();
  const name = useCompanyNames();
  const [queue, setQueue] = useState('open');
  const [open, setOpen] = useState<string | null>(null);
  return <Page title="Заявки на финансирование" subtitle="Заявки продавцов по вашим программам">
    <Tabs value={queue} onChange={setQueue} tabs={queues} />
    <Panel><FinanceTable rows={q.data?.filter((a) => inQueue(queue, a.status))} loading={q.isLoading} error={q.error}
      counterparty={(a) => name(a.sellerCompanyId)} onOpen={setOpen} /></Panel>
    {open && <FinanceDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}

export function ProgramsPage() {
  const s = useSession();
  const q = useData(['programs', 'own'], () => list<Program>('/financing/programs?limit=100'));
  const [open, setOpen] = useState<string | null>(null);
  const manage = s.can('financing.programs.manage');
  return <Page title="Программы" subtitle="Продавцы видят только опубликованную версию; новые версии не меняют поданные заявки"
    actions={manage && <ActionButton label="Новая программа" variant="primary" fields={programFields()} refresh={[['programs']]}
      intro={<p>Расчёт: фиксированная наценка на цену автомобиля, равные ежемесячные платежи. Политика расчёта ожидает утверждения бизнесом.</p>}
      onSubmit={(v) => post('/financing/programs', programInput(v))} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(p) => p.id} onRowClick={(p) => setOpen(p.id)} columns={[
      { title: 'Программа', render: (p) => p.versions[p.versions.length - 1]?.name },
      { title: 'Версий', render: (p) => p.versions.length },
      { title: 'Опубликована', render: (p) => p.publishedVersion ? `версия ${p.publishedVersion}` : '—' },
      { title: 'Статус', render: (p) => <Badge tone={p.status === 'published' ? 'success' : undefined}>{programLabel[p.status] ?? p.status}</Badge> },
    ]} /></Panel>
    {open && <ProgramDialog id={open} manage={manage} onClose={() => setOpen(null)} />}
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
