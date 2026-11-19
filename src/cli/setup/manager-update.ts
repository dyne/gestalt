import { chmod, lstat, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { digest, readProfile } from './profile-files.ts';
import { CommandFailure, SetupContext } from './context.ts';
import { assertStableParents } from '../platform/namespace.ts';
import { NativePrivateStorage } from '../platform/storage.ts';
import { windowsSystem } from '../platform/windows-system.ts';

/** A replacement hands its original argv to the new host-side API once during
 * normal control flow. A crash never implies that continuation completed. */
export async function updateManager(context: SetupContext, originalArgv: readonly string[], resumed: boolean): Promise<boolean> {
  if (context.environment.GESTALT_MANAGER_DISTRIBUTION === 'npm') {
    await context.log('manager is installed with npm; update it with: npm install -g @jaromil/gestalt@latest');
    return false;
  }
  if (resumed) return false;
  const target = context.paths.manager, directory = path.dirname(target);
  await assertStableParents(target, context.paths.unixProbe, context.windowsReadHelper);
  const current = await readProfile(target);
  if (!current) throw new Error(`could not resolve manager executable: ${target}`);
  // A user executable directory may intentionally be 0755. Keep staging private
  // without changing that directory or weakening private-storage validation.
  const lockRoot = path.join(directory, '.gestalt-manager-update.lock');
  if (process.platform === 'win32') await windowsSystem({ operation: 'create-directory', path: lockRoot });
  else await mkdir(lockRoot, { mode: 0o700 });
  const storage = await NativePrivateStorage.open(lockRoot, context.paths.unixProbe, context.windowsReadHelper);
  const lockIdentity = await lstat(lockRoot, { bigint: true });
  const scratch = await mkdtemp(path.join(tmpdir(), 'gestalt-update.'));
  const unlisten = context.listen();
  let promoted = false, phase = 'download';
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
        (text.match(/^\/\/ GESTALT_CLI_VERSION='(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)'$/gm) ?? []).length !== 1 ||
        (text.match(/^\/\/ GESTALT_UPDATED_MANAGER_API='1'$/gm) ?? []).length !== 1)
      throw new Error('downloaded manager is missing its native version or host update API v1 declaration');
    const downloaded = path.join(scratch, 'gestalt.mjs');
    await writeFile(downloaded, bytes, { mode: 0o600 });
    phase = 'validating';
    const receipt = { schemaVersion: 1, target, argv: [...originalArgv], downloadedSha256: digest(bytes) };
    await storage.replaceFile(path.join(lockRoot, 'transaction.json'), Buffer.from(JSON.stringify({ ...receipt, phase, promoted }) + '\n'));
    await context.run(process.execPath, ['--check', downloaded]);
    context.check();
    if (current.bytes.equals(bytes)) { await context.log(`manager is already current at ${target}`); return false; }
    const staged = path.join(storage.root, 'gestalt.mjs');
    await storage.replaceFile(staged, bytes);
    await assertStableParents(target, context.paths.unixProbe, context.windowsReadHelper);
    const rechecked = await readProfile(target);
    if (!rechecked || rechecked.dev !== current.dev || rechecked.ino !== current.ino || !rechecked.bytes.equals(current.bytes))
      throw new Error('manager changed during update; installed executable retained');
    context.check();
    if (process.platform === 'win32') await windowsSystem({ operation: 'replace', source: staged, path: target });
    else { await chmod(staged, current.mode); await rename(staged, target); }
    promoted = true; phase = 'installed-pending-continuation';
    await storage.replaceFile(path.join(lockRoot, 'transaction.json'), Buffer.from(JSON.stringify({ ...receipt, phase, promoted }) + '\n'));
    await context.log(`updated manager at ${target}; continuing update with the newly installed manager`);
    context.check();
    // Load the replaced source in this host namespace. The API owns no global
    // environment/signal swap and receives only the shared owned command port.
    const api = await import(`${pathToFileURL(target).href}?gestalt-update=${receipt.downloadedSha256}`);
    if (api.UPDATED_MANAGER_API_VERSION !== 1 || typeof api.continueUpdatedManager !== 'function' ||
        !['host-filesystem-v1', 'async-owned-commands-v1', 'cancellation-v1'].every(value => api.UPDATED_MANAGER_API_CAPABILITIES?.includes(value)))
      throw new Error('installed manager lacks the required host update API v1; executable retained for inspection');
    phase = 'continuing';
    const outcome = await api.continueUpdatedManager({ argv: ['__updated-manager', ...originalArgv], cwd: context.invocation.cwd,
      environment: { ...context.environment } }, context.streams, { commands: await context.managedCommands(), signal: context.signal });
    context.check();
    if (outcome?.apiVersion !== 1 || !Number.isInteger(outcome.status) || outcome.status < 0 || outcome.status > 255 ||
        !['released', 'unknown'].includes(outcome.ownership?.kind)) {
      context.retainDelegatedState();
      throw new Error('updated manager returned an invalid outcome; transaction requires inspection');
    }
    if (outcome.ownership.kind === 'unknown') context.retainDelegatedState();
    // Successful macOS command results remain distinct from unknown lifetime.
    if (outcome.status !== 0) throw new CommandFailure('updated manager continuation failed', outcome.status);
    if (outcome.ownership.kind === 'unknown' && process.platform !== 'darwin')
      throw new CommandFailure('updated manager continuation ownership is unknown; transaction retained');
    phase = 'continued';
    return true;
  } finally {
    try {
      await context.stopActive();
      const currentLock = await lstat(lockRoot, { bigint: true });
      if (currentLock.dev !== lockIdentity.dev || currentLock.ino !== lockIdentity.ino) throw new Error('manager update lock identity changed; retained');
      if (context.canRollback()) {
        await rm(scratch, { recursive: true, force: true });
        await rm(lockRoot, { recursive: true });
      } else {
        const unknown = Buffer.from(JSON.stringify({ schemaVersion: 1, phase, promoted, scratch, claims: context.unknownOwnership }) + '\n');
        await writeFile(path.join(scratch, 'ownership-unknown.json'), unknown, { mode: 0o600 });
        await storage.replaceFile(path.join(lockRoot, 'ownership-unknown.json'), unknown);
        await context.log(`manager update subprocess cleanup is unknown; update lock and temporary download retained at ${lockRoot} and ${scratch}`);
      }
    } finally {
      unlisten();
    }
  }
}
