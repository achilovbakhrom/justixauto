import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ActionButton,
  Badge,
  Button,
  Cell,
  Details,
  FilterSelect,
  Modal,
  Page,
  Panel,
  Stat,
  Stats,
  Table,
  Toolbar,
  dateTime,
  get,
  list,
  canonicalCountry,
  countries,
  matches,
  patch,
  post,
  regionsFor,
  useData,
  useSearchQuery,
} from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { kindLabel } from './labels';

// ---- types (see /api/v1/identity) ----
interface Label {
  key?: string;
  label: string;
}
interface Company {
  id: string;
  kind: string;
  name: string;
  legalName: string;
  country: Label;
  region: Label | null;
  registration: string;
  email: string;
  address: string;
  phone: string;
  access: string;
  accessReason: string;
  revision: string;
}
interface User {
  id: string;
  displayName: string;
  email: string;
  login: string | null;
  status: string;
  roles: { id: string; name: string }[];
  revision: string;
}
interface Role {
  id: string;
  name: string;
  system: boolean;
  scope: string;
  companyKind: string | null;
  permissionKeys: string[];
  revision: string;
}
interface Permission {
  key: string;
  scope: string;
  assignable: boolean;
}
interface AuditEvent {
  id: string;
  occurredAt: string;
  actorId: string | null;
  action: string;
  resourceType: string;
  resourceId: string;
  companyId: string | null;
  reason: string;
  details: unknown;
}

const accessTone = (a: string) => (a === 'active' ? 'success' : a === 'suspended' ? 'danger' : 'warning');
const accessLabel: Record<string, string> = { draft: 'Черновик', active: 'Активна', suspended: 'Приостановлена' };

const companyFields = (c?: Company): FieldSpec[] => [
  {
    name: 'name',
    label: 'Название компании',
    type: 'text',
    required: true,
    initial: c?.name ?? '',
    group: 'Информация о компании',
  },
  {
    name: 'country',
    label: 'Страна',
    type: 'combobox',
    initial: c?.country.label ?? '',
    options: countries(),
    canonicalize: canonicalCountry,
    placeholder: 'Выберите или найдите страну',
    ariaLabel: 'Показать страны',
    group: 'Адрес',
  },
  {
    name: 'region',
    label: 'Регион',
    type: 'combobox',
    initial: c?.region?.label ?? '',
    dependsOn: 'country',
    optionsFor: regionsFor,
    placeholder: 'Выберите или найдите регион',
    disabledPlaceholder: 'Сначала выберите страну',
    ariaLabel: 'Показать регионы',
    group: 'Адрес',
  },
  { name: 'address', label: 'Адрес', type: 'text', initial: c?.address ?? '', group: 'Адрес' },
  {
    name: 'email',
    label: 'Электронная почта',
    type: 'email',
    initial: c?.email ?? '',
    group: 'Контакты',
  },
  { name: 'phone', label: 'Телефон', type: 'text', initial: c?.phone ?? '', group: 'Контакты' },
];
// registration is never entered in a form; on create it stays empty and on
// edit the company's current value is preserved unchanged (it only ever
// changes through the future government-source integration).
const companyInput = (v: Record<string, unknown>, legalName = '', registration = '') => ({
  name: v.name,
  legalName,
  country: { label: canonicalCountry(String(v.country)) ?? v.country },
  region: v.region ? { label: v.region } : null,
  registration,
  email: v.email,
  phone: v.phone,
  address: v.address,
});
const adminFields: FieldSpec[] = [
  {
    name: 'displayName',
    label: 'Имя администратора',
    type: 'text',
    hint: 'Если не заполнено, используется логин.',
    group: 'Данные для входа',
  },
  { name: 'login', label: 'Логин', type: 'text', required: true, group: 'Данные для входа' },
  {
    name: 'password',
    label: 'Пароль',
    type: 'password',
    required: true,
    hint: 'Не менее 12 символов.',
    group: 'Данные для входа',
  },
  {
    name: 'passwordConfirmation',
    label: 'Повторите пароль',
    type: 'password',
    required: true,
    group: 'Данные для входа',
  },
];
const firstAdmin = (v: Record<string, unknown>) => ({
  displayName: v.displayName,
  login: v.login,
  email: v.email,
  password: v.password,
  passwordConfirmation: v.passwordConfirmation,
});

