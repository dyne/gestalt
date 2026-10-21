import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { parse } from 'smol-toml';

const fixtures = new URL('./fixtures/supervision-packages/', import.meta.url);
const provenance = JSON.parse(readFileSync(new URL('provenance.json', fixtures)));
const source = readFileSync(new URL('../public/gestalt', import.meta.url), 'utf8');
const functions = ['read_supervision_capabilities', 'has_supervision_capability',
  'check_supervision_capability_coherence'].map(name => {
  const match = source.match(new RegExp(`^${name}\\(\\) \\{[\\s\\S]*?^\\}`, 'm'));
  assert.ok(match, name); return match[0];
}).join('\n');
const manifest = name => JSON.parse(readFileSync(new URL(`${name}/capabilities.json`, fixtures)));

function isolated(run) {
  const root = mkdtempSync(join(tmpdir(), 'gestalt-packaged-supervision-'));
  try { return run(root); } finally { rmSync(root, { recursive: true, force: true }); }
}
function negotiate(agents, mobile) {
  return isolated(root => {
    writeFileSync(join(root, 'agents.json'), JSON.stringify(agents));
    writeFileSync(join(root, 'mobile.json'), JSON.stringify(mobile));
    const result = spawnSync('bash', ['-c', `set -Eeuo pipefail
SUPERVISION_CAPABILITY_SCHEMA=1
SUPERVISION_CONTRACT_VERSION=1
agents_capability_manifest=$1/agents.json
mobile_capability_manifest=$1/mobile.json
print_check() { printf '%s: %s\\n' "$1" "$2"; }
${functions}
check_supervision_capability_coherence`, 'package-test', root], { encoding: 'utf8' });
    assert.equal(result.stderr, ''); return result;
  });
}

test('fixture bytes and identities match the accepted package revisions', () => {
  assert.equal(provenance.components.agents.revision, '70c2014730f56952c37f397071e69e058cd70dd4');
  assert.equal(provenance.components.mobile.revision, '7989790b100fc61cd8444a739d7fe05cc18c1458');
  for (const component of Object.values(provenance.components)) {
    for (const file of component.files) {
      const actual = createHash('sha256').update(readFileSync(new URL(file.fixture, fixtures))).digest('hex');
      assert.equal(actual, file.sha256, file.fixture);
    }
  }
  assert.equal(JSON.parse(readFileSync(new URL('agents/package.json', fixtures))).name, 'gestalt');
  assert.equal(JSON.parse(readFileSync(new URL('mobile/package.json', fixtures))).name, 'gestalt-mobile');
  assert.equal(manifest('agents').component, 'agents');
  assert.equal(manifest('mobile').component, 'mobile');
  const result = negotiate(manifest('agents'), manifest('mobile'));
  assert.equal(result.status, 0, result.stdout);
  assert.match(result.stdout, /SUPPORTED .*runtimeEnforcement=false/);
});

test('actual pinned owner helper installs, repeats and regenerates its managed roles', () => isolated(root => {
  const agents = join(root, 'agents'); mkdirSync(agents);
  const unmanaged = 'model="gpt-6-luna"\n';
  writeFileSync(join(agents, 'custom.toml'), unmanaged);
  const run = () => {
    const result = spawnSync('bash', [new URL('agents/org-plan', fixtures).pathname,
      'prepare-supervision', '--agents-dir', agents], {
      env: { ...process.env, HOME: root, CODEX_HOME: join(root, 'codex') }, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    return Object.fromEntries(['executor', 'reviewer'].map(role => [role,
      readFileSync(join(agents, `org-plan-${role}.toml`), 'utf8')]));
  };
  const first = run();
  for (const [role, text] of Object.entries(first)) {
    const parsed = parse(text);
    assert.equal(parsed.model, 'gpt-6.1-sol');
    assert.equal(parsed.model_reasoning_effort, 'medium');
    assert.ok(parsed.developer_instructions.includes('org-plan'));
    if (role === 'reviewer') assert.equal(parsed.sandbox_mode, 'read-only');
  }
  assert.deepEqual(run(), first);
  writeFileSync(join(agents, 'org-plan-executor.toml'), 'model="manual-generated-role-edit"\n');
  writeFileSync(join(agents, 'org-plan-reviewer.toml'), 'model="manual-generated-role-edit"\n');
  assert.deepEqual(run(), first, 'explicit provider setup must retain documented regeneration behavior');
  assert.equal(readFileSync(join(agents, 'custom.toml'), 'utf8'), unmanaged);
}));

test('pinned contracts diagnose partial upgrade and coordinated legacy rollback', () => {
  const legacy = component => {
    const value = manifest(component); delete value.modelPolicyContract;
    value.capabilities = value.capabilities.filter(name => name !== 'l1-model-policy-v1'); return value;
  };
  assert.equal(negotiate(manifest('agents'), legacy('mobile')).status, 1);
  const rollback = negotiate(legacy('agents'), legacy('mobile'));
  assert.equal(rollback.status, 0, rollback.stdout);
  assert.match(rollback.stdout, /LEGACY/);
  assert.equal(negotiate(manifest('agents'), manifest('mobile')).status, 0);
});

for (const component of ['agents', 'mobile']) {
  for (const kind of ['identity', 'policy']) {
    test(`installed ${component} ${kind} mismatch cannot advertise policy support`, () => {
      const agents = manifest('agents'), mobile = manifest('mobile');
      const changed = component === 'agents' ? agents : mobile;
      if (kind === 'identity') changed.component = 'wrong-package';
      else changed.modelPolicyContract.runtimeEnforcement = 'false';
      const result = negotiate(agents, mobile);
      assert.equal(result.status, 1);
      assert.match(result.stdout, /INCOMPATIBLE \(malformed/);
    });
  }
}
