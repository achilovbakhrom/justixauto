import { OrgSettings } from '@justixauto/kit';
import type { NavItem } from '@justixauto/kit';
import { ApplicationsPage, OverviewPage, PartnersPage, ProgramsPage } from './pages';

export const navigation: NavItem[] = [
  { to: '/overview', label: 'Обзор', group: 'Работа', permission: 'financing.read', element: <OverviewPage /> },
  { to: '/applications', label: 'Заявки', group: 'Работа', permission: 'financing.read', element: <ApplicationsPage /> },
  { to: '/programs', label: 'Программы', group: 'Работа', permission: 'financing.read', element: <ProgramsPage /> },
  { to: '/partners', label: 'Партнёры', group: 'Работа', permission: 'financing.read', element: <PartnersPage /> },
  { to: '/settings', label: 'Настройки', group: 'Работа', element: <OrgSettings /> },
];
