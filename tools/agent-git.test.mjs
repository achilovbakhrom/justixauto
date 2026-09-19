import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, mkdtempSync, readFileSync, writeFileSync, mkdirSync, symlinkSync, unlinkSync, existsSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { GateError, commitScoped, createWorktree, inspectRepo, prepareDev, pushBranch } from './agent-git.mjs';

const CLI = resolve('tools/agent-git.mjs');
function run(cwd, args, ok = true) {
  try { const out = execFileSync('node', [CLI, ...args], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }); return JSON.parse(out); }
  catch (error) { if (ok) throw error; return error.stderr.toString(); }
}
function git(cwd, ...args) { return execFileSync('git', args, { cwd, encoding: 'utf8' }).trim(); }
function head(cwd) { return git(cwd, 'rev-parse', 'HEAD'); }
function repo({ dev = true, remote = true } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'agent-git-'));
  git(root, 'init', '-q'); git(root, 'config', 'user.email', 'test@example.test'); git(root, 'config', 'user.name', 'Test');
  writeFileSync(join(root, 'seed.txt'), 'seed\n'); git(root, 'add', 'seed.txt'); git(root, 'commit', '-qm', 'chore: seed');
  git(root, 'branch', '-M', 'main'); if (dev) git(root, 'branch', 'dev');
  if (remote) { const bare = mkdtempSync(join(tmpdir(), 'agent-git-remote-')); git(bare, 'init', '--bare', '-q'); git(root, 'remote', 'add', 'origin', bare); git(root, 'push', '-q', 'origin', 'main'); if (dev) git(root, 'push', '-q', 'origin', 'dev'); }
  return root;
}
function task(root, name = 'task/hub-git') { git(root, 'checkout', '-q', '-b', name, 'dev'); return root; }
function expectGate(fn, fragment) { assert.throws(fn, (error) => error instanceof GateError && error.message.includes(fragment)); }

test('requires the exact repository root', () => { const root = repo(); mkdirSync(join(root, 'nested')); expectGate(() => inspectRepo(join(root, 'nested')), 'exact root'); });

test('creates an explicit allowed worktree from dev and never falls back to main', () => {
  const root = repo(); const target = join(mkdtempSync(join(tmpdir(), 'agent-wt-parent-')), 'work');
  const result = createWorktree({ repo: root, branch: 'task/hub-git-worktree', path: target, expectedDev: git(root, 'rev-parse', 'dev') });
  assert.equal(result.branch, 'task/hub-git-worktree'); assert.equal(git(target, 'rev-parse', 'HEAD'), git(root, 'rev-parse', 'dev'));
  const missing = repo({ dev: false }); expectGate(() => createWorktree({ repo: missing, branch: 'task/no-dev', path: `${missing}-wt` }), 'dev is missing');
  const boot = createWorktree({ repo: missing, branch: 'task/bootstrap', path: `${missing}-boot`, bootstrapBase: git(missing, 'rev-parse', 'HEAD') });
  assert.equal(boot.branch, 'task/bootstrap'); assert.equal(git(missing, 'branch', '--list', 'dev'), '');
  writeFileSync(join(root, 'dirty.txt'), 'dirty');
  expectGate(() => createWorktree({ repo: root, branch: 'task/dirty', path: `${root}-dirty`, expectedDev: git(root, 'rev-parse', 'dev') }), 'must be clean');
});

test('create-worktree rejects stale local dev and bootstrap when origin/dev exists', () => {
  const stale = repo(); git(stale, 'checkout', '-q', 'dev'); writeFileSync(join(stale, 'remote-behind.txt'), 'x'); git(stale, 'add', 'remote-behind.txt'); git(stale, 'commit', '-qm', 'chore: advance dev'); git(stale, 'checkout', '-q', 'main');
  expectGate(() => createWorktree({ repo: stale, branch: 'task/stale', path: `${stale}-wt`, expectedDev: git(stale, 'rev-parse', 'dev') }), 'origin/dev must equal');
  const remoteDev = repo(); git(remoteDev, 'branch', '-D', 'dev');
  expectGate(() => createWorktree({ repo: remoteDev, branch: 'task/bootstrap-denied', path: `${remoteDev}-wt`, bootstrapBase: head(remoteDev) }), 'origin/dev exists');
  const missingRemoteDev = repo({ dev: false, remote: true }); git(missingRemoteDev, 'branch', 'dev');
  expectGate(() => createWorktree({ repo: missingRemoteDev, branch: 'task/remote-missing', path: `${missingRemoteDev}-wt`, expectedDev: git(missingRemoteDev, 'rev-parse', 'dev') }), 'origin/dev must equal');
});

