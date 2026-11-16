// Explicit Unix authority gate; missing prerequisites FAIL, never skip/pass.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { chmod, mkdir, mkdtemp, readFile, realpath, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { build, version as esbuildVersion } from 'esbuild';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
assert.ok(['linux','darwin'].includes(process.platform));
const root=path.resolve(import.meta.dirname,'..'),args=process.argv.slice(2);
assert.equal(args[0],'--agents');assert.equal(args[2],'--evidence');
const agents=path.resolve(args[1]),evidence=path.resolve(args[3]);await mkdir(evidence,{recursive:true,mode:0o700});
const fixture=await realpath(await mkdtemp(path.join(tmpdir(),'gu-')));await mkdir(path.join(fixture,'status'),{mode:0o700});
const plan=['#+TITLE: Private compatibility probe','#+SUBTITLE: Disposable fixture only','#+DATE: 2026-11-16','#+KEYWORDS: private compatibility','* TODO [#A] Probe',':PROPERTIES:',':ID: compat-probe',':SKILLS: $gestalt:coding',':REVIEW_STATUS: UNREVIEWED',':END:',
  '- Effort :: Small.','- Goal :: Test isolated publication.','- Notes :: Never a session plan.',
  ...['first','second'].flatMap(id=>['** TODO [#A] '+id,':PROPERTIES:',':ID: compat-'+id,':END:','- Why :: Test inherited authority.','- Change :: Private fixture only.','- Tests :: Actual publication and IPC.','- Done when :: Probe completes.'])].join('\n')+'\n';
await writeFile(path.join(fixture,'probe.org'),plan,{mode:0o600});
const module=path.join(evidence,'interactive-producer.mjs');
const compiled=await build({entryPoints:[path.join(root,'src/cli/platform/interactive.ts')],outfile:module,bundle:true,platform:'node',target:'node24',format:'esm',metafile:true});
const digest=async file=>({path:file,bytes:(await readFile(file)).length,sha256:createHash('sha256').update(await readFile(file)).digest('hex')});
await writeFile(path.join(evidence,'producer-build.json'),JSON.stringify({esbuildVersion,artifact:await digest(module),inputs:await Promise.all(Object.keys(compiled.metafile.inputs).map(file=>digest(path.resolve(file))))},null,2)+'\n');
const peer=await buildNativePlatform(root,path.join(evidence,'native')),host=JSON.parse(execFileSync(peer,['--identity',String(process.pid)],{encoding:'utf8'}));
await writeFile(path.join(fixture,'host.json'),JSON.stringify(host));
const sockets=new Set(),measurements=[],pending=[];
const server=createServer(socket=>{
  sockets.add(socket);socket.on('close',()=>sockets.delete(socket));socket.on('error',()=>{});
  const measurement=(async()=>{
    const probe=spawn(peer,['--peer'],{shell:false,stdio:['ignore','pipe','pipe',socket]});let out='',err='';
    probe.stdout.on('data',b=>{out+=b});probe.stderr.on('data',b=>{err+=b});const [exit,signal]=await once(probe,'close');
    const record={exit,signal,stderr:err,peer:exit===0?JSON.parse(out):null};measurements.push(record);return record;
  })();pending.push(measurement);
  socket.on('data',bytes=>{void measurement.then(()=>{if(!socket.destroyed)socket.write(bytes);});});
});
const oldCwd=process.cwd();process.chdir(fixture);server.listen('s');await once(server,'listening');await chmod('s',0o600);
const resultFile=path.join(evidence,'authority.json'),before=path.join(evidence,'before-exec.json');
const environment={...process.env,GESTALT_TEST_INTERACTIVE_MODULE:module,GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY:path.join(fixture,'status')};delete environment.GESTALT_MOBILE_ORG_PLAN_STATUS_FILE;
const argv=[path.join(root,'tests/fixtures/unix-compat-entry.mjs'),process.execPath,peer,before,fixture,path.join(root,'tests/fixtures/unix-compat-authority.mjs'),agents,peer,fixture,path.join(fixture,'host.json'),resultFile];
const child=spawn(process.execPath,argv,{env:environment,shell:false,stdio:['ignore','pipe','pipe']});let stdout='',stderr='';child.stdout.on('data',b=>{stdout+=b});child.stderr.on('data',b=>{stderr+=b});
const timeout=setTimeout(()=>child.kill('SIGKILL'),20000);let status;
try {
  const [exit,signal]=await once(child,'close');status={exit,signal};await Promise.allSettled(pending);
  const observed=JSON.parse(await readFile(resultFile,'utf8')),previous=JSON.parse(await readFile(before,'utf8'));
  assert.deepEqual(observed.actualIdentity,previous.identity);assert.equal(observed.pid,child.pid);assert.deepEqual(observed.sameSocketCredentials?.peer,host);
  assert.ok(measurements.some(m=>m.exit===0&&m.peer.pid===child.pid&&m.peer.started===previous.identity.started&&m.peer.executable===previous.identity.executable&&m.peer.uid===process.getuid()));
  process.exitCode=exit===0&&observed.pass?0:1;
} finally {
  clearTimeout(timeout);for(const socket of sockets)socket.destroy();await new Promise(resolve=>server.close(resolve));process.chdir(oldCwd);
  await writeFile(path.join(evidence,'execution.json'),JSON.stringify({observedAt:new Date().toISOString(),command:['node','tests/native-unix-compatibility.mjs',...args],argv,fixture,host,status,stdout,stderr,measurements,ownerContextPreserved:{GESTALT_SESSION_TMP_CONTEXT:process.env.GESTALT_SESSION_TMP_CONTEXT??null,TMPDIR:process.env.TMPDIR??null,TMP:process.env.TMP??null,TEMP:process.env.TEMP??null},limits:'producer/unchanged guard and whole IPC only; no full L4 consumer slice or managed complete-tree claim'},null,2)+'\n');
}
