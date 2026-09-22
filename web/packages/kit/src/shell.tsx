import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { BrowserRouter, NavLink, Navigate, Route, Routes } from 'react-router-dom';
import { errorText, setStepUpHandler } from './http';
import { ChangePassword, MFASetup, SessionGate, stepUp, useSession } from './session';
import type { Company } from './session';
import { Button, Field, FormDialog, KitStyles, Modal, Notice, Tabs, css } from './ui';
import type { FieldSpec } from './ui';

export interface NavItem { to: string; label: string; group?: string; permission?: string; element: ReactNode }

const shellStyles = `
.kit-shell { display:grid;grid-template-columns:248px minmax(0,1fr);grid-template-rows:auto 1fr;min-height:100vh }
.kit-side { grid-row:1/3;display:flex;flex-direction:column;gap:14px;padding:18px 14px;background:var(--sidebar);border-right:1px solid var(--shell-border) }
.kit-brand { display:flex;gap:10px;align-items:center;padding:0 6px }
.kit-brand-mark { width:34px;height:34px;display:grid;place-items:center;border-radius:9px;background:var(--primary);color:#fff;font-weight:800 }
.kit-brand small { display:block;color:var(--text-muted);font-size:12px }
.kit-nav { display:grid;gap:2px }
.kit-nav-group { margin:10px 6px 4px;color:var(--text-muted);font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.06em }
.kit-nav a { padding:8px 10px;border-radius:var(--radius);color:var(--nav-text);text-decoration:none;font-weight:500 }
.kit-nav a:hover { background:var(--surface-subtle) }
.kit-nav a.active { background:var(--nav-active-background);color:var(--nav-active-text);font-weight:600 }
.kit-top { display:flex;gap:12px;align-items:center;padding:10px 24px;background:var(--topbar);border-bottom:1px solid var(--shell-border) }
.kit-top select { min-height:34px;padding:4px 8px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);font:inherit }
.kit-user { margin-left:auto;display:flex;gap:10px;align-items:center }
@media(max-width:800px){ .kit-shell{grid-template-columns:1fr} .kit-side{grid-row:auto} }
`;

/** Mounts one of the four apps: session gate, router, sidebar and routes. */
export function mountApp(opts: { rootId: string; basename: string; brand: string; kinds?: Company['kind'][]; nav: NavItem[]; banner?: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: true } } });
  createRoot(document.getElementById(opts.rootId)!).render(
    <QueryClientProvider client={client}>
      <div className="kit-app"><KitStyles /><style>{shellStyles}</style>
        <SessionGate title={`JustixAuto — ${opts.brand}`} {...(opts.kinds ? { kinds: opts.kinds } : {})}>
          <BrowserRouter basename={opts.basename}><Shell {...opts} /></BrowserRouter>
        </SessionGate>
      </div>
    </QueryClientProvider>,
  );
}

function Shell({ brand, nav, banner }: { brand: string; nav: NavItem[]; banner?: ReactNode }) {
  const s = useSession();
  const items = nav.filter((n) => !n.permission || s.can(n.permission));
  const groups = [...new Set(items.map((n) => n.group ?? ''))];
  const [security, setSecurity] = useState(false);
  return <div className="kit-shell">
    <aside className="kit-side">
      <div className="kit-brand"><div className="kit-brand-mark">J</div><div><b>JustixAuto</b><small>{brand}</small></div></div>
      {groups.map((g) => <nav key={g} className="kit-nav" aria-label={g || brand}>
        {g && <div className="kit-nav-group">{g}</div>}
        {items.filter((n) => (n.group ?? '') === g).map((n) => <NavLink key={n.to} to={n.to}>{n.label}</NavLink>)}
      </nav>)}
    </aside>
    <header className="kit-top">
      <CompanySwitch />
      <div className="kit-user">
        <span>{s.view.user.displayName}</span>
        <Button variant="link" onClick={() => setSecurity(true)}>Безопасность</Button>
        <Button variant="link" onClick={() => void s.logout()}>Выйти</Button>
      </div>
    </header>
    <div>
      {banner}
      <Routes>
        {items.map((n) => <Route key={n.to} path={`${n.to}/*`} element={n.element} />)}
        <Route path="*" element={items[0] ? <Navigate to={items[0].to} replace /> : <p className={css.muted}>Нет доступных разделов.</p>} />
      </Routes>
    </div>
    {security && <SecurityDialog onClose={() => setSecurity(false)} />}
    <StepUpHost />
  </div>;
}