type Kind = 'seller' | 'bank' | 'mfo' | 'insurance';
const kindTitle: Record<Kind, [string, string]> = {
  seller: ['Компании', 'Продавцы автомобилей · JustixAuto Реализация'],
  mfo: ['МФО', 'Микрофинансовые организации · кабинет Финансирование'],
  bank: ['Банки', 'Банки · кабинет Финансирование'],
  insurance: ['Страховые компании', 'Страховые компании · кабинет Страховая'],
};
const cabinetLabel: Record<string, string> = {
  draft: 'Черновик',
  active: 'Доступ активен',
  suspended: 'Доступ приостановлен',
};

export function CompaniesPage({ kind }: { kind: Kind }) {
  const q = useData(['admin-companies', kind], () => list<Company>(`/identity/admin/companies?kind=${kind}&limit=100`));
  const [open, setOpen] = useState<Company | null>(null);
  const [query, setQuery] = useSearchQuery();
  const [access, setAccess] = useState('');
  const rows = (q.data ?? []).filter(
    (c) => (!access || c.access === access) && matches(query, c.name, c.registration, c.email, c.legalName),
  );
  const [title, subtitle] = kindTitle[kind];
  return (
    <Page
      title={title}
      subtitle={subtitle}
      actions={
        kind === 'seller' ? (
          <ActionButton
            label="+ Добавить компанию"
            title="Новая компания и администратор"
            submitLabel="Создать компанию"
            variant="primary"
            size="wide"
            fields={[...companyFields(), ...adminFields]}
            refresh={[['admin-companies']]}
            intro={
              <p>
                Контактная электронная почта также используется для первого администратора. Компания остаётся черновиком
                до активации.
              </p>
            }
            onSubmit={(v) =>
              post('/identity/admin/seller-companies', { company: companyInput(v), firstAdmin: firstAdmin(v) })
            }
          />
        ) : (
          <ActionButton
            label={`+ Подключить: ${kindLabel[kind]}`}
            title={`Новая компания и администратор: ${kindLabel[kind]}`}
            submitLabel="Создать компанию"
            variant="primary"
            size="wide"
            fields={[...companyFields(), ...adminFields]}
            refresh={[['admin-companies']]}
            intro={
              <p>
                Контактная электронная почта также используется для первого администратора. Подключение создаёт
                отдельный кабинет.
              </p>
            }
            onSubmit={(v) =>
              post('/identity/admin/provider-companies', { kind, company: companyInput(v), firstAdmin: firstAdmin(v) })
            }
          />
        )
      }
    >
      <Panel>
        <Toolbar
          query={query}
          onQuery={setQuery}
          placeholder="Название, БИН / регистрационный номер или email"
          onReset={() => setAccess('')}
        >
          <FilterSelect value={access} onChange={setAccess} all="Все статусы" options={Object.entries(cabinetLabel)} />
        </Toolbar>
        <Table
          rows={rows}
          loading={q.isLoading}
          error={q.error}
          rowKey={(c) => c.id}
          onRowClick={setOpen}
          empty="Компаний пока нет"
          columns={[
            {
              title: 'Компания',
              render: (c) => (
                <Cell
                  main={c.name}
                  sub={[c.registration, kind === 'seller' ? 'Реализация' : kindLabel[kind]].filter(Boolean).join(' · ')}
                />
              ),
            },
            { title: 'Страна / регион', render: (c) => <Cell main={c.country.label} sub={c.region?.label} /> },
            {
              title: 'Кабинет',
              render: (c) => <Badge tone={accessTone(c.access)}>{cabinetLabel[c.access] ?? c.access}</Badge>,
            },
            { title: 'Email', render: (c) => c.email },
            {
              title: '',
              render: (c) => (
                <Button size="sm" onClick={() => setOpen(c)}>
                  Открыть
                </Button>
              ),
            },
          ]}
        />
      </Panel>
      {open && <CompanyDialog id={open.id} onClose={() => setOpen(null)} />}
    </Page>
  );
}

