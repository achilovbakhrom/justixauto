import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ActionButton, Badge, Button, Cell, Details, FilterSelect, Modal, Page, Panel, Stat, Stats, Table, Toolbar, dateTime, get, list, matches, patch,
  post, useData, useRefresh, useSearchQuery,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { kindLabel } from './labels';

// ---- types (see /api/v1/identity) ----
interface Label { key?: string; label: string }
interface Company {
  id: string; kind: string; name: string; legalName: string; country: Label; region: Label | null; registration: string;
  email: string; address: string; phone: string; access: string; accessReason: string; revision: string;
}
interface User { id: string; displayName: string; email: string; login: string | null; status: string; roles: { id: string; name: string }[]; mfaEnabled: boolean; revision: string }
interface Role { id: string; name: string; system: boolean; permissionKeys: string[]; revision: string }
interface Permission { key: string; scope: string; requiresMfa: boolean; assignable: boolean }
interface Membership { id: string; companyId: string; status: string; branchAccess: { mode: string }; revision: string }
interface AuditEvent { id: string; occurredAt: string; actorId: string | null; action: string; resourceType: string; resourceId: string; companyId: string | null; reason: string; details: unknown }

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

type Kind = 'seller' | 'bank' | 'mfo' | 'insurance';
const kindTitle: Record<Kind, [string, string]> = {
  seller: ['Компании', 'Продавцы автомобилей · JustixAuto Реализация'], mfo: ['МФО', 'Микрофинансовые организации · кабинет Финансирование'],
  bank: ['Банки', 'Банки · кабинет Финансирование'], insurance: ['Страховые компании', 'Страховые компании · кабинет Страховая'],
};
const cabinetLabel: Record<string, string> = { draft: 'Черновик', active: 'Доступ активен', suspended: 'Доступ приостановлен' };

export function CompaniesPage({ kind }: { kind: Kind }) {
  const q = useData(['admin-companies', kind], () => list<Company>(`/identity/admin/companies?kind=${kind}&limit=100`));
  const [open, setOpen] = useState<Company | null>(null);
  const [query, setQuery] = useSearchQuery();
  const [access, setAccess] = useState('');
  const rows = (q.data ?? []).filter((c) => (!access || c.access === access) && matches(query, c.name, c.registration, c.email, c.legalName));
  const [title, subtitle] = kindTitle[kind];
  return <Page title={title} subtitle={subtitle}
    actions={kind === 'seller'
      ? <ActionButton label="+ Добавить компанию" variant="primary" size="wide" fields={[...companyFields(), ...adminFields]} refresh={[['admin-companies']]}
          intro={<p>Компания, её первый администратор и доступ создаются одной операцией. Компания остаётся черновиком до активации.</p>}
          onSubmit={(v) => post('/identity/admin/seller-companies', { company: companyInput(v), firstAdmin: firstAdmin(v) })} />
      : <ActionButton label={`+ Подключить: ${kindLabel[kind]}`} variant="primary" size="wide" fields={[...companyFields(), ...adminFields]} refresh={[['admin-companies']]}
          intro={<p>Подключение создаёт отдельный кабинет. Оно не подключает API и не публикует программы.</p>}
          onSubmit={(v) => post('/identity/admin/provider-companies', { kind, company: companyInput(v), firstAdmin: firstAdmin(v) })} />}>
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Название, БИН / регистрационный номер или email" onReset={() => setAccess('')}>
        <FilterSelect value={access} onChange={setAccess} all="Все статусы" options={Object.entries(cabinetLabel)} />
      </Toolbar>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(c) => c.id} onRowClick={setOpen} empty="Компаний пока нет" columns={[
        { title: 'Компания', render: (c) => <Cell main={c.name} sub={`${c.registration} · ${kind === 'seller' ? 'Реализация' : kindLabel[kind]}`} /> },
        { title: 'Страна / регион', render: (c) => <Cell main={c.country.label} sub={c.region?.label} /> },
        { title: 'Кабинет', render: (c) => <Badge tone={accessTone(c.access)}>{cabinetLabel[c.access] ?? c.access}</Badge> },
        { title: 'Email', render: (c) => c.email },
        { title: '', render: (c) => <Button size="sm" onClick={() => setOpen(c)}>Открыть</Button> },
      ]} />
    </Panel>
    {open && <CompanyDialog id={open.id} onClose={() => setOpen(null)} />}
  </Page>;
}

