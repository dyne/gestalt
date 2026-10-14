// Isolated hosted proof. Actual Native model tools use a controlled provider; no genuine credentials, deployment or IPC repair.
import assert from 'node:assert/strict';
import {spawn,execFileSync} from 'node:child_process';
import {createHash,randomUUID,randomBytes} from 'node:crypto';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,readlinkSync,realpathSync,lstatSync,readdirSync,openSync,closeSync,rmSync,renameSync,symlinkSync,copyFileSync,chmodSync,existsSync} from 'node:fs';
import {createServer,createConnection} from 'node:net';
import {request as httpRequest,createServer as createHttpServer} from 'node:http';
import {gunzipSync,inflateSync,zstdDecompressSync} from 'node:zlib';
import {dirname,join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';

const hostedActions=process.env.GITHUB_ACTIONS==='true';
const binary=realpathSync(process.env.LIVE_HOST_CODEX_BINARY);
const mobileModule=realpathSync(process.env.LIVE_HOST_MOBILE_MODULE);
const electionModule=realpathSync(process.env.LIVE_HOST_ELECTION_MODULE);
const caddyBinary=realpathSync(process.env.LIVE_HOST_CADDY_BINARY);
const evidence=resolve(process.env.LIVE_HOST_POLICY_EVIDENCE ?? 'test-results/live-host-policy');
const manager=realpathSync(process.env.LIVE_HOST_MANAGER_BINARY);
const expectedBinary='b2730ae901da317bcc5933d9b87de6dfbea3e46a46b64f806e4268fd9a095578';
const runtimePinsPath='tests/fixtures/live-host-native-runtime.json';
const runtimePins=JSON.parse(readFileSync(runtimePinsPath));
const expectedBwrap='335aa87510fc0eee1bcdbedd3d9bf4aa4d112cffed72d5f1ad7a10b6cc855066';
const expectedSandboxHelper='396915091a1f0387b48fd06e98d06e61d22f6ff6af33bc806891a634ee03d199';
assert.equal(runtimePins.binarySha256,expectedBinary);
assert.equal(runtimePins.bwrapSha256,expectedBwrap);
assert.equal(runtimePins.sandboxHelperSha256,expectedSandboxHelper);
assert.equal(runtimePins.managerHead,'75e420c5f65b1b1acb0cd0b3615564d462e365ce');
assert.equal(runtimePins.runId,'38645681271');
const mobilePin='c558dd644e4b6ac8d3bce9e57bce762791ea25de';
const acceptedMobilePin='c558dd644e4b6ac8d3bce9e57bce762791ea25de';
const mobileInputPaths=['src/server/platform/codex/json-rpc-client.ts','src/server/platform/catalog/profile-command.ts','src/server/platform/codex/codex-process-launcher.ts'];
const agentsPin='4c4f4566ac968e1b7d425e825ba2ad9a8d9f8f1f';
const hash=data=>createHash('sha256').update(data).digest('hex');
const pause=ms=>new Promise(done=>setTimeout(done,ms));
// Match pinned native legacy resource resolution without relocating its binary.
const bwrapCandidates=[join(dirname(binary),'codex-resources','bwrap'),join(dirname(dirname(binary)),'codex-resources','bwrap'),join(dirname(binary),'bwrap')];
const bundledBwrap=bwrapCandidates.find(path=>existsSync(path)&&(lstatSync(path).mode&0o111));
assert.ok(bundledBwrap,'PINNED_NATIVE_PACKAGE_RESOURCE_MISSING');
assert.equal(createHash('sha256').update(readFileSync(bundledBwrap)).digest('hex'),expectedBwrap);
const sandboxHelper=join(dirname(binary),'codex-linux-sandbox');
assert.equal(createHash('sha256').update(readFileSync(sandboxHelper)).digest('hex'),expectedSandboxHelper);
const nativeDiagnostic=(text,roles={})=>({createFileFailureRole:(()=>{const match=text.match(/Can't create file at (.+): Read-only file system/);if(!match)return undefined;const path=match[1].replace(/^["']|["']$/g,'');return Object.entries(roles).filter(([,root])=>path===root||path.startsWith(root+'/')).sort((a,b)=>b[1].length-a[1].length)[0]?.[0]??'undeclared-path';})(),errorTemplates:text.split('\n').filter(line=>/^(?:Error:|error:|error building bubblewrap command:|bwrap:)/.test(line)).slice(0,3).map(line=>line.replace(/Can't create file at .+: Read-only file system/,"Can't create file at <path>: Read-only file system").replace(/"[^"]*"|'[^']*'/g,'<quoted>').replace(/\/[^\s,;:)]+/g,'<path>').replace(/fixture[-a-z]*/g,'<fixture>').replace(/[a-f0-9]{16,}/gi,'<token>').slice(0,220)),shellFailure:/command failed near line/.test(text),bytes:Buffer.byteLength(text),rustPanic:/panicked at/.test(text),
 nodeCodes:[...new Set(text.match(/\bERR_[A-Z_]+\b/g)??[])].slice(0,4),nodeErrnos:[...new Set(text.match(/\b(?:EACCES|EPERM|ENOENT|ENOTDIR|EISDIR|EROFS|ENOSYS)\b/g)??[])].slice(0,4),nodeErrors:['TypeError','SyntaxError','ReferenceError','RangeError'].filter(name=>text.includes(name+':')||text.includes(name+' [')),temporaryAliasesRefused:/Refusing to create helper binaries under temporary dir/.test(text),invalidSandboxState:/invalid --sandbox-state-json/.test(text),
 bubblewrapUnavailable:/bubblewrap is unavailable/.test(text),bundledResourceFailure:/failed to (open|exec) bundled bubblewrap/.test(text),
 daemonDirectoryRefused:/app-server socket directory/.test(text),permissionDenied:/Permission denied|Operation not permitted/.test(text),
 missingPath:/No such file or directory/.test(text),sourceLocations:[...text.matchAll(/([a-z_-]+\.rs):(\d+):(\d+)/g)].slice(0,3).map(match=>match.slice(1).join(':'))});

const managerResultDiagnostic=result=>{
 const diagnostic={code:result.code,signal:result.signal??null,spawnError:result.errorCode,stdoutBytes:Buffer.byteLength(result.stdout),stderrBytes:Buffer.byteLength(result.stderr),
  sameHomeRefused:/refusing to use HOME itself as CODEX_HOME/.test(result.stderr),missingCommand:/command not found|ENOENT/.test(result.stderr),
  nodeSyntaxError:/SyntaxError:/.test(result.stderr),shellFailure:/command failed near line/.test(result.stderr)};
 try{const parsed=JSON.parse(result.stdout);diagnostic.json=parsed&&typeof parsed==='object'&&!Array.isArray(parsed)?'valid-object':'invalid-shape';
  if(diagnostic.json==='valid-object'){diagnostic.schemaVersion=parsed.schemaVersion;diagnostic.available=parsed.available;
   diagnostic.reason=typeof parsed.reason==='string'&&/^[a-z-]{1,64}$/.test(parsed.reason)?parsed.reason:undefined;}}
 catch(error){diagnostic.json='invalid-json';diagnostic.errorClass=error.name==='SyntaxError'?'SyntaxError':'UnknownError';}
 return diagnostic;
};

const tree=execFileSync('git',['rev-parse','HEAD^{tree}'],{encoding:'utf8'}).trim();
const commit=execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim();
const provenance={repository:process.env.GITHUB_REPOSITORY,runId:process.env.GITHUB_RUN_ID,commit,tree,
 nativeMaskRevision:{...runtimePins,fixtureSha256:hash(readFileSync(runtimePinsPath))},
 agentsPackageLoaded:false,sourcePin:'979011409de0a60b52f179721948e65531d26144',mobilePin,acceptedMobilePin,agentsPin,nativeVersion:'0.161.0',
 mobileFixtureProvenanceSha256:hash(readFileSync('tests/fixtures/live-host-mobile-input/provenance.json')),
 mobileInputs:Object.fromEntries(mobileInputPaths.map(path=>[path,hash(readFileSync(join('.host-mobile-input',path)))])),
 bundledBwrap:{sha256:hash(readFileSync(bundledBwrap)),mode:(lstatSync(bundledBwrap).mode&0o777).toString(8)},
 electionModuleSha256:hash(readFileSync(electionModule)),
 electionInputs:Object.fromEntries(Object.keys(JSON.parse(readFileSync(join(dirname(manager),'election-inputs.json'))).inputs).sort().map(path=>[path,hash(readFileSync(path))])),
 binarySha256:hash(readFileSync(binary)),sourceBinaryMode:(lstatSync(binary).mode&0o777).toString(8),mobileLauncherSha256:hash(readFileSync(mobileModule)),
 files:Object.fromEntries(['public/gestalt','scripts/live-host-policy-native-proof.mjs','tests/live-host-policy.test.mjs','.github/workflows/live-host-policy-proof.yml','docs/caddy-live.md','tests/run.sh']
  .map(path=>[path,hash(readFileSync(path))]))};
mkdirSync(evidence,{recursive:true});
const root=mkdtempSync(join(dirname(manager),'live host policy ')),app=join(root,'app with spaces'),privateRoot=join(root,'controller'),nativeHome=join(privateRoot,'owning-runtime'),home=join(privateRoot,'sandbox-home'),bin=join(root,'bin');
for(const path of [app,privateRoot,nativeHome,home,bin])mkdirSync(path,{mode:0o700});
const admittedNode=join(bin,'admitted-node');copyFileSync(realpathSync(process.execPath),admittedNode);chmodSync(admittedNode,0o700);
const manifest=join(privateRoot,'manifest.json'),daemon=`/tmp/codex-daemon-${process.getuid()}`;
const socket=join(privateRoot,'controller.sock'),caddySocket=join(privateRoot,'caddy.sock');
const auth=join(privateRoot,'auth.fixture'),controlFile=join(privateRoot,'controller.fixture');
writeFileSync(auth,'private-auth-fixture',{mode:0o600});writeFileSync(controlFile,'private-controller-fixture',{mode:0o600});
const key=join(privateRoot,'live-policy.key');writeFileSync(key,randomBytes(32),{mode:0o600});
const publicFile=join(app,'public.txt'),readOnlyFile=join(app,'read-only.txt');writeFileSync(publicFile,'public-fixture');writeFileSync(readOnlyFile,'preserved-read-only');
const isolatedConfig='[shell_environment_policy]\ninherit = "none"\n';
writeFileSync(join(home,'config.toml'),isolatedConfig,{mode:0o600});
const profile='live-host-policy-proof',narrowProfile='live-host-policy-narrow';
const captureId=randomUUID(),sessionId=randomUUID(),sourcePath=join(privateRoot,'actual-source-inventory.json');
const metadataPaths=[];for(let path=app;;path=dirname(path)){metadataPaths.push(join(path,'.codex'));if(path==='/')break;}
writeFileSync(join(app,'AGENTS.md'),'Use the registered app and preserve private state.\n',{mode:0o644});
// Every Native invocation is through the reviewed managed CLI. The actual Mobile
// launcher keeps its original ChildProcess; this fixed adapter adds no runtime.
const quote=value=>"'"+value.replaceAll("'","'\\''")+"'";
writeFileSync(join(bin,'codex'),`#!/bin/sh\n[ "$#" = 2 ] && [ "$1" = app-server ] && [ "$2" = --stdio ] || exit 64\nexec ${quote(manager)} cli -- --live-policy-owning-runtime --manifest ${quote(manifest)} --capture-id ${quote(captureId)}\n`,{mode:0o700});
const controllerEnv={PATH:`${bin}:${dirname(process.execPath)}:/usr/bin:/bin`,HOME:privateRoot,CODEX_HOME:nativeHome,TMPDIR:'/tmp',LANG:'C.UTF-8'};
const managerEnv={...controllerEnv};
for(const name of Object.keys(process.env))delete process.env[name];Object.assign(process.env,controllerEnv);
const stable=value=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const identity=pid=>{
 const raw=readFileSync(`/proc/${pid}/stat`,'utf8');
 return {pid,birth:raw.slice(raw.lastIndexOf(')')+2).split(' ')[19],uid:Number(readFileSync(`/proc/${pid}/status`,'utf8').match(/^Uid:\s+(\d+)/m)[1]),
 namespaces:Object.fromEntries(['mnt','user','pid'].map(name=>[name,readlinkSync(`/proc/${pid}/ns/${name}`)]))};
};
const report={schemaVersion:1,provenance,stage:'bootstrap',passed:false,checks:{},controller:identity(process.pid),
 limitations:['Controlled provider; genuine credential-dependent provider proof remains L11.','No production MCP admission or Live picker flow; L10/L11 remain separate gates.']};
const rpcFailureDiagnostic=(method,error,roles={})=>{
 const text=typeof error.message==='string'?error.message:'';
 const errnoSuffixes={EISDIR:'Is a directory',ENOTDIR:'Not a directory',EEXIST:'File exists',EBADF:'Bad file descriptor',EINVAL:'Invalid argument',EROFS:'Read-only file system',EACCES:'Permission denied',EPERM:'Operation not permitted',ENOENT:'No such file or directory',EBUSY:'Device or resource busy',ENOSPC:'No space left on device',ELOOP:'Too many levels of symbolic links'};
 const bwrapMatch=text.match(/bwrap: Can't create file at ([^\n]+): ([^\n]+)$/);
 const errnoClass=bwrapMatch?Object.entries(errnoSuffixes).find(([,phrase])=>bwrapMatch[2]===phrase)?.[0]??'unclassified':undefined;
 const operation=bwrapMatch?{kind:'create-file',errnoClass,targetRole:Object.entries(roles).find(([,path])=>bwrapMatch[1]===path)?.[0]??'unclassified'}:undefined;
 return {operation,method,code:typeof error.code==='number'?error.code:undefined,messageBytes:Buffer.byteLength(text),messageSha256:hash(text),
  threadCreation:/^error creating thread:/i.test(text),configLoad:/config|configuration/i.test(text),
  permissionDenied:/Permission denied|Operation not permitted|os error (?:1|13)\b/i.test(text),readOnly:/Read-only file system|os error 30\b/i.test(text),missingPath:/No such file|os error 2\b/i.test(text),
  permissionProfile:/permission.?profile/i.test(text),instructionDiscovery:/AGENTS|instruction|project.?doc/i.test(text),filesystemHelper:/fs.helper|filesystem.helper|bubblewrap|bwrap/i.test(text),
  bubblewrapCreateFile:/bwrap: Can\'t create file at/i.test(text),protectedMountFailure:/synthetic.*mount|protected.*(?:target|path)|mount.*target/i.test(text),
  createFile:/creat(?:e|ing)|mkdir/i.test(text),readFile:/read|open/i.test(text),shellSnapshot:/shell.?snapshot/i.test(text),rollout:/rollout/i.test(text),
  matchedPathRoles:Object.entries(roles).filter(([,path])=>text.includes(path)).map(([role])=>role),
  sourceLocations:[...text.matchAll(/([a-z_-]+\.rs):(\d+):(\d+)/g)].slice(0,3).map(match=>match.slice(1).join(':'))};
};
let runtime,caddy,unowned,active,extraFD,elected,providerFailure,thread,currentRpcMethod,controllerAccepted=0;
const server=createServer(peer=>{controllerAccepted++;peer.end();});
async function waitFor(fn,budget=12000){const end=Date.now()+budget;while(Date.now()<end){try{const result=fn();if(result)return result;}catch{}await pause(50);}throw Error('PROOF_DEADLINE');}
function managerCall(args){const result=spawn('bash',[manager,...args],{cwd:app,env:managerEnv,stdio:['ignore','pipe','pipe']});
 let stdout='',stderr='';result.stdout.on('data',b=>stdout+=b);result.stderr.on('data',b=>stderr+=b);
 return new Promise((done,reject)=>{const timer=setTimeout(()=>{result.kill('SIGKILL');reject(Error('MANAGER_DEADLINE'));},15000);result.on('error',error=>{clearTimeout(timer);done({code:null,signal:null,stdout,stderr,errorCode:['ENOENT','EACCES','EPERM'].includes(error.code)?error.code:'SPAWN_ERROR'});});result.on('close',(code,signal)=>{clearTimeout(timer);done({code,signal,stdout,stderr});});});}
const api=()=>new Promise((done,reject)=>{const req=httpRequest({socketPath:caddySocket,path:'/config/',method:'GET'},res=>{res.resume();res.on('end',()=>done(res.statusCode));});req.on('error',reject);req.end();});
async function rpc(method,params){currentRpcMethod=method;try{return await Promise.race([runtime.rpc.request(method,params),new Promise((_,reject)=>{const timer=setTimeout(()=>reject(Error('RPC_DEADLINE')),12000);timer.unref();})]);}
 catch(error){report.failedRpc=rpcFailureDiagnostic(method,error,{appRoot:app,originalHome:nativeHome,controllerDirectory:privateRoot,metadata:join(app,'.codex'),daemon});throw error;}}
function ownedPids(marker){const found=[];for(const pid of readdirSync('/proc').filter(name=>/^\d+$/.test(name))){try{
 const args=readFileSync(`/proc/${pid}/cmdline`).toString().split('\0');if(args.some(arg=>arg===marker))found.push(identity(Number(pid)));
 }catch{}}return found;}
const dead=processRecord=>{try{const raw=readFileSync(`/proc/${processRecord.pid}/stat`,'utf8');return raw.slice(raw.lastIndexOf(')')+2).split(' ')[0]==='Z'||identity(processRecord.pid).birth!==processRecord.birth;}catch{return true;}};
async function childProgram() {
 const depth=Number(process.argv[3]??0),stage=name=>process.stdout.write(JSON.stringify({kind:'probe-stage',depth,stage:name,pid:process.pid})+'\n');stage('startup');
 const fs=await import('node:fs'),net=await import('node:net'),cp=await import('node:child_process');
 const input=JSON.parse(process.argv[2]);let inputRoundtrip;
 if(depth===0){stage('project-input');inputRoundtrip=await new Promise(done=>{let received='',chunks=0;const stream=process.stdin,timer=setTimeout(()=>done({eof:false,bytes:Buffer.byteLength(received),chunks,fifo:fs.fstatSync(0).isFIFO(),matches:false}),1500);stream.on('data',chunk=>{received+=chunk;chunks++;});stream.on('end',()=>{clearTimeout(timer);done({eof:true,bytes:Buffer.byteLength(received),chunks,fifo:fs.fstatSync(0).isFIFO(),matches:received===input.stdinExpected});});stream.on('error',()=>{clearTimeout(timer);done({eof:false,matches:false});});});}
 stage('filesystem');
 const denied=path=>{try{fs.readFileSync(path);return false;}catch(e){return ['EACCES','EPERM','ENOENT','ENOTDIR'].includes(e.code);}};
 const socketDenied=path=>new Promise(done=>{const peer=net.createConnection(path);let finished=false,timer;const end=value=>{if(finished)return;finished=true;clearTimeout(timer);peer.destroy();done(value);};timer=setTimeout(()=>end(false),500);peer.on('connect',()=>end(false));peer.on('error',e=>end(['EACCES','EPERM','ENOENT','ENOTDIR'].includes(e.code)));peer.setTimeout(500,()=>end(false));});
 let readOnlyPreserved=false;try{fs.writeFileSync(input.readOnlyFile,'escape');}catch(e){readOnlyPreserved=['EACCES','EPERM','EROFS'].includes(e.code);}
 const checks={readOnlyPreserved,publicRead:fs.readFileSync(input.publicFile,'utf8')==='public-fixture',
 privateFiles:input.privateFiles.every(denied),procAliases:input.aliases.every(denied),
 environment:!Object.keys(process.env).some(name=>/TOKEN|SECRET|API_KEY|AUTH|GESTALT_MOBILE_ORG/.test(name))};
 checks.extraFD=true;
 for(let fd=3;fd<128;fd++){try{const info=fs.fstatSync(fd);if(info.dev===input.fdIdentity.dev&&info.ino===input.fdIdentity.ino)checks.extraFD=false;}catch{}}
 try{fs.writeFileSync(input.writeFile+'-'+depth,'public-write');checks.publicWrite=true;}catch(e){checks.publicWrite=false;}
 if(input.metadataPath){checks.metadataInjection=true;for(const name of ['config.toml','hooks.json','rules','agents']){try{fs.writeFileSync(input.metadataPath+'/'+name,'injection');checks.metadataInjection=false;}catch(e){if(!['EACCES','EPERM','ENOENT','ENOTDIR','EROFS'].includes(e.code))checks.metadataInjection=false;}}}
 stage('controller-socket');checks.controllerSocket=await socketDenied(input.controllerSocket);stage('caddy-socket');checks.caddySocket=await socketDenied(input.caddySocket);stage('daemon');
 try{checks.daemonMask=(fs.lstatSync(input.daemon).mode&0o777)===0;}catch{checks.daemonMask=false;}
 try{fs.readdirSync(input.daemon);checks.daemon=false;}catch(e){checks.daemon=['EACCES','EPERM','ENOENT'].includes(e.code);}
 if(depth===0&&input.publicSocket){
  const code=`import os,socket,json\ntry:open(${JSON.stringify(input.privateFiles[0])}).read();raise Exception('DEVSERVER_PRIVATE_BREACH')\nexcept (PermissionError,FileNotFoundError):pass\ns=socket.socket(socket.AF_UNIX);s.bind(${JSON.stringify(input.publicSocket)});s.listen(8)\nprint(json.dumps({'kind':'devserver-ready','privateDenied':True,'pid':os.getpid()}),flush=True)\nwhile True:\n c,_=s.accept();c.recv(4096);body=open(${JSON.stringify(input.publicFile)},'rb').read();c.sendall(b'HTTP/1.1 200 OK\\r\\nConnection: close\\r\\nContent-Length: '+str(len(body)).encode()+b'\\r\\n\\r\\n'+body);c.close()\n`;
  cp.spawn('/usr/bin/python3',['-u','-c',code],{stdio:['ignore','inherit','inherit']}).on('error',()=>stage('devserver-spawn-error'));
 }
 stage('ordinary-command');const ordinary=cp.spawnSync('/usr/bin/true',[],{stdio:'ignore',timeout:2000});checks.ordinaryCommand=ordinary.status===0;
 process.stdout.write(JSON.stringify({kind:'probe-result',depth,checks,inputRoundtrip,uid:process.getuid(),pid:process.pid})+'\n');
 if(depth<2){stage('descendant-spawn');cp.spawn(process.execPath,[process.argv[1],process.argv[2],String(depth+1)],{stdio:'inherit',detached:depth===1}).on('error',()=>stage('descendant-spawn-error'));}
 process.on('SIGTERM',()=>{});
 if(depth===0&&input.mode==='leader-exit')setTimeout(()=>process.exit(0),1200);
 setInterval(()=>{},1000);
}
const childFile=join(app,'native helper.mjs');writeFileSync(childFile,`await (${childProgram.toString()})();\n`);
let m;
const save=()=>writeFileSync(manifest,JSON.stringify(m),{mode:0o600});
async function prepare(){save();const priorStage=report.stage;report.stage=priorStage+'/prepare';
 const result=await managerCall(['live-policy','prepare','--manifest',manifest]);
 report.lastPrepare=managerResultDiagnostic(result);
 if(report.lastPrepare.json!=='valid-object')throw Error('MANAGER_OUTPUT_INVALID');
 const parsed=JSON.parse(result.stdout);
 assert.equal(result.code,0,JSON.stringify(report.lastPrepare));report.stage=priorStage;return parsed;}

async function launch(mode='hold') {
 const stdinChunks=['native input with spaces\n','second ','line\n'];
 const id=randomUUID(),input={mode,publicSocket:join(app,`public-devserver-${id}.sock`),metadataPath:join(app,'.codex'),stdinExpected:stdinChunks.join(''),publicFile,readOnlyFile,writeFile:join(app,'written'),privateFiles:[auth,controlFile,manifest,key,join(nativeHome,'config.toml')],
 controllerSocket:socket,caddySocket,daemon,fdIdentity:{dev:lstatSync(auth).dev,ino:lstatSync(auth).ino},aliases:[`/proc/${process.pid}/root${auth}`,`/proc/${process.pid}/fd/${extraFD}`,`/proc/self/root${auth}`]};
 m.commands.probe.args=[childFile,JSON.stringify(input)];await prepare();
 const child=spawn('bash',[manager,'live-policy','run','--manifest',manifest,'--command','probe','--launch-id',id],
 {cwd:app,env:{...managerEnv,OPENAI_API_KEY:'fixture-secret',GESTALT_MOBILE_ORG_PLAN_MEASUREMENT_TOKEN:'fixture-secret'},stdio:['pipe','pipe','pipe','ignore','ignore','ignore','ignore',extraFD]});
 void(async()=>{await waitFor(()=>output.includes('project-input'));for(const chunk of stdinChunks){child.stdin.write(chunk);await pause(50);}child.stdin.end();})().catch(()=>{report.projectInputFeedFailed=true;child.stdin.destroy();});child.stdin.on('error',()=>{report.projectInputFeedFailed=true;});let output='',stderr='';child.stdout.on('data',b=>output+=b);child.stderr.on('data',b=>stderr+=b);
 let closed=false,exitCode,exitSignal;const ended=new Promise(done=>child.on('close',(code,signal)=>{closed=true;exitCode=code;exitSignal=signal;done(code);}));
 const result={id,child,ended,path:join(privateRoot,`process-${id}.json`),messages:()=>output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line)).filter(value=>value.kind==='probe-result'),
 state:()=>({closed,code:exitCode??null,signal:exitSignal??null,stdoutBytes:Buffer.byteLength(output),stderrBytes:Buffer.byteLength(stderr)}),diagnostic:()=>nativeDiagnostic(stderr,{policyDirectory:privateRoot,authentication:auth,controllerFile:controlFile,controllerSocket:socket,caddySocket,caddyConfig:join(privateRoot,'caddy.json'),manifest,key,runtimeHome:home,daemon})};
 child.stdout.on('data',()=>{try{report.probeStages=output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line)).filter(value=>value.kind==='probe-stage').slice(0,40).map(value=>({depth:value.depth,stage:value.stage,pid:value.pid}));report.nativeResults=result.messages().filter(value=>Number.isInteger(value.depth)&&value.depth>=0&&value.depth<=2).map(value=>({depth:value.depth,uid:value.uid,pid:value.pid,checks:Object.fromEntries(Object.entries(value.checks??{}).filter(([,flag])=>typeof flag==='boolean'))}));}catch{report.nativeOutputMalformed=true;}});
 result.publicSocket=input.publicSocket;active=result;return result;
}
async function nativeMessages(launch){await Promise.race([waitFor(()=>launch.messages().filter(value=>Number.isInteger(value.depth)&&value.depth>=0&&value.depth<=2).length===3),launch.ended.then(()=>{throw Error('NATIVE_LAUNCH_EARLY_EXIT');})]);}
function launchBoundary(launch){const state=launch.state();try{const body=JSON.parse(readFileSync(launch.path)).body;
 state.receipt={kind:body.kind,status:body.status,termination:body.termination,failure:body.failure};}catch(error){state.receipt={error:['ENOENT','EACCES','EPERM'].includes(error.code)?error.code:'INVALID_RECEIPT'};}return state;}