export function OverviewPage() {
  const navigate = useNavigate();
  const all = useData(['admin-companies', 'all'], () => list<Company>('/identity/admin/companies?limit=200'));
  const audit = useData(['admin-audit'], () => list<AuditEvent>('/identity/admin/audit?limit=100'));
  const users = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const card = (kind: Kind, path: string) => {
    const cs = (all.data ?? []).filter((c) => c.kind === kind);
    return (
      <Stat
        key={kind}
        label={kindTitle[kind][0]}
        value={cs.length}
        note={`${cs.filter((c) => c.access === 'active').length} с активным доступом →`}
        onClick={() => navigate(path)}
      />
    );
  };
  return (
    <Page title="Обзор" subtitle="Управление доступом участников JustixAuto">
      <Stats>
        {card('mfo', '/integrations/mfo')}
        {card('bank', '/integrations/bank')}
        {card('insurance', '/integrations/insurance')}
        {card('seller', '/companies')}
      </Stats>
      <Panel title="Последние действия" actions={<Button onClick={() => navigate('/audit')}>Весь журнал</Button>}>
        <AuditTable
          rows={audit.data?.slice(0, 6)}
          loading={audit.isLoading}
          error={audit.error}
          users={users.data ?? []}
          companies={all.data ?? []}
        />
      </Panel>
      <div className="admin-note">
        Создание компании не подключает API и не публикует финансовые программы. Решения по заявкам остаются в кабинетах
        страховых и финансовых организаций.
      </div>
    </Page>
  );
}

function CompanyDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const q = useData(['admin-company', id], () => get<Company>(`/identity/companies/${id}`));
  const c = q.data?.data;
  const refresh = [['admin-company', id], ['admin-companies']];
  const access = (action: string, label: string) => (
    <ActionButton
      label={label}
      refresh={refresh}
      fields={[{ name: 'reason', label: 'Основание', type: 'textarea', required: true }]}
      onSubmit={(v) =>
        post(`/identity/admin/companies/${id}/${action}`, { reason: v.reason }, { ifMatch: c!.revision })
      }
    />
  );
  return (
    <Modal
      title={c?.name ?? 'Компания'}
      onClose={onClose}
      size="wide"
      footer={
        c && (
          <>
            {c.access === 'draft' && access('activate', 'Активировать')}
            {c.access === 'active' && access('suspend', 'Приостановить')}
            {c.access === 'suspended' && access('restore', 'Восстановить')}
            <ActionButton
              label="Удалить"
              variant="danger"
              intro={
                <p>
                  Компания исчезнет из всех списков и справочников, её сотрудники потеряют к ней доступ. Данные и
                  история сохраняются.
                </p>
              }
              fields={[{ name: 'reason', label: 'Основание', type: 'textarea', required: true }]}
              refresh={[['admin-companies'], ['admin-memberships']]}
              onSubmit={async (v) => {
                const r = await post(
                  `/identity/admin/companies/${id}/delete`,
                  { reason: v.reason },
                  { ifMatch: c.revision },
                );
                onClose();
                return r;
              }}
            />
            <ActionButton
              label="Изменить реквизиты"
              fields={companyFields(c)}
              refresh={refresh}
              onSubmit={(v) =>
                patch(`/identity/companies/${id}`, companyInput(v, c.legalName, c.registration), {
                  ifMatch: c.revision,
                })
              }
            />
          </>
        )
      }
    >
      {c && (
        <Details
          items={[
            ['Тип', kindLabel[c.kind]],
            ['Доступ', <Badge tone={accessTone(c.access)}>{accessLabel[c.access]}</Badge>],
            ['Основание', c.accessReason || '—'],
            ['Юр. название', c.legalName || '—'],
            ['Страна / регион', `${c.country.label}${c.region ? ', ' + c.region.label : ''}`],
            ...(c.registration ? [['Рег. номер', c.registration] as [string, string]] : []),
            ['E-mail', c.email],
            ['Телефон', c.phone || '—'],
            ['Адрес', c.address || '—'],
          ]}
        />
      )}
      <p className="cell-sub">
        Активация открывает доступ к платформе; она не подтверждает лицензию, API банка или право продавать продукт.
      </p>
    </Modal>
  );
}

const statusLabel: Record<string, string> = { pending: 'Без пароля', active: 'Активен', suspended: 'Приостановлен' };

