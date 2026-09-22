import { describe, expect, it } from 'vitest';
import { minorToMajor } from '@justixauto/kit';

describe('realization', () => {
  it('formats minor units without floating point', () => {
    expect(minorToMajor('123456')).toBe('1234.56');
    expect(minorToMajor('5')).toBe('0.05');
  });
});
