import { useEffect, useId, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import '@justixauto/tokens/tokens.css';
import { ApiError, errorText, upload } from './http';
import { Icon } from './icons';
import type { IconName } from './icons';
import './design.css';

/** Layout helpers the reference has no class for; everything else uses the reference classes (design.css). */
export const css = {
  centered: 'kit-centered', stack: 'kit-stack', row: 'kit-row', codes: 'kit-codes', grid: 'kit-grid',
  muted: 'cell-sub', right: 'kit-right',
};

const styles = `
.sidebar a.nav-item, .sidebar a.btn { text-decoration:none }
.kit-centered { min-height:100vh;display:grid;place-items:center;padding:24px;background:var(--canvas) }
.kit-auth { width:min(420px,100%);padding:28px }
.kit-auth .brand { padding:0 0 20px }
.kit-auth h1 { margin:0 0 6px;font-size:20px }
.kit-stack { display:grid;gap:14px }
.kit-row { display:flex;gap:10px;align-items:center;flex-wrap:wrap }
.kit-right { margin-left:auto }
.kit-codes { margin:0;padding:12px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface-subtle);font-size:14px;letter-spacing:.04em;white-space:pre-wrap;word-break:break-all }
.kit-grid { display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:12px }
.kit-page-body > * + * { margin-top:18px }
.kit-page-body > .summary-strip { margin-bottom:0 }
.kit-notice { padding:11px 13px;border-radius:6px;font-size:13px;border-left:3px solid currentColor }
.kit-notice[data-kind=danger] { color:var(--danger);background:var(--danger-soft) }
.kit-notice[data-kind=success] { color:var(--success);background:var(--success-soft) }
.kit-notice[data-kind=warning] { color:var(--warning);background:var(--warning-soft) }
.kit-notice[data-kind=info] { color:var(--text-secondary);background:var(--surface-subtle);border-left-color:var(--primary) }
.kit-link { height:auto;padding:0;border:0;background:none;color:var(--primary);font-weight:600;cursor:pointer }
.kit-info { display:grid }
.kit-info .info-row > :last-child { text-align:right;overflow-wrap:anywhere }
.kit-clickable tbody tr { cursor:pointer }
.kit-clickable tbody tr:hover td { background:var(--surface-subtle) }
.modal-body > * + * { margin-top:14px }
.kit-field { display:grid;gap:6px;color:var(--text-secondary);font-size:12px;font-weight:600 }
.kit-field input,.kit-field select,.kit-field textarea { width:100%;min-height:40px;padding:9px 11px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);color:var(--text);font-size:14px;font-weight:400 }
.kit-field textarea { min-height:80px;resize:vertical }
.kit-timeline { display:grid;gap:8px;margin:0;padding:0;list-style:none }
.kit-timeline li { padding:9px 12px;border-left:3px solid var(--primary-soft);background:var(--surface-subtle);border-radius:0 var(--radius) var(--radius) 0 }
`;

export function KitStyles() { return <style>{styles}</style>; }

type Variant = 'primary' | 'secondary' | 'danger' | 'link';

export function Button({ children, onClick, variant = 'secondary', type = 'button', busy, disabled, title, size, icon }: {
  children: ReactNode; onClick?: (() => void) | undefined; variant?: Variant | undefined;
  type?: 'button' | 'submit'; busy?: boolean | undefined; disabled?: boolean | undefined; title?: string | undefined;
  size?: 'sm' | undefined; icon?: IconName | undefined;
}) {
  const cls = variant === 'link' ? 'kit-link' : `btn btn-${variant}${size === 'sm' ? ' btn-sm' : ''}`;
  return <button className={cls} type={type} onClick={onClick} disabled={disabled || busy}
    aria-busy={busy || undefined} title={title}>{icon && <Icon name={icon} />}{busy ? '…' : children}</button>;
}

/** Centered card for sign-in and other pre-app screens. */
export function Card({ title, subtitle, children }: { title: string; subtitle?: string | undefined; children: ReactNode }) {
  return <section className="surface kit-auth">
    <div className="brand"><div className="brand-mark">J</div><div><div className="brand-name">JustixAuto</div>{subtitle && <div className="brand-role">{subtitle}</div>}</div></div>
    <h1>{title}</h1>{children}
  </section>;
}

export function Notice({ kind = 'info', children }: { kind?: 'info' | 'success' | 'warning' | 'danger'; children: ReactNode }) {
  return <div className="kit-notice" data-kind={kind} role={kind === 'danger' ? 'alert' : 'status'}>{children}</div>;
}

const toneClass = { success: 'status-success', warning: 'status-warning', danger: 'status-danger', info: 'status-info' } as const;

/** Status pill of the reference (`.status`). */
export function Badge({ tone, children }: { tone?: 'success' | 'warning' | 'danger' | 'info' | undefined; children: ReactNode }) {
  return <span className={`status ${tone ? toneClass[tone] : 'status-neutral'}`}>{children}</span>;
}

export function Field({ label, value, onChange, type = 'text', error, required, autoComplete, placeholder }: {
  label: string; value: string; onChange: (v: string) => void; type?: string; error?: string | undefined;
  required?: boolean; autoComplete?: string; placeholder?: string;
}) {
  const id = useId();
  return <div className="field"><label htmlFor={id}>{label}</label>
    <input id={id} type={type} value={value} onChange={(e) => onChange(e.target.value)} required={required}
      autoComplete={autoComplete} placeholder={placeholder} aria-invalid={error ? true : undefined} />
    {error && <div className="field-error">{error}</div>}
  </div>;
}

export function Page({ title, subtitle, actions, children }: { title: string; subtitle?: ReactNode; actions?: ReactNode; children: ReactNode }) {
  return <section className="page">
    <div className="page-header"><div><h1 className="page-title">{title}</h1>{subtitle && <div className="page-subtitle">{subtitle}</div>}</div>
      {actions && <div className="page-actions">{actions}</div>}</div>
    <div className="kit-page-body">{children}</div>
  </section>;
}

/** A `.surface` block with an optional section header. */
export function Panel({ title, actions, children, padded }: { title?: string | undefined; actions?: ReactNode; children: ReactNode; padded?: boolean }) {
  return <section className="surface">
    {title && <div className="section-head"><h2 className="section-title">{title}</h2>{actions && <div className="page-actions">{actions}</div>}</div>}
    {padded ? <div className="section-body">{children}</div> : children}
  </section>;
}

export interface Column<T> { title: string; render: (row: T) => ReactNode }

export function Table<T>({ rows, columns, rowKey, onRowClick, loading, error, empty = 'Пока ничего нет' }: {
  rows: T[] | undefined; columns: Column<T>[]; rowKey: (row: T) => string; onRowClick?: (row: T) => void;
  loading?: boolean; error?: unknown; empty?: string;
}) {
  if (error) return <div className="table-empty-inline"><Notice kind="danger">{errorText(error)}</Notice></div>;
  if (loading || !rows) return <div className="table-empty-inline"><span className="cell-sub">Загрузка…</span></div>;
  if (rows.length === 0) return <div className="table-empty-inline"><strong>{empty}</strong></div>;
  return <div className={`table-wrap${onRowClick ? ' kit-clickable' : ''}`}><table>
    <thead><tr>{columns.map((c, i) => <th key={i}>{c.title}</th>)}</tr></thead>
    <tbody>{rows.map((r) => <tr key={rowKey(r)} onClick={onRowClick ? () => onRowClick(r) : undefined}
      tabIndex={onRowClick ? 0 : undefined} onKeyDown={onRowClick ? (e) => { if (e.key === 'Enter') onRowClick(r); } : undefined}>
      {columns.map((c, i) => <td key={i}>{c.render(r)}</td>)}</tr>)}</tbody></table></div>;
}

/** Label / value rows (`.info-row`). */
export function Details({ items }: { items: [string, ReactNode][] }) {
  return <div className="kit-info">{items.map(([k, v]) =>
    <div key={k} className="info-row"><span>{k}</span><strong>{v ?? '—'}</strong></div>)}</div>;
}

/** View tabs (`.view-tabs`); a third tuple element shows a count. `channel` = page-level tabs (Клиентам / Партнёрам). */
export function Tabs<T extends string>({ value, onChange, tabs, channel, actions }: {
  value: T; onChange: (v: T) => void; tabs: ([T, string] | [T, string, number | undefined])[]; channel?: boolean; actions?: ReactNode;
}) {
  return <div className={`view-tabs${channel ? ' channel-tabs' : ''}`} role="tablist">{tabs.map(([k, label, count]) =>
    <button key={k} role="tab" aria-selected={value === k} className={`view-tab${value === k ? ' active' : ''}`} onClick={() => onChange(k)}>
      {label}{count !== undefined && <b>{count}</b>}</button>)}
    {actions && <div className="kit-row" style={{ marginLeft: 'auto', alignSelf: 'center' }}>{actions}</div>}</div>;
}

/** Search and filters above a list (`.toolbar`); Сбросить clears them. */
export function Toolbar({ query, onQuery, placeholder, children, onReset }: {
  query: string; onQuery: (q: string) => void; placeholder: string; children?: ReactNode; onReset?: () => void;
}) {
  return <div className="toolbar">
    <label className="search-field"><Icon name="search" /><input placeholder={placeholder} value={query} onChange={(e) => onQuery(e.target.value)} aria-label={placeholder} /></label>
    {children}
    <button className="btn btn-secondary btn-sm" type="button" onClick={() => { onQuery(''); onReset?.(); }}>Сбросить</button>
  </div>;
}

/** Filter select of a toolbar; the first option means "all". */
export function FilterSelect({ value, onChange, all, options }: { value: string; onChange: (v: string) => void; all: string; options: [string, string][] }) {
  return <select className="select" value={value} onChange={(e) => onChange(e.target.value)} aria-label={all}>
    <option value="">{all}</option>{options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}</select>;
}

/** "N записей · hint" line above a table. */
export function ResultMeta({ children }: { children: ReactNode }) { return <div className="result-meta">{children}</div>; }

/** Two-line table cell. */
export function Cell({ main, sub }: { main: ReactNode; sub?: ReactNode }) {
  return <><div className="cell-main">{main}</div>{sub && <div className="cell-sub">{sub}</div>}</>;
}

export function Progress({ value, max }: { value: number; max: number }) {
  return <div className="progress"><span style={{ width: `${max > 0 ? Math.min(100, (value / max) * 100) : 0}%` }} /></div>;
}

/** Case-insensitive match of a search query against any of the texts. */
export const matches = (query: string, ...texts: (string | undefined | null)[]) => {
  const q = query.trim().toLowerCase();
  return !q || texts.some((t) => t?.toLowerCase().includes(q));
};

/** Russian plural: plural(5, ['запись', 'записи', 'записей']). */
export function plural(n: number, forms: [string, string, string]) {
  const a = Math.abs(n) % 100, b = a % 10;
  return `${n} ${a > 10 && a < 20 ? forms[2] : b === 1 ? forms[0] : b >= 2 && b <= 4 ? forms[1] : forms[2]}`;
}

/** One figure of a `.summary-strip`. */
export function Stat({ label, value, note }: { label: string; value: ReactNode; note?: string | undefined }) {
  return <div className="summary-item"><div className="summary-label">{label}</div><div className="summary-value">{value}</div>
    {note && <div className="summary-note">{note}</div>}</div>;
}

export function Stats({ children, columns }: { children: ReactNode; columns?: number }) {
  return <div className="summary-strip" style={columns ? { gridTemplateColumns: `repeat(${columns},minmax(0,1fr))` } : undefined}>{children}</div>;
}

export function EmptyState({ icon = 'info', title, text, action }: { icon?: IconName; title: string; text?: string; action?: ReactNode }) {
  return <div className="empty-state"><div className="empty-icon"><Icon name={icon} size={24} /></div><h3>{title}</h3>{text && <p>{text}</p>}{action}</div>;
}

export function Modal({ title, onClose, children, footer, size, help, icon = 'file' }: {
  title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; size?: 'wide' | undefined; help?: ReactNode; icon?: IconName;
}) {
  const ref = useRef<HTMLElement>(null);
  const close = useRef(onClose);
  useEffect(() => { close.current = onClose; });
  // Focus once on open; parents re-render with new callbacks while the user types.
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('.modal-body input,.modal-body select,.modal-body textarea,.modal-close')?.focus();
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') close.current(); };
    document.addEventListener('keydown', esc);
    return () => document.removeEventListener('keydown', esc);
  }, []);
  return <div className="modal-scrim"><section className={`modal${size === 'wide' ? ' modal-wide' : ''}`} role="dialog" aria-modal="true" aria-label={title} ref={ref}>
    <header className="modal-header"><div className="modal-icon"><Icon name={icon} size={22} /></div>
      <div><h2 className="modal-title">{title}</h2>{help && <div className="modal-help">{help}</div>}</div>
      <button className="modal-close" type="button" onClick={onClose} aria-label="Закрыть"><Icon name="close" /></button></header>
    <div className="modal-body">{children}</div>
    {footer && <footer className="modal-footer">{footer}</footer>}
  </section></div>;
}