export function UsersPage() {
  const q = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const roles = useData(['admin-roles'], () => list<Role>('/identity/admin/roles'));
  const staffRoles = (roles.data ?? []).filter((r) => r.scope === 'platform');
  const [open, setOpen] = useState<string | null>(null);
  const [query, setQuery] = useSearchQuery();
  const [status, setStatus] = useState('');
  const rows = (q.data ?? []).filter(
    (u) => (!status || u.status === status) && matches(query, u.displayName, u.email, u.login),
  );
  return (
    <Page
      title="Сотрудники платформы"
      subtitle="Администраторы и операторы JustixAuto. Сотрудников компаний добавляет администратор компании в своём кабинете."
      actions={
        <ActionButton
          label="+ Добавить сотрудника"
          variant="primary"
          refresh={[['admin-users']]}
          fields={[
            { name: 'displayName', label: 'Имя', type: 'text', required: true },
            { name: 'login', label: 'Логин', type: 'text' },
            { name: 'password', label: 'Временный пароль (не менее 12 символов)', type: 'password' },
            { name: 'email', label: 'E-mail', type: 'email' },
            { name: 'roleIds', label: 'Роли', type: 'multiselect', options: staffRoles.map((r) => [r.id, r.name]) },
          ]}
          intro={
            <p>
              Логин и временный пароль можно выдать сразу или позже в карточке сотрудника — при первом входе пароль
              нужно сменить. Письма не отправляются.
            </p>
          }
          onSubmit={(v) => post('/identity/admin/users', v)}
        />
      }
    >
      <Panel>
        <Toolbar query={query} onQuery={setQuery} placeholder="Имя, логин или email" onReset={() => setStatus('')}>
          <FilterSelect value={status} onChange={setStatus} all="Все статусы" options={Object.entries(statusLabel)} />
        </Toolbar>
        <Table
          rows={rows}
          loading={q.isLoading}
          error={q.error}
          rowKey={(u) => u.id}
          onRowClick={(u) => setOpen(u.id)}
          empty="Сотрудников нет"
          columns={[
            {
              title: 'Сотрудник',
              render: (u) => <Cell main={u.displayName} sub={[u.login, u.email].filter(Boolean).join(' · ')} />,
            },
            { title: 'Роли', render: (u) => u.roles.map((r) => r.name).join(', ') || '—' },
            {
              title: 'Доступ',
              render: (u) => (
                <Badge tone={u.status === 'active' ? 'success' : u.status === 'suspended' ? 'danger' : 'warning'}>
                  {statusLabel[u.status] ?? u.status}
                </Badge>
              ),
            },
            {
              title: '',
              render: (u) => (
                <Button size="sm" onClick={() => setOpen(u.id)}>
                  Открыть
                </Button>
              ),
            },
          ]}
        />
      </Panel>
      <div className="admin-note">Пароли в журнал не попадают.</div>
      {open && <UserDialog id={open} roles={staffRoles} onClose={() => setOpen(null)} />}
    </Page>
  );
}

