// @vitest-environment jsdom
import { createElement } from 'react';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './http';
import { FormDialog } from './ui';
import type { FieldSpec } from './ui';

afterEach(cleanup);

const groupedFields: FieldSpec[] = [
  { name: 'name', label: 'Название компании', type: 'text', required: true, group: 'Информация о компании' },
  { name: 'email', label: 'Электронная почта', type: 'email', required: true, group: 'Контакты' },
  { name: 'login', label: 'Логин', type: 'text', required: true, group: 'Данные для входа' },
];

describe('FormDialog grouping and server errors', () => {
  it('keeps grouped fields in one flat submission payload', async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    render(
      createElement(FormDialog, { title: 'Новая компания', fields: groupedFields, onSubmit: submit, onClose: vi.fn() }),
    );

    expect(screen.getAllByRole('group').map((group) => group.querySelector('legend')?.textContent)).toEqual([
      'Информация о компании',
      'Контакты',
      'Данные для входа',
    ]);
    fireEvent.change(screen.getByLabelText('Название компании'), { target: { value: 'Авто плюс' } });
    fireEvent.change(screen.getByLabelText('Электронная почта'), { target: { value: 'admin@example.test' } });
    fireEvent.change(screen.getByLabelText('Логин'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }));

    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({ name: 'Авто плюс', email: 'admin@example.test', login: 'admin' }),
    );
  });

  it('shows both nested email errors beside the one visible email field', async () => {
    const submit = vi.fn().mockRejectedValue(
      new ApiError(422, 'validation', 'Ошибка проверки', {
        'company.email': 'Некорректный адрес компании',
        'firstAdmin.email': 'Адрес администратора уже занят',
        'company.legalName': 'Скрытое поле',
      }),
    );
    render(
      createElement(FormDialog, { title: 'Новая компания', fields: groupedFields, onSubmit: submit, onClose: vi.fn() }),
    );

    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }));

    expect(await screen.findByText('Некорректный адрес компании')).toBeTruthy();
    expect(screen.getByText('Адрес администратора уже занят')).toBeTruthy();
    expect(screen.getByText(/company\.legalName: Скрытое поле/)).toBeTruthy();
    expect(screen.getAllByLabelText('Электронная почта')).toHaveLength(1);
  });

  it('does not repeat an identical message returned for both email paths', async () => {
    const submit = vi.fn().mockRejectedValue(
      new ApiError(422, 'validation', 'Ошибка проверки', {
        'company.email': 'Некорректный адрес',
        'firstAdmin.email': 'Некорректный адрес',
      }),
    );
    render(
      createElement(FormDialog, { title: 'Новая компания', fields: groupedFields, onSubmit: submit, onClose: vi.fn() }),
    );

    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }));
    await screen.findByText('Некорректный адрес');
    expect(document.querySelectorAll('.field-error')).toHaveLength(1);
  });

  it('does not clear a region when an unchanged canonical country blurs', () => {
    const fields: FieldSpec[] = [
      { name: 'country', label: 'Страна', type: 'combobox', initial: 'Казахстан', canonicalize: (value) => value },
      { name: 'region', label: 'Регион', type: 'combobox', initial: 'Алматы', dependsOn: 'country' },
    ];
    render(
      createElement(FormDialog, {
        title: 'Компания',
        fields,
        onSubmit: vi.fn().mockResolvedValue(undefined),
        onClose: vi.fn(),
      }),
    );

    fireEvent.blur(screen.getByLabelText('Страна'));
    expect((screen.getByLabelText('Регион') as HTMLInputElement).value).toBe('Алматы');
  });

  it('submits once while the request is pending', async () => {
    let resolve!: () => void;
    const submit = vi.fn(() => new Promise<void>((done) => (resolve = done)));
    render(createElement(FormDialog, { title: 'Компания', fields: groupedFields, onSubmit: submit, onClose: vi.fn() }));

    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }));
    fireEvent.submit(document.querySelector('form')!);
    expect(submit).toHaveBeenCalledTimes(1);
    resolve();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Сохранить' })).toBeTruthy());
  });

  it('keeps ungrouped money and selection fields flat', async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    const fields: FieldSpec[] = [
      { name: 'amount', label: 'Сумма', type: 'money', required: true, currency: 'UZS' },
      { name: 'kind', label: 'Тип', type: 'select', options: [['bank', 'Банк']] },
    ];
    render(createElement(FormDialog, { title: 'Проверка', fields, onSubmit: submit, onClose: vi.fn() }));

    fireEvent.change(screen.getByLabelText('Сумма'), { target: { value: '1500.50' } });
    fireEvent.change(screen.getByLabelText('Тип'), { target: { value: 'bank' } });
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({ amount: { amountMinor: '150050', currency: 'UZS' }, kind: 'bank' }),
    );
  });
});
