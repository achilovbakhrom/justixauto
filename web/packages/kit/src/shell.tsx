import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { BrowserRouter, NavLink, Navigate, Route, Routes, useNavigate, useSearchParams } from 'react-router-dom';
import { errorText, list, setStepUpHandler } from './http';
import { Icon } from './icons';
import type { IconName } from './icons';
import { ChangePassword, MFASetup, SessionGate, stepUp, useSession } from './session';
import type { Company } from './session';
import { Button, Field, FormDialog, KitStyles, Modal, Notice, Tabs } from './ui';
import type { FieldSpec } from './ui';

export interface NavItem {
  to: string; label: string; group?: string; permission?: string; element: ReactNode;
  icon?: IconName; bottom?: boolean;
}

export interface ShellOptions {
  rootId: string; basename: string; brand: string; kinds?: Company['kind'][]; nav: NavItem[]; banner?: ReactNode;
  /** Shows the branch-scope control (Realization). */
  branches?: boolean;
  /** Extra class of the app root from the reference (Realization: dealer-shell, Admin: admin-shell). */
  shellClass?: string;
  /** Global search in the top bar: placeholder and where a query leads. */
  search?: { placeholder: string; path: (q: string) => string };
}

/** Mounts one of the four apps: session gate, router, sidebar and routes. */
export function mountApp(opts: ShellOptions) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: true } } });
  createRoot(document.getElementById(opts.rootId)!).render(
    <QueryClientProvider client={client}>
      <KitStyles />
      <SessionGate title={`JustixAuto — ${opts.brand}`} brand={opts.brand} {...(opts.kinds ? { kinds: opts.kinds } : {})}>
        <BrowserRouter basename={opts.basename}><Shell {...opts} /></BrowserRouter>
      </SessionGate>
    </QueryClientProvider>,
  );
}

const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]!.toUpperCase()).join('') || 'J';

/** Other cabinets the user can open (links at the bottom of the sidebar). */
const cabinets: { kinds: Company['kind'][]; label: string; href: string }[] = [
  { kinds: ['seller'], label: 'Кабинет продавца ↗', href: '/' },
  { kinds: ['bank', 'mfo'], label: 'Кабинет банка / МФО ↗', href: '/finance/' },
  { kinds: ['insurance'], label: 'Кабинет страховой ↗', href: '/insurance/' },
];

const kindLabel: Record<Company['kind'], string> = { seller: 'Продавец', bank: 'Банк', mfo: 'МФО', insurance: 'Страховая компания' };