function UserDialog({ id, roles, onClose }: { id: string; roles: Role[]; onClose: () => void }) {
  const q = useData(['admin-user', id], () => get<User>(`/identity/admin/users/${id}`));
  const u = q.data?.data;
  const refresh = [['admin-user', id], ['admin-users']];
  const reason: FieldSpec[] = [{ name: 'reason', label: 'Основание', type: 'textarea', required: true }];
  return (
    <Modal
      title={u?.displayName ?? 'Сотрудник'}
      onClose={onClose}
      size="wide"
      footer={
        u && (
          <>
            <ActionButton
              label="Изменить"
              refresh={refresh}
              fields={[
                { name: 'displayName', label: 'Имя', type: 'text', required: true, initial: u.displayName },
                {
                  name: 'roleIds',
                  label: 'Роли',
                  type: 'multiselect',
                  options: roles.map((r) => [r.id, r.name]),
                  initial: u.roles.map((r) => r.id),
                },
              ]}
              onSubmit={(v) => patch(`/identity/admin/users/${id}`, v, { ifMatch: u.revision })}
            />
            <ActionButton
              label={u.login ? 'Сбросить пароль' : 'Выдать доступ'}
              refresh={refresh}
              intro={<p>Сотрудник сменит временный пароль при первом входе.</p>}
              fields={[
                ...(u.login ? [] : [{ name: 'login', label: 'Логин', type: 'text', required: true } as FieldSpec]),
                { name: 'password', label: 'Временный пароль', type: 'password', required: true },
                { name: 'passwordConfirmation', label: 'Повторите пароль', type: 'password', required: true },
              ]}
              onSubmit={(v) => post(`/identity/admin/users/${id}/password`, v, { ifMatch: u.revision })}
            />
            {u.status === 'suspended' ? (
              <ActionButton
                label="Восстановить"
                fields={reason}
                refresh={refresh}
                onSubmit={(v) => post(`/identity/admin/users/${id}/restore`, v, { ifMatch: u.revision })}
              />
            ) : (
              <ActionButton
                label="Приостановить"
                variant="danger"
                fields={reason}
                refresh={refresh}
                onSubmit={(v) => post(`/identity/admin/users/${id}/suspend`, v, { ifMatch: u.revision })}
              />
            )}
            <ActionButton
              label="Завершить сеансы"
              fields={reason}
              onSubmit={(v) => post(`/identity/admin/users/${id}/revoke-sessions`, v)}
            />
          </>
        )
      }
    >
      {u && (
        <Details
          items={[
            ['E-mail', u.email || '—'],
            ['Логин', u.login ?? '—'],
            ['Статус', statusLabel[u.status]],
            ['Роли', u.roles.map((r) => r.name).join(', ') || '—'],
          ]}
        />
      )}
    </Modal>
  );
}

const scopeLabel: Record<string, string> = { company: 'Роль компании', platform: 'Роль платформы' };

export function RolesPage() {
  const q = useData(['admin-roles'], () => list<Role>('/identity/admin/roles'));
  const perms = useData(['admin-permissions'], () => list<Permission>('/identity/admin/permissions'));
  const permOptions = (scope: string) =>
    (perms.data ?? []).filter((p) => p.assignable && p.scope === scope).map((p): [string, string] => [p.key, p.key]);
  const fields = (scope: string, r?: Role): FieldSpec[] => [
    { name: 'name', label: 'Название', type: 'text', required: true, initial: r?.name ?? '' },
    ...(scope === 'company'
      ? [
          {
            name: 'companyKind',
            label: 'Тип компании (пусто — любой)',
            type: 'select',
            options: Object.entries(kindLabel),
            initial: r?.companyKind ?? '',
          } as FieldSpec,
        ]
      : []),
    {
      name: 'permissionKeys',
      label: 'Разрешения',
      type: 'multiselect',
      options: permOptions(scope),
      initial: r?.permissionKeys ?? [],
    },
  ];
  const submit = (scope: string, v: Record<string, unknown>) => ({ ...v, scope, companyKind: v.companyKind || '' });
  return (
    <Page
      title="Роли и разрешения"
      subtitle="Роль — готовый набор разрешений. Роли компании администратор компании назначает своим сотрудникам; роли платформы — сотрудникам JustixAuto."
      actions={
        <>
          <ActionButton
            label="+ Роль компании"
            variant="primary"
            size="wide"
            fields={fields('company')}
            refresh={[['admin-roles']]}
            onSubmit={(v) => post('/identity/admin/roles', submit('company', v))}
          />
          <ActionButton
            label="+ Роль платформы"
            size="wide"
            fields={fields('platform')}
            refresh={[['admin-roles']]}
            onSubmit={(v) => post('/identity/admin/roles', submit('platform', v))}
          />
        </>
      }
    >
      <Panel>
        <Table
          rows={q.data}
          loading={q.isLoading}
          error={q.error}
          rowKey={(r) => r.id}
          columns={[
            {
              title: 'Роль',
              render: (r) => <Cell main={r.name} sub={r.system ? 'Встроенная' : 'Подготовленная'} />,
            },
            { title: 'Для кого', render: (r) => scopeLabel[r.scope] ?? r.scope },
            {
              title: 'Тип компании',
              render: (r) => (r.scope === 'company' ? (r.companyKind ? kindLabel[r.companyKind] : 'Любой') : '—'),
            },
            {
              title: 'Разрешения',
              render: (r) => <span title={r.permissionKeys.join(', ')}>{r.permissionKeys.length}</span>,
            },
            {
              title: '',
              render: (r) =>
                r.system ? (
                  <ActionButton
                    small
                    label="Просмотреть"
                    fields={[]}
                    submitLabel="Закрыть"
                    intro={<p>{r.permissionKeys.join(', ') || 'Права платформы'}</p>}
                    onSubmit={async () => undefined}
                  />
                ) : (
                  <ActionButton
                    small
                    label="Изменить"
                    size="wide"
                    fields={fields(r.scope, r)}
                    refresh={[['admin-roles']]}
                    onSubmit={(v) =>
                      patch(`/identity/admin/roles/${r.id}`, submit(r.scope, v), { ifMatch: r.revision })
                    }
                  />
                ),
            },
          ]}
        />
      </Panel>
      <div className="admin-note">
        Встроенные роли не изменяются. «Company administrator» получает все разрешения компании и может добавлять её
        сотрудников.
      </div>
    </Page>
  );
}

