import { list, patch, post } from './http';
import { useSession } from './session';
import { ActionButton, useData } from './shell';
import { Badge, Cell, Notice, Panel, Table } from './ui';
import type { FieldSpec } from './ui';

/** Permission that lets a company admin manage the company's employees. */
export const PERM_COMPANY_USERS = 'company.users.manage';

interface Employee {
  id: string;
  displayName: string;
  email: string;
  login: string | null;
  status: string;
  roles: { id: string; name: string }[];
  revision: string;
}
interface PreparedRole {
  id: string;
  name: string;
  permissionKeys: string[];
}

const statusLabel: Record<string, string> = { pending: 'Без пароля', active: 'Активен', suspended: 'Приостановлен' };
const reason: FieldSpec[] = [{ name: 'reason', label: 'Основание', type: 'textarea', required: true }];

/**
 * Employees of the signed-in user's company. A company admin (with
 * company.users.manage) adds employees with a login and temporary password and
 * assigns roles prepared by the JustixAuto platform admin; one user belongs
 * to one company.
 */
export function CompanyEmployees() {
  const s = useSession();
  const id = s.company?.id ?? '';
  const manage = !!id && s.can(PERM_COMPANY_USERS);
  const base = `/identity/companies/${id}`;
  const users = useData(['company-users', id], () => list<Employee>(`${base}/users`), manage);
  const roles = useData(['company-roles', id], () => list<PreparedRole>(`${base}/roles`), manage);
  const refresh = [['company-users', id]];
  const roleOptions = (roles.data ?? []).map((r): [string, string] => [r.id, r.name]);

  if (!manage) {
    return (
      <Notice>
        Сотрудников компании добавляет её администратор. Обратитесь к нему, если вам нужен доступ к другим разделам.
      </Notice>
    );
  }
  return (
    <Panel
      title="Сотрудники"
      actions={
        <ActionButton
          label="+ Добавить сотрудника"
          variant="primary"
          refresh={refresh}
          fields={[
            { name: 'displayName', label: 'Имя', type: 'text', required: true },
            { name: 'login', label: 'Логин', type: 'text', required: true },
            { name: 'password', label: 'Временный пароль (не менее 12 символов)', type: 'password', required: true },
            { name: 'email', label: 'E-mail', type: 'email' },
            { name: 'roleIds', label: 'Роли', type: 'multiselect', options: roleOptions },
          ]}
          intro={
            <p>
              Роли подготовлены администратором платформы. Сотрудник сменит временный пароль при первом входе; письма не
              отправляются.
            </p>
          }
          onSubmit={(v) => post(`${base}/users`, v)}
        />
      }
    >
      <Table
        rows={users.data}
        loading={users.isLoading}
        error={users.error}
        rowKey={(u) => u.id}
        empty="Сотрудников пока нет"
        columns={[
          {
            title: 'Сотрудник',
            render: (u) => <Cell main={u.displayName} sub={[u.login, u.email].filter(Boolean).join(' · ')} />,
          },
          { title: 'Роли', render: (u) => u.roles.map((r) => r.name).join(', ') || '—' },
          {
            title: 'Статус',
            render: (u) => (
              <Badge tone={u.status === 'active' ? 'success' : u.status === 'suspended' ? 'danger' : 'warning'}>
                {statusLabel[u.status] ?? u.status}
              </Badge>
            ),
          },
          {
            title: '',
            render: (u) => (
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                <ActionButton
                  small
                  label="Изменить"
                  refresh={refresh}
                  fields={[
                    { name: 'displayName', label: 'Имя', type: 'text', required: true, initial: u.displayName },
                    { name: 'email', label: 'E-mail', type: 'email', initial: u.email },
                    {
                      name: 'roleIds',
                      label: 'Роли',
                      type: 'multiselect',
                      options: roleOptions,
                      initial: u.roles.map((r) => r.id),
                    },
                  ]}
                  onSubmit={(v) => patch(`${base}/users/${u.id}`, v, { ifMatch: u.revision })}
                />
                {u.id !== s.view.user.id && (
                  <>
                    <ActionButton
                      small
                      label="Сбросить пароль"
                      refresh={refresh}
                      intro={<p>Сотрудник сменит временный пароль при первом входе.</p>}
                      fields={[
                        {
                          name: 'password',
                          label: 'Временный пароль (не менее 12 символов)',
                          type: 'password',
                          required: true,
                        },
                      ]}
                      onSubmit={(v) => post(`${base}/users/${u.id}/password`, v, { ifMatch: u.revision })}
                    />
                    {u.status === 'suspended' ? (
                      <ActionButton
                        small
                        label="Восстановить"
                        refresh={refresh}
                        fields={reason}
                        onSubmit={(v) => post(`${base}/users/${u.id}/restore`, v, { ifMatch: u.revision })}
                      />
                    ) : (
                      <ActionButton
                        small
                        label="Приостановить"
                        variant="danger"
                        refresh={refresh}
                        fields={reason}
                        onSubmit={(v) => post(`${base}/users/${u.id}/suspend`, v, { ifMatch: u.revision })}
                      />
                    )}
                  </>
                )}
              </div>
            ),
          },
        ]}
      />
    </Panel>
  );
}
