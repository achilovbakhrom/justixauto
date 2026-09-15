import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  test: { name: '@justixauto/financing', environment: 'jsdom', include: ['src/**/*.test.tsx'], passWithNoTests: false },
});