test('CLI end-to-end creates, commits, pushes, and prepares an uppercase task branch', () => {
  const root = repo(); const target = join(mkdtempSync(join(tmpdir(), 'agent-cli-parent-')), 'T-933');
  const created = run(root, ['create-worktree', '--branch', 'task/T-933', '--expected-dev', git(root, 'rev-parse', 'dev'), '--path', target]);
  assert.equal(created.branch, 'task/T-933'); assert.equal(created.path, target);
  writeFileSync(join(target, 'cli.txt'), 'cli\n');
  const made = run(target, ['commit', '--expected-head', created.base, '--task', 'T-933', '--message', 'feat(T-933): CLI workflow', '--path', 'cli.txt']);
  const pushed = run(target, ['push', '--expected-head', made.sha]); assert.equal(pushed.sha, made.sha);
  const handoff = run(target, ['prepare-dev', '--expected-head', made.sha]);
  assert.equal(handoff.source, 'task/T-933'); assert.equal(handoff.sourceSha, made.sha); assert.ok(handoff.remoteDevSha);
});

test('commit is scoped, conventional, task-labelled, and rejects protected branches', () => {
  const root = task(repo()); writeFileSync(join(root, 'good.txt'), 'good\n');
  expectGate(() => commitScoped({ repo: root, paths: ['good.txt'], taskId: 'HUB-GIT', message: 'feat(HUB-GIT): add gate' }), 'expected head');
  const result = commitScoped({ repo: root, paths: ['good.txt', 'seed.txt'], taskId: 'HUB-GIT', message: 'feat(HUB-GIT): add gate', expectedHead: head(root) });
  assert.match(result.sha, /^[0-9a-f]{40}$/); assert.deepEqual(result.paths, ['good.txt']);
  writeFileSync(join(root, 'other.txt'), 'other\n'); git(root, 'add', 'other.txt'); writeFileSync(join(root, 'next.txt'), 'x\n');
  expectGate(() => commitScoped({ repo: root, paths: ['next.txt'], taskId: 'HUB-GIT', message: 'feat(HUB-GIT): next', expectedHead: head(root) }), 'outside requested');
  git(root, 'reset', '-q'); git(root, 'checkout', '-q', 'dev'); writeFileSync(join(root, 'dev.txt'), 'x');
  expectGate(() => commitScoped({ repo: root, paths: ['dev.txt'], taskId: 'HUB-GIT', message: 'feat(HUB-GIT): denied' }), 'protected');
});

