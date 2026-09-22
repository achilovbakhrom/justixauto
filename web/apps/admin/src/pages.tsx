import { useState } from 'react';
import {
  ActionButton, Badge, Details, Modal, Page, Panel, Table, Tabs, dateTime, get, list, patch, post, useData, useRefresh,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { kindLabel } from './labels';

// ---- types (see /api/v1/identity) ----
interface Label { key?: string; label: string }
interface Company {
  id: string; kind: string; name: string; legalName: string; country: Label; region: Label | null; registration: string;
  email: string; address: string; phone: string; access: string; accessReason: string; revision: string;
}
interface User { id: string; displayName: string; email: string; login: string | null; status: string; roles: { id: string; name: string }[]; revision: string }
interface Role { id: string; name: string; system: boolean; permissionKeys: string[]; revision: string }
interface Permission { key: string; scope: string; requiresMfa: boolean; assignable: boolean }
interface Membership { id: string; companyId: string; status: string; branchAccess: { mode: string }; revision: string }
interface AuditEvent { id: string; occurredAt: string; actorId: string | null; action: string; resourceType: string; resourceId: string; reason: string; details: unknown }

const accessTone = (a: string) => a === 'active' ? 'success' : a === 'suspended' ? 'danger' : 'warning';
const accessLabel: Record<string, string> = { draft: 'Черновик', active: 'Активна', suspended: 'Приостановлена' };

const companyFields = (c?: Company): FieldSpec[] => [
  { name: 'name', label: 'Название', type: 'text', required: true, initial: c?.name ?? '' },
  { name: 'legalName', label: 'Юридическое название', type: 'text', initial: c?.legalName ?? '' },
  { name: 'country', label: 'Страна', type: 'text', required: true, initial: c?.country.label ?? '' },
  { name: 'region', label: 'Регион', type: 'text', initial: c?.region?.label ?? '' },
  { name: 'registration', label: 'Регистрационный номер (ИНН/БИН)', type: 'text', required: true, initial: c?.registration ?? '' },
  { name: 'email', label: 'E-mail', type: 'email', required: true, initial: c?.email ?? '' },
  { name: 'phone', label: 'Телефон', type: 'text', initial: c?.phone ?? '' },
  { name: 'address', label: 'Адрес', type: 'text', initial: c?.address ?? '' },
];
const companyInput = (v: Record<string, unknown>) => ({
  name: v.name, legalName: v.legalName, country: { label: v.country }, region: v.region ? { label: v.region } : null,
  registration: v.registration, email: v.email, phone: v.phone, address: v.address,
});
const adminFields: FieldSpec[] = [
  { name: 'displayName', label: 'Имя администратора', type: 'text', required: true },
  { name: 'login', label: 'Логин', type: 'text', required: true },
  { name: 'adminEmail', label: 'E-mail администратора', type: 'email', required: true },
  { name: 'password', label: 'Пароль (не менее 12 символов)', type: 'password', required: true },
  { name: 'passwordConfirmation', label: 'Повторите пароль', type: 'password', required: true },
];
const firstAdmin = (v: Record<string, unknown>) => ({ displayName: v.displayName, login: v.login, email: v.adminEmail,
  password: v.password, passwordConfirmation: v.passwordConfirmation });

export function CompaniesPage() {
  const [kind, setKind] = useState<'seller' | 'bank' | 'mfo' | 'insurance'>('seller');
  const q = useData(['admin-companies', kind], () => list<Company>(`/identity/admin/companies?kind=${kind}&limit=100`));
  const [open, setOpen] = useState<Company | null>(null);
  return <Page title="Компании" subtitle="Реестр продавцов и подключённых банков, МФО и страховых"
    actions={kind === 'seller'
      ? <ActionButton label="Добавить продавца" variant="primary" fields={[...companyFields(), ...adminFields]} refresh={[['admin-companies']]}
          intro={<p>Компания создаётся в статусе «черновик» вместе с первым администратором.</p>}
          onSubmit={(v) => post('/identity/admin/seller-companies', { company: companyInput(v), firstAdmin: firstAdmin(v) })} />
      : <ActionButton label={`Подключить: ${kindLabel[kind]}`} variant="primary" fields={[...companyFields(), ...adminFields]} refresh={[['admin-companies']]}
          onSubmit={(v) => post('/identity/admin/provider-companies', { kind, company: companyInput(v), firstAdmin: firstAdmin(v) })} />}>
    <Tabs value={kind} onChange={setKind} tabs={[['seller', 'Продавцы'], ['bank', 'Банки'], ['mfo', 'МФО'], ['insurance', 'Страховые']]} />
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(c) => c.id} onRowClick={setOpen} columns={[
      { title: 'Название', render: (c) => <b>{c.name}</b> },
      { title: 'Страна', render: (c) => c.country.label },
      { title: 'Рег. номер', render: (c) => c.registration },
      { title: 'Доступ', render: (c) => <Badge tone={accessTone(c.access)}>{accessLabel[c.access] ?? c.access}</Badge> },
    ]} /></Panel>
    {open && <CompanyDialog id={open.id} onClose={() => setOpen(null)} />}
  </Page>;
}

function CompanyDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['admin-company', id], () => get<Company>(`/identity/companies/${id}`));
  const c = q.data?.data;
  const refresh = [['admin-company', id], ['admin-companies']];
  const access = (action: string, label: string) => <ActionButton label={label} refresh={refresh}
    fields={[{ name: 'reason', label: 'Основание', type: 'textarea', required: true }]}
    onSubmit={(v) => post(`/identity/admin/companies/${id}/${action}`, { reason: v.reason }, { ifMatch: c!.revision })} />;
  return <Modal title={c?.name ?? 'Компания'} onClose={onClose} size="wide" footer={c && <>
    {c.access === 'draft' && access('activate', 'Активировать')}
    {c.access === 'active' && access('suspend', 'Приостановить')}
    {c.access === 'suspended' && access('restore', 'Восстановить')}
    <ActionButton label="Изменить реквизиты" fields={companyFields(c)} refresh={refresh}
      onSubmit={(v) => patch(`/identity/companies/${id}`, companyInput(v), { ifMatch: c.revision })} />
  </>}>
    {c && <Details items={[
      ['Тип', kindLabel[c.kind]], ['Доступ', <Badge tone={accessTone(c.access)}>{accessLabel[c.access]}</Badge>],
      ['Основание', c.accessReason || '—'], ['Юр. название', c.legalName || '—'], ['Страна / регион', `${c.country.label}${c.region ? ', ' + c.region.label : ''}`],
      ['Рег. номер', c.registration], ['E-mail', c.email], ['Телефон', c.phone || '—'], ['Адрес', c.address || '—'],
    ]} />}
    <p className="kit-muted">Активация открывает доступ к платформе; она не подтверждает лицензию, API банка или право продавать продукт.</p>
  </Modal>;
}

const statusLabel: Record<string, string> = { pending: 'Без пароля', active: 'Активен', suspended: 'Приостановлен' };

export function UsersPage() {
  const q = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=100'));
  const roles = useData(['admin-roles'], () => list<Role>('/identity/admin/roles'));
  const [open, setOpen] = useState<string | null>(null);
  return <Page title="Пользователи" subtitle="Глобальные роли действуют во всех компаниях пользователя"
    actions={<ActionButton label="Добавить пользователя" variant="primary" refresh={[['admin-users']]} fields={[
      { name: 'displayName', label: 'Имя', type: 'text', required: true },
      { name: 'email', label: 'E-mail', type: 'email', required: true },
      { name: 'roleIds', label: 'Роли', type: 'multiselect', options: (roles.data ?? []).map((r) => [r.id, r.name]) },
    ]} onSubmit={(v) => post('/identity/admin/users', v)} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(u) => u.id} onRowClick={(u) => setOpen(u.id)} columns={[
      { title: 'Имя', render: (u) => <b>{u.displayName}</b> }, { title: 'Логин', render: (u) => u.login ?? '—' },
      { title: 'E-mail', render: (u) => u.email }, { title: 'Роли', render: (u) => u.roles.map((r) => r.name).join(', ') || '—' },
      { title: 'Статус', render: (u) => <Badge tone={u.status === 'active' ? 'success' : u.status === 'suspended' ? 'danger' : 'warning'}>{statusLabel[u.status]}</Badge> },
    ]} /></Panel>
    {open && <UserDialog id={open} roles={roles.data ?? []} onClose={() => setOpen(null)} />}
  </Page>;
}

