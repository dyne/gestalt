import { randomUUID } from 'node:crypto';
import { cp, lstat, readFile, realpath, rename, rm } from 'node:fs/promises';
import path from 'node:path';
import { NativePaths } from '../platform/paths.ts';
import { commandTarget } from '../platform/executables.ts';
import { optionalStat, SetupContext } from './context.ts';

export function mobileHostSupport(platform = process.platform): { supported: boolean; prerequisite: string | null } {
  if (platform === 'linux' || platform === 'darwin') return { supported: true, prerequisite: null };
  return { supported: false, prerequisite: `a native ${platform} gestalt-mobile host with owned-process and session-scratch support is required` };
}
export async function installMobile(context: SetupContext, explicit = false): Promise<boolean> {
  const support = mobileHostSupport();
  if (!support.supported) {
    const message = `Mobile omitted: ${support.prerequisite}; Gestalt CLI and Agents remain available`;
    if (explicit) throw new Error(message);
    await context.log(message); return false;
  }
  await context.require('npm');
  const storage = await context.storage(context.paths.gestaltHome);
  const lock = await storage.acquireLock(path.join(storage.root, '.gestalt-mobile-install.lock'));
  const destination = path.join(storage.root, 'mobile'), stage = path.join(storage.root, `.mobile-${randomUUID()}`);
  const previous = path.join(storage.root, `.mobile-previous-${randomUUID()}`);
  let movedPrevious = false, activated = false;
  let retainLocks = false;
  const unlisten = context.listen();
  try {
    const old = await optionalStat(destination);
    if (old) {
      if (!old.isDirectory() || old.isSymbolicLink()) throw new Error('Mobile package root must be a directory without links');
      await storage.assertPrivate(destination, 'directory');
      const paths = new NativePaths();
      // Preserve unrelated files while refusing any link that redirects a package
      // update outside the installation. npm's own internal .bin links are valid.
      await cp(destination, stage, { recursive: true, dereference: false, verbatimSymlinks: true,
        filter: async source => { await paths.assertContained(destination, await realpath(source)); return true; } });
    } else await storage.createDirectory(stage);
    const spec = context.environment.GESTALT_MOBILE_PACKAGE || `gestalt-mobile@${context.environment.GESTALT_MOBILE_VERSION || 'latest'}`;
    await context.log(`installing ${spec} under ${destination}`);
    await context.run('npm', ['install', '--prefix', stage, '--no-audit', '--no-fund', '--save-exact', spec]);
    const binary = path.join(stage, 'node_modules/.bin', process.platform === 'win32' ? 'gestalt-mobile.cmd' : 'gestalt-mobile');
    if (!(await optionalStat(binary))) throw new Error(`npm completed but the Mobile executable is missing: ${binary}`);
    await commandTarget(binary);
    const manifest = JSON.parse(await readFile(path.join(stage, 'node_modules/gestalt-mobile/package.json'), 'utf8')) as { name?: string; os?: string[] };
    if (manifest.name !== 'gestalt-mobile') throw new Error('unexpected Mobile package identity');
    if (manifest.os && (!manifest.os.includes(process.platform) || manifest.os.includes(`!${process.platform}`)))
      throw new Error(`installed Mobile package does not support ${process.platform}`);
    context.check();
    if (old) { await rename(destination, previous); movedPrevious = true; }
    await rename(stage, destination); activated = true;
    if (movedPrevious && context.canRollback()) await rm(previous, { recursive: true });
    return true;
  } catch (error) {
    if (!context.canRollback()) {
      retainLocks = true;
      await context.log(`Mobile update cleanup is unconfirmed; installation stage, previous package and lock retained at ${stage}`);
      throw error;
    }
    if (movedPrevious && !activated) await rename(previous, destination);
    throw error;
  } finally {
    unlisten();
    if (!activated && !retainLocks) await rm(stage, { recursive: true, force: true });
    if (!retainLocks) await lock.release();
  }
}
