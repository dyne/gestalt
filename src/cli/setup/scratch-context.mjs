// Current v1/v2 validation and existing borrower publication/release fences.
import * as scratchFs from 'node:fs';
import scratchPath from 'node:path';
import * as scratchCrypto from 'node:crypto';
const scratchVariable = 'GESTALT_SESSION_TMP_CONTEXT';
const scratchNames = [scratchVariable, 'TMPDIR', 'TMP', 'TEMP'];

const scratchId = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/.test(value);
const scratchUuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
function scratchAssert(condition) { if (!condition) throw Error('SESSION_SCRATCH_INVALID_CONTEXT'); }

function scratchCanonical(directory) {
  scratchAssert(typeof directory === 'string' && scratchPath.isAbsolute(directory) &&
    !/[\x00-\x1f\x7f]/.test(directory) && scratchPath.resolve(directory) === directory);
  let current = scratchPath.parse(directory).root;
  for (const component of directory.slice(current.length).split(scratchPath.sep).filter(Boolean)) {
    current = scratchPath.join(current, component);
    const info = scratchFs.lstatSync(current);
    scratchAssert(info.isDirectory() && !info.isSymbolicLink());
  }
  scratchAssert(scratchFs.realpathSync(directory) === directory);
}

function scratchPrivate(directory) {
  scratchCanonical(directory);
  const info = scratchFs.lstatSync(directory);
  scratchAssert((!process.getuid || info.uid === process.getuid()) &&
    (process.platform === 'win32' || (info.mode & 0o777) === 0o700));
}

function scratchJson(file) {
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
export { scratchEnvironment };

function scratchWorkspacePublication(workspace,publish) {
  scratchCanonical(workspace);
  const path=scratchPath.join(workspace,'.gestalt-scratch-lock');
  let claimed=false;
  for(let attempt=0;attempt<100;attempt++) {
    try {scratchFs.mkdirSync(path,{mode:0o700});claimed=true;break;}
    catch(error){if(error.code!=='EEXIST')throw error;Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);}
  }
  if(!claimed)throw Error('SESSION_SCRATCH_BUSY');
  scratchPrivate(path);
  const fd=scratchFs.openSync(path,scratchFs.constants.O_RDONLY|scratchFs.constants.O_DIRECTORY|scratchFs.constants.O_NOFOLLOW);
  const held=scratchFs.fstatSync(fd,{bigint:true});
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
    } finally {scratchFs.closeSync(fd);}
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

function scratchPublish(file, value, initial = false) {
  const staged = initial ? file : scratchPath.join(scratchPath.dirname(file), '.' + scratchCrypto.randomUUID() + '.json');
  const fd = scratchFs.openSync(staged, 'wx', 0o600);
  try { scratchFs.writeFileSync(fd, JSON.stringify(value) + '\n'); scratchFs.fsyncSync(fd); }
  finally { scratchFs.closeSync(fd); }
  if (!initial) scratchFs.renameSync(staged, file);
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

export function borrowSetupScratch(environment) {
  const context = scratchContext(environment);
  if (!context) return null;
  if (process.platform === "win32") throw new Error("native setup borrower publication requires the portable Windows scratch broker; no subprocess was started");
  return scratchBorrow(context, "manager-setup");
}
export async function releaseSetupScratch(ownership) {
  const deadline = Date.now() + 3500;
  while (!scratchRelease(ownership)) {
    if (Date.now() >= deadline) return false;
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  return true;
}