function ownedProcessSnapshot(launch){let rootPid;try{rootPid=JSON.parse(readFileSync(launch.path)).body.process.pid;}catch{return [];}
 let rootStdout;try{rootStdout=readlinkSync(`/proc/${rootPid}/fd/1`);}catch{}
 const pending=[rootPid],seen=new Set(),results=[];while(pending.length&&results.length<24){const pid=pending.shift();if(seen.has(pid))continue;seen.add(pid);
 try{const id=identity(pid),args=readFileSync(`/proc/${pid}/cmdline`).toString().split('\0'),raw=readFileSync(`/proc/${pid}/stat`,'utf8');
  const state=raw.slice(raw.lastIndexOf(')')+2).split(' ')[0],wchan=readFileSync(`/proc/${pid}/wchan`,'utf8').trim();
  const executable=readlinkSync(`/proc/${pid}/exe`);
  const role=executable===admittedNode?'admitted-probe':executable===binary?(args[0]?.includes('codex-linux-sandbox')?'native-linux-sandbox':'native-cli'):executable.endsWith('/bwrap')?'native-bwrap':executable==='/usr/bin/true'?'ordinary-command':'owned-helper';
  const stdio=Object.fromEntries([0,1,2].map(fd=>{try{const target=readlinkSync(`/proc/${pid}/fd/${fd}`);return [fd,{kind:target.startsWith('pipe:')?'pipe':target.startsWith('socket:')?'socket':target==='/dev/null'?'null':'file',sameAsNativeStdout:fd===1&&target===rootStdout}];}catch{return [fd,{kind:'unavailable'}];}}));
  results.push({...id,role,state,stdio,wchan:/^[a-zA-Z0-9_]{1,64}$/.test(wchan)?wchan:'unknown'});
  for(const tid of readdirSync(`/proc/${pid}/task`).filter(value=>/^\d+$/.test(value)).slice(0,64)){try{pending.push(...readFileSync(`/proc/${pid}/task/${tid}/children`,'utf8').trim().split(/\s+/).filter(Boolean).map(Number));}catch{}}
 }catch{}}return results;
}
async function closeOwned(child) {
 if (!child) return true;
 if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
 try { await waitFor(()=>child.exitCode !== null || child.signalCode !== null,2500); }
 catch { child.kill('SIGKILL');await waitFor(()=>child.exitCode !== null || child.signalCode !== null,2000); }
 return true;
}
async function stopped(launch){await Promise.race([launch.ended,pause(12000).then(()=>{throw Error('REAP_DEADLINE');})]);
 await waitFor(()=>JSON.parse(readFileSync(launch.path)).body.status==='exited');active=undefined;}
