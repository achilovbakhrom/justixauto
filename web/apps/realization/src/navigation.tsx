import type { NavItem } from '@justixauto/kit';
import { DashboardPage, VehiclesPage, WarehousesPage } from './pages/inventory';
import { FinancingPage, InsurancePage } from './pages/partners-finance';
import { CRMPage, ListingsPage, SalesPage } from './pages/retail';
import { SettingsPage } from './pages/settings';
import { BillingPage, OffersPage, PartnersPage, PurchasesPage } from './pages/trade';

export const navigation: NavItem[] = [
  { to: '/dashboard', label: 'Обзор', permission: 'inventory.read', element: <DashboardPage /> },
  { to: '/warehouses', label: 'Склады', group: 'Склад', permission: 'inventory.read', element: <WarehousesPage /> },
  { to: '/vehicles', label: 'Автомобили', group: 'Склад', permission: 'inventory.read', element: <VehiclesPage /> },
  { to: '/partners', label: 'Партнёры', group: 'Опт', permission: 'commerce.read', element: <PartnersPage /> },
  { to: '/offers', label: 'Предложения', group: 'Опт', permission: 'commerce.read', element: <OffersPage /> },
  { to: '/purchases', label: 'Заказы', group: 'Опт', permission: 'commerce.read', element: <PurchasesPage /> },
  { to: '/billing', label: 'Счета', group: 'Опт', permission: 'commerce.read', element: <BillingPage /> },
  { to: '/sales', label: 'Продажи', group: 'Розница', permission: 'retail.read', element: <SalesPage /> },
  { to: '/listings', label: 'Витрина', group: 'Розница', permission: 'retail.read', element: <ListingsPage /> },
  { to: '/crm', label: 'Клиенты', group: 'Розница', permission: 'retail.read', element: <CRMPage /> },
  { to: '/insurance', label: 'Страхование', group: 'Партнёры', permission: 'insurance.read', element: <InsurancePage /> },
  { to: '/financing', label: 'Финансирование', group: 'Партнёры', permission: 'financing.read', element: <FinancingPage /> },
  { to: '/settings', label: 'Настройки', group: 'Компания', element: <SettingsPage /> },
];
