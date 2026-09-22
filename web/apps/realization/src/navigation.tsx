import type { NavItem } from '@justixauto/kit';
import { BillingPage } from './pages/billing';
import { CRMPage } from './pages/crm';
import { OffersPage } from './pages/offers';
import { InsurancePage } from './pages/partners-finance';
import { PartnersPage } from './pages/partners';
import { PurchasesPage } from './pages/purchases';
import { SalesPage } from './pages/sales';
import { SettingsPage } from './pages/settings';
import { DashboardPage, VehiclesPage, WarehousesPage } from './pages/stock';

export const navigation: NavItem[] = [
  { to: '/dashboard', label: 'Дашборд', group: 'Обзор', icon: 'dashboard', element: <DashboardPage /> },
  { to: '/partners', label: 'Партнёры', group: 'Работа', icon: 'users', permission: 'commerce.read', element: <PartnersPage /> },
  { to: '/warehouses', label: 'Склады', group: 'Работа', icon: 'warehouse', permission: 'inventory.read', element: <WarehousesPage /> },
  { to: '/vehicles', label: 'Автомобили', group: 'Работа', icon: 'car', permission: 'inventory.read', element: <VehiclesPage /> },
  { to: '/purchases', label: 'Закупки', group: 'Работа', icon: 'cart', permission: 'commerce.read', element: <PurchasesPage /> },
  { to: '/offers', label: 'Предложения', group: 'Работа', icon: 'tag', permission: 'retail.read', element: <OffersPage /> },
  { to: '/sales', label: 'Продажи', group: 'Работа', icon: 'cart', permission: 'retail.read', element: <SalesPage /> },
  { to: '/crm', label: 'CRM', group: 'Работа', icon: 'users', permission: 'retail.read', element: <CRMPage /> },
  { to: '/billing', label: 'Счета и оплаты', group: 'Финансы', icon: 'wallet', permission: 'commerce.read', element: <BillingPage /> },
  { to: '/insurance-applications', label: 'Страхование', group: 'Финансы', icon: 'shield', permission: 'insurance.read', element: <InsurancePage /> },
  { to: '/settings', label: 'Настройки', icon: 'settings', bottom: true, element: <SettingsPage /> },
];
