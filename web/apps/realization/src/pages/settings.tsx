import { useState } from 'react';
import {
  ActionButton, Badge, Button, Cell, ChangePassword, Details, MFASetup, Notice, Panel, Table, get, patch, post, useData, useRefresh, useScopeLabel, useSession,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { useBranches, useWarehouses } from '../data';
import type { Branch } from '../data';
import { WarehouseDialog } from './inventory';

interface Company { id: string; name: string; legalName: string; country: { key?: string; label: string }; region: { key?: string; label: string } | null; registration: string; email: string; address: string; phone: string; access: string }

const branchFields = (b?: Branch): FieldSpec[] => [
  { name: 'name', label: 'Название', type: 'text', required: true, ...(b ? { initial: b.name } : {}) },
  { name: 'address', label: 'Адрес', type: 'text', required: true, ...(b ? { initial: b.address } : {}) },
];
type Section = 'companies' | 'members' | 'account';
const sections: [Section, string, string][] = [
  ['companies', 'Компании и филиалы', 'Юридические лица и точки продаж'],
  ['members', 'Пользователи и роли', 'Доступ сотрудников'],
  ['account', 'Личный аккаунт', 'Профиль, пароль и MFA'],
];
const initials = (n: string) => n.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase()).join('');

export function SettingsPage() {
  const [section, setSection] = useState<Section>('companies');
  return <section className="page">
    <div className="page-header"><div><h1 className="page-title">Настройки</h1><div className="page-subtitle">Компания, филиалы, склады, доступ сотрудников и личная безопасность</div></div></div>
    <div className="settings-layout">
      <nav className="settings-nav">{sections.map(([k, title, sub]) =>
        <button key={k} className={`settings-nav-item${section === k ? ' active' : ''}`} onClick={() => setSection(k)}><strong>{title}</strong><span>{sub}</span></button>)}</nav>
      <div>{section === 'companies' ? <Companies /> : section === 'members' ? <Members /> : <Account />}</div>
    </div>
  </section>;
}

function Companies() {
  const s = useSession();
  const id = s.company?.id ?? '';
  const company = useData(['company', id], () => get<Company>(`/identity/companies/${id}`), !!id);
  const branches = useBranches();
  const warehouses = useWarehouses();
  const scope = useScopeLabel();
  const reload = useRefresh();
  const [open, setOpen] = useState<string | null>(null);
  const c = company.data?.data;
  const refresh = [['company', id], ['branches']];
  return <div className="settings-content">
    <div className="settings-title-row"><div><h2>Компания и филиалы</h2><p>Изменение данных не меняет рабочий контекст. Новые компании подключает администратор платформы.</p></div></div>
    <section className="surface">
      <div className="section-head">
        <div className="partner-id"><div className="partner-logo">{c ? initials(c.name) : ''}</div><div><strong>{c?.name}</strong>
          <span>{[c?.legalName, c?.registration && `Рег. № ${c.registration}`].filter(Boolean).join(' · ')}</span></div></div>
        <div className="partner-actions">
          <Badge tone={c?.access === 'active' ? 'success' : 'warning'}>{c?.access === 'active' ? 'Активна' : c?.access === 'suspended' ? 'Приостановлена' : 'Черновик'}</Badge>
          {c && s.can('company.edit') && <ActionButton small label="Изменить данные" refresh={refresh} fields={[
            { name: 'name', label: 'Название', type: 'text', required: true, initial: c.name },
            { name: 'legalName', label: 'Юридическое название', type: 'text', required: true, initial: c.legalName },
            { name: 'registration', label: 'Регистрационный номер', type: 'text', required: true, initial: c.registration },
            { name: 'email', label: 'Email', type: 'email', required: true, initial: c.email },
            { name: 'phone', label: 'Телефон', type: 'text', initial: c.phone },
            { name: 'address', label: 'Юридический адрес', type: 'text', initial: c.address },
          ]} onSubmit={(v) => patch(`/identity/companies/${id}`, { ...v, country: c.country, region: c.region }, { ifMatch: company.data!.revision })} />}
        </div>
      </div>
      <div className="section-body"><div className="facts-grid">
        <div className="fact"><span>Страна и регион</span><strong>{c ? [c.country.label, c.region?.label].filter(Boolean).join(', ') : '—'}</strong></div>
        <div className="fact"><span>Юридический адрес</span><strong>{c?.address || '—'}</strong></div>
        <div className="fact"><span>Контакты</span><strong>{[c?.email, c?.phone].filter(Boolean).join(' · ') || '—'}</strong></div>
        <div className="fact"><span>Рабочий контекст</span><strong>{scope}</strong></div>
      </div></div>
    </section>
    <Panel title={`Филиалы · ${c?.name ?? ''}`} actions={s.can('branches.create') && <ActionButton small variant="primary" label="Добавить филиал" refresh={refresh}
      fields={branchFields()} onSubmit={(v) => post(`/identity/companies/${id}/branches`, v)} />}>
      <Table rows={branches.data} loading={branches.isLoading} error={branches.error} rowKey={(b) => b.id} empty="Филиалов пока нет" columns={[
        { title: 'Филиал', render: (b) => <Cell main={b.name} /> },
        { title: 'Адрес', render: (b) => b.address },
        { title: 'Склад', render: (b) => { const w = warehouses.data?.find((x) => x.branchId === b.id); return w
          ? <><Cell main={w.name} sub={`${w.occupied} из ${w.capacity} мест`} /><div className="kit-row" style={{ marginTop: 6 }}>
            <Button size="sm" onClick={() => setOpen(w.id)}>Открыть</Button>
            <ActionButton small label="Отвязать" onSubmit={async () => { await post(`/inventory/warehouses/${w.id}/branch-attachment`, { branchId: null }, { ifMatch: w.revision }); await reload(['warehouses']); }} /></div></>
          : <span className="cell-sub">Основной склад не привязан</span>; } },
        { title: 'Действия', render: (b) => s.can('branches.edit') && <ActionButton small label="Изменить" fields={branchFields(b)} refresh={refresh}
          onSubmit={(v) => patch(`/identity/companies/${id}/branches/${b.id}`, v, { ifMatch: b.revision })} /> },
      ]} />
    </Panel>
    {open && <WarehouseDialog id={open} onClose={() => setOpen(null)} />}
  </div>;
}

