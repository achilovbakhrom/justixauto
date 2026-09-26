import { createElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { UsersPage } from './pages';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const response = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const companies = [
  { id: 'c-1', name: 'Авто плюс', kind: 'seller', access: 'active', revision: '1' },
  { id: 'c-2', name: 'Банк Один', kind: 'bank', access: 'active', revision: '1' },
];
const user = (id: string, name: string, companyIds: string[]) => ({
  id,
  displayName: name,
  email: '',
  login: id,
  status: 'active',
  roles: [],
  companyIds,
  revision: '1',
});

function stubApi(onCreate?: (body: unknown) => void) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes('/identity/admin/users') && init?.method === 'POST') {
      onCreate?.(JSON.parse(String(init.body)));
      return response({ data: user('new', 'Новый', ['c-1']), revision: '1' });
    }
    if (url.includes('/identity/admin/users')) {
      return response({ items: [user('ivan', 'Иван', ['c-1']), user('olga', 'Ольга', ['c-2'])] });
    }
    if (url.includes('/identity/admin/roles')) return response({ items: [] });
    if (url.includes('/identity/admin/companies')) return response({ items: companies });
    throw new Error(`Unexpected request ${url} ${init?.method}`);
  });
  vi.stubGlobal('fetch', fetch);
}

function renderUsers() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    createElement(QueryClientProvider, { client }, createElement(MemoryRouter, null, createElement(UsersPage))),
  );
}

describe('users page', () => {
  it('shows the companies each user belongs to and filters by company', async () => {
    stubApi();
    renderUsers();

    const ivan = (await screen.findByText('Иван')).closest('tr')!;
    await waitFor(() => expect(within(ivan).getByText('Авто плюс')).toBeTruthy());

    fireEvent.change(screen.getByDisplayValue('Все компании'), { target: { value: 'c-2' } });
    await waitFor(() => expect(screen.queryByText('Иван')).toBeNull());
    expect(screen.getByText('Ольга')).toBeTruthy();
  });

  it('creates a company employee with login and temporary password and no email', async () => {
    let body: unknown;
    stubApi((b) => (body = b));
    renderUsers();
    await screen.findByText('Иван');

    fireEvent.click(screen.getByRole('button', { name: '+ Добавить пользователя' }));
    const dialog = await screen.findByRole('dialog', { name: '+ Добавить пользователя' });
    fireEvent.change(within(dialog).getByLabelText('Имя'), { target: { value: 'Новый' } });
    fireEvent.change(within(dialog).getByLabelText('Компания'), { target: { value: 'c-1' } });
    fireEvent.change(within(dialog).getByLabelText('Логин'), { target: { value: 'new-seller' } });
    fireEvent.change(within(dialog).getByLabelText('Временный пароль (не менее 12 символов)'), {
      target: { value: 'long-enough-password' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: '+ Добавить пользователя' }));

    await waitFor(() => expect(body).toBeDefined());
    expect(body).toMatchObject({
      displayName: 'Новый',
      companyId: 'c-1',
      login: 'new-seller',
      password: 'long-enough-password',
    });
  });
});
