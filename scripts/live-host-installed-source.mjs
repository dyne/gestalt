// Test-only authentic package preparation. Never used by Manager or Live Start.
import assert from 'node:assert/strict';
import {cpSync,readFileSync,mkdirSync,lstatSync,readdirSync,chmodSync,realpathSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {join,dirname,relative} from 'node:path';
import {pathToFileURL} from 'node:url';
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
export const ACCEPTED_AGENTS={commit:'129d33bc6d6c58d280d9bcf5155fab6ed9b6e0bd',tree:'414a89284efbb050188ca1802563b92667c50994',contextTree:'640de6ca23b854b3b1125d1c034a196179ac7ff8',configuratorSha256:'012391f97159b24b8c77a3132c80d6b28429ba0383cdf1b71da702dbe445168a',preparerSha256:'1fefbf31f1e79b0c6ae4f2c6448b7aa1c6cd3de3db4ce57a432a2e20f71aa165'};
export async function prepareOriginalInstalledBridge({agentsRoot,originalHome,gestaltHome,installation}) {
 const source=join(agentsRoot,'plugins/context-mode'),version=JSON.parse(readFileSync(join(source,'package.json'))).version;
 const preparer=join(agentsRoot,'plugins/gestalt/scripts/prepare-live-runtime.mjs'),configurator=join(source,'scripts/configure-codex.mjs');
 assert.equal(hash(readFileSync(configurator)),ACCEPTED_AGENTS.configuratorSha256);assert.equal(hash(readFileSync(preparer)),ACCEPTED_AGENTS.preparerSha256);
 const packageRoot=join(installation,'context-mode',version);mkdirSync(dirname(packageRoot),{recursive:true,mode:0o700});
 cpSync(source,packageRoot,{recursive:true,filter:path=>!relative(source,path).split('/').some(name=>name==='node_modules'||name==='build'||name.startsWith('.context-mode-prepare'))});
 mkdirSync(gestaltHome,{recursive:true,mode:0o700});mkdirSync(join(gestaltHome,'runtime'),{mode:0o700});
 const {prepareLiveRuntime}=await import(pathToFileURL(preparer).href),{measureTree}=await import(pathToFileURL(join(agentsRoot,'plugins/gestalt/live-admission.mjs')).href);
 const prepared=await prepareLiveRuntime(packageRoot,join(gestaltHome,'runtime/context-mode'));
 // Normalize only freshly copied package storage before admission. The actual
 // generated launcher is kept at the configurator's original0755, untouched.
 const protect=path=>{const info=lstatSync(path);assert.equal(info.isSymbolicLink(),false);if(info.isDirectory())for(const name of readdirSync(path))protect(join(path,name));else assert.ok(info.isFile());chmodSync(path,info.mode&0o755);};protect(packageRoot);
 const {configureCodexIntegration}=await import(pathToFileURL(configurator).href);
 mkdirSync(join(originalHome,'bin'),{mode:0o700});
 const configured=configureCodexIntegration({codexHome:originalHome,gestaltHome,pluginRoot:packageRoot,pluginId:'context-mode@dyne-gestalt-agents'});
 assert.equal(lstatSync(configured.launcherPath).mode&0o7777,0o755);
 const interpreter=realpathSync(process.execPath),descriptor={kind:'context-mode-bridge-v1',path:configured.launcherPath,sha256:hash(readFileSync(configured.launcherPath)),gestaltHome,interpreter:{path:interpreter,sha256:hash(readFileSync(interpreter))},package:{root:packageRoot,version,treeDigest:measureTree(packageRoot)},runtime:{root:prepared.root,treeDigest:prepared.sha256}};
 const result={descriptor,provenance:{...ACCEPTED_AGENTS,version,interpreter:descriptor.interpreter,package:descriptor.package,runtime:descriptor.runtime,launcher:{sha256:descriptor.sha256,mode:'0755'},preparer:'actual accepted prepareLiveRuntime; npm .bin aliases removed by that preparer only',configurator:'actual accepted configureCodexIntegration; generated launcher untouched'},verify:()=>{
  assert.equal(lstatSync(descriptor.path).mode&0o7777,0o755);assert.equal(hash(readFileSync(descriptor.path)),descriptor.sha256);assert.equal(measureTree(packageRoot),descriptor.package.treeDigest);assert.equal(measureTree(prepared.root),descriptor.runtime.treeDigest);
 }};
 result.forHome=home=>{mkdirSync(join(home,'bin'),{mode:0o700});const next=configureCodexIntegration({codexHome:home,gestaltHome,pluginRoot:packageRoot,pluginId:'context-mode@dyne-gestalt-agents'});const value={...result,descriptor:{...descriptor,path:next.launcherPath,sha256:hash(readFileSync(next.launcherPath))}};value.verify=()=>{result.verify();assert.equal(lstatSync(next.launcherPath).mode&0o7777,0o755);assert.equal(hash(readFileSync(next.launcherPath)),value.descriptor.sha256);};return value;};
 return result;
}
