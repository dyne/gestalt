import { mkdir, readFile, realpath, rm, unlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { approvalConfiguration, permissionConfiguration } from './configuration.mjs';
import { acquireSetupGate, digest, readProfile, replaceProfile } from './profile-files.ts';
import type { ProfileSnapshot } from './profile-files.ts';
import { optionalStat, SetupContext } from './context.ts';
import type { NativePrivateStorage } from '../platform/storage.ts';

type Plugin = { name?: string; version?: string; installed?: boolean; enabled?: boolean };
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

/** Manager owns only config/provenance. The portable Agents owner retains its
 * package, runtime, hooks, profiles and its independent rollback transaction. */
export async function setupAgents(context: SetupContext, mode: 'install' | 'upgrade', extraSkills: boolean): Promise<void> {
  if (Number(process.versions.node.split('.')[0]) < 24) throw new Error(`Node.js 24 or newer is required; found ${process.versions.node}`);
  await context.require('npm'); await context.require('codex');
  const { codexHome, gestaltHome, marketplace, marketplaceName, userHome, manager } = context.paths;
  const codexStorage = await context.storage(codexHome), gestaltStorage = await context.storage(gestaltHome);
  const operation = await codexStorage.acquireLock(path.join(codexHome, '.gestalt-manager-setup.lock'));
  let releaseGate: (() => Promise<void>) | null = null;
  let stage: string | null = null, original: ProfileSnapshot | null = null, snapshotReady = false, preserve = false;
  const configFile = path.join(codexHome, 'config.toml');
  const normalize = async () => {
    const before = await readProfile(configFile), source = before?.bytes.toString('utf8') ?? '';
    const temporaryRoot = process.platform === 'win32'
      ? path.join(context.environment.LOCALAPPDATA || path.join(userHome, 'AppData/Local'), 'Temp') : '/tmp';
    const output = permissionConfiguration(approvalConfiguration(source, manager, codexHome),
      [...new Set([codexHome, gestaltHome, path.join(userHome, '.agents'), path.join(userHome, '.local'), path.join(userHome, '.config')])], temporaryRoot);
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
    const sourceRoot = override ? await realpath(override) : path.join(codexHome, '.tmp/marketplaces', marketplaceName);
    const setup = path.join(sourceRoot, 'gestalt-setup.mjs');
    const action = mode === 'install' && !(await optionalStat(setup)) ? 'add' : 'upgrade';
    await context.log(`${action} marketplace ${override || marketplace} in ${codexHome}`);
    // Explicit immutable/local checkout transport never upgrades its Git tree.
    if (override) await context.run('codex', ['plugin', 'marketplace', 'add', sourceRoot]);
    else {
      try { await context.run('codex', ['plugin', 'marketplace', action, action === 'add' ? marketplace : marketplaceName]); }
      catch {
        context.check();
        await context.log(`marketplace ${action} unavailable; attempting ${action === 'add' ? 'an upgrade' : 'a fresh registration'}`);
        await context.run('codex', ['plugin', 'marketplace', action === 'add' ? 'upgrade' : 'add', action === 'add' ? marketplaceName : marketplace]);
      }
    }
    if (!(await optionalStat(setup))?.isFile()) throw new Error(`marketplace native Node setup is missing: ${setup}; update Gestalt Agents to a native-capable release`);
    const expected = context.environment.GESTALT_AGENTS_SETUP_SHA256;
    const setupDigest = digest(await readFile(setup));
    if (expected && expected !== setupDigest) throw new Error('Agents Node setup digest differs from the selected dependency');
    await context.log('preparing context mode, plugins, and agent profiles');
    await context.run(process.execPath, [setup, ...(extraSkills ? ['--extra-skills'] : [])]);
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
      manager: { version: context.version, sha256: digest(await readFile(manager)) }, delegatedSetupSha256: setupDigest, outputs }, null, 2) + '\n');
    const destination = path.join(codexHome, '.gestalt-setup-provenance.json');
    const old = await readProfile(destination);
    if (!old?.bytes.equals(provenance)) await codexStorage.replaceFile(destination, provenance);
  } catch (error) {
    if (snapshotReady) {
      try {
        releaseGate ??= await claimHomes([codexStorage, gestaltStorage]);
        const current = await readProfile(configFile);
        if (original) await replaceProfile(codexStorage, configFile, original.bytes, current);
        else if (current) await unlink(configFile);
      } catch (rollback) {
        preserve = true;
        throw new Error(`${error instanceof Error ? error.message : error}; configuration rollback needs inspection at ${stage}: ${rollback instanceof Error ? rollback.message : rollback}`, { cause: error });
      }
    }
    throw error;
  } finally {
    unlisten();
    if (releaseGate) await releaseGate();
    if (stage && !preserve) await rm(stage, { recursive: true });
    await operation.release();
  }
}
