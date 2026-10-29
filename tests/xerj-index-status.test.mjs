import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { test } from 'node:test';
import { promisify } from 'node:util';

const exec = promisify(execFile);
const manager = resolve('public/gestalt');
test('index status selects the indexing root and endpoint without writing managed state', async t => {
  const temporary = await mkdtemp(join(tmpdir(), 'xerj-index-status-'));
  t.after(async () => {
    await chmod(join(temporary, 'codex', 'xerj-data'), 0o700);
    await rm(temporary, { recursive: true, force: true });
  });
  const home = join(temporary, 'managed');
  const codex = join(temporary, 'codex');
  const root = join(temporary, 'source root');
  const data = join(codex, 'xerj-data');
  const prefix = 'ax-' + createHash('sha256').update(root).digest('hex').slice(0, 16);
  const journal = join(data, 'autoindex', prefix);
  for (const path of [join(home, 'xerj'), join(data, '.lifecycle'), journal, root])
    await mkdir(path, { recursive: true });
  await writeFile(join(home, 'xerj', 'xerj'), `#!/usr/bin/env node
console.log(JSON.stringify({args:process.argv.slice(2),url:process.env.XERJ_URL}));
`, { mode: 0o755 });
  await writeFile(join(data, '.lifecycle', 'endpoint.json'), JSON.stringify({schemaVersion:1,endpoint:'http://127.0.0.1:19300'}));
  await writeFile(join(journal, 'journal.ndjson'), 'retained journal\n');
  const env = { ...process.env, GESTALT_HOME:home, CODEX_HOME:codex };
  for (const key of Object.keys(env)) if (key.startsWith('XERJ_')) delete env[key];
  const run = async (...args) => JSON.parse((await exec('bash', [manager, 'xerj', 'autoindex', 'status', ...args], {env,cwd:root,timeout:15000})).stdout);
  await chmod(data, 0o500);
  const result = await run();
  assert.deepEqual(result.args, ['autoindex','--state-dir',journal,'status','--prefix',prefix,'--url','http://127.0.0.1:19300']);
  assert.equal(result.url, 'http://127.0.0.1:19300');
  assert.deepEqual(await readdir(data), ['.lifecycle','autoindex']);
  assert.equal(await readFile(join(journal, 'journal.ndjson'), 'utf8'), 'retained journal\n');
  assert.deepEqual((await run('--cwd', root)).args, result.args);
  assert.deepEqual((await run('--cwd='+root)).args, result.args);
  const explicit = await run('--prefix','custom','--url','http://127.0.0.1:19400');
  assert.deepEqual(explicit.args, ['autoindex','--state-dir',join(data,'autoindex','custom'),'status','--prefix','custom','--url','http://127.0.0.1:19400']);
  await assert.rejects(run('--cwd'), /requires a directory/);
  await assert.rejects(run('--prefix','../escape'), /prefix/);
  await assert.rejects(run('--watch'), /status/);
});
