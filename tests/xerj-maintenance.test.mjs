import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile, spawn } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import { test } from 'node:test';

const exec = promisify(execFile);
const manager = resolve('public/gestalt');
async function fixture(t, binaryVersion = '1.0.0-rc.999') {
  const root = await mkdtemp(join(tmpdir(), 'xerj-maintenance-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'managed');
  const bin = join(root, 'bin');
  const scratch = join(root, 'temporary');
  const stage = 'xerj-1.0.0-rc.999-x86_64-unknown-linux-musl';
  for (const path of [bin, scratch, join(home, 'xerj'), join(root, stage)]) await mkdir(path, { recursive: true });
  const executable = join(root, stage, 'xerj');
  await writeFile(executable, `#!/usr/bin/env node\nif(process.argv.includes('-V')) console.log('xerj v${binaryVersion}'); else process.exit(1);\n`, { mode: 0o755 });
  await exec('tar', ['-czf', join(root, 'release.tar.gz'), '-C', root, stage]);
  const digest = createHash('sha256').update(await readFile(join(root, 'release.tar.gz'))).digest('hex');
  await writeFile(join(root, 'release.sha256'), `${digest}  release.tar.gz\n`);
  await writeFile(join(home, 'xerj', 'xerj'), 'prior executable', { mode: 0o755 });
  await writeFile(join(bin, 'uname'), '#!/bin/sh\ncase "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n', { mode: 0o755 });
  await writeFile(join(bin, 'curl'), `#!/usr/bin/env node
const fs=require('node:fs'), p=require('node:path');
const args=process.argv.slice(2), url=args.find(a=>a.startsWith('https://'));
const root=${JSON.stringify(root)};
fs.appendFileSync(p.join(root,'requests'),url+'\\n');
if(url==='https://api.github.com/repos/xerj-org/xerj/releases/latest') {
 console.log(JSON.stringify({tag_name:'v1.0.0-rc.999',draft:false}));
} else {
 if(!url.startsWith('https://github.com/xerj-org/xerj/releases/download/v1.0.0-rc.999/')) process.exit(22);
 fs.copyFileSync(p.join(root,url.endsWith('.sha256')?'release.sha256':'release.tar.gz'),args[args.indexOf('--output')+1]);
}
`, { mode: 0o755 });
  const env = { ...process.env, GESTALT_HOME: home, CODEX_HOME: join(root, 'codex'), TMPDIR: scratch, PATH: `${bin}:${process.env.PATH}` };
  for (const key of ['XERJ_INSTALL_DIR','XERJ_VERSION','XERJ_REPO','XERJ_INSECURE_SKIP_CHECKSUM']) delete env[key];
  const run = (...args) => exec('bash', [manager, 'xerj', ...args], { env, timeout: 15000 });
  return { root, home, scratch, env, run };
}

test('update resolves latest official release, verifies it and preserves runtime indexes', async t => {
  const f = await fixture(t);
  const data = join(f.env.CODEX_HOME, 'xerj-data'); await mkdir(data, { recursive: true });
  await writeFile(join(data, 'keep'), 'persistent index sentinel');
  await f.run('update');
  assert.match((await f.run('-V')).stdout, /1\.0\.0-rc\.999/);
  assert.equal(await readFile(join(data, 'keep'), 'utf8'), 'persistent index sentinel');
  assert.match(await readFile(join(f.root, 'requests'), 'utf8'), /api.github.com/);
  assert.deepEqual((await readdir(join(f.home, 'xerj'))).filter(name => name.startsWith('.install.')), []);
});

test('explicit update version avoids latest lookup and rejects malformed arguments', async t => {
  const f = await fixture(t);
  await f.run('update', 'v1.0.0-rc.999');
  assert.doesNotMatch(await readFile(join(f.root, 'requests'), 'utf8'), /api.github.com/);
  for (const args of [['update', '../escape'], ['update', '1.2.3', 'extra'], ['test', '--unknown']])
    await assert.rejects(f.run(...args));
});

test('checksum and candidate-version failures preserve the previous binary', async t => {
  const mismatch = await fixture(t, '1.0.0-rc.998');
  await assert.rejects(mismatch.run('update', '1.0.0-rc.999'), /version does not match/);
  assert.equal(await readFile(join(mismatch.home, 'xerj', 'xerj'), 'utf8'), 'prior executable');
  const checksum = await fixture(t);
  await writeFile(join(checksum.root, 'release.sha256'), `${'0'.repeat(64)}  archive\n`);
  await assert.rejects(checksum.run('update'), /checksum mismatch/);
  assert.equal(await readFile(join(checksum.home, 'xerj', 'xerj'), 'utf8'), 'prior executable');
});

test('test --json reports backend failure as harness error and removes disposable data', async t => {
  const f = await fixture(t);
  await f.run('update');
  try { await f.run('test', '--json'); assert.fail('test must reject a non-server binary'); }
  catch (error) {
    assert.equal(error.code, 2);
    const result = JSON.parse(error.stdout);
    assert.equal(result.version, '1.0.0-rc.999');
    assert.equal(result.errors, 1);
    assert.equal(result.results.at(-1).status, 'error');
    assert.doesNotMatch(error.stderr, /command failed near line/);
  }
  assert.deepEqual(await readdir(f.scratch), []);
  await assert.rejects(readFile(join(f.env.CODEX_HOME, 'xerj-data')), { code: 'ENOENT' });
});

test('interrupting test terminates its native child and cleans temporary state', async t => {
  const f = await fixture(t);
  const binary = join(f.home, 'xerj', 'xerj');
  await writeFile(binary, `#!/usr/bin/env node
if(process.argv.includes('-V')) {console.log('xerj v1.0.0-rc.999');process.exit(0);}
require('node:fs').writeFileSync(require('node:path').join(process.env.TMPDIR,'child.pid'),String(process.pid));
setInterval(()=>{},1000);
`); await chmod(binary, 0o755);
  const child = spawn('bash', [manager, 'xerj', 'test', '--json'], { env: f.env, stdio: ['ignore','pipe','pipe'] });
  t.after(() => { if(child.exitCode===null && child.signalCode===null) child.kill('SIGKILL'); });
  let output=''; child.stdout.on('data', chunk=>output+=chunk);
  const closed = new Promise(resolve=>child.once('close',resolve));
  let pid;
  for(let i=0;i<100 && !pid;i++) {
    const dirs=await readdir(f.scratch);
    try { if(dirs[0]) pid=Number(await readFile(join(f.scratch,dirs[0],'child.pid'),'utf8')); } catch {}
    if(!pid) await new Promise(resolve=>setTimeout(resolve,50));
  }
  assert.ok(pid,'native child started');
  child.kill('SIGTERM');
  assert.equal(await closed,130);
  assert.equal(JSON.parse(output).errors,1);
  assert.throws(()=>process.kill(pid,0),{code:'ESRCH'});
  assert.deepEqual(await readdir(f.scratch),[]);
});
