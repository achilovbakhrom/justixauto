import { mountApp } from '@justixauto/kit';
import { navigation } from './navigation';
import './design.css';

mountApp({
  rootId: 'realization-root', basename: '/', brand: 'Реализация', kinds: ['seller'], nav: navigation, branches: true, shellClass: 'dealer-shell',
  search: { placeholder: 'VIN, модель или клиент', path: (q) => `/vehicles?q=${encodeURIComponent(q)}` },
});
