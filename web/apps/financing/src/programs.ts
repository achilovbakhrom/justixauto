import { minorToMajor } from '@justixauto/kit';
import type { FieldSpec, ProgramVersion } from '@justixauto/kit';

export const programLabel: Record<string, string> = { draft: 'Черновик', published: 'Опубликована', withdrawn: 'Снята' };

/** Form fields of one program version (fixed-markup policy). */
export function programFields(v?: ProgramVersion): FieldSpec[] {
  return [
    { name: 'name', label: 'Название', type: 'text', required: true, ...(v ? { initial: v.name } : {}) },
    { name: 'currency', label: 'Валюта', type: 'select', required: true, options: [['USD', 'USD'], ['UZS', 'UZS'], ['EUR', 'EUR'], ['RUB', 'RUB']], initial: v?.currency ?? 'USD' },
    { name: 'markup', label: 'Наценка, %', type: 'number', required: true, ...(v ? { initial: String(v.terms.markupBps / 100) } : {}) },
    { name: 'minDown', label: 'Минимальный первый взнос, %', type: 'number', required: true, ...(v ? { initial: String(v.terms.minDownPaymentBps / 100) } : {}) },
    { name: 'terms', label: 'Сроки, месяцев (через запятую)', type: 'text', required: true, ...(v ? { initial: v.terms.termMonths.join(', ') } : {}) },
    { name: 'minPrice', label: 'Мин. цена автомобиля (пусто — без ограничения)', type: 'money', ...(v?.eligibility.minPriceMinor ? { initial: minorToMajor(v.eligibility.minPriceMinor) } : {}) },
    { name: 'maxPrice', label: 'Макс. цена автомобиля', type: 'money', ...(v?.eligibility.maxPriceMinor ? { initial: minorToMajor(v.eligibility.maxPriceMinor) } : {}) },
  ];
}

const bps = (s: unknown) => Math.round(Number(String(s).replace(',', '.')) * 100);
const minor = (m: unknown) => (m as { amountMinor?: string } | undefined)?.amountMinor ?? '';

export function programInput(v: Record<string, unknown>) {
  return {
    name: v.name, currency: v.currency,
    terms: { markupBps: bps(v.markup), minDownPaymentBps: bps(v.minDown), termMonths: String(v.terms).split(',').map((t) => Number(t.trim())).filter((t) => t > 0) },
    eligibility: { minPriceMinor: minor(v.minPrice), maxPriceMinor: minor(v.maxPrice) },
    calculationPolicyId: 'fixed-markup', calculationPolicyVersion: 1,
  };
}

