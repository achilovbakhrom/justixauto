#!/usr/bin/env node
/**
 * Deliberately small Git surface for task agents.  This is a workflow guard,
 * not an authorization system: protected branches still require a human PR
 * approval and server-side branch protection.
 */
import { execFileSync } from 'node:child_process';
import { lstatSync, realpathSync } from 'node:fs';
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path';

const PROTECTED = new Set(['dev', 'main', 'master']);
const ALLOWED_BRANCH = /^(?:feature|task|fix|infra)\/[A-Za-z0-9][A-Za-z0-9._-]*$/;
const SHA = /^[0-9a-f]{40}$/;
const CONVENTIONAL = /^(?:feat|fix|chore|docs|refactor|test|build|ci)(?:\([A-Za-z0-9._/-]+\))?!?: .+$/;

export class GateError extends Error { constructor(message) { super(message); this.name = 'GateError'; } }

function git(cwd, args, { allowFailure = false } = {}) {
  try {
    return execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch (error) {
    if (allowFailure) return null;
    const detail = error.stderr?.toString().trim();
    throw new GateError(`git ${args[0]} failed${detail ? `: ${detail}` : ''}`);
  }
}

function requireString(value, name) {
  if (typeof value !== 'string' || !value) throw new GateError(`${name} is required`);
  return value;
}

function assertSha(value, name = 'SHA') {
  if (!SHA.test(requireString(value, name))) throw new GateError(`${name} must be a 40-character lowercase SHA`);
}

function assertBranch(branch) {
  requireString(branch, 'branch');
  if (PROTECTED.has(branch)) throw new GateError(`protected branch denied: ${branch}`);
  if (!ALLOWED_BRANCH.test(branch)) throw new GateError('branch must start feature/, task/, fix/, or infra/');
}

function currentBranch(repo) {
  const branch = git(repo, ['symbolic-ref', '--quiet', '--short', 'HEAD'], { allowFailure: true });
  if (!branch) throw new GateError('detached HEAD is denied');
  return branch;
}

/** Require the supplied directory itself to be the repository root. */
export function inspectRepo(cwd = process.cwd()) {
  const supplied = realpathSync(resolve(cwd));
  const root = git(supplied, ['rev-parse', '--show-toplevel']);
  const canonicalRoot = realpathSync(root);
  if (supplied !== canonicalRoot) throw new GateError(`repository must be invoked at its exact root: ${canonicalRoot}`);
  return { root: canonicalRoot, branch: currentBranch(canonicalRoot), head: git(canonicalRoot, ['rev-parse', 'HEAD']) };
}

function assertExpectedHead(repo, expectedHead) {
  if (expectedHead === undefined) throw new GateError('expected head is required');
  assertSha(expectedHead, 'expected head');
  const actual = git(repo, ['rev-parse', 'HEAD']);
  if (actual !== expectedHead) throw new GateError(`HEAD moved: expected ${expectedHead}, found ${actual}`);
  return actual;
}

function remoteBranchSha(repo, branch) {
  const output = git(repo, ['ls-remote', '--heads', 'origin', `refs/heads/${branch}`]);
  if (!output) return null;
  const [sha, ref] = output.split(/\s+/);
  if (ref !== `refs/heads/${branch}` || !SHA.test(sha)) throw new GateError('origin returned an invalid branch SHA');
  return sha;
}

function hasOrigin(repo) {
  return git(repo, ['remote', 'get-url', 'origin'], { allowFailure: true }) !== null;
}

function lstatOrNull(path) {
  try { return lstatSync(path); } catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}

function indexModes(repo, path) {
  const raw = git(repo, ['--literal-pathspecs', 'ls-files', '-s', '-z', '--', path]);
  return raw.split('\0').filter(Boolean).map((entry) => entry.split(/\s+/, 1)[0]);
}

function safePath(repo, item) {
  requireString(item, 'path');
  const pieces = item.split('/');
  if (isAbsolute(item) || item === '.' || item.includes('\\') || item.startsWith(':') || /[*?\[\]]/.test(item) ||
      pieces.includes('..') || pieces.some((piece) => piece.toLowerCase() === '.git') || item.includes('\0')) throw new GateError(`unsafe path: ${item}`);
  const normalized = resolve(repo, item);
  if (normalized === repo || !normalized.startsWith(`${repo}${sep}`) || relative(repo, normalized).split(sep).includes('..')) {
    throw new GateError(`path escapes repository: ${item}`);
  }
  let check = normalized;
  while (check !== repo) {
    if (lstatOrNull(check)?.isSymbolicLink()) throw new GateError(`symlink path denied: ${item}`);
    check = dirname(check);
  }
  const safe = relative(repo, normalized);
  const stat = lstatOrNull(normalized);
  if (stat && !stat.isFile()) throw new GateError(`path must be an exact regular file: ${item}`);
  const modes = indexModes(repo, safe);
  if (!stat && !modes.length) throw new GateError(`path must be an existing file or tracked deletion: ${item}`);
  if (modes.includes('120000')) throw new GateError(`index symlink denied: ${item}`);
  return safe;
}

function stagedPaths(repo) {
  const raw = git(repo, ['diff', '--cached', '--name-only', '-z']);
  return raw ? raw.split('\0').filter(Boolean) : [];
}

function assertOnlyScopedStaged(repo, scopes) {
  const staged = stagedPaths(repo);
  const unexpected = staged.filter((file) => !scopes.includes(file));
  if (unexpected.length) throw new GateError(`staged path outside requested scope: ${unexpected.join(', ')}`);
  for (const file of staged) {
    const absolute = resolve(repo, file);
    if (lstatOrNull(absolute)?.isSymbolicLink() || indexModes(repo, file).includes('120000')) {
      throw new GateError(`staged symlink denied: ${file}`);
    }
  }
  return staged;
}

function assertClean(repo) {
  if (git(repo, ['status', '--porcelain=v1'])) throw new GateError('base worktree must be clean before creating a worktree');
}

/** Make a named task worktree from dev; bootstrap never creates or moves dev. */
export function createWorktree({ repo = process.cwd(), branch, path, expectedDev, bootstrapBase } = {}) {
  const { root } = inspectRepo(repo);
  assertBranch(branch);
  assertClean(root);
  requireString(path, 'worktree path');
  const target = resolve(path);
  if (lstatOrNull(target)) throw new GateError(`worktree path already exists: ${target}`);
  const devExists = git(root, ['show-ref', '--verify', '--quiet', 'refs/heads/dev'], { allowFailure: true }) !== null;
  let base;
  if (!devExists) {
    if (!bootstrapBase) throw new GateError('dev is missing; first setup requires --bootstrap-base <40sha>');
    assertSha(bootstrapBase, 'bootstrap base');
    if (git(root, ['cat-file', '-e', `${bootstrapBase}^{commit}`], { allowFailure: true }) === null) {
      throw new GateError('bootstrap base is not a local commit');
    }
    if (hasOrigin(root) && remoteBranchSha(root, 'dev')) throw new GateError('bootstrap denied: origin/dev exists');
    base = bootstrapBase;
  } else if (bootstrapBase) {
    throw new GateError('bootstrap base is allowed only while dev is absent');
  } else {
    assertSha(expectedDev, 'expected dev');
    const localDev = git(root, ['rev-parse', 'refs/heads/dev']);
    if (localDev !== expectedDev) throw new GateError(`dev moved: expected ${expectedDev}, found ${localDev}`);
    if (hasOrigin(root)) {
      const remoteDev = remoteBranchSha(root, 'dev');
      if (remoteDev && remoteDev !== expectedDev) throw new GateError(`local dev is stale: origin has ${remoteDev}`);
    }
    base = expectedDev;
  }
  if (git(root, ['show-ref', '--verify', '--quiet', `refs/heads/${branch}`], { allowFailure: true }) !== null) {
    throw new GateError(`branch already exists: ${branch}`);
  }
  git(root, ['worktree', 'add', '--no-checkout', '-b', branch, target, base]);
  // A fresh worktree is deliberately checked out only after Git has created it.
  git(target, ['checkout']);
  return { branch, path: target, base: git(target, ['rev-parse', 'HEAD']) };
}

export function commitScoped({ repo = process.cwd(), paths, message, taskId, expectedHead } = {}) {
  const { root, branch } = inspectRepo(repo);
  assertBranch(branch);
  if (!Array.isArray(paths) || paths.length === 0) throw new GateError('one or more explicit --path values are required');
  const scopes = [...new Set(paths.map((item) => safePath(root, item)))];
  requireString(message, 'message');
  requireString(taskId, 'task ID');
  if (!CONVENTIONAL.test(message) || message.includes('\n')) throw new GateError('message must be a one-line conventional commit message');
  if (!message.includes(taskId)) throw new GateError('message must include the task or packet ID');
  assertExpectedHead(root, expectedHead);
  assertOnlyScopedStaged(root, scopes);
  git(root, ['--literal-pathspecs', 'add', '--', ...scopes]);
  const staged = assertOnlyScopedStaged(root, scopes);
  if (!staged.length) throw new GateError('no scoped changes staged');
  assertExpectedHead(root, expectedHead);
  git(root, ['commit', '-m', message]);
  return { branch, sha: git(root, ['rev-parse', 'HEAD']), paths: staged };
}

export function pushBranch({ repo = process.cwd(), expectedHead } = {}) {
  const { root, branch } = inspectRepo(repo);
  assertBranch(branch);
  const sha = assertExpectedHead(root, expectedHead);
  // The source is the validated immutable object, never a movable local ref.
  git(root, ['push', 'origin', `${sha}:refs/heads/${branch}`]);
  const remote = remoteBranchSha(root, branch);
  if (remote !== sha) throw new GateError(`origin SHA mismatch after push: expected ${sha}, found ${remote ?? 'missing'}`);
  return { branch, sha };
}

/** Read-only handoff data for a human-approved dev PR or integration. */
export function prepareDev({ repo = process.cwd(), expectedHead } = {}) {
  const { root, branch } = inspectRepo(repo);
  assertBranch(branch);
  const sourceSha = assertExpectedHead(root, expectedHead);
  const remoteDevSha = remoteBranchSha(root, 'dev');
  const range = remoteDevSha ? `${remoteDevSha}..${sourceSha}` : sourceSha;
  const changes = git(root, ['log', '--format=%H%x09%s', range]).split('\n').filter(Boolean)
    .map((line) => { const [sha, subject] = line.split('\t'); return { sha, subject }; });
  return { source: branch, sourceSha, remoteDevSha, changes };
}

function parse(argv) {
  const [command, ...rest] = argv;
  const data = { command, paths: [] };
  const names = new Map([['--repo', 'repo'], ['--branch', 'branch'], ['--path', 'path'], ['--message', 'message'], ['--task', 'taskId'], ['--expected-head', 'expectedHead'], ['--expected-dev', 'expectedDev'], ['--bootstrap-base', 'bootstrapBase']]);
  for (let index = 0; index < rest.length; index += 2) {
    const flag = rest[index]; const key = names.get(flag);
    if (!key || rest[index + 1] === undefined) throw new GateError(`unsupported or incomplete argument: ${flag ?? ''}`);
    if (key === 'path' && command === 'commit') data.paths.push(rest[index + 1]);
    else if (key === 'path' && command === 'create-worktree') data.path = rest[index + 1];
    else if (key === 'path') throw new GateError('--path is valid only for commit or create-worktree');
    else data[key] = rest[index + 1];
  }
  return data;
}

export function main(argv = process.argv.slice(2)) {
  const args = parse(argv);
  let result;
  if (args.command === 'inspect') result = inspectRepo(args.repo);
  else if (args.command === 'create-worktree') result = createWorktree(args);
  else if (args.command === 'commit') result = commitScoped(args);
  else if (args.command === 'push') result = pushBranch(args);
  else if (args.command === 'prepare-dev') result = prepareDev(args);
  else throw new GateError('commands: inspect, create-worktree, commit, push, prepare-dev');
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try { main(); } catch (error) { process.stderr.write(`agent-git: ${error.message}\n`); process.exitCode = 2; }
}
