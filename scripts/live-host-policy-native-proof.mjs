// Isolated hosted proof. No model calls, credentials, deployment or IPC repair.
import assert from 'node:assert/strict';
import {spawn,execFileSync} from 'node:child_process';
import {createHash,randomUUID,randomBytes} from 'node:crypto';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,readlinkSync,realpathSync,lstatSync,readdirSync,openSync,closeSync,rmSync,copyFileSync,chmodSync,existsSync} from 'node:fs';
import {createServer,createConnection} from 'node:net';
import {request as httpRequest} from 'node:http';
import {dirname,join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';

const binary=realpathSync(process.env.LIVE_HOST_CODEX_BINARY);
const mobileModule=realpathSync(process.env.LIVE_HOST_MOBILE_MODULE);
const caddyBinary=realpathSync(process.env.LIVE_HOST_CADDY_BINARY);
const evidence=resolve(process.env.LIVE_HOST_POLICY_EVIDENCE ?? 'test-results/live-host-policy');
const manager=realpathSync(process.env.LIVE_HOST_MANAGER_BINARY);
const expectedBinary='9a820c17865fa825d04db416818679a9d63bd72e50835c396f496e5684626c9c';
const mobilePin='2facdae77f2b807798aa02a5d63a12d9161aebc6';
const acceptedMobilePin='c558dd644e4b6ac8d3bce9e57bce762791ea25de';
const mobileInputPaths=['src/server/platform/codex/json-rpc-client.ts','src/server/platform/catalog/profile-command.ts','src/server/platform/codex/codex-process-launcher.ts'];
const agentsPin='4c4f4566ac968e1b7d425e825ba2ad9a8d9f8f1f';
const hash=data=>createHash('sha256').update(data).digest('hex');
const pause=ms=>new Promise(done=>setTimeout(done,ms));
// Match pinned native legacy resource resolution without relocating its binary.
const bwrapCandidates=[join(dirname(binary),'codex-resources','bwrap'),join(dirname(dirname(binary)),'codex-resources','bwrap'),join(dirname(binary),'bwrap')];
const bundledBwrap=bwrapCandidates.find(path=>existsSync(path)&&(lstatSync(path).mode&0o111));
assert.ok(bundledBwrap,'PINNED_NATIVE_PACKAGE_RESOURCE_MISSING');
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
 sourcePin:'979011409de0a60b52f179721948e65531d26144',mobilePin,acceptedMobilePin,agentsPin,nativeVersion:'0.161.0',
 mobileInputs:Object.fromEntries(mobileInputPaths.map(path=>[path,hash(readFileSync(join('.host-mobile-input',path)))])),
 bundledBwrap:{sha256:hash(readFileSync(bundledBwrap)),mode:(lstatSync(bundledBwrap).mode&0o777).toString(8)},
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
const key=join(privateRoot,'signing-key');writeFileSync(key,randomBytes(32),{mode:0o600});
const publicFile=join(app,'public.txt'),readOnlyFile=join(app,'read-only.txt');writeFileSync(publicFile,'public-fixture');writeFileSync(readOnlyFile,'preserved-read-only');
const isolatedConfig='[shell_environment_policy]\ninherit = "none"\n';
writeFileSync(join(home,'config.toml'),isolatedConfig,{mode:0o600});
const profile='live-host-policy-proof';
const owningConfig=`approval_policy = "never"\ndefault_permissions = "${profile}"\nmcp_servers = {}\nplugins = {}\n\n[features]\ndaemon_auto_start = false\nmulti_agent = false\nmulti_agent_v2 = false\nhooks = false\n\n[permissions.${profile}]\nextends = ":workspace"\n\n[permissions.${profile}.filesystem]\n":root" = "read"\n${JSON.stringify(app)} = "write"\n${JSON.stringify(readOnlyFile)} = "read"\n${JSON.stringify(privateRoot)} = "deny"\n\n[permissions.${profile}.network]\nenabled = false\n`;
writeFileSync(join(nativeHome,'config.toml'),owningConfig,{mode:0o600});
// Actual Mobile launcher resolves codex from its trusted PATH. This wrapper only
// execs the checksum-pinned native executable; it is not a second app-server.
writeFileSync(join(bin,'codex'),`#!/bin/sh\nexec '${binary}' "$@"\n`,{mode:0o700});
const controllerEnv={PATH:`${bin}:${dirname(process.execPath)}:/usr/bin:/bin`,HOME:nativeHome,CODEX_HOME:nativeHome,TMPDIR:'/tmp',LANG:'C.UTF-8'};
const managerEnv={...controllerEnv,HOME:privateRoot};
for(const name of Object.keys(process.env))delete process.env[name];Object.assign(process.env,controllerEnv);
const identity=pid=>{
 const raw=readFileSync(`/proc/${pid}/stat`,'utf8');
 return {pid,birth:raw.slice(raw.lastIndexOf(')')+2).split(' ')[19],uid:Number(readFileSync(`/proc/${pid}/status`,'utf8').match(/^Uid:\s+(\d+)/m)[1]),
 namespaces:Object.fromEntries(['mnt','user','pid'].map(name=>[name,readlinkSync(`/proc/${pid}/ns/${name}`)]))};
};
const report={schemaVersion:1,provenance,stage:'bootstrap',passed:false,checks:{},controller:identity(process.pid),
 limitations:['No provider/model credentials or native model-file-tool proof.','No production MCP admission or Live picker flow; L10/L11 remain separate gates.']};
let runtime,caddy,unowned,active,extraFD,controllerAccepted=0;
const server=createServer(peer=>{controllerAccepted++;peer.end();});
async function waitFor(fn,budget=12000){const end=Date.now()+budget;while(Date.now()<end){try{const result=fn();if(result)return result;}catch{}await pause(50);}throw Error('PROOF_DEADLINE');}
function managerCall(args){const result=spawn('bash',[manager,...args],{cwd:app,env:managerEnv,stdio:['ignore','pipe','pipe']});
 let stdout='',stderr='';result.stdout.on('data',b=>stdout+=b);result.stderr.on('data',b=>stderr+=b);
 return new Promise((done,reject)=>{const timer=setTimeout(()=>{result.kill('SIGKILL');reject(Error('MANAGER_DEADLINE'));},15000);result.on('error',error=>{clearTimeout(timer);done({code:null,signal:null,stdout,stderr,errorCode:['ENOENT','EACCES','EPERM'].includes(error.code)?error.code:'SPAWN_ERROR'});});result.on('close',(code,signal)=>{clearTimeout(timer);done({code,signal,stdout,stderr});});});}
const api=()=>new Promise((done,reject)=>{const req=httpRequest({socketPath:caddySocket,path:'/config/',method:'GET'},res=>{res.resume();res.on('end',()=>done(res.statusCode));});req.on('error',reject);req.end();});
async function rpc(method,params){return await Promise.race([runtime.rpc.request(method,params),new Promise((_,reject)=>{const timer=setTimeout(()=>reject(Error('RPC_DEADLINE')),12000);timer.unref();})]);}
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
 stage('controller-socket');checks.controllerSocket=await socketDenied(input.controllerSocket);stage('caddy-socket');checks.caddySocket=await socketDenied(input.caddySocket);stage('daemon');
 try{checks.daemonMask=(fs.lstatSync(input.daemon).mode&0o777)===0;}catch{checks.daemonMask=false;}
 try{fs.readdirSync(input.daemon);checks.daemon=false;}catch(e){checks.daemon=['EACCES','EPERM','ENOENT'].includes(e.code);}
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
 const id=randomUUID(),input={mode,stdinExpected:stdinChunks.join(''),publicFile,readOnlyFile,writeFile:join(app,'written'),privateFiles:[auth,controlFile,manifest,key,join(nativeHome,'config.toml')],
 controllerSocket:socket,caddySocket,daemon,fdIdentity:{dev:lstatSync(auth).dev,ino:lstatSync(auth).ino},aliases:[`/proc/${process.pid}/root${auth}`,`/proc/${process.pid}/fd/${extraFD}`,`/proc/self/root${auth}`]};
 m.commands.probe.args=[childFile,JSON.stringify(input)];await prepare();
 const child=spawn('bash',[manager,'live-policy','run','--manifest',manifest,'--command','probe','--launch-id',id],
 {cwd:app,env:{...managerEnv,OPENAI_API_KEY:'fixture-secret',GESTALT_MOBILE_ORG_PLAN_MEASUREMENT_TOKEN:'fixture-secret'},stdio:['pipe','pipe','pipe','ignore','ignore','ignore','ignore',extraFD]});
 void(async()=>{await waitFor(()=>output.includes('project-input'));for(const chunk of stdinChunks){child.stdin.write(chunk);await pause(50);}child.stdin.end();})().catch(()=>{report.projectInputFeedFailed=true;child.stdin.destroy();});child.stdin.on('error',()=>{report.projectInputFeedFailed=true;});let output='',stderr='';child.stdout.on('data',b=>output+=b);child.stderr.on('data',b=>stderr+=b);
 let closed=false,exitCode,exitSignal;const ended=new Promise(done=>child.on('close',(code,signal)=>{closed=true;exitCode=code;exitSignal=signal;done(code);}));
 const result={id,child,ended,path:join(privateRoot,`process-${id}.json`),messages:()=>output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line)).filter(value=>value.kind==='probe-result'),
 state:()=>({closed,code:exitCode??null,signal:exitSignal??null,stdoutBytes:Buffer.byteLength(output),stderrBytes:Buffer.byteLength(stderr)}),diagnostic:()=>nativeDiagnostic(stderr,{policyDirectory:privateRoot,authentication:auth,controllerFile:controlFile,controllerSocket:socket,caddySocket,caddyConfig:join(privateRoot,'caddy.json'),manifest,key,runtimeHome:home,daemon})};
 child.stdout.on('data',()=>{try{report.probeStages=output.split('\n').slice(0,-1).filter(Boolean).map(line=>JSON.parse(line)).filter(value=>value.kind==='probe-stage').slice(0,40).map(value=>({depth:value.depth,stage:value.stage,pid:value.pid}));report.nativeResults=result.messages().filter(value=>Number.isInteger(value.depth)&&value.depth>=0&&value.depth<=2).map(value=>({depth:value.depth,uid:value.uid,pid:value.pid,checks:Object.fromEntries(Object.entries(value.checks??{}).filter(([,flag])=>typeof flag==='boolean'))}));}catch{report.nativeOutputMalformed=true;}});
 active=result;return result;
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
try {
 assert.equal(hash(readFileSync(manager)),provenance.files['public/gestalt']);report.checks.managerInstalledBytes=true;
 assert.equal(provenance.binarySha256,expectedBinary);report.checks.binaryPinned=true;
 await new Promise((done,reject)=>{server.once('error',reject);server.listen(socket,done);});
 const caddyConfig=join(privateRoot,'caddy.json');writeFileSync(caddyConfig,JSON.stringify({admin:{listen:'unix/'+caddySocket},apps:{http:{servers:{fixture:{listen:['127.0.0.1:0'],routes:[{handle:[{handler:'static_response',body:'fixture'}]}]}}}}}),{mode:0o600});
 caddy=spawn(caddyBinary,['run','--config',caddyConfig],{env:controllerEnv,stdio:'ignore'});await waitFor(()=>lstatSync(caddySocket).isSocket());assert.equal(await api(),200);
 report.checks.actualCaddyHostAuthority=true;report.caddy={binarySha256:hash(readFileSync(caddyBinary)),version:execFileSync(caddyBinary,['version'],{env:controllerEnv,encoding:'utf8'}).trim()};
 const {launchCodexAppServer}=await import(pathToFileURL(mobileModule).href);runtime=launchCodexAppServer({profile:'default',cwd:app,environment:controllerEnv});runtime.child.stderr.on('data',()=>{});
 await rpc('initialize',{clientInfo:{name:'l9_host_policy_proof',version:'1'},capabilities:{experimentalApi:true}});
 runtime.child.stdin.write(JSON.stringify({method:'initialized',params:{}})+'\n');
 report.appServer=identity(runtime.child.pid);assert.equal(hash(readFileSync(`/proc/${runtime.child.pid}/exe`)),expectedBinary);
 assert.equal(report.appServer.namespaces.mnt,report.controller.namespaces.mnt);report.checks.actualMobilePrivateStdio=true;
 const hostRead=await rpc('fs/readFile',{path:auth});assert.equal(hostRead.dataBase64,Buffer.from('private-auth-fixture').toString('base64'));report.checks.controllerFsAuthority=true;
 const profiles=await rpc('permissionProfile/list',{cwd:app});assert.ok(profiles.data.some(value=>value.id===profile&&value.allowed));report.checks.namedProfileAllowed=true;
 report.nativeAliasPrerequisite={runtimeHomeOutsideTemporaryRoot:!nativeHome.startsWith('/tmp/'),runtimeHomeMode:(lstatSync(nativeHome).mode&0o777).toString(8),helperAliasesCreated:existsSync(join(nativeHome,'tmp','arg0'))};assert.equal(report.nativeAliasPrerequisite.runtimeHomeOutsideTemporaryRoot,true);assert.equal(report.nativeAliasPrerequisite.helperAliasesCreated,true);
 report.stage='native-bootstrap';
 try{report.daemonBefore={mode:(lstatSync(daemon).mode&0o777).toString(8)};}catch(error){report.daemonBefore={reason:error.code};}
 const bootstrap=await rpc('command/exec',{command:['/usr/bin/true'],cwd:app,permissionProfile:profile,timeoutMs:12000,outputBytesCap:1024});
 report.bootstrapExitCode=bootstrap.exitCode;
 if(bootstrap.exitCode!==0){report.bootstrapDiagnostic=nativeDiagnostic(String(bootstrap.stderr??''));throw Error('NATIVE_BOOTSTRAP_REFUSED');}
 assert.equal(lstatSync(daemon).mode&0o777,0o700);report.checks.stockNativeHostBootstrap=true;
 report.stage='native-node-baseline';const baseline=await rpc('command/exec',{command:[admittedNode,'-e',"process.stdout.write('native-node-startup\\n')"],cwd:app,permissionProfile:profile,timeoutMs:3000,outputBytesCap:512});
 report.nativeNodeBaseline={exitCode:baseline.exitCode,startup:baseline.stdout==='native-node-startup\n',diagnostic:nativeDiagnostic(String(baseline.stderr??''))};
 assert.equal(report.nativeNodeBaseline.exitCode,0);assert.equal(report.nativeNodeBaseline.startup,true);
 report.nodeInterpreter={sha256:hash(readFileSync(admittedNode)),sourceSha256:hash(readFileSync(process.execPath)),sourceMode:(lstatSync(realpathSync(process.execPath)).mode&0o777).toString(8),sourcePathCanonical:realpathSync(process.execPath)===process.execPath,version:process.version};
 assert.equal(report.nodeInterpreter.sha256,report.nodeInterpreter.sourceSha256);
 extraFD=openSync(auth,'r');
 unowned=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{env:controllerEnv,stdio:'ignore'});
 m={schemaVersion:1,profileId:profile,appRoot:app,controller:report.controller,binary:{path:binary,version:'0.161.0',sha256:expectedBinary},
 sandboxState:{permissionProfile:{type:'managed',file_system:{type:'restricted',entries:[{path:{type:'special',value:{kind:'root'}},access:'read'},{path:{type:'path',path:app},access:'write'},{path:{type:'path',path:privateRoot},access:'deny'},{path:{type:'path',path:readOnlyFile},access:'read'}]},network:'restricted'},sandboxCwd:pathToFileURL(app).href,useLegacyLandlock:false},
 configFiles:[join(home,'config.toml'),join(nativeHome,'config.toml')].map(path=>({path,sha256:hash(readFileSync(path))})),components:{manager:commit,mobile:mobilePin,agents:agentsPin},
 privatePaths:{auth:[auth],controller:[privateRoot,socket],caddy:[caddySocket,caddyConfig],daemon:[daemon]},keyFile:key,runtimeHome:home,
 environment:{PATH:'/usr/bin:/bin',HOME:home,TMPDIR:'/tmp'},commands:{probe:{executable:admittedNode,sha256:hash(readFileSync(admittedNode)),args:[],timeoutMs:20000}}};
 report.stage='native-descendants';
 const first=await launch();report.stage='native-descendants/messages';
 await nativeMessages(first);
 const messages=first.messages();report.nativeResults=messages;
 const roundtrip=messages.find(value=>value.depth===0).inputRoundtrip;assert.equal(roundtrip.matches,true);assert.equal(roundtrip.eof,true);assert.equal(roundtrip.fifo,true);assert.ok(roundtrip.chunks>=2);assert.equal(roundtrip.bytes,Buffer.byteLength('native input with spaces\nsecond line\n'));report.checks.projectInputRoundtrip=true;for(const value of messages)assert.ok(Object.values(value.checks).every(Boolean));
 assert.ok(messages.every(value=>value.uid===process.getuid()));report.checks.publicAndPrivateAllGenerations=true;
 report.owned=ownedPids(childFile);assert.ok(report.owned.length>=3);assert.equal(controllerAccepted,0);
 const verified=await managerCall(['live-policy','verify-process','--manifest',manifest,'--launch-id',first.id]);assert.equal(verified.code,0);
 const firstReceipt=JSON.parse(readFileSync(first.path)).body;report.launch={launchId:first.id,owner:firstReceipt.owner,process:firstReceipt.process,contractDigest:firstReceipt.contractDigest,
 policyDigest:firstReceipt.policy.policyDigest,configDigest:firstReceipt.policy.configDigest,manager:firstReceipt.policy.manager};
 assert.equal(firstReceipt.policy.manager.sha256,provenance.files['public/gestalt']);
 report.checks.processBoundReceipt=true;
 const busy=await managerCall(['live-policy','prepare','--manifest',manifest]);assert.equal(JSON.parse(busy.stdout).reason,'active-launch');report.checks.noPolicyReplacementWhileActive=true;
 report.stage='permission-narrowing';m.sandboxState.permissionProfile.file_system.entries[1].access='read';save();await stopped(first);
 assert.equal(JSON.parse(readFileSync(first.path)).body.termination,'policy-changed');await waitFor(()=>report.owned.every(dead));report.checks.narrowingReapsDescendants=true;
 const stale=await managerCall(['live-policy','verify-process','--manifest',manifest,'--launch-id',first.id]);assert.equal(stale.code,1);report.checks.staleReceiptRefused=true;
 const narrow=await launch();await nativeMessages(narrow);
 assert.ok(narrow.messages().every(value=>!value.checks.publicWrite&&value.checks.publicRead&&value.checks.privateFiles));report.checks.narrowedPolicyEnforced=true;
 assert.equal((await managerCall(['live-policy','stop','--manifest',manifest,'--launch-id',narrow.id])).code,0);await stopped(narrow);
 report.stage='owner-death';m.sandboxState.permissionProfile.file_system.entries[1].access='write';const orphan=await launch();await nativeMessages(orphan);
 const orphanPids=ownedPids(childFile);orphan.child.kill('SIGKILL');await stopped(orphan);await waitFor(()=>orphanPids.every(dead));report.checks.ownerDeathReaps=true;
 report.stage='leader-exit';const leader=await launch('leader-exit');await stopped(leader);assert.equal(JSON.parse(readFileSync(leader.path)).body.termination,'leader-exit');
 await waitFor(()=>ownedPids(childFile).length===0);report.checks.leaderExitReapsDetachedGrandchild=true;
 process.kill(unowned.pid,0);assert.equal(await api(),200);assert.equal(lstatSync(daemon).mode&0o777,0o700);
 const retained=await rpc('fs/readFile',{path:auth});assert.equal(retained.dataBase64,hostRead.dataBase64);report.checks.ownedOnlyCleanupAndHostAuthority=true;
 writeFileSync(join(evidence,'effective-policy.json'),JSON.stringify(JSON.parse(readFileSync(join(privateRoot,'policy-receipt.json'))).body.sandboxState,null,2)+'\n');
 report.stage='complete';report.passed=true;
} catch(error) {
 report.failureDetail={stage:report.stage,assertion:error.operator,actual:typeof error.actual==='number'||typeof error.actual==='boolean'?error.actual:undefined,expected:typeof error.expected==='number'||typeof error.expected==='boolean'?error.expected:undefined};
 report.failure=error.code??(/^[A-Z_]+$/.test(error.message)?error.message:'ASSERTION_FAILED');
 if(active){report.nativeDiagnostic=active.diagnostic();report.launchBoundary=launchBoundary(active);report.ownedProcessSnapshot=ownedProcessSnapshot(active);report.runtimeHomeAudit={mode:(lstatSync(home).mode&0o777).toString(8),configUnchanged:hash(readFileSync(join(home,'config.toml')))===m.configFiles[0].sha256,prohibited:Object.fromEntries(['auth.json','AGENTS.md','hooks.json','hooks','plugins','skills','.codex'].map(name=>[name,existsSync(join(home,name))]))};}process.exitCode=1;
} finally {
 if(active){try{await managerCall(['live-policy','stop','--manifest',manifest,'--launch-id',active.id]);await stopped(active);}catch{report.passed=false;process.exitCode=1;active.child.kill('SIGTERM');}}
 if(extraFD!==undefined)closeSync(extraFD);
 try {
  report.checks.ownedControllerShutdown=await closeOwned(runtime?.child);
  report.checks.ownedCaddyShutdown=await closeOwned(caddy);
  await closeOwned(unowned);
 } catch {report.passed=false;report.failure='OWNED_SHUTDOWN_FAILED';process.exitCode=1;}
 await new Promise(done=>server.close(done));
 writeFileSync(join(evidence,'native-proof.json'),JSON.stringify(report,null,2)+'\n');
 rmSync(root,{recursive:true,force:true});
 console.log(JSON.stringify({stage:report.stage,passed:report.passed,checks:report.checks,failure:report.failure}));
}
