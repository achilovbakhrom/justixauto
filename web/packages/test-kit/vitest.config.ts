import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  test: {
    name: '@justixauto/test-kit',
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    passWithNoTests: false,
  },
});
