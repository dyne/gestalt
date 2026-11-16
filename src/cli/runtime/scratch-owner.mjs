// Filesystem/schema owner stays on the host. Process ownership is supplied by
// the caller's managed port; this module contains no spawn/group/PID fallback.
import * as nativeFs from 'node:fs';
import scratchPath from 'node:path';
import * as scratchCrypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { windowsSystemScript } from '../platform/windows-system.ts';

export function createScratchOwner(privateProbe = scratchPath.join(scratchPath.dirname(fileURLToPath(import.meta.url)),'native/gestalt-unix-peer')) {
function scratchWindows(request) {
  const systemRoot = process.env.SystemRoot;
  scratchAssert(systemRoot && scratchPath.isAbsolute(systemRoot));
  if (request.path) request = {...request, path: scratchPath.toNamespacedPath(request.path)};
  execFileSync(scratchPath.join(systemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'),
    ['-NoLogo','-NoProfile','-NonInteractive','-EncodedCommand',Buffer.from(windowsSystemScript,'utf16le').toString('base64')],
    {env:{...process.env,GESTALT_NATIVE_REQUEST:Buffer.from(JSON.stringify(request)).toString('base64')},shell:false,windowsHide:true,timeout:10000,stdio:['ignore','ignore','pipe']});
}
const scratchFs = {...nativeFs, mkdirSync(directory, options) {
  if (process.platform !== 'win32') return nativeFs.mkdirSync(directory,options);
  if (nativeFs.existsSync(directory)) return nativeFs.mkdirSync(directory,options); // preserve EEXIST semantics, never repair ACLs
  scratchWindows({operation:'create-directory',path:directory});
}};
function scratchAcl(file) {
  if (process.platform === 'win32') scratchWindows({operation:'assert-private',path:file});
  if (process.platform === 'darwin') execFileSync(privateProbe,['--private',file],{timeout:5000,stdio:'pipe'});
}
// POSIX fences also hold an open directory descriptor. Windows uses the native
// private DACL/namespace prerequisite and the same dev/ino/birth identity
// recheck as NativePrivateStorage; no process-lifetime authority is inferred.
function scratchOpenDirectory(directory) {
  scratchPrivate(directory);
  if (process.platform === 'win32') return {directory,info:nativeFs.lstatSync(directory,{bigint:true})};
  return nativeFs.openSync(directory,nativeFs.constants.O_RDONLY|nativeFs.constants.O_DIRECTORY|nativeFs.constants.O_NOFOLLOW);
}
const scratchDirectoryStat = held => typeof held === 'number' ? nativeFs.fstatSync(held,{bigint:true}) : held.info;
const scratchCloseDirectory = held => {if(typeof held === 'number')nativeFs.closeSync(held);};
const scratchVariable = 'GESTALT_SESSION_TMP_CONTEXT';
const scratchNames = [scratchVariable, 'TMPDIR', 'TMP', 'TEMP'];

const scratchId = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/.test(value);
const scratchUuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
function scratchAssert(condition) { if (!condition) throw Error('SESSION_SCRATCH_INVALID_CONTEXT'); }
function scratchCanonical(directory) {
  scratchAssert(typeof directory === 'string' && scratchPath.isAbsolute(directory) &&
    !/[\x00-\x1f\x7f]/.test(directory) && scratchPath.resolve(directory) === directory);
  let current = scratchPath.parse(directory).root;
  const parents = [];
  for (const component of directory.slice(current.length).split(scratchPath.sep).filter(Boolean)) {
    parents.push(current);
    current = scratchPath.join(current, component);
    const info = scratchFs.lstatSync(current);
    scratchAssert(info.isDirectory() && !info.isSymbolicLink());
  }
  scratchAssert(scratchFs.realpathSync(directory) === directory);
  if (process.platform === 'win32') scratchWindows({operation:'assert-namespace',paths:parents.map(value=>scratchPath.toNamespacedPath(value))});
  if (process.platform === 'darwin') for(const parent of parents)
    execFileSync(privateProbe,['--namespace',parent],{timeout:5000,stdio:'pipe'});
}
// Existing context validation remains compatible with the established marker
// contract. Mutating metadata additionally requires a stable native namespace;
// read-only diagnostics never acquire that write authority.
function scratchStableNamespace(directory) {
  scratchCanonical(directory);
  if (!process.getuid) return; // Windows namespace ACL check is in canonical.
  for (let current = directory; ; current = scratchPath.dirname(current)) {
    const info = scratchFs.lstatSync(current);
    if (![0,process.getuid()].includes(info.uid) || ((info.mode & 0o022) && !(info.uid === 0 && (info.mode & 0o1000))))
      throw Error('SESSION_SCRATCH_NAMESPACE_UNAVAILABLE: an untrusted ancestor can redirect metadata; use a private native workspace namespace');
    if (current === scratchPath.dirname(current)) break;
  }
}
function scratchPrivate(directory) {
  scratchCanonical(directory);
  const info = scratchFs.lstatSync(directory);
  scratchAssert((!process.getuid || info.uid === process.getuid()) &&
    (process.platform === 'win32' || (info.mode & 0o777) === 0o700));
  scratchAcl(directory);
}
function scratchJson(file) {
  const before = scratchFs.lstatSync(file); scratchAssert(!before.isSymbolicLink()); scratchAcl(file);
  const fd = scratchFs.openSync(file, scratchFs.constants.O_RDONLY | scratchFs.constants.O_NOFOLLOW);
  try {
    const info = scratchFs.fstatSync(fd);
    scratchAssert(info.isFile() && info.nlink === 1 && info.size <= 16384 &&
      (!process.getuid || info.uid === process.getuid()) &&
      (process.platform === 'win32' || (info.mode & 0o077) === 0));
    return JSON.parse(scratchFs.readFileSync(fd, 'utf8'));
  } finally { scratchFs.closeSync(fd); }
}
function scratchRuntimeContainer() {
  scratchAssert(typeof process.getuid === 'function');
  return scratchPath.join(scratchFs.realpathSync('/tmp'), 'gs-' + process.getuid());
}
function scratchRuntimeIdentity(directory) {
  scratchPrivate(directory);
  const info = scratchFs.lstatSync(directory, {bigint:true});
  return `${info.dev}:${info.ino}:${info.birthtimeNs}`;
}
function scratchRuntimeRecord(owner, runtime) {
  return {schemaVersion:2,ownerId:owner.ownerId,workspacePath:owner.workspacePath,token:runtime.token,identity:runtime.identity};
}
function scratchAllocateRuntime(owner) {
  const container = scratchRuntimeContainer();
  try {scratchFs.mkdirSync(container,{mode:0o700});} catch(error) {if(error.code!=='EEXIST')throw error;}
  scratchPrivate(container);
  for(let attempt=0;attempt<16;attempt++) {
    const directory=scratchPath.join(container,scratchCrypto.randomBytes(10).toString('hex'));
    if(Buffer.byteLength(directory)>48)throw Error('SESSION_SCRATCH_RUNTIME_ROOT_TOO_LONG');
    try {scratchFs.mkdirSync(directory,{mode:0o700});} catch(error) {if(error.code==='EEXIST')continue;throw error;}
    const runtime={directory,token:scratchCrypto.randomBytes(32).toString('hex'),identity:scratchRuntimeIdentity(directory)};
    scratchPublish(scratchPath.join(directory,'.gestalt-runtime.json'),scratchRuntimeRecord(owner,runtime),true);
    return runtime;
  }
  throw Error('SESSION_SCRATCH_RUNTIME_COLLISION');
}
function scratchValidateRuntime(owner,runtime) {
  const container=scratchRuntimeContainer();
  scratchAssert(runtime && typeof runtime.directory==='string' && scratchPath.dirname(runtime.directory)===container &&
    /^[0-9a-f]{20}$/.test(scratchPath.basename(runtime.directory)) && Buffer.byteLength(runtime.directory)<=48 &&
    typeof runtime.token==='string' && /^[0-9a-f]{64}$/.test(runtime.token) &&
    typeof runtime.identity==='string' && /^\d+:\d+:\d+$/.test(runtime.identity) && runtime.retiring===undefined);
  scratchPrivate(container);
  scratchAssert(scratchRuntimeIdentity(runtime.directory)===runtime.identity &&
    JSON.stringify(scratchJson(scratchPath.join(runtime.directory,'.gestalt-runtime.json')))===JSON.stringify(scratchRuntimeRecord(owner,runtime)));
}
function scratchTemporaryDirectory(context) {return context.runtime?.directory ?? context.directory;}
function scratchSocketPath(endpoint,capacity=({linux:108,darwin:104})[process.platform]) {
  if(!capacity)throw Error('UNIX_SOCKET_PLATFORM_UNSUPPORTED: no verified pathname capacity for this platform');
  if(endpoint.includes('\0') || Buffer.byteLength(endpoint)+1>capacity)
    throw Error('UNIX_SOCKET_PATH_TOO_LONG: shorten the endpoint name or temporary root');
  return endpoint;
}
function scratchWorkspacePublication(workspace,publish) {
  scratchStableNamespace(workspace);
  const path=scratchPath.join(workspace,'.gestalt-scratch-lock');
  let claimed=false;
  for(let attempt=0;attempt<100;attempt++) {
    try {scratchFs.mkdirSync(path,{mode:0o700});claimed=true;break;}
    catch(error){if(error.code!=='EEXIST')throw error;Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);}
  }
  if(!claimed)throw Error('SESSION_SCRATCH_BUSY');
  scratchPrivate(path);
  const fd=scratchOpenDirectory(path);
  const held=scratchDirectoryStat(fd);
  try {
    const parents=scratchManagedAncestors(workspace).map(ancestor=>ancestor.context.workspacePath);
    scratchCheckParentPublications(workspace,parents);
    const result=publish();
    scratchCheckParentPublications(workspace,parents);
    return result;
  }
  finally {
    try {
      const current=scratchFs.lstatSync(path,{bigint:true});
      scratchAssert(current.isDirectory() && !current.isSymbolicLink() && current.dev===held.dev && current.ino===held.ino && current.birthtimeNs===held.birthtimeNs);
      scratchFs.rmdirSync(path);
    } finally {scratchCloseDirectory(fd);}
  }
}
function scratchCheckParentPublications(workspace,logicalParents=[]) {
  const parents=new Set(logicalParents);let current=scratchPath.parse(workspace).root;
  for(const part of workspace.slice(current.length).split(scratchPath.sep).filter(Boolean)) {
    current=scratchPath.join(current,part);if(part==='tmp')parents.add(scratchPath.dirname(current));
  }
  parents.delete(workspace);
  for(const parent of parents) {
    try{scratchFs.lstatSync(scratchPath.join(parent,'.gestalt-scratch-lock'));}
    catch(error){if(error.code==='ENOENT')continue;throw error;}
    throw Error('SESSION_SCRATCH_BUSY');
  }
}
function scratchContext(environment = process.env, allowReleased = false) {
  if (!Object.hasOwn(environment, scratchVariable)) return null;
  const context = JSON.parse(environment[scratchVariable]);
  scratchAssert([1,2].includes(context?.schemaVersion) && scratchId(context.ownerId));
  scratchCanonical(context.workspacePath);
  const container = scratchPath.join(context.workspacePath, 'tmp', 'sessions');
  scratchAssert(context.directory === scratchPath.join(container, context.ownerId) &&
    context.markerPath === scratchPath.join(context.directory, '.gestalt-owner.json'));
  // tmp may already contain user files/rules. Match the producer's parent
  // check without changing its existing permissions; managed descendants alone
  // require 0700.
  const parent = scratchPath.dirname(container);
  scratchCanonical(parent);
  scratchAssert(!process.getuid || scratchFs.lstatSync(parent).uid === process.getuid());
  for (const directory of [container, context.directory]) scratchPrivate(directory);
  const marker = scratchJson(context.markerPath);
  scratchAssert(marker?.schemaVersion === context.schemaVersion && marker.ownerId === context.ownerId &&
    marker.workspacePath === context.workspacePath && scratchId(marker.sessionId) &&
    typeof marker.createdAt === 'string' && Number.isFinite(Date.parse(marker.createdAt)) &&
    Array.isArray(marker.providerThreads) && marker.providerThreads.every(thread =>
      ['codex', 'kimi'].includes(thread?.provider) && scratchId(thread.threadId)));
  if(context.schemaVersion===2) {
    scratchAssert(JSON.stringify(context.runtime)===JSON.stringify(marker.runtime));
    scratchValidateRuntime(marker,marker.runtime);
  } else scratchAssert(context.runtime===undefined && marker.runtime===undefined);
  if (marker.ownerKind === 'mobile-session') {
    scratchAssert(context.ownerId === 'mobile-' + scratchCrypto.createHash('sha256').update(marker.sessionId).digest('hex') &&
      marker.authority?.kind === 'mobile-roster' && marker.authority.sessionId === marker.sessionId);
  } else {
    const prefix = marker.ownerKind === 'cli-session' ? 'cli-' : marker.ownerKind === 'workspace-service' ? 'service-' : '';
    scratchAssert(prefix && context.ownerId.startsWith(prefix) && scratchUuid(context.ownerId.slice(prefix.length)) &&
      marker.sessionId === context.ownerId && marker.authority?.kind === 'lease' && scratchUuid(marker.authority.leaseId));
    const leases = scratchPath.join(context.directory, 'leases');
    scratchPrivate(leases);
    const lease = scratchJson(scratchPath.join(leases, marker.authority.leaseId + '.json'));
    scratchAssert(lease?.schemaVersion === 1 && lease.ownerId === context.ownerId && lease.workspacePath === context.workspacePath &&
      lease.leaseId === marker.authority.leaseId && (lease.state === 'active' || allowReleased && lease.state === 'released') && scratchUuid(lease.processIdentity) &&
      typeof lease.createdAt === 'string' && Number.isFinite(Date.parse(lease.createdAt)) &&
      typeof lease.updatedAt === 'string' && Number.isFinite(Date.parse(lease.updatedAt)) &&
      Array.isArray(lease.consumers) && lease.consumers.every(scratchId));
  }
  return context;
}
function scratchEnvironment(environment = process.env) {
  const context = scratchContext(environment);
  const directory=context && scratchTemporaryDirectory(context);
  return context ? { ...environment, TMPDIR: directory, TMP: directory, TEMP: directory } : { ...environment };
}
function scratchDiscoveryEnvironment(environment = process.env) {
  const result = { ...environment };
  if (Object.hasOwn(result, scratchVariable)) for (const name of scratchNames) delete result[name];
  return result;
}
function scratchPublish(file, value, initial = false) {
  const staged = initial ? file : scratchPath.join(scratchPath.dirname(file), '.' + scratchCrypto.randomUUID() + '.json');
  const fd = scratchFs.openSync(staged, 'wx', 0o600);
  try { scratchFs.writeFileSync(fd, JSON.stringify(value) + '\n'); scratchFs.fsyncSync(fd); }
  finally { scratchFs.closeSync(fd); }
  if (!initial) scratchFs.renameSync(staged, file);
}
function scratchContainer(workspace) {
  scratchCanonical(workspace);
  const parent = scratchPath.join(workspace, 'tmp');
  try { scratchFs.mkdirSync(parent, {mode: 0o700}); } catch (e) { if (e.code !== 'EEXIST') throw e; }
  scratchCanonical(parent);
  scratchAssert(!process.getuid || scratchFs.lstatSync(parent).uid === process.getuid());
  const container = scratchPath.join(parent, 'sessions');
  try { scratchFs.mkdirSync(container, {mode: 0o700}); } catch (e) { if (e.code !== 'EEXIST') throw e; }
  scratchPrivate(container);
  try { scratchFs.writeFileSync(scratchPath.join(container, '.gitignore'), '*\n', {mode: 0o600, flag: 'wx'}); }
  catch (e) { if (e.code !== 'EEXIST') throw e; }
  return container;
}
function scratchOwnerContext(workspace, ownerId) {
  const directory = scratchPath.join(workspace, 'tmp', 'sessions', ownerId);
  const context={schemaVersion:1,workspacePath:workspace,ownerId,directory,markerPath:scratchPath.join(directory,'.gestalt-owner.json')};
  try {const marker=scratchJson(context.markerPath);if(marker.schemaVersion===2){context.schemaVersion=2;context.runtime=marker.runtime;}}
  catch(error){if(error.code!=='ENOENT')throw error;}
  return context;
}
function scratchLeaseRecords(context) {
  const directory = scratchPath.join(context.directory, 'leases');
  scratchPrivate(directory);
  return scratchFs.readdirSync(directory).map(name => {
    scratchAssert(scratchUuid(name.replace(/\.json$/, '')) && name.endsWith('.json'));
    const value = scratchJson(scratchPath.join(directory, name));
    scratchAssert(value?.schemaVersion === 1 && value.ownerId === context.ownerId && value.workspacePath === context.workspacePath &&
      name === value.leaseId + '.json' && ['active', 'released'].includes(value.state) && scratchUuid(value.processIdentity) &&
      typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt)) &&
      typeof value.updatedAt === 'string' && Number.isFinite(Date.parse(value.updatedAt)) &&
      Array.isArray(value.consumers) && value.consumers.every(scratchId) && (value.state !== 'released' || value.consumers.length === 0));
    return value;
  });
}
function scratchDirectoryIdentity(directory) {
  const info = scratchFs.lstatSync(directory, {bigint: true});
  scratchAssert(info.isDirectory() && !info.isSymbolicLink());
  return {dev: info.dev, ino: info.ino, birth: info.birthtimeNs};
}
function scratchSameDirectory(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.birth === right.birth;
}
function scratchAncestorOwner(context) {
  // Released CLI/service ancestors still have positive attribution. This read
  // grants no cleanup authority and never expires a lease or steals a fence.
  scratchContext({[scratchVariable]: JSON.stringify(context)}, true);
  const marker = scratchJson(context.markerPath);
  try {
    scratchFs.lstatSync(scratchPath.join(context.directory, 'leases'));
    scratchLeaseRecords(context);
  } catch (error) {if (error.code !== 'ENOENT' || marker.ownerKind !== 'mobile-session') throw error;}
  return {
    context, identity: scratchDirectoryIdentity(context.directory),
    provenance: JSON.stringify([marker.schemaVersion, marker.ownerKind, marker.ownerId, marker.workspacePath,
      marker.sessionId, marker.createdAt, marker.authority.kind, marker.authority.sessionId ?? null, marker.authority.leaseId ?? null, marker.runtime ?? null]),
  };
}
function scratchManagedAncestors(workspace, ancestors=[], visiting=new Set()) {
  scratchAssert(!visiting.has(workspace) && visiting.size<64);
  visiting.add(workspace);
  const parts = workspace.slice(scratchPath.parse(workspace).root.length).split(scratchPath.sep).filter(Boolean);
  const runtimeBase=process.getuid?scratchRuntimeContainer():undefined;
  let directory = scratchPath.parse(workspace).root;
  for (let i = 0; i < parts.length; i++) {
    directory = scratchPath.join(directory, parts[i]);
    if(runtimeBase && scratchPath.dirname(directory)===runtimeBase) {
      scratchPrivate(directory);
      const locator=scratchJson(scratchPath.join(directory,'.gestalt-runtime.json'));
      scratchAssert(typeof locator.workspacePath==='string' && scratchId(locator.ownerId));
      const owner=scratchOwnerContext(locator.workspacePath,locator.ownerId);
      scratchContext({[scratchVariable]:JSON.stringify(owner)},true);
      scratchAssert(owner.runtime?.directory===directory);
      scratchManagedAncestors(owner.directory,ancestors,visiting);
    }
    if (i >= 2 && parts[i - 1] === 'sessions' && parts[i - 2] === 'tmp') {
      if(ancestors.some(ancestor=>ancestor.context.directory===directory))continue;
      scratchAssert(ancestors.length<64);
      const ownerWorkspace = scratchPath.dirname(scratchPath.dirname(scratchPath.dirname(directory)));
      ancestors.push(scratchAncestorOwner(scratchOwnerContext(ownerWorkspace, parts[i])));
    }
  }
  visiting.delete(workspace);
  return ancestors;
}
function scratchRevalidateAncestor(ancestor) {
  const current = scratchAncestorOwner(ancestor.context);
  scratchAssert(scratchSameDirectory(ancestor.identity, current.identity) && ancestor.provenance === current.provenance);
}
// Offline operator recovery only. An empty directory supplies no evidence that
// its publisher died, so normal allocation must never invoke this override.
function scratchContainmentSnapshot(workspace, ownerId) {
  scratchAssert(scratchId(ownerId));
  const context = scratchOwnerContext(workspace, ownerId);
  const ancestor = scratchAncestorOwner(context);
  scratchAssert(scratchJson(context.markerPath).ownerKind === 'mobile-session');
  const path = scratchPath.join(context.directory, '.containment-lock');
  const expected = scratchDirectoryIdentity(path);
  scratchPrivate(path);
  scratchAssert(scratchFs.readdirSync(path).length === 0);
  // Other metadata writers must be quiescent too; their claims are never
  // cleared by containment recovery, regardless of age or apparent PID state.
  for (const name of ['.controller-lock', '.association-lock']) {
    try {scratchFs.lstatSync(scratchPath.join(context.directory, name));}
    catch (error) {if (error.code === 'ENOENT') continue; throw error;}
    throw new Error('SESSION_SCRATCH_RECOVERY_WRITER_BUSY');
  }
  const token = scratchCrypto.createHash('sha256').update(JSON.stringify({
    owner: ancestor.provenance,
    ownerIdentity: ancestor.identity,
    marker: scratchJson(context.markerPath),
    leases: scratchFs.existsSync(scratchPath.join(context.directory, 'leases'))
      ? scratchLeaseRecords(context).sort((left, right) => left.leaseId.localeCompare(right.leaseId)) : [],
    lockIdentity: expected,
  }, (_, value) => typeof value === 'bigint' ? value.toString() : value)).digest('hex');
  return {context, ancestor, path, expected, token};
}
function scratchRecoverContainment(workspace, ownerId, token) {
  scratchAssert(typeof token === 'string' && /^[a-f0-9]{64}$/.test(token));
  const snapshot = scratchContainmentSnapshot(workspace, ownerId);
  if (snapshot.token !== token) throw new Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
  const fd = scratchOpenDirectory(snapshot.path);
  try {
    const held = scratchDirectoryStat(fd);
    scratchAssert(scratchSameDirectory(snapshot.expected, {dev: held.dev, ino: held.ino, birth: held.birthtimeNs}));
    scratchRevalidateAncestor(snapshot.ancestor);
    // Recheck after opening. This is explicitly offline recovery, not a claim
    // that check-then-rmdir is safe against a concurrent cooperating writer.
    const current = scratchContainmentSnapshot(workspace, ownerId);
    if (current.token !== token) throw new Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
    scratchFs.rmdirSync(snapshot.path); // Empty only; never recursive.
  } finally {scratchCloseDirectory(fd);}
}
// A closed roster row cannot prove detached tool exit. These legacy leases
// have no durable process handle; only an explicit offline operator assertion
// can retire them. Normal allocation, shutdown and cleanup never call this.
function scratchToolLeaseSnapshot(workspace, ownerId) {
  scratchAssert(scratchId(ownerId));
  const context = scratchOwnerContext(workspace, ownerId);
  const ancestor = scratchAncestorOwner(context);
  const marker = scratchJson(context.markerPath);
  scratchAssert(marker.ownerKind === 'mobile-session');
  for (const name of ['.controller-lock', '.association-lock', '.containment-lock']) {
    try {scratchFs.lstatSync(scratchPath.join(context.directory, name));}
    catch (error) {if (error.code === 'ENOENT') continue; throw error;}
    throw new Error('SESSION_SCRATCH_RECOVERY_WRITER_BUSY');
  }
  const directory = scratchPath.join(context.directory, 'leases');
  const leases = scratchLeaseRecords(context).sort((a, b) => a.leaseId.localeCompare(b.leaseId));
  const active = leases.filter(lease => lease.state === 'active');
  scratchAssert(active.length > 0 && active.every(lease => lease.consumers.length === 1 &&
    ['serena-mcp', 'playwright-mcp'].includes(lease.consumers[0])));
  const fileIdentity = path => {
    const info = scratchFs.lstatSync(path, {bigint: true});
    return {dev: info.dev, ino: info.ino, birth: info.birthtimeNs};
  };
  const token = scratchCrypto.createHash('sha256').update(JSON.stringify({
    kind: 'mobile-tool-leases', owner: ancestor.provenance, ownerIdentity: ancestor.identity,
    marker, markerIdentity: fileIdentity(context.markerPath),
    leaseDirectory: scratchDirectoryIdentity(directory),
    leases: leases.map(lease => ({lease, identity: fileIdentity(scratchPath.join(directory, lease.leaseId + '.json'))})),
  }, (_, value) => typeof value === 'bigint' ? value.toString() : value)).digest('hex');
  return {context, active, token};
}
function scratchRecoverToolLeases(workspace, ownerId, token) {
  scratchAssert(typeof token === 'string' && /^[a-f0-9]{64}$/.test(token));
  const snapshot = scratchToolLeaseSnapshot(workspace, ownerId);
  if (snapshot.token !== token) throw new Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
  // Recheck the entire inspected set before publishing any changes. This is
  // offline recovery, not synchronization with a running metadata writer.
  if (scratchToolLeaseSnapshot(workspace, ownerId).token !== token)
    throw new Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
  for (const lease of snapshot.active) {
    scratchPublish(scratchPath.join(snapshot.context.directory, 'leases', lease.leaseId + '.json'),
      {...lease, state: 'released', consumers: [], updatedAt: new Date().toISOString()});
  }
}
// Offline-only publication recovery also works when allocation never reached an owner.
function scratchPublicationSnapshot(workspace) {
  scratchCanonical(workspace);
  const path=scratchPath.join(workspace,'.gestalt-scratch-lock');
  scratchPrivate(path);
  scratchAssert(scratchFs.readdirSync(path).length===0);
  const expected=scratchDirectoryIdentity(path);
  const token=scratchCrypto.createHash('sha256').update(JSON.stringify({
    workspace:scratchDirectoryIdentity(workspace),lock:expected,
    ancestors:scratchManagedAncestors(workspace).map(ancestor=>({
      owner:ancestor.provenance,identity:ancestor.identity,
      leases:scratchFs.existsSync(scratchPath.join(ancestor.context.directory,'leases'))
        ? scratchLeaseRecords(ancestor.context).sort((a,b)=>a.leaseId.localeCompare(b.leaseId)):[],
    })),
  },(_,value)=>typeof value==='bigint'?value.toString():value)).digest('hex');
  return {path,expected,token};
}
function scratchRecoverPublication(workspace,token) {
  scratchAssert(typeof token==='string' && /^[a-f0-9]{64}$/.test(token));
  const snapshot=scratchPublicationSnapshot(workspace);
  if(snapshot.token!==token)throw Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
  const fd=scratchOpenDirectory(snapshot.path);
  try {
    const held=scratchDirectoryStat(fd);
    scratchAssert(scratchSameDirectory(snapshot.expected,{dev:held.dev,ino:held.ino,birth:held.birthtimeNs}));
    if(scratchPublicationSnapshot(workspace).token!==token)throw Error('SESSION_SCRATCH_RECOVERY_SNAPSHOT_CHANGED');
    scratchFs.rmdirSync(snapshot.path);
  } finally {scratchCloseDirectory(fd);}
}
function scratchRecoveryCommand(args, write = value => process.stdout.write(value)) {
  const [action, ...options] = args;
  scratchAssert(['inspect', 'recover', 'inspect-tool-leases', 'recover-tool-leases', 'inspect-publication', 'recover-publication'].includes(action));
  const values = {};
  let confirmed = false;
  for (let index = 0; index < options.length; index++) {
    const name = options[index];
    if (name === '--confirm-quiescent') {scratchAssert(!confirmed); confirmed = true; continue;}
    scratchAssert(['--cwd', '--owner', '--expect'].includes(name) && !Object.hasOwn(values, name));
    scratchAssert(typeof options[index + 1] === 'string' && !options[index + 1].startsWith('--'));
    values[name] = options[++index];
  }
  scratchAssert(values['--cwd'] && (action.endsWith('-publication') ? !values['--owner'] : values['--owner']));
  if(action.endsWith('-publication')) {
    if(action==='inspect-publication') {
      scratchAssert(!confirmed && !values['--expect']);
      write(JSON.stringify({schemaVersion:1,status:'confirmationRequired',token:scratchPublicationSnapshot(values['--cwd']).token})+'\n');
    } else {
      scratchAssert(confirmed && values['--expect']);
      scratchRecoverPublication(values['--cwd'],values['--expect']);
      write(JSON.stringify({schemaVersion:1,status:'recovered'})+'\n');
    }
    return;
  }
  if (action === 'inspect' || action === 'inspect-tool-leases') {
    scratchAssert(!confirmed && !values['--expect']);
    const snapshot = action === 'inspect' ? scratchContainmentSnapshot(values['--cwd'], values['--owner'])
      : scratchToolLeaseSnapshot(values['--cwd'], values['--owner']);
    write(JSON.stringify({schemaVersion: 1, status: 'confirmationRequired', token: snapshot.token,
      ...(snapshot.active ? {activeToolLeases: snapshot.active.length} : {})}) + '\n');
  } else {
    scratchAssert(confirmed && values['--expect']);
    if (action === 'recover') scratchRecoverContainment(values['--cwd'], values['--owner'], values['--expect']);
    else scratchRecoverToolLeases(values['--cwd'], values['--owner'], values['--expect']);
    write(JSON.stringify({schemaVersion: 1, status: 'recovered'}) + '\n');
  }
}
function scratchWithContainment(workspace, publish) {
  return scratchWorkspacePublication(workspace,()=>{
  scratchCanonical(workspace);
  const selected = scratchDirectoryIdentity(workspace), ancestors = scratchManagedAncestors(workspace), claims = [];
  try {
    // Path traversal supplies outermost-first order. These short fences precede
    // new-owner controller/association work and never span a runtime lifetime.
    for (const ancestor of ancestors) {
      const path = scratchPath.join(ancestor.context.directory, '.containment-lock');
      try {scratchFs.mkdirSync(path, {mode: 0o700});}
      catch (error) {
        if (error.code === 'EEXIST') throw new Error('SESSION_SCRATCH_CONTAINMENT_BUSY');
        throw error;
      }
      const claim = {path, identity: scratchDirectoryIdentity(path), fd: undefined};
      claims.push(claim);
      scratchPrivate(path);
      claim.fd = scratchOpenDirectory(path);
      const held = scratchDirectoryStat(claim.fd);
      scratchAssert(scratchSameDirectory(claim.identity, {dev: held.dev, ino: held.ino, birth: held.birthtimeNs}));
      scratchRevalidateAncestor(ancestor);
    }
    scratchCanonical(workspace);
    scratchAssert(scratchSameDirectory(selected, scratchDirectoryIdentity(workspace)));
    for (const ancestor of ancestors) scratchRevalidateAncestor(ancestor);
    // This publication callback is synchronous. No recursive workspace creation:
    // retirement or alias/identity changes
    // between discovery and fencing fail before scratchContainer publishes.
    return publish();
  } finally {
    let failure;
    for (const claim of claims.reverse()) {
      try {
        if (!scratchSameDirectory(claim.identity, scratchDirectoryIdentity(claim.path)))
          throw new Error('SESSION_SCRATCH_CONTAINMENT_FENCE_CHANGED');
        scratchFs.rmdirSync(claim.path);
      } catch (error) {failure ??= error;}
      finally {
        try {if (claim.fd !== undefined) scratchCloseDirectory(claim.fd);}
        catch (error) {failure ??= error;}
      }
    }
    if (failure) throw failure;
  }
  });
}
function scratchAllocate(workspace,kind,consumer) {
 scratchAssert(['cli-session','workspace-service'].includes(kind)&&scratchId(consumer));
 return scratchWithContainment(workspace,()=>{
 scratchContainer(workspace);
 const ownerId=(kind==='cli-session'?'cli-':'service-')+scratchCrypto.randomUUID(),context=scratchOwnerContext(workspace,ownerId);
 scratchFs.mkdirSync(context.directory,{mode:0o700});scratchPrivate(context.directory);
 const lock=scratchPath.join(context.directory,'.controller-lock');scratchFs.mkdirSync(lock,{mode:0o700});
 scratchFs.mkdirSync(scratchPath.join(context.directory,'leases'),{mode:0o700});
 const leaseId=scratchCrypto.randomUUID(),time=new Date().toISOString();
 const lease={schemaVersion:1,ownerId,workspacePath:workspace,leaseId,state:'active',processIdentity:scratchCrypto.randomUUID(),createdAt:time,updatedAt:time,consumers:[consumer]};
 const marker={schemaVersion:1,ownerKind:kind,ownerId,workspacePath:workspace,sessionId:ownerId,createdAt:time,authority:{kind:'lease',leaseId},providerThreads:[]};
 if(process.platform!=='win32') {
   marker.schemaVersion=2;marker.runtime=scratchAllocateRuntime(marker);
   context.schemaVersion=2;context.runtime=marker.runtime;
 }
 scratchPublish(scratchPath.join(context.directory,'leases',leaseId+'.json'),lease,true);scratchPublish(context.markerPath,marker,true);
 scratchContext({[scratchVariable]:JSON.stringify(context)});return {context,lease,lock};
 });
}
function scratchRelease(ownership) {
  const {context, lease, lock} = ownership;
  return scratchMetadata(context, () => {
  scratchPrivate(scratchPath.join(context.directory, 'leases'));
  // Unknown or unreleased borrowers protect the complete owner, not just one
  // process's view of it. Never use a PID or timestamp to retire another lease.
  if (!ownership.borrowed && scratchLeaseRecords(context).some(value => value.leaseId !== lease.leaseId && value.state !== 'released')) return false;
  const file = scratchPath.join(context.directory, 'leases', lease.leaseId + '.json');
  const current = scratchJson(file);
  scratchAssert(current.schemaVersion === 1 && current.ownerId === context.ownerId && current.workspacePath === context.workspacePath &&
    current.leaseId === lease.leaseId && current.processIdentity === lease.processIdentity && current.state === 'active' &&
    current.createdAt === lease.createdAt && typeof current.updatedAt === 'string' && Number.isFinite(Date.parse(current.updatedAt)) &&
    JSON.stringify(current.consumers) === JSON.stringify(lease.consumers));
  scratchPublish(file, {...current, state: 'released', consumers: [], updatedAt: new Date().toISOString()});
  if (!ownership.borrowed) {scratchPrivate(lock); scratchFs.rmdirSync(lock);}
  return true;
  });
}
function scratchMetadata(context, operation, waitMs = 0) {
  return scratchWorkspacePublication(context.workspacePath,()=>{
  scratchContext({[scratchVariable]: JSON.stringify(context)});
  const mobile = scratchJson(context.markerPath).ownerKind === 'mobile-session';
  // Mobile cleanup/association use this same controller fence. CLI/service
  // controllers already hold it for their lifetime and serialize borrowers
  // against primary release with the association fence instead.
  const lock = scratchPath.join(context.directory, mobile ? '.controller-lock' : '.association-lock');
  const deadline = Date.now() + waitMs;
  while (true) {
    try {scratchFs.mkdirSync(lock, {mode: 0o700}); break;} catch (error) {
      if (error.code !== 'EEXIST') throw error;
      if (Date.now() >= deadline) return false;
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 10);
    }
  }
  try {
    scratchPrivate(lock);
    scratchContext({[scratchVariable]: JSON.stringify(context)});
    if (mobile) scratchAssert(!scratchFs.existsSync(scratchPath.join(context.directory, '.association-lock')));
    return operation();
  } finally {scratchFs.rmdirSync(lock);}
  });
}
function scratchBorrow(context, consumer) {
  scratchAssert(scratchId(consumer));
  const ownership = scratchMetadata(context, () => {
  const directory = scratchPath.join(context.directory, 'leases');
  try {scratchFs.mkdirSync(directory, {mode: 0o700});} catch (error) {if (error.code !== 'EEXIST') throw error;}
  scratchPrivate(directory);
  scratchLeaseRecords(context);
  const leaseId = scratchCrypto.randomUUID(), time = new Date().toISOString();
  const lease = {schemaVersion: 1, ownerId: context.ownerId, workspacePath: context.workspacePath, leaseId,
    state: 'active', processIdentity: scratchCrypto.randomUUID(), createdAt: time, updatedAt: time, consumers: [consumer]};
  scratchPublish(scratchPath.join(context.directory, 'leases', leaseId + '.json'), lease, true);
  return {context, lease, borrowed: true};
  }, 1000);
  scratchAssert(ownership);
  return ownership;
}
async function scratchReleaseReady(ownership) {
  const deadline = Date.now() + 3500;
  while (!scratchRelease(ownership)) {
    if (Date.now() >= deadline) return false;
    await scratchDelay(25);
  }
  return true;
}

