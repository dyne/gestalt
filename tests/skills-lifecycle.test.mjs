import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, writeFile, readFile, readdir, readlink, lstat, rm, symlink, link, rename } from 'node:fs/promises';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';

const wrapper = new URL('../public/gestalt', import.meta.url).pathname;
const canonicalSource = 'https://github.com/dyne/gestalt-skills';
const realGit = execFileSync('sh', ['-c', 'command -v git'], { encoding: 'utf8' }).trim();
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function waitFor(file) {
  for (let attempt = 0; attempt < 500; attempt++) {
    try { await lstat(file); return; } catch (error) { if (error.code !== 'ENOENT') throw error; }
    await pause(10);
  }
  assert.fail(`timed out waiting for ${file}`);
}
async function snapshot(root, prefix = '') {
  let entries;
  try { entries = await readdir(root); } catch (error) { if (error.code === 'ENOENT') return []; throw error; }
  const rows = [];
  for (const name of entries.sort()) {
    const file = join(root, name), relative = prefix ? `${prefix}/${name}` : name;
    const info = await lstat(file);
    if (info.isSymbolicLink()) rows.push([relative, 'link', await readlink(file)]);
    else if (info.isDirectory()) rows.push(...await snapshot(file, relative));
    else rows.push([relative, 'file', digest(await readFile(file)), info.mode & 0o777]);
  }
  return rows;
}
async function fixture(t) {
  const temp = await mkdtemp(join(tmpdir(), 'gestalt skills lifecycle '));
  const home = join(temp, 'home'), codex = join(home, 'custom codex'), gestalt = join(home, 'custom gestalt');
  const source = join(temp, 'release repository'), bin = join(temp, 'bin');
  const root = join(codex, 'gestalt-skills'), discovery = join(codex, 'skills');
  await mkdir(source, { recursive: true }); await mkdir(bin);
  await mkdir(join(discovery, '.system', 'system'), { recursive: true });
  await mkdir(join(discovery, 'unrelated'), { recursive: true });
  await mkdir(join(gestalt, 'skill-profiles'), { recursive: true });
  await writeFile(join(discovery, '.system', 'system', 'SKILL.md'), '# system');
  await writeFile(join(discovery, 'unrelated', 'SKILL.md'), '# unrelated');
  await writeFile(join(gestalt, 'skill-profiles', 'saved.yml'), `skills:\n  - path: ${root}/skills/nuklear-gui/SKILL.md\n    enabled: false\n`);
  const profile = await snapshot(gestalt);
  const git = args => execFileSync(realGit, ['-c', 'core.hooksPath=/dev/null', ...args], {
    cwd: source, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, GIT_AUTHOR_NAME: 'Fixture', GIT_AUTHOR_EMAIL: 'fixture@example.test', GIT_COMMITTER_NAME: 'Fixture', GIT_COMMITTER_EMAIL: 'fixture@example.test' },
  }).trim();
  git(['init', '--initial-branch=main', '--template=']);
  await writeFile(join(bin, 'git'), `#!${process.execPath}\nconst fs = require('node:fs');
const args = process.argv.slice(2).map(arg => arg === ${JSON.stringify(canonicalSource)} ? process.env.FIXTURE_SOURCE : arg);
async function run() {
  if (args.includes('fetch') && process.env.FIXTURE_GATE) {
    fs.writeFileSync(process.env.FIXTURE_GATE + '.waiting', String(process.pid));
    for (let i = 0; !fs.existsSync(process.env.FIXTURE_GATE); i++) {
      if (i > 1000) process.exit(19);
      await new Promise(resolve => setTimeout(resolve, 10));
    }
  }
  const result = require('node:child_process').spawnSync(${JSON.stringify(realGit)}, args, { stdio: 'inherit' });
  process.exit(result.status ?? 1);
}
run();\n`, { mode: 0o755 });
  let revision;
  async function release(skills = { 'nuklear-gui': 'first' }, change, rawTransform = text => text) {
    await rm(join(source, 'skills'), { recursive: true, force: true });
    const manifest = { schemaVersion: 1, repository: 'dyne/gestalt-skills', source: canonicalSource, version: '0.1.0', skills: [] };
    for (const name of Object.keys(skills).sort()) {
      const text = skills[name];
      const files = [
        [`skills/${name}/SKILL.md`, `---\nname: ${name}\ndescription: Fixture skill\n---\n${text}\n`],
        [`skills/${name}/references/guide.md`, `# Guide ${text}\n`],
      ];
      for (const [file, bytes] of files) {
        await mkdir(dirname(join(source, file)), { recursive: true }); await writeFile(join(source, file), bytes);
      }
      manifest.skills.push({ name, entry: files[0][0], files: files.map(([path, bytes]) => ({ path, sha256: digest(bytes) })) });
    }
    await writeFile(join(source, 'LICENSE'), 'MIT fixture license\n');
    if (change) await change(manifest);
    await writeFile(join(source, 'manifest.json'), rawTransform(JSON.stringify(manifest, null, 2) + '\n'));
    git(['add', '--all']); git(['commit', '--allow-empty', '-m', 'fixture release']);
    revision = git(['rev-parse', 'HEAD']); return revision;
  }
  const children = new Set();
  function startArgs(args, overrides = {}) {
    const env = { HOME: home, CODEX_HOME: codex, GESTALT_HOME: gestalt, PATH: `${bin}:${process.env.PATH}`, GESTALT_SKILLS_SOURCE: source, FIXTURE_SOURCE: source, ...overrides };
    for (const key of Object.keys(env)) if (env[key] === undefined) delete env[key];
    const child = spawn('bash', [wrapper, ...args], { env, cwd: temp });
    children.add(child);
    let stdout = '', stderr = '';
    child.stdout.on('data', data => stdout += data); child.stderr.on('data', data => stderr += data);
    const done = new Promise((resolve, reject) => {
      child.on('error', reject);
      child.on('close', (code, signal) => { children.delete(child); resolve({ code, signal, stdout, stderr }); });
    });
    return { child, done };
  }
  function start(operation = 'install', overrides = {}, args = []) { return startArgs(['skills', operation, ...args], overrides); }
  async function run(operation = 'install', overrides = {}, args = []) { return await start(operation, overrides, args).done; }
  async function runCommand(command, args = [], overrides = {}) { return await startArgs([command, ...args], overrides).done; }
  async function clean() {
    assert.deepEqual((await readdir(codex)).filter(name => name.startsWith('.gestalt-skills-')), []);
    assert.deepEqual(await snapshot(gestalt), profile, 'saved profiles are unchanged');
    assert.equal(await readFile(join(discovery, '.system/system/SKILL.md'), 'utf8'), '# system');
    assert.equal(await readFile(join(discovery, 'unrelated/SKILL.md'), 'utf8'), '# unrelated');
  }
  async function installed() { return { root: await snapshot(root), links: await snapshot(discovery), profiles: await snapshot(gestalt) }; }
  async function fault(mode, point = 'promotion') {
    const file = join(temp, `fault-${mode}-${point}.cjs`), marker = join(temp, `fault-${mode}-${point}.marker`);
    await writeFile(file, `const fs = require('node:fs');
if (process.argv[1] === '-') {
  const operation = ${JSON.stringify(point === 'links' ? 'unlinkSync' : 'renameSync')};
  const original = fs[operation]; let injected = false;
  fs[operation] = function(from, to) {
    const result = original.apply(this, arguments);
    if (!injected && (${point === 'links' ? `String(from) === ${JSON.stringify(join(discovery, 'a-retired'))}` : point === 'backup' ? `String(from) === ${JSON.stringify(root)}` : `String(from).endsWith('/candidate') && String(to) === ${JSON.stringify(root)}`})) {
      injected = true;
      fs.writeFileSync(${JSON.stringify(marker)}, String(process.pid));
      ${mode === 'throw' ? "throw new Error('fixture promotion failure');" : `process.kill(process.pid, '${mode}');`}
    }
    return result;
  };
}\n`);
    return { NODE_OPTIONS: `--require=${JSON.stringify(file)}`, marker };
  }
  t.after(async () => {
    for (const child of children) child.kill('SIGTERM');
    await Promise.all([...children].map(child => new Promise(resolve => child.on('close', resolve))));
    await rm(temp, { recursive: true, force: true });
  });
  await release();
  return { temp, home, source, root, discovery, codex, gestalt, git, release, run, runCommand, start, clean, installed, fault, get revision() { return revision; } };
}
function success(result) { assert.equal(result.code, 0, result.stderr); }
function failure(result, pattern) { assert.notEqual(result.code, 0, result.stdout); assert.match(result.stderr, pattern); }

