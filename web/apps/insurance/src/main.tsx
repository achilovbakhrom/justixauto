import { mountApp } from '@justixauto/kit';
import { navigation } from './navigation';

mountApp({ rootId: 'insurance-root', basename: '/insurance', brand: 'Страховая', kinds: ['insurance'], nav: navigation });
