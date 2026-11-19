import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, openSync } from 'node:fs';
import { copyFile, lstat, mkdir, mkdtemp, readFile, realpath, rename, rm, symlink, writeFile } from 'node:fs/promises';
import { release, tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { digest, pinnedCodex } from '../fixtures/native-runtime/codex.mjs';
import { exactPattern, groups, lanes } from './selectors.mjs';
import { checkLinuxPackage } from './package-check.mjs';

const now = () => new Date().toISOString();
const manager = path.resolve(import.meta.dirname, '../..');
const [agentsPath, destination] = process.argv.slice(2);
const evidence = path.resolve(destination ?? '');
assert.equal(path.relative(manager, evidence), path.join('native-evidence', 'unix-setup-correction'));
await mkdir(evidence, { recursive: true, mode: 0o700 });
const inputs = JSON.parse(await readFile(path.join(import.meta.dirname, 'inputs.json'), 'utf8'));
const target = process.platform + '-' + process.arch;
const lane = process.env.SETUP_LANE ?? null;
const configuration = Object.hasOwn(lanes, lane) ? lanes[lane] : null;
const scheduled = configuration ? groups(lane, inputs.shellNames) : [];
const proof = {
  schemaVersion: 1, state: 'started', startedAt: now(), lane, target, scheduledRunner: process.env.SETUP_SCHEDULED_LABEL ?? null,
  node: process.version, abi: process.versions.modules, uid: process.getuid?.() ?? null, runnerImage: process.env.ImageVersion ?? null,
  runnerImageOS: process.env.ImageOS ?? null, kernel: release(),
  commands: [], groups: scheduled.map(group => ({ id: group.id, state: 'unexecuted', expected: group.names ?? group.count ?? null })),
  prerequisites: {}, failure: null, exclusions: [
    'interruption, controller crash and descendant lifetime groups', 'concurrency wait, restart ownership, forced stop and stress',
    'unfiltered native suites, Windows execution, actual npm installation lifecycle, public release and default remote marketplace',
  ],
};
let work;
let preserveTools = false;
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) =>
  ['PATH', 'LANG', 'LC_ALL', 'TZ', 'TERM', 'TMPDIR', 'TMP', 'TEMP', 'CI'].includes(key)));
const errorInfo = error => ({ name: error.name, message: error.message, stack: error.stack });

