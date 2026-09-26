import { describe, expect, it } from 'vitest';
import { permissionLabel, permissionOptions } from './permissions';

describe('permission labels', () => {
  it('names known keys in Russian and keeps unknown keys as is', () => {
    expect(permissionLabel('retail.read')).toBe('Продажи: просмотр');
    expect(permissionLabel('future.module.key')).toBe('future.module.key');
  });

  it('builds picker options sorted by name', () => {
    expect(permissionOptions(['retail.read', 'inventory.read'])).toEqual([
      ['retail.read', 'Продажи: просмотр'],
      ['inventory.read', 'Склад: просмотр'],
    ]);
  });
});
