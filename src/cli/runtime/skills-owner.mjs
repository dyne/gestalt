// Host-side extraction of public/gestalt's collection transaction. Package contents remain data.
import * as fs from 'node:fs';
import path from 'node:path';
import * as crypto from 'node:crypto';

export async function manageSkills(context, operation, outputMode = 'text', recovery = null) {
const process = { env: context.environment, pid: globalThis.process.pid };
const selectedHome = context.paths.codexHome, canonicalSource = 'https://github.com/dyne/gestalt-skills', defaultRef = 'main';
let writing = Promise.resolve();
const console = { log: value => { writing = writing.then(() => context.streams.stdout(Buffer.from(String(value) + '\n'))); } };
const overridden = Object.hasOwn(process.env, 'GESTALT_SKILLS_SOURCE');
const source = overridden ? process.env.GESTALT_SKILLS_SOURCE : canonicalSource;
const ref = process.env.GESTALT_SKILLS_REF ?? defaultRef;
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const fail = message => { throw new Error(message); };
const exists = file => { try { return fs.lstatSync(file); } catch (e) { if (e.code === 'ENOENT') return null; throw e; } };
const json = file => JSON.parse(fs.readFileSync(file, 'utf8'));
let home, root, discovery, lock, owner, stage;
let held = false, preserveRecovery = false;
async function checkpoint() {
  await new Promise(resolve => setImmediate(resolve));
  context.check();
}
function plainDirectory(file) {
  const info = exists(file);
  if (info && (!info.isDirectory() || info.isSymbolicLink())) fail(`expected a directory without links: ${file}`);
  return info;
}
function plainFile(file) {
  const info = exists(file);
  if (!info?.isFile() || info.isSymbolicLink() || info.nlink !== 1) fail(`expected an ordinary package file: ${file}`);
}
function canonicalHome(file) {
  if (!path.isAbsolute(file) || file === path.parse(file).root || path.normalize(file) !== file || /[\x00-\x1f\x7f]/.test(file)) fail('CODEX_HOME must be an absolute normalized directory');
  let ancestor = file, suffix = [];
  while (!exists(ancestor)) { suffix.unshift(path.basename(ancestor)); ancestor = path.dirname(ancestor); }
  if (!fs.statSync(ancestor).isDirectory()) fail('CODEX_HOME ancestor is not a directory');
  const resolved = path.join(fs.realpathSync(ancestor), ...suffix);
  if (resolved === path.parse(resolved).root || resolved === fs.realpathSync(context.paths.userHome)) fail('refusing resolved CODEX_HOME: root directory or HOME');
  return resolved;
}
function relativeFile(file) {
  return typeof file === 'string' && file.length <= 1024 && !/[\x00-\x1f\x7f\\]/.test(file)
    && !path.posix.isAbsolute(file) && file.split('/').every(p => p && p !== '.' && p !== '..');
}
function exactFields(value, fields) {
  if (!value || typeof value !== 'object' || Array.isArray(value)
      || !same(Object.keys(value).sort(), [...fields].sort())) fail(`invalid manifest object fields; expected ${fields.join(', ')}`);
}
function parseManifest(bytes) {
  // JSON.parse alone discards duplicate keys. Walk the already syntax-checked
  // document, comparing decoded keys in each object (including escaped aliases).
  const text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
  const result = JSON.parse(text);
  let offset = 0;
  const space = () => { while (offset < text.length && /[\t\n\r ]/.test(text[offset])) offset++; };
  function string() {
    const start = offset++;
    while (text[offset] !== '"') { if (text[offset] === '\\') offset++; offset++; }
    return JSON.parse(text.slice(start, ++offset));
  }
  function value(depth) {
    if (depth > 64) fail('manifest nesting exceeds supported depth');
    space();
    if (text[offset] === '{') {
      offset++; space(); const keys = new Set();
      while (text[offset] !== '}') {
        const key = string();
        if (keys.has(key)) fail(`duplicate JSON key in manifest: ${key}`);
        keys.add(key); space(); offset++; space();
        const start = offset; value(depth + 1);
        if (depth === 0 && key === 'schemaVersion' && text.slice(start, offset) !== '1') fail('manifest schemaVersion must be integer 1');
        space();
        if (text[offset] !== ',') break;
        offset++; space();
      }
      offset++;
    } else if (text[offset] === '[') {
      offset++; space();
      while (text[offset] !== ']') { value(depth + 1); space(); if (text[offset] !== ',') break; offset++; }
      offset++;
    } else if (text[offset] === '"') string();
    else while (offset < text.length && !/[,\]}\s]/.test(text[offset])) offset++;
  }
  value(0);
  return result;
}
function validateManifest(manifest) {
  exactFields(manifest, ['schemaVersion', 'repository', 'source', 'version', 'skills']);
  if (manifest.schemaVersion !== 1 || manifest.repository !== 'dyne/gestalt-skills' || manifest.source !== canonicalSource
      || typeof manifest.version !== 'string' || !/^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$/.test(manifest.version)
      || !Array.isArray(manifest.skills) || !manifest.skills.length || manifest.skills.length > 256) fail('unsupported collection manifest');
  const names = new Set(), files = new Set();
  let previousName;
  for (const skill of manifest.skills) {
    exactFields(skill, ['name', 'entry', 'files']);
    if (typeof skill.name !== 'string' || skill.name.length >= 64 || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(skill.name) || names.has(skill.name)
        || skill.entry !== `skills/${skill.name}/SKILL.md` || !Array.isArray(skill.files) || !skill.files.length) fail('invalid or duplicate skill in manifest');
    if (previousName !== undefined && previousName >= skill.name) fail('manifest skills must be sorted by name');
    names.add(skill.name); previousName = skill.name;
    let previousPath;
    for (const file of skill.files) {
      exactFields(file, ['path', 'sha256']);
      if (!relativeFile(file.path) || !file.path.startsWith(`skills/${skill.name}/`) || files.has(file.path)
          || typeof file.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(file.sha256)) fail('invalid, escaping or duplicate manifest file');
      if (previousPath !== undefined && previousPath >= file.path) fail('manifest files must be sorted by path');
      previousPath = file.path;
      files.add(file.path);
      if (files.size > 4096) fail('collection has too many files');
    }
    if (!skill.files.some(file => file.path === skill.entry)) fail('skill entry is absent from its file inventory');
  }
  return files;
}
function treeFiles(directory, prefix = '', directories = []) {
  plainDirectory(directory);
  const files = [];
  for (const name of fs.readdirSync(directory)) {
    const relative = prefix ? `${prefix}/${name}` : name;
    const file = path.join(directory, name), info = fs.lstatSync(file);
    if (info.isDirectory() && !info.isSymbolicLink()) {
      directories.push(relative); files.push(...treeFiles(file, relative, directories));
    }
    else { plainFile(file); files.push(relative); }
  }
  return files.sort();
}
function same(a, b) { return JSON.stringify(a) === JSON.stringify(b); }
function inspectInstalled() {
  if (!plainDirectory(root)) return null;
  plainFile(path.join(root, 'manifest.json')); plainFile(path.join(root, 'receipt.json'));
  const manifestBytes = fs.readFileSync(path.join(root, 'manifest.json'));
  const manifest = parseManifest(manifestBytes), inventory = validateManifest(manifest);
  const receipt = json(path.join(root, 'receipt.json'));
  if (receipt?.schemaVersion !== 1 || receipt.repository !== manifest.repository || receipt.managedRoot !== root
      || receipt.discoveryRoot !== discovery || receipt.manifestSha256 !== hash(manifestBytes)
      || receipt.version !== manifest.version || !same(receipt.skills, manifest.skills)
      || !/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(receipt.revision)
      || typeof receipt.source !== 'string' || typeof receipt.verifiedSource !== 'boolean'
      || (receipt.verifiedSource && receipt.source !== canonicalSource)
      || !Array.isArray(receipt.packageFiles) || receipt.packageFiles.length !== 1
      || receipt.packageFiles[0].path !== 'LICENSE' || !/^[a-f0-9]{64}$/.test(receipt.packageFiles[0].sha256)) fail('collection receipt is missing, damaged or belongs to another installation; preserve it and inspect before retrying');
  const expected = ['LICENSE', 'manifest.json', 'receipt.json', ...inventory].sort();
  const directories = [], expectedDirectories = new Set();
  for (const file of inventory) {
    let parent = path.posix.dirname(file);
    while (parent !== '.') { expectedDirectories.add(parent); parent = path.posix.dirname(parent); }
  }
  if (!same(treeFiles(root, '', directories), expected) || !same(directories.sort(), [...expectedDirectories].sort())) fail('managed collection contains local or missing files; preserve local changes before updating');
  for (const file of [...manifest.skills.flatMap(skill => skill.files), ...receipt.packageFiles]) {
    if (hash(fs.readFileSync(path.join(root, file.path))) !== file.sha256) fail(`managed collection has local changes: ${file.path}; preserve them before updating`);
  }
  return { manifest, receipt };
}
function target(name) { return path.join(root, 'skills', name); }
function linkValue(name) {
  const file = path.join(discovery, name), info = exists(file);
  if (!info) return null;
  if (!info.isSymbolicLink()) fail(`skill collision: ${file}; move the unrelated entry before retrying`);
  return fs.readlinkSync(file);
}
function writeOwner() {
  const temp = path.join(lock, 'owner.tmp');
  fs.writeFileSync(temp, JSON.stringify(owner) + '\n', { mode: 0o600 });
  fs.renameSync(temp, path.join(lock, 'owner.json'));
}
function ownsCandidate() {
  try { return json(path.join(root, 'receipt.json')).transactionId === owner.token; } catch { return false; }
}
// The journal lives under the lock and precedes any visible mutation. Recovery
// handles process termination too; no stale PID is ever signalled or stolen.
function rollback() {
  if (!owner.journal) return;
  const backup = path.join(owner.stage, 'previous');
  if (exists(backup)) {
    if (exists(root)) {
      if (!ownsCandidate()) fail('recovery found unexpected collection content; preserved transaction for inspection');
      fs.rmSync(root, { recursive: true });
    }
    fs.renameSync(backup, root);
  } else if (!owner.journal.original && exists(root)) {
    if (!ownsCandidate()) fail('recovery found an unrelated collection; preserved transaction for inspection');
    fs.rmSync(root, { recursive: true });
  }
  for (const { name, before } of owner.journal.links) {
    const current = linkValue(name);
    if (current === before) continue;
    if (current !== null && current !== target(name)) fail(`recovery found a changed discovery entry: ${name}`);
    const file = path.join(discovery, name);
    if (current !== null) fs.unlinkSync(file);
    if (before !== null) fs.symlinkSync(before, file, globalThis.process.platform === 'win32' ? 'junction' : 'dir');
  }
}
function validateOwner(record) {
  if (record?.schemaVersion !== 1 || !Number.isSafeInteger(record.pid) || record.pid < 1
      || !/^[a-f0-9]{48}$/.test(record.token) || record.home !== home) fail('skills lock has unknown ownership; inspect .gestalt-skills.lock before retrying');
  if (record.stage !== null && (typeof record.stage !== 'string' || path.dirname(record.stage) !== home
      || !/^\.gestalt-skills-stage-[A-Za-z0-9]+$/.test(path.basename(record.stage)))) fail('skills recovery path is invalid');
  if (record.stage !== null) plainDirectory(record.stage);
  if (record.journal !== null && (!record.stage || typeof record.journal.original !== 'boolean'
      || typeof record.committed !== 'boolean' || !Array.isArray(record.journal.links)
      || record.journal.links.some(link => !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(link.name)
        || (link.before !== null && link.before !== target(link.name))))) fail('skills recovery journal is invalid');
}
function releaseLock() {
  const current = json(path.join(lock, 'owner.json'));
  if (current.token !== owner.token) fail('skills lock ownership changed; preserving recovery state');
  if (owner.stage) fs.rmSync(owner.stage, { recursive: true, force: true });
  fs.rmSync(lock, { recursive: true });
  held = false;
}
function acquireLock() {
  const transfer = path.join(home, '.gestalt-skills-transfer.lock');
  if (exists(transfer)) fail('a skills transfer is active or retained; inspect its transaction before maintenance');
  try { fs.mkdirSync(lock, { mode: 0o700 }); }
  catch (e) {
    if (e.code !== 'EEXIST') throw e;
    plainDirectory(lock); plainFile(path.join(lock, 'owner.json'));
    const previous = json(path.join(lock, 'owner.json'));
    validateOwner(previous);
    fail('skills transaction is retained; use gestalt skills inspect-recovery, stop every user of this owner, then explicit confirmed recovery; PID or elapsed time cannot release ownership');
  }
  held = true;
  owner = { schemaVersion: 1, pid: process.pid, token: crypto.randomBytes(24).toString('hex'), home, stage: null, journal: null, committed: false };
  writeOwner();
  if (exists(transfer)) { releaseLock(); fail('a skills transfer claimed this collection; retry after it finishes'); }
}
function loadCollectionPaths() {
  home = canonicalHome(selectedHome);
  root = path.join(home, 'gestalt-skills'); discovery = path.join(home, 'skills'); lock = path.join(home, '.gestalt-skills.lock');
  plainDirectory(root); plainDirectory(discovery);
}
function status() {
  const report = { schemaVersion: 1, status: 'missing', integrity: 'missing', version: null, revision: null,
    managedRoot: null, discoveryRoot: null, verifiedSource: false, recoveryCommand: 'gestalt skills install' };
  try {
    loadCollectionPaths(); report.managedRoot = root; report.discoveryRoot = discovery;
    if (exists(root)) {
      plainFile(path.join(root, 'receipt.json'));
      const header = json(path.join(root, 'receipt.json'));
      if (typeof header.version === 'string' && header.version.length < 128 && /^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$/.test(header.version)) report.version = header.version;
      if (typeof header.revision === 'string' && /^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(header.revision)) report.revision = header.revision;
    }
    const installed = inspectInstalled();
    if (installed) {
      report.version = installed.receipt.version; report.revision = installed.receipt.revision;
      report.verifiedSource = installed.receipt.verifiedSource;
      for (const { name } of installed.manifest.skills) if (linkValue(name) !== target(name)) fail(`missing or changed discovery link: ${name}`);
      report.status = 'ready'; report.integrity = 'ok'; report.recoveryCommand = null;
    }
    if (exists(lock)) {
      report.status = 'recovery-required'; report.recoveryCommand = 'gestalt skills update';
      plainDirectory(lock); plainFile(path.join(lock, 'owner.json'));
      const record = json(path.join(lock, 'owner.json')); validateOwner(record);
      report.reason = 'retained transaction; lifetime absence is unknown; stop all users before explicit confirmed recovery';
      report.recoveryCommand = 'gestalt skills inspect-recovery';
    }
  } catch (error) {
    report.status = 'damaged'; report.integrity = 'invalid'; report.verifiedSource = false;
    report.recoveryCommand = 'gestalt skills update'; report.reason = error.message.slice(0, 512);
  }
  if (outputMode === 'version') {
    const value = report.status === 'ready' ? report.version
      : report.status === 'missing' ? 'NOT INSTALLED (optional)'
      : `${report.status.toUpperCase()} (see gestalt skills status)`;
    console.log('Gestalt Skills'.padEnd(24) + value);
    return;
  }
  if (outputMode === 'json') console.log(JSON.stringify(report));
  else {
    console.log(`Gestalt skills           ${report.status.toUpperCase()}${report.version ? ` (${report.version}; ${report.revision})` : ''}`);
    console.log(`Skills integrity         ${report.integrity}${report.status === 'ready' && !report.verifiedSource ? ' (source override; unverified)' : ''}`);
    if (report.reason) console.log(`Skills detail            ${report.reason}`);
    if (report.recoveryCommand) console.log(`Skills recovery          ${report.recoveryCommand}${report.status === 'damaged' ? ' (preserve local changes and resolve the reported conflict first)' : ''}`);
  }
  if (report.status !== 'ready') process.exitCode = 1;
}
async function git(args, limit = 16 * 1024 * 1024) {
  await checkpoint();
  const env = { ...context.environment, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: globalThis.process.platform === 'win32' ? 'NUL' : '/dev/null', GIT_TERMINAL_PROMPT: '0' };
  for (const key of Object.keys(env)) if (/^GIT_/.test(key) && !['GIT_CONFIG_NOSYSTEM', 'GIT_CONFIG_GLOBAL', 'GIT_TERMINAL_PROMPT'].includes(key)) delete env[key];
  const bytes = await context.run('git', ['-c', 'core.hooksPath=' + path.join(stage, 'no-hooks'), '-c', 'maintenance.auto=false', '-c', 'gc.auto=0', ...args],
    { capture: true, binary: true, environment: env, timeoutMs: 60000 });
  if (bytes.length > limit) fail('collection data exceeds supported size');
  return bytes;
}
async function install() {
  if (typeof source !== 'string' || !source || /[\x00-\x1f\x7f]/.test(source)) fail('invalid collection source');
  if (!path.isAbsolute(source)) {
    let url; try { url = new URL(source); } catch { fail('collection source must be HTTPS or an absolute local test repository'); }
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) fail('collection source must be HTTPS without credentials, query or fragment');
  }
  if (typeof ref !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(ref) || ref.includes('..')) fail('invalid collection revision or ref');
  loadCollectionPaths();
  fs.mkdirSync(home, { recursive: true });
  acquireLock();
  try {
    const previous = inspectInstalled();
    stage = fs.mkdtempSync(path.join(home, '.gestalt-skills-stage-'));
    fs.chmodSync(stage, 0o700); owner.stage = stage; writeOwner();
    const repository = path.join(stage, 'repository'), candidate = path.join(stage, 'candidate');
    await git(['init', '--bare', '--template=', repository]);
    const inRepo = args => ['--git-dir', repository, ...args];
    await git(inRepo(['-c', 'protocol.file.allow=always', 'fetch', '--depth=1', '--no-tags', '--no-recurse-submodules', '--', source, ref]));
    const revision = (await git(inRepo(['rev-parse', '--verify', 'FETCH_HEAD^{commit}']))).toString().trim();
    if (!/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(revision)) fail('download did not resolve to an immutable revision');
    const tree = (await git(inRepo(['ls-tree', '-r', '-z', revision]))).toString('utf8').split('\0').filter(Boolean);
    const blobs = new Map();
    for (const entry of tree) {
      const match = /^(100644|100755) blob ([a-f0-9]+)\t(.+)$/.exec(entry);
      if (!match) {
        const name = entry.slice(entry.indexOf('\t') + 1);
        if (name === 'manifest.json' || name === 'LICENSE' || name.startsWith('skills/')) fail('collection contains a symbolic link or unsupported file');
        continue;
      }
      blobs.set(match[3], { oid: match[2], mode: match[1] });
    }
    async function blob(name) {
      if (!blobs.has(name)) fail(`package is missing ${name}`);
      return await git(inRepo(['cat-file', 'blob', blobs.get(name).oid]), 4 * 1024 * 1024);
    }
    const manifestBytes = await blob('manifest.json');
    const manifest = parseManifest(manifestBytes), inventory = validateManifest(manifest);
    if (!same([...blobs.keys()].filter(name => name.startsWith('skills/')).sort(), [...inventory].sort())) fail('manifest does not cover every bundled skill file');
    fs.mkdirSync(candidate);
    let totalBytes = 0;
    for (const file of manifest.skills.flatMap(skill => skill.files)) {
      const bytes = await blob(file.path);
      if (hash(bytes) !== file.sha256) fail(`package digest mismatch: ${file.path}`);
      totalBytes += bytes.length;
      if (totalBytes > 64 * 1024 * 1024) fail('collection payload exceeds supported size');
      const destination = path.join(candidate, file.path);
      fs.mkdirSync(path.dirname(destination), { recursive: true });
      fs.writeFileSync(destination, bytes, { mode: blobs.get(file.path).mode === '100755' ? 0o755 : 0o644 });
    }
    const license = await blob('LICENSE');
    fs.writeFileSync(path.join(candidate, 'LICENSE'), license);
    fs.writeFileSync(path.join(candidate, 'manifest.json'), manifestBytes);
    const receipt = {
      schemaVersion: 1, repository: manifest.repository, source, revision, version: manifest.version,
      manifestSha256: hash(manifestBytes), managedRoot: root, discoveryRoot: discovery,
      verifiedSource: !overridden && source === canonicalSource,
      skills: manifest.skills, packageFiles: [{ path: 'LICENSE', sha256: hash(license) }],
      installedAt: new Date().toISOString(), transactionId: owner.token,
    };
    fs.writeFileSync(path.join(candidate, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n', { mode: 0o600 });
    const oldNames = new Set(previous?.manifest.skills.map(skill => skill.name) ?? []);
    const newNames = new Set(manifest.skills.map(skill => skill.name));
    const links = [...new Set([...oldNames, ...newNames])].sort().map(name => {
      const before = linkValue(name);
      if (before !== null && (!oldNames.has(name) || before !== target(name))) fail(`skill collision: ${path.join(discovery, name)}; preserve the existing entry before retrying`);
      return { name, before };
    });
    // Recheck after downloading so edits made while the network was busy cannot
    // be overwritten. The collection lock serializes cooperating installers.
    const current = inspectInstalled();
    if (!same(current, previous)) fail('managed collection changed during download; retry');
    if (previous && previous.receipt.revision === revision && previous.receipt.manifestSha256 === receipt.manifestSha256
        && previous.receipt.source === source && previous.receipt.verifiedSource === receipt.verifiedSource
        && links.every(link => link.before === target(link.name))) {
      console.log(`Gestalt skills ${manifest.version} (${revision}): unchanged`);
      return;
    }
    plainDirectory(discovery); fs.mkdirSync(discovery, { recursive: true });
    owner.journal = { original: Boolean(previous), links }; writeOwner();
    await checkpoint();
    if (previous) fs.renameSync(root, path.join(stage, 'previous'));
    await checkpoint();
    fs.renameSync(candidate, root);
    await checkpoint();
    for (const { name, before } of links) {
      if (linkValue(name) !== before) fail(`discovery entry changed during update: ${name}`);
      if (newNames.has(name)) { if (before === null) fs.symlinkSync(target(name), path.join(discovery, name), globalThis.process.platform === 'win32' ? 'junction' : 'dir'); }
      else if (before !== null) fs.unlinkSync(path.join(discovery, name));
      await checkpoint();
    }
    owner.committed = true; writeOwner();
    console.log(`Gestalt skills ${manifest.version} (${revision}): installed${receipt.verifiedSource ? '' : ' (source override; unverified)'}`);
  } catch (error) {
    if (!context.canRollback()) { preserveRecovery = true; throw error; }
    try { if (!owner.committed) rollback(); }
    catch (recoveryError) { preserveRecovery = true; throw new Error(`${error.message}; rollback needs inspection: ${recoveryError.message}`); }
    throw error;
  } finally {
    if (held && !preserveRecovery && context.canRollback()) releaseLock();
  }
}
try {
  if (operation === 'status') {
    status(); await writing;
    return process.exitCode || 0;
  }
  if (operation === 'inspect-recovery' || operation === 'recover') {
    loadCollectionPaths(); plainDirectory(lock); plainFile(path.join(lock, 'owner.json'));
    owner = json(path.join(lock, 'owner.json')); validateOwner(owner);
    const inventory = owner.stage ? treeFiles(owner.stage).map(file => [file, hash(fs.readFileSync(path.join(owner.stage, file)))]) : [];
    const token = hash(Buffer.from(JSON.stringify({ owner, inventory })));
    if (operation === 'inspect-recovery') {
      console.log(JSON.stringify({ schemaVersion: 1, status: 'confirmationRequired', token }));
    } else {
      if (!recovery?.confirmed || recovery.token !== token) fail('explicit quiescence confirmation and current recovery token are required');
      // Confirmation is an operator override, never a PID/timeout absence proof.
      if (!owner.committed) rollback();
      releaseLock(); console.log(JSON.stringify({ schemaVersion: 1, status: 'recovered' }));
    }
  } else {
    if (!['install', 'update'].includes(operation)) fail('unsupported skills operation');
    await context.prepareHomes(); await install();
  }
  await writing; return 0;
} finally { await writing; }
}
