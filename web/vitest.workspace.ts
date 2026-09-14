import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

// Historical filename; Vitest consumes this as an explicit config, not
// through the removed defineWorkspace API. Each workspace supplies its own
// vitest.config.ts/mts (including environment and setup) with a unique name.
// The runner loader handles ESM config without changing the CommonJS root
// package required by the existing reference tests.
export default defineConfig({
  root: fileURLToPath(new URL('..', import.meta.url)),
  test: {
    passWithNoTests: false,
    projects: ['web/{apps,packages}/*/vitest.config.{ts,mts}'],
    coverage: {
      provider: 'v8',
      include: ['web/{apps,packages}/*/src/**/*.{ts,tsx}'],
      exclude: ['**/*.d.ts', '**/*.{test,spec}.{ts,tsx}', '**/__tests__/**'],
      reporter: ['text', 'lcov'],
    },
  },
});
