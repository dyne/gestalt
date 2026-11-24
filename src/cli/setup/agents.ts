import { mkdir, readFile, realpath, rename, rm, stat, unlink } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { parse } from 'smol-toml';
import { approvalConfiguration, permissionConfiguration } from './configuration.mjs';
import { acquireSetupGate, digest, readProfile, replaceProfile } from './profile-files.ts';
import type { ProfileSnapshot } from './profile-files.ts';
import { absent, CommandFailure, optionalStat, SetupContext } from './context.ts';
import type { NativePrivateStorage } from '../platform/storage.ts';

type Plugin = { name?: string; version?: string; installed?: boolean; enabled?: boolean };
const setupCapabilities = ['async-owned-commands-v1', 'host-filesystem-v1', 'shared-context-v1'] as const;
// Agents main does not yet ship the native setup API. Use the release-tested
// revision for new default Windows registrations until that API reaches main.
const windowsAgentsRevision = 'b4f783d75c0543010c665072bf7aef07f74f973f';

/** Move an unregistered marketplace cache out of Codex's install path. */
async function moveMarketplaceCache(source: string, destination: string): Promise<boolean> {
  const entry = await optionalStat(source);
  if (!entry) return false;
  if (!entry.isDirectory() || await realpath(source) !== source)
    throw new Error(`marketplace cache is not a direct directory: ${source}`);
  await rename(source, destination);
  return true;
}

function installedPlugins(text: string): Plugin[] {
  const result: unknown = JSON.parse(text);
  const plugins = Array.isArray(result) ? result : (result as { installed?: unknown })?.installed;
  if (!Array.isArray(plugins)) throw new Error('Codex returned an invalid plugin list');
  return plugins as Plugin[];
}
export async function verifyAgents(context: SetupContext): Promise<void> {
  const { codexHome, gestaltHome, marketplaceName } = context.paths;
  const plugins = installedPlugins(await context.run('codex', ['plugin', 'list', '--marketplace', marketplaceName, '--json'], { capture: true }));
  const version = (name: string) => {
    const value = plugins.find(plugin => plugin?.name === name && plugin.installed !== false)?.version;
    if (typeof value !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9.+_-]*$/.test(value) || value.includes('..'))
      throw new Error(`${name} plugin is not installed after marketplace setup`);
    return value;
  };
  const gestaltVersion = version('gestalt'), contextVersion = version('context-mode');
  if (gestaltVersion !== contextVersion) throw new Error(`plugin version skew after marketplace setup: gestalt=${gestaltVersion} context-mode=${contextVersion}`);
  const runtime = path.join(gestaltHome, 'runtime/context-mode');
  const legacy = path.join(runtime, contextVersion, [process.platform, process.arch, `node-${process.versions.modules}`].join('-'));
  const root = await optionalStat(path.join(runtime, '.context-mode-prepared.json')) ? runtime : legacy;
  const marker = JSON.parse(await readFile(path.join(root, '.context-mode-prepared.json'), 'utf8')) as Record<string, unknown>;
  if (marker.packageVersion !== contextVersion || marker.nodeModulesAbi !== process.versions.modules ||
      marker.platform !== process.platform || marker.arch !== process.arch ||
      !(await optionalStat(path.join(root, 'cli.bundle.mjs')))?.isFile() || !(await optionalStat(path.join(root, 'server.bundle.mjs')))?.isFile())
    throw new Error(`context-mode ${contextVersion} runtime is not prepared: ${root}`);
  if (plugins.find(plugin => plugin?.name === 'context-mode')?.enabled !== false)
    throw new Error('context-mode plugin source must remain disabled; run gestalt install or gestalt update to restore the native MCP bridge');
  const mcp = JSON.parse(await context.run('codex', ['mcp', 'get', 'context-mode', '--json'], { capture: true })) as
    { enabled?: boolean; command?: string; args?: string[]; transport?: { command?: string; args?: string[] } };
  const command = mcp.transport?.command ?? mcp.command, args = mcp.transport?.args ?? mcp.args;
  if (mcp.enabled !== true || command !== 'node' || !Array.isArray(args) || args.length !== 1 || args[0] !== path.join(codexHome, 'bin/context-mode-mcp.mjs'))
    throw new Error('context-mode native MCP bridge is missing or disabled; run gestalt install or gestalt update');
  await context.log(`verified context-mode ${contextVersion} plugin and external runtime`);
}

