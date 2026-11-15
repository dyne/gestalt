import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdir,mkdtemp,readFile,writeFile,rm,realpath} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {promisify} from 'node:util';
import test from 'node:test';

// Explicit host/CI gate: executes a real native sandbox, never a mock or a
// relaxed profile. Missing prerequisites fail rather than skip the regression.
const execute=promisify(execFile);
const source=await readFile(resolve('public/gestalt'),'utf8');
const program=source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const api=new Function('require',program+'\nreturn {scratchAllocate,scratchRelease,scratchEnvironment};')(createRequire(import.meta.url));
test('real workspace sandbox permits bounded session sockets while preserving sealed configuration restrictions',{timeout:30000},async t=>{
  const root=await realpath(await mkdtemp(join(tmpdir(),'scratch-native-')));
  const workspace=join(root,'workspace'),home=join(root,'codex-home');
  await mkdir(workspace);await mkdir(home);
  const clean={...process.env,CODEX_HOME:home};
  for(const key of ['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'])delete clean[key];
  await execute('git',['init','-q',workspace],{env:clean});
  await mkdir(join(workspace,'.codex'));
  const sealed=join(workspace,'.codex','protected');await writeFile(sealed,'preserved');
  const own=api.scratchAllocate(workspace,'cli-session','native-test');
  t.after(async()=>{assert.equal(api.scratchRelease(own),true);await rm(own.context.runtime.directory,{recursive:true,force:true});await rm(root,{recursive:true,force:true});});
  const env=api.scratchEnvironment({...clean,GESTALT_SESSION_TMP_CONTEXT:JSON.stringify(own.context)});
  const code=`import os,socket,tempfile,sys
root=tempfile.gettempdir();assert root==os.path.realpath(root)==os.environ['TMP']==os.environ['TEMP'];assert len(root.encode())<=48
try:
 with open(sys.argv[1],'w') as file: file.write('forbidden')
except OSError: pass
else: raise AssertionError('sandbox configuration restriction was broadened')
path=root+'/bitcoin_sock0_qR2DkZ'
with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as server:
 server.settimeout(5);server.bind(path);server.listen(1)
 with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as client:
  client.settimeout(5);client.connect(path);client.sendall(b'ping');peer,_=server.accept()
  with peer:
   assert peer.recv(4)==b'ping';peer.sendall(b'pong');assert client.recv(4)==b'pong'
os.unlink(path);print('native-socket-and-restriction-ok')`;
  const binary=process.env.GESTALT_TEST_CODEX || resolve('node_modules/.bin/codex');
  const policy=['-c','default_permissions=":workspace"','-c','approval_policy="never"','sandbox','--'];
  const result=await execute(binary,[...policy,'python3','-c',code,sealed],{cwd:workspace,env,timeout:20000});
  assert.equal(result.stdout.trim(),'native-socket-and-restriction-ok');
  assert.equal(await readFile(sealed,'utf8'),'preserved');
  const cpp=join(workspace,'socket-probe');
  await execute('c++',['-std=c++17',resolve('tests/fixtures/session-scratch-socket.cpp'),'-o',cpp],{env,cwd:workspace,timeout:20000});
  const native=await execute(binary,[...policy,cpp],{cwd:workspace,env,timeout:20000});
  assert.equal(native.stdout.trim(),`cpp-socket-ok ${process.platform==='linux'?108:104}`);
});
