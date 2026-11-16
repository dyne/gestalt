// Explicit Unix authority gate; missing prerequisites FAIL, never skip/pass.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { openSync, writeFileSync, fsyncSync, closeSync } from 'node:fs';
import { observeHostSocket, readAuthorityReport } from './fixtures/unix-compat-peer.mjs';
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
const progressFile=path.join(evidence,'host-progress.jsonl');
function phase(name,detail={}){const fd=openSync(progressFile,'a',0o600);try{writeFileSync(fd,JSON.stringify({at:new Date().toISOString(),phase:name,...detail})+'\n');fsyncSync(fd);}finally{closeSync(fd);}}
phase('host-start');
const fixture=await realpath(await mkdtemp(path.join(tmpdir(),'gu-')));await mkdir(path.join(fixture,'status'),{mode:0o700});
const plan=['#+TITLE: Private compatibility probe','#+SUBTITLE: Disposable fixture only','#+DATE: 2026-11-16','#+KEYWORDS: private compatibility','* TODO [#A] Probe',':PROPERTIES:',':ID: compat-probe',':SKILLS: $gestalt:coding',':REVIEW_STATUS: UNREVIEWED',':END:',
  '- Effort :: Small.','- Goal :: Test isolated publication.','- Notes :: Never a session plan.',
  ...['first','second'].flatMap(id=>['** TODO [#A] '+id,':PROPERTIES:',':ID: compat-'+id,':END:','- Why :: Test inherited authority.','- Change :: Private fixture only.','- Tests :: Actual publication and IPC.','- Done when :: Probe completes.'])].join('\n')+'\n';
await writeFile(path.join(fixture,'probe.org'),plan,{mode:0o600});
const module=path.join(evidence,'interactive-producer.mjs');
phase('producer-build-start');
const compiled=await build({entryPoints:[path.join(root,'src/cli/platform/interactive.ts')],outfile:module,bundle:true,platform:'node',target:'node24',format:'esm',metafile:true});
const digest=async file=>({path:file,bytes:(await readFile(file)).length,sha256:createHash('sha256').update(await readFile(file)).digest('hex')});
await writeFile(path.join(evidence,'producer-build.json'),JSON.stringify({esbuildVersion,artifact:await digest(module),inputs:await Promise.all(Object.keys(compiled.metafile.inputs).map(file=>digest(path.resolve(file))))},null,2)+'\n');
phase('producer-build-complete');phase('peer-build-start');
const peer=await buildNativePlatform(root,path.join(evidence,'native')),host=JSON.parse(execFileSync(peer,['--identity',String(process.pid)],{encoding:'utf8'}));
await writeFile(path.join(fixture,'host.json'),JSON.stringify(host));
phase('peer-build-complete',{host});
const sockets=new Set(),measurements=[],pending=[];let nextConnection=0;
const server=createServer(socket=>{
  const connection=++nextConnection,mark=(name,detail={})=>phase(name,{connection,...detail});
  sockets.add(socket);mark('host-connection');socket.on('close',()=>{sockets.delete(socket);mark('host-connection-closed');});socket.on('error',error=>mark('host-socket-error',{message:error.message}));
  const measurement=observeHostSocket(peer,socket,mark).then(record=>{measurements.push({connection,...record});return record;}).catch(error=>{const record={error:error.message};measurements.push({connection,...record});mark('host-credential-error',record);return record;});pending.push(measurement);
  socket.on('data',bytes=>{mark('host-protocol-read',{bytes:bytes.length,hex:bytes.toString('hex')});void measurement.then(record=>{if(record.exit===0&&!socket.destroyed){mark('host-protocol-write-start',{bytes:bytes.length});socket.write(bytes,error=>mark('host-protocol-write-complete',{error:error?.message??null}));}});});
});
const oldCwd=process.cwd();process.chdir(fixture);server.listen('s');await once(server,'listening');await chmod('s',0o600);
phase('host-listening');
const resultFile=path.join(evidence,'authority.json'),before=path.join(evidence,'before-exec.json');
const environment={...process.env,GESTALT_TEST_INTERACTIVE_MODULE:module,GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY:path.join(fixture,'status')};delete environment.GESTALT_MOBILE_ORG_PLAN_STATUS_FILE;
const argv=[path.join(root,'tests/fixtures/unix-compat-entry.mjs'),process.execPath,peer,before,fixture,path.join(root,'tests/fixtures/unix-compat-authority.mjs'),agents,peer,fixture,path.join(fixture,'host.json'),resultFile];
phase('authority-spawn-start');
const child=spawn(process.execPath,argv,{env:environment,shell:false,stdio:['ignore','pipe','pipe']});let stdout='',stderr='';child.stdout.on('data',b=>{stdout+=b;phase('authority-stdout',{text:b.toString()});});child.stderr.on('data',b=>{stderr+=b;phase('authority-stderr',{text:b.toString()});});
phase('authority-spawned',{pid:child.pid});
let timedOut=false,status,observed=null,primaryError=null;
const timeout=setTimeout(()=>{timedOut=true;phase('authority-deadline',{pid:child.pid,stdout,stderr});child.kill('SIGKILL');},20000);
try {
  const [exit,signal]=await once(child,'close');clearTimeout(timeout);status={exit,signal};phase('authority-closed',{...status,timedOut});await Promise.allSettled(pending);
  if(timedOut||signal)primaryError={message:timedOut?'Authority deadline exceeded; child terminated':'Authority child terminated by signal',status};
  const report=await readAuthorityReport(resultFile,primaryError);observed=report.observed;primaryError=report.primaryError;
  if(report.readError)phase('authority-report-read-failed',{message:report.readError});
  if(primaryError)throw Error(primaryError.message);
  const previous=JSON.parse(await readFile(before,'utf8'));
  assert.deepEqual(observed.actualIdentity,previous.identity);assert.equal(observed.pid,child.pid);assert.deepEqual(observed.sameSocketCredentials?.peer,host);
  assert.ok(measurements.some(m=>m.exit===0&&m.peer.pid===child.pid&&m.peer.started===previous.identity.started&&m.peer.executable===previous.identity.executable&&m.peer.uid===process.getuid()));
  process.exitCode=exit===0&&observed.pass?0:1;
} catch(error){primaryError??={message:error.message,stack:error.stack};process.exitCode=1;phase('authority-failed',{primaryError,lastAuthorityPhase:observed?.phase??null});
} finally {
  clearTimeout(timeout);for(const socket of sockets)socket.destroy();await new Promise(resolve=>server.close(resolve));process.chdir(oldCwd);
  phase('host-settled',{status,timedOut,primaryError});
  await writeFile(path.join(evidence,'execution.json'),JSON.stringify({timedOut,primaryError,lastAuthorityPhase:observed?.phase??null,observedAt:new Date().toISOString(),command:['node','tests/native-unix-compatibility.mjs',...args],argv,fixture,host,status,stdout,stderr,measurements,ownerContextPreserved:{GESTALT_SESSION_TMP_CONTEXT:process.env.GESTALT_SESSION_TMP_CONTEXT??null,TMPDIR:process.env.TMPDIR??null,TMP:process.env.TMP??null,TEMP:process.env.TEMP??null},limits:'producer/unchanged guard and whole IPC only; no full L4 consumer slice or managed complete-tree claim'},null,2)+'\n');
}
