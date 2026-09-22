import type { NavItem } from '@justixauto/kit';
import { ApplicationsPage, ProgramsPage } from './pages';

export const navigation: NavItem[] = [
  { to: '/applications', label: 'Заявки', permission: 'financing.read', element: <ApplicationsPage /> },
  { to: '/programs', label: 'Программы', permission: 'financing.read', element: <ProgramsPage /> },
];
