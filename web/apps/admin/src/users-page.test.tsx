import { createElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RolesPage, UsersPage } from './pages';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const response = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const roles = [
  {
    id: 'r-platform',
    name: 'Platform administrator',
    system: true,
    scope: 'platform',
    companyKind: null,
    permissionKeys: [],
    revision: '1',
  },
  {
    id: 'r-company',
    name: 'Company administrator',
    system: true,
    scope: 'company',
    companyKind: null,
    permissionKeys: [],
    revision: '1',
  },
  {
    id: 'r-sales',
    name: 'Менеджер продаж',
    system: false,
    scope: 'company',
    companyKind: 'seller',
    permissionKeys: ['retail.read'],
    revision: '1',
  },
];
const permissions = [
  { key: 'retail.read', scope: 'company', assignable: true },
  { key: 'platform.audit.read', scope: 'platform', assignable: true },
];

function stubApi(onPost?: (url: string, body: unknown) => void) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      onPost?.(url, JSON.parse(String(init.body)));
      return response({ data: {}, revision: '1' });
    }
    if (url.includes('/identity/admin/users')) {
      return response({
        items: [
          { id: 'u-1', displayName: 'Иван', email: '', login: 'ivan', status: 'active', roles: [], revision: '1' },
        ],
      });
    }
    if (url.includes('/identity/admin/roles')) return response({ items: roles });
    if (url.includes('/identity/admin/permissions')) return response({ items: permissions });
    throw new Error(`Unexpected request ${url} ${init?.method}`);
  });
  vi.stubGlobal('fetch', fetch);
}

function renderPage(page: typeof UsersPage) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client }, createElement(MemoryRouter, null, createElement(page))));
}

describe('platform staff page', () => {
  it('adds staff with platform roles only and no company', async () => {
    let body: unknown;
    stubApi((_, b) => (body = b));
    renderPage(UsersPage);
    await screen.findByText('Иван');

    fireEvent.click(screen.getByRole('button', { name: '+ Добавить сотрудника' }));
    const dialog = await screen.findByRole('dialog', { name: 'Новый сотрудник платформы' });
    expect(within(dialog).queryByLabelText('Компания')).toBeNull();
    expect(within(dialog).queryByText('Company administrator')).toBeNull();
    expect(within(dialog).getByText('Platform administrator')).toBeTruthy();

    fireEvent.change(within(dialog).getByLabelText('Имя'), { target: { value: 'Оператор' } });
    fireEvent.change(within(dialog).getByLabelText('Логин'), { target: { value: 'operator' } });
    fireEvent.change(within(dialog).getByLabelText('Временный пароль'), {
      target: { value: 'long-enough-password' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Добавить' }));
    await waitFor(() => expect(body).toMatchObject({ displayName: 'Оператор', login: 'operator' }));
  });
});

describe('roles page', () => {
  it('prepares a company role for one company type with company permissions only', async () => {
    let posted: { url: string; body: unknown } | undefined;
    stubApi((url, body) => (posted = { url, body }));
    renderPage(RolesPage);
    expect(await screen.findByText('Менеджер продаж')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: '+ Роль компании' }));
    const dialog = await screen.findByRole('dialog', { name: '+ Роль компании' });
    expect(within(dialog).queryByText('platform.audit.read')).toBeNull();
    fireEvent.change(within(dialog).getByLabelText('Название'), { target: { value: 'Кассир' } });
    fireEvent.change(within(dialog).getByLabelText('Тип компании (пусто — любой)'), { target: { value: 'seller' } });
    fireEvent.click(within(dialog).getByText('retail.read'));
    fireEvent.click(within(dialog).getByRole('button', { name: '+ Роль компании' }));

    await waitFor(() => expect(posted).toBeDefined());
    expect(posted!.url).toContain('/identity/admin/roles');
    expect(posted!.body).toMatchObject({
      name: 'Кассир',
      scope: 'company',
      companyKind: 'seller',
      permissionKeys: ['retail.read'],
    });
  });
});
