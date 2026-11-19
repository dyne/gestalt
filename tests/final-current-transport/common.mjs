import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstat, readFile, realpath, rename, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { createRequire } from 'node:module';
import { verifyOwnershipBuildInputs } from '../fixtures/native-runtime/source-binding.mjs';

export const root = path.resolve(import.meta.dirname, '../..');
export const inputs = JSON.parse(await readFile(path.join(import.meta.dirname, 'inputs.json'), 'utf8'));
export const now = () => new Date().toISOString();
export const json = async file => JSON.parse((await readFile(file, 'utf8')).replace(/^\uFEFF/, ''));
export const hash = bytes => createHash('sha256').update(bytes).digest('hex');
export async function digest(file) {
  const bytes = await readFile(file);
  return { path: file, bytes: bytes.length, sha256: hash(bytes) };
}
export async function save(file, data) {
  const next = file + '.next';
  await writeFile(next, JSON.stringify(data, null, 2) + '\n', { mode: 0o600 });
  await rename(next, file);
}
export const errorInfo = error => ({ name: error.name, message: error.message, stack: error.stack });
export const exactPattern = names => '^(' + names.map(name => name.replace(/[.*+?^$${}()|[\]\\]/g, '\\$&')).join('|') + ')$';

export function phases(evidence, state, environment) {
  async function record() { await save(path.join(evidence, 'result.json'), state); }
  async function group(id, prerequisites, action) {
    assert.ok(!state.groups.some(item => item.id === id));
    const row = { id, state: 'unexecuted', blockedBy: prerequisites.filter(id => state.groups.find(item => item.id === id)?.state !== 'pass') };
    state.groups.push(row);
    if (row.blockedBy.length) { await record(); return; }
    row.state = 'started'; row.startedAt = now();
    await record();
    try { row.detail = await action(); row.state = 'pass'; }
    catch (error) { row.state = 'fail'; row.failure = errorInfo(error); }
    row.observedAt = now(); await record();
    return row;
  }
  async function command(name, executable, argv, options = {}) {
    assert.ok(!state.commands.some(item => item.name === name));
    const cwd = options.cwd ?? root, startedAt = now(), timeout = options.timeout ?? 180_000;
    const result = spawnSync(executable, argv, { cwd, timeout, env: options.env ?? environment,
      encoding: 'buffer', maxBuffer: 64 * 1024 * 1024, shell: false, windowsHide: true });
    const out = path.join(evidence, name + '.stdout'), err = path.join(evidence, name + '.stderr');
    await writeFile(out, result.stdout ?? Buffer.alloc(0), { flag: 'wx', mode: 0o600 });
    await writeFile(err, result.stderr ?? Buffer.alloc(0), { flag: 'wx', mode: 0o600 });
    const row = { name, argv: [executable, ...argv], cwd, timeout, startedAt, observedAt: now(),
      exit: result.status, signal: result.signal, error: result.error?.message ?? null, stdout: await digest(out), stderr: await digest(err) };
    state.commands.push(row); await save(path.join(evidence, 'commands.json'), state.commands);
    if (options.required !== false) assert.equal(row.exit, 0, name + ' failed; inspect retained command streams');
    return { row, text: (result.stdout ?? Buffer.alloc(0)).toString('utf8') };
  }
  return { command, group, record };
}