test('a project pre-commit hook cannot add an out-of-scope file to the real commit', () => {
  const root = task(repo()); const before = head(root); writeFileSync(join(root, 'allowed.txt'), 'allowed\n');
  const hook = join(root, '.git', 'hooks', 'pre-commit');
  writeFileSync(hook, '#!/bin/sh\nprintf outside > outside.txt\ngit add outside.txt\n'); chmodSync(hook, 0o755);
  expectGate(() => commitScoped({ repo: root, paths: ['allowed.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): hook scope', expectedHead: before }), 'hook changed candidate outside');
  assert.equal(head(root), before); assert.equal(existsSync(join(root, 'outside.txt')), false);
  const staged = git(root, 'diff', '--cached', '--name-only'); assert.equal(staged, '');
});

test('a rejecting reference transaction hook leaves real bytes and modes untouched', () => {
  const root = task(repo()); const before = head(root); const file = join(root, 'allowed.txt');
  writeFileSync(file, 'raw\n'); chmodSync(file, 0o744); const originalMode = statSync(file).mode & 0o777;
  writeFileSync(join(root, '.git', 'hooks', 'pre-commit'), "#!/bin/sh\nprintf 'formatted\\n' > allowed.txt\ngit add allowed.txt\n");
  chmodSync(join(root, '.git', 'hooks', 'pre-commit'), 0o755);
  writeFileSync(join(root, '.git', 'hooks', 'reference-transaction'), "#!/bin/sh\nif [ \"$1\" = prepared ]; then\n  while read old new ref; do\n    [ \"$ref\" = refs/heads/task/hub-git ] && exit 1\n  done\nfi\nexit 0\n");
  chmodSync(join(root, '.git', 'hooks', 'reference-transaction'), 0o755);
  expectGate(() => commitScoped({ repo: root, paths: ['allowed.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): reject ref', expectedHead: before }), 'candidate was not committed');
  assert.equal(head(root), before); assert.equal(readFileSync(file, 'utf8'), 'raw\n'); assert.equal(statSync(file).mode & 0o777, originalMode);
});

test('candidate diff preserves literal Unicode and newline filenames', () => {
  const root = task(repo()); const name = 'café\nname.txt'; writeFileSync(join(root, name), 'x\n');
  const made = commitScoped({ repo: root, paths: [name], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): literal path', expectedHead: head(root) });
  assert.deepEqual(made.paths, [name]);
});

test('post-hook candidate types are checked and scoped hook formatting is retained', () => {
  const bad = task(repo()); const badHead = head(bad); writeFileSync(join(bad, 'allowed.txt'), 'plain\n');
  const badHook = join(bad, '.git', 'hooks', 'pre-commit');
  writeFileSync(badHook, '#!/bin/sh\nrm allowed.txt\nln -s missing allowed.txt\ngit add allowed.txt\n'); chmodSync(badHook, 0o755);
  expectGate(() => commitScoped({ repo: bad, paths: ['allowed.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): type', expectedHead: badHead }), 'non-file candidate');
  assert.equal(head(bad), badHead); assert.equal(existsSync(join(bad, 'allowed.txt')), true);

  const formatted = task(repo()); writeFileSync(join(formatted, 'allowed.txt'), 'raw\n');
  const formatHook = join(formatted, '.git', 'hooks', 'pre-commit');
  writeFileSync(formatHook, "#!/bin/sh\nprintf 'formatted\\n' > allowed.txt\ngit add allowed.txt\n"); chmodSync(formatHook, 0o755);
  const made = commitScoped({ repo: formatted, paths: ['allowed.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): format', expectedHead: head(formatted) });
  assert.equal(git(formatted, 'show', `${made.sha}:allowed.txt`), 'formatted'); assert.equal(git(formatted, 'status', '--porcelain'), '');
});

test('rejects traversal, pathspecs, directories, and index symlinks', () => {
  const root = task(repo()); writeFileSync(join(root, 'real.txt'), 'x'); symlinkSync('real.txt', join(root, 'link.txt'));
  expectGate(() => commitScoped({ repo: root, paths: ['../outside'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): bad' }), 'unsafe');
  expectGate(() => commitScoped({ repo: root, paths: ['link.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): bad' }), 'symlink');
  mkdirSync(join(root, 'nested')); expectGate(() => commitScoped({ repo: root, paths: ['nested'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): nested', expectedHead: head(root) }), 'exact regular');
  expectGate(() => commitScoped({ repo: root, paths: [':(glob)*'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): glob', expectedHead: head(root) }), 'unsafe');
  expectGate(() => commitScoped({ repo: root, paths: ['.GIT/config'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): git', expectedHead: head(root) }), 'unsafe');
  symlinkSync('missing-target', join(root, 'dangling.txt')); git(root, 'add', 'dangling.txt'); unlinkSync(join(root, 'dangling.txt'));
  expectGate(() => commitScoped({ repo: root, paths: ['dangling.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): dangling', expectedHead: head(root) }), 'index symlink');
});

test('push permits only an allowed current branch, checks HEAD, and disallows arbitrary refspec flags', () => {
  const root = task(repo()); writeFileSync(join(root, 'push.txt'), 'x'); const made = commitScoped({ repo: root, paths: ['push.txt'], taskId: 'HUB-GIT', message: 'fix(HUB-GIT): push', expectedHead: head(root) });
  assert.deepEqual(pushBranch({ repo: root, expectedHead: made.sha }), { branch: 'task/hub-git', sha: made.sha });
  writeFileSync(join(root, 'moved.txt'), 'x'); git(root, 'add', 'moved.txt'); git(root, 'commit', '-qm', 'chore: moved'); expectGate(() => pushBranch({ repo: root, expectedHead: made.sha }), 'HEAD moved');
  const denied = run(root, ['push', '--refspec', 'HEAD:refs/heads/dev'], false); assert.match(denied, /unsupported/);
  git(root, 'checkout', '-q', 'main'); assert.match(run(root, ['push', '--expected-head', git(root, 'rev-parse', 'HEAD')], false), /protected/);
});

test('prepare-dev is read-only and does not accept pretend approval flags', () => {
  const root = task(repo()); writeFileSync(join(root, 'review.txt'), 'x'); const made = commitScoped({ repo: root, paths: ['review.txt'], taskId: 'HUB-GIT', message: 'docs(HUB-GIT): review', expectedHead: head(root) });
  const before = git(root, 'status', '--porcelain=v1'); const data = prepareDev({ repo: root, expectedHead: made.sha });
  assert.equal(data.source, 'task/hub-git'); assert.equal(data.sourceSha, made.sha); assert.ok(data.remoteDevSha); assert.equal(git(root, 'status', '--porcelain=v1'), before);
  assert.match(run(root, ['prepare-dev', '--approval', 'human'], false), /unsupported/);
  assert.equal(existsSync(join(root, '.git', 'approval.json')), false);
});
