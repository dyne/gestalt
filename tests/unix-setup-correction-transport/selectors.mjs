import assert from 'node:assert/strict';

const transactions = 'tests/native-setup-transactions.test.mjs';
const installed = 'tests/native-setup-installed.test.mjs';
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
const managedComposition = 'managed JavaScript install/update preserves native skills, extra selection and optional Mobile policy';
const linuxManaged = [
  ...['fresh', 'custom'].map(profile => 'managed JavaScript ' + profile + ' Agents setup repeats with stable config and provenance'),
  ...flags.flatMap(flag => ['absent', 'custom'].map(profile => flag + ' restores ' + profile + ' native profile without successful provenance')),
  'native setup preserves last successful provenance on a failed repeat',
  managedComposition,
];

export const lanes = {
  'mac-bash': { target: 'darwin-arm64', label: 'macos-latest', imageOS: 'macos' },
  'linux-managed': { target: 'linux-x64', label: 'ubuntu-22.04', imageOS: 'ubuntu22', release: '22.04' },
  'linux-delivery': { target: 'linux-x64', label: 'ubuntu-latest', imageOS: 'ubuntu26', release: '26.04' },
};

export const exactPattern = names => '^(' + names.map(name => name.replace(/[.*+?^$${}()|[\]\\]/g, '\\$&')).join('|') + ')$';

export function groups(lane, shellNames) {
  const installedGroup = { id: 'installed-public', file: installed, names: [installedPublic], prerequisites: ['ancestry', 'codex', 'bash'] };
  switch (lane) {
    case 'mac-bash':
      assert.equal(shellNames.length, 24);
      assert.equal(new Set(shellNames).size, shellNames.length);
      return [
        { id: 'shell-contract', file: 'tests/setup-contract.test.mjs', names: shellNames, prerequisites: ['bash'] },
        { id: 'ordinary-public', file: transactions, names: publicNames, prerequisites: ['bash'] },
        installedGroup,
        { id: 'mac-managed-composition', file: transactions, names: [managedComposition], prerequisites: ['ancestry', 'bash'] },
      ];
    case 'linux-managed':
      return [{ id: 'linux-managed-retry', file: transactions, names: linuxManaged, prerequisites: ['ancestry', 'managed-host', 'bash'] }];
    case 'linux-delivery':
      return [installedGroup, { id: 'linux-package', prerequisites: [] }];
    default:
      throw new Error('Unknown focused setup lane: ' + lane);
  }
}
