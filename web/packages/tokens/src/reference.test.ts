import { readFileSync } from 'node:fs';
import { afterAll, describe, expect, it } from 'vitest';

type Rule = { selector: string; media: string; declarations: Record<string, string> };

// A deliberately narrow reader for declaration provenance, not a CSS engine.
// Browser evidence separately checks the cascade, including inherited overrides.
function rules(css: string, media = ''): Rule[] {
  const input = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const result: Rule[] = [];
  let start = 0;
  while (start < input.length) {
    const open = input.indexOf('{', start);
    if (open < 0) break;
    let end = open + 1;
    let depth = 1;
    while (depth && end < input.length) {
      if (input[end] === '{') depth++;
      if (input[end] === '}') depth--;
      end++;
    }
    if (depth) throw new Error('Unbalanced reference CSS');
    const selector = input.slice(start, open).trim();
    const body = input.slice(open + 1, end - 1);
    if (selector.startsWith('@media')) {
      result.push(...rules(body, selector.replace(/\s/g, '')));
    } else if (!selector.startsWith('@')) {
      const declarations: Record<string, string> = {};
      for (const entry of body.split(';')) {
        const colon = entry.indexOf(':');
        if (colon >= 0) declarations[entry.slice(0, colon).trim()] = entry.slice(colon + 1).trim();
      }
      result.push({ selector, media, declarations });
    }
    start = end;
  }
  return result;
}

const reference = (path: string) =>
  readFileSync(new URL(`../../../../docs/justix-auto/mocks/${path}`, import.meta.url), 'utf8');
const extracted = readFileSync(new URL('./tokens.css', import.meta.url), 'utf8');
const tokens = rules(extracted);
const normalize = (value: string) => value.replace(/\s/g, '');
const query = (width: number) => `@media(max-width:${width}px)`;
function declaration(list: Rule[], selector: string, property: string, media = ''): string {
  const value = list
    .filter((rule) => rule.selector === selector && rule.media === media && property in rule.declarations)
    .at(-1)?.declarations[property];
  if (value === undefined) throw new Error(`Missing ${media} ${selector} { ${property} }`);
  return value;
}

