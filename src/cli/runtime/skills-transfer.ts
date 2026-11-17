import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { cp, link, lstat, mkdir, readFile, readdir, realpath, rename, rm, symlink, unlink, writeFile } from 'node:fs/promises';
import { NativeArchives } from '../platform/archive.ts';
import { assertStableParents } from '../platform/namespace.ts';
import { optionalStat, type SetupContext } from '../setup/context.ts';

const archiveRoot = 'gestalt-skills';
const skillName = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
const profileName = /^[A-Za-z0-9][A-Za-z0-9._-]*\.ya?ml$/;
const categories = ['user-agents-skills', 'managed-skills', 'codex-skills', 'skill-profiles'] as const;
function roots(context: SetupContext): string[] {
  return [path.join(context.paths.userHome, '.agents/skills'), path.join(context.paths.gestaltHome, '.agents/skills'),
    path.join(context.paths.codexHome, 'skills'), path.join(context.paths.gestaltHome, 'skill-profiles')];
}
const within = (root: string, candidate: string): boolean => {
  const relative = path.relative(root, candidate);
  return !path.isAbsolute(relative) && relative !== '..' && !relative.startsWith('..' + path.sep);
};
async function ordinaryTree(root: string): Promise<void> {
  const info = await lstat(root);
  if (info.isSymbolicLink() || (!info.isDirectory() && !info.isFile()) || (info.isFile() && info.nlink !== 1)) throw new Error(`skill contains an unsupported link or special file: ${root}`);
  if (/[\x00-\x1f\x7f]/.test(root)) throw new Error('skill archive path contains control characters');
  if (info.isDirectory()) for (const entry of await readdir(root)) await ordinaryTree(path.join(root, entry));
}
async function copyTree(source: string, destination: string): Promise<void> {
  await ordinaryTree(source);
  await cp(source, destination, { recursive: true, dereference: false, errorOnExist: false, force: true,
    filter: async file => { const info = await lstat(file); if (info.isSymbolicLink() || (!info.isDirectory() && !info.isFile())) throw new Error('source changed during transfer'); return true; } });
}
async function stage(context: SetupContext) {
  const storage = await context.storage(context.paths.gestaltHome);
  const codexStorage = await context.storage(context.paths.codexHome);
  const lock = await codexStorage.acquireLock(path.join(codexStorage.root, '.gestalt-skills-transfer.lock'));
  const directory = path.join(storage.root, `.skills-transfer-${randomUUID()}`);
  try {
    if (await optionalStat(path.join(codexStorage.root, '.gestalt-skills.lock'))) throw new Error('skills maintenance is active or retained; inspect its transaction before transfer');
    await storage.createDirectory(directory); return { storage, lock, directory };
  }
  catch (error) { await lock.release(); throw error; }
}

