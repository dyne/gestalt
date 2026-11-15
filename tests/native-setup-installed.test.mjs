/** Hosted integration: the compiled manager invokes the exact accepted Agents
 * Node owner and actual npm/Codex, in isolated homes. No live installation. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { buildCli } from '../scripts/build-cli.mjs';
import { buildNativePlatform } from '../scripts/build-native-platform.mjs';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { commandTarget, NativeExecutables } from '../src/cli/platform/executables.ts';
import { NativeOwnedProcesses } from '../src/cli/platform/processes.ts';

const root = path.resolve(import.meta.dirname, '..');
const evidence = process.env.GESTALT_NATIVE_SETUP_EVIDENCE;
const dependency = process.env.GESTALT_NATIVE_AGENTS_SOURCE;
if (!evidence || !dependency) throw new Error('native installed setup requires exact dependency and evidence paths');
await mkdir(evidence, { recursive: true });
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

test(`actual compiled manager provisions accepted native Agents ${process.platform === 'darwin' ? 'and retains managed unknown authority' : 'and repeats offline'}`, { timeout: 1_200_000 }, async t => {
  const base = await realpath(await mkdtemp(path.join(tmpdir(), 'gestalt core installed ü & ')));
  t.after(() => rm(base, { recursive: true, force: true }));
  const manager = await buildCli(root, path.join(base, 'gestalt.mjs'));
  const probe = await buildNativePlatform(root, path.join(base, 'native'));
  const ownershipHelper = await buildOwnedProcess(root, path.join(base, 'native'));
  await writeFile(path.join(evidence, 'installed-ownership.provenance.json'), await readFile(`${ownershipHelper}.provenance.json`));
  const home = path.join(base, 'home ü &'), codex = path.join(home, 'Codex ü &'), gestalt = path.join(home, 'Gestalt ü &');
  await mkdir(home, { mode: 0o700 });
  const environment = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CONTEXT_MODE_DIR: path.join(home, 'context-mode-data'),
    CODEX_HOME: codex, GESTALT_HOME: gestalt, GESTALT_MANAGER_DISTRIBUTION: 'npm', GESTALT_AGENTS_SOURCE: await realpath(dependency),
    GESTALT_AGENTS_SETUP_SHA256: digest(await readFile(path.join(dependency, 'gestalt-setup.mjs'))),
    GESTALT_AGENTS_SETUP_OWNER_SHA256: digest(await readFile(path.join(dependency, 'scripts/setup-agents.mjs'))),
    CONTEXT_MODE_PACKAGE_MANAGER: 'npm', npm_config_ignore_scripts: 'true', CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: base };
  for (const key of Object.keys(environment)) if (key.startsWith('GESTALT_MOBILE_ORG_PLAN_')) delete environment[key];
  const hostAncestors = [];
  for (let parent = home; ; parent = path.dirname(parent)) {
    const info = await stat(parent); hostAncestors.push({ path: parent, uid: info.uid, gid: info.gid, mode: info.mode & 0o7777 });
    if (parent === path.dirname(parent)) break;
  }
  await writeFile(path.join(evidence, 'host-setup-namespace.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    uid: process.getuid?.() ?? null, gid: process.getgid?.() ?? null, ancestors: hostAncestors, setupApiVersion: 1 }, null, 2));
  // Capture the actual filesystem UID view inside the same ownership backend.
  // This diagnostic never substitutes for the real accepted Agents setup below.
  const namespaceProbe = `const fs=require('node:fs'),path=require('node:path');const roots=process.argv.slice(1),ancestors=[];for(let p=path.dirname(roots[0]);;p=path.dirname(p)){try{const s=fs.statSync(p);ancestors.push({path:p,uid:s.uid,gid:s.gid,mode:s.mode&511});}catch(e){ancestors.push({path:p,error:e.code});}if(p===path.dirname(p))break;}let uidMap=null,gidMap=null;for(const [key,file] of [['uid','/proc/self/uid_map'],['gid','/proc/self/gid_map']])try{const value=fs.readFileSync(file,'utf8');if(key==='uid')uidMap=value;else gidMap=value;}catch{}console.log(JSON.stringify({platform:process.platform,uid:process.getuid?.()??null,gid:process.getgid?.()??null,uidMap,gidMap,ancestors}));`;
  const diagnosticOutput = [], diagnosticError = [];
  const diagnostic = await new NativeOwnedProcesses(ownershipHelper).launch(process.execPath,
    { argv: ['-e', namespaceProbe, codex, gestalt], cwd: base, environment },
    { stdin: (async function* () {})(), stdout: async bytes => { diagnosticOutput.push(Buffer.from(bytes)); }, stderr: async bytes => { diagnosticError.push(Buffer.from(bytes)); } });
  const diagnosticResult = await diagnostic.result, diagnosticClaim = await diagnostic.stop();
  await writeFile(path.join(evidence, 'owned-setup-namespace.json'), JSON.stringify({ observedAt: new Date().toISOString(), result: diagnosticResult, claim: diagnosticClaim,
    stdout: Buffer.concat(diagnosticOutput).toString('utf8'), stderr: Buffer.concat(diagnosticError).toString('utf8'), ownershipHelperSha256: digest(await readFile(ownershipHelper)) }, null, 2));
  const checks = [];
  const run = async (label, argv, extra = {}, blocked = false) => {
    const observedAt = new Date().toISOString();
    const result = spawnSync(process.execPath, [manager, ...argv], { env: { ...environment, ...extra }, cwd: base, encoding: 'utf8', timeout: 1_080_000, maxBuffer: 16 * 1024 * 1024, shell: false });
    const log = path.join(evidence, `${label}.log`), bytes = Buffer.from((result.stdout ?? '') + (result.stderr ?? ''));
    await writeFile(log, bytes);
    checks.push({ label, command: [process.execPath, manager, ...argv], cwd: base, status: result.status, signal: result.signal, error: result.error?.message ?? null,
      observedAt, log, sha256: digest(bytes) });
    await writeFile(path.join(evidence, 'installed-checks.json'), JSON.stringify(checks, null, 2));
    if (blocked) {
      assert.notEqual(result.status, 0); assert.equal(result.error, undefined);
      assert.match(result.stderr, /EEXIST|already running/);
    } else assert.equal(result.status, 0, `${label}: ${result.error?.message ?? ''}; see ${log}: ${(result.stderr ?? '').slice(-3000)}`);
  };
  await run('manager-agents-fresh', ['agents', 'install']);
  const outputs = ['config.toml', 'hooks.json', 'bin/context-mode-mcp.mjs', 'bin/context-mode-hook.mjs',
    ...(process.platform === 'darwin' ? [] : ['.gestalt-setup-provenance.json'])];
  const first = await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file)))));
  await run('manager-agents-repeat-offline', ['agents', 'update'], { npm_config_offline: 'true' }, process.platform === 'darwin');
  assert.deepEqual(await Promise.all(outputs.map(async file => digest(await readFile(path.join(codex, file))))), first);
  const prepared = JSON.parse(await readFile(path.join(gestalt, 'runtime/context-mode/.context-mode-prepared.json'), 'utf8'));
  assert.equal(prepared.platform, process.platform); assert.equal(prepared.arch, process.arch); assert.equal(prepared.nodeModulesAbi, process.versions.modules);
  let provenance = null, retained = null;
  if (process.platform === 'darwin') {
    const stages = (await readdir(codex)).filter(name => name.startsWith('.gestalt-setup.'));
    assert.equal(stages.length, 1);
    retained = JSON.parse(await readFile(path.join(codex, stages[0], 'ownership-unknown.json'), 'utf8'));
    assert.equal(retained.status, 'command-succeeded-cleanup-unknown');
    assert.equal(retained.delegated.mode, 'managed'); assert.equal(retained.delegated.ownership.kind, 'unknown');
    assert.ok(retained.delegated.retained.length > 0);
    assert.ok((await stat(path.join(codex, '.gestalt-manager-setup.lock'))).isFile());
    assert.ok((await stat(path.join(codex, '.gestalt-setup.lock'))).isDirectory());
  } else {
    provenance = JSON.parse(await readFile(path.join(codex, '.gestalt-setup-provenance.json'), 'utf8'));
    assert.equal(provenance.manager.sha256, digest(await readFile(manager)));
    assert.equal(provenance.delegatedSetupSha256, environment.GESTALT_AGENTS_SETUP_SHA256);
    assert.equal(provenance.delegatedApi.apiVersion, 1); assert.equal(provenance.delegatedApi.mode, 'managed');
    assert.equal(provenance.delegatedApi.ownerSha256, environment.GESTALT_AGENTS_SETUP_OWNER_SHA256);
    assert.equal(provenance.delegatedApi.ownership, 'released');
  }
  const codexExecutable = await new NativeExecutables().resolve('codex', environment);
  assert.ok(codexExecutable);
  const target = await commandTarget(codexExecutable);
  const version = spawnSync(target.executable, [...target.prefix, '--version'], { env: environment, encoding: 'utf8', shell: false });
  await writeFile(path.join(evidence, 'installed-identity.json'), JSON.stringify({ platform: process.platform, architecture: process.arch, node: process.version,
    abi: process.versions.modules, managerSha256: digest(await readFile(manager)), dependencySource: environment.GESTALT_AGENTS_SOURCE,
    delegatedSetupSha256: environment.GESTALT_AGENTS_SETUP_SHA256, delegatedApiOwnerSha256: environment.GESTALT_AGENTS_SETUP_OWNER_SHA256,
    setupApiVersion: 1, managedOwnership: process.platform === 'darwin' ? 'unknown' : 'released', retained,
    repeat: process.platform === 'darwin' ? 'refused-retained-lock' : 'offline-success', codexVersion: version.stdout?.trim(), codexStatus: version.status,
    runtime: prepared, ownershipHelperSha256: digest(await readFile(ownershipHelper)), observedAt: new Date().toISOString() }, null, 2));
});