test('fresh and repeated installs validate contents, preserve profiles and stable paths, and retain the receipt on unchanged reruns', async t => {
  const f = await fixture(t);
  success(await f.run());
  const receipt = JSON.parse(await readFile(join(f.root, 'receipt.json')));
  assert.equal(receipt.schemaVersion, 1); assert.equal(receipt.revision, f.revision);
  assert.equal(receipt.source, f.source); assert.equal(receipt.verifiedSource, false);
  assert.equal(receipt.managedRoot, f.root); assert.equal(receipt.discoveryRoot, f.discovery);
  assert.equal(receipt.manifestSha256, digest(await readFile(join(f.root, 'manifest.json'))));
  assert.equal(await readlink(join(f.discovery, 'nuklear-gui')), join(f.root, 'skills/nuklear-gui'));
  const previous = await f.installed();
  const result = await f.run(); success(result); assert.match(result.stdout, /unchanged/);
  assert.deepEqual(await f.installed(), previous);
  await f.clean();
});

test('only the canonical default source can record upstream provenance; even an identical explicit override is unverified', async t => {
  const f = await fixture(t);
  success(await f.run('install', { GESTALT_SKILLS_SOURCE: undefined }));
  let receipt = JSON.parse(await readFile(join(f.root, 'receipt.json')));
  assert.equal(receipt.source, canonicalSource); assert.equal(receipt.verifiedSource, true);
  success(await f.run('update', { GESTALT_SKILLS_SOURCE: canonicalSource }));
  receipt = JSON.parse(await readFile(join(f.root, 'receipt.json')));
  assert.equal(receipt.verifiedSource, false);
  await f.clean();
});