function CompanySwitch() {
  const s = useSession();
  const qc = useQueryClient();
  if (s.view.accessibleCompanies.length < 2) return <b>{s.company?.name ?? 'Платформа'}</b>;
  return <label className={css.row}>Компания
    <select value={s.view.context.companyId ?? ''} onChange={async (e) => {
      await s.selectCompany(e.target.value || null);
      qc.clear();
    }}>{s.view.accessibleCompanies.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select>
  </label>;
}

function SecurityDialog({ onClose }: { onClose: () => void }) {
  const s = useSession();
  const [tab, setTab] = useState<'mfa' | 'password'>('mfa');
  return <Modal title="Безопасность" onClose={onClose}>
    <Tabs value={tab} onChange={setTab} tabs={[['mfa', 'Двухфакторная защита'], ['password', 'Пароль']]} />
    {tab === 'mfa' && (s.view.mfa.enrolled
      ? <Notice kind="success">Двухфакторная защита включена.</Notice>
      : <MFASetup onDone={() => { void s.refresh(); onClose(); }} />)}
    {tab === 'password' && <ChangePassword onDone={() => { void s.refresh(); onClose(); }} />}
  </Modal>;
}

/** Asks for a TOTP/recovery code when a sensitive action needs a fresh second factor. */
function StepUpHost() {
  const [pending, setPending] = useState<((ok: boolean) => void) | null>(null);
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    setStepUpHandler(() => new Promise<boolean>((resolve) => { setCode(''); setError(''); setPending(() => resolve); }));
    return () => setStepUpHandler(undefined);
  }, []);
  if (!pending) return null;
  const close = (ok: boolean) => { pending(ok); setPending(null); };
  return <Modal title="Подтвердите действие" onClose={() => close(false)}
    footer={<><Button onClick={() => close(false)}>Отмена</Button><Button variant="primary" onClick={async () => {
      try { await stepUp(code); close(true); } catch (e) { setError(errorText(e)); }
    }}>Подтвердить</Button></>}>
    <p>Это чувствительное действие. Введите код из приложения-аутентификатора.</p>
    {error && <Notice kind="danger">{error}</Notice>}
    <Field label="Код" value={code} onChange={setCode} autoComplete="one-time-code" />
  </Modal>;
}

// ---- data hooks ----

/** Loads data with react-query; key parts identify the resource. */
export function useData<T>(key: unknown[], fn: () => Promise<T>, enabled = true, refetchInterval?: number) {
  return useQuery({ queryKey: key, queryFn: fn, enabled, ...(refetchInterval ? { refetchInterval } : {}) });
}

/** Runs a command and refreshes the given query keys (prefixes) afterwards. */
export function useCommand<A, R>(fn: (a: A) => Promise<R>, invalidate: unknown[][]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSettled: () => Promise.all(invalidate.map((k) => qc.invalidateQueries({ queryKey: k }))),
  });
}

/** Refreshes queries after a dialog closes. */
export function useRefresh() {
  const qc = useQueryClient();
  return (...keys: unknown[][]) => Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: k })));
}

/**
 * A button that opens a form dialog, runs the command and refreshes data.
 * With no fields it asks for confirmation only.
 */
export function ActionButton({ label, title, fields = [], submitLabel, onSubmit, refresh = [], variant, intro, size, disabled }: {
  label: string; title?: string; fields?: FieldSpec[]; submitLabel?: string; intro?: ReactNode; size?: 'wide';
  onSubmit: (values: Record<string, unknown>) => Promise<unknown>; refresh?: unknown[][];
  variant?: 'primary' | 'secondary' | 'danger'; disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const reload = useRefresh();
  return <>
    <Button variant={variant ?? 'secondary'} onClick={() => setOpen(true)} disabled={disabled}>{label}</Button>
    {open && <FormDialog title={title ?? label} fields={fields} submitLabel={submitLabel ?? label} intro={intro}
      {...(size ? { size } : {})}
      onClose={() => setOpen(false)} onSubmit={async (v) => { const r = await onSubmit(v); await reload(...refresh); return r; }} />}
  </>;
}