describe('immutable HTML token provenance', () => {
  const covered = new Set<string>();
  const key = (selector: string, token: string, media: string) => `${media}|${selector}|${token}`;
  function compare(
    selector: string,
    token: string,
    source: string,
    anchor: string,
    property: string,
    width?: number,
    transform: (value: string) => string = (value) => value,
  ) {
    const media = width === undefined ? '' : query(width);
    const actual = declaration(tokens, selector, token, media);
    const expected = transform(declaration(rules(reference(source)), anchor, property, media));
    expect(normalize(actual), `${source}: ${media} ${anchor} ${property}`).toBe(normalize(expected));
    covered.add(key(selector, token, media));
  }

  it('preserves every common custom property and its root responsive override', () => {
    for (const rule of rules(reference('common.css')).filter((rule) => rule.selector === ':root')) {
      for (const [name, value] of Object.entries(rule.declarations)) {
        expect(normalize(declaration(tokens, ':root', name, rule.media))).toBe(normalize(value));
        covered.add(key(':root', name, rule.media));
      }
    }
  });

  it('extracts common typography, table geometry, focus and centered modal values', () => {
    const bindings: [string, string, string][] = [
      ['font-family', 'body', 'font-family'],
      ['font-size', 'body', 'font-size'],
      ['line-height', 'body', 'line-height'],
      ['sidebar-padding', '.sidebar', 'padding'],
      ['topbar-padding', '.topbar', 'padding'],
      ['page-padding', '.page', 'padding'],
      ['table-width', 'table', 'width'],
      ['table-layout', 'table', 'table-layout'],
      ['table-heading-height', 'th', 'height'],
      ['table-heading-padding', 'th', 'padding'],
      ['table-row-height', 'td', 'height'],
      ['table-cell-padding', 'td', 'padding'],
      ['dialog-width', '.modal', 'width'],
      ['dialog-compact-width', '.modal-compact', 'width'],
      ['dialog-wide-width', '.modal-wide', 'width'],
      ['dialog-max-height', '.modal', 'max-height'],
      ['dialog-radius', '.modal', 'border-radius'],
      ['dialog-backdrop', '.modal-scrim', 'background'],
      ['dialog-overlay-padding', '.modal-scrim', 'padding'],
      ['dialog-overlay-z-index', '.modal-scrim', 'z-index'],
      ['dialog-header-padding', '.modal-header', 'padding'],
      ['dialog-body-padding', '.modal-body', 'padding'],
      ['dialog-footer-padding', '.modal-footer', 'padding'],
      ['field-focus-outline', '.field input:focus,.field select:focus,.field textarea:focus', 'outline'],
    ];
    for (const [name, anchor, property] of bindings) compare(':root', `--${name}`, 'common.css', anchor, property);
    compare(':root', '--sidebar-padding', 'common.css', '.sidebar', 'padding-inline', 1180, (value) => `20px ${value}`);
    compare(':root', '--topbar-padding', 'common.css', '.topbar', 'padding-inline', 1180, (value) => `0 ${value}`);
    compare(':root', '--page-padding', 'common.css', '.page', 'padding', 900);
    compare(':root', '--dialog-overlay-padding', 'common.css', '.modal-scrim', 'padding', 900);
    compare(':root', '--dialog-wide-width', 'common.css', '.modal-wide', 'width', 900);
  });

  it('retains the dealer override and the financing-specific modal width', () => {
    compare('.dealer-shell', '--sidebar', 'styles.css', '.dealer-shell', '--sidebar');
    compare(
      '.finance-workspace',
      '--dialog-wide-width',
      'finance/workspace.css',
      '.finance-workspace .modal-wide',
      'width',
    );
  });

  it.each([
    {
      selector: '.ins-workspace',
      file: 'insurance.css',
      side: '.ins-sidebar',
      top: '.ins-account',
      table: '.ins-table',
      dialog: '.ins-dialog',
      shade: '.ins-overlay',
      header: '.ins-dialog header',
      footer: '.ins-dialog footer',
      nav: '.ins-sidebar button',
      narrow: 900,
      mobile: 600,
    },
    {
      selector: '.admin-shell',
      file: 'admin/admin.css',
      side: '.admin-sidebar',
      top: '.admin-topbar',
      table: '.admin-table',
      dialog: '.admin-modal',
      shade: '.admin-modal-shade',
      header: '.admin-modal header',
      footer: '.admin-modal footer',
      nav: '.admin-sidebar nav button',
      narrow: 850,
      mobile: 580,
    },
  ])('preserves distinct $selector values and actual responsive conditions', (app) => {
    const bindings: [string, string, string][] = [
      ['shell-columns', app.selector, 'grid-template-columns'],
      ['shell-display', app.selector, 'display'],
      ['sidebar-padding', app.side, 'padding'],
      ['topbar-padding', app.top, 'padding'],
      ['nav-text', app.nav, 'color'],
      ['nav-active-text', `${app.nav}.active`, 'color'],
      ['nav-active-background', `${app.nav}.active`, 'background'],
      ['table-layout', app.table, 'table-layout'],
      ['dialog-width', app.dialog, 'width'],
      ['dialog-max-height', app.dialog, 'max-height'],
      ['dialog-radius', app.dialog, 'border-radius'],
      ['dialog-backdrop', app.shade, 'background'],
      ['dialog-overlay-padding', app.shade, 'padding'],
      ['dialog-overlay-z-index', app.shade, 'z-index'],
      ['shadow-overlay', app.dialog, 'box-shadow'],
      ['dialog-header-padding', app.header, 'padding'],
      ['dialog-footer-padding', app.footer, 'padding'],
    ];
    for (const [name, anchor, property] of bindings) compare(app.selector, `--${name}`, app.file, anchor, property);
    compare(
      app.selector,
      '--shell-border',
      app.file,
      app.side,
      'border-right',
      undefined,
      (value) => value.split(' ').at(-1) ?? '',
    );
    compare(app.selector, '--shell-columns', app.file, app.selector, 'grid-template-columns', app.narrow);
    compare(app.selector, '--shell-display', app.file, app.selector, 'display', app.mobile);
    compare(app.selector, '--sidebar-padding', app.file, app.side, 'padding', app.mobile);
    compare(app.selector, '--dialog-overlay-padding', app.file, app.shade, 'padding', app.mobile);
    if (app.selector === '.ins-workspace') {
      compare(app.selector, '--dialog-body-padding', app.file, '.ins-dialog-body', 'padding');
      for (const name of ['header', 'body', 'footer']) {
        compare(
          app.selector,
          `--dialog-${name}-padding`,
          app.file,
          '.ins-dialog footer,.ins-dialog header,.ins-dialog-body',
          'padding',
          app.mobile,
        );
      }
      compare(app.selector, '--dialog-max-height', app.file, app.dialog, 'max-height', app.mobile);
    } else {
      compare(app.selector, '--page-padding', app.file, '.admin-page', 'padding');
      compare(app.selector, '--page-padding', app.file, '.admin-page', 'padding', app.narrow);
      compare(app.selector, '--topbar-padding', app.file, app.top, 'padding', app.mobile);
    }
  });

  afterAll(() => {
    for (const rule of tokens) {
      for (const name of Object.keys(rule.declarations)) {
        expect(name).toMatch(/^--/);
        expect(
          covered.has(key(rule.selector, name, rule.media)),
          `Unverified ${key(rule.selector, name, rule.media)}`,
        ).toBe(true);
      }
    }
  });

  it('leaves SVG/font assets untouched and exports only the token stylesheet', () => {
    expect(extracted).not.toMatch(/@import|@font-face|url\s*\(/i);
    // No shipping reference bundles, browser state, or mock identity in this package.
    const manifest = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as {
      files: string[];
      exports: Record<string, string>;
      sideEffects: string[];
    };
    expect(manifest.files).toEqual(['src/tokens.css']);
    expect(manifest.exports).toEqual({ './tokens.css': './src/tokens.css' });
    expect(manifest.sideEffects).toEqual(['**/*.css']);
  });
});
