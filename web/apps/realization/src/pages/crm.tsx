import { useState } from 'react';
import {
  ActionButton, Badge, Button, Cell, FilterSelect, Page, Panel, ResultMeta, Stat, Stats, Table, Tabs, Toolbar, dateTime, matches, patch,
  plural, post, useSearchQuery, useSession,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { sourceLabel, stageLabel, useBranches, useCustomers, useLeads, useTasks } from '../data';
import type { Customer } from '../data';
import { LeadDialog } from './retail';

type Tab = 'leads' | 'customers' | 'tasks';
const customerFields = (c?: Customer): FieldSpec[] => [
  { name: 'displayName', label: 'Имя', type: 'text', required: true, ...(c ? { initial: c.displayName } : {}) },
  { name: 'phone', label: 'Телефон', type: 'text', ...(c ? { initial: c.phone } : {}) },
];
const stageTone = (s: string) => s === 'won' ? 'success' : s === 'lost' ? 'danger' : s === 'new' ? 'warning' : 'info';

export function CRMPage() {
  const leads = useLeads();
  const customers = useCustomers();
  const tasks = useTasks();
  const branches = useBranches();
  const s = useSession();
  const [tab, setTab] = useState<Tab>('leads');
  const [query, setQuery] = useSearchQuery();
  const [stage, setStage] = useState('');
  const [source, setSource] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const L = leads.data ?? [], C = customers.data ?? [], T = tasks.data ?? [];
  const today = new Date().toDateString();
  const openTasks = T.filter((t) => t.status === 'open');
  const closed = L.filter((l) => l.stage === 'won' || l.stage === 'lost');
  const customer = (id: string) => C.find((c) => c.id === id);
  const nextTask = (leadId: string, customerId: string) => openTasks.filter((t) => t.leadId === leadId || (!t.leadId && t.customerId === customerId))
    .sort((a, b) => a.dueAt.localeCompare(b.dueAt))[0];
  const leadRows = L.filter((l) => (!stage || l.stage === stage) && (!source || l.source === source)
    && matches(query, l.customer?.displayName, l.customer?.phone, sourceLabel[l.source]));
  const customerRows = C.filter((c) => matches(query, c.displayName, c.phone));
  const taskRows = T.filter((t) => matches(query, t.title, customer(t.customerId)?.displayName)).sort((a, b) => a.dueAt.localeCompare(b.dueAt));
  const refreshTasks = [['tasks']];
  return <Page title="CRM" subtitle="Лиды, клиенты, задачи и история контактов в одном месте"
    actions={<ActionButton label="Добавить лид" variant="primary" refresh={[['leads']]} fields={[
      { name: 'customerId', label: 'Клиент', type: 'select', required: true, options: C.map((c) => [c.id, `${c.displayName}${c.phone ? ` · ${c.phone}` : ''}`]) },
      { name: 'branchId', label: 'Филиал', type: 'select', required: true, options: (branches.data ?? []).map((b) => [b.id, b.name]) },
      { name: 'source', label: 'Источник', type: 'select', required: true, options: Object.entries(sourceLabel) },
      { name: 'mine', label: 'Назначить на меня', type: 'checkbox' },
    ]} intro={<p>Нет клиента в списке? Сначала добавьте его на вкладке «Клиенты».</p>}
      onSubmit={(v) => post('/retail/leads', { customerId: v.customerId, branchId: v.branchId, source: v.source, assignedUserId: v.mine ? s.view.user.id : null })} />}>
    <Stats>
      <Stat label="Новые" value={L.filter((l) => l.stage === 'new').length} note="нужен первый контакт" />
      <Stat label="Активные" value={L.filter((l) => !['new', 'won', 'lost'].includes(l.stage)).length} note="в работе менеджеров" />
      <Stat label="Задачи сегодня" value={openTasks.filter((t) => new Date(t.dueAt).toDateString() === today).length} note={`просрочено: ${openTasks.filter((t) => new Date(t.dueAt) < new Date()).length}`} />
      <Stat label="Конверсия" value={closed.length ? `${Math.round((L.filter((l) => l.stage === 'won').length / closed.length) * 100)}%` : '—'} note="успешные из закрытых лидов" />
    </Stats>
    <Panel>
      <Tabs value={tab} onChange={setTab} tabs={[['leads', 'Лиды', L.length], ['customers', 'Клиенты', C.length], ['tasks', 'Задачи', openTasks.length]]}
        actions={tab === 'customers'
          ? <ActionButton small label="Новый клиент" fields={customerFields()} refresh={[['customers']]} onSubmit={(v) => post('/retail/customers', { profile: v })} />
          : tab === 'tasks' ? <ActionButton small label="Новая задача" refresh={refreshTasks} fields={[
            { name: 'title', label: 'Что сделать', type: 'text', required: true },
            { name: 'customerId', label: 'Клиент', type: 'select', required: true, options: C.map((c) => [c.id, c.displayName]) },
            { name: 'dueAt', label: 'Срок', type: 'datetime', required: true },
          ]} onSubmit={(v) => post('/retail/tasks', { ...v, ownerUserId: s.view.user.id, leadId: null, dealId: null })} /> : undefined} />
      <Toolbar query={query} onQuery={setQuery} placeholder={tab === 'tasks' ? 'Задача или клиент' : 'Клиент или телефон'} onReset={() => { setStage(''); setSource(''); }}>
        {tab === 'leads' && <>
          <FilterSelect value={stage} onChange={setStage} all="Все этапы" options={Object.entries(stageLabel)} />
          <FilterSelect value={source} onChange={setSource} all="Все источники" options={Object.entries(sourceLabel)} />
        </>}
      </Toolbar>
      {tab === 'leads' && <>
        <ResultMeta>{plural(leadRows.length, ['лид', 'лида', 'лидов'])} · этап изменяется только из карточки лида</ResultMeta>
        <Table rows={leadRows} loading={leads.isLoading} error={leads.error} rowKey={(l) => l.id} onRowClick={(l) => setOpen(l.id)} empty="Лидов пока нет" columns={[
          { title: 'Лид', render: (l) => <Cell main={sourceLabel[l.source] ?? l.source} sub={branches.data?.find((b) => b.id === l.branchId)?.name} /> },
          { title: 'Клиент', render: (l) => <Cell main={l.customer?.displayName ?? customer(l.customerId)?.displayName ?? '—'} sub={l.customer?.phone} /> },
          { title: 'Этап', render: (l) => <Badge tone={stageTone(l.stage)}>{stageLabel[l.stage]}</Badge> },
          { title: 'Следующее действие', render: (l) => { const t = nextTask(l.id, l.customerId); return t ? <Cell main={t.title} sub={dateTime(t.dueAt)} /> : <span className="cell-sub">—</span>; } },
          { title: 'Ответственный', render: (l) => l.assignedUserId === s.view.user.id ? 'Вы' : l.assignedUserId ? 'Назначен' : 'Не назначен' },
          { title: 'Действие', render: (l) => <Button size="sm" onClick={() => setOpen(l.id)}>Открыть</Button> },
        ]} />
      </>}
      {tab === 'customers' && <>
        <ResultMeta>{plural(customerRows.length, ['клиент', 'клиента', 'клиентов'])}</ResultMeta>
        <Table rows={customerRows} loading={customers.isLoading} error={customers.error} rowKey={(c) => c.id} empty="Клиентов пока нет" columns={[
          { title: 'Клиент', render: (c) => <Cell main={c.displayName} sub={c.phone || 'телефон не указан'} /> },
          { title: 'Лиды', render: (c) => L.filter((l) => l.customerId === c.id).length },
          { title: 'Открытые задачи', render: (c) => openTasks.filter((t) => t.customerId === c.id).length },
          { title: 'Действие', render: (c) => <ActionButton small label="Изменить" fields={customerFields(c)} refresh={[['customers']]}
            onSubmit={(v) => patch(`/retail/customers/${c.id}`, { profile: v }, { ifMatch: c.revision })} /> },
        ]} />
      </>}
      {tab === 'tasks' && <>
        <ResultMeta>{plural(taskRows.length, ['задача', 'задачи', 'задач'])}</ResultMeta>
        <Table rows={taskRows} loading={tasks.isLoading} error={tasks.error} rowKey={(t) => t.id} empty="Задач пока нет" columns={[
          { title: 'Задача', render: (t) => <Cell main={t.title} /> },
          { title: 'Клиент', render: (t) => customer(t.customerId)?.displayName ?? '—' },
          { title: 'Срок', render: (t) => <span style={t.status === 'open' && new Date(t.dueAt) < new Date() ? { color: 'var(--danger)', fontWeight: 600 } : undefined}>{dateTime(t.dueAt)}</span> },
          { title: 'Статус', render: (t) => <Badge tone={t.status === 'open' ? 'info' : 'success'}>{t.status === 'open' ? 'Открыта' : 'Выполнена'}</Badge> },
          { title: 'Действие', render: (t) => t.status === 'open' && <ActionButton small label="Выполнено" refresh={refreshTasks}
            onSubmit={() => post(`/retail/tasks/${t.id}/complete`, {}, { ifMatch: t.revision })} /> },
        ]} />
      </>}
    </Panel>
    {open && <LeadDialog id={open} onClose={() => setOpen(null)} />}
  </Page>;
}