const negativeKinds=['config','hooks','rules','agents','replacement','symlink','stale-owner','generation','session','race'];
const cases=negativeKinds.map(kind=>({kind,app:join(root,`negative ${kind}`),directory:join(privateRoot,`case-${kind}`),profile:`live-case-${kind}`,capture:randomUUID(),session:randomUUID()}));
for(const c of cases){mkdirSync(c.app,{mode:0o700});mkdirSync(c.directory,{mode:0o700});mkdirSync(join(c.directory,'worker'),{mode:0o700});writeFileSync(join(c.directory,'worker','config.toml'),isolatedConfig,{mode:0o600});writeFileSync(join(c.app,'AGENTS.md'),'Preserve private state.\n',{mode:0o644});}
function nativeProfile(name,appRoot,write=true){
 const metadata=[];for(let path=appRoot;;path=dirname(path)){metadata.push(join(path,'.codex'));if(path==='/')break;}
 return `[permissions.${name}]\nextends = ":read-only"\n[permissions.${name}.filesystem]\n":root" = "read"\n${JSON.stringify(appRoot)} = "${write?'write':'read'}"\n${JSON.stringify(privateRoot)} = "deny"\n"/proc" = "deny"\n${JSON.stringify(daemon)} = "deny"\n${appRoot===app?JSON.stringify(readOnlyFile)+' = "read"\n':''}${metadata.map(path=>JSON.stringify(path)+' = "deny"\n').join('')}[permissions.${name}.network]\nenabled = false\n`;
}
function nativeState(appRoot,write=true){const metadata=[];for(let path=appRoot;;path=dirname(path)){metadata.push(join(path,'.codex'));if(path==='/')break;}
 return {permissionProfile:{type:'managed',file_system:{type:'restricted',entries:[{path:{type:'special',value:{kind:'root'}},access:'read'},{path:{type:'path',path:appRoot},access:write?'write':'read'},{path:{type:'path',path:privateRoot},access:'deny'},{path:{type:'path',path:'/proc'},access:'deny'},{path:{type:'path',path:daemon},access:'deny'},...(appRoot===app?[{path:{type:'path',path:readOnlyFile},access:'read'}]:[]),...metadata.map(path=>({path:{type:'path',path},access:'deny'}))]},network:'restricted'},sandboxCwd:pathToFileURL(appRoot).href,useLegacyLandlock:false};}
