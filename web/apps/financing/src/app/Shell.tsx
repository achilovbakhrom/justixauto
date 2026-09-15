import type { ReactNode } from 'react';
import { Button } from '@justixauto/ui/Button';
import '@justixauto/tokens/tokens.css';
import { navigation } from '../routes/registry';
import styles from './Shell.module.css';

/** Display-only inputs from the eventual session adapter; no identity defaults. */
export interface ShellContext {
  organizationName: string;
  organizationTypeLabel: string;
  organizationMark: string;
  accountLabel: string;
  employeeName: string;
  employeeInitials: string;
}

export function Shell({ children, context }: { children: ReactNode; context?: ShellContext }) {
  return <div className={`finance-workspace ${styles.shell}`}>
    <aside className={styles.sidebar}>
      <div className={styles.brand}><div className={styles.brandMark}>J</div><div>
        <div className={styles.brandName}>JustixAuto</div>
        <div className={styles.brandRole}>Финансирование</div>
      </div></div>
      {context && <div className={styles.contextSummary}>
        <div className={styles.contextAvatar}>{context.organizationMark}</div>
        <div className={styles.contextText}><strong>{context.organizationName}</strong><span>{context.organizationTypeLabel}</span></div>
      </div>}
      <div className={styles.navGroup}>Работа</div>
      <nav className={styles.navigation} aria-label="Финансирование">
        {navigation.map((item) => <Button key={item.id} disabled className={styles.navItem} data-page-id={item.id}>{item.label}</Button>)}
      </nav>
      <div className={styles.sidebarBottom}><Button disabled variant="secondary" className={styles.control}>Кабинет продавца ↗</Button></div>
    </aside>
    <header className={styles.topbar}>
      {context && <label className={styles.account}><span>{context.accountLabel}</span>
        <select disabled aria-label="Организация финансирования" value="current">
          <option value="current">{context.organizationName} · {context.organizationTypeLabel}</option>
        </select>
      </label>}
      <div className={styles.actions}>
        <Button disabled variant="secondary" className={styles.control}>Уведомления</Button>
        {context && <><span>{context.employeeName}</span><div className={styles.userChip}>{context.employeeInitials}</div></>}
      </div>
    </header>
    <main className={styles.main} id="finance-main"><section className={styles.page}>{children}</section></main>
  </div>;
}