test('update adds, changes and retires owned skills while keeping canonical identities stable', async t => {
  const f = await fixture(t);
  await f.release({ 'nuklear-gui': 'before', retired: 'retire me' }); success(await f.run());
  const entry = join(f.root, 'skills/nuklear-gui/SKILL.md');
  const savedLink = await readlink(join(f.discovery, 'nuklear-gui'));
  const revision = await f.release({ 'nuklear-gui': 'after', added: 'new' });
  success(await f.run('update'));
  assert.equal(JSON.parse(await readFile(join(f.root, 'receipt.json'))).revision, revision);
  assert.match(await readFile(entry, 'utf8'), /after/);
  assert.equal(await readlink(join(f.discovery, 'nuklear-gui')), savedLink);
  await assert.rejects(lstat(join(f.discovery, 'retired')), { code: 'ENOENT' });
  await assert.rejects(lstat(join(f.root, 'skills/retired')), { code: 'ENOENT' });
  assert.equal(await readlink(join(f.discovery, 'added')), join(f.root, 'skills/added'));
  await f.clean();
});

test('a missing owned discovery link is repaired without replacing unrelated entries', async t => {
  const f = await fixture(t); success(await f.run());
  await rm(join(f.discovery, 'nuklear-gui'));
  success(await f.run('update'));
  assert.equal(await readlink(join(f.discovery, 'nuklear-gui')), join(f.root, 'skills/nuklear-gui'));
  await f.clean();
});

test('user directories and plugin links collide truthfully on fresh install and update', async t => {
  for (const kind of ['directory', 'plugin-link', 'unowned-link-to-managed-path']) await t.test(kind, async t => {
    const f = await fixture(t); success(await f.run());
    await f.release({ 'nuklear-gui': 'new', conflict: 'collision' });
    const destination = join(f.discovery, 'conflict');
    if (kind === 'directory') { await mkdir(destination); await writeFile(join(destination, 'SKILL.md'), 'user'); }
    else await symlink(kind === 'plugin-link' ? join(f.temp, 'plugin') : join(f.root, 'skills/conflict'), destination);
    const before = await f.installed();
    failure(await f.run('update'), /collision/);
    assert.deepEqual(await f.installed(), before); await f.clean();
    // The same collision also blocks an initial install, with no managed receipt.
    await rm(f.root, { recursive: true }); await rm(join(f.discovery, 'nuklear-gui'));
    failure(await f.run(), /collision/);
    await assert.rejects(lstat(f.root), { code: 'ENOENT' }); await f.clean();
  });
});