const auditLabel: Record<string, string> = {
  'company.created': 'Компания создана',
  'company.activate': 'Доступ компании активирован',
  'company.suspend': 'Доступ компании приостановлен',
  'company.restore': 'Доступ компании восстановлен',
  'company.provider_provisioned': 'Подключена организация',
  'user.mfa_enrolled': 'Включена двухфакторная защита',
  'user.password_changed': 'Пароль изменён пользователем',
  'company.updated': 'Реквизиты изменены',
  'company.activated': 'Доступ компании активирован',
  'company.suspended': 'Доступ компании приостановлен',
  'company.restored': 'Доступ компании восстановлен',
  'company.deleted': 'Компания удалена',
  'branch.created': 'Филиал создан',
  'branch.updated': 'Филиал изменён',
  'user.created': 'Пользователь создан',
  'user.updated': 'Пользователь изменён',
  'user.suspended': 'Пользователь приостановлен',
  'user.restored': 'Пользователь восстановлен',
  'user.password_set': 'Пароль задан администратором',
  'user.sessions_revoked': 'Сеансы завершены',
  'membership.granted': 'Доступ к компании выдан',
  'membership.revoked': 'Доступ к компании отозван',
  'membership.branch_access_changed': 'Доступ к филиалам изменён',
  'role.created': 'Роль создана',
  'role.updated': 'Роль изменена',
  'user.bootstrapped': 'Первый администратор платформы',
  'mfa.enrolled': 'Включена двухфакторная защита',
  'password.changed': 'Пароль изменён',
};

function AuditTable({
  rows,
  loading,
  error,
  users,
  companies,
}: {
  rows: AuditEvent[] | undefined;
  loading: boolean;
  error: unknown;
  users: User[];
  companies: Company[];
}) {
  return (
    <Table
      rows={rows}
      loading={loading}
      error={error}
      rowKey={(e) => e.id}
      empty="Действий пока нет"
      columns={[
        { title: 'Время', render: (e) => dateTime(e.occurredAt) },
        {
          title: 'Сотрудник',
          render: (e) =>
            users.find((u) => u.id === e.actorId)?.displayName ?? (e.actorId ? e.actorId.slice(0, 8) : 'Система'),
        },
        {
          title: 'Действие',
          render: (e) => (
            <Cell main={auditLabel[e.action] ?? e.action} sub={e.reason ? `Основание: ${e.reason}` : undefined} />
          ),
        },
        { title: 'Компания', render: (e) => companies.find((c) => c.id === e.companyId)?.name ?? '—' },
      ]}
    />
  );
}

export function AuditPage() {
  const q = useData(['admin-audit'], () => list<AuditEvent>('/identity/admin/audit?limit=100'));
  const users = useData(['admin-users'], () => list<User>('/identity/admin/users?limit=200'));
  const companies = useData(['admin-companies', 'all'], () => list<Company>('/identity/admin/companies?limit=200'));
  return (
    <Page title="Журнал действий" subtitle="История изменений без возможности редактирования">
      <Panel>
        <AuditTable
          rows={q.data}
          loading={q.isLoading}
          error={q.error}
          users={users.data ?? []}
          companies={companies.data ?? []}
        />
      </Panel>
    </Page>
  );
}
