import { useState } from 'react';
import { ActionButton, Badge, EmptyState, Page, Panel, ResultMeta, Stat, Stats, Tabs, Toolbar, list, matches, plural, post, useData, useSearchQuery, useSession } from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { partnershipLabel, usePartners } from '../data';
import type { Partnership } from '../data';

const reason: FieldSpec[] = [{ name: 'reason', label: 'Причина', type: 'textarea', required: true }];
const initials = (n: string) => n.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase()).join('');
type Tab = 'active' | 'incoming' | 'outgoing' | 'closed';
const inTab = (p: Partnership, t: Tab) => t === 'active' ? p.status === 'active'
  : t === 'closed' ? !['active', 'requested'].includes(p.status) : p.status === 'requested' && p.direction === t;

export function PartnersPage() {
  const q = usePartners();
  const s = useSession();
  const dir = useData(['directory', 'seller'], () => list<{ id: string; name: string; country: string }>('/identity/directory/companies?kind=seller&limit=200'));
  const [tab, setTab] = useState<Tab>('active');
  const [query, setQuery] = useSearchQuery();
  const refresh = [['partnerships']];
  const all = q.data ?? [];
  const count = (t: Tab) => all.filter((p) => inTab(p, t)).length;
  const known = new Set(all.filter((p) => ['active', 'requested'].includes(p.status)).map((p) => p.counterparty.id));
  const rows = all.filter((p) => inTab(p, tab) && matches(query, p.counterparty.name, p.counterparty.country));
  return <Page title="Партнёры" subtitle="После подтверждения партнёрства доступны цены, предложения и новые закупки"
    actions={<ActionButton label="Добавить партнёра" variant="primary" refresh={refresh} fields={[
      { name: 'counterpartyCompanyId', label: 'Компания', type: 'select', required: true,
        options: (dir.data ?? []).filter((c) => c.id !== s.company?.id && !known.has(c.id)).map((c) => [c.id, `${c.name} · ${c.country}`]) },
    ]} intro={<p>Компания получит запрос и сможет принять или отклонить его. До подтверждения доступен только профиль.</p>}
      onSubmit={(v) => post('/commerce/partnerships', v)} />}>
    <Stats columns={3}>
      <Stat label="Активные" value={count('active')} note="доступен каталог" />
      <Stat label="Входящие" value={count('incoming')} note="нужно принять решение" />
      <Stat label="Исходящие" value={count('outgoing')} note="ожидают ответа" />
    </Stats>
    <Panel>
      <Tabs value={tab} onChange={setTab} tabs={[['active', 'Активные', count('active')], ['incoming', 'Входящие', count('incoming')],
        ['outgoing', 'Исходящие', count('outgoing')], ['closed', 'Завершённые', count('closed')]]} />
      <Toolbar query={query} onQuery={setQuery} placeholder="Компания или страна" />
      {rows.length > 0 && <ResultMeta>{plural(rows.length, ['компания', 'компании', 'компаний'])}</ResultMeta>}
      {rows.map((p) => <div key={p.id} className="partner-band">
        <div className="partner-id"><div className="partner-logo">{initials(p.counterparty.name)}</div><div>
          <strong>{p.counterparty.name}</strong>
          <span>{p.counterparty.country} · {p.direction === 'incoming' ? 'Запросил партнёрство у вас' : 'Вы запросили партнёрство'}</span>
          {p.statusReason && <span>Причина: {p.statusReason}</span>}
        </div></div>
        <div><Badge tone={p.status === 'active' ? 'success' : p.status === 'requested' ? 'warning' : undefined}>{partnershipLabel[p.status]}</Badge></div>
        <div className="partner-actions">{p.allowedActions.map((a) => a === 'accept'
          ? <ActionButton key={a} label="Принять" variant="primary" refresh={refresh} onSubmit={() => post(`/commerce/partnerships/${p.id}/accept`, {}, { ifMatch: p.revision })} />
          : <ActionButton key={a} label={{ decline: 'Отклонить', withdraw: 'Отозвать', end: 'Завершить' }[a] ?? a} fields={reason} refresh={refresh}
            onSubmit={(v) => post(`/commerce/partnerships/${p.id}/${a}`, v, { ifMatch: p.revision })} />)}</div>
      </div>)}
      {rows.length === 0 && !q.isLoading && <EmptyState icon="users" title="Здесь пока пусто"
        text={tab === 'active' ? 'Добавьте партнёра, чтобы видеть его предложения и закупать автомобили.' : 'Запросов в этом разделе нет.'} />}
    </Panel>
  </Page>;
}
