import { mountApp } from '@justixauto/kit';
import { AuditPage, CompaniesPage, RolesPage, UsersPage } from './pages';

mountApp({
  rootId: 'admin-root', basename: '/admin', brand: 'Администрирование',
  nav: [
    { to: 'companies', label: 'Компании', permission: 'platform.directory.read', element: <CompaniesPage /> },
    { to: 'users', label: 'Пользователи', permission: 'platform.users.manage', element: <UsersPage /> },
    { to: 'roles', label: 'Роли и права', permission: 'platform.roles.manage', element: <RolesPage /> },
    { to: 'audit', label: 'Аудит', permission: 'platform.audit.read', element: <AuditPage /> },
  ],
});
