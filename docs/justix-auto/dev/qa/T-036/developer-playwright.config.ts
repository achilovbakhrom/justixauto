import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: '.', testMatch: 'developer-browser.spec.ts', workers: 1, retries: 0, forbidOnly: true,
  outputDir: '/private/tmp/justixauto-t036-browser-results',
  use: { browserName: 'chromium', locale: 'ru-RU', timezoneId: 'Asia/Tashkent' },
  projects: [1440, 600, 580].map((width) => ({ name: `width-${width}`, use: { viewport: { width, height: 1000 } } })),
  webServer: [
    { command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 4196 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4196/docs/justix-auto/dev/qa/T-036/developer-fixture.html', reuseExistingServer: false },
    { command: 'npm run dev --workspace @justixauto/admin -- --port 4197 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4197/admin/', reuseExistingServer: false },
    { command: 'node node_modules/vite/bin/vite.js preview --config web/apps/admin/vite.config.ts --host 127.0.0.1 --port 4198 --strictPort', cwd: process.cwd(), url: 'http://127.0.0.1:4198/admin/', reuseExistingServer: false },
  ],
});
