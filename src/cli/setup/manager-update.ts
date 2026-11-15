import { chmod, lstat, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readProfile } from './profile-files.ts';
import { CommandFailure, SetupContext } from './context.ts';
import { assertStableParents } from '../platform/namespace.ts';
import { NativePrivateStorage } from '../platform/storage.ts';
import { windowsSystem } from '../platform/windows-system.ts';

/** A successful replacement resumes its original argv exactly once. The old
 * process returns the replacement's outcome and never runs old component code. */
export async function updateManager(context: SetupContext, originalArgv: readonly string[], resumed: boolean): Promise<boolean> {
  if (context.environment.GESTALT_MANAGER_DISTRIBUTION === 'npm') {
    await context.log('manager is installed with npm; update it with: npm install -g @jaromil/gestalt@latest');
    return false;
  }
  if (resumed) return false;
  const target = context.paths.manager, directory = path.dirname(target);
  await assertStableParents(target, context.paths.unixProbe);
  const current = await readProfile(target);
  if (!current) throw new Error(`could not resolve manager executable: ${target}`);
  // A user executable directory may intentionally be 0755. Keep staging private
  // without changing that directory or weakening private-storage validation.
  const lockRoot = path.join(directory, '.gestalt-manager-update.lock');
  if (process.platform === 'win32') await windowsSystem({ operation: 'create-directory', path: lockRoot });
  else await mkdir(lockRoot, { mode: 0o700 });
  const storage = await NativePrivateStorage.open(lockRoot, context.paths.unixProbe);
  const lockIdentity = await lstat(lockRoot, { bigint: true });
  const scratch = await mkdtemp(path.join(tmpdir(), 'gestalt-update.'));
  const unlisten = context.listen();
  let validated = false;
  try {
    const base = context.environment.GESTALT_INSTALL_BASE_URL || 'https://dyne.github.io/gestalt';
    const url = new URL(`${base.replace(/\/$/, '')}/gestalt.mjs`);
    await context.log(`checking for a manager update at ${url.href}`);
    let bytes: Buffer;
    if (url.protocol === 'file:') bytes = await readFile(fileURLToPath(url));
    else {
      if (!['https:', 'http:'].includes(url.protocol)) throw new Error('unsupported manager update URL');
      const response = await fetch(url, { signal: AbortSignal.timeout(60_000) });
      if (!response.ok) throw new Error(`manager download failed (HTTP ${response.status})`);
      const chunks: Buffer[] = []; let length = 0;
      if (!response.body) throw new Error('manager download has no body');
      for await (const chunk of response.body) {
        length += chunk.length;
        if (length > 32 * 1024 * 1024) { await response.body.cancel().catch(() => {}); throw new Error('manager download exceeds supported size'); }
        chunks.push(Buffer.from(chunk));
      }
      bytes = Buffer.concat(chunks);
    }
    if (bytes.length > 32 * 1024 * 1024) throw new Error('manager download exceeds supported size');
    const text = bytes.toString('utf8');
    if (!/^#!\/usr\/bin\/env node\r?\n/.test(text) ||
        (text.match(/^\/\/ GESTALT_CLI_VERSION='(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)'$/gm) ?? []).length !== 1)
      throw new Error('downloaded manager is missing its native version declaration');
    const downloaded = path.join(scratch, 'gestalt.mjs');
    await writeFile(downloaded, bytes, { mode: 0o600 });
    await context.run(process.execPath, ['--check', downloaded]);
    validated = true;
    if (current.bytes.equals(bytes)) { await context.log(`manager is already current at ${target}`); return false; }
    const staged = path.join(storage.root, 'gestalt.mjs');
    await storage.replaceFile(staged, bytes);
    await assertStableParents(target, context.paths.unixProbe);
    const rechecked = await readProfile(target);
    if (!rechecked || rechecked.dev !== current.dev || rechecked.ino !== current.ino || !rechecked.bytes.equals(current.bytes))
      throw new Error('manager changed during update; installed executable retained');
    if (process.platform === 'win32') await windowsSystem({ operation: 'replace', source: staged, path: target });
    else { await chmod(staged, current.mode); await rename(staged, target); }
    await context.log(`updated manager at ${target}; continuing update with the newly installed manager`);
  } finally {
    unlisten();
    if (context.canRollback()) await rm(scratch, { recursive: true, force: true });
    else {
      await writeFile(path.join(scratch, 'ownership-unknown.json'), JSON.stringify({ schemaVersion: 1, claims: context.unknownOwnership }) + '\n', { mode: 0o600 });
      await context.log(`manager update subprocess cleanup is unknown; temporary download retained at ${scratch}`);
    }
    const currentLock = await lstat(lockRoot, { bigint: true });
    if (currentLock.dev !== lockIdentity.dev || currentLock.ino !== lockIdentity.ino) throw new Error('manager update lock identity changed; retained');
    if (context.canRollback() || validated) await rm(lockRoot, { recursive: true });
  }
  try { await context.run(process.execPath, [target, '__updated-manager', ...originalArgv]); }
  catch (error) { if (error instanceof CommandFailure) throw error; throw new Error('could not resume the updated manager', { cause: error }); }
  return true;
}
