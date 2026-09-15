import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: '.', testMatch: 'developer-r2-browser.spec.ts', workers: 1, retries: 0, forbidOnly: true,
  outputDir: '/private/tmp/justixauto-t037-r2-browser-results',
  use: { browserName: 'chromium', locale: 'ru-RU', timezoneId: 'Asia/Tashkent' },
  projects: [1440, 1180, 600].map((width) => ({ name: `width-${width}`, use: { viewport: { width, height: 1000 } } })),
  webServer: [
    { command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 4206 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4206/docs/justix-auto/dev/qa/T-037/developer-fixture.html', reuseExistingServer: false },
    { command: 'npm run dev --workspace @justixauto/realization -- --port 4207 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4207/', reuseExistingServer: false },
    { command: 'node node_modules/vite/bin/vite.js preview --config web/apps/realization/vite.config.ts --host 127.0.0.1 --port 4208 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4208/', reuseExistingServer: false },
  ],
});
