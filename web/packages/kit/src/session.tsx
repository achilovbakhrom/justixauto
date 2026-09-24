import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { ApiError, call, configureHttp, errorText, setCsrf } from './http';
import { Button, Card, Field, Notice, css } from './ui';

export interface Company {
  id: string;
  name: string;
  kind: 'seller' | 'bank' | 'mfo' | 'insurance';
  access: string;
}
export interface SessionView {
  user: { id: string; displayName: string; status: string; passwordChangeRequired: boolean };
  roles: { id: string; name: string }[];
  permissions: string[];
  mfa: { enrolled: boolean; disabled?: boolean; authenticatedAt?: string };
  context: {
    revision: string;
    companyId: string | null;
    branchScope: { mode: 'ALL' | 'SELECTED'; branchIds: string[] };
  };
  accessibleCompanies: Company[];
  setup: { next: 'company' | 'branch' | 'none' };
}

interface SessionApi {
  view: SessionView;
  company: Company | undefined;
  can: (permission: string) => boolean;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  selectCompany: (id: string | null) => Promise<void>;
  setBranchScope: (mode: 'ALL' | 'SELECTED', branchIds: string[]) => Promise<void>;
}

const Ctx = createContext<SessionApi | null>(null);

export function useSession(): SessionApi {
  const s = useContext(Ctx);
  if (!s) throw new Error('useSession outside SessionGate');
  return s;
}

type State =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'anonymous' }
  | { kind: 'challenge'; challengeId: string }
  | { kind: 'ready'; view: SessionView };

async function loadSession(): Promise<State> {
  try {
    return { kind: 'ready', view: (await call<SessionView>('GET', '/identity/session')).data };
  } catch (e) {
    return e instanceof ApiError && e.status === 401 ? { kind: 'anonymous' } : { kind: 'error', message: errorText(e) };
  }
}

/**
 * Loads the session and shows sign-in, the MFA challenge or the forced
 * password change before rendering the app. `kinds` limits which company
 * kinds this app works with (e.g. the insurer cabinet).
 */
export function SessionGate({
  children,
  title,
  brand,
  kinds,
}: {
  children: ReactNode;
  title: string;
  brand?: string;
  kinds?: Company['kind'][];
}) {
  const [state, setState] = useState<State>({ kind: 'loading' });

  const refresh = useCallback(() => loadSession().then(setState), []);

  useEffect(() => {
    configureHttp({ onUnauthenticated: () => setState({ kind: 'anonymous' }) });
    let live = true;
    void loadSession().then((s) => {
      if (live) setState(s);
    });
    return () => {
      live = false;
    };
  }, []);

  const api = useMemo<SessionApi | null>(() => {
    if (state.kind !== 'ready') return null;
    const view = state.view;
    return {
      view,
      company: view.accessibleCompanies.find((c) => c.id === view.context.companyId),
      can: (p) => view.permissions.includes(p),
      refresh,
      logout: async () => {
        try {
          await call('POST', '/identity/session/logout');
        } finally {
          setCsrf('');
          setState({ kind: 'anonymous' });
        }
      },
      selectCompany: async (id) => {
        await call('PUT', '/identity/session/context', { companyId: id }, { ifMatch: view.context.revision });
        await refresh();
      },
      setBranchScope: async (mode, branchIds) => {
        await call('PUT', '/identity/session/branch-scope', { mode, branchIds }, { ifMatch: view.context.revision });
        await refresh();
      },
    };
  }, [state, refresh]);

  if (state.kind === 'loading')
    return (
      <Centered>
        <p>Загрузка…</p>
      </Centered>
    );
  if (state.kind === 'error') {
    return (
      <Centered>
        <Card title="Нет связи">
          <Notice kind="danger">{state.message}</Notice>
          <Button onClick={() => void refresh()}>Повторить</Button>
        </Card>
      </Centered>
    );
  }
  if (state.kind === 'anonymous') {
    return (
      <Login
        title={title}
        brand={brand}
        onDone={(r) => (r.challengeId ? setState({ kind: 'challenge', challengeId: r.challengeId }) : refresh())}
      />
    );
  }
  if (state.kind === 'challenge') {
    return (
      <MFAChallenge
        challengeId={state.challengeId}
        onDone={() => void refresh()}
        onCancel={() => setState({ kind: 'anonymous' })}
      />
    );
  }
  if (state.view.user.passwordChangeRequired) return <ChangePassword forced onDone={() => void refresh()} />;
  return (
    <Ctx.Provider value={api}>
      <CompanyGate kinds={kinds}>{children}</CompanyGate>
    </Ctx.Provider>
  );
}