export function testCounts(text) {
  const blocks = [];
  for (const [, key, number] of text.matchAll(/^(?:# |ℹ )(tests|pass|fail|cancelled|skipped|todo) (\d+)\s*$/gm)) {
    if (key === 'tests') blocks.push({});
    assert.ok(blocks.length && !Object.hasOwn(blocks.at(-1), key));
    blocks.at(-1)[key] = +number;
  }
  for (const block of blocks) {
    assert.equal(Object.keys(block).length, 6);
    assert.equal(block.tests, block.pass + block.fail + block.cancelled + block.skipped + block.todo);
  }
  return blocks;
}
export function selectedTap(text, names) {
  const counts = testCounts(text);
  assert.deepEqual(counts, [{ tests: names.length, pass: names.length, fail: 0, cancelled: 0, skipped: 0, todo: 0 }]);
  const actual = [...text.matchAll(/^ok \d+ - ([^\r\n]+)\r?$/gm)].map(row => row[1]);
  assert.deepEqual(actual.sort(), [...names].sort());
  return counts[0];
}

export async function verifyInventory(directory, entries, { modes = process.platform !== 'win32' } = {}) {
  for (const entry of entries) {
    assert.match(entry.mode, /^100(?:644|755)$/);
    assert.ok(!path.isAbsolute(entry.name) && !entry.name.split('/').includes('..') && !entry.name.includes('\\'));
    const file = path.join(directory, entry.name), info = await lstat(file);
    assert.ok(info.isFile() && !info.isSymbolicLink(), entry.name);
    assert.equal(await realpath(file), file, entry.name);
    if (modes) assert.equal(Boolean(info.mode & 0o111), entry.mode === '100755', entry.name);
    const bytes = await readFile(file);
    assert.equal(bytes.length, entry.bytes, entry.name);
    assert.equal(hash(bytes), entry.sha256, entry.name);
    assert.equal(createHash('sha1').update('blob ' + bytes.length + '\0').update(bytes).digest('hex'), entry.blob, entry.name);
  }
}
export async function gitInventory(directory, label, command, evidence) {
  const { text } = await command(label + '-git-tree', 'git', ['--no-optional-locks', 'ls-tree', '-rz', 'HEAD'], { cwd: directory });
  const entries = [];
  for (const line of text.split('\0').filter(Boolean)) {
    const match = /^(100644|100755) blob ([0-9a-f]{40})\t([^\n]+)$/.exec(line);
    assert.ok(match);
    const [, mode, blob, name] = match;
    const info = await digest(path.join(directory, name));
    entries.push({ mode, blob, name, bytes: info.bytes, sha256: info.sha256 });
  }
  await verifyInventory(directory, entries);
  await save(path.join(evidence, label + '-inventory.json'), entries);
  return entries;
}

export async function verifyHelper(directory) {
  const file = path.join(directory, 'gestalt-owned-process.exe');
  const provenanceFile = file + '.provenance.json';
  const provenance = await json(provenanceFile);
  const helper = await digest(file);
  assert.equal(provenance.artifactSha256, helper.sha256);
  assert.equal(provenance.platform, 'win32');
  assert.equal(provenance.architecture, 'x64');
  assert.equal(provenance.node, inputs.node);
  assert.equal(provenance.compiler.toLowerCase(), 'cl.exe');
  assert.match(provenance.compilerVersion, /Microsoft/i);
  const source = await verifyOwnershipBuildInputs(provenance, root);
  assert.equal(source.length, 3);
  return { helper, provenance: await digest(provenanceFile), source };
}
export async function verifySameRun(directory, head, tree) {
  const binding = await json(path.join(directory, 'helper-receipt.json'));
  assert.equal(binding.head, head); assert.equal(binding.tree, tree); assert.equal(binding.run, process.env.GITHUB_RUN_ID);
  assert.equal(binding.attempt, process.env.GITHUB_RUN_ATTEMPT);
  const current = await verifyHelper(directory);
  for (const key of ['helper', 'provenance']) {
    assert.equal(binding[key].sha256, current[key].sha256);
    assert.equal(binding[key].bytes, current[key].bytes);
  }
  assert.deepEqual(binding.source, current.source);
  return current;
}
export async function checkPack(tarball, npmCli, destination, head, helper) {
  const tar = createRequire(npmCli)('tar');
  const expected = ['LICENSE','README.md','bin/gestalt','lib/gestalt.mjs','lib/legacy/gestalt','lib/native/LICENSE.txt',
    'lib/native/gestalt-owned-process.exe','lib/native/gestalt-owned-process.exe.provenance.json','package.json','postinstall.mjs'];
  const entries = new Map();
  let parseError;
  await tar.t({ file: tarball, strict: true, onReadEntry(entry) {
    try {
      assert.match(entry.path, /^package\//);
      const name = entry.path.slice(8);
      assert.ok(expected.includes(name) && !entries.has(name) && entry.type === 'File' && entry.size > 0 && entry.size <= 4 * 1024 * 1024, name);
      const executable = ['bin/gestalt', 'lib/gestalt.mjs', 'lib/legacy/gestalt'].includes(name);
      const mode = entry.mode & 0o7777;
      assert.equal(mode, process.platform === 'win32' ? 0o644 : executable ? 0o755 : 0o644, name);
      const chunks = [];
      entries.set(name, { mode, size: entry.size, chunks });
      entry.on('data', bytes => chunks.push(bytes)); entry.on('error', error => { parseError ??= error; });
    } catch (error) { parseError ??= error; }
  } });
  if (parseError) throw parseError;
  assert.deepEqual([...entries.keys()].sort(), expected);
  const members = {};
  for (const [name, entry] of entries) {
    const bytes = Buffer.concat(entry.chunks);
    assert.equal(bytes.length, entry.size); assert.deepEqual(bytes, await readFile(path.join(destination, name)));
    members[name] = { size: entry.size, mode: entry.mode.toString(8), sha256: hash(bytes) };
  }
  const manifest = await json(path.join(destination, 'package.json'));
  assert.equal(manifest.gitHead, head);
  assert.equal(members['lib/gestalt.mjs'].sha256, inputs.normalCliSha256);
  assert.equal(members['lib/legacy/gestalt'].sha256, inputs.sourceDigests['public/gestalt']);
  assert.equal(members['README.md'].sha256, inputs.sourceDigests['npm/README.md']);
  assert.equal(members['lib/native/gestalt-owned-process.exe'].sha256, helper.helper.sha256);
  assert.equal(members['lib/native/gestalt-owned-process.exe.provenance.json'].sha256, helper.provenance.sha256);
  return { tarball: await digest(tarball), members, manifest };
}
