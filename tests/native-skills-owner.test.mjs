import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstat, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';
import { manageSkills } from '../src/cli/runtime/skills-owner.mjs';

async function fixture(t) {
  const root = await mkdtemp(path.join(tmpdir(), 'gestalt skills host ü '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = path.join(root, 'Codex'), output = [], calls = [], phases = [];
  const skill = Buffer.from('---\nname: native-fixture\ndescription: fixture\n---\nBinary-safe payload \0\xff');
  const hash = bytes => createHash('sha256').update(bytes).digest('hex');
  const manifest = Buffer.from(JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills', source: 'https://github.com/dyne/gestalt-skills', version: '0.1.0',
    skills: [{ name: 'native-fixture', entry: 'skills/native-fixture/SKILL.md', files: [{ path: 'skills/native-fixture/SKILL.md', sha256: hash(skill) }] }] }));
  const blobs = new Map([['manifest.json', manifest], ['LICENSE', Buffer.from('MIT fixture\n')], ['skills/native-fixture/SKILL.md', skill]]);
  const objects = new Map([...blobs].map(([file, bytes]) => [hash(bytes).slice(0, 40), { file, bytes }]));
  let absent = true, fail = false;
  const context = { paths: { userHome: root, codexHome: home }, environment: { HOME: root, CODEX_HOME: home, GESTALT_SKILLS_SOURCE: root, GESTALT_SKILLS_REF: 'main', GIT_ALTERNATE_OBJECT_DIRECTORIES: 'must-not-leak' },
    streams: { stdout: async bytes => output.push(Buffer.from(bytes)) }, check() { if (fail) throw new Error('controlled interruption'); },
    canRollback: () => absent,
    prepareHomes: async () => { phases.push('controlled-private-home-fixture'); await mkdir(home, { mode: 0o700 }); },
    async run(command, argv, options) {
      calls.push({ command, argv, options });
      assert.equal(command, 'git'); assert.equal(options.binary, true); assert.equal(options.capture, true);
      assert.equal(options.environment.GIT_ALTERNATE_OBJECT_DIRECTORIES, undefined);
      if (argv.includes('rev-parse')) return Buffer.from('a'.repeat(40) + '\n');
      if (argv.includes('ls-tree')) return Buffer.from([...objects].map(([oid, item]) => `100644 blob ${oid}\t${item.file}\0`).join(''));
      if (argv.includes('cat-file')) return objects.get(argv.at(-1)).bytes;
      return Buffer.alloc(0);
    } };
  return { root, home, output, calls, phases, skill, context, unknown() { absent = false; }, interrupt() { fail = true; } };
}
test('host skills transaction delegates binary-safe Git only and preserves exact receipt/discovery on status', async t => {
  const f = await fixture(t);
  assert.equal(await manageSkills(f.context, 'install'), 0); assert.equal(f.phases.length, 1);
  for (const operation of ['init', 'fetch', 'rev-parse', 'ls-tree', 'cat-file']) assert.ok(f.calls.some(call => call.argv.includes(operation)));
  assert.deepEqual(await readFile(path.join(f.home, 'gestalt-skills/skills/native-fixture/SKILL.md')), f.skill);
  assert.equal((await lstat(path.join(f.home, 'skills/native-fixture'))).isSymbolicLink(), true);
  assert.ok(!(await readdir(f.home)).includes('.gestalt-skills.lock'));
  const receiptFile = path.join(f.home, 'gestalt-skills/receipt.json'), before = await readFile(receiptFile), names = await readdir(f.home), calls = f.calls.length;
  assert.equal(await manageSkills(f.context, 'status', 'json'), 0);
  const report = JSON.parse(f.output.at(-1)); assert.equal(report.status, 'ready'); assert.equal(report.verifiedSource, false);
  assert.equal(await manageSkills(f.context, 'status', 'version'), 0);
  assert.match(f.output.at(-1).toString(), /^Gestalt Skills +0\.1\.0$/m);
  assert.deepEqual(await readFile(receiptFile), before); assert.deepEqual(await readdir(f.home), names); assert.equal(f.calls.length, calls);
});
test('host skills keeps transaction stage/lock on successful Git with unknown lifetime and refuses automatic repeat', async t => {
  const f = await fixture(t); f.unknown();
  assert.equal(await manageSkills(f.context, 'install'), 0);
  const lock = path.join(f.home, '.gestalt-skills.lock'), owner = JSON.parse(await readFile(path.join(lock, 'owner.json'), 'utf8'));
  assert.equal(owner.committed, true); assert.ok((await lstat(owner.stage)).isDirectory());
  f.context.prepareHomes = async () => {};
  const before = (await readdir(f.home)).sort(), calls = f.calls.length;
  await assert.rejects(manageSkills(f.context, 'update'), /transaction is retained/);
  assert.deepEqual((await readdir(f.home)).sort(), before); assert.equal(f.calls.length, calls);
  assert.deepEqual(JSON.parse(await readFile(path.join(lock, 'owner.json'), 'utf8')), owner);
});
test('host skills retained recovery uses current inventory token and explicit quiescence, never PID/age', async t => {
  const f = await fixture(t); f.unknown(); await manageSkills(f.context, 'install');
  await manageSkills(f.context, 'inspect-recovery', 'json'); const { token } = JSON.parse(f.output.at(-1));
  await assert.rejects(manageSkills(f.context, 'recover', 'json', { confirmed: false, token }), /confirmation/);
  const names = await readdir(f.home); assert.ok(names.includes('.gestalt-skills.lock'));
  await assert.rejects(manageSkills(f.context, 'recover', 'json', { confirmed: true, token: '0'.repeat(64) }), /current recovery token/);
  assert.equal(await manageSkills(f.context, 'recover', 'json', { confirmed: true, token }), 0);
  assert.equal(JSON.parse(f.output.at(-1)).status, 'recovered'); assert.ok(!(await readdir(f.home)).includes('.gestalt-skills.lock'));
  assert.deepEqual(await readFile(path.join(f.home, 'gestalt-skills/skills/native-fixture/SKILL.md')), f.skill);
});
test('host skills refuses local modified payload without replacing it', async t => {
  const f = await fixture(t); await manageSkills(f.context, 'install'); f.context.prepareHomes = async () => {};
  const file = path.join(f.home, 'gestalt-skills/skills/native-fixture/SKILL.md'); await writeFile(file, 'operator changes');
  await assert.rejects(manageSkills(f.context, 'update'), /local changes/); assert.equal(await readFile(file, 'utf8'), 'operator changes');
  assert.ok(!(await readdir(f.home)).includes('.gestalt-skills.lock'));
});
