import { defineConfig } from '@playwright/test';
import shared from '../../../../../tests/e2e/playwright.config';
export default defineConfig(shared, {
  testDir: '.', testMatch: 'independent-probes.spec.ts',
  outputDir: '/private/tmp/justixauto-t035-independent-results',
  webServer: {
    command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 4196 --strictPort',
    url: 'http://127.0.0.1:4196/docs/justix-auto/dev/qa/T-034/developer-fixture.html', cwd: process.cwd(), reuseExistingServer: false,
  },
});