export function OverviewPage() {
  const navigate = useNavigate();
  const all = useData(['admin-companies', 'all'], () => list<Company>('/identity/admin/companies?limit=200'));
  const audit = useData(['admin-audit'], () => list<AuditEvent>('/identity/admin/audit?limit=100'));
  const users = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const card = (kind: Kind, path: string) => {
    const cs = (all.data ?? []).filter((c) => c.kind === kind);
    return <Stat key={kind} label={kindTitle[kind][0]} value={cs.length} note={`${cs.filter((c) => c.access === 'active').length} с активным доступом →`} onClick={() => navigate(path)} />;
  };
  return <Page title="Обзор" subtitle="Управление доступом участников JustixAuto">
    <Stats>{card('mfo', '/integrations/mfo')}{card('bank', '/integrations/bank')}{card('insurance', '/integrations/insurance')}{card('seller', '/companies')}</Stats>
    <Panel title="Последние действия" actions={<Button onClick={() => navigate('/audit')}>Весь журнал</Button>}>
      <AuditTable rows={audit.data?.slice(0, 6)} loading={audit.isLoading} error={audit.error} users={users.data ?? []} companies={all.data ?? []} />
    </Panel>
    <div className="admin-note">Создание компании не подключает API и не публикует финансовые программы. Решения по заявкам остаются в кабинетах страховых и финансовых организаций.</div>
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
    <p className="cell-sub">Активация открывает доступ к платформе; она не подтверждает лицензию, API банка или право продавать продукт.</p>
  </Modal>;
}

const statusLabel: Record<string, string> = { pending: 'Без пароля', active: 'Активен', suspended: 'Приостановлен' };

export function UsersPage() {
  const q = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const roles = useData(['admin-roles'], () => list<Role>('/identity/admin/roles'));
  const [open, setOpen] = useState<string | null>(null);
  const [query, setQuery] = useSearchQuery();
  const [status, setStatus] = useState('');
  const rows = (q.data ?? []).filter((u) => (!status || u.status === status) && matches(query, u.displayName, u.email, u.login));
  return <Page title="Пользователи и доступ" subtitle="Глобальные роли и доступ к компаниям"
    actions={<ActionButton label="+ Добавить пользователя" variant="primary" refresh={[['admin-users']]} fields={[
      { name: 'displayName', label: 'Имя', type: 'text', required: true },
      { name: 'email', label: 'E-mail', type: 'email', required: true },
      { name: 'roleIds', label: 'Роли', type: 'multiselect', options: (roles.data ?? []).map((r) => [r.id, r.name]) },
    ]} intro={<p>Логин и временный пароль выдаются в карточке пользователя; письма не отправляются.</p>} onSubmit={(v) => post('/identity/admin/users', v)} />}>
    <Panel>
      <Toolbar query={query} onQuery={setQuery} placeholder="Имя, логин или email" onReset={() => setStatus('')}>
        <FilterSelect value={status} onChange={setStatus} all="Все статусы" options={Object.entries(statusLabel)} />
      </Toolbar>
      <Table rows={rows} loading={q.isLoading} error={q.error} rowKey={(u) => u.id} onRowClick={(u) => setOpen(u.id)} empty="Пользователей нет" columns={[
        { title: 'Пользователь', render: (u) => <Cell main={u.displayName} sub={u.login ? `${u.login} · ${u.email}` : u.email} /> },
        { title: 'Роли', render: (u) => u.roles.map((r) => r.name).join(', ') || '—' },
        { title: 'MFA', render: (u) => u.mfaEnabled ? 'Включена' : 'Не настроена' },
        { title: 'Доступ', render: (u) => <Badge tone={u.status === 'active' ? 'success' : u.status === 'suspended' ? 'danger' : 'warning'}>{statusLabel[u.status] ?? u.status}</Badge> },
        { title: '', render: (u) => <Button size="sm" onClick={() => setOpen(u.id)}>Открыть</Button> },
      ]} />
    </Panel>
    <div className="admin-note">Двухфакторную защиту пользователь настраивает сам в своём кабинете. Пароли в журнал не попадают.</div>
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
  const users = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const options = (perms.data ?? []).filter((p) => p.assignable).map((p): [string, string] => [p.key, p.key + (p.requiresMfa ? ' (MFA)' : '')]);
  const fields = (r?: Role): FieldSpec[] => [
    { name: 'name', label: 'Название', type: 'text', required: true, initial: r?.name ?? '' },
    { name: 'permissionKeys', label: 'Права', type: 'multiselect', options, initial: r?.permissionKeys ?? [] },
  ];
  return <Page title="Роли и разрешения" subtitle="Роли назначаются пользователю и действуют во всех доступных ему компаниях"
    actions={<ActionButton label="+ Создать роль" variant="primary" size="wide" fields={fields()} refresh={[['admin-roles']]} onSubmit={(v) => post('/identity/admin/roles', v)} />}>
    <Panel><Table rows={q.data} loading={q.isLoading} error={q.error} rowKey={(r) => r.id} columns={[
      { title: 'Роль', render: (r) => <Cell main={r.name} sub={r.system ? 'Системная' : 'Пользовательская'} /> },
      { title: 'Разрешения', render: (r) => <span title={r.permissionKeys.join(', ')}>{r.permissionKeys.length}</span> },
      { title: 'Пользователи', render: (r) => (users.data ?? []).filter((u) => u.roles.some((x) => x.id === r.id)).length },
      { title: '', render: (r) => r.system
        ? <ActionButton small label="Просмотреть" fields={[]} submitLabel="Закрыть" intro={<p>{r.permissionKeys.join(', ') || 'Права платформы'}</p>} onSubmit={async () => undefined} />
        : <ActionButton small label="Изменить" size="wide" fields={fields(r)} refresh={[['admin-roles']]}
          onSubmit={(v) => patch(`/identity/admin/roles/${r.id}`, v, { ifMatch: r.revision })} /> },
    ]} /></Panel>
    <div className="admin-note">Системные роли не изменяются. Права платформы выдаются только системной ролью; администрирование не даёт финансовых и страховых решений.</div>
  </Page>;
}

