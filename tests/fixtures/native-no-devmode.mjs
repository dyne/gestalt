// Disposable ordinary-user CI check after explicit Developer Mode DWORD0 readback.
// This negative capability observation supplements actual OFF state; it never
// substitutes a token-only assertion for changing/observing the host state.
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, symlinkSync, unlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
const directory = mkdtempSync(path.join(tmpdir(), 'gestalt-no-devmode-'));
const target = path.join(directory, 'target'), link = path.join(directory, 'link');
writeFileSync(target, 'owned disposable fixture');
let created = false, failure = null;
try { symlinkSync(target, link, 'file'); created = true; }
catch (error) { failure = { code: error.code, errno: error.errno, syscall: error.syscall }; }
finally {
  if (created) unlinkSync(link);
  rmSync(directory, { recursive: true });
}
writeFileSync(process.argv[2], JSON.stringify({ observedAt: new Date().toISOString(), unprivilegedSymlinkCreated: created,
  failure, scope: 'Actual ordinary-token capability check after host DWORD0; no product runtime/compiler/administrator dependency' }, null, 2) + '\n');
assert.equal(created, false, 'Unprivileged symlink unexpectedly available; no Developer-Mode-OFF acceptance');
assert.equal(failure?.code, 'EPERM', 'Unexpected capability result; preserve evidence and do not infer OFF');
