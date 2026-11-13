#!/usr/bin/env node

import { appendFile, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

export function shouldStage(expected, published) {
  if (!published) return true;
  if (published.name !== expected.name || published.version !== expected.version ||
      published.gitHead !== expected.gitHead)
    throw new Error('existing npm version does not match the release commit; refusing to replace it');
  return false;
}

async function main() {
  const expected = JSON.parse(await readFile(join(process.argv[2] ?? 'dist/npm', 'package.json'), 'utf8'));
  if (expected.name !== '@jaromil/gestalt' || !/^[a-f0-9]{40}$/.test(expected.gitHead) ||
      !/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(expected.version))
    throw new Error('invalid npm release identity');
  const response = await fetch(
    `https://registry.npmjs.org/${encodeURIComponent(expected.name)}/${expected.version}`,
    { signal: AbortSignal.timeout(30_000) },
  );
  if (!response.ok && response.status !== 404)
    throw new Error(`npm registry query failed with status ${response.status}`);
  const published = response.status === 404 ? null : await response.json();
  const stage = shouldStage(expected, published);
  if (!process.env.GITHUB_OUTPUT) throw new Error('GITHUB_OUTPUT is required');
  await appendFile(process.env.GITHUB_OUTPUT, `stage=${stage}\n`);
  console.log(stage ? `Stage ${expected.name}@${expected.version} for approval` : 'Exact release already published');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    console.error(`inspect-npm-publication: ${error.message}`);
    process.exitCode = 1;
  });
}