test('modified contents, extra files, damaged receipts and replaced retired links are preserved', async t => {
  for (const kind of ['entry', 'reference', 'extra', 'empty-directory', 'receipt', 'retired-link']) await t.test(kind, async t => {
    const f = await fixture(t); success(await f.run());
    if (kind === 'retired-link') {
      await rm(join(f.discovery, 'nuklear-gui')); await symlink(join(f.temp, 'user-plugin'), join(f.discovery, 'nuklear-gui'));
      await f.release({ added: 'new' });
    } else if (kind === 'empty-directory') await mkdir(join(f.root, 'skills/nuklear-gui/local-directory'));
    else if (kind === 'receipt') await writeFile(join(f.root, 'receipt.json'), '{}');
    else await writeFile(join(f.root, kind === 'entry' ? 'skills/nuklear-gui/SKILL.md' : kind === 'reference' ? 'skills/nuklear-gui/references/guide.md' : 'skills/nuklear-gui/local.txt'), 'local edit');
    const before = await f.installed();
    failure(await f.run('update'), /local|receipt|collision/);
    assert.deepEqual(await f.installed(), before); await f.clean();
  });
});

test('invalid releases cannot replace a usable collection or receipt', async t => {
  for (const kind of ['schema', 'digest', 'escaping', 'duplicate', 'unlisted', 'symlink', 'missing-entry']) await t.test(kind, async t => {
    const f = await fixture(t); success(await f.run()); const before = await f.installed();
    await f.release({ 'nuklear-gui': 'bad release' }, async manifest => {
      if (kind === 'schema') manifest.schemaVersion = 2;
      if (kind === 'digest') manifest.skills[0].files[0].sha256 = '0'.repeat(64);
      if (kind === 'escaping') manifest.skills[0].files[0].path = 'skills/nuklear-gui/../../outside';
      if (kind === 'duplicate') manifest.skills[0].files.push(manifest.skills[0].files[0]);
      if (kind === 'unlisted') await writeFile(join(f.source, 'skills/nuklear-gui/extra.txt'), 'unlisted');
      if (kind === 'symlink') { await rm(join(f.source, 'skills/nuklear-gui/SKILL.md')); await symlink('/etc/passwd', join(f.source, 'skills/nuklear-gui/SKILL.md')); }
      if (kind === 'missing-entry') manifest.skills[0].files.shift();
    });
    failure(await f.run('update'), /manifest|digest|symbolic|entry/);
    assert.deepEqual(await f.installed(), before); await f.clean();
  });
});

const invalidSchemas = [
  'empty-skills', 'extra-root-field', 'extra-skill-field', 'extra-file-field',
  'leading-zero-version', 'prerelease-version', 'build-version', 'long-name',
  'duplicate-root-key', 'duplicate-skill-key', 'duplicate-file-key',
  'unsorted-skills', 'unsorted-files', 'decimal-schema-version', 'exponent-schema-version',
];
function invalidShape(kind, manifest) {
  if (kind === 'empty-skills') manifest.skills = [];
  if (kind === 'extra-root-field') manifest.unexpected = true;
  if (kind === 'extra-skill-field') manifest.skills[0].unexpected = true;
  if (kind === 'extra-file-field') manifest.skills[0].files[0].unexpected = true;
  if (kind === 'leading-zero-version') manifest.version = '00.1.0';
  if (kind === 'prerelease-version') manifest.version = '0.1.0-rc.1';
  if (kind === 'build-version') manifest.version = '0.1.0+build.1';
  if (kind === 'long-name') {
    const skill = manifest.skills[0], old = skill.name;
    skill.name = 'a'.repeat(64);
    skill.entry = skill.entry.replace(`/${old}/`, `/${skill.name}/`);
    for (const file of skill.files) file.path = file.path.replace(`/${old}/`, `/${skill.name}/`);
  }
  if (kind === 'unsorted-skills') manifest.skills.reverse();
  if (kind === 'unsorted-files') manifest.skills[0].files.reverse();
}
function invalidRaw(kind, text) {
  if (kind === 'decimal-schema-version' || kind === 'exponent-schema-version') return text.replace(/("schemaVersion":\s*)1/, (_, prefix) => prefix + (kind === 'decimal-schema-version' ? '1.0' : '1e0'));
  if (kind === 'duplicate-root-key') return text.replace('"version":', '"version":"9.9.9", "version":');
  // Escaped property spellings must compare by decoded key, at every depth.
  if (kind === 'duplicate-skill-key') return text.replace('"name":', '"\\u006eame":"hidden", "name":');
  if (kind === 'duplicate-file-key') return text.replace('"path":', '"\\u0070ath":"hidden", "path":');
  return text;
}