function Members() {
  const s = useSession();
  return <div className="settings-content">
    <div className="settings-title-row"><div><h2>Пользователи и роли</h2><p>Роли глобальные и действуют во всех компаниях пользователя.</p></div></div>
    <Notice>Сотрудников, их доступ к компаниям и роли назначает администратор платформы JustixAuto. Отправьте ему запрос с именем, email и нужными разделами.</Notice>
    <Panel title="Ваш доступ" padded><Details items={[
      ['Роли', s.view.roles.map((r) => r.name).join(', ') || '—'],
      ['Разрешений', String(s.view.permissions.length)],
      ['Компаний доступно', String(s.view.accessibleCompanies.length)],
    ]} /></Panel>
  </div>;
}

function Account() {
  const s = useSession();
  const [tab, setTab] = useState<'mfa' | 'password' | null>(null);
  return <div className="settings-content">
    <div className="settings-title-row"><div><h2>Личный аккаунт</h2><p>Профиль, пароль и двухфакторная защита</p></div></div>
    <Panel title="Профиль" padded><Details items={[['Имя', s.view.user.displayName], ['Статус', s.view.user.status === 'active' ? 'Активен' : s.view.user.status]]} /></Panel>
    <Panel title="Безопасность" padded>
      {!s.view.mfa.disabled && <Details items={[['Двухфакторная защита', s.view.mfa.enrolled ? <Badge tone="success">Включена</Badge> : <Badge tone="warning">Выключена</Badge>]]} />}
      <div className="kit-row" style={{ marginTop: 14 }}>
        {!s.view.mfa.enrolled && !s.view.mfa.disabled && <Button variant="primary" onClick={() => setTab('mfa')}>Включить двухфакторную защиту</Button>}
        <Button onClick={() => setTab('password')}>Сменить пароль</Button>
      </div>
      {tab === 'mfa' && <div style={{ marginTop: 16 }}><MFASetup onDone={() => { void s.refresh(); setTab(null); }} /></div>}
      {tab === 'password' && <div style={{ marginTop: 16, maxWidth: 420 }}><ChangePassword onDone={() => { void s.refresh(); setTab(null); }} /></div>}
    </Panel>
  </div>;
}
