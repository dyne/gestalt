import assert from 'node:assert/strict';

const transactions = 'tests/native-setup-transactions.test.mjs';
const installed = 'tests/native-setup-installed.test.mjs';
export const portableFiles = ['tests/native-archive.test.mjs', 'tests/native-diagnostics.test.mjs', 'tests/native-skills-transfer.test.mjs'];
export const portableMacSkip = 'validated runtime-query environment preserves the real inherited v1 context and all TMP values without metadata writes';

const flags = ['FIXTURE_FAIL', 'FIXTURE_VERIFY_FAIL', 'FIXTURE_MARKETPLACE_FAIL'];
const publicNames = [
  ...['fresh profile', 'custom profile'].flatMap(profile => [
    'public Unix Bash installs, repeats and retains last successful provenance (' + profile + ')',
    ...flags.map(flag => 'public Unix Bash ' + flag + ' reaches its configured phase (' + profile + ')'),
  ]),
  ...['codex', 'gestalt'].map(home => 'public Unix Bash refuses unsafe existing ' + home + ' home with a selected native owner'),
  'public Unix Bash validates, replaces and reenters the packaged update asset',
  'managed JavaScript updated-manager continuation rejects invalid reentry without mutation',
];
const installedPublic = 'actual compiled Unix Bash replaces its packaged asset, provisions pinned Agents and repeats offline';
const installedManaged = 'actual macOS managed JavaScript API retains unknown authority from pinned Agents';

export const exactPattern = names => '^(' + names.map(name => name.replace(/[.*+?^$${}()|[\]\\]/g, '\\$&')).join('|') + ')$';

export function groups(platform) {
  assert.ok(platform === 'linux' || platform === 'darwin');
  const mac = platform === 'darwin';
  const success = mac ? 'retains unknown authority and refuses repeat' : 'repeats with stable config and provenance';
  const failure = mac ? 'retains unknown setup and' : 'restores';
  const managedNames = [
    ...['fresh', 'custom'].map(profile => 'managed JavaScript ' + profile + ' Agents setup ' + success),
    ...flags.flatMap(flag => ['absent', 'custom'].map(profile => flag + ' ' + failure + ' ' + profile + ' native profile without successful provenance')),
    'native setup preserves last successful provenance on a failed repeat',
    'native setup refuses an existing delegated-owner lock before modifying config',
    'native setup rejects invalid session context and unsupported options before mutation',
    ...['codex', 'gestalt'].map(home => 'install/update reject existing permissive ' + home + ' root without repair or subordinate mutation'),
    'npm-owned native manager update preserves package executable and retains component failure status',
    'managed JavaScript install/update preserves native skills, extra selection and optional Mobile policy',
  ];
  const selected = [
    { id: 'ordinary-public', file: transactions, names: publicNames, prerequisites: [] },
    { id: 'portable', files: portableFiles, count: 31, expectedSkips: mac ? [portableMacSkip] : [], prerequisites: [] },
    { id: 'ordinary-managed', file: transactions, names: managedNames, prerequisites: ['ancestry', 'managed-host'] },
    { id: 'installed-public', file: installed, names: [installedPublic], prerequisites: ['ancestry', 'codex'] },
  ];
  if (mac) selected.push({ id: 'installed-managed-macos', file: installed, names: [installedManaged], prerequisites: ['ancestry', 'managed-host', 'codex'] });
  return selected;
}