function scratchAssociate(environment, threadId) {
  const context = scratchContext(environment);
  scratchAssert(context && scratchId(threadId));
  const lock = scratchPath.join(context.directory, '.association-lock'), deadline = Date.now() + 1000;
  while (true) {
    try {scratchFs.mkdirSync(lock, {mode: 0o700}); break;} catch (e) {
      if (e.code !== 'EEXIST' || Date.now() >= deadline) throw e;
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 10);
    }
  }
  try {
    const marker = scratchJson(context.markerPath);
    scratchAssert(marker.ownerKind === 'cli-session');
    if (!marker.providerThreads.some(value => value.provider === 'codex' && value.threadId === threadId)) {
      const updated={...marker,providerThreads:[...marker.providerThreads,{provider:'codex',threadId}]};
      scratchPublish(context.markerPath,updated);
    }
  } finally {scratchFs.rmdirSync(lock);}
}
const scratchDelay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

return { scratchContext, scratchEnvironment, scratchDiscoveryEnvironment, scratchAllocate, scratchBorrow, scratchRelease, scratchReleaseReady, scratchRecoveryCommand, scratchAssociate, scratchJson, scratchSocketPath };
}
export const { scratchContext, scratchEnvironment, scratchDiscoveryEnvironment, scratchAllocate, scratchBorrow, scratchRelease, scratchReleaseReady, scratchRecoveryCommand, scratchAssociate, scratchJson, scratchSocketPath } = createScratchOwner();