function UserDialog({ id, roles, onClose }: { id: string; roles: Role[]; onClose: () => void }) {
  const q = useData(['admin-user', id], () => get<User>(`/identity/admin/users/${id}`));
  const ms = useData(['admin-memberships', id], () => list<Membership>(`/identity/admin/users/${id}/memberships`));
  const companies = useData(['admin-companies', 'all'], () => list<Company>('/identity/admin/companies?limit=100'));
  const reload = useRefresh();
  const u = q.data?.data;
  const refresh = [['admin-user', id], ['admin-users']];
  const name = (cid: string) => companies.data?.find((c) => c.id === cid)?.name ?? cid;
  const reason: FieldSpec[] = [{ name: 'reason', label: 'Основание', type: 'textarea', required: true }];
  return <Modal title={u?.displayName ?? 'Пользователь'} onClose={onClose} size="wide" footer={u && <>
    <ActionButton label="Изменить" refresh={refresh} fields={[
      { name: 'displayName', label: 'Имя', type: 'text', required: true, initial: u.displayName },
      { name: 'roleIds', label: 'Роли', type: 'multiselect', options: roles.map((r) => [r.id, r.name]), initial: u.roles.map((r) => r.id) },
    ]} onSubmit={(v) => patch(`/identity/admin/users/${id}`, v, { ifMatch: u.revision })} />
    <ActionButton label={u.login ? 'Сбросить пароль' : 'Выдать доступ'} refresh={refresh}
      intro={<p>Пользователь сменит временный пароль при первом входе. Двухфакторная защита не отключается.</p>} fields={[
        ...(u.login ? [] : [{ name: 'login', label: 'Логин', type: 'text', required: true } as FieldSpec]),
        { name: 'password', label: 'Временный пароль', type: 'password', required: true },
        { name: 'passwordConfirmation', label: 'Повторите пароль', type: 'password', required: true },
      ]} onSubmit={(v) => post(`/identity/admin/users/${id}/password`, v, { ifMatch: u.revision })} />
    {u.status === 'suspended'
      ? <ActionButton label="Восстановить" fields={reason} refresh={refresh} onSubmit={(v) => post(`/identity/admin/users/${id}/restore`, v, { ifMatch: u.revision })} />
      : <ActionButton label="Приостановить" variant="danger" fields={reason} refresh={refresh} onSubmit={(v) => post(`/identity/admin/users/${id}/suspend`, v, { ifMatch: u.revision })} />}
    <ActionButton label="Завершить сеансы" fields={reason} onSubmit={(v) => post(`/identity/admin/users/${id}/revoke-sessions`, v)} />
  </>}>
    {u && <Details items={[['E-mail', u.email], ['Логин', u.login ?? '—'], ['Статус', statusLabel[u.status]],
      ['Роли', u.roles.map((r) => r.name).join(', ') || '—']]} />}
    <Panel title="Членство в компаниях" actions={<ActionButton label="Добавить" refresh={[['admin-memberships', id]]} fields={[
      { name: 'companyId', label: 'Компания', type: 'select', required: true, options: (companies.data ?? []).map((c) => [c.id, c.name]) },
    ]} onSubmit={(v) => post(`/identity/admin/users/${id}/memberships`, { companyId: v.companyId, branchAccess: { mode: 'ALL_BRANCHES', branchIds: [] } })} />}>
      <Table rows={ms.data} loading={ms.isLoading} error={ms.error} rowKey={(m) => m.id} columns={[
        { title: 'Компания', render: (m) => name(m.companyId) },
        { title: 'Филиалы', render: (m) => m.branchAccess.mode === 'ALL_BRANCHES' ? 'Все' : 'Выбранные' },
        { title: 'Статус', render: (m) => <Badge tone={m.status === 'active' ? 'success' : undefined}>{m.status === 'active' ? 'Активно' : 'Отозвано'}</Badge> },
        { title: '', render: (m) => m.status === 'active' && <ActionButton label="Отозвать" fields={reason}
          onSubmit={async (v) => { await post(`/identity/admin/memberships/${m.id}/revoke`, v, { ifMatch: m.revision }); await reload(['admin-memberships', id]); }} /> },
      ]} />
    </Panel>
  </Modal>;
}

export function RolesPage() {
  const q = useData(['admin-roles'], () => list<Role>('/identity/admin/roles'));
  const perms = useData(['admin-permissions'], () => list<Permission>('/identity/admin/permissions'));
  const options = (perms.data ?? []).filter((p) => p.assignable).map((p): [string, string] => [p.key, p.key + (p.requiresMfa ? ' (MFA)' : '')]);
  const fields = (r?: Role): FieldSpec[] => [
    { name: 'name', label: 'Название', type: 'text', required: true, initial: r?.name ?? '' },
    { name: 'permissionKeys', label: 'Права', type: 'multiselect', options, initial: r?.permissionKeys ?? [] },
  ];
  return <Page title="Роли и права" subtitle="Системные роли не изменяются; права платформы выдаются только системной ролью"
    actions={<ActionButton label="Создать роль" variant="primary" size="wide" fields={fields()} refresh={[['admin-roles']]} onSubmit={(v) => post('/identity/admin/roles', v)} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(r) => r.id} columns={[
      { title: 'Роль', render: (r) => <b>{r.name}</b> },
      { title: 'Тип', render: (r) => r.system ? <Badge tone="info">Системная</Badge> : 'Пользовательская' },
      { title: 'Права', render: (r) => r.permissionKeys.join(', ') },
      { title: '', render: (r) => !r.system && <ActionButton label="Изменить" size="wide" fields={fields(r)} refresh={[['admin-roles']]}
        onSubmit={(v) => patch(`/identity/admin/roles/${r.id}`, v, { ifMatch: r.revision })} /> },
    ]} /></Panel>
  </Page>;
}

export function AuditPage() {
  const q = useData(['admin-audit'], () => list<AuditEvent>('/identity/admin/audit?limit=100'));
  return <Page title="Аудит" subtitle="Кто, что, когда и почему — только чтение">
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(e) => e.id} columns={[
      { title: 'Время', render: (e) => dateTime(e.occurredAt) }, { title: 'Действие', render: (e) => e.action },
      { title: 'Объект', render: (e) => `${e.resourceType} ${e.resourceId.slice(0, 8)}` }, { title: 'Основание', render: (e) => e.reason || '—' },
      { title: 'Детали', render: (e) => <code style={{ fontSize: 11 }}>{JSON.stringify(e.details)}</code> },
    ]} /></Panel>
  </Page>;
}
