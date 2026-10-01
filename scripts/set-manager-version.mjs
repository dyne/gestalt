#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { chmod, readFile, rename, stat, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const strictSemver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const versionDeclaration = /^readonly GESTALT_CLI_VERSION='[^']*'$/gm;

export async function setManagerVersion(root, version) {
  if (!strictSemver.test(version)) {
    throw new Error(`invalid strict semantic version: ${version}`);
  }

  const managerPath = join(root, 'public', 'gestalt');
  const checksumPath = join(root, 'public', 'gestalt.sha256');
  const manager = await readFile(managerPath, 'utf8');
  const declarations = manager.match(versionDeclaration) ?? [];
  if (declarations.length !== 1) {
    throw new Error(`expected exactly one manager version declaration; found ${declarations.length}`);
  }

  const updated = manager.replace(
    versionDeclaration,
    `readonly GESTALT_CLI_VERSION='${version}'`,
  );
  const digest = createHash('sha256').update(updated).digest('hex');
  const mode = (await stat(managerPath)).mode & 0o777;
  const managerTemporary = `${managerPath}.tmp-${process.pid}`;
  const checksumTemporary = `${checksumPath}.tmp-${process.pid}`;

  await writeFile(managerTemporary, updated, { mode });
  await chmod(managerTemporary, mode);
  await writeFile(checksumTemporary, `${digest}  public/gestalt\n`);
  await rename(managerTemporary, managerPath);
  await rename(checksumTemporary, checksumPath);
}

async function main() {
  const version = process.argv[2];
  if (!version || process.argv.length !== 3) {
    throw new Error('usage: set-manager-version.mjs <major.minor.patch>');
  }

  const scriptDirectory = dirname(fileURLToPath(import.meta.url));
  await setManagerVersion(dirname(scriptDirectory), version);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(`set-manager-version: ${error.message}`);
    process.exitCode = 1;
  });
}
