import type { ReactNode } from 'react';
import { Button } from '@justixauto/ui/Button';
import '@justixauto/tokens/tokens.css';
import { navigation } from '../routes/registry';
import styles from './Shell.module.css';

/** Display values from the eventual session/context adapter, never identity defaults. */
export interface ShellContext { company: string; branches: string; companyMark: string; userMark: string; locale: string }
const paths = {
  dashboard: 'M3 3h7v7H3zM14 3h7v4h-7zM14 11h7v10h-7zM3 14h7v7H3z',
  people: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75',
  warehouse: 'M3 10 12 4l9 6v10H3zM7 20v-6h10v6M3 10h18',
  car: 'M5 17h14v-5l-2-5H7l-2 5v5zM7 17v2M17 17v2M5 12h14M8 14h.01M16 14h.01',
  cart: 'M3 3h2l2.5 12h10l2-8H6M9 20h.01M17 20h.01',
  tag: 'M20.6 13.4 11 3.8V3H4v7h.8l9.6 9.6a2 2 0 0 0 2.8 0l3.4-3.4a2 2 0 0 0 0-2.8zM7.5 7.5h.01',
  wallet: 'M20 7V5a2 2 0 0 0-2-2H5a3 3 0 0 0 0 6h15v12H5a3 3 0 0 1-3-3V6M16 13h.01',
  shield: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10zM9 12l2 2 4-4',
  settings: 'M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-2.9 1.2v.1h-4v-.1a1.7 1.7 0 0 0-2.9-1.2l-.1.1L4.2 17l.1-.1A1.7 1.7 0 0 0 3.1 14H3v-4h.1A1.7 1.7 0 0 0 4.3 7.1L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 10 3.1V3h4v.1a1.7 1.7 0 0 0 2.9 1.2l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0 1.2 2.9h.1v4h-.1a1.7 1.7 0 0 0-1.5 1z',
  chevron: 'm6 9 6 6 6-6', search: 'm21 21-4.4-4.4',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0',
};
function Icon({ name, size = 18 }: { name: keyof typeof paths; size?: number }) {
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    {name === 'settings' && <circle cx="12" cy="12" r="3" />}
    {name === 'search' && <circle cx="11" cy="11" r="8" />}<path d={paths[name]} />
  </svg>;
}

export function Shell({ children, context }: { children: ReactNode; context?: ShellContext }) {
  function items(group: typeof navigation[number]['group']) {
    return <nav className={styles.nav} aria-label={group === 'bottom' ? 'Настройки' : group}>
      {navigation.filter((item) => item.group === group).map((item) => <Button key={item.id}
        className={styles.item} disabled aria-label={item.label} data-page-id={item.id}>
        <Icon name={item.icon} /><span>{item.label}</span>
      </Button>)}
    </nav>;
  }
  return <div className={`dealer-shell ${styles.shell}`}>
    <aside className={styles.sidebar}>
      <div className={styles.brand}><div className={styles.brandMark}>J</div><div>
        <div className={styles.brandName}>JustixAuto</div><div className={styles.brandRole}>Реализация</div>
      </div></div>
      <div className={styles.summary}><div className={styles.avatar}>{context?.companyMark}</div>
        <div className={styles.summaryText}><strong>{context?.company}</strong><span>{context?.branches}</span></div>
      </div>
      {(['Обзор', 'Работа', 'Финансы'] as const).map((group) => <div key={group} className={styles.groupWrap}>
        <div className={styles.group}>{group}</div>{items(group)}
      </div>)}
      <div className={styles.bottom}>{items('bottom')}
        <Button className={styles.external} disabled>Кабинет банка / МФО ↗</Button>
        <Button className={styles.external} disabled>Кабинет страховой ↗</Button>
      </div>
    </aside>
    <header className={styles.topbar}>
      {(['company', 'branches'] as const).map((key) => <Button className={styles.context} disabled key={key}
        aria-label={key === 'company' ? 'Компания' : 'Филиалы'}>
        <span><span className={styles.label}>{key === 'company' ? 'Компания' : 'Филиалы'}</span>
          <span className={styles.value}>{context?.[key]}</span></span><Icon name="chevron" size={16} />
      </Button>)}
      <label className={styles.search}><Icon name="search" /><input disabled aria-label="Глобальный поиск" placeholder="VIN, заказ, клиент" /></label>
      <Button className={styles.notifications} disabled aria-label="Уведомления"><Icon name="bell" /></Button>
      <span>{context?.locale}</span><div className={styles.user}>{context?.userMark}</div>
    </header>
    <main className={styles.main}><section className={styles.page}>{children}</section></main>
  </div>;
}