function Shell({ brand, nav, banner, branches, search, kinds, shellClass }: ShellOptions) {
  const s = useSession();
  const items = nav.filter((n) => !n.permission || s.can(n.permission));
  const main = items.filter((n) => !n.bottom);
  const groups = [...new Set(main.map((n) => n.group ?? ''))];
  const [security, setSecurity] = useState(false);
  const scope = useScopeLabel();
  const others = cabinets.filter((c) => !kinds?.some((k) => c.kinds.includes(k)) && s.view.accessibleCompanies.some((x) => c.kinds.includes(x.kind)));
  const link = (n: NavItem) => <NavLink key={n.to} to={n.to} className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}>
    <Icon name={n.icon ?? 'info'} className="nav-icon" /><span>{n.label}</span></NavLink>;
  return <div className={`app-shell${shellClass ? ` ${shellClass}` : ''}`}>
    <aside className="sidebar">
      <div className="brand"><div className="brand-mark">J</div><div><div className="brand-name">JustixAuto</div><div className="brand-role">{brand}</div></div></div>
      {s.company && <div className="context-summary"><div className="context-avatar">{initials(s.company.name)}</div>
        <div><strong>{s.company.name}</strong><span>{branches ? scope : kindLabel[s.company.kind]}</span></div></div>}
      {groups.map((g) => <div key={g}>{g && <div className="nav-group">{g}</div>}<nav className="nav" aria-label={g || brand}>{main.filter((n) => (n.group ?? '') === g).map(link)}</nav></div>)}
      <div className="sidebar-bottom">
        <nav className="nav">{items.filter((n) => n.bottom).map(link)}</nav>
        {others.map((c) => <a key={c.href} className="btn btn-secondary btn-sm" style={{ marginTop: 10, textDecoration: 'none', whiteSpace: 'normal' }} href={c.href}>{c.label}</a>)}
      </div>
    </aside>
    <header className="topbar">
      <CompanySwitch kinds={kinds} />
      {branches && <BranchSwitch />}
      {search ? <GlobalSearch {...search} /> : <div style={{ marginLeft: 'auto' }} />}
      <Popover align="right" button={(open) => <button className="icon-btn" aria-label="Уведомления" aria-expanded={open}><Icon name="bell" /></button>}>
        <div className="popover-title">Уведомления</div><div className="menu-option" style={{ cursor: 'default' }}>Новых уведомлений нет</div>
      </Popover>
      <span>RU</span>
      <Popover align="right" button={(open) => <button className="user-chip" style={{ border: 0, cursor: 'pointer' }} aria-label={`Пользователь ${s.view.user.displayName}`} aria-expanded={open}>{initials(s.view.user.displayName)}</button>}>
        <div className="popover-title">{s.view.user.displayName}</div>
        <button className="menu-option" onClick={() => setSecurity(true)}><Icon name="shield" />Безопасность</button>
        <button className="menu-option" onClick={() => void s.logout()}><Icon name="close" />Выйти</button>
      </Popover>
    </header>
    <main className="main">
      {banner}
      <Routes>
        {items.map((n) => <Route key={n.to} path={`${n.to}/*`} element={n.element} />)}
        <Route path="*" element={items[0] ? <Navigate to={items[0].to} replace /> : <section className="page"><p className="cell-sub">Нет доступных разделов.</p></section>} />
      </Routes>
    </main>
    {security && <SecurityDialog onClose={() => setSecurity(false)} />}
    <StepUpHost />
  </div>;
}

/** Anchored dropdown (`.popover`) that closes on outside click, Escape or a chosen option. */
export function Popover({ button, children, align = 'left' }: { button: (open: boolean) => ReactNode; children: ReactNode; align?: 'left' | 'right' }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', away);
    document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('mousedown', away); document.removeEventListener('keydown', esc); };
  }, [open]);
  return <div ref={ref} style={{ position: 'relative' }} onClick={(e) => { if ((e.target as HTMLElement).closest('.menu-option')) setOpen(false); }}>
    <div onClick={() => setOpen((o) => !o)}>{button(open)}</div>
    {open && <div className="popover" style={align === 'right' ? { left: 'auto', right: 0 } : undefined}>{children}</div>}
  </div>;
}

function useBranchList() {
  const s = useSession();
  const id = s.company?.id ?? '';
  return useQuery({ queryKey: ['branches', id], queryFn: () => list<{ id: string; name: string }>(`/identity/companies/${id}/branches`), enabled: !!id });
}

/** "Все филиалы" or the names of the selected branches. */
export function useScopeLabel() {
  const s = useSession();
  const q = useBranchList();
  const scope = s.view.context.branchScope;
  if (scope.mode === 'ALL') return 'Все филиалы';
  const names = scope.branchIds.map((b) => q.data?.find((x) => x.id === b)?.name).filter(Boolean);
  return names.length ? names.join(', ') : `Филиалов: ${scope.branchIds.length}`;
}