const auditLabel: Record<string, string> = {
  'company.created': 'Компания создана', 'company.activate': 'Доступ компании активирован', 'company.suspend': 'Доступ компании приостановлен',
  'company.restore': 'Доступ компании восстановлен', 'company.provider_provisioned': 'Подключена организация', 'user.mfa_enrolled': 'Включена двухфакторная защита',
  'user.password_changed': 'Пароль изменён пользователем', 'company.updated': 'Реквизиты изменены', 'company.activated': 'Доступ компании активирован',
  'company.suspended': 'Доступ компании приостановлен', 'company.restored': 'Доступ компании восстановлен', 'branch.created': 'Филиал создан',
  'branch.updated': 'Филиал изменён', 'user.created': 'Пользователь создан', 'user.updated': 'Пользователь изменён', 'user.suspended': 'Пользователь приостановлен',
  'user.restored': 'Пользователь восстановлен', 'user.password_set': 'Пароль задан администратором', 'user.sessions_revoked': 'Сеансы завершены',
  'membership.granted': 'Доступ к компании выдан', 'membership.revoked': 'Доступ к компании отозван', 'membership.branch_access_changed': 'Доступ к филиалам изменён',
  'role.created': 'Роль создана', 'role.updated': 'Роль изменена', 'user.bootstrapped': 'Первый администратор платформы',
  'mfa.enrolled': 'Включена двухфакторная защита', 'password.changed': 'Пароль изменён',
};

function AuditTable({ rows, loading, error, users, companies }: { rows: AuditEvent[] | undefined; loading: boolean; error: unknown; users: User[]; companies: Company[] }) {
  return <Table rows={rows} loading={loading} error={error} rowKey={(e) => e.id} empty="Действий пока нет" columns={[
    { title: 'Время', render: (e) => dateTime(e.occurredAt) },
    { title: 'Сотрудник', render: (e) => users.find((u) => u.id === e.actorId)?.displayName ?? (e.actorId ? e.actorId.slice(0, 8) : 'Система') },
    { title: 'Действие', render: (e) => <Cell main={auditLabel[e.action] ?? e.action} sub={e.reason ? `Основание: ${e.reason}` : undefined} /> },
    { title: 'Компания', render: (e) => companies.find((c) => c.id === e.companyId)?.name ?? '—' },
  ]} />;
}

export function AuditPage() {
  const q = useData(['admin-audit'], () => list<AuditEvent>('/identity/admin/audit?limit=100'));
  const users = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const companies = useData(['admin-companies', 'all'], () => list<Company>('/identity/admin/companies?limit=200'));
  return <Page title="Журнал действий" subtitle="История изменений без возможности редактирования">
    <Panel><AuditTable rows={q.data} loading={q.isLoading} error={q.error} users={users.data ?? []} companies={companies.data ?? []} /></Panel>
  </Page>;
}
