import assert from 'node:assert/strict';
import { chmod, copyFile, lstat, mkdir, readFile, realpath } from 'node:fs/promises';
import path from 'node:path';
import { digest, inputs, json, root, save, verifyHelper } from './common.mjs';

const prebuilt = await realpath(process.env.FINAL_WIN_PREBUILT);
const manifest = await json(path.join(prebuilt, 'artifacts.json'));
assert.equal(manifest.platform, 'win32');
assert.equal(manifest.arch, 'x64');
assert.equal(manifest.node, inputs.node);
assert.equal(manifest.abi, process.versions.modules);
const helperFile = 'gestalt-owned-process.exe';
assert.equal(manifest.helper, path.join('native', helperFile));
const helper = await verifyHelper(path.join(prebuilt, 'native'));
assert.deepEqual(helper.source, manifest.nativeHelperInputs);

async function fixtureBase(repository, candidate, name) {
  assert.equal(await realpath(repository), await realpath(root));
  assert.ok(path.isAbsolute(candidate));
  assert.equal(path.basename(candidate), name);
  const base = await realpath(path.dirname(candidate));
  const temporary = await realpath(process.env.TEMP);
  assert.equal(path.dirname(base), temporary);
  assert.ok(path.basename(base).startsWith('gestalt core installed ü & '));
  return base;
}
async function copyExpected(relative, destination, mode, expected) {
  const source = path.join(prebuilt, relative);
  const found = manifest.artifacts.find(item => item.relative === relative);
  const actual = await digest(source);
  assert.equal(actual.sha256, found?.sha256); assert.equal(actual.bytes, found.bytes);
  if (expected) assert.equal(actual.sha256, expected);
  await copyFile(source, destination, 1);
  await chmod(destination, mode);
  assert.ok((await lstat(destination)).isFile());
  assert.deepEqual(await readFile(destination), await readFile(source));
  return digest(destination);
}

export async function buildCli(...args) {
  assert.equal(args.length, 2);
  const [repository, output] = args;
  const base = await fixtureBase(repository, output, 'gestalt.mjs');
  const legacy = path.join(base, 'legacy');
  await mkdir(legacy);
  const entry = await copyExpected('gestalt.mjs', output, 0o755, inputs.normalCliSha256);
  const shell = await copyExpected(path.join('legacy', 'gestalt'), path.join(legacy, 'gestalt'), 0o755, inputs.sourceDigests['public/gestalt']);
  await save(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'fixture-prebuilt-cli.json'), { entry, shell });
  return output;
}
export async function buildOwnedProcess(...args) {
  assert.equal(args.length, 2);
  const [repository, outputDirectory] = args;
  await fixtureBase(repository, outputDirectory, 'native');
  await mkdir(outputDirectory);
  const relative = path.join('native', helperFile), output = path.join(outputDirectory, helperFile);
  const binary = await copyExpected(relative, output, 0o755, helper.helper.sha256);
  const provenance = await copyExpected(relative + '.provenance.json', output + '.provenance.json', 0o644, helper.provenance.sha256);
  const source = await verifyHelper(outputDirectory).then(x => x.source);
  await save(path.join(process.env.GESTALT_NATIVE_SETUP_EVIDENCE, 'fixture-prebuilt-helper.json'), { binary, provenance, source });
  return output;
}
