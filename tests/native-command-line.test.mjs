import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { countProbeSource, vectorBytes, quotingCases, checkCountResult } from './fixtures/native-command-line.mjs';
const root = path.resolve(import.meta.dirname, '..'), native = process.platform !== 'win32';
const location = process.env.GESTALT_QUOTING_EVIDENCE ? path.resolve(process.env.GESTALT_QUOTING_EVIDENCE) : await mkdtemp(path.join(tmpdir(), 'quote-'));
await mkdir(location, { recursive: true });
const capture = JSON.parse(await readFile(new URL('./fixtures/native-owned-query-vector.json', import.meta.url)));
let executable;
test.before(() => { assert.equal(capture.allAscii, true); });
test.before(async () => {
  if (!native) return;
  const source = await readFile(path.join(root, 'src/cli/platform/native/owned-process.c'), 'utf8'), generated = await countProbeSource(source);
  const file = path.join(location, 'count.c'); executable = path.join(location, 'count'); await writeFile(file, generated);
  const args = ['-std=c11', '-Wall', '-Wextra', '-Werror', '-O2', file, '-o', executable], result = spawnSync(process.env.CC || 'cc', args, { encoding: 'utf8', shell: false });
  await writeFile(path.join(location, 'compile.log'), `${result.stdout ?? ''}${result.stderr ?? ''}`);
  await writeFile(path.join(location, 'compile.json'), JSON.stringify({ observedAt: new Date().toISOString(), command: [process.env.CC || 'cc', ...args], cwd: root, exit: result.status,
    sourceSha256: createHash('sha256').update(source).digest('hex'), generatedSha256: createHash('sha256').update(generated).digest('hex'), scope: 'Exact production quoting block; UTF16 units explicit, not a native Windows launch' }, null, 2) + '\n');
  assert.equal(result.status, 0, result.stderr);
});
for (const item of quotingCases(capture)) test(item.name, { skip: native ? false : 'Actual Windows count/Job probe runs in targeted preflight' }, async () => {
  const result = spawnSync(executable, [], { input: vectorBytes(item.argv), timeout: 5000, maxBuffer: 1024 * 1024, shell: false });
  assert.equal(result.status, 0, result.stderr?.toString());
  const record = checkCountResult(item, result.stdout); await writeFile(path.join(location, item.name.replace(/[^a-z0-9]/gi, '-') + '.json'), JSON.stringify({ observedAt: new Date().toISOString(), command: [executable], exit: result.status, ...record }) + '\n');
});
test.after(async () => { if (!process.env.GESTALT_QUOTING_EVIDENCE) await rm(location, { recursive: true, force: true }); });
