import assert from 'node:assert/strict';
import { chmod, cp, lstat, mkdir, mkdtemp, readFile, readdir, rm, symlink, unlink, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';
import { importSkills } from '../src/cli/runtime/skills-transfer.ts';
import { windowsSystem } from '../src/cli/platform/windows-system.ts';

// Controlled filesystem transaction seam. This suite verifies selection,
// journals and rollback; real native namespace/ACL admission remains mandatory
// in the installed-host suite and is never inferred from this fixture.
async function fixture(t) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt transfer logic ü ')); t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { userHome: path.join(root, 'home'), codexHome: path.join(root, 'Codex'), gestaltHome: path.join(root, 'Gestalt') };
  for (const directory of Object.values(paths)) await mkdir(directory, { mode: 0o700 });
  const events = []; let interrupted = false, interruptPromotion = false;
  const context = { paths, version: '0.16.1', invocation: { cwd: root }, check() { if (interrupted) throw new Error('controlled interruption after promotion'); }, log: async text => events.push(text),
    storage: async directory => {
      await mkdir(directory, { recursive: true, mode: 0o700 });
      return { root: directory, assertPrivate: async () => {},
        createDirectory: target => process.platform === 'win32' ? windowsSystem({ operation: 'create-directory', path: target }) : mkdir(target, { mode: 0o700 }),
        acquireLock: async target => { await writeFile(target, 'controlled lock', { flag: 'wx', mode: 0o600 }); return { release: () => unlink(target) }; },
        replaceFile: async (target, bytes) => { await writeFile(target, bytes, { mode: 0o600 });
          if (interruptPromotion && path.basename(target) === 'journal.json' && JSON.parse(bytes).promotions.some(entry => entry.promoted)) interrupted = true; },
      };
    } };
  const user = path.join(paths.userHome, '.agents/skills'), managed = path.join(paths.gestaltHome, '.agents/skills'), codex = path.join(paths.codexHome, 'skills'), profiles = path.join(paths.gestaltHome, 'skill-profiles');
  for (const directory of [user, managed, codex, profiles]) await mkdir(directory, { recursive: true, mode: 0o700 });
  for (const [directory, name] of [[user, 'user-skill'], [managed, 'managed-skill']]) { await mkdir(path.join(directory, name)); await writeFile(path.join(directory, name, 'SKILL.md'), '# ' + name); }
  await writeFile(path.join(profiles, 'default.yaml'), `skills:\n  - ${user}/user-skill\n  - ${managed}/managed-skill\n`);
  return { root, paths, context, user, managed, codex, profiles, events, interruptAfterPromotion() { interruptPromotion = true; } };
}
test('skills transfer round trip preserves unrelated entries and relocates profiles without invoking package contents', async t => {
  const source = await fixture(t), target = await fixture(t), archive = path.join(source.root, 'export.tar.gz');
  const resource = path.join(source.user, 'user-skill/resource');
  await writeFile(resource, `#!${process.execPath}\nprocess.stdout.write('imported-resource\\n');\n`);
  if (process.platform !== 'win32') await chmod(resource, 0o755);
  // The export command's namespace/ACL proof belongs to the installed native
  // suite. This transaction unit uses the same writer on an explicit bundle.
  const { NativeArchives } = await import('../src/cli/platform/archive.ts');
  const bundle = path.join(source.root, 'gestalt-skills'); await mkdir(bundle);
  for (const [directory, name] of [[source.user, 'user-agents-skills'], [source.managed, 'managed-skills'], [source.codex, 'codex-skills'], [source.profiles, 'skill-profiles']]) await cp(directory, path.join(bundle, name), { recursive: true });
  await writeFile(path.join(bundle, 'manifest.json'), JSON.stringify({ format: 'gestalt-skills', version: 1, sourceRoots: { userSkillsRoot: source.user, managedSkillsRoot: source.managed, codexSkillsRoot: source.codex } }));
  await new NativeArchives().create(bundle, archive);
  await mkdir(path.join(target.codex, 'unrelated')); await writeFile(path.join(target.codex, 'unrelated/operator'), 'keep');
  await importSkills(target.context, [archive]);
  assert.equal(await readFile(path.join(target.codex, 'unrelated/operator'), 'utf8'), 'keep');
  assert.equal(await readFile(path.join(target.user, 'user-skill/SKILL.md'), 'utf8'), '# user-skill');
  const importedResource = path.join(target.user, 'user-skill/resource');
  assert.deepEqual(await readFile(importedResource), await readFile(resource));
  if (process.platform !== 'win32') {
    assert.equal((await lstat(importedResource)).mode & 0o7777, 0o700);
    assert.equal(execFileSync(importedResource, [], { encoding: 'utf8', shell: false }), 'imported-resource\n');
  }
  assert.equal((await lstat(path.join(target.codex, 'managed-skill'))).isSymbolicLink(), true);
  const profile = await readFile(path.join(target.profiles, 'default.yaml'), 'utf8'); assert.ok(profile.includes(target.user)); assert.ok(!profile.includes(source.user));
  assert.ok(!(await readdir(target.paths.codexHome)).includes('.gestalt-skills-transfer.lock'));
});
test('skills import rejects a selected link collision and retains user content before any target promotion', async t => {
  const source = await fixture(t), target = await fixture(t), { NativeArchives } = await import('../src/cli/platform/archive.ts');
  const bundle = path.join(source.root, 'gestalt-skills'); await mkdir(bundle);
  for (const name of ['user-agents-skills', 'managed-skills', 'codex-skills', 'skill-profiles']) await mkdir(path.join(bundle, name));
  await mkdir(path.join(bundle, 'user-agents-skills/collision')); await writeFile(path.join(bundle, 'user-agents-skills/collision/SKILL.md'), 'imported');
  await writeFile(path.join(bundle, 'manifest.json'), JSON.stringify({ format: 'gestalt-skills', version: 1, sourceRoots: { userSkillsRoot: source.user, managedSkillsRoot: source.managed, codexSkillsRoot: source.codex } }));
  const outside = path.join(target.root, 'operator'); await mkdir(outside); await writeFile(path.join(outside, 'keep'), 'retain');
  await symlink(outside, path.join(target.user, 'collision'), process.platform === 'win32' ? 'junction' : 'dir');
  const archive = path.join(source.root, 'input.tar.gz'); await new NativeArchives().create(bundle, archive);
  await assert.rejects(importSkills(target.context, [archive]), /link or special file/);
  assert.equal(await readFile(path.join(outside, 'keep'), 'utf8'), 'retain'); assert.equal((await lstat(path.join(target.user, 'collision'))).isSymbolicLink(), true);
});
test('skills import compensates selected promoted entries after a controlled interruption', async t => {
  const source = await fixture(t), target = await fixture(t), { NativeArchives } = await import('../src/cli/platform/archive.ts');
  const bundle = path.join(source.root, 'gestalt-skills'); await mkdir(bundle);
  for (const [directory, name] of [[source.user, 'user-agents-skills'], [source.managed, 'managed-skills'], [source.codex, 'codex-skills'], [source.profiles, 'skill-profiles']]) await cp(directory, path.join(bundle, name), { recursive: true });
  await writeFile(path.join(bundle, 'manifest.json'), JSON.stringify({ format: 'gestalt-skills', version: 1, sourceRoots: { userSkillsRoot: source.user, managedSkillsRoot: source.managed, codexSkillsRoot: source.codex } }));
  await writeFile(path.join(target.user, 'user-skill/SKILL.md'), 'operator original');
  const archive = path.join(source.root, 'input.tar.gz'); await new NativeArchives().create(bundle, archive);
  target.interruptAfterPromotion(); await assert.rejects(importSkills(target.context, [archive]), /controlled interruption/);
  assert.equal(await readFile(path.join(target.user, 'user-skill/SKILL.md'), 'utf8'), 'operator original');
  assert.equal(await readFile(path.join(target.managed, 'managed-skill/SKILL.md'), 'utf8'), '# managed-skill');
  assert.ok((await readdir(target.user)).every(name => !name.startsWith('.skills-import')));
});