// ---- money: exact decimal strings, never floating point ----

/** Formats minor units ("150000", "USD") as "1 500.00 USD". */
export function money(m: { amountMinor: string; currency: string } | undefined | null): string {
  if (!m) return '—';
  const neg = m.amountMinor.startsWith('-');
  const digits = (neg ? m.amountMinor.slice(1) : m.amountMinor).padStart(3, '0');
  const major = digits.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return `${neg ? '−' : ''}${major}.${digits.slice(-2)} ${m.currency}`;
}

/** Parses "1 500,5" into minor units "150050" (2 decimals); null if invalid. */
/** Minor units ("123456") to a decimal string for inputs ("1234.56"). */
export function minorToMajor(minor: string): string {
  const d = minor.padStart(3, '0');
  return `${d.slice(0, -2)}.${d.slice(-2)}`;
}

export function toMinor(input: string): string | null {
  const s = input.replace(/\s/g, '').replace(',', '.');
  const m = /^(\d+)(?:\.(\d{0,2}))?$/.exec(s);
  if (!m) return null;
  const minor = (m[1]! + (m[2] ?? '').padEnd(2, '0')).replace(/^0+(?=\d)/, '');
  return minor;
}

export const date = (s: string | null | undefined) => s ? new Date(s).toLocaleDateString('ru-RU') : '—';
export const dateTime = (s: string | null | undefined) => s ? new Date(s).toLocaleString('ru-RU') : '—';