function CompanySwitch({ kinds }: { kinds: Company['kind'][] | undefined }) {
  const s = useSession();
  const qc = useQueryClient();
  const usable = s.view.accessibleCompanies.filter((c) => !kinds || kinds.includes(c.kind));
  const control = (open: boolean) => <div className="context-control" role="button" tabIndex={0} aria-expanded={open} aria-label={`Компания: ${s.company?.name ?? 'Платформа'}`}>
    <span><span className="label">Компания</span><span className="value">{s.company?.name ?? 'Платформа'}</span></span>{usable.length > 1 && <Icon name="down" size={16} />}</div>;
  if (usable.length < 2) return control(false);
  return <Popover button={control}>
    <div className="popover-title">Компании</div>
    {usable.map((c) => <button key={c.id} className={`menu-option company-menu-option${c.id === s.company?.id ? ' selected' : ''}`}
      onClick={async () => { await s.selectCompany(c.id); qc.clear(); }}>
      <span className="context-avatar">{initials(c.name)}</span><span className="company-menu-copy"><strong>{c.name}</strong><small>{kindLabel[c.kind]}</small></span>
      {c.id === s.company?.id ? <Icon name="check" /> : <span />}</button>)}
  </Popover>;
}

function BranchSwitch() {
  const s = useSession();
  const qc = useQueryClient();
  const q = useBranchList();
  const scope = s.view.context.branchScope;
  const label = useScopeLabel();
  const apply = async (mode: 'ALL' | 'SELECTED', ids: string[]) => { await s.setBranchScope(mode, ids); await qc.invalidateQueries(); };
  return <Popover button={(open) => <div className="context-control" role="button" tabIndex={0} aria-expanded={open} aria-label={`Филиалы: ${label}`}>
    <span><span className="label">Филиалы</span><span className="value">{label}</span></span><Icon name="down" size={16} /></div>}>
    <div className="popover-title">Рабочие филиалы</div>
    <button className={`menu-option${scope.mode === 'ALL' ? ' selected' : ''}`} onClick={() => void apply('ALL', [])}>Все филиалы</button>
    {(q.data ?? []).map((b) => {
      const on = scope.mode === 'SELECTED' && scope.branchIds.includes(b.id);
      const next = on ? scope.branchIds.filter((x) => x !== b.id) : [...(scope.mode === 'SELECTED' ? scope.branchIds : []), b.id];
      return <button key={b.id} className={`menu-option${on ? ' selected' : ''}`}
        onClick={() => void (next.length ? apply('SELECTED', next) : apply('ALL', []))}>{b.name}</button>;
    })}
    {q.data?.length === 0 && <div className="menu-option" style={{ cursor: 'default' }}>Филиалов пока нет</div>}
  </Popover>;
}

function GlobalSearch({ placeholder, path }: { placeholder: string; path: (q: string) => string }) {
  const navigate = useNavigate();
  const [q, setQ] = useState('');
  return <form className="global-search" role="search" onSubmit={(e) => { e.preventDefault(); if (q.trim()) navigate(path(q.trim())); }}>
    <Icon name="search" /><input aria-label="Глобальный поиск" placeholder={placeholder} value={q} onChange={(e) => setQ(e.target.value)} />
  </form>;
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

/** Search text kept in the URL (?q=), so the top-bar search can open a filtered list. */
export function useSearchQuery(): [string, (q: string) => void] {
  const [params, setParams] = useSearchParams();
  return [params.get('q') ?? '', (q) => setParams(q ? { q } : {}, { replace: true })];
}

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
export function ActionButton({ label, title, fields = [], submitLabel, onSubmit, refresh = [], variant, intro, size, disabled, small, defaultOpen }: {
  label: string; title?: string; fields?: FieldSpec[]; submitLabel?: string; intro?: ReactNode; size?: 'wide';
  onSubmit: (values: Record<string, unknown>) => Promise<unknown>; refresh?: unknown[][];
  variant?: 'primary' | 'secondary' | 'danger'; disabled?: boolean; small?: boolean; defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(!!defaultOpen);
  const reload = useRefresh();
  return <>
    <Button variant={variant ?? 'secondary'} onClick={() => setOpen(true)} disabled={disabled} {...(small ? { size: 'sm' as const } : {})}>{label}</Button>
    {open && <FormDialog title={title ?? label} fields={fields} submitLabel={submitLabel ?? label} intro={intro}
      {...(size ? { size } : {})}
      onClose={() => setOpen(false)} onSubmit={async (v) => { const r = await onSubmit(v); await reload(...refresh); return r; }} />}
  </>;
}