test('downloaded manifests obey the exact version-1 package schema and reject duplicate raw JSON keys', async t => {
  for (const kind of invalidSchemas) await t.test(kind, async t => {
    const f = await fixture(t); success(await f.run()); const before = await f.installed();
    const skills = kind === 'empty-skills' ? {} : kind === 'long-name' ? { ['a'.repeat(64)]: 'invalid long name' }
      : kind === 'unsorted-skills' ? { alpha: 'a', zebra: 'z' } : { 'nuklear-gui': 'invalid schema' };
    await f.release(skills, manifest => invalidShape(kind, manifest), text => invalidRaw(kind, text));
    failure(await f.run('update'), /manifest|duplicate JSON/);
    assert.deepEqual(await f.installed(), before, 'old complete content, receipt, discovery links and profiles survive');
    await f.clean();
  });
});

test('installed manifests receive the same strict schema and duplicate-key checks before any download', async t => {
  for (const kind of invalidSchemas) await t.test(kind, async t => {
    const f = await fixture(t);
    if (kind === 'unsorted-skills') await f.release({ alpha: 'a', zebra: 'z' });
    success(await f.run());
    const manifestPath = join(f.root, 'manifest.json'), receiptPath = join(f.root, 'receipt.json');
    const manifest = JSON.parse(await readFile(manifestPath)), receipt = JSON.parse(await readFile(receiptPath));
    invalidShape(kind, manifest);
    const bytes = invalidRaw(kind, JSON.stringify(manifest, null, 2) + '\n');
    await writeFile(manifestPath, bytes);
    // Keep the receipt internally consistent so it cannot mask a schema error.
    receipt.manifestSha256 = digest(bytes); receipt.version = manifest.version; receipt.skills = manifest.skills;
    await writeFile(receiptPath, JSON.stringify(receipt));
    const before = await f.installed(), gate = join(f.temp, 'must-not-fetch');
    failure(await f.run('update', { FIXTURE_GATE: gate }), /manifest|duplicate JSON/);
    await assert.rejects(lstat(gate + '.waiting'), { code: 'ENOENT' });
    assert.deepEqual(await f.installed(), before, 'invalid installed bytes and all unrelated state are preserved');
    await f.clean();
  });
});

test('download failures and unknown refs preserve the last installed release and clear the lock', async t => {
  const f = await fixture(t); success(await f.run()); const before = await f.installed();
  failure(await f.run('update', { GESTALT_SKILLS_SOURCE: join(f.temp, 'unavailable') }), /retrieve collection/);
  failure(await f.run('update', { GESTALT_SKILLS_REF: 'missing-ref' }), /retrieve collection/);
  failure(await f.run('update', { GESTALT_SKILLS_SOURCE: 'ext::malicious-command' }), /source must be HTTPS/);
  assert.deepEqual(await f.installed(), before); await f.clean();
});

test('concurrent invocations serialize without stealing a live lock', async t => {
  const f = await fixture(t); success(await f.run());
  await f.release({ 'nuklear-gui': 'updated' });
  const gate = join(f.temp, 'fetch-gate'), first = f.start('update', { FIXTURE_GATE: gate });
  await waitFor(gate + '.waiting');
  const ownerPath = join(f.codex, '.gestalt-skills.lock/owner.json'), ownerBefore = await readFile(ownerPath, 'utf8');
  const busy = await f.run('status', {}, ['--json']); assert.equal(busy.code, 1); assert.equal(JSON.parse(busy.stdout).status, 'busy');
  assert.equal(await readFile(ownerPath, 'utf8'), ownerBefore, 'status never changes lock ownership or recovers it');
  failure(await f.run('update'), /another skills operation is running/);
  await writeFile(gate, 'release'); success(await first.done);
  assert.match(await readFile(join(f.root, 'skills/nuklear-gui/SKILL.md'), 'utf8'), /updated/);
  await f.clean();
});

