import { describe, expect, it } from 'vitest';
import { programInput } from './programs';

describe('programInput', () => {
  it('converts percents to basis points and parses terms', () => {
    const r = programInput({ name: 'A', currency: 'USD', markup: '12,5', minDown: '20', terms: '12, 24,36' });
    expect(r.terms).toEqual({ markupBps: 1250, minDownPaymentBps: 2000, termMonths: [12, 24, 36] });
    expect(r.eligibility).toEqual({ minPriceMinor: '', maxPriceMinor: '' });
  });
});
