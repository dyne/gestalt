#!/usr/bin/env node

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, copyFile, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { managerVersion } from './manager-metadata.mjs';
import { buildCli } from './build-cli.mjs';

export async function buildNpmPackage(root, { nativeDirectory = join(root, 'dist/cli/native') } = {}) {
  const source = await readFile(join(root, 'public/gestalt'), 'utf8');
  const version = managerVersion(source);
  const manifest = JSON.parse(await readFile(join(root, 'npm/package.json'), 'utf8'));
  if (manifest.name !== '@jaromil/gestalt') throw new Error('unexpected npm package identity');
  manifest.version = version;
  manifest.gitHead = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim();
  // Validate target-built delivery bytes before replacing a prior package.
  // The consumer lifecycle never runs a compiler.
  const digest = bytes => createHash('sha256').update(bytes).digest('hex');
  const helperName = 'gestalt-owned-process.exe';
  let helper, helperBytes;
  try {
    helperBytes = await readFile(join(nativeDirectory, helperName));
  } catch (error) {
    if (error.code !== 'ENOENT' || manifest.os.includes('win32')) throw error;
  }
  if (helperBytes) {
    const bytes = helperBytes;
    const provenanceBytes = await readFile(join(nativeDirectory, `${helperName}.provenance.json`));
    const provenance = JSON.parse(provenanceBytes);
    if (provenance.platform !== 'win32' || provenance.architecture !== 'x64' || provenance.artifactSha256 !== digest(bytes))
      throw new Error('Windows helper target/artifact provenance mismatch');
    const inputs = ['src/cli/platform/native/owned-process.c', 'src/cli/platform/native/windows-storage-check.c', 'src/cli/platform/native/windows-storage-check.h'];
    if (!Array.isArray(provenance.sourceInputs) || provenance.sourceInputs.length !== inputs.length)
      throw new Error('Windows helper complete source provenance is required');
    for (const input of inputs) {
      const actual = await readFile(join(root, input));
      const record = provenance.sourceInputs.find(record => record.path === input);
      if (!record || record.bytes !== actual.length || record.sha256 !== digest(actual))
        throw new Error(`Windows helper source provenance mismatch: ${input}`);
    }
    helper = { bytes, provenanceBytes };
  }
  const destination = join(root, 'dist/npm');
  await rm(destination, { recursive: true, force: true });
  await mkdir(join(destination, 'bin'), { recursive: true });
  await mkdir(join(destination, 'lib'));
  await copyFile(join(root, 'npm/gestalt'), join(destination, 'bin/gestalt'));
  await copyFile(join(root, 'npm/postinstall.mjs'), join(destination, 'postinstall.mjs'));
  await buildCli(root, join(destination, 'lib/gestalt.mjs'));
  if (helper) {
    await mkdir(join(destination, 'lib/native'));
    await writeFile(join(destination, 'lib/native', helperName), helper.bytes);
    await writeFile(join(destination, 'lib/native', `${helperName}.provenance.json`), helper.provenanceBytes);
    const nativeSource = await readFile(join(root, 'src/cli/platform/native/owned-process.c'), 'utf8');
    const license = nativeSource.match(/^\/\* Original Gestalt implementation\. SPDX-License-Identifier: MIT[\s\S]*?\*\//)?.[0];
    if (!license) throw new Error('native helper MIT license notice is required');
    await writeFile(join(destination, 'lib/native/LICENSE.txt'), `${license}\n`);
  }
  await copyFile(join(root, 'npm/README.md'), join(destination, 'README.md'));
  await copyFile(join(root, 'npm/LICENSE'), join(destination, 'LICENSE'));
  await chmod(join(destination, 'bin/gestalt'), 0o755);
  await writeFile(join(destination, 'package.json'), `${JSON.stringify(manifest, null, 2)}\n`);
  return destination;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const root = dirname(dirname(fileURLToPath(import.meta.url)));
  buildNpmPackage(root).then(path => console.log(path)).catch(error => {
    console.error(`build-npm-package: ${error.message}`);
    process.exitCode = 1;
  });
}