test('promotion errors and termination restore the old files, links and receipt', async t => {
  for (const point of ['backup', 'promotion', 'links']) for (const mode of ['throw', 'SIGTERM']) await t.test(`${point} ${mode}`, async t => {
    const f = await fixture(t); await f.release({ 'nuklear-gui': 'old', 'a-retired': 'retired' }); success(await f.run()); const before = await f.installed();
    await f.release({ 'nuklear-gui': 'new', added: 'new' });
    const fault = await f.fault(mode, point);
    failure(await f.run('update', { NODE_OPTIONS: fault.NODE_OPTIONS }), /fixture promotion failure|interrupted/);
    await waitFor(fault.marker);
    assert.deepEqual(await f.installed(), before); await f.clean();
  });
});

test('interrupted fresh promotion and interrupted download clear their locks without publishing a receipt', async t => {
  const f = await fixture(t), fault = await f.fault('SIGTERM');
  failure(await f.run('install', { NODE_OPTIONS: fault.NODE_OPTIONS }), /interrupted/);
  await assert.rejects(lstat(f.root), { code: 'ENOENT' }); await f.clean();
  const gate = join(f.temp, 'blocked-download');
  const operation = f.start('install', { FIXTURE_GATE: gate });
  await waitFor(gate + '.waiting');
  const owner = JSON.parse(await readFile(join(f.codex, '.gestalt-skills.lock/owner.json')));
  process.kill(owner.pid, 'SIGTERM');
  failure(await operation.done, /interrupted/);
  await assert.rejects(lstat(f.root), { code: 'ENOENT' }); await f.clean();
});

test('edits made during download are detected and preserved before promotion', async t => {
  const f = await fixture(t); success(await f.run());
  const gate = join(f.temp, 'edit-gate'), operation = f.start('update', { FIXTURE_GATE: gate });
  await waitFor(gate + '.waiting');
  await writeFile(join(f.root, 'skills/nuklear-gui/SKILL.md'), 'edit during download');
  const before = await f.installed(); await writeFile(gate, 'release');
  failure(await operation.done, /local changes/);
  assert.deepEqual(await f.installed(), before); await f.clean();
});

test('an abruptly killed promotion is recovered on the next invocation before a failing download', async t => {
  const f = await fixture(t); success(await f.run()); const before = await f.installed();
  await f.release({ 'nuklear-gui': 'new', added: 'new' });
  const fault = await f.fault('SIGKILL');
  const killed = await f.run('update', { NODE_OPTIONS: fault.NODE_OPTIONS }); assert.notEqual(killed.code, 0);
  await waitFor(fault.marker);
  failure(await f.run('update', { GESTALT_SKILLS_REF: 'unavailable' }), /retrieve collection/);
  assert.deepEqual(await f.installed(), before); await f.clean();
});

test('installer never runs downloaded validation scripts, hooks or skill executables', async t => {
  const f = await fixture(t), marker = join(f.temp, 'should-not-exist');
  await f.release({ 'nuklear-gui': 'data only' }, async manifest => {
    const relative = 'skills/nuklear-gui/run.sh', bytes = `#!/bin/sh\ntouch '${marker}'\n`;
    await writeFile(join(f.source, relative), bytes, { mode: 0o755 });
    manifest.skills[0].files.push({ path: relative, sha256: digest(bytes) });
    await mkdir(join(f.source, 'scripts'), { recursive: true });
    await writeFile(join(f.source, 'scripts/validate.mjs'), `require('node:fs').writeFileSync(${JSON.stringify(marker)}, 'executed')`);
    await mkdir(join(f.source, '.git/hooks'), { recursive: true });
    await writeFile(join(f.source, '.git/hooks/post-checkout'), bytes, { mode: 0o755 });
  });
  success(await f.run()); await assert.rejects(lstat(marker), { code: 'ENOENT' }); await f.clean();
});

test('unsafe managed roots, root links, hard-linked content and invalid arguments fail without mutation', async t => {
  const f = await fixture(t); success(await f.run()); const before = await f.installed();
  failure(await f.run('install', { CODEX_HOME: `${f.codex}/../custom codex` }), /normalized/);
  failure(await f.run('install', {}, ['unexpected']), /additional arguments/);
  failure(await f.run('unknown'), /usage/);
  assert.deepEqual(await f.installed(), before); await f.clean();
  const entry = join(f.root, 'skills/nuklear-gui/SKILL.md'), alias = join(f.temp, 'hardlink');
  await link(entry, alias);
  failure(await f.run('update'), /ordinary package file/);
  assert.deepEqual(await f.installed(), before); await rm(alias); await f.clean();
  const moved = join(f.temp, 'collection'); await rename(f.root, moved); await symlink(moved, f.root);
  failure(await f.run('update'), /directory without links/);
  assert.equal(await readlink(f.root), moved);
  assert.deepEqual(await snapshot(moved), before.root);
  await rm(f.root); await rename(moved, f.root); await f.clean();
});