/** Makes sure a suitable company is active; offers a choice if several. */
function CompanyGate({ children, kinds }: { children: ReactNode; kinds: Company['kind'][] | undefined }) {
  const s = useSession();
  const usable = s.view.accessibleCompanies.filter((c) => !kinds || kinds.includes(c.kind));
  const [error, setError] = useState('');
  const tried = useRef(false);
  const only = usable.length === 1 ? usable[0]!.id : undefined;
  const hasCompany = !!s.company;
  useEffect(() => {
    if (hasCompany || !only || tried.current) return;
    tried.current = true;
    s.selectCompany(only).catch((e) => setError(errorText(e)));
  }, [s, hasCompany, only]);
  if (!kinds) return <>{children}</>; // platform admin app works without a company
  if (s.company && kinds.includes(s.company.kind)) return <>{children}</>;
  if (usable.length === 0) {
    return (
      <Centered>
        <Card title="Нет доступа">
          <p>У вашей учётной записи нет компании для этого кабинета.</p>
          <Button onClick={() => void s.logout()}>Выйти</Button>
        </Card>
      </Centered>
    );
  }
  return (
    <Centered>
      <Card title="Выберите компанию">
        {error && <Notice kind="danger">{error}</Notice>}
        <div className={css.stack}>
          {usable.map((c) => (
            <Button key={c.id} onClick={() => s.selectCompany(c.id).catch((e) => setError(errorText(e)))}>
              {c.name}
            </Button>
          ))}
        </div>
      </Card>
    </Centered>
  );
}

function Centered({ children }: { children: ReactNode }) {
  return <div className={css.centered}>{children}</div>;
}

function Login({
  brand,
  onDone,
}: {
  title: string;
  brand: string | undefined;
  onDone: (r: { challengeId?: string }) => void;
}) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <Centered>
      <Card title="Вход" subtitle={brand}>
        <form
          className={css.stack}
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError('');
            try {
              const r = await call<{ challengeId?: string }>('POST', '/identity/session/login', { login, password });
              onDone(r.data ?? {});
            } catch (err) {
              setError(
                err instanceof ApiError && err.status === 429
                  ? 'Слишком много попыток. Попробуйте позже.'
                  : errorText(err),
              );
            } finally {
              setBusy(false);
            }
          }}
        >
          {error && <Notice kind="danger">{error}</Notice>}
          <Field label="Логин" value={login} onChange={setLogin} autoComplete="username" required />
          <Field
            label="Пароль"
            type="password"
            value={password}
            onChange={setPassword}
            autoComplete="current-password"
            required
          />
          <Button type="submit" variant="primary" busy={busy}>
            Войти
          </Button>
          <p className="cell-sub">
            Нет доступа? Компанию и её первого администратора подключает администратор платформы JustixAuto.
          </p>
        </form>
      </Card>
    </Centered>
  );
}