async function save(name, value) {
  const file = path.join(evidence, name), next = file + '.tmp';
  await writeFile(next, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
  await rename(next, file);
}

async function run(name, executable, argv, options = {}) {
  const cwd = options.cwd ?? manager;
  const timeout = options.timeout ?? 120_000;
  const stdout = path.join(evidence, name + '.stdout'), stderr = path.join(evidence, name + '.stderr');
  const out = openSync(stdout, 'wx', 0o600), err = openSync(stderr, 'wx', 0o600);
  const startedAt = now();
  let result;
  try {
    result = spawnSync(executable, argv, { cwd, env: options.env ?? env, timeout, shell: false, stdio: ['ignore', out, err] });
  } finally { closeSync(out); closeSync(err); }
  const receipt = { name, argv: [executable, ...argv], cwd, timeout, startedAt, observedAt: now(), exit: result.status,
    signal: result.signal, error: result.error?.message ?? null, stdout: await digest(stdout), stderr: await digest(stderr) };
  proof.commands.push(receipt);
  await save('commands.json', proof.commands);
  return { receipt, text: (await readFile(stdout, 'utf8')).trim() };
}

async function command(name, executable, argv, options) {
  const { receipt, text } = await run(name, executable, argv, options);
  assert.equal(receipt.exit, 0, name + ' failed: ' + (receipt.error ?? receipt.stderr.path));
  return text;
}

async function inventory(root, label) {
  const rows = [];
  const source = await command(label + '-git-tree', 'git', ['--no-optional-locks', 'ls-tree', '-rz', 'HEAD'], { cwd: root });
  for (const line of source.split('\0').filter(Boolean)) {
    const match = /^(100644|100755) blob ([0-9a-f]{40})\t(.+)$/.exec(line);
    assert.ok(match, label + ' expected a regular tracked file');
    const [, mode, blob, name] = match;
    const file = path.join(root, name), stat = await lstat(file), bytes = await readFile(file);
    assert.ok(stat.isFile() && !stat.isSymbolicLink());
    const content = createHash('sha1').update('blob ' + bytes.length + '\0').update(bytes).digest('hex');
    assert.equal(content, blob, name);
    assert.equal(Boolean(stat.mode & 0o111), mode === '100755', name);
    rows.push({ name, mode, blob, bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  await save(label + '-inventory.json', rows);
  return rows;
}

async function frozenToolchain() {
  const response = await fetch(inputs.npm.url, { redirect: 'error', signal: AbortSignal.timeout(60_000) });
  assert.ok(response.ok && response.body);
  const chunks = [];
  let bytes = 0;
  for await (const chunk of response.body) {
    bytes += chunk.byteLength;
    assert.ok(bytes <= inputs.npm.bytes, 'Pinned npm archive exceeds recorded size');
    chunks.push(chunk);
  }
  assert.equal(bytes, inputs.npm.bytes);
  const archive = path.join(work, 'npm.tgz'), contents = Buffer.concat(chunks);
  assert.equal(createHash('sha512').update(contents).digest('hex'), inputs.npm.sha512);
  await writeFile(archive, contents, { mode: 0o600 });
  const npmRoot = path.join(work, 'npm');
  await mkdir(npmRoot, { mode: 0o700 });
  await command('extract-pinned-npm', '/usr/bin/tar', ['-xzf', archive, '-C', npmRoot]);
  const cli = path.join(npmRoot, 'package/bin/npm-cli.js');
  const bin = path.join(work, 'bin');
  await mkdir(bin, { mode: 0o700 });
  await symlink(cli, path.join(bin, 'npm'));
  if (lane === 'mac-bash') await symlink('/bin/bash', path.join(bin, 'bash'));
  const configuration = path.join(work, 'npmrc');
  await writeFile(configuration, '', { mode: 0o600 });
  const toolEnv = { ...env, HOME: work, PATH: [bin, path.dirname(process.execPath), env.PATH].join(path.delimiter),
    npm_config_cache: path.join(work, 'npm-cache'), npm_config_userconfig: configuration, npm_config_globalconfig: '/dev/null',
    npm_config_registry: 'https://registry.npmjs.org/', npm_config_update_notifier: 'false' };
  const version = await command('pinned-npm-version', process.execPath, [cli, '--version'], { env: toolEnv });
  assert.equal(version, inputs.npm.version);
  proof.npm = { version, source: inputs.npm.url, bytes, sha512: inputs.npm.sha512, cli: await digest(cli) };
  return { cli, bin, env: toolEnv };
}

async function validateModules(toolEnv) {
  const renderer = "import { realpathSync } from 'node:fs'; import { fileURLToPath } from 'node:url';\n" +
    "console.log(JSON.stringify(['esbuild','smol-toml','typescript','@types/node/package.json','@esbuild/'+process.platform+'-'+process.arch+'/package.json'].map(name=>({name,physical:realpathSync(fileURLToPath(import.meta.resolve(name)))}))));";
  const selected = JSON.parse(await command('lockfile-local-module-paths', process.execPath, ['--input-type=module', '-e', renderer], { env: toolEnv }));
  const modules = await realpath(path.join(manager, 'node_modules'));
  assert.equal(modules, path.join(manager, 'node_modules'));
  for (const entry of selected) assert.ok(entry.physical.startsWith(modules + path.sep), entry.name);
  proof.lockfileModules = selected;
}

async function prerequisite(name, action) {
  const row = proof.prerequisites[name] = { state: 'started', startedAt: now() };
  try { row.detail = await action(); row.state = 'pass'; }
  catch (error) { row.failure = errorInfo(error); row.state = 'fail'; }
  row.observedAt = now();
  await save('result.json', proof);
}

async function ancestry(agents) {
  const physical = await realpath(tmpdir()), chain = [];
  for (let directory = work;; directory = path.dirname(directory)) {
    const info = await lstat(directory);
    chain.push({ path: directory, uid: info.uid, mode: (info.mode & 0o7777).toString(8) });
    if (directory === path.dirname(directory)) break;
  }
  await save('physical-ancestors.json', { physical, chain, observedAt: now() });
  const { ensureManagedRoot } = await import(pathToFileURL(path.join(agents, 'scripts/setup-storage.mjs')));
  ensureManagedRoot(work, { create: false });
  return { admitted: true, physical, ownedProbe: work, chain };
}

async function inspectHost() {
  assert.ok(proof.runnerImage, 'GitHub runner image version must be recorded');
  const kernel = await command('kernel', '/usr/bin/uname', ['-srm']);
  if (target === 'darwin-arm64') {
    const version = await command('macos-release', '/usr/bin/sw_vers', []);
    return { kernel, osRelease: version };
  }
  const version = await readFile('/etc/os-release', 'utf8');
  await writeFile(path.join(evidence, 'os-release.txt'), version, { mode: 0o600 });
  const id = /^ID="?([^"\n]+)"?$/m.exec(version)?.[1];
  const number = /^VERSION_ID="?([^"\n]+)"?$/m.exec(version)?.[1];
  assert.equal(id, 'ubuntu');
  assert.equal(number, configuration.release);
  return { kernel, osRelease: version, release: number };
}

