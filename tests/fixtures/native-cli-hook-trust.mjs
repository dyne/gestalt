import assert from 'node:assert/strict';

/** Explicit isolated-fixture authorization, never an operator trust policy.
 * Every current hook must match one verified generated command/source/event;
 * unknown hooks, changed commands and duplicate keys abort before trust writes.
 */
export function authorizeFixtureHooks(hooks, expected) {
  assert.equal(hooks.length, expected.length, 'Unexpected hooks in the isolated fixture');
  const keys = new Set();
  return expected.map(entry => {
    const selected = hooks.filter(hook => hook.source === 'user' && hook.sourcePath === entry.sourcePath &&
      hook.eventName === entry.eventName && hook.command === entry.command);
    assert.equal(selected.length, 1, `Unverified fixture hook: ${entry.eventName}`);
    const hook = selected[0];
    assert.equal(hook.enabled, true); assert.match(hook.currentHash, /^sha256:[a-f0-9]{64}$/);
    assert.equal(keys.has(hook.key), false, 'Duplicate native hook key'); keys.add(hook.key);
    return { ...entry, key: hook.key, currentHash: hook.currentHash };
  });
}
