// Test-only byte-work benchmark. No Native, provider, policy or home is launched.
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,writeFileSync,openSync,ftruncateSync,closeSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {spawn,spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const manager=resolve('public/gestalt'),source=readFileSync(manager,'utf8');
const program=source.split("<<'LIVE_POLICY'\n")[1].split('\nLIVE_POLICY\n')[0].split('const args = process.argv.slice(2);')[0];
const root=mkdtempSync(join(tmpdir(),'live-digest-benchmark-'));
try {
 const file=join(root,'neutral-image'),size=545123336,fd=openSync(file,'wx',0o600);try{ftruncateSync(fd,size);}finally{closeSync(fd);}
 const rows=[];
 // Fixed three alternating pairs, isolated processes and a 30s limit each.
 for(const mode of ['baseline','scoped','scoped','baseline','baseline','scoped']) {
  const work=mode==='baseline'?`let sha;for(let i=0;i<3;i++)sha=digest(readFileSync(file));const stats={calls:3,reads:3,hits:0,bytes:size*3,uniqueBytes:size};`:
   `const scope=digestScope();let sha;for(let i=0;i<3;i++)sha=fileDigest(file,scope);digestFence(scope);const stats=scope.stats;`;
  const body=`process.env.GESTALT_LIVE_MANAGER_SOURCE=${JSON.stringify(manager)};\n`+program+`\nconst file=${JSON.stringify(file)},size=${size},start=process.hrtime.bigint(),cpu=process.cpuUsage();${work}
console.log(JSON.stringify({mode:${JSON.stringify(mode)},stats,sha,wallMs:Number(process.hrtime.bigint()-start)/1e6,cpu:process.cpuUsage(cpu),maxRssKiB:process.resourceUsage().maxRSS}));`;
  const result=spawnSync(process.execPath,['--input-type=module','-'],{input:body,encoding:'utf8',timeout:30000,maxBuffer:4096,env:{PATH:'/usr/bin:/bin'}});
  assert.equal(result.status,0,result.stderr||result.error?.message);rows.push(JSON.parse(result.stdout));
 }
 assert.equal(new Set(rows.map(row=>row.sha)).size,1);
 for(const row of rows)assert.equal(row.stats.bytes,size*(row.mode==='baseline'?3:1));
 const cancellation=await new Promise((done,reject)=>{
  const body=`process.env.GESTALT_LIVE_MANAGER_SOURCE=${JSON.stringify(manager)};\n`+program+`\nprocess.stdout.write('started\\n');for(let i=0;i<100;i++)fileDigest(${JSON.stringify(file)},digestScope());`;
  const child=spawn(process.execPath,['--input-type=module','-'],{stdio:['pipe','pipe','pipe'],env:{PATH:'/usr/bin:/bin'}});
  let output='',error='',sentAt;const timer=setTimeout(()=>{child.kill('SIGKILL');reject(Error('owned cancellation exceeded 2s'));},2000);
  child.once('error',reject);child.stdout.on('data',chunk=>{output+=chunk;if(!sentAt&&output==='started\n'){sentAt=process.hrtime.bigint();child.kill('SIGTERM');}});child.stderr.on('data',chunk=>error+=chunk);
  child.once('close',(code,signal)=>{clearTimeout(timer);try{assert.ok(sentAt);assert.equal(signal,'SIGTERM');assert.equal(error,'');done({signal,closed:true,afterSignalMs:Number(process.hrtime.bigint()-sentAt)/1e6});}catch(e){reject(e);}});child.stdin.end(body);
 });
 console.log(JSON.stringify({cancellation,kind:'bounded-neutral-byte-work-not-Native-or-full-validation-timing',managerSha256:createHash('sha256').update(source).digest('hex'),fixtureBytes:size,pairs:3,timeoutPerProcessMs:30000,rows},null,2));
}finally{rmSync(root,{recursive:true,force:true});}
