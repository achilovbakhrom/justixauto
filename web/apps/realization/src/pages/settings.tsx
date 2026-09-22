import { ActionButton, Details, Notice, Page, Panel, Table, get, patch, post, useData, useSession } from '@justixauto/kit';
import type { FieldSpec } from '@justixauto/kit';
import { useBranches } from '../data';
import type { Branch } from '../data';

interface Company { id: string; name: string; legalName: string; country: { key?: string; label: string }; region: { key?: string; label: string } | null; registration: string; email: string; address: string; phone: string; revision?: string }

const branchFields = (b?: Branch): FieldSpec[] => [
  { name: 'name', label: 'Название', type: 'text', required: true, ...(b ? { initial: b.name } : {}) },
  { name: 'address', label: 'Адрес', type: 'text', required: true, ...(b ? { initial: b.address } : {}) },
];

export function SettingsPage() {
  const s = useSession();
  const id = s.company?.id ?? '';
  const company = useData(['company', id], () => get<Company>(`/identity/companies/${id}`), !!id);
  const branches = useBranches();
  const c = company.data?.data;
  const scope = s.view.context.branchScope;
  const refresh = [['company', id], ['branches']];
  return <Page title="Настройки компании">
    <Panel title="Реквизиты" padded actions={c && s.can('company.edit') && <ActionButton label="Изменить" refresh={refresh} fields={[
      { name: 'name', label: 'Название', type: 'text', required: true, initial: c.name },
      { name: 'legalName', label: 'Юридическое название', type: 'text', required: true, initial: c.legalName },
      { name: 'registration', label: 'Регистрационный номер', type: 'text', required: true, initial: c.registration },
      { name: 'email', label: 'Email', type: 'email', required: true, initial: c.email },
      { name: 'phone', label: 'Телефон', type: 'text', required: true, initial: c.phone },
      { name: 'address', label: 'Адрес', type: 'text', required: true, initial: c.address },
    ]} onSubmit={(v) => patch(`/identity/companies/${id}`, { ...v, country: c.country, region: c.region }, { ifMatch: company.data!.revision })} />}>
      {c && <Details items={[['Название', c.name], ['Юридическое название', c.legalName], ['Страна', c.country.label], ['Регион', c.region?.label ?? '—'],
        ['Рег. номер', c.registration], ['Email', c.email], ['Телефон', c.phone], ['Адрес', c.address]]} />}
    </Panel>
    <Panel title="Филиалы" actions={s.can('branches.create') && <ActionButton label="Новый филиал" variant="primary" refresh={refresh} fields={branchFields()}
      onSubmit={(v) => post(`/identity/companies/${id}/branches`, v)} />}>
      <Table rows={branches.data} loading={branches.isLoading} error={branches.error} rowKey={(b) => b.id} columns={[
        { title: 'Название', render: (b) => b.name }, { title: 'Адрес', render: (b) => b.address },
        { title: '', render: (b) => s.can('branches.edit') && <ActionButton label="Изменить" fields={branchFields(b)} refresh={refresh}
          onSubmit={(v) => patch(`/identity/companies/${id}/branches/${b.id}`, v, { ifMatch: b.revision })} /> },
      ]} />
    </Panel>
    <Panel title="Рабочие филиалы в этой сессии" padded actions={<ActionButton label="Выбрать" fields={[
      { name: 'all', label: 'Все доступные филиалы', type: 'checkbox', initial: scope.mode === 'ALL' },
      { name: 'branchIds', label: 'Или только эти', type: 'multiselect', options: (branches.data ?? []).map((b) => [b.id, b.name]), initial: scope.branchIds },
    ]} onSubmit={(v) => s.setBranchScope(v.all ? 'ALL' : 'SELECTED', v.all ? [] : v.branchIds as string[])} />}>
      <Notice>{scope.mode === 'ALL' ? 'Показаны данные всех доступных филиалов.'
        : `Выбрано: ${scope.branchIds.map((b) => branches.data?.find((x) => x.id === b)?.name ?? b).join(', ')}`}</Notice>
    </Panel>
  </Page>;
}