test('resolved CODEX_HOME rejects root and HOME aliases before any mutation outside the fixture', async t => {
  const f = await fixture(t); success(await f.run()); const before = await f.installed();
  const rootAlias = join(f.temp, 'root-alias'), homeAlias = join(f.temp, 'home-alias'), ancestorAlias = join(f.temp, 'ancestor-alias');
  await symlink('/', rootAlias); await symlink(f.home, homeAlias); await symlink(f.temp, ancestorAlias);
  const guard = join(f.temp, 'guard.cjs'), marker = join(f.temp, 'outside-attempt');
  await writeFile(guard, `const fs = require('node:fs'), path = require('node:path');
if (process.argv[1] === '-') {
  const savedWrite = fs.writeFileSync;
  function check(file, operation) {
    if (typeof file !== 'string') return;
    let ancestor = path.resolve(file), suffix = [];
    while (!fs.existsSync(ancestor)) { suffix.unshift(path.basename(ancestor)); ancestor = path.dirname(ancestor); }
    const resolved = path.join(fs.realpathSync(ancestor), ...suffix), allowed = ${JSON.stringify(f.temp)};
    if (resolved !== allowed && !resolved.startsWith(allowed + '/')) {
      savedWrite(${JSON.stringify(marker)}, operation + ' ' + resolved);
      throw new Error('fixture prevented mutation outside temp roots');
    }
  }
  for (const operation of ['mkdirSync', 'mkdtempSync', 'writeFileSync', 'renameSync', 'symlinkSync', 'rmSync', 'rmdirSync', 'unlinkSync']) {
    const original = fs[operation];
    fs[operation] = function(file, destination) {
      check(operation === 'symlinkSync' ? destination : file, operation);
      if (operation === 'renameSync') check(destination, operation);
      return original.apply(this, arguments);
    };
  }
}\n`);
  for (const overrides of [
    { CODEX_HOME: rootAlias },
    { CODEX_HOME: homeAlias },
    { CODEX_HOME: join(ancestorAlias, 'home') },
    { HOME: homeAlias, CODEX_HOME: join(ancestorAlias, 'home') },
  ]) {
    failure(await f.run('install', { NODE_OPTIONS: `--require=${JSON.stringify(guard)}`, ...overrides }), /refusing resolved CODEX_HOME/);
    await assert.rejects(lstat(marker), { code: 'ENOENT' }, 'rejection precedes any attempted mutation outside temporary roots');
    assert.deepEqual(await f.installed(), before); await f.clean();
  }
  const validAlias = join(f.temp, 'valid-codex-alias'); await symlink(f.codex, validAlias);
  success(await f.run('update', { CODEX_HOME: validAlias }));
  assert.deepEqual(await f.installed(), before, 'safe home aliases retain the canonical installation identity'); await f.clean();
});

test('collection help and status are read-only, including a missing custom home and unavailable download source', async t => {
  const f = await fixture(t);
  const missing = join(f.temp, 'missing codex'), gate = join(f.temp, 'must-not-fetch');
  const before = await snapshot(f.temp);
  for (const args of [['help'], ['--help'], ['skills', '--help'], ['skills', 'install', '--help'], ['skills', 'update', '--help'], ['skills', 'status', '--help']]) {
    success(await f.runCommand(args[0], args.slice(1), { CODEX_HOME: missing }));
  }
  const result = await f.run('status', { CODEX_HOME: missing, GESTALT_SKILLS_SOURCE: 'ext::unused', FIXTURE_GATE: gate }, ['--json']);
  assert.equal(result.code, 1); const status = JSON.parse(result.stdout);
  assert.equal(status.status, 'missing'); assert.equal(status.version, null); assert.equal(status.recoveryCommand, 'gestalt skills install');
  assert.deepEqual(await snapshot(f.temp), before);
  success(await f.run()); const installed = await f.installed();
  const ready = await f.run('status', { GESTALT_SKILLS_SOURCE: 'ext::unused', FIXTURE_GATE: gate }, ['--json']); success(ready);
  assert.equal(JSON.parse(ready.stdout).status, 'ready'); assert.equal(JSON.parse(ready.stdout).verifiedSource, false);
  assert.deepEqual(await f.installed(), installed); await assert.rejects(lstat(gate + '.waiting'), { code: 'ENOENT' });
  await writeFile(join(f.root, 'skills/nuklear-gui/SKILL.md'), 'local change');
  const modified = await f.installed(), damaged = await f.run('status', {}, ['--json']);
  assert.equal(damaged.code, 1); const report = JSON.parse(damaged.stdout);
  assert.equal(report.status, 'damaged'); assert.equal(report.integrity, 'invalid'); assert.equal(report.version, '0.1.0');
  assert.equal(report.revision, f.revision); assert.equal(report.verifiedSource, false); assert.match(report.reason, /local changes/);
  assert.deepEqual(await f.installed(), modified); await f.clean();
});

