// Actual unchanged Org publication and whole authenticated NativeLocalIpc.
// Separate raw credential measurement is diagnostic, never whole-IPC success.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { lstat, readFile, realpath, writeFile } from 'node:fs/promises';
import { createConnection } from 'node:net';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { NativePrivateStorage } from '../../src/cli/platform/storage.ts';
import { NativeLocalIpc } from '../../src/cli/platform/ipc.ts';
import { verifySourceRecord } from './native-runtime/source-binding.mjs';
const [agents, peerProbe, fixture, expectedFile, resultFile] = process.argv.slice(2);
const expected = JSON.parse(await readFile(expectedFile,'utf8'));
const result={observedAt:new Date().toISOString(),pid:process.pid,uid:process.getuid(),gid:process.getgid(),expected,
  actualIdentity:JSON.parse(execFileSync(peerProbe,['--identity',String(process.pid)],{encoding:'utf8'})),ancestors:[],sourceBindings:[]};
for(let current=await realpath(fixture);;current=path.dirname(current)) {
  const entry=await lstat(current);result.ancestors.push({path:current,uid:entry.uid,gid:entry.gid,mode:(entry.mode&0o7777).toString(8)});
  if(current===path.dirname(current))break;
}
try {
  const binding=JSON.parse(await readFile(new URL('./native-runtime/agents-source.json',import.meta.url),'utf8'));
  const head=execFileSync('git',['-C',agents,'rev-parse','HEAD'],{encoding:'utf8'}).trim();
  assert.ok([binding.acceptedCommit,binding.remoteCheckoutCommit].includes(head));result.agentsCheckout=head;
  for(const record of binding.files) {
    const file=path.join(agents,record.path),bytes=await readFile(file),info=await lstat(file);
    result.sourceBindings.push({path:record.path,...verifySourceRecord(record,{bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex'),mode:(info.mode&0o7777).toString(8),regularFile:info.isFile()&&!info.isSymbolicLink()},execFileSync('git',['-C',agents,'ls-tree','HEAD','--',record.path],{encoding:'utf8'}).trim())});
  }
  const library=path.join(agents,'plugins/gestalt/skills/org-plan/scripts');
  const {ensureManagedRoot}=await import(pathToFileURL(path.join(library,'org-plan-storage.mjs')));
  try {ensureManagedRoot(await realpath(path.join(fixture,'status')),{privateMode:false,create:false});result.guard={ok:true};}
  catch(error){result.guard={ok:false,error:error.message};}
  const {publishStatus}=await import(pathToFileURL(path.join(library,'org-plan-publication.mjs')));
  result.publication=publishStatus(await realpath(path.join(fixture,'probe.org')),'private Unix compatibility probe');
  if(result.publication.published) {
    const value=JSON.parse(await readFile(result.publication.path,'utf8')),info=await lstat(result.publication.path);
    result.publishedFile={planMatches:value.planPath===await realpath(path.join(fixture,'probe.org')),schemaVersion:value.schemaVersion,uid:info.uid,mode:(info.mode&0o7777).toString(8)};
  }
  const socket=createConnection({path:'s'});await once(socket,'connect');
  try {
    const child=spawn(peerProbe,['--peer'],{shell:false,stdio:['ignore','pipe','pipe',socket]});let out='',err='';
    child.stdout.on('data',b=>{out+=b});child.stderr.on('data',b=>{err+=b});
    const [exit,signal]=await once(child,'close');result.sameSocketCredentials={exit,signal,stderr:err,peer:exit===0?JSON.parse(out):null};
  } finally {socket.destroy();}
  try {
    const storage=await NativePrivateStorage.open(fixture,peerProbe),ipc=new NativeLocalIpc(storage,peerProbe),request=Buffer.from('authenticated host echo é東京');
    const streams=await ipc.connect('s',expected),chunks=[];await streams.stdout(request);let size=0;
    for await(const bytes of streams.stdin){chunks.push(Buffer.from(bytes));size+=bytes.length;if(size>=request.length)break;}
    result.ipc={ok:Buffer.concat(chunks).equals(request),requestHex:request.toString('hex'),replyHex:Buffer.concat(chunks).toString('hex')};
    try {const wrong=await ipc.connect('s',{...expected,started:expected.started+'-wrong'});wrong.stdin.destroy?.();result.identityMismatch={refused:false};}
    catch(error){result.identityMismatch={refused:/creation identity differs/.test(error.message),error:error.message};}
  } catch(error){result.ipc={ok:false,error:error.message};}
  result.pass=result.guard?.ok===true&&result.publication.published===true&&result.publishedFile?.planMatches===true&&result.publishedFile?.schemaVersion===1&&result.publishedFile?.uid===process.getuid()&&result.publishedFile?.mode==='600'&&result.ipc?.ok===true&&result.identityMismatch?.refused===true&&result.sameSocketCredentials.exit===0;
} catch(error){result.pass=false;result.error={message:error.message,stack:error.stack};}
await writeFile(resultFile,JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify({pass:result.pass,publication:result.publication,ipc:result.ipc,credentials:result.sameSocketCredentials}));
process.exitCode=result.pass?0:1;
