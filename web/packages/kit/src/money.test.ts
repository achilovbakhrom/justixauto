import { describe, expect, it } from 'vitest';
import { minorToMajor, money, toMinor } from './ui';

describe('money helpers', () => {
  it('parses decimal input to minor units without floats', () => {
    expect(toMinor('1 234,5')).toBe('123450');
    expect(toMinor('0.07')).toBe('7');
    expect(toMinor('1.234')).toBeNull();
    expect(toMinor('-1')).toBeNull();
  });
  it('round-trips minor units', () => {
    expect(minorToMajor('123450')).toBe('1234.50');
    expect(toMinor(minorToMajor('9007199254740993123'))).toBe('9007199254740993123');
  });
  it('formats money with its currency', () => {
    expect(money({ amountMinor: '100', currency: 'USD' })).toContain('USD');
    expect(money(undefined)).toBe('—');
  });
});

describe('error messages', () => {
  it('translates known API codes', async () => {
    const { errorMessages } = await import('./messages');
    expect(errorMessages.stale_revision).toMatch(/Обновите/);
  });
});