async function verifyBash(npm) {
  const shell = await command('bash-resolution', 'bash',
    ['-c', 'printf "%s\\n" "$BASH_VERSION"; command -v bash'], { env: npm.env });
  const [version, lookup] = shell.split('\n');
  assert.ok(version && path.isAbsolute(lookup));
  const physical = await realpath(lookup);
  if (lane === 'mac-bash') {
    assert.match(version, /^3\.2\./);
    assert.equal(lookup, path.join(npm.bin, 'bash'));
    assert.equal(physical, await realpath('/bin/bash'));
  }
  return { version, lookup, physical, executable: await digest(physical) };
}

async function managedHost() {
  const files = ['unprivileged_userns_clone', 'apparmor_restrict_unprivileged_userns'];
  const observed = {};
  for (const name of files)
    observed[name] = await readFile(path.join('/proc/sys/kernel', name), 'utf8').then(s => s.trim(), error => error.code === 'ENOENT' ? null : Promise.reject(error));
  await save('linux-namespace-policy.json', { observed, altered: false, observedAt: now() });
  assert.notEqual(observed.unprivileged_userns_clone, '0', 'Host disables user namespaces; ordinary managed tests did not execute');
  assert.notEqual(observed.apparmor_restrict_unprivileged_userns, '1',
    'Host advertises the namespace restriction that blocked the prior managed test group');
  return { observed, policyAltered: false, note: 'Static observations do not certify per-process namespace admission. Actual selected tests determine managed outcome.' };
}

