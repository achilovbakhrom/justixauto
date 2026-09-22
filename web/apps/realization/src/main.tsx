import { mountApp } from '@justixauto/kit';
import { navigation } from './navigation';

mountApp({ rootId: 'realization-root', basename: '/', brand: 'Реализация', kinds: ['seller'], nav: navigation });