// ---- generic form dialog ----

export type FieldSpec =
  | { name: string; label: string; type: 'text' | 'textarea' | 'date' | 'datetime' | 'number' | 'password' | 'email'; required?: boolean; initial?: string; hint?: string }
  | { name: string; label: string; type: 'money'; required?: boolean; initial?: string; currency?: string }
  | { name: string; label: string; type: 'select'; options: [string, string][]; required?: boolean; initial?: string }
  | { name: string; label: string; type: 'multiselect'; options: [string, string][]; initial?: string[] }
  | { name: string; label: string; type: 'checkbox'; initial?: boolean }
  | { name: string; label: string; type: 'file'; purpose: string; required?: boolean };

export type FormValues = Record<string, string | string[] | boolean>;

/**
 * A modal form: collects values, calls submit, shows server field errors
 * next to fields (matching by field name suffix) and closes on success.
 * Money fields yield {amountMinor, currency}; file fields upload first and
 * yield the file ID.
 */
export function FormDialog({ title, fields, submitLabel = 'Сохранить', onSubmit, onClose, intro, size }: {
  title: string; fields: FieldSpec[]; submitLabel?: string; intro?: ReactNode; size?: 'wide' | undefined;
  onSubmit: (values: Record<string, unknown>) => Promise<unknown>; onClose: () => void;
}) {
  const [values, setValues] = useState<FormValues>(() => Object.fromEntries(fields.map((f) =>
    [f.name, f.type === 'checkbox' ? !!f.initial : f.type === 'multiselect' ? (f.initial ?? []) : f.type === 'file' ? '' :
      f.type === 'money' ? (f.initial ?? '') : (f.initial ?? '')])));
  const [files, setFiles] = useState<Record<string, File | null>>({});
  const [currency, setCurrency] = useState<Record<string, string>>(() => Object.fromEntries(fields
    .filter((f) => f.type === 'money').map((f) => [f.name, (f as { currency?: string }).currency ?? 'USD'])));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (name: string, v: string | string[] | boolean) => setValues((s) => ({ ...s, [name]: v }));

  async function submit() {
    setBusy(true); setError(''); setErrors({});
    try {
      const out: Record<string, unknown> = {};
      for (const f of fields) {
        const v = values[f.name];
        if (f.type === 'money') {
          if (v === '' && !f.required) continue;
          const minor = toMinor(String(v));
          if (minor === null) { setErrors({ [f.name]: 'сумма, например 1500.00' }); setBusy(false); return; }
          out[f.name] = { amountMinor: minor, currency: currency[f.name] };
        } else if (f.type === 'file') {
          const file = files[f.name];
          if (file) out[f.name] = (await upload(file, f.purpose)).id;
        } else if (f.type === 'datetime') {
          out[f.name] = v ? new Date(String(v)).toISOString() : '';
        } else out[f.name] = v;
      }
      await onSubmit(out);
      onClose();
    } catch (e) {
      if (e instanceof ApiError && Object.keys(e.fields).length) {
        const byField: Record<string, string> = {};
        const rest: string[] = [];
        for (const [k, msg] of Object.entries(e.fields)) {
          const f = fields.find((x) => k === x.name || k.endsWith('.' + x.name) || k.startsWith(x.name + '.'));
          if (f) byField[f.name] = msg; else rest.push(`${k}: ${msg}`);
        }
        setErrors(byField);
        setError(rest.length ? rest.join('; ') : e.message);
      } else setError(errorText(e));
    } finally { setBusy(false); }
  }

  return <Modal title={title} onClose={onClose} size={size}
    footer={<><Button onClick={onClose} disabled={busy}>Отмена</Button><Button variant="primary" busy={busy} onClick={() => void submit()}>{submitLabel}</Button></>}>
    {intro}
    {error && <Notice kind="danger">{error}</Notice>}
    <form onSubmit={(e) => { e.preventDefault(); void submit(); }} className="form-grid">
      {fields.map((f) => <FieldInput key={f.name} spec={f} value={values[f.name]!} error={errors[f.name]}
        onChange={(v) => set(f.name, v)} onFile={(file) => setFiles((s) => ({ ...s, [f.name]: file }))}
        currency={currency[f.name]} onCurrency={(c) => setCurrency((s) => ({ ...s, [f.name]: c }))} />)}
      <button type="submit" hidden />
    </form>
  </Modal>;
}

