import { createElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CompaniesPage } from './pages';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const company = {
  id: 'company-1',
  kind: 'seller',
  name: 'Авто плюс',
  legalName: 'АО Старое имя',
  country: { label: 'Казахстан' },
  region: { label: 'Алматы' },
  registration: '12345',
  email: 'old@example.test',
  address: 'Алматы',
  phone: '+7 700 000 00 00',
  access: 'draft',
  accessReason: '',
  revision: '7',
};

function renderCompanies(kind: 'seller' | 'bank') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    createElement(
      QueryClientProvider,
      { client },
      createElement(MemoryRouter, null, createElement(CompaniesPage, { kind })),
    ),
  );
}

const response = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

function fillCreationForm() {
  fireEvent.change(screen.getByLabelText('Название компании'), { target: { value: 'Новая компания' } });
  fireEvent.change(screen.getByLabelText('Страна'), { target: { value: 'Казахстан' } });
  fireEvent.change(screen.getByLabelText('Электронная почта'), { target: { value: 'admin@example.test' } });
  fireEvent.change(screen.getByLabelText('Логин'), { target: { value: 'first-admin' } });
  fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: 'long-enough-password' } });
  fireEvent.change(screen.getByLabelText('Повторите пароль'), { target: { value: 'long-enough-password' } });
}

describe('company onboarding form', () => {
  it.each([
    ['seller', '/identity/admin/seller-companies'],
    ['bank', '/identity/admin/provider-companies'],
  ] as const)('submits the one rendered email to both API objects for %s', async (kind, endpoint) => {
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('/admin/companies')) return response({ items: [] });
      if (String(input).includes(endpoint)) return response({ data: {}, revision: '1' });
      throw new Error(`Unexpected request ${String(input)} ${init?.method}`);
    });
    vi.stubGlobal('fetch', fetch);
    renderCompanies(kind);

    fireEvent.click(
      await screen.findByRole('button', { name: kind === 'seller' ? '+ Добавить компанию' : '+ Подключить: Банк' }),
    );
    expect(screen.getAllByRole('group').map((group) => group.querySelector('legend')?.textContent)).toEqual([
      'Информация о компании',
      'Адрес',
      'Контакты',
      'Данные для входа',
    ]);
    expect(screen.getAllByLabelText('Электронная почта')).toHaveLength(1);
    fillCreationForm();
    fireEvent.click(screen.getByRole('button', { name: 'Создать компанию' }));

    await waitFor(() => expect(fetch).toHaveBeenCalledWith(expect.stringContaining(endpoint), expect.any(Object)));
    const [, init] = fetch.mock.calls.find(([input]) => String(input).includes(endpoint))!;
    const body = JSON.parse(String(init?.body));
    expect(body.company).toMatchObject({ name: 'Новая компания', email: 'admin@example.test', legalName: '' });
    expect(body.firstAdmin).toMatchObject({ email: 'admin@example.test', login: 'first-admin' });
    expect(body.firstAdmin).not.toHaveProperty('adminEmail');
    if (kind === 'bank') expect(body.kind).toBe('bank');
  });

  it('only requires the company name, admin login and password to create a seller company', async () => {
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('/admin/companies')) return response({ items: [] });
      if (String(input).includes('/admin/seller-companies')) return response({ data: {}, revision: '1' });
      throw new Error(`Unexpected request ${String(input)} ${init?.method}`);
    });
    vi.stubGlobal('fetch', fetch);
    renderCompanies('seller');

    fireEvent.click(await screen.findByRole('button', { name: '+ Добавить компанию' }));
    expect(screen.queryByLabelText('Регистрационный номер (ИНН/БИН)')).toBeNull();

    fireEvent.change(screen.getByLabelText('Название компании'), { target: { value: 'Минимальная компания' } });
    fireEvent.change(screen.getByLabelText('Логин'), { target: { value: 'bare-admin' } });
    fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: 'long-enough-password' } });
    fireEvent.change(screen.getByLabelText('Повторите пароль'), { target: { value: 'long-enough-password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Создать компанию' }));

    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/admin/seller-companies'), expect.any(Object)),
    );
    const [, init] = fetch.mock.calls.find(([input]) => String(input).includes('/admin/seller-companies'))!;
    const body = JSON.parse(String(init?.body));
    expect(body.company).toMatchObject({ name: 'Минимальная компания', registration: '' });
    expect(body.firstAdmin).toMatchObject({ login: 'bare-admin' });
  });

  it('hides the registration sub-line when a company has no registration number', async () => {
    const noRegistration = { ...company, registration: '' };
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('/admin/companies')) return response({ items: [noRegistration] });
      throw new Error(`Unexpected request ${String(input)} ${init?.method}`);
    });
    vi.stubGlobal('fetch', fetch);
    renderCompanies('seller');

    await screen.findByText('Авто плюс');
    expect(screen.queryByText('Реализация')?.textContent).toBe('Реализация');
    expect(screen.queryByText(/·\s*Реализация/)).toBeNull();
  });

  it('keeps the fetched legal name and revision when editing displayed requisites', async () => {
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/admin/companies')) return response({ items: [company] });
      if (url.endsWith('/identity/companies/company-1')) {
        if (init?.method === 'PATCH') return response({ data: company, revision: '8' });
        return response({ data: company, revision: '7' });
      }
      throw new Error(`Unexpected request ${url} ${init?.method}`);
    });
    vi.stubGlobal('fetch', fetch);
    renderCompanies('seller');

    fireEvent.click(await screen.findByText('Авто плюс'));
    fireEvent.click(await screen.findByRole('button', { name: 'Изменить реквизиты' }));
    const dialog = await screen.findByRole('dialog', { name: 'Изменить реквизиты' });
    fireEvent.change(within(dialog).getByLabelText('Название компании'), { target: { value: 'Новое имя' } });
    fireEvent.change(within(dialog).getByLabelText('Электронная почта'), { target: { value: 'new@example.test' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Изменить реквизиты' }));

    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/identity/companies/company-1'), expect.any(Object)),
    );
    const [, init] = fetch.mock.calls.find(([, request]) => request?.method === 'PATCH')!;
    expect(JSON.parse(String(init?.body))).toMatchObject({
      name: 'Новое имя',
      email: 'new@example.test',
      legalName: 'АО Старое имя',
      registration: '12345',
    });
    expect(new Headers(init?.headers).get('If-Match')).toBe('"7"');
  });
});