let publicSocket,publicIngress;
let roleSteps=new Map(),modelThreads=new Set(),primaryThread,modelPhase='broad',providerRequests=0,providerStage='request',providerShape;
const modelFinal=id=>({type:'response.output_item.done',item:{type:'message',role:'assistant',phase:'final_answer',id,content:[{type:'output_text',text:'Controlled confinement probe completed.'}]}});
const flatten=(tools,namespace)=>tools.flatMap(tool=>tool.type==='namespace'?flatten(tool.tools,tool.name):[{...tool,namespace}]);
function modelCall(tools,id,name,args,custom=false){const tool=tools.find(tool=>tool.name===name);assert.ok(tool,'NATIVE_TOOL_UNAVAILABLE');return {type:'response.output_item.done',item:{type:custom?'custom_tool_call':'function_call',call_id:id,name,input:custom?args:undefined,arguments:custom?undefined:JSON.stringify(args),...(tool.namespace?{namespace:tool.namespace}:{})}};}
const modelProbe=role=>`import os,socket,json\nprivate=${JSON.stringify([auth,controlFile,key,manifest])}\nfor path in private+${JSON.stringify([`/proc/${process.pid}/root${auth}`,`/proc/${process.pid}/fd/7`,`/proc/self/root${auth}`])}:\n try:\n  open(path,'rb').read();raise Exception('PRIVATE_BREACH')\n except OSError as e:\n  assert e.errno in (1,2,13,20)\nfor path in ${JSON.stringify([socket,caddySocket])}:\n s=socket.socket(socket.AF_UNIX)\n try:\n  s.connect(path);raise Exception('SOCKET_BREACH')\n except OSError as e:\n  assert e.errno in (1,2,13,20)\n finally:s.close()\nfor name in ['config.toml','hooks.json','rules','agents']:\n try:\n  open(${JSON.stringify(join(app,'.codex'))}+'/'+name,'w').write('injection');raise Exception('METADATA_BREACH')\n except OSError as e:\n  assert e.errno in (1,2,13,20,30)\nassert open(${JSON.stringify(publicFile)}).read()=='public-fixture'\nassert not any(('TOKEN' in k or 'SECRET' in k or 'API_KEY' in k) for k in os.environ)\nassert os.stat(${JSON.stringify(daemon)}).st_mode & 511 == 0\nprint('native-private-and-metadata-denied')\n`;
async function modelEvent(body){
 providerStage='model-identity';providerShape={expectedModel:body.model==='gpt-6-sol',threadIdType:typeof body.client_metadata?.thread_id};
 assert.equal(body.model,'gpt-6-sol');const tid=body.client_metadata?.thread_id;providerStage='thread-identity';assert.equal(typeof tid,'string');modelThreads.add(tid);
 const role=tid===primaryThread?'root':tid,step=roleSteps.get(role)??0;roleSteps.set(role,step+1);
 const tools=flatten([...(body.tools??[]),...(body.input??[]).filter(x=>x.type==='additional_tools').flatMap(x=>x.tools??[])]);
 providerShape.tools=Object.fromEntries(['exec_command','shell_command','apply_patch','spawn_agent'].map(name=>[name,tools.some(tool=>tool.name===name)]));providerShape.step=step;providerShape.role=role==='root'?'root':'descendant';providerStage='model-step';
 if(step===0)return modelCall(tools,`${role}-0`,'exec_command',{cmd:`/usr/bin/python3 -c ${quote(modelProbe(role))}`,workdir:app,yield_time_ms:1000,max_output_tokens:1000});
 if(step===1){assert.ok(JSON.stringify(body.input).includes('native-private-and-metadata-denied'));return modelCall(tools,`${role}-1`,'apply_patch',`*** Begin Patch\n*** Add File: ${join(app,`model-${role.replace(/[^a-z0-9-]/gi,'')}.txt`)}\n+permitted-model-file-tool\n*** End Patch`,true);}
 if(step===2)return modelCall(tools,`${role}-2`,'apply_patch',`*** Begin Patch\n*** Add File: ${join(privateRoot,`forbidden-${role.replace(/[^a-z0-9-]/gi,'')}.txt`)}\n+forbidden\n*** End Patch`,true);
 if(role==='root'&&step===3){const tool=tools.find(t=>t.name==='spawn_agent');assert.ok(tool,'NATIVE_CHILD_TOOL_UNAVAILABLE');const p=tool.parameters?.properties??{};return modelCall(tools,`${role}-3`,'spawn_agent',{message:'Run the controlled inherited confinement probe and finish.',agent_type:'default',...(p.fork_turns?{fork_turns:'none'}:{fork_context:false}),...(p.task_name?{task_name:'native_confined_probe'}:{})});}
 if(role==='root'&&step===4)await waitFor(()=>[...roleSteps].some(([name,count])=>name!=='root'&&count>=4),20000);
 return modelFinal(`${role}-final`);
}
const provider=createHttpServer(async(req,res)=>{try{
 if(req.method==='GET'&&req.url?.startsWith('/v1/models')){res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify({data:[{id:'gpt-6-sol',object:'model',owned_by:'controlled'}]}));return;}
 providerStage='request-method';assert.equal(req.method,'POST');providerStage='request-path';assert.equal(req.url,'/v1/responses');let size=0,chunks=[];for await(const chunk of req){size+=chunk.length;assert.ok(size<4*1024*1024);chunks.push(chunk);}
 const raw=Buffer.concat(chunks),encoding=req.headers['content-encoding'],decode={gzip:gunzipSync,deflate:inflateSync,zstd:zstdDecompressSync}[encoding];assert.ok(!encoding||encoding==='identity'||decode);const bytes=decode?decode(raw,{maxOutputLength:4*1024*1024}):raw;
 assert.ok(!bytes.includes(Buffer.from('private-auth-fixture')));assert.ok(++providerRequests<=40);const event=await modelEvent(JSON.parse(bytes)),id=`l9-native-${providerRequests}`;
 res.writeHead(200,{'content-type':'text/event-stream','cache-control':'no-store'});for(const data of [{type:'response.created',response:{id}},event,{type:'response.completed',response:{id,usage:{input_tokens:1,output_tokens:1,total_tokens:2}}}])res.write(`event: ${data.type}\ndata: ${JSON.stringify(data)}\n\n`);res.end();
 }catch(error){providerFailure=true;report.providerFailureDiagnostic={stage:providerStage,shape:providerShape,assertion:error.operator,knownFailure:['NATIVE_TOOL_UNAVAILABLE','NATIVE_CHILD_TOOL_UNAVAILABLE'].includes(error.message)?error.message:undefined,errorClass:error instanceof SyntaxError?'SyntaxError':error.code==='ERR_ASSERTION'?'Assertion':'Other'};res.writeHead(500);res.end('controlled-provider-failed');}});
