import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {once} from 'node:events';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import test from 'node:test';

const source = await readFile(resolve('public/gestalt'), 'utf8');
const runtime = source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const {scratchSpawnProgram} = new Function('require', runtime + '\nreturn {scratchSpawnProgram};')(createRequire(import.meta.url));

test('source pipes hide embedded programs and arguments while preserving lifetime stdin and IPC', {timeout:5000}, async t => {
  const args = ['a path with spaces', 'literal `quotes` $(text)\nsecond line'];
  const child = scratchSpawnProgram(`
process.send({args:process.argv.slice(1)});
let input='';process.stdin.on('data',b=>input+=b);
process.stdin.on('end',()=>{process.stdout.write(input);process.disconnect();});
`, args, {stdio:['pipe','pipe','pipe','ipc'], label:'gestalt-source-pipe-test'});
  const exited = once(child, 'close');
  let output = '';
  child.stdout.on('data', bytes => output += bytes);
  child.stderr.resume();
  t.after(async () => {
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await exited;
  });
  assert.deepEqual((await once(child,'message'))[0], {args});
  if (process.platform === 'linux') {
    const argv = (await readFile(`/proc/${child.pid}/cmdline`, 'utf8')).split('\0').filter(Boolean);
    assert.deepEqual(argv.slice(1), ['--input-type=commonjs', '-', 'gestalt-source-pipe-test']);
  }
  child.stdin.end('original input bytes\n');
  assert.deepEqual(await exited, [0, null]);
  assert.equal(output, 'original input bytes\n');
});
