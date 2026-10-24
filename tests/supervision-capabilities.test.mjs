import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

const cli = readFileSync(new URL('../public/gestalt', import.meta.url), 'utf8');
// Exercise the production offline reader and negotiation, without starting a session.
const functions = ['read_supervision_capabilities', 'has_supervision_capability',
  'check_supervision_capability_coherence'].map(name => {
  const match = cli.match(new RegExp(`^${name}\\(\\) \\{[\\s\\S]*?^\\}`, 'm'));
  assert.ok(match, name);
  return match[0];
}).join('\n');
const baseCapabilities = ['supervision-start', 'wait-lease-tool', 'checkpoint-tool',
  'agent-capacity-recovery', 'canonical-agent-identity', 'org-plan-contract'];
const manifest = (component, modern = true) => ({
  schemaVersion: 1, component, supervisionContract: 1,
  capabilities: [...baseCapabilities,
    ...(component === 'mobile' ? ['controller-status', 'session-verdict', 'acknowledgement-safe-composer'] : []),
    ...(modern ? ['l2-report-boundary', 'l1-model-policy-v1'] : [])],
  ...(modern ? { modelPolicyContract: { version: 1,
    scope: component === 'agents' ? 'schema-and-local-contracts' : 'schema-and-retention',
    runtimeEnforcement: false } } : {}),
});

function negotiate(agents, mobile) {
  const root = mkdtempSync(join(tmpdir(), 'gestalt-capabilities-'));
  try {
    for (const [name, value] of Object.entries({ agents, mobile })) {
      if (value !== undefined) writeFileSync(join(root, name), typeof value === 'string' ? value : JSON.stringify(value));
    }
    const result = spawnSync('bash', ['-c', `set -Eeuo pipefail
SUPERVISION_CAPABILITY_SCHEMA=1
SUPERVISION_CONTRACT_VERSION=1
agents_capability_manifest=$1/agents
mobile_capability_manifest=$1/mobile
print_check() { printf '%s: %s\\n' "$1" "$2"; }
${functions}
check_supervision_capability_coherence`, 'capability-test', root], { encoding: 'utf8' });
    assert.equal(result.signal, null);
    assert.equal(result.stderr, '');
    return result;
  } finally { rmSync(root, { recursive: true, force: true }); }
}

function expect(agents, mobile, status, message) {
  const result = negotiate(agents, mobile);
  assert.equal(result.status, status, result.stdout);
  assert.match(result.stdout, message);
  return result.stdout;
}

test('compatible trio supports diagnostics and explicitly disclaims automatic enforcement', () => {
  const output = expect(manifest('agents'), manifest('mobile'), 0, /L1 model policy: SUPPORTED/);
  assert.match(output, /runtimeEnforcement=false; automatic policy launches unsupported/);
});

const configured = component => ({ ...manifest(component), modelPolicyContract: {
  version: 1, scope: 'configured-launch-and-retention', defaultAssurance: 'configured',
  strictAttestation: false, runtimeEnforcement: false,
} });
test('configured pair admits compatibility without claiming strict attestation', () => {
  expect(configured('agents'), configured('mobile'), 0,
    /configured compatibility; explicit strict attestation unsupported; runtimeEnforcement=false/);
  expect(configured('agents'), manifest('mobile'), 1, /INCOMPATIBLE/);
  expect(manifest('agents'), configured('mobile'), 1, /INCOMPATIBLE/);
});
for (const key of ['defaultAssurance', 'strictAttestation']) {
  test(`configured support requires truthful ${key}`, () => {
    const mobile = configured('mobile');
    delete mobile.modelPolicyContract[key];
    expect(configured('agents'), mobile, 1, /INCOMPATIBLE/);
  });
}

test('legacy packages retain base supervision and coordinated rollback restores it', () => {
  expect(manifest('agents', false), manifest('mobile', false), 0, /L1 model policy: LEGACY/);
  expect(manifest('agents'), manifest('mobile'), 0, /L1 model policy: SUPPORTED/);
  expect(manifest('agents', false), manifest('mobile', false), 0, /L1 model policy: LEGACY/);
});

for (const component of ['agents', 'mobile']) {
  function pair(value) { return component === 'agents' ? [value, manifest('mobile')] : [manifest('agents'), value]; }
  test(`${component}: missing package metadata cannot advertise support`, () => {
    expect(...pair(undefined), 1, /UNAVAILABLE/);
  });
  for (const [name, invalid] of [['invalid JSON', '{'], ['null JSON', 'null'], ['oversized metadata', ' '.repeat(16385)]]) {
    test(`${component}: ${name} rejected`, () => expect(...pair(invalid), 1, /INCOMPATIBLE \(malformed/));
  }
  test(`${component}: old package in partial update or rollback is incompatible`, () => {
    expect(...pair(manifest(component, false)), 1, /L1 model policy: INCOMPATIBLE/);
  });
  for (const [name, change, malformed = false] of [
    ['wrong identity', value => { value.component = 'other'; }, true],
    ['old base contract', value => { value.supervisionContract = 0; }],
    ['unknown schema', value => { value.schemaVersion = 2; }, true],
    ['missing contract', value => { delete value.modelPolicyContract; }, true],
    ['undeclared contract', value => { value.capabilities = value.capabilities.filter(x => x !== 'l1-model-policy-v1'); }, true],
    ['future policy version', value => { value.modelPolicyContract.version = 2; }],
    ['future capability', value => { value.capabilities = value.capabilities.map(x => x === 'l1-model-policy-v1' ? 'l1-model-policy-v2' : x); }],
    ['wrong scope', value => { value.modelPolicyContract.scope = 'schema-only'; }],
    ['runtime claim', value => { value.modelPolicyContract.runtimeEnforcement = true; }],
    ['nonboolean runtime claim', value => { value.modelPolicyContract.runtimeEnforcement = 'false'; }, true],
    ['null policy', value => { value.modelPolicyContract = null; }, true],
    ['missing report boundary', value => { value.capabilities = value.capabilities.filter(x => x !== 'l2-report-boundary'); }],
  ]) {
    test(`${component}: ${name} rejected`, () => {
      const value = manifest(component);
      change(value);
      expect(...pair(value), 1, malformed ? /INCOMPATIBLE \(malformed/ : /INCOMPATIBLE/);
    });
  }
}

test('extra independent capabilities and duplicate declarations preserve compatibility', () => {
  const agents = manifest('agents');
  agents.capabilities.push('unrelated-feature', 'l1-model-policy-v1');
  expect(agents, manifest('mobile'), 0, /SUPPORTED/);
});

test('Bash syntax remains valid', () => {
  execFileSync('bash', ['-n', new URL('../public/gestalt', import.meta.url).pathname]);
});
