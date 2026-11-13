#!/usr/bin/env node

import { execFileSync } from 'node:child_process';
import { chmod, copyFile, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export async function buildNpmPackage(root) {
  const source = await readFile(join(root, 'public/gestalt'), 'utf8');
  const versions = [...source.matchAll(/^readonly GESTALT_CLI_VERSION='([^']*)'$/gm)];
  if (versions.length !== 1 || !/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(versions[0][1]))
    throw new Error('expected exactly one strict manager version');
  const manifest = JSON.parse(await readFile(join(root, 'npm/package.json'), 'utf8'));
  if (manifest.name !== '@jaromil/gestalt') throw new Error('unexpected npm package identity');
  manifest.version = versions[0][1];
  manifest.gitHead = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim();
  const destination = join(root, 'dist/npm');
  await rm(destination, { recursive: true, force: true });
  await mkdir(join(destination, 'bin'), { recursive: true });
  await mkdir(join(destination, 'lib'));
  await copyFile(join(root, 'npm/gestalt'), join(destination, 'bin/gestalt'));
  await copyFile(join(root, 'npm/postinstall.mjs'), join(destination, 'postinstall.mjs'));
  await copyFile(join(root, 'public/gestalt'), join(destination, 'lib/gestalt'));
  await copyFile(join(root, 'npm/README.md'), join(destination, 'README.md'));
  await copyFile(join(root, 'npm/LICENSE'), join(destination, 'LICENSE'));
  await chmod(join(destination, 'bin/gestalt'), 0o755);
  await chmod(join(destination, 'lib/gestalt'), 0o755);
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