function filePin(path){const info=lstatSync(path);return {sha256:hash(readFileSync(path)),identity:{dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs}};}
function folderPin(path){const info=lstatSync(path);return {dev:info.dev,ino:info.ino,uid:info.uid,mode:info.mode,ctime:info.ctimeMs,mtime:info.mtimeMs,birthtime:info.birthtimeMs};}
function manifestFor(appRoot,directory,profileId,session){return {schemaVersion:2,profileId,appRoot,controller:report.controller,election:report.election,generation:{sessionId:session,generation:1},owningRuntime:{args:['app-server','--stdio'],home:nativeHome},binary:{path:binary,version:'0.161.0',sha256:expectedBinary},sandboxState:nativeState(appRoot),configFiles:[join(directory,'worker','config.toml'),join(nativeHome,'config.toml')].map(path=>({path,sha256:hash(readFileSync(path))})),components:{manager:commit,mobile:mobilePin,agents:agentsPin},privatePaths:{auth:[auth],controller:[privateRoot],caddy:[caddySocket,join(privateRoot,'caddy.json')],daemon:[daemon]},keyFile:key,runtimeHome:join(directory,'worker'),environment:{PATH:'/usr/bin:/bin',HOME:join(directory,'worker'),TMPDIR:'/tmp'},commands:{probe:{executable:admittedNode,sha256:hash(readFileSync(admittedNode)),args:[],timeoutMs:20000}}};}
async function callFor(path,args){return await managerCall(['live-policy',...args.slice(0,1),'--manifest',path,...args.slice(1)]);}
async function captureFor(value,path,id){writeFileSync(path,JSON.stringify(value),{mode:0o600});const result=await callFor(path,['capture-runtime','--capture-id',id]);assert.equal(result.code,0,JSON.stringify(managerResultDiagnostic(result)));return JSON.parse(result.stdout);}
async function actualSources(value,path,id,startReply,captured){
 const configuration=await rpc('config/read',{cwd:value.appRoot,includeLayers:true});assert.ok(configuration.config&&Array.isArray(configuration.layers)&&configuration.layers.length);
 report.sourceGuard='hooks-disabled';assert.equal(configuration.config.features.hooks,false);
 report.sourceGuard='mcp-empty';assert.deepEqual(configuration.config.mcp_servers??{},{});
 report.sourceGuard='plugins-empty';assert.deepEqual(configuration.config.plugins??{},{});
 const declared=configuration.config.permissions?.[value.profileId];assert.ok(declared);assert.equal(declared.extends,':read-only');assert.equal(declared.network.enabled,false);
 // Native config/read serializes FilesystemPermissionsToml's unset scan depth
 // as null. Include that known metadata field while requiring every grant exact.
 const expectedFilesystem={glob_scan_max_depth:null,...Object.fromEntries(value.sandboxState.permissionProfile.file_system.entries.map(entry=>[entry.path.type==='special'?':root':entry.path.path,entry.access]))};
 report.sourceGuardDetails={declaredEntryCount:Object.keys(declared.filesystem??{}).length,expectedEntryCount:Object.keys(expectedFilesystem).length,
  hasScanDepth:Object.hasOwn(declared.filesystem??{},'glob_scan_max_depth'),unsetScanDepth:declared.filesystem?.glob_scan_max_depth===null,
  missingEntryCount:Object.keys(expectedFilesystem).filter(key=>!Object.hasOwn(declared.filesystem??{},key)).length,
  unexpectedEntryCount:Object.keys(declared.filesystem??{}).filter(key=>!Object.hasOwn(expectedFilesystem,key)).length,
  changedEntryCount:Object.keys(expectedFilesystem).filter(key=>declared.filesystem?.[key]!==expectedFilesystem[key]).length};
 report.sourceGuard='declared-filesystem-exact';assert.deepEqual(declared.filesystem,expectedFilesystem);assert.equal(startReply.activePermissionProfile?.extends,':read-only');
 assert.equal(startReply.activePermissionProfile?.id,value.profileId);assert.equal(startReply.cwd,value.appRoot);assert.ok(startReply.instructionSources.includes(join(value.appRoot,'AGENTS.md')));
 const pins=new Map(value.configFiles.map(file=>[file.path,file]));let sources=[];const presentFiles=new Set();
 for(const layer of configuration.layers){assert.equal(typeof layer.version,'string');const name=layer.name;assert.ok(name&&typeof name.type==='string');
  if(name.type==='sessionFlags')continue;
  if(name.type==='project'){assert.ok(typeof name.dotCodexFolder==='string');assert.ok(name.dotCodexFolder===join(value.appRoot,'.codex')||value.appRoot.startsWith(dirname(name.dotCodexFolder)+'/'));report.sourceGuard='project-layer-empty';assert.deepEqual(layer.config,{});continue;}
  assert.ok(['user','system','packagedDefaults','legacyManagedConfigTomlFromFile'].includes(name.type),'UNKNOWN_NATIVE_SOURCE');assert.equal(typeof name.file,'string');
  if(!existsSync(name.file)){assert.equal(Object.keys(layer.config).length,0);assert.ok(['/etc/codex/config.toml','/etc/codex/managed_config.toml','/etc/codex/requirements.toml'].includes(name.file),'UNSUPPORTED_ABSENT_SOURCE');continue;}
  assert.ok(pins.has(name.file),'UNINVENTORIED_NATIVE_FILE');presentFiles.add(name.file);sources.push({kind:'file',role:name.type==='user'?'user':name.type==='system'?'system':name.type==='packagedDefaults'?'packaged-defaults':'managed',...pins.get(name.file)});
 }
 assert.ok(presentFiles.has(join(nativeHome,'config.toml')));sources.push({kind:'file',role:'sandbox-worker',...pins.get(join(value.runtimeHome,'config.toml'))});
 const before=JSON.parse(readFileSync(join(dirname(path),`birth-${id}.json`))).body;
 for(const entry of before.directories)sources.push(existsSync(entry.path)?{kind:'directory',role:'project',path:entry.path,identity:folderPin(entry.path)}:{kind:'absent',role:'project',path:entry.path});
 for(const file of ['/etc/codex/config.toml','/etc/codex/managed_config.toml','/etc/codex/requirements.toml'])sources.push({kind:'absent',role:'system',path:file});
 for(const file of startReply.instructionSources)sources.push({kind:'instruction',path:file,...filePin(file)});
 const flags=Object.fromEntries(['model','modelProvider','cwd','approvalPolicy','activePermissionProfile','sandbox','reasoningEffort','runtimeWorkspaceRoots','instructionSources'].filter(key=>Object.hasOwn(startReply,key)).map(key=>[key,startReply[key]]));
 const inventory={schemaVersion:2,runtime:identity(runtime.child.pid),policyDigest:captured.policyDigest,effectiveConfigDigest:hash(stable(configuration.config)),sessionFlagsDigest:hash(stable(flags)),sources};
 const inventoryFile=join(dirname(path),`sources-${id}.json`);writeFileSync(inventoryFile,JSON.stringify(inventory),{mode:0o600});const seal=await callFor(path,['seal-runtime','--capture-id',id,'--runtime-pid',String(runtime.child.pid),'--sources',inventoryFile]);assert.equal(seal.code,0,JSON.stringify(managerResultDiagnostic(seal)));
 value.runtimeProvenance={captureId:id,effectiveConfigDigest:inventory.effectiveConfigDigest,sessionFlagsDigest:inventory.sessionFlagsDigest};writeFileSync(path,JSON.stringify(value),{mode:0o600});
 return {configuration,inventory,startReply,inventoryDigest:hash(stable(inventory))};
}
function originalRuntimeDescendant(pid){for(let depth=0;depth<16;depth++){if(pid===runtime.child.pid)return identity(pid).birth===report.appServer.birth;const match=readFileSync(`/proc/${pid}/status`,'utf8').match(/^PPid:\s+(\d+)/m);if(!match)return false;pid=Number(match[1]);if(pid<=1)return false;}return false;}
async function startupAt(value){let sampling=false;const sightings=[],masks=[],metadata=[];
 const protectedPaths=[];for(let path=value.appRoot;;path=dirname(path)){protectedPaths.push(join(path,'.codex'));if(path==='/')break;}
 const observed=new Set(),maskOps=new Set(['--tmpfs','--remount-ro','--ro-bind-data','--bind-data','--ro-bind','--bind','--dir','--file']);
 const sample=()=>{if(sampling)return;sampling=true;try{
  for(const [ancestor,path] of protectedPaths.entries()){try{const info=lstatSync(path),snapshot={ancestor,kind:info.isDirectory()?'directory':info.isSymbolicLink()?'symlink':'file',identity:folderPin(path)};const key=JSON.stringify(snapshot);if(!observed.has(key)&&metadata.length<64){observed.add(key);metadata.push(snapshot);}}catch{}}
  for(const pid of readdirSync('/proc').filter(x=>/^\d+$/.test(x))){try{const args=readFileSync(`/proc/${pid}/cmdline`).toString().split('\0');
   const fsHelper=args.includes('--codex-run-as-fs-helper')&&readlinkSync(`/proc/${pid}/exe`)===binary;
   const hasMasks=args.some(arg=>protectedPaths.includes(arg))&&args.some(arg=>maskOps.has(arg));if(!fsHelper&&!hasMasks)continue;
   if(!originalRuntimeDescendant(Number(pid)))continue;const processIdentity=identity(Number(pid));
   if(fsHelper&&!sightings.some(x=>x.pid===processIdentity.pid&&x.birth===processIdentity.birth)&&sightings.length<64)sightings.push(processIdentity);
   if(hasMasks&&!masks.some(x=>x.process.pid===processIdentity.pid&&x.process.birth===processIdentity.birth)&&masks.length<8&&hash(readFileSync(`/proc/${pid}/exe`))===provenance.bundledBwrap.sha256){
    const selected=[];for(const [index,arg] of args.entries()){const ancestor=protectedPaths.indexOf(arg);if(ancestor<0)continue;let operation;for(let n=index-1;n>=Math.max(0,index-3);n--)if(maskOps.has(args[n])){operation=args[n];break;}if(operation)selected.push({argumentIndex:index,operation,ancestor});}
    masks.push({process:processIdentity,executable:'pinned-bubblewrap',metadataMasks:selected});
   }
  }catch{}}
 }finally{sampling=false;}};
 sample();const timer=setInterval(sample,1);let reply;try{reply=await rpc('thread/start',{cwd:value.appRoot,permissions:value.profileId,approvalPolicy:'never',model:'gpt-6-sol',modelProvider:'controlled'});}finally{clearInterval(timer);sample();report.startupFilesystemHelpers=(report.startupFilesystemHelpers??[]).concat(sightings);report.startupMetadataObservations=(report.startupMetadataObservations??[]).concat(metadata);report.startupMasks=(report.startupMasks??[]).concat(masks);}
 assert.equal(typeof reply.thread?.id,'string');return reply;}