const currencies = ['USD', 'UZS', 'EUR', 'RUB', 'KZT'];

function FieldInput({ spec, value, error, onChange, onFile, currency, onCurrency }: {
  spec: FieldSpec; value: string | string[] | boolean; error: string | undefined;
  onChange: (v: string | string[] | boolean) => void; onFile: (f: File | null) => void;
  currency: string | undefined; onCurrency: (c: string) => void;
}) {
  const id = useId();
  const req = 'required' in spec && spec.required;
  const label = <label htmlFor={id}>{spec.label}{req ? ' *' : ''}</label>;
  const err = error && <div className="field-error">{error}</div>;
  // Long inputs span both columns of the reference .form-grid.
  const wide = ['textarea', 'multiselect', 'checkbox', 'file'].includes(spec.type) || spec.label.length > 34;
  const cls = `field${wide ? ' field-full' : ''}`;
  switch (spec.type) {
    case 'textarea':
      return <div className={cls}>{label}<textarea id={id} value={String(value)} onChange={(e) => onChange(e.target.value)} />{err}</div>;
    case 'select':
      return <div className={cls}>{label}<select id={id} value={String(value)} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>{spec.options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}</select>{err}</div>;
    case 'multiselect':
      return <fieldset className={`${cls} checkbox-fieldset`}><legend className="form-section-label">{spec.label}</legend>
        <div className="checkbox-grid">{spec.options.map(([v, l]) =>
          <label key={v} className="check-line"><input type="checkbox" checked={(value as string[]).includes(v)}
            onChange={(e) => onChange(e.target.checked ? [...(value as string[]), v] : (value as string[]).filter((x) => x !== v))} /><span>{l}</span></label>)}
          {spec.options.length === 0 && <span className="cell-sub">Нет вариантов</span>}</div>{err}</fieldset>;
    case 'checkbox':
      return <div className={cls}><label className="check-line"><input type="checkbox" checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} /><span>{spec.label}</span></label>{err}</div>;
    case 'file':
      return <div className={cls}>{label}<input id={id} type="file" accept="application/pdf,image/jpeg,image/png"
        onChange={(e) => onFile(e.target.files?.[0] ?? null)} /><div className="field-hint">PDF, JPEG или PNG до 10 МБ</div>{err}</div>;
    case 'money':
      return <div className={cls}>{label}<div className="kit-row" style={{ flexWrap: 'nowrap' }}>
        <input id={id} inputMode="decimal" value={String(value)} onChange={(e) => onChange(e.target.value)} placeholder="0.00" />
        <select value={currency} onChange={(e) => onCurrency(e.target.value)} style={{ width: 96 }} aria-label="Валюта">
          {currencies.map((c) => <option key={c}>{c}</option>)}</select></div>{err}</div>;
    default: {
      const type = spec.type === 'datetime' ? 'datetime-local' : spec.type;
      return <div className={cls}>{label}<input id={id} type={type} value={String(value)}
        onChange={(e) => onChange(e.target.value)} />{'hint' in spec && spec.hint && <div className="field-hint">{spec.hint}</div>}{err}</div>;
    }
  }
}
