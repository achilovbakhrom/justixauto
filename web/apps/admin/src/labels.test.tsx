import { describe, expect, it } from 'vitest';
import { kindLabel } from './labels';

describe('admin', () => {
  it('labels every company kind of the backend', () => {
    expect(Object.keys(kindLabel).sort()).toEqual(['bank', 'insurance', 'mfo', 'seller']);
  });
});
