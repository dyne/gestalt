// Current v1/v2 managed-context validation; no allocation or cleanup authority.
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