function MFAChallenge({
  challengeId,
  onDone,
  onCancel,
}: {
  challengeId: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  return (
    <Centered>
      <Card title="Двухфакторная проверка">
        <form
          className={css.stack}
          onSubmit={async (e) => {
            e.preventDefault();
            try {
              await call('POST', '/identity/session/mfa/verify', { challengeId, code });
              onDone();
            } catch (err) {
              setError(errorText(err));
            }
          }}
        >
          <p>Введите 6-значный код из приложения-аутентификатора или резервный код.</p>
          {error && <Notice kind="danger">{error}</Notice>}
          <Field label="Код" value={code} onChange={setCode} autoComplete="one-time-code" required />
          <div className={css.row}>
            <Button type="submit" variant="primary">
              Подтвердить
            </Button>
            <Button onClick={onCancel}>Назад</Button>
          </div>
        </form>
      </Card>
    </Centered>
  );
}

export function ChangePassword({ forced, onDone }: { forced?: boolean; onDone: () => void }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const form = (
    <form
      className={css.stack}
      onSubmit={async (e) => {
        e.preventDefault();
        try {
          await call('POST', '/identity/session/password', {
            currentPassword: current,
            newPassword: next,
            newPasswordConfirmation: confirm,
          });
          onDone();
        } catch (err) {
          setError(errorText(err));
        }
      }}
    >
      {forced && <p>Администратор задал вам временный пароль. Придумайте собственный (не менее 12 символов).</p>}
      {error && <Notice kind="danger">{error}</Notice>}
      <Field
        label="Текущий пароль"
        type="password"
        value={current}
        onChange={setCurrent}
        autoComplete="current-password"
        required
      />
      <Field
        label="Новый пароль"
        type="password"
        value={next}
        onChange={setNext}
        autoComplete="new-password"
        required
      />
      <Field
        label="Повторите новый пароль"
        type="password"
        value={confirm}
        onChange={setConfirm}
        autoComplete="new-password"
        required
      />
      <Button type="submit" variant="primary">
        Сохранить пароль
      </Button>
    </form>
  );
  return forced ? (
    <Centered>
      <Card title="Смена пароля">{form}</Card>
    </Centered>
  ) : (
    form
  );
}

/** Sets up TOTP: shows the secret, confirms a code, shows recovery codes once. */
export function MFASetup({ onDone }: { onDone: () => void }) {
  const [enrollment, setEnrollment] = useState<{ enrollmentId: string; secret: string; otpauthUri: string } | null>(
    null,
  );
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[] | null>(null);
  const [error, setError] = useState('');
  if (codes) {
    return (
      <div className={css.stack}>
        <Notice kind="success">
          Двухфакторная защита включена. Сохраните резервные коды — они показываются один раз.
        </Notice>
        <pre className={css.codes}>{codes.join('\n')}</pre>
        <Button variant="primary" onClick={onDone}>
          Готово
        </Button>
      </div>
    );
  }
  if (!enrollment) {
    return (
      <div className={css.stack}>
        {error && <Notice kind="danger">{error}</Notice>}
        <p>
          Для чувствительных действий нужна двухфакторная аутентификация (приложение Google Authenticator, 1Password и
          т.п.).
        </p>
        <Button
          variant="primary"
          onClick={async () => {
            try {
              setEnrollment(
                (
                  await call<{ enrollmentId: string; secret: string; otpauthUri: string }>(
                    'POST',
                    '/identity/session/mfa/enrollment',
                  )
                ).data,
              );
            } catch (e) {
              setError(errorText(e));
            }
          }}
        >
          Начать настройку
        </Button>
      </div>
    );
  }
  return (
    <form
      className={css.stack}
      onSubmit={async (e) => {
        e.preventDefault();
        try {
          const r = await call<{ recoveryCodes: string[] }>(
            'POST',
            `/identity/session/mfa/enrollment/${enrollment.enrollmentId}/confirm`,
            { code },
          );
          setCodes(r.data.recoveryCodes);
        } catch (err) {
          setError(errorText(err));
        }
      }}
    >
      <p>Добавьте ключ в приложение-аутентификатор:</p>
      <pre className={css.codes}>{enrollment.secret}</pre>
      <a href={enrollment.otpauthUri}>Открыть в приложении</a>
      {error && <Notice kind="danger">{error}</Notice>}
      <Field label="Код из приложения" value={code} onChange={setCode} autoComplete="one-time-code" required />
      <Button type="submit" variant="primary">
        Включить
      </Button>
    </form>
  );
}

/** Re-confirms the second factor for sensitive actions (5 minutes). */
export async function stepUp(code: string) {
  await call('POST', '/identity/session/mfa/step-up', { code });
}
