import type { ReactNode } from 'react';
import { Button } from '@justixauto/ui/Button';
import '@justixauto/tokens/tokens.css';
import { navigation } from '../routes/registry';
import styles from './Shell.module.css';

/** Display-only inputs from the eventual session adapter; no identity defaults. */
export interface ShellContext {
  organizationName: string;
  accountLabel: string;
}

export function Shell({ children, context }: { children: ReactNode; context?: ShellContext }) {
  return <div className={`ins-workspace ${styles.shell}`}>
    <aside className={styles.sidebar}>
      <div className={styles.brand}>JustixAuto<small>Страховая компания</small></div>
      <nav className={styles.navigation} aria-label="Страховая">
        {navigation.map((item) => <Button key={item.id} disabled className={styles.navItem} data-page-id={item.id}>{item.label}</Button>)}
      </nav>
    </aside>
    <main className={styles.main} id="insurance-main">
      <header className={styles.account}>
        {context && <><strong>{context.organizationName}</strong><label>{context.accountLabel}{' '}
          <select disabled aria-label="Страховая организация" value="current">
            <option value="current">{context.organizationName}</option>
          </select>
        </label></>}
      </header>
      <section className={styles.page}>{children}</section>
    </main>
  </div>;
}
