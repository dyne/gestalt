import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, mkdir, readFile, readdir, rm, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import test from 'node:test';
import {sessionScratch} from './session-scratch-fixture.mjs';

// Explicit real-browser gate. The caller supplies an existing Chrome binary;
// no browser installation, sandbox relaxation, or production profile is used.
test('installed Chrome fails with a long canonical TMPDIR and renders with scratch run', {timeout:45000}, async t => {
  assert.equal(process.platform, 'linux');
  assert.ok(process.env.GESTALT_TEST_CHROME, 'Set GESTALT_TEST_CHROME to an installed Chrome executable');
  const root = await mkdtemp(join(tmpdir(), 'chrome socket regression '));
  const workspace = join(root, 'workspace ' + 'x'.repeat(80));
  await mkdir(workspace);
  const scratch = await sessionScratch(workspace);
  t.after(() => rm(root, {recursive:true, force:true}));
  const script = join(root, 'browser.mjs');
  const module = new URL('../node_modules/@playwright/test/index.mjs', import.meta.url).href;
  await writeFile(script, `
import {chromium} from ${JSON.stringify(module)};
const browser=await chromium.launch({executablePath:process.env.GESTALT_TEST_CHROME,headless:true,chromiumSandbox:true,timeout:12000});
try {
 const page=await browser.newPage();await page.setContent('<title>Short socket works</title><h1>Socket regression</h1>');
 if(await page.title()!=='Short socket works')throw Error('page title mismatch');
 console.log('chrome-rendered');
} finally {await browser.close();}
`);
  async function invoke(wrapped) {
    const command = wrapped ? 'bash' : process.execPath;
    const args = wrapped ? [resolve('public/gestalt'), 'scratch', 'run', '--', process.execPath, script] : [script];
    const child = spawn(command, args, {cwd:workspace,
      env:{...process.env, ...scratch.environment, HOME:root, XDG_CONFIG_HOME:join(root,'config'), XDG_CACHE_HOME:join(root,'cache')},
      stdio:['ignore','pipe','pipe']});
    let stdout='', stderr='';
    child.stdout.on('data', b=>stdout+=b);child.stderr.on('data', b=>stderr+=b);
    const [code]=await once(child,'close');
    return {code,stdout,stderr};
  }
  const before = await invoke(false);
  assert.notEqual(before.code, 0);
  assert.match(before.stderr, /Socket path too long/);
  const after = await invoke(true);
  assert.equal(after.code, 0, after.stderr);
  assert.equal(after.stdout, 'chrome-rendered\n');
  const directory = join(scratch.context.directory,'leases');
  const records = await Promise.all((await readdir(directory)).map(async name=>JSON.parse(await readFile(join(directory,name),'utf8'))));
  assert.equal(records.length,1);
  assert.equal(records[0].state,'released');
});
