import type { NavItem } from '@justixauto/kit';
import { ApplicationsPage } from './pages';

export const navigation: NavItem[] = [
  { to: '/applications', label: 'Заявки', permission: 'insurance.read', element: <ApplicationsPage /> },
];
