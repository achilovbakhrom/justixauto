// @vitest-environment jsdom
import { createElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CompanyEmployees, PERM_COMPANY_USERS } from './company-employees';

const session = {
  view: { user: { id: 'admin-1' } },
  company: { id: 'c-1', kind: 'seller' },
  can: (p: string) => p === PERM_COMPANY_USERS,
};
vi.mock('./session', () => ({ useSession: () => session }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  session.can = (p: string) => p === PERM_COMPANY_USERS;
});

const response = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const employees = [
  { id: 'admin-1', displayName: 'Я админ', email: '', login: 'boss', status: 'active', roles: [], revision: '1' },
  { id: 'u-2', displayName: 'Иван', email: '', login: 'ivan', status: 'active', roles: [], revision: '3' },
];

function stubApi(onPost?: (url: string, body: unknown, init?: RequestInit) => void) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST' || init?.method === 'PATCH') {
      onPost?.(url, JSON.parse(String(init.body)), init);
      return response({ data: employees[1], revision: '4' });
    }
    if (url.endsWith('/identity/companies/c-1/users')) return response({ items: employees });
    if (url.endsWith('/identity/companies/c-1/roles')) {
      return response({ items: [{ id: 'r-sales', name: 'Менеджер продаж', permissionKeys: [] }] });
    }
    throw new Error(`Unexpected request ${url} ${init?.method}`);
  });
  vi.stubGlobal('fetch', fetch);
  return fetch;
}

function renderEmployees() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client }, createElement(CompanyEmployees)));
}

describe('company employees', () => {
  it('adds an employee with login, temporary password and a prepared role', async () => {
    let posted: { url: string; body: unknown } | undefined;
    stubApi((url, body) => (posted = { url, body }));
    renderEmployees();
    await screen.findByText('Иван');

    fireEvent.click(screen.getByRole('button', { name: '+ Добавить сотрудника' }));
    const dialog = await screen.findByRole('dialog', { name: 'Новый сотрудник' });
    fireEvent.change(within(dialog).getByLabelText('Имя'), { target: { value: 'Ольга' } });
    fireEvent.change(within(dialog).getByLabelText('Логин'), { target: { value: 'olga' } });
    fireEvent.change(within(dialog).getByLabelText('Временный пароль'), {
      target: { value: 'long-enough-password' },
    });
    fireEvent.click(await within(dialog).findByText('Менеджер продаж'));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Добавить' }));

    await waitFor(() => expect(posted).toBeDefined());
    expect(posted!.url).toContain('/identity/companies/c-1/users');
    expect(posted!.body).toMatchObject({
      displayName: 'Ольга',
      login: 'olga',
      password: 'long-enough-password',
      roleIds: ['r-sales'],
    });
  });

  it('does not offer reset or suspend for the admin themselves', async () => {
    stubApi();
    renderEmployees();
    const own = (await screen.findByText('Я админ')).closest('tr')!;
    const other = screen.getByText('Иван').closest('tr')!;
    expect(within(own).queryByRole('button', { name: 'Приостановить' })).toBeNull();
    expect(within(other).getByRole('button', { name: 'Приостановить' })).toBeTruthy();
  });

  it('shows a notice instead of the list without company.users.manage', () => {
    const fetch = stubApi();
    session.can = () => false;
    renderEmployees();
    expect(screen.getByText(/добавляет её администратор/)).toBeTruthy();
    expect(fetch).not.toHaveBeenCalled();
  });
});
