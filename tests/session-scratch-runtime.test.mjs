import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdtemp,mkdir,readFile,writeFile,readdir,rm,rename,symlink,realpath} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {promisify} from 'node:util';
import test from 'node:test';
import {sessionScratch} from './session-scratch-fixture.mjs';
const source=await readFile(resolve('public/gestalt'),'utf8');
const program=source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const api=new Function('require',program+'\nreturn {scratchAllocate,scratchRelease,scratchBorrow,scratchEnvironment,scratchSocketPath};')(createRequire(import.meta.url));
const execute=promisify(execFile);
async function fixture(t) {
  const root=await realpath(await mkdtemp(join(tmpdir(),'runtime-contract-')));
  const workspace=join(root,...Array.from({length:8},()=> '深い-é-workspace'));
  await mkdir(workspace,{recursive:true});
  const owners=[];
  t.after(async()=>{
    for(const owner of owners.reverse())await rm(owner.context.runtime.directory,{recursive:true,force:true});
    await rm(root,{recursive:true,force:true});
  });
  const allocate=(kind='cli-session')=>{const own=api.scratchAllocate(workspace,kind,'test-controller');owners.push(own);return own;};
  return{root,workspace,allocate};
}
test('CLI and service owners automatically export canonical bounded physical roots to Node, Python and shell',async t=>{
  const f=await fixture(t);
  for(const kind of ['cli-session','workspace-service']) {
    const own=f.allocate(kind),env=api.scratchEnvironment({...process.env,GESTALT_SESSION_TMP_CONTEXT:JSON.stringify(own.context),TMPDIR:'/hostile'});
    assert.equal(own.context.schemaVersion,2);assert.ok(Buffer.byteLength(env.TMPDIR)<=48);
    assert.equal(await realpath(env.TMPDIR),env.TMPDIR);
    const python=`import os,socket,threading
root=os.environ['TMPDIR'];assert os.path.realpath(root)==root==os.environ['TMP']==os.environ['TEMP']
path=root+'/bitcoin_sock0_qR2DkZ'
with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as server:
 server.settimeout(5);server.bind(path);server.listen(1)
 with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as client:
  client.connect(path);client.sendall(b'ping');peer,_=server.accept()
  with peer:
   assert peer.recv(4)==b'ping';peer.sendall(b'pong');assert client.recv(4)==b'pong'
os.unlink(path);print('exchange-ok')`;
    const node=`const cp=require('node:child_process'),assert=require('node:assert/strict');assert.equal(require('node:os').tmpdir(),process.env.TMPDIR);process.stdout.write(cp.execFileSync('sh',['-c','exec python3 -c "$1"','sh',process.argv[1]],{encoding:'utf8'}));`;
    assert.equal((await execute(process.execPath,['-e',node,python],{cwd:f.workspace,env})).stdout.trim(),'exchange-ok');
    assert.equal(api.scratchRelease(own),true);
  }
});
test('byte limits account for separators, multibyte names and terminating NUL',()=>{
  assert.equal(api.scratchSocketPath('é'.repeat(53)+'a',108).length,54);
  assert.throws(()=>api.scratchSocketPath('é'.repeat(54),108),/UNIX_SOCKET_PATH_TOO_LONG/);
  assert.equal(api.scratchSocketPath('x'.repeat(103),104).length,103);
  assert.throws(()=>api.scratchSocketPath('x'.repeat(104),104),/UNIX_SOCKET_PATH_TOO_LONG/);
  assert.throws(()=>api.scratchSocketPath('a\0b'),/UNIX_SOCKET_PATH_TOO_LONG/);
});
test('runtime metadata, inode and alias replacement cannot redirect an owner; v1 remains compatible',async t=>{
  const f=await fixture(t),one=f.allocate(),two=f.allocate();
  const environment=own=>({GESTALT_SESSION_TMP_CONTEXT:JSON.stringify(own.context)});
  const path=join(one.context.runtime.directory,'.gestalt-runtime.json'),original=await readFile(path,'utf8');
  await writeFile(path,await readFile(join(two.context.runtime.directory,'.gestalt-runtime.json'),'utf8'));
  assert.throws(()=>api.scratchEnvironment(environment(one)));await writeFile(path,original);
  const held=one.context.runtime.directory+'-held';await rename(one.context.runtime.directory,held);await symlink(held,one.context.runtime.directory);
  assert.throws(()=>api.scratchEnvironment(environment(one)));await rm(one.context.runtime.directory);await rename(held,one.context.runtime.directory);
  const legacy=await sessionScratch(f.workspace,'retained-legacy');
  assert.equal(api.scratchEnvironment(legacy.environment).TMPDIR,legacy.context.directory);
  assert.equal(api.scratchRelease(one),true);assert.equal(api.scratchRelease(two),true);
});
test('runtime-contained workspaces and borrowers retain all logical ancestor fences',async t=>{
  const f=await fixture(t),outer=f.allocate();
  const selected=join(outer.context.runtime.directory,'nested-workspace');await mkdir(selected);
  const inner=api.scratchAllocate(selected,'cli-session','inner-controller');
  t.after(()=>rm(inner.context.runtime.directory,{recursive:true,force:true}));
  const borrower=api.scratchBorrow(inner.context,'tool-fixture');
  assert.equal(api.scratchRelease(inner),false);
  assert.equal(api.scratchRelease(borrower),true);assert.equal(api.scratchRelease(inner),true);assert.equal(api.scratchRelease(outer),true);
  assert.equal((await readdir(outer.context.directory)).includes('.containment-lock'),false);
});
