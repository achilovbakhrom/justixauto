import { describe, expect, it } from 'vitest';
import { canonicalCountry, countries, geoCatalogue, regionsFor } from './geo';

describe('geo catalogue (business-logic.md:110)', () => {
  it('lists the demo catalogue countries without limiting product geography', () => {
    expect(countries()).toEqual(['Узбекистан', 'Казахстан', 'Китай', 'Кыргызстан', 'Таджикистан']);
  });

  it('matches catalogue spelling case-insensitively, trimming whitespace', () => {
    expect(canonicalCountry('казахстан')).toBe('Казахстан');
    expect(canonicalCountry('  УзБекИстан  ')).toBe('Узбекистан');
    expect(canonicalCountry('Казахстан')).toBe('Казахстан');
  });

  it('accepts free text outside the demo catalogue (country autocomplete stays free-form)', () => {
    expect(canonicalCountry('Германия')).toBeUndefined();
    expect(canonicalCountry('')).toBeUndefined();
    expect(canonicalCountry('   ')).toBeUndefined();
  });

  it('returns the exact catalogue regions for a canonical or loosely-typed country', () => {
    expect(regionsFor('Кыргызстан')).toEqual(geoCatalogue['Кыргызстан']);
    expect(regionsFor('кыргызстан')).toEqual(geoCatalogue['Кыргызстан']);
  });

  it('has no regions for an unknown or empty country', () => {
    expect(regionsFor('Германия')).toEqual([]);
    expect(regionsFor('')).toEqual([]);
  });
});