async function claimHomes(storages: readonly NativePrivateStorage[]): Promise<() => Promise<void>> {
  const releases: (() => Promise<void>)[] = [];
  try {
    for (const storage of [...storages].sort((a, b) => a.root.localeCompare(b.root))) releases.push(await acquireSetupGate(storage));
  } catch (error) { for (const release of releases.reverse()) await release(); throw error; }
  return async () => { for (const release of releases.reverse()) await release(); };
}

/** Reconcile marketplace registration and manager configuration around the
 * Agents owner's independent package, runtime, hook and profile transaction. */
export async function setupAgents(context: SetupContext, _mode: 'install' | 'upgrade', extraSkills: boolean): Promise<void> {
  if (Number(process.versions.node.split('.')[0]) < 24) throw new Error(`Node.js 24 or newer is required; found ${process.versions.node}`);
  await context.require('npm'); await context.require('codex'); await context.requireOwnership();
  const { codexHome, gestaltHome, marketplace, marketplaceName, userHome, manager } = context.paths;
  const codexStorage = await context.storage(codexHome), gestaltStorage = await context.storage(gestaltHome);
  const operation = await codexStorage.acquireLock(path.join(codexHome, '.gestalt-manager-setup.lock'));
  const cache = path.join(codexHome, '.tmp/marketplaces', marketplaceName);
  let releaseGate: (() => Promise<void>) | null = null;
  let stage: string | null = null, original: ProfileSnapshot | null = null, snapshotReady = false, preserve = false, retainLocks = false;
  let registering = false, savedCache: string | null = null;
  const configFile = path.join(codexHome, 'config.toml');
  const normalize = async () => {
    const before = await readProfile(configFile), source = before?.bytes.toString('utf8') ?? '';
    const { LOCALAPPDATA, XDG_CACHE_HOME, XDG_RUNTIME_DIR } = context.environment;
    const windows = process.platform === 'win32';
    const localAppData = LOCALAPPDATA || path.join(userHome, 'AppData/Local');
    const temporaryRoot = windows ? path.join(localAppData, 'Temp') : '/tmp';
    const cacheHome = windows ? localAppData : XDG_CACHE_HOME && path.isAbsolute(XDG_CACHE_HOME) ? XDG_CACHE_HOME
      : path.join(userHome, process.platform === 'darwin' ? 'Library/Caches' : '.cache');
    const legacyCache = path.join(userHome, '.ccache');
    const legacy = await stat(legacyCache).catch(error => { if (!absent(error)) throw error; });
    const cacheDir = legacy?.isDirectory() ? legacyCache : path.join(cacheHome, 'ccache');
    const cacheRoots = [cacheDir];
    if (!windows && XDG_RUNTIME_DIR && path.isAbsolute(XDG_RUNTIME_DIR))
      cacheRoots.push(path.join(XDG_RUNTIME_DIR, 'ccache-tmp'));
    const readRoots = [...new Set([codexHome, gestaltHome, ...['.agents', '.local', '.config'].map(name => path.join(userHome, name))])];
    const output = permissionConfiguration(approvalConfiguration(source, manager, codexHome), readRoots, temporaryRoot, cacheRoots);
    if (!readRoots.includes(cacheDir)) {
      // Codex needs an existing mount target for writes beneath a read-only home.
      await mkdir(cacheDir, { recursive: true, mode: 0o700 }).catch((error: NodeJS.ErrnoException) => {
        if (!['EACCES', 'EPERM', 'EROFS', 'ENOTDIR', 'EEXIST'].includes(error.code ?? '')) throw error;
      });
    }
    await replaceProfile(codexStorage, configFile, Buffer.from(output), before);
  };
  const unlisten = context.listen();
  try {
    releaseGate = await claimHomes([codexStorage, gestaltStorage]);
    original = await readProfile(configFile);
    stage = path.join(codexHome, `.gestalt-setup.${crypto.randomUUID()}`);
    await codexStorage.createDirectory(stage);
    await codexStorage.replaceFile(path.join(stage, 'snapshot.json'), Buffer.from(JSON.stringify({ present: Boolean(original), mode: original?.mode ?? null }) + '\n'));
    if (original) await codexStorage.replaceFile(path.join(stage, 'config.toml'), original.bytes);
    snapshotReady = true;
    await normalize();
    await releaseGate(); releaseGate = null;

    const override = context.environment.GESTALT_AGENTS_SOURCE;
    const sourceRoot = override ? await realpath(override) : cache;
    const setup = path.join(sourceRoot, 'gestalt-setup.mjs');
    const marketplaces = original && parse(original.bytes.toString('utf8')).marketplaces;
    // Explicit immutable/local checkout transport never upgrades its Git tree.
    if (override) {
      await context.log(`add marketplace ${sourceRoot} in ${codexHome}`);
      await context.run('codex', ['plugin', 'marketplace', 'add', sourceRoot]);
    } else if (marketplaces && typeof marketplaces === 'object' && Object.hasOwn(marketplaces, marketplaceName)) {
      await context.log(`upgrade marketplace ${marketplaceName} in ${codexHome}`);
      await context.run('codex', ['plugin', 'marketplace', 'upgrade', marketplaceName]);
    } else {
      const source = process.platform === 'win32' && !context.environment.GESTALT_MARKETPLACE
        ? `${marketplace}@${windowsAgentsRevision}` : marketplace;
      const backup = path.join(stage, 'previous-marketplace');
      if (await moveMarketplaceCache(cache, backup)) savedCache = backup;
      registering = true;
      await context.log(`add marketplace ${source} in ${codexHome}`);
      await context.run('codex', ['plugin', 'marketplace', 'add', source]);
    }
    if (!(await optionalStat(setup))?.isFile() || await realpath(setup) !== setup)
      throw new Error(`marketplace native Node setup is missing or redirected: ${setup}; update Gestalt Agents to a native-capable release`);
    const owner = path.join(sourceRoot, 'scripts/setup-agents.mjs');
    if (!(await optionalStat(owner))?.isFile() || await realpath(owner) !== owner)
      throw new Error(`canonical Agents setup API is missing or redirected: ${owner}; update the selected Agents checkout`);
    const expected = context.environment.GESTALT_AGENTS_SETUP_SHA256;
    const setupDigest = digest(await readFile(setup));
    if (expected && expected !== setupDigest) throw new Error('Agents Node setup digest differs from the selected dependency');
    const ownerDigest = digest(await readFile(owner));
    if (context.environment.GESTALT_AGENTS_SETUP_OWNER_SHA256 && context.environment.GESTALT_AGENTS_SETUP_OWNER_SHA256 !== ownerDigest)
      throw new Error('Agents setup API owner digest differs from the selected dependency');
    const api = await import(pathToFileURL(owner).href);
    if (api.SETUP_API_VERSION !== 1 || typeof api.setupAgents !== 'function' ||
        !Array.isArray(api.SETUP_API_CAPABILITIES) || !setupCapabilities.every(value => api.SETUP_API_CAPABILITIES.includes(value)))
      throw new Error('Agents setup API v1 with host filesystem and async owned commands is required; update the selected Agents checkout');
    await context.log('preparing context mode, plugins, and agent profiles');
    let outcome;
    try {
      outcome = await api.setupAgents(extraSkills ? ['--extra-skills'] : [], {
        mode: 'managed', commands: await context.managedCommands(), environment: { ...context.environment },
        cwd: context.invocation.cwd, streams: context.streams, signal: context.signal,
      });
    } catch (error) {
      await context.stopActive();
      if ((error as { execution?: { ownership?: { kind?: string } } })?.execution?.ownership?.kind === 'unknown') context.retainDelegatedState();
      const status = (error as { status?: number })?.status;
      throw new CommandFailure(error instanceof Error ? error.message : 'Agents setup failed',
        Number.isInteger(status) && status! > 0 && status! < 256 ? status : 1);
    }
    if (outcome?.apiVersion !== 1 || outcome.mode !== 'managed' || !Array.isArray(outcome.retained) ||
        !['released', 'unknown'].includes(outcome.ownership?.kind) ||
        (outcome.ownership.kind === 'released' && outcome.ownership.completeTree !== true)) {
      context.retainDelegatedState();
      throw new Error('Agents returned an invalid managed setup outcome; preserve the transaction for inspection');
    }
    if (outcome.ownership.kind === 'unknown' && process.platform !== 'darwin') {
      context.retainDelegatedState();
      throw new Error('Agents setup did not establish native descendant absence; retained transaction requires inspection');
    }
    if (outcome.ownership.kind === 'unknown' || !context.canRollback()) {
      preserve = true; retainLocks = true;
      await codexStorage.replaceFile(path.join(stage, 'ownership-unknown.json'), Buffer.from(JSON.stringify({
        schemaVersion: 1, status: 'command-succeeded-cleanup-unknown', claims: context.unknownOwnership, delegated: outcome,
      }) + '\n'));
      await context.log(`Agents setup command succeeded; complete descendant absence is unknown; manager snapshot and setup locks retained at ${stage}; inspect retained transaction state before repeating managed setup`);
      return;
    }
    await verifyAgents(context);
    releaseGate = await claimHomes([codexStorage, gestaltStorage]);
    context.check();
    await normalize();
    const outputs: Record<string, string | null> = {};
    for (const relative of ['config.toml', 'hooks.json', 'agents/org-plan-executor.toml', 'agents/org-plan-reviewer.toml']) {
      const file = path.join(codexHome, relative);
      outputs[relative] = await optionalStat(file) ? digest(await readFile(file)) : null;
    }
    const provenance = Buffer.from(JSON.stringify({ schemaVersion: 1, scope: 'setup-artifacts-only', runtimeEnforcement: false,
      manager: { version: context.version, sha256: digest(await readFile(manager)) }, delegatedSetupSha256: setupDigest,
      delegatedApi: { apiVersion: 1, capabilities: setupCapabilities, ownerSha256: ownerDigest, mode: 'managed', ownership: 'released' }, outputs }, null, 2) + '\n');
    const destination = path.join(codexHome, '.gestalt-setup-provenance.json');
    const old = await readProfile(destination);
    if (!old?.bytes.equals(provenance)) await codexStorage.replaceFile(destination, provenance);
    if (!context.canRollback()) {
      preserve = true; retainLocks = true;
      await codexStorage.replaceFile(path.join(stage, 'ownership-unknown.json'), Buffer.from(JSON.stringify({
        schemaVersion: 1, status: 'command-succeeded-cleanup-unknown', claims: context.unknownOwnership,
      }) + '\n'));
      await context.log(`setup command succeeded; complete descendant absence is unknown on macOS; snapshot retained at ${stage}`);
    }
  } catch (error) {
    await context.stopActive();
    if (!context.canRollback()) {
      preserve = true; retainLocks = true;
      if (stage) await codexStorage.replaceFile(path.join(stage, 'ownership-unknown.json'), Buffer.from(JSON.stringify({
        schemaVersion: 1, status: 'failed-cleanup-unknown', claims: context.unknownOwnership,
      }) + '\n')).catch(() => {});
      throw new CommandFailure(`${error instanceof Error ? error.message : error}; setup rollback is unconfirmed; configuration snapshot and setup locks retained at ${stage ?? codexHome}`,
        error instanceof CommandFailure ? error.status : 1);
    }
    if (snapshotReady) {
      try {
        releaseGate ??= await claimHomes([codexStorage, gestaltStorage]);
        const current = await readProfile(configFile);
        if (original) await replaceProfile(codexStorage, configFile, original.bytes, current);
        else if (current) await unlink(configFile);
        if (registering && stage) {
          await moveMarketplaceCache(cache, path.join(stage, 'failed-marketplace'));
          if (savedCache) await rename(savedCache, cache);
        }
        await context.log('previous configuration restored after owned setup writers stopped');
      } catch (rollback) {
        preserve = true;
        throw new Error(`${error instanceof Error ? error.message : error}; configuration rollback needs inspection at ${stage}: ${rollback instanceof Error ? rollback.message : rollback}`, { cause: error });
      }
    }
    throw error;
  } finally {
    unlisten();
    if (releaseGate && !retainLocks) await releaseGate();
    if (stage && !preserve) await rm(stage, { recursive: true });
    if (!retainLocks) await operation.release();
  }
}
