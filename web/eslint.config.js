const { defineConfig, globalIgnores } = require('eslint/config');
const js = require('@eslint/js');
const ts = require('typescript-eslint');
const hooks = require('eslint-plugin-react-hooks');
const refresh = require('eslint-plugin-react-refresh').default;
const globals = require('globals');

// This config is explicitly loaded from the repository root. Keep linting
// scoped to web; the immutable reference bundle has its own verification.
module.exports = defineConfig([
  globalIgnores(['**/node_modules/**', '**/dist/**', '**/coverage/**']),
  {
    files: ['web/**/*.{js,cjs,mjs,ts,tsx,mts,cts}'],
    extends: [js.configs.recommended],
  },
  {
    files: ['web/**/*.{ts,tsx,mts,cts}'],
    extends: [ts.configs.recommended],
  },
  {
    files: ['web/{apps,packages}/*/src/**/*.{ts,tsx}'],
    languageOptions: { globals: globals.browser },
    plugins: { 'react-hooks': hooks },
    rules: hooks.configs.recommended.rules,
  },
  {
    files: ['web/apps/*/src/**/*.tsx'],
    plugins: { 'react-refresh': refresh },
    rules: {
      'react-refresh/only-export-components': ['error', { allowConstantExport: true }],
    },
  },
  {
    files: ['web/**/*.config.{js,cjs,mjs,ts,mts,cts}', 'web/vitest.workspace.ts'],
    languageOptions: { globals: globals.node },
  },
]);
