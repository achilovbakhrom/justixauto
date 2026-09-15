import type { ReactNode } from 'react';
import { Button } from '@justixauto/ui/Button';
import '@justixauto/tokens/tokens.css';
import { navigation } from '../routes/registry';
import styles from './Shell.module.css';

export interface ShellProps { children: ReactNode; identityLabel?: string }

/** Presentation only; the injected session boundary must admit this subtree. */
export function Shell({ children, identityLabel }: ShellProps) {
  function items(group: typeof navigation[number]['group']) {
    return navigation.filter((item) => item.group === group).map((item) =>
      <Button key={item.id} className={styles.item} disabled data-page-id={item.id}>{item.label}</Button>);
  }
  return <div className={`admin-shell ${styles.shell}`}>
    <aside className={styles.sidebar}>
      <div className={styles.brand}>JustixAuto<small>Администрирование</small></div>
      <nav className={styles.navigation} aria-label="Администрирование">
        {items('main')}
        <div className={styles.heading}>Интеграции</div>
        <div className={styles.subnav}><div className={styles.navigation}>{items('integrations')}</div></div>
        {items('audit')}
      </nav>
    </aside>
    <main className={styles.main} id="admin-main">
      <header className={styles.topbar}><span>Управление платформой</span>
        {identityLabel && <span className={styles.identity}>{identityLabel}</span>}
      </header>
      <section className={styles.page}>{children}</section>
    </main>
  </div>;
}