try {
 assert.equal(hash(readFileSync(manager)),provenance.files['public/gestalt']);assert.equal(provenance.binarySha256,expectedBinary);report.checks.managerInstalledBytes=true;report.checks.binaryPinned=true;
 const {electLiveController}=await import(pathToFileURL(electionModule).href);elected=await electLiveController({privateDirectory:privateRoot,controllerId:'l9-proof',initialize:true});
 const holderPid=Number(readFileSync(`/proc/${process.pid}/task/${process.pid}/children`,'utf8').trim().split(/\s+/).find(pid=>{try{return readFileSync(`/proc/${pid}/cmdline`,'utf8').includes('elected');}catch{return false;}}));assert.ok(holderPid);report.election={holder:identity(holderPid),lockFile:join(privateRoot,'controller.lock')};report.checks.existingMobileElection=true;
 await new Promise(done=>server.listen(socket,done));const caddyConfig=join(privateRoot,'caddy.json');writeFileSync(caddyConfig,JSON.stringify({admin:{listen:'unix/'+caddySocket},apps:{http:{servers:{fixture:{listen:['127.0.0.1:0'],routes:[{handle:[{handler:'static_response',body:'fixture'}]}]}}}}}),{mode:0o600});
 caddy=spawn(caddyBinary,['run','--config',caddyConfig],{env:controllerEnv,stdio:'ignore'});await waitFor(()=>existsSync(caddySocket));assert.equal(await api(),200);report.checks.actualCaddyHostAuthority=true;
 await new Promise(done=>provider.listen(0,'127.0.0.1',done));const providerPort=provider.address().port;
 const allApps=[app,...cases.map(c=>c.app)];const owningConfig=`model = "gpt-6-sol"\nmodel_provider = "controlled"\nmodel_reasoning_effort = "medium"\napproval_policy = "never"\ndefault_permissions = "${profile}"\nmcp_servers = {}\nplugins = {}\n[shell_environment_policy]\ninherit = "none"\n[model_providers.controlled]\nname = "Controlled Native proof"\nbase_url = "http://127.0.0.1:${providerPort}/v1"\nwire_api = "responses"\nrequires_openai_auth = false\n[features]\nshell_snapshot = false\nshell_snapshot_v2 = false\ndaemon_auto_start = false\ncode_mode = false\nmulti_agent = true\nmulti_agent_v2 = true\nhooks = false\n${allApps.map(path=>`[projects.${JSON.stringify(path)}]\ntrust_level = "trusted"\n`).join('')}${nativeProfile(profile,app)}${nativeProfile(narrowProfile,app,false)}${cases.map(c=>nativeProfile(c.profile,c.app)).join('')}`;writeFileSync(join(nativeHome,'config.toml'),owningConfig,{mode:0o600});
 const worker=join(privateRoot,'worker');mkdirSync(worker,{mode:0o700});writeFileSync(join(worker,'config.toml'),isolatedConfig,{mode:0o600});m=manifestFor(app,privateRoot,profile,sessionId);save();
 report.stage='managed-native-bootstrap';const bootstrap=await managerCall(['cli','--','--live-policy-bootstrap','--manifest',manifest]);report.bootstrap=managerResultDiagnostic(bootstrap);assert.equal(bootstrap.code,0);assert.equal(lstatSync(daemon).mode&0o777,0o700);report.checks.stockNativeHostBootstrap=true;
 report.stage='absence-before-owned-birth';const captured=await captureFor(m,manifest,captureId);for(const c of cases){c.manifest=join(c.directory,'manifest.json');c.value=manifestFor(c.app,c.directory,c.profile,c.session);c.captured=await captureFor(c.value,c.manifest,c.capture);}
 report.checks.absentBeforeOriginalBirth=true;
 const {launchCodexAppServer}=await import(pathToFileURL(mobileModule).href);runtime=launchCodexAppServer({profile:'default',cwd:app,environment:controllerEnv});runtime.child.stderr.on('data',()=>{});
 // The accepted client intentionally truncates error messages to 256 bytes.
 // Classify the original error frame before that boundary; retain no frame/text.
 let wireBuffer='';runtime.child.stdout.on('data',chunk=>{wireBuffer+=chunk.toString();if(Buffer.byteLength(wireBuffer)>1024*1024){wireBuffer='';report.wireDiagnosticOverflow=true;return;}
  let newline;while((newline=wireBuffer.indexOf('\n'))>=0){const line=wireBuffer.slice(0,newline);wireBuffer=wireBuffer.slice(newline+1);try{const packet=JSON.parse(line);
   if(typeof packet.id==='number'&&typeof packet.error?.code==='number'&&typeof packet.error?.message==='string')report.nativeWireFailure={id:packet.id,...rpcFailureDiagnostic(currentRpcMethod,packet.error,{appRoot:app,originalHome:nativeHome,controllerDirectory:privateRoot,metadata:join(app,'.codex'),daemon})};
  }catch{}}});
 await rpc('initialize',{clientInfo:{name:'l9_actual_provenance_proof',version:'2'},capabilities:{experimentalApi:true}});runtime.child.stdin.write(JSON.stringify({method:'initialized',params:{}})+'\n');
 report.appServer=identity(runtime.child.pid);assert.equal(readlinkSync(`/proc/${runtime.child.pid}/exe`),binary);assert.equal(hash(readFileSync(`/proc/${runtime.child.pid}/exe`)),expectedBinary);report.checks.originalOwningNativeIdentity=true;
 const hostRead=await rpc('fs/readFile',{path:auth});assert.equal(hostRead.dataBase64,Buffer.from('private-auth-fixture').toString('base64'));report.checks.controllerFsAuthority=true;
 report.stage='actual-original-startup';const initial=await startupAt(m);thread=initial.thread.id;primaryThread=thread;const observed=await actualSources(m,manifest,captureId,initial,captured);
 assert.ok(existsSync(join(app,'.codex')),'NATIVE_STARTUP_DID_NOT_CREATE_METADATA');assert.equal(readdirSync(join(app,'.codex')).length,0);assert.ok(initial.instructionSources.includes(join(app,'AGENTS.md')));report.checks.actualNativeStartupMetadata=true;report.checks.projectInstructionsLoaded=true;report.appliedFeatures={shellSnapshot:observed.configuration.config.features.shell_snapshot,shellSnapshotV2:observed.configuration.config.features.shell_snapshot_v2,hooks:observed.configuration.config.features.hooks};
 report.sources={nativeLayerKinds:observed.configuration.layers.map(layer=>layer.name.type),directoryCount:observed.inventory.sources.filter(source=>source.kind==='directory').length,instructionCount:initial.instructionSources.length,inventoryDigest:observed.inventoryDigest,effectiveConfigDigest:observed.inventory.effectiveConfigDigest,sessionFlagsDigest:observed.inventory.sessionFlagsDigest};
 report.stage='native-model-and-child-tools';await prepare();let completed=false;const off=runtime.rpc.onNotification(message=>{if(message.method==='turn/completed'&&message.params?.threadId===thread)completed=true;});
 await rpc('turn/start',{threadId:thread,input:[{type:'text',text:'Run the controlled native confinement probe.'}]});await waitFor(()=>completed||providerFailure,90000);off();assert.equal(providerFailure,undefined);assert.ok(modelThreads.size>=2);assert.ok(roleSteps.get('root')>=5);
 assert.ok(existsSync(join(app,'model-root.txt')));assert.equal(readdirSync(privateRoot).some(name=>name.startsWith('forbidden-')),false);report.checks.actualNativeModelFileTools=true;report.checks.actualNativeChildAgentInheritance=true;report.model={provider:'controlled',requests:providerRequests,threads:modelThreads.size};
 report.stage='native-helper-and-grandchildren';extraFD=openSync(auth,'r');unowned=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{env:controllerEnv,stdio:'ignore'});
 const first=await launch();await nativeMessages(first);
 await waitFor(()=>existsSync(first.publicSocket));publicSocket=first.publicSocket;
 publicIngress=createHttpServer((req,res)=>{if(req.method!=='GET'||req.url!=='/'){res.writeHead(404);res.end();return;}
  const upstream=httpRequest({socketPath:publicSocket,path:'/',method:'GET'},incoming=>{res.writeHead(incoming.statusCode,{'content-type':'text/plain'});incoming.pipe(res);});upstream.on('error',()=>{res.writeHead(503);res.end();});upstream.end();});
 await new Promise(done=>publicIngress.listen(0,'127.0.0.1',done));const ingressPort=publicIngress.address().port;
 const appBody=await new Promise((done,reject)=>{const request=httpRequest({hostname:'127.0.0.1',port:ingressPort,path:'/'},incoming=>{let text='';incoming.on('data',chunk=>text+=chunk);incoming.on('end',()=>done(text));});request.on('error',reject);request.end();});
 assert.equal(appBody,'public-fixture');report.checks.actualConfinedDevserverAndRegisteredLoopbackIngress=true;
 report.devserver={nativeTransport:'public Unix HTTP socket',registeredIngress:'loopback TCP',privateDenied:true};assert.ok(first.messages().every(frame=>Object.values(frame.checks).every(Boolean)));report.nativeResults=first.messages();report.checks.helperAndGrandchildConfinement=true;
 assert.equal((await managerCall(['live-policy','verify-process','--manifest',manifest,'--launch-id',first.id])).code,0);
 report.stage='permission-narrowing';m.sandboxState=nativeState(app,false);save();await stopped(first);await waitFor(()=>ownedPids(childFile).every(dead));assert.equal(JSON.parse(readFileSync(first.path)).body.termination,'policy-changed');
 assert.equal((await managerCall(['live-policy','prepare','--manifest',manifest])).code,1);report.checks.narrowingRevokesAndRefusesStaleAdmission=true;
 const narrowed=await rpc('command/exec',{cwd:app,permissionProfile:narrowProfile,command:['/usr/bin/python3','-c',`import pathlib\np=pathlib.Path(${JSON.stringify(join(app,'narrow-forbidden'))})\ntry:p.write_text('forbidden');raise Exception('WRITE_BREACH')\nexcept PermissionError:pass\ntry:open(${JSON.stringify(auth)}).read();raise Exception('PRIVATE_BREACH')\nexcept (PermissionError,FileNotFoundError):pass\nprint('narrowed-native-denied')`],timeoutMs:8000,outputBytesCap:1000});assert.equal(narrowed.exitCode,0);assert.ok(narrowed.stdout.includes('narrowed-native-denied'));report.checks.actualNarrowedNativePolicy=true;m.sandboxState=nativeState(app);save();
 const eof=await launch();await nativeMessages(eof);eof.child.kill('SIGKILL');await stopped(eof);await waitFor(()=>ownedPids(childFile).every(dead));report.checks.callerDeathReaps=true;
 const leader=await launch('leader-exit');await stopped(leader);await waitFor(()=>ownedPids(childFile).length===0);report.checks.detachedGrandchildReaped=true;
 process.kill(unowned.pid,0);assert.equal(await api(),200);assert.equal((await rpc('fs/readFile',{path:auth})).dataBase64,hostRead.dataBase64);report.checks.ownedOnlyCleanupAndControllerAuthority=true;
 report.stage='actual-provenance-negative-matrix';report.negative={};
 for(const c of cases){const started=await startupAt(c.value);await actualSources(c.value,c.manifest,c.capture,started,c.captured);assert.equal((await callFor(c.manifest,['prepare'])).code,0);const metadata=join(c.app,'.codex');assert.ok(existsSync(metadata));
  if(['config','hooks','rules','agents'].includes(c.kind)){const name={config:'config.toml',hooks:'hooks.json',rules:'rules',agents:'agents'}[c.kind];writeFileSync(join(metadata,name),'injected-native-source');}
  if(c.kind==='replacement'){renameSync(metadata,metadata+'-original');mkdirSync(metadata,{mode:0o755});}
  if(c.kind==='symlink'){renameSync(metadata,metadata+'-original');symlinkSync(privateRoot,metadata);}
  if(c.kind==='stale-owner'){const receiptPath=join(c.directory,`runtime-${c.capture}.json`),r=JSON.parse(readFileSync(receiptPath));r.body.runtime.process.birth='0';writeFileSync(receiptPath,JSON.stringify(r),{mode:0o600});}
  if(c.kind==='generation')c.value.generation.generation++;
  if(c.kind==='session')c.value.generation.sessionId=randomUUID();
  if(c.kind==='race'){const racer=spawn(process.execPath,['-e',`const fs=require('node:fs'),p=${JSON.stringify(metadata)};for(let i=0;i<8;i++){fs.renameSync(p,p+'-race-'+i);fs.mkdirSync(p,{mode:0o755});}`],{stdio:'ignore',env:controllerEnv});await new Promise(done=>racer.once('close',done));assert.equal(racer.exitCode,0);}
  writeFileSync(c.manifest,JSON.stringify(c.value),{mode:0o600});const result=await callFor(c.manifest,['verify']);assert.equal(result.code,1);const reason=JSON.parse(result.stdout).reason;assert.ok(['metadata-content-present','metadata-changed','symlink-path','tampered-receipt','revoked-runtime-provenance'].includes(reason),reason);report.negative[c.kind]={passed:true,reason};
 }
 report.checks.nativeBornInjectionReplacementLifetimeSessionMatrix=true;
 report.stage='actual-owning-runtime-loss';const orphan=await launch();await nativeMessages(orphan);runtime.child.kill('SIGKILL');await stopped(orphan);await waitFor(()=>ownedPids(childFile).every(dead));assert.equal((await managerCall(['live-policy','verify','--manifest',manifest])).code,1);report.checks.owningRuntimeLossRevokesWorkers=true;
 report.stage='complete';report.passed=true;
} catch(error){report.failureMetadata=metadataPaths.map((path,index)=>{try{const info=lstatSync(path);return {ancestor:index,kind:info.isDirectory()?'directory':info.isSymbolicLink()?'symlink':'file',identity:folderPin(path),empty:info.isDirectory()?readdirSync(path).length===0:undefined};}catch{return {ancestor:index,kind:'absent'};}});report.failureDetail={stage:report.stage,assertion:error.operator,actual:typeof error.actual==='number'||typeof error.actual==='boolean'?error.actual:undefined,expected:typeof error.expected==='number'||typeof error.expected==='boolean'?error.expected:undefined};report.failure=error.code??(/^[A-Z_]+$/.test(error.message)?error.message:'ASSERTION_FAILED');if(active){report.nativeDiagnostic=active.diagnostic();report.launchBoundary=launchBoundary(active);report.ownedProcessSnapshot=ownedProcessSnapshot(active);}process.exitCode=1;
} finally {
 if(active){try{await managerCall(['live-policy','stop','--manifest',manifest,'--launch-id',active.id]);await stopped(active);}catch{report.passed=false;process.exitCode=1;await closeOwned(active.child);}}
 if(extraFD!==undefined)closeSync(extraFD);try{await closeOwned(runtime?.child);await closeOwned(caddy);await closeOwned(unowned);await elected?.close();report.checks.ownedShutdown=true;}catch{report.passed=false;report.failure='OWNED_SHUTDOWN_FAILED';process.exitCode=1;}
 if(publicIngress?.listening)await new Promise(done=>publicIngress.close(done));
 if(provider.listening)await new Promise(done=>provider.close(done));if(server.listening)await new Promise(done=>server.close(done));
 report.metadataPreserved=true;report.fixtureRootRetained=true;report.providerRequests=providerRequests;writeFileSync(join(evidence,'native-proof.json'),JSON.stringify(report,null,2)+'\n');
 const summary={stage:report.stage,passed:report.passed,checks:report.checks,failure:report.failure,failureDetail:report.failureDetail,sourceGuard:report.sourceGuard,sourceGuardDetails:report.sourceGuardDetails,providerFailureDiagnostic:report.providerFailureDiagnostic,failedRpc:report.failedRpc,nativeWireFailure:report.nativeWireFailure,failureMetadata:report.failureMetadata,startupMetadataObservations:report.startupMetadataObservations,startupMasks:report.startupMasks,bootstrap:report.bootstrap};
 console.log(JSON.stringify(summary));
 if(!report.passed&&hostedActions)console.log('::error title=Bounded Native proof failure::'+JSON.stringify(summary).replaceAll('%','%25').replaceAll('\r','%0D').replaceAll('\n','%0A'));
}