export async function exportSkills(context: SetupContext, argv: readonly string[]): Promise<void> {
  if (argv.length > 1) throw new Error('usage: gestalt skills-export [ARCHIVE]');
  const target = path.resolve(context.invocation.cwd, argv[0] || 'gestalt-skills.tar.gz');
  if (await optionalStat(target)) throw new Error(`refusing to overwrite skills archive: ${target}`);
  const parent = await realpath(path.dirname(target));
  if (parent !== path.dirname(target)) throw new Error('skills archive destination parent must be canonical');
  await assertStableParents(target, context.paths.unixProbe);
  const transaction = await stage(context), bundle = path.join(transaction.directory, archiveRoot);
  const destinations = roots(context); let skillCount = 0, profileCount = 0;
  const temporary = path.join(parent, `.gestalt-skills-${randomUUID()}.tar.gz`);
  try {
    await mkdir(bundle, { mode: 0o700 });
    for (let index = 0; index < categories.length; index++) {
      context.check(); const source = destinations[index]!, destination = path.join(bundle, categories[index]!);
      await mkdir(destination, { mode: 0o700 });
      const info = await optionalStat(source); if (!info) continue;
      if (!info.isDirectory() || info.isSymbolicLink()) throw new Error(`skill root must be a directory without links: ${source}`);
      for (const entry of (await readdir(source)).sort()) {
        context.check(); if (entry === '.system') continue;
        let file = path.join(source, entry), materialized = false;
        const item = await lstat(file);
        if (index === 3) {
          if (!/\.ya?ml$/.test(entry)) continue;
          if (!profileName.test(entry) || !item.isFile() || item.isSymbolicLink() || item.nlink !== 1) throw new Error(`invalid skill profile: ${file}`);
          await copyTree(file, path.join(destination, entry)); profileCount++; continue;
        }
        if (item.isSymbolicLink()) {
          const resolved = await realpath(file), collection = path.join(context.paths.codexHome, 'gestalt-skills/skills');
          const { readlink } = await import('node:fs/promises');
          if (index === 2 && resolved === path.join(collection, entry) && await readlink(file) === resolved && /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(entry)) {
            for (const directory of [path.dirname(collection), collection, resolved]) if ((await lstat(directory)).isSymbolicLink()) throw new Error('redirected managed collection');
            file = resolved; materialized = true;
          } else if (destinations.slice(0, 3).some(root => within(root, resolved))) continue;
          else throw new Error(`skill link points outside exported roots: ${file}`);
        } else if (!item.isDirectory()) continue;
        if (!skillName.test(entry) || !(await optionalStat(path.join(file, 'SKILL.md')))?.isFile()) throw new Error(`invalid skill: ${file}`);
        await copyTree(file, path.join(destination, entry));
        if (materialized && !await optionalStat(path.join(destination, entry, 'LICENSE')))
          await copyTree(path.join(context.paths.codexHome, 'gestalt-skills/LICENSE'), path.join(destination, entry, 'LICENSE'));
        skillCount++;
      }
    }
    const profiles = path.join(bundle, 'skill-profiles'), collection = path.join(context.paths.codexHome, 'gestalt-skills/skills');
    if (await optionalStat(collection)) for (const profile of await readdir(profiles)) {
      const file = path.join(profiles, profile);
      await writeFile(file, (await readFile(file, 'utf8')).replaceAll(await realpath(collection) + path.sep, destinations[2]! + path.sep), { mode: 0o600 });
    }
    await writeFile(path.join(bundle, 'manifest.json'), JSON.stringify({ format: archiveRoot, version: 1, managerVersion: context.version,
      sourceRoots: { userSkillsRoot: destinations[0], managedSkillsRoot: destinations[1], codexSkillsRoot: destinations[2] }, counts: { skills: skillCount, profiles: profileCount } }) + '\n', { mode: 0o600 });
    context.check(); await new NativeArchives().create(bundle, temporary); context.check();
    // Exclusive publication preserves a concurrently created operator archive.
    await link(temporary, target); await unlink(temporary);
    await context.log(`exported ${skillCount} skills and ${profileCount} profiles to ${target}`);
  } finally {
    await rm(temporary, { force: true }); await rm(transaction.directory, { recursive: true, force: true }); await transaction.lock.release();
  }
}

