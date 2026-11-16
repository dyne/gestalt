import { lstat, readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { captureCopyTree } from '../../scripts/native-copy-provenance.mjs';

const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const managerStage = new RegExp(`^\\.gestalt-setup\\.${uuid}$`);
const agentsStage = new RegExp(`^\\.gestalt-setup-state-${uuid}$`);
const json = async file => JSON.parse(await readFile(file, 'utf8'));

/** Inspect each existing owner's records without adopting or cleaning them.
 * In particular, .gestalt-setup.lock is not a manager snapshot. */
export async function captureSetupRetention(root) {
  const entries = await captureCopyTree(root);
  for (const entry of entries) {
    const info = await lstat(path.join(root, entry.path));
    Object.assign(entry, { mode: info.mode & 0o7777, dev: info.dev, ino: info.ino, mtimeMs: info.mtimeMs, ctimeMs: info.ctimeMs });
  }
  const managerSnapshots = [], agentsSnapshots = [];
  for (const entry of entries.filter(item => item.kind === 'directory' && !item.path.includes('/'))) {
    if (managerStage.test(entry.path)) managerSnapshots.push({ name: entry.path,
      snapshot: await json(path.join(root, entry.path, 'snapshot.json')),
      ownership: await json(path.join(root, entry.path, 'ownership-unknown.json')) });
    if (agentsStage.test(entry.path)) agentsSnapshots.push({ name: entry.path,
      owner: await json(path.join(root, entry.path, 'owner.json')) });
  }
  const lock = path.join(root, '.gestalt-setup.lock');
  const lockEntry = entries.find(entry => entry.path === '.gestalt-setup.lock');
  const agentsLock = lockEntry?.kind === 'directory'
    ? await Promise.all((await readdir(lock)).sort().map(async name => ({ name, record: await json(path.join(lock, name)) }))) : null;
  const managerLock = entries.some(entry => entry.path === '.gestalt-manager-setup.lock')
    ? await json(path.join(root, '.gestalt-manager-setup.lock')) : null;
  return { root, entries, managerSnapshots, agentsSnapshots, managerLock, agentsLock };
}
