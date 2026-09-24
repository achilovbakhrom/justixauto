import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  test: {
    name: '@justixauto/realization',
    environment: 'jsdom',
    include: ['src/**/*.test.tsx'],
    passWithNoTests: false,
  },
});
