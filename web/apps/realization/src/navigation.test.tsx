import { describe, expect, it } from 'vitest';
import { navigation } from './navigation';

describe('navigation', () => {
  it('never uses the URL prefixes of the other cabinets', () => {
    // The server gives /finance/, /insurance/ and /admin/ to the other apps, so a
    // reload or shared link of such a seller page would open the wrong cabinet.
    for (const item of navigation) {
      expect(item.to).not.toMatch(/^\/(finance|insurance|admin)(\/|$)/);
    }
  });
});
