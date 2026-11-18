import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { buildOwnedProcess } from '../../scripts/build-owned-process.mjs';
import { stageOwnedProcessSource } from '../fixtures/stage-owned-process-source.mjs';

assert.equal(process.platform, 'win32');
const repository = path.resolve(import.meta.dirname, '../..');
const [fixture] = process.argv.slice(2);
assert.ok(path.isAbsolute(fixture));
const source = await readFile(path.join(repository, 'src/cli/platform/native/owned-process.c'), 'utf8');
await stageOwnedProcessSource(repository, fixture, source + '\n/* Build-only Windows companion staging check. */\n');
console.log(await buildOwnedProcess(fixture, path.join(fixture, 'bin')));
