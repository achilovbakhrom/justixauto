/**
 * Demo country/region catalogue for the company-fields combobox (docs/justix-auto/mocks/company-fields.js).
 * Country autocomplete allows free text; the catalogue never limits the product's geography
 * (docs/justix-auto/business-logic.md:110).
 */
export const geoCatalogue: Record<string, string[]> = {
  Узбекистан: [
    'Ташкент',
    'Республика Каракалпакстан',
    'Андижанская область',
    'Бухарская область',
    'Джизакская область',
    'Кашкадарьинская область',
    'Навоийская область',
    'Наманганская область',
    'Самаркандская область',
    'Сурхандарьинская область',
    'Сырдарьинская область',
    'Ташкентская область',
    'Ферганская область',
    'Хорезмская область',
  ],
  Казахстан: ['Астана', 'Алматы', 'Шымкент', 'Алматинская область'],
  Китай: ['Пекин', 'Шанхай', 'Гуандун', 'Гуанчжоу'],
  Кыргызстан: ['Бишкек', 'Ош', 'Чуйская область'],
  Таджикистан: ['Душанбе', 'Худжанд', 'Согдийская область'],
};

/** Catalogue country names, in declaration order. */
export const countries = (): string[] => Object.keys(geoCatalogue);

/**
 * Matches free text against the catalogue case-insensitively, ignoring
 * surrounding whitespace. Returns undefined for text outside the demo
 * catalogue (kept as free text; the catalogue does not limit geography).
 */
export function canonicalCountry(input: string): string | undefined {
  const needle = input.trim().toLowerCase();
  if (!needle) return undefined;
  return countries().find((c) => c.toLowerCase() === needle);
}

/** Regions of a (possibly non-canonical) country string; [] if unknown or empty. */
export function regionsFor(country: string): string[] {
  const canonical = canonicalCountry(country);
  return canonical ? geoCatalogue[canonical]! : [];
}