test('portable archives materialize collection links as ordinary skills, relocate profiles and carry no verified receipt', async t => {
  const f = await fixture(t); success(await f.run('install', { GESTALT_SKILLS_SOURCE: undefined }));
  const original = await f.installed(), archive = join(f.temp, 'portable.tar.gz');
  success(await f.runCommand('skills-export', [archive]));
  const members = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' });
  assert.match(members, /gestalt-skills\/codex-skills\/nuklear-gui\/SKILL.md/);
  assert.match(members, /gestalt-skills\/codex-skills\/nuklear-gui\/LICENSE/);
  assert.ok(!members.includes('receipt.json'));
  assert.deepEqual(await f.installed(), original, 'export leaves the live collection/profiles untouched');
  const target = await fixture(t); success(await target.runCommand('skills-import', [archive]));
  const imported = join(target.discovery, 'nuklear-gui'); assert.ok((await lstat(imported)).isDirectory());
  assert.equal(await readFile(join(imported, 'LICENSE'), 'utf8'), await readFile(join(f.root, 'LICENSE'), 'utf8'));
  assert.equal(await readFile(join(imported, 'SKILL.md'), 'utf8'), await readFile(join(f.root, 'skills/nuklear-gui/SKILL.md'), 'utf8'));
  assert.equal(await readFile(join(imported, 'references/guide.md'), 'utf8'), await readFile(join(f.root, 'skills/nuklear-gui/references/guide.md'), 'utf8'));
  const profile = await readFile(join(target.gestalt, 'skill-profiles/saved.yml'), 'utf8');
  assert.ok(profile.includes(`${target.discovery}/nuklear-gui/SKILL.md`)); assert.ok(profile.includes('enabled: false'));
  assert.ok(!profile.includes(f.root)); await assert.rejects(lstat(target.root), { code: 'ENOENT' });
  const status = await target.run('status', {}, ['--json']); assert.equal(JSON.parse(status.stdout).status, 'missing');
  assert.equal(JSON.parse(status.stdout).verifiedSource, false);
  // A copied receipt inside an ordinary skill is data, not an installed receipt.
  await writeFile(join(imported, 'receipt.json'), await readFile(join(f.root, 'receipt.json')));
  assert.equal(JSON.parse((await target.run('status', {}, ['--json'])).stdout).verifiedSource, false);
  failure(await target.run('install'), /collision/);
  await f.clean();
});

test('export refuses escaping or nested payload links and import preserves existing managed targets', async t => {
  const f = await fixture(t); success(await f.run());
  const archive = join(f.temp, 'clean.tar.gz'); success(await f.runCommand('skills-export', [archive]));
  const before = await f.installed(); failure(await f.runCommand('skills-import', [archive]), /unsafe skill destination/);
  assert.deepEqual(await f.installed(), before);
  await symlink('/etc/passwd', join(f.root, 'skills/nuklear-gui/reference-link'));
  failure(await f.runCommand('skills-export', [join(f.temp, 'nested.tar.gz')]), /unsupported link/);
  await rm(join(f.root, 'skills/nuklear-gui/reference-link'));
  await rm(join(f.discovery, 'nuklear-gui')); await symlink(f.source, join(f.discovery, 'nuklear-gui'));
  failure(await f.runCommand('skills-export', [join(f.temp, 'escape.tar.gz')]), /outside exported roots/);
  await f.clean();
});
