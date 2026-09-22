import { useEffect, useId, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import '@justixauto/tokens/tokens.css';
import { ApiError, errorText, upload } from './http';

/** Class names; the styles are injected once by <KitStyles/>. */
export const css = {
  centered: 'kit-centered', stack: 'kit-stack', row: 'kit-row', codes: 'kit-codes', grid: 'kit-grid',
  muted: 'kit-muted', right: 'kit-right',
};

const styles = `
.kit-app { font-family:var(--font-family);font-size:var(--font-size);line-height:var(--line-height);color:var(--text);background:var(--canvas);min-height:100vh }
.kit-centered { min-height:100vh;display:grid;place-items:center;padding:24px;background:var(--canvas);font-family:var(--font-family);color:var(--text) }
.kit-stack { display:grid;gap:12px }
.kit-row { display:flex;gap:10px;align-items:center;flex-wrap:wrap }
.kit-right { margin-left:auto }
.kit-muted { color:var(--text-muted);font-size:12px }
.kit-codes { padding:12px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface-subtle);font-size:14px;letter-spacing:.04em;white-space:pre-wrap;word-break:break-all }
.kit-card { width:min(440px,100%);padding:24px;border:1px solid var(--border);border-radius:12px;background:var(--surface);box-shadow:var(--shadow-overlay) }
.kit-card h1 { margin:0 0 16px;font-size:20px }
.kit-btn { height:36px;padding:0 14px;display:inline-flex;align-items:center;gap:6px;border:1px solid var(--border-strong);border-radius:var(--radius);background:var(--surface);color:var(--text);font:inherit;font-weight:600;cursor:pointer;white-space:nowrap }
.kit-btn:hover:not(:disabled) { background:var(--surface-subtle) }
.kit-btn:disabled { opacity:.55;cursor:not-allowed }
.kit-btn[data-variant=primary] { background:var(--primary);border-color:var(--primary);color:#fff }
.kit-btn[data-variant=primary]:hover:not(:disabled) { background:var(--primary-hover) }
.kit-btn[data-variant=danger] { background:var(--danger);border-color:var(--danger);color:#fff }
.kit-btn[data-variant=link] { border:0;background:none;color:var(--primary);padding:0;height:auto }
.kit-field { display:grid;gap:5px;min-width:0 }
.kit-field > span { color:var(--text-secondary);font-size:12px;font-weight:600 }
.kit-field input, .kit-field select, .kit-field textarea { box-sizing:border-box;width:100%;min-height:38px;padding:8px 10px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);color:var(--text);font:inherit }
.kit-field textarea { min-height:80px;resize:vertical }
.kit-field input:focus, .kit-field select:focus, .kit-field textarea:focus { outline:var(--field-focus-outline);border-color:var(--primary) }
.kit-field-error { color:var(--danger);font-size:12px }
.kit-notice { padding:10px 12px;border-radius:var(--radius);font-size:13px }
.kit-notice[data-kind=danger] { background:var(--danger-soft);color:var(--danger) }
.kit-notice[data-kind=success] { background:var(--success-soft);color:var(--success) }
.kit-notice[data-kind=info] { background:var(--info-soft);color:var(--info) }
.kit-notice[data-kind=warning] { background:var(--warning-soft);color:var(--warning) }
.kit-badge { display:inline-block;padding:2px 8px;border-radius:999px;font-size:12px;font-weight:600;background:var(--surface-subtle);color:var(--text-secondary);white-space:nowrap }
.kit-badge[data-tone=success] { background:var(--success-soft);color:var(--success) }
.kit-badge[data-tone=warning] { background:var(--warning-soft);color:var(--warning) }
.kit-badge[data-tone=danger] { background:var(--danger-soft);color:var(--danger) }
.kit-badge[data-tone=info] { background:var(--info-soft);color:var(--info) }
.kit-page { padding:var(--page-padding);display:grid;gap:16px;align-content:start }
.kit-page-head { display:flex;gap:12px;align-items:flex-start;flex-wrap:wrap }
.kit-page-head h1 { margin:0;font-size:22px }
.kit-page-head p { margin:2px 0 0;color:var(--text-secondary) }
.kit-panel { border:1px solid var(--border);border-radius:10px;background:var(--surface);overflow:auto }
.kit-panel-head { display:flex;align-items:center;gap:10px;padding:12px 16px;border-bottom:1px solid var(--border) }
.kit-panel-head h2 { margin:0;font-size:15px }
.kit-panel-body { padding:16px }
.kit-table { width:100%;border-collapse:collapse }
.kit-table th { text-align:left;padding:10px 14px;font-size:12px;color:var(--text-secondary);background:var(--surface-subtle);border-bottom:1px solid var(--border);white-space:nowrap }
.kit-table td { padding:10px 14px;border-bottom:1px solid var(--border);vertical-align:top }
.kit-table tr[data-click=true] { cursor:pointer }
.kit-table tr[data-click=true]:hover td { background:var(--surface-subtle) }
.kit-empty { padding:28px;text-align:center;color:var(--text-muted) }
.kit-grid { display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:12px }
.kit-dl { display:grid;grid-template-columns:minmax(120px,max-content) 1fr;gap:8px 16px;margin:0 }
.kit-dl dt { color:var(--text-secondary) }
.kit-dl dd { margin:0;overflow-wrap:anywhere }
.kit-tabs { display:flex;gap:4px;border-bottom:1px solid var(--border) }
.kit-tabs button { padding:9px 14px;border:0;border-bottom:2px solid transparent;background:none;font:inherit;color:var(--text-secondary);cursor:pointer }
.kit-tabs button[aria-selected=true] { color:var(--primary);border-bottom-color:var(--primary);font-weight:600 }
.kit-overlay { position:fixed;inset:0;z-index:100;display:grid;place-items:center;padding:24px;background:var(--dialog-backdrop) }
.kit-modal { width:min(560px,100%);max-height:90vh;overflow:auto;border-radius:12px;background:var(--surface);box-shadow:var(--shadow-overlay) }
.kit-modal[data-size=wide] { width:min(860px,100%) }
.kit-modal-head { display:flex;align-items:center;gap:12px;padding:16px 20px;border-bottom:1px solid var(--border) }
.kit-modal-head h2 { margin:0;font-size:18px;flex:1 }
.kit-modal-body { padding:20px;display:grid;gap:12px }
.kit-modal-foot { display:flex;justify-content:flex-end;gap:10px;padding:14px 20px;border-top:1px solid var(--border) }
.kit-stat { padding:16px;border:1px solid var(--border);border-radius:10px;background:var(--surface) }
.kit-stat b { display:block;font-size:24px }
.kit-stat span { color:var(--text-secondary);font-size:12px }
.kit-timeline { display:grid;gap:8px;margin:0;padding:0;list-style:none }
.kit-timeline li { padding:8px 12px;border-left:3px solid var(--primary-soft);background:var(--surface-subtle);border-radius:0 var(--radius) var(--radius) 0 }
`;

export function KitStyles() { return <style>{styles}</style>; }

export function Button({ children, onClick, variant = 'secondary', type = 'button', busy, disabled, title }: {
  children: ReactNode; onClick?: (() => void) | undefined; variant?: 'primary' | 'secondary' | 'danger' | 'link' | undefined;
  type?: 'button' | 'submit'; busy?: boolean | undefined; disabled?: boolean | undefined; title?: string | undefined;
}) {
  return <button className="kit-btn" data-variant={variant} type={type} onClick={onClick} disabled={disabled || busy}
    aria-busy={busy || undefined} title={title}>{busy ? '…' : children}</button>;
}

export function Card({ title, children }: { title: string; children: ReactNode }) {
  return <div className="kit-card"><h1>{title}</h1>{children}</div>;
}

export function Notice({ kind = 'info', children }: { kind?: 'info' | 'success' | 'warning' | 'danger'; children: ReactNode }) {
  return <div className="kit-notice" data-kind={kind} role={kind === 'danger' ? 'alert' : 'status'}>{children}</div>;
}

export function Badge({ tone, children }: { tone?: 'success' | 'warning' | 'danger' | 'info' | undefined; children: ReactNode }) {
  return <span className="kit-badge" data-tone={tone}>{children}</span>;
}

export function Field({ label, value, onChange, type = 'text', error, required, autoComplete, placeholder }: {
  label: string; value: string; onChange: (v: string) => void; type?: string; error?: string | undefined;
  required?: boolean; autoComplete?: string; placeholder?: string;
}) {
  const id = useId();
  return <label className="kit-field" htmlFor={id}><span>{label}</span>
    <input id={id} type={type} value={value} onChange={(e) => onChange(e.target.value)} required={required}
      autoComplete={autoComplete} placeholder={placeholder} aria-invalid={error ? true : undefined} />
    {error && <em className="kit-field-error">{error}</em>}
  </label>;
}

export function Page({ title, subtitle, actions, children }: { title: string; subtitle?: string; actions?: ReactNode; children: ReactNode }) {
  return <main className="kit-page">
    <div className="kit-page-head"><div><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>
      <div className="kit-row kit-right">{actions}</div></div>
    {children}
  </main>;
}

export function Panel({ title, actions, children, padded }: { title?: string; actions?: ReactNode; children: ReactNode; padded?: boolean }) {
  return <section className="kit-panel">
    {title && <div className="kit-panel-head"><h2>{title}</h2><div className="kit-row kit-right">{actions}</div></div>}
    {padded ? <div className="kit-panel-body">{children}</div> : children}
  </section>;
}

export interface Column<T> { title: string; render: (row: T) => ReactNode }

export function Table<T>({ rows, columns, rowKey, onRowClick, loading, error, empty = 'Пока ничего нет' }: {
  rows: T[] | undefined; columns: Column<T>[]; rowKey: (row: T) => string; onRowClick?: (row: T) => void;
  loading?: boolean; error?: unknown; empty?: string;
}) {
  if (error) return <div className="kit-empty"><Notice kind="danger">{errorText(error)}</Notice></div>;
  if (loading || !rows) return <div className="kit-empty">Загрузка…</div>;
  if (rows.length === 0) return <div className="kit-empty">{empty}</div>;
  return <table className="kit-table"><thead><tr>{columns.map((c) => <th key={c.title}>{c.title}</th>)}</tr></thead>
    <tbody>{rows.map((r) => <tr key={rowKey(r)} data-click={!!onRowClick} onClick={onRowClick ? () => onRowClick(r) : undefined}
      tabIndex={onRowClick ? 0 : undefined} onKeyDown={onRowClick ? (e) => { if (e.key === 'Enter') onRowClick(r); } : undefined}>
      {columns.map((c) => <td key={c.title}>{c.render(r)}</td>)}</tr>)}</tbody></table>;
}

export function Details({ items }: { items: [string, ReactNode][] }) {
  return <dl className="kit-dl">{items.map(([k, v]) => <div key={k} style={{ display: 'contents' }}><dt>{k}</dt><dd>{v ?? '—'}</dd></div>)}</dl>;
}

export function Tabs<T extends string>({ value, onChange, tabs }: { value: T; onChange: (v: T) => void; tabs: [T, string][] }) {
  return <div className="kit-tabs" role="tablist">{tabs.map(([k, label]) =>
    <button key={k} role="tab" aria-selected={value === k} onClick={() => onChange(k)}>{label}</button>)}</div>;
}

export function Stat({ label, value }: { label: string; value: ReactNode }) {
  return <div className="kit-stat"><b>{value}</b><span>{label}</span></div>;
}

export function Modal({ title, onClose, children, footer, size }: { title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; size?: 'wide' | undefined }) {
  const ref = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  useEffect(() => { close.current = onClose; });
  // Focus once on open; parents re-render with new callbacks while the user types.
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('.kit-modal-body input,.kit-modal-body select,.kit-modal-body textarea,button')?.focus();
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') close.current(); };
    document.addEventListener('keydown', esc);
    return () => document.removeEventListener('keydown', esc);
  }, []);
  return <div className="kit-overlay"><div className="kit-modal" data-size={size} role="dialog" aria-modal="true" aria-label={title} ref={ref}>
    <div className="kit-modal-head"><h2>{title}</h2><Button variant="link" onClick={onClose}>Закрыть</Button></div>
    <div className="kit-modal-body">{children}</div>
    {footer && <div className="kit-modal-foot">{footer}</div>}
  </div></div>;
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
    <form onSubmit={(e) => { e.preventDefault(); void submit(); }} className="kit-stack">
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
  const label = <span>{spec.label}{req ? ' *' : ''}</span>;
  const err = error && <em className="kit-field-error">{error}</em>;
  switch (spec.type) {
    case 'textarea':
      return <label className="kit-field" htmlFor={id}>{label}<textarea id={id} value={String(value)} onChange={(e) => onChange(e.target.value)} />{err}</label>;
    case 'select':
      return <label className="kit-field" htmlFor={id}>{label}<select id={id} value={String(value)} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>{spec.options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}</select>{err}</label>;
    case 'multiselect':
      return <fieldset className="kit-field"><span>{spec.label}</span>{spec.options.map(([v, l]) =>
        <label key={v} className="kit-row"><input type="checkbox" checked={(value as string[]).includes(v)}
          onChange={(e) => onChange(e.target.checked ? [...(value as string[]), v] : (value as string[]).filter((x) => x !== v))} />{l}</label>)}{err}</fieldset>;
    case 'checkbox':
      return <label className="kit-row"><input type="checkbox" checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />{spec.label}{err}</label>;
    case 'file':
      return <label className="kit-field" htmlFor={id}>{label}<input id={id} type="file" accept="application/pdf,image/jpeg,image/png"
        onChange={(e) => onFile(e.target.files?.[0] ?? null)} /><em className="kit-muted">PDF, JPEG или PNG до 10 МБ</em>{err}</label>;
    case 'money':
      return <label className="kit-field" htmlFor={id}>{label}<div className="kit-row" style={{ flexWrap: 'nowrap' }}>
        <input id={id} inputMode="decimal" value={String(value)} onChange={(e) => onChange(e.target.value)} placeholder="0.00" />
        <select value={currency} onChange={(e) => onCurrency(e.target.value)} style={{ width: 90 }} aria-label="Валюта">
          {currencies.map((c) => <option key={c}>{c}</option>)}</select></div>{err}</label>;
    default: {
      const type = spec.type === 'datetime' ? 'datetime-local' : spec.type;
      return <label className="kit-field" htmlFor={id}>{label}<input id={id} type={type} value={String(value)}
        onChange={(e) => onChange(e.target.value)} />{'hint' in spec && spec.hint && <em className="kit-muted">{spec.hint}</em>}{err}</label>;
    }
  }
}
