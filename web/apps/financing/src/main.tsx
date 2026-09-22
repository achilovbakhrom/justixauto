import { mountApp } from '@justixauto/kit';
import { navigation } from './navigation';

mountApp({ rootId: 'finance-root', basename: '/finance', brand: 'Банк / МФО', kinds: ['bank', 'mfo'], nav: navigation });
