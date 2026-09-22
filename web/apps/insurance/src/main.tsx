import { mountApp } from '@justixauto/kit';
import { navigation } from './navigation';

mountApp({ rootId: 'insurance-root', basename: '/insurance', brand: 'Страховая компания', kinds: ['insurance'], nav: navigation, variant: 'insurance' });
