import { existsSync, globSync, readFileSync, statSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

const root = fileURLToPath(new URL('..', import.meta.url));
const manifest = JSON.parse(readFileSync(resolve(root, 'package.json'), 'utf8')) as {
  workspaces?: unknown;
};
if (
  !Array.isArray(manifest.workspaces) ||
  !manifest.workspaces.every((pattern): pattern is string => typeof pattern === 'string')
) {
  throw new Error('The root package.json must declare npm workspace directory patterns.');
}

// Discover package manifests first: a config-only glob could silently omit an
// entire workspace while another project's tests still produce a green run.
const workspaceManifests = globSync(
  manifest.workspaces.map((pattern) => `${pattern}/package.json`),
  { cwd: root },
).sort();
if (workspaceManifests.length === 0) {
  throw new Error('No npm workspaces found; application tests are not available yet.');
}
const projects = workspaceManifests.map((packagePath) => {
  const workspace = dirname(packagePath);
  const configs = ['vitest.config.ts', 'vitest.config.mts']
    .map((name) => resolve(root, workspace, name))
    .filter((path) => existsSync(path) && statSync(path).isFile());
  const [config] = configs;
  if (!config) {
    throw new Error(
      `Missing Vitest project config for npm workspace "${workspace}": expected vitest.config.ts or vitest.config.mts.`,
    );
  }
  if (configs.length !== 1) {
    throw new Error(
      `Ambiguous Vitest project configs for npm workspace "${workspace}": keep exactly one vitest.config.ts or vitest.config.mts.`,
    );
  }
  return config;
});

// Historical filename; Vitest consumes this as an explicit config, not
// through the removed defineWorkspace API. Each workspace supplies its own
// vitest.config.ts/mts (including environment and setup) with a unique name.
// The runner loader handles ESM config without changing the CommonJS root
// package required by the existing reference tests.
export default defineConfig({
  root,
  test: {
    passWithNoTests: false,
    projects,
    coverage: {
      provider: 'v8',
      include: ['web/{apps,packages}/*/src/**/*.{ts,tsx}'],
      exclude: ['**/*.d.ts', '**/*.{test,spec}.{ts,tsx}', '**/__tests__/**'],
      reporter: ['text', 'lcov'],
    },
  },
});