export async function importSkills(context: SetupContext, argv: readonly string[]): Promise<void> {
  if (argv.length !== 1) throw new Error('usage: gestalt skills-import ARCHIVE');
  const archive = path.resolve(context.invocation.cwd, argv[0]!), archives = new NativeArchives();
  const members = await archives.list(archive);
  if (members.some(member => member.path !== archiveRoot && member.path !== archiveRoot + '/manifest.json' &&
      !categories.some(category => member.path === archiveRoot + '/' + category || member.path.startsWith(archiveRoot + '/' + category + '/')))) throw new Error('unexpected skills archive member');
  const transaction = await stage(context), extracted = path.join(transaction.directory, 'extracted');
  const promotions: { target: string; previous: string; original: boolean; promoted: boolean; candidate: string }[] = [];
  const createdLinks: string[] = [];
  let committed = false, retain = false, skillCount = 0, profileCount = 0;
  try {
    await transaction.storage.createDirectory(extracted);
    await archives.extract(archive, extracted, archiveRoot);
    const bundle = path.join(extracted, archiveRoot), manifestFile = path.join(bundle, 'manifest.json');
    const manifest = JSON.parse(await readFile(manifestFile, 'utf8'));
    if (manifest.format !== archiveRoot || manifest.version !== 1 || !manifest.sourceRoots || typeof manifest.sourceRoots !== 'object') throw new Error('unsupported skills archive manifest');
    const destinations = roots(context), sources = categories.map(category => path.join(bundle, category));
    const replacements = [manifest.sourceRoots.userSkillsRoot, manifest.sourceRoots.managedSkillsRoot, manifest.sourceRoots.codexSkillsRoot].map((source, index) => {
      if (typeof source !== 'string' || (!path.posix.isAbsolute(source) && !path.win32.isAbsolute(source)) ||
          source === path.posix.parse(source).root || source === path.win32.parse(source).root || /[\x00-\x1f\x7f]/.test(source)) throw new Error('skills archive contains an invalid source root');
      return [source, destinations[index]!] as const;
    }).sort((a, b) => b[0].length - a[0].length);
    // Validate all archive payloads and existing destinations before promotion.
    for (let index = 0; index < sources.length; index++) {
      const source = sources[index]!; await ordinaryTree(source);
      for (const entry of await readdir(source)) {
        const file = path.join(source, entry), info = await lstat(file);
        if (index === 3) { if (!profileName.test(entry) || !info.isFile()) throw new Error('invalid skill profile archive entry'); }
        else if (!skillName.test(entry) || !info.isDirectory() || !(await optionalStat(path.join(file, 'SKILL.md')))?.isFile()) throw new Error('invalid skill in archive');
      }
      if (await optionalStat(destinations[index]!)) {
        await context.storage(destinations[index]!);
        for (const entry of await readdir(source)) {
          const target = path.join(destinations[index]!, entry);
          if (await optionalStat(target)) await ordinaryTree(target);
        }
      }
    }
    for (let index = 0; index < sources.length; index++) {
      context.check(); const destination = destinations[index]!, source = sources[index]!;
      const storage = await context.storage(destination);
      for (const entry of await readdir(source)) {
        context.check();
        const target = path.join(destination, entry), original = Boolean(await optionalStat(target));
        const candidate = path.join(destination, `.skills-import-${randomUUID()}`);
        const previous = path.join(destination, `.skills-import-previous-${randomUUID()}`);
        const record = { target, previous, original, promoted: false, candidate }; promotions.push(record);
        // Journal only selected entries; unrelated discovery links/data never
        // move and are never traversed or admitted as archive source payloads.
        await transaction.storage.replaceFile(path.join(transaction.directory, 'journal.json'), Buffer.from(JSON.stringify({ schemaVersion: 1, promotions }) + '\n'));
        if (index === 3) {
          let text = await readFile(path.join(source, entry), 'utf8');
          for (const [from, to] of replacements) text = text.replaceAll(from, to);
          await storage.replaceFile(candidate, Buffer.from(text)); profileCount++;
        } else {
          await storage.createDirectory(candidate);
          if (original) await copyTree(target, candidate);
          await copyTree(path.join(source, entry), candidate); skillCount++;
        }
        context.check();
        if (original) await rename(target, previous);
        try { await rename(candidate, target); record.promoted = true; }
        catch (error) { if (original) await rename(previous, target); throw error; }
        await transaction.storage.replaceFile(path.join(transaction.directory, 'journal.json'), Buffer.from(JSON.stringify({ schemaVersion: 1, promotions }) + '\n'));
      }
    }
    // Match legacy discovery: managed imports link only into absent Codex names.
    for (const entry of await readdir(sources[1]!)) {
      const target = path.join(destinations[2]!, entry);
      if (!await optionalStat(target)) { await symlink(path.join(destinations[1]!, entry), target, process.platform === 'win32' ? 'junction' : 'dir'); createdLinks.push(target); }
    }
    context.check(); committed = true;
    await context.log(`imported ${skillCount} skills and ${profileCount} profiles`);
  } catch (error) {
    try {
      for (const file of createdLinks) await unlink(file);
      for (const record of [...promotions].reverse()) {
        if (record.promoted) await rm(record.target, { recursive: true });
        if (await optionalStat(record.previous)) await rename(record.previous, record.target);
      }
    } catch { retain = true; await context.log(`skills import recovery needs inspection; exact journal and previous data retained at ${transaction.directory}`); }
    throw error;
  } finally {
    if (!retain) {
      for (const record of promotions) {
        await rm(record.candidate, { recursive: true, force: true });
        if (committed) await rm(record.previous, { recursive: true, force: true });
      }
      await rm(transaction.directory, { recursive: true, force: true }); await transaction.lock.release();
    }
  }
}
