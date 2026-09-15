import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    name: '@justixauto/api',
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
