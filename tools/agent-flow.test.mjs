import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const script = path.resolve(path.dirname(fileURLToPath(import.meta.url)), 'agent-flow.mjs');
const hash = (body) => createHash('sha256').update(body).digest('hex');
const git = (root, args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8' }).trim();
function setup(t) {
  const box = fs.mkdtempSync(path.join(os.tmpdir(), 'agent-flow-cli-'));
  const repo = path.join(box, 'repo'); const plans = path.join(box, 'plans');
  fs.mkdirSync(repo); fs.mkdirSync(path.join(plans, 'briefs'), { recursive: true });
  fs.writeFileSync(path.join(repo, 'owned.txt'), 'first\n');
  execFileSync('git', ['init', '-q', repo]); git(repo, ['config', 'user.email', 'test@example.invalid']); git(repo, ['config', 'user.name', 'Test']);
  git(repo, ['add', 'owned.txt']); git(repo, ['commit', '-qm', 'base']);
  const base = git(repo, ['rev-parse', 'HEAD']); const briefBody = 'bounded packet brief\n'; const contractBody = 'contract reference\n';
  fs.writeFileSync(path.join(plans, 'briefs', 'brief.md'), briefBody); fs.writeFileSync(path.join(plans, 'contract.md'), contractBody);
  const ref = { path: 'briefs/brief.md', sha256: hash(briefBody) }; const contract = { path: 'contract.md', sha256: hash(contractBody) };
  const make = (task, packets) => ({ schema: 2, task_id: task, planner_actor: 'planner.a', hub_actor: 'hub.owner', hub: 'infrastructure', base_sha: base, contracts: [contract], requirements: ['REQ-ONE'], packets: packets.map((packet) => ({ ...packet, brief: ref, acceptance: ['REQ-ONE'], resources: packet.resources ?? [] })) });
  const main = make('HUB-CTRL', [
    { id: 'PKT-IMPL', kind: 'implementation', depends_on: [], owned_paths: ['owned.txt'], resources: ['browser'] },
    { id: 'PKT-QA', kind: 'qa', depends_on: ['PKT-IMPL'], owned_paths: [] },
    { id: 'PKT-INTEGRATION', kind: 'integration', depends_on: ['PKT-QA'], owned_paths: [], resources: ['postgres'] },
  ]);
  const other = make('HUB-OTHER', [{ id: 'PKT-OTHER', kind: 'qa', depends_on: [], owned_paths: [], resources: ['browser'] }]);
  fs.writeFileSync(path.join(plans, 'main.json'), JSON.stringify(main)); fs.writeFileSync(path.join(plans, 'other.json'), JSON.stringify(other));
  t.after(() => fs.rmSync(box, { recursive: true, force: true }));
  const run = (...args) => spawnSync(process.execPath, [script, ...args], { cwd: repo, encoding: 'utf8' });
  const ok = (...args) => { const result = run(...args); assert.equal(result.status, 0, `${args.join(' ')}\n${result.stderr}`); return JSON.parse(result.stdout); };
  const bad = (match, ...args) => { const result = run(...args); assert.notEqual(result.status, 0, args.join(' ')); assert.match(result.stderr, match); };
  const alternate = path.join(box, 'alternate');
  git(repo, ['worktree', 'add', '-q', '-b', 'agent-flow-alternate', alternate, 'HEAD']);
  return { repo, alternate, plans, run, ok, bad, base };
}
function approve(env, task, plan) {
  const initialized = env.ok('init', plan); env.ok('review-plan', '--task', task, '--digest', initialized.digest, '--actor', 'reviewer.plan', '--verdict', 'GREEN', '--evidence', 'plan checks pass'); env.ok('accept-hub', '--task', task, '--actor', 'hub.owner', '--evidence', 'hub accepts bounded task');
}

test('real CLI runs multi-task implementation, QA-only, integration, and aggregate workflow', (t) => {
  const env = setup(t);
  approve(env, 'HUB-CTRL', path.join(env.plans, 'main.json'));
  approve(env, 'HUB-OTHER', path.join(env.plans, 'other.json'));
  env.bad(/registered/, 'claim', 'implementation', '--task', 'HUB-CTRL', '--run', 'run-impl', '--actor', 'worker.a', '--worktree', env.repo);
  env.ok('register-worktree', '--worktree', env.repo);
  env.ok('register-worktree', '--worktree', env.alternate);
  env.bad(/planner cannot/, 'review-plan', '--task', 'HUB-CTRL', '--digest', '0'.repeat(64), '--actor', 'planner.a', '--verdict', 'GREEN', '--evidence', 'not independent');
  const claim = env.ok('claim', 'implementation', '--task', 'HUB-CTRL', '--run', 'run-impl', '--actor', 'worker.a', '--worktree', env.repo);
  assert.equal(claim.packet, 'PKT-IMPL');
  fs.appendFileSync(path.join(env.repo, 'owned.txt'), 'implementation\n'); git(env.repo, ['add', 'owned.txt']); git(env.repo, ['commit', '-qm', 'implementation']);
  const submitted = env.ok('submit', 'PKT-IMPL', '--task', 'HUB-CTRL', '--run', 'run-impl', '--actor', 'worker.a', '--worktree', env.repo);
  assert.equal(submitted.before, env.base);
  env.bad(/worktree already reserved/, 'claim', 'qa', '--task', 'HUB-OTHER', '--run', 'run-other', '--actor', 'worker.other', '--worktree', env.repo);
  env.bad(/no ready/, 'claim', 'qa', '--task', 'HUB-OTHER', '--run', 'run-other', '--actor', 'worker.other', '--worktree', env.alternate);
  env.ok('review', 'PKT-IMPL', '--task', 'HUB-CTRL', '--actor', 'reviewer.code', '--verdict', 'GREEN', '--evidence', 'review command pass', '--worktree', env.repo);
  env.ok('qa', 'PKT-IMPL', '--task', 'HUB-CTRL', '--actor', 'qa.code', '--verdict', 'GREEN', '--evidence', 'qa command pass', '--worktree', env.repo);
  env.ok('accept', 'PKT-IMPL', '--task', 'HUB-CTRL', '--actor', 'hub.owner', '--evidence', 'accept implementation', '--worktree', env.repo);
  env.ok('claim', 'qa', '--task', 'HUB-CTRL', '--run', 'run-qa', '--actor', 'qa.packet', '--worktree', env.repo);
  env.ok('verify', 'PKT-QA', '--task', 'HUB-CTRL', '--actor', 'qa.packet', '--verdict', 'GREEN', '--evidence', 'QA-only executable check pass', '--worktree', env.repo);
  env.ok('accept', 'PKT-QA', '--task', 'HUB-CTRL', '--actor', 'hub.owner', '--evidence', 'accept QA packet', '--worktree', env.repo);
  env.ok('claim', 'integration', '--task', 'HUB-CTRL', '--run', 'run-integration', '--actor', 'qa.integration', '--worktree', env.repo);
  env.ok('verify', 'PKT-INTEGRATION', '--task', 'HUB-CTRL', '--actor', 'qa.integration', '--verdict', 'GREEN', '--evidence', 'integration command pass', '--worktree', env.repo);
  env.ok('accept', 'PKT-INTEGRATION', '--task', 'HUB-CTRL', '--actor', 'hub.owner', '--evidence', 'accept integration', '--worktree', env.repo);
  const aggregate = env.ok('aggregate', '--task', 'HUB-CTRL', '--actor', 'vc.agent', '--evidence', 'all coverage verified', '--worktree', env.repo);
  assert.equal(aggregate.final_sha, git(env.repo, ['rev-parse', 'HEAD']));
  fs.appendFileSync(path.join(env.repo, 'owned.txt'), 'post acceptance change\n'); git(env.repo, ['add', 'owned.txt']); git(env.repo, ['commit', '-qm', 'post acceptance change']);
  env.bad(/not verified at current final HEAD/, 'aggregate', '--task', 'HUB-CTRL', '--actor', 'vc.agent.two', '--evidence', 'stale integration report', '--worktree', env.repo);
  env.ok('recover', 'PKT-INTEGRATION', '--task', 'HUB-CTRL', '--reason', 'reopen after final code change');
  env.ok('claim', 'integration', '--task', 'HUB-CTRL', '--run', 'run-integration-two', '--actor', 'qa.integration.two', '--worktree', env.repo);
  env.ok('verify', 'PKT-INTEGRATION', '--task', 'HUB-CTRL', '--actor', 'qa.integration.two', '--verdict', 'GREEN', '--evidence', 'rechecked final head', '--worktree', env.repo);
  env.ok('accept', 'PKT-INTEGRATION', '--task', 'HUB-CTRL', '--actor', 'hub.owner', '--evidence', 'accept refreshed integration', '--worktree', env.repo);
  env.ok('aggregate', '--task', 'HUB-CTRL', '--actor', 'vc.agent.two', '--evidence', 'refreshed final coverage', '--worktree', env.repo);
  const compact = env.ok('status', '--task', 'HUB-CTRL'); assert.equal(compact.packets['PKT-INTEGRATION'], 'accepted'); assert.equal(Object.hasOwn(compact, 'history'), false);
  const audit = env.ok('audit', '--task', 'HUB-CTRL'); assert.ok(audit.packets['PKT-IMPL'].history.length > 0);
  assert.equal(Object.keys(env.ok('status').tasks).length, 2);
});

test('CLI rejects dirty/stale state, holds submitted reservations, supports recovery, and detects final code changes', (t) => {
  const env = setup(t); approve(env, 'HUB-CTRL', path.join(env.plans, 'main.json')); env.ok('register-worktree', '--worktree', env.repo);
  env.bad(/duplicate flag/, 'status', '--task', 'HUB-CTRL', '--task', 'HUB-CTRL');
  env.bad(/unknown flag/, 'status', '--nope', 'x');
  env.ok('claim', 'implementation', '--task', 'HUB-CTRL', '--run', 'run-impl', '--actor', 'worker.a', '--worktree', env.repo);
  fs.appendFileSync(path.join(env.repo, 'owned.txt'), 'uncommitted\n'); env.bad(/clean/, 'submit', 'PKT-IMPL', '--task', 'HUB-CTRL', '--run', 'run-impl', '--actor', 'worker.a', '--worktree', env.repo);
  fs.writeFileSync(path.join(env.repo, 'owned.txt'), 'first\n'); env.ok('recover', 'PKT-IMPL', '--task', 'HUB-CTRL', '--reason', 'interrupted dirty attempt');
  assert.equal(env.ok('status', '--task', 'HUB-CTRL').packets['PKT-IMPL'], 'pending');
  env.ok('claim', 'implementation', '--task', 'HUB-CTRL', '--run', 'run-two', '--actor', 'worker.a', '--worktree', env.repo);
  fs.appendFileSync(path.join(env.repo, 'owned.txt'), 'committed\n'); git(env.repo, ['add', 'owned.txt']); git(env.repo, ['commit', '-qm', 'committed']);
  env.ok('submit', 'PKT-IMPL', '--task', 'HUB-CTRL', '--run', 'run-two', '--actor', 'worker.a', '--worktree', env.repo);
  env.ok('recover', 'PKT-IMPL', '--task', 'HUB-CTRL', '--reason', 'submitted checks interrupted');
  assert.equal(env.ok('status', '--task', 'HUB-CTRL').packets['PKT-IMPL'], 'pending');
  env.ok('claim', 'implementation', '--task', 'HUB-CTRL', '--run', 'run-three', '--actor', 'worker.a', '--worktree', env.repo);
  fs.appendFileSync(path.join(env.repo, 'owned.txt'), 'repair\n'); git(env.repo, ['add', 'owned.txt']); git(env.repo, ['commit', '-qm', 'repair']);
  const repaired = env.ok('submit', 'PKT-IMPL', '--task', 'HUB-CTRL', '--run', 'run-three', '--actor', 'worker.a', '--worktree', env.repo);
  assert.equal(repaired.before, env.base);
});
