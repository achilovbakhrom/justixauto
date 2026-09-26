// @vitest-environment jsdom
import { createElement } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { FormDialog } from './ui';
import type { FieldSpec } from './ui';
import { canonicalCountry, countries, regionsFor } from './geo';

afterEach(cleanup);

const fields: FieldSpec[] = [
  {
    name: 'country',
    label: 'Страна',
    type: 'combobox',
    required: true,
    options: countries(),
    canonicalize: canonicalCountry,
    placeholder: 'Выберите или найдите страну',
    ariaLabel: 'Показать страны',
  },
  {
    name: 'region',
    label: 'Регион',
    type: 'combobox',
    dependsOn: 'country',
    optionsFor: regionsFor,
    placeholder: 'Выберите или найдите регион',
    disabledPlaceholder: 'Сначала выберите страну',
    ariaLabel: 'Показать регионы',
  },
];

function renderForm() {
  return render(
    createElement(FormDialog, {
      title: 'Компания',
      fields,
      onSubmit: () => Promise.resolve(),
      onClose: () => {},
    }),
  );
}

describe('combobox field (business-logic.md:110)', () => {
  it('disables the region field with its placeholder until a country is entered', () => {
    renderForm();
    const region = screen.getByLabelText('Регион') as HTMLInputElement;
    expect(region.disabled).toBe(true);
    expect(region.placeholder).toBe('Сначала выберите страну');
    expect((screen.getByLabelText('Показать регионы') as HTMLButtonElement).disabled).toBe(true);
  });

  it('enables the region field once a country is entered and offers its regions', () => {
    renderForm();
    const country = screen.getByLabelText('Страна') as HTMLInputElement;
    fireEvent.change(country, { target: { value: 'Казахстан' } });
    const region = screen.getByLabelText('Регион') as HTMLInputElement;
    expect(region.disabled).toBe(false);
    expect(region.placeholder).toBe('Выберите или найдите регион');
  });

  it('clears the region value when the country text changes', () => {
    renderForm();
    const country = screen.getByLabelText('Страна') as HTMLInputElement;
    fireEvent.change(country, { target: { value: 'Казахстан' } });
    const region = screen.getByLabelText('Регион') as HTMLInputElement;
    fireEvent.change(region, { target: { value: 'Алматы' } });
    expect(region.value).toBe('Алматы');
    fireEvent.change(country, { target: { value: 'Китай' } });
    expect(region.value).toBe('');
  });

  it('canonicalizes a case-insensitive country match on blur', () => {
    renderForm();
    const country = screen.getByLabelText('Страна') as HTMLInputElement;
    fireEvent.change(country, { target: { value: 'казахстан' } });
    fireEvent.blur(country);
    expect(country.value).toBe('Казахстан');
  });

  it('accepts free text for a country outside the demo catalogue', () => {
    renderForm();
    const country = screen.getByLabelText('Страна') as HTMLInputElement;
    fireEvent.change(country, { target: { value: 'Германия' } });
    fireEvent.blur(country);
    expect(country.value).toBe('Германия');
  });
});