function parseTap(text) {
  const cases = Array.from(text.matchAll(/^(ok|not ok) \d+ - (.*)$/gm), ([, kind, label]) => {
    const [name, skip] = label.split(/\s+#\s+SKIP\s*/i);
    return { name, state: skip === undefined ? kind === 'ok' ? 'pass' : 'fail' : 'skip', skipReason: skip ?? null };
  });
  const counts = Object.fromEntries(Array.from(text.matchAll(/^# (tests|pass|fail|skipped) (\d+)\s*$/gm), ([, kind, count]) => [kind, Number(count)]));
  return { cases, counts };
}

function verifyInstalledPublicReceipt(receipt, source, inputs) {
  assert.equal(receipt.backend, 'compiled-to-unix-bash');
  const sourceSha256 = createHash('sha256').update(source).digest('hex');
  assert.equal(sourceSha256, inputs.sourceDigests['public/gestalt']);
  assert.equal(receipt.previousBashSha256, sourceSha256);
  // The installed fixture serves a replacement copied from the same source
  // with a marker appended, so it can prove that replacement really occurred.
  const replacementSha256 = createHash('sha256').update(source)
    .update('\n# installed Unix Bash replacement fixture\n').digest('hex');
  assert.equal(receipt.replacementBashSha256, replacementSha256);
  assert.equal(receipt.provenance.manager.sha256, replacementSha256);
  assert.equal(receipt.agentsTree, inputs.agentsTree);
  assert.equal(receipt.agentsHead, inputs.agentsSnapshot);
  assert.equal(receipt.repeat, 'offline-success');
}

async function tests(definition, row, toolEnv, agents) {
  const groupEvidence = path.join(evidence, definition.id);
  await mkdir(groupEvidence, { mode: 0o700 });
  const args = ['--test', '--test-reporter=tap', '--test-concurrency=1',
    '--test-name-pattern=' + exactPattern(definition.names), definition.file];
  const execution = await run(definition.id, process.execPath, args, { timeout: definition.id.startsWith('installed-') ? 840_000 : 240_000,
    env: { ...toolEnv, GESTALT_NATIVE_SETUP_EVIDENCE: groupEvidence, GESTALT_NATIVE_AGENTS_SOURCE: agents } });
  row.command = execution.receipt.name;
  row.tap = parseTap(execution.text);
  assert.equal(execution.receipt.exit, 0, definition.id + ' failed; see raw TAP and stderr');
  const count = definition.names.length;
  assert.deepEqual(row.tap.counts, { tests: count, pass: count, fail: 0, skipped: 0 });
  assert.deepEqual(row.tap.cases.map(item => item.name).sort(), [...definition.names].sort());
  assert.ok(row.tap.cases.every(item => item.state === 'pass'));
  if (definition.id === 'mac-managed-composition') {
    const file = path.join(groupEvidence, 'compiled-setup-selection.json');
    const receipt = JSON.parse(await readFile(file, 'utf8'));
    assert.equal(receipt.complete, true);
    assert.deepEqual(receipt.observations.map(item => [item.phase, item.result.status]),
      [['install', 0], ['update-gestalt', 1], ['retained-state-verified', 0]]);
    row.phaseReceipt = await digest(file);
  }
  if (definition.id === 'installed-public') {
    const file = path.join(groupEvidence, 'unix-public-installed', 'installed-identity.json');
    const receipt = JSON.parse(await readFile(file, 'utf8'));
    verifyInstalledPublicReceipt(receipt, await readFile(path.join(manager, 'public/gestalt')), inputs);
    row.phaseReceipt = await digest(file);
  }
}

function cleanupDecision(work, state, groups, macManagedAuthority) {
  const retentionReasons = [];
  if (work && (state !== 'pass' || groups.some(row => row.state !== 'pass'))) {
    retentionReasons.push('failed-or-unexecuted');
  }
  if (work && macManagedAuthority) retentionReasons.push('macos-managed-authority');
  return { path: work ?? null, attempted: Boolean(work && !retentionReasons.length), removed: false,
    retainedForMacManagedAuthority: macManagedAuthority, retentionReasons };
}

try {
  assert.ok(configuration && scheduled.length > 0);
  assert.equal(target, configuration.target);
  assert.equal(target, process.env.SETUP_EXPECTED_TARGET);
  assert.equal(process.env.SETUP_SCHEDULED_LABEL, configuration.label);
  assert.equal(process.version, inputs.node);
  assert.notEqual(proof.uid, 0);
  assert.ok(!Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT'));
  proof.host = await inspectHost();
  const agents = await realpath(agentsPath);
  const head = await command('manager-head', 'git', ['rev-parse', 'HEAD']);
  assert.equal(head, process.env.GITHUB_SHA);
  const parent = await command('manager-parent', 'git', ['rev-parse', 'HEAD^']);
  assert.equal(parent, inputs.managerBase);
  const tree = await command('manager-tree', 'git', ['rev-parse', 'HEAD^{tree}']);
  const managerFiles = await inventory(manager, 'manager');
  for (const [name, sha256] of Object.entries(inputs.sourceDigests))
    assert.equal(managerFiles.find(row => row.name === name)?.sha256, sha256, name);
  const changed = (await command('manager-correction-paths', 'git', ['diff', '--name-only', parent, head])).split('\n').sort();
  assert.deepEqual(changed, [...inputs.sourcePaths, ...inputs.overlayPaths].sort());
  proof.manager = { head, parent, tree, files: managerFiles.length, correctionPaths: inputs.sourcePaths };
  const gitAgents = argv => command('agents-' + argv.at(-1).replaceAll(/[^a-zA-Z0-9]/g, ''), 'git', argv, { cwd: agents });
  assert.equal(await gitAgents(['rev-parse', 'HEAD']), inputs.agentsSnapshot);
  assert.equal(await gitAgents(['rev-parse', 'HEAD^{tree}']), inputs.agentsTree);
  assert.equal((await inventory(agents, 'agents')).length, 755);
  proof.agents = { head: inputs.agentsSnapshot, tree: inputs.agentsTree, count: 755, checkout: agents };

  work = await mkdtemp(path.join(await realpath(tmpdir()), 'gestalt-unix-setup-gate-'));
  const npm = await frozenToolchain();
  await command('lockfile-local-npm-ci', process.execPath, [npm.cli, 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], { env: npm.env, timeout: 180_000 });
  assert.equal((await digest(path.join(manager, 'package-lock.json'))).sha256, inputs.sourceDigests['package-lock.json']);
  await validateModules(npm.env);
  const requiredPrerequisites = new Set(scheduled.flatMap(group => group.prerequisites));
  if (requiredPrerequisites.has('bash')) await prerequisite('bash', () => verifyBash(npm));
  if (requiredPrerequisites.has('ancestry')) await prerequisite('ancestry', () => ancestry(agents));
  if (requiredPrerequisites.has('managed-host')) await prerequisite('managed-host', managedHost);
  if (requiredPrerequisites.has('codex')) await prerequisite('codex', async () => {
    const directory = path.join(work, 'pinned-codex');
    try {
      const result = await pinnedCodex(directory);
      const metadata = path.join(evidence, 'codex-artifact.json');
      await copyFile(path.join(directory, 'artifact.json'), metadata);
      await symlink(result.binary, path.join(npm.bin, 'codex'));
      return { metadata: await digest(metadata), binary: result.record.binary };
    } catch (error) {
      await copyFile(path.join(directory, 'download-http-failure.json'), path.join(evidence, 'codex-download-failure.json')).catch(copyError => {
        if (copyError.code !== 'ENOENT') throw copyError;
      });
      throw error;
    }
  });
  for (const [index, definition] of scheduled.entries()) {
    const row = proof.groups[index];
    const blocked = definition.prerequisites.filter(key => proof.prerequisites[key]?.state !== 'pass');
    if (blocked.length) {
      row.blockedBy = blocked;
      await save('result.json', proof);
      continue;
    }
    row.state = 'started'; row.startedAt = now();
    await save('result.json', proof);
    if (definition.id === 'mac-managed-composition') preserveTools = true;
    try {
      if (definition.id === 'linux-package') {
        row.package = await checkLinuxPackage({ manager, npmCli: npm.cli, inputs, head, destination: evidence, command, env: npm.env });
      } else await tests(definition, row, npm.env, agents);
      row.state = 'pass';
    } catch (error) { row.state = 'fail'; row.failure = errorInfo(error); }
    row.observedAt = now();
    await save('result.json', proof);
  }
  assert.ok(proof.groups.every(row => row.state === 'pass'), 'One or more required groups failed or did not execute');
  proof.state = 'pass';
} catch (error) {
  proof.state = 'fail';
  proof.failure = errorInfo(error);
  process.exitCode = 1;
} finally {
  proof.cleanup = cleanupDecision(work, proof.state, proof.groups, preserveTools);
  if (proof.cleanup.attempted) {
    try { await rm(work, { recursive: true }); proof.cleanup.removed = true; }
    catch (error) { proof.cleanup.error = errorInfo(error); proof.state = 'fail'; process.exitCode = 1; }
  }
  proof.finishedAt = now();
  await save('result.json', proof);
  if (proof.failure) console.error(proof.failure.message);
}
