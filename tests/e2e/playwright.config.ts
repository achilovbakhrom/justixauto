import { defineConfig } from '@playwright/test';

// Feature tasks add their own journeys here; an empty suite is not a pass.
// Servers and synthetic reset adapters are explicit caller-owned setup.
export default defineConfig({
  testDir: '.',
  testMatch: '**/*.spec.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  timeout: 30_000,
  outputDir: '../../test-results/e2e',
  reporter: [['list']],
  use: {
    browserName: 'chromium',
    locale: 'ru-RU',
    timezoneId: 'Asia/Tashkent',
    colorScheme: 'light',
    reducedMotion: 'reduce',
    deviceScaleFactor: 1,
    serviceWorkers: 'block',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium-desktop', use: { viewport: { width: 1440, height: 1000 } } },
    { name: 'chromium-narrow', use: { viewport: { width: 600, height: 1000 } } },
  ],
});
