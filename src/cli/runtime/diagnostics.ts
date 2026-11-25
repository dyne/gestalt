import path from 'node:path';
import { lstat, readFile, realpath } from 'node:fs/promises';
import { parse } from 'smol-toml';
import type { CommandInvocation, CommandStreams, OwnedProcessPort } from '../contracts.ts';
import type { CliMetadata, CommandHandlers } from '../dispatch.ts';
import { optionalStat, setupPaths, SetupContext, type SetupPaths } from '../setup/context.ts';
import { RuntimeCommands, diagnosticEnvironment } from './commands.ts';
import { isGestaltManagedMcp } from './approvals.mjs';
import { probeSandbox, type SandboxReport } from './sandbox.ts';
import { manageSkills } from './skills-owner.mjs';
import { commandTarget } from '../platform/executables.ts';

async function json(file: string): Promise<any> {
  const info = await lstat(file);
  if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || info.size > 1024 * 1024) throw new Error(`invalid metadata file: ${file}`);
  return JSON.parse(await readFile(file, 'utf8'));
}
export function pluginVersion(inventory: unknown, name: string): string | null {
  const installed = Array.isArray(inventory) ? inventory : (inventory as { installed?: unknown[] })?.installed;
  const entry = installed?.find((entry: any) => entry?.name === name && entry.installed !== false) as { version?: unknown } | undefined;
  return typeof entry?.version === 'string' && entry.version.length > 0 && entry.version.length < 256 &&
    !/[\x00-\x1f\x7f\\/]/.test(entry.version) && entry.version !== '.' && entry.version !== '..' ? entry.version : null;
}
export class DiagnosticContext {
  readonly commands: RuntimeCommands;
  readonly paths: SetupPaths;
  readonly invocation: CommandInvocation;
  readonly streams: CommandStreams;
  readonly commandQueries: boolean;
  constructor(paths: SetupPaths, invocation: CommandInvocation, streams: CommandStreams, port?: OwnedProcessPort,
    commandQueries = false) {
    this.paths = paths; this.invocation = invocation; this.streams = streams;
    this.commandQueries = commandQueries;
    this.commands = new RuntimeCommands(invocation, streams, paths.manager, port);
  }
  async print(label: string, value: string): Promise<void> { await this.streams.stdout(Buffer.from(label.padEnd(24) + value + '\n')); }
  /** Reject unsupported shims before asking the command owner to launch a probe. */
  async probeTarget(name: string): Promise<Awaited<ReturnType<typeof commandTarget>> | null> {
    const executable = await this.commands.resolve(name);
    if (!executable) return null;
    try { return await commandTarget(executable); } catch { return null; }
  }
  async query(name: string, argv: readonly string[]): Promise<string | null> {
    if (!this.commandQueries) return null;
    const target = await this.probeTarget(name);
    if (!target) return null;
    try {
      const result = await this.commands.capture(target.executable, [...target.prefix, ...argv]);
      if (result.status !== 0) return null;
      // macOS may observe a successful command while complete lifetime remains
      // unknown. Diagnostics never delete state or turn that into a release.
      if (result.claim.kind !== 'released' && process.platform !== 'darwin') return null;
      return result.stdout.trim();
    } catch { return null; }
  }
  async inventory(): Promise<unknown> {
    if (!this.commandQueries) {
      // Read exact cached manifests only. Never start Codex against the user's
      // profile to discover metadata: even a query can initialize caches/logs.
      let prepared: any, config: any;
      try { prepared = await json(path.join(this.paths.gestaltHome, 'runtime/context-mode/.context-mode-prepared.json')); } catch { /* absent metadata */ }
      try { config = parse(await readFile(path.join(this.paths.codexHome, 'config.toml'), 'utf8')); } catch { /* absent metadata */ }
      const installed = [];
      for (const name of ['gestalt', 'context-mode'] as const) {
        const version = pluginVersion([{ name, version: this.invocation.environment[name === 'gestalt' ? 'GESTALT_AGENTS_VERSION' : 'GESTALT_CONTEXT_MODE_VERSION'] || prepared?.packageVersion }], name);
        if (!version) continue;
        const override = this.invocation.environment[name === 'gestalt' ? 'GESTALT_AGENTS_PLUGIN_ROOT' : 'GESTALT_CONTEXT_MODE_PLUGIN_ROOT'];
        try {
          const manifest = await json(path.join(override || path.join(this.paths.codexHome, 'plugins/cache', this.paths.marketplaceName, name, version), '.codex-plugin/plugin.json'));
          if (manifest.name === name && manifest.version === version) installed.push({ name, version, enabled: config?.plugins?.[name + '@' + this.paths.marketplaceName]?.enabled !== false });
        } catch { /* no verified installed manifest */ }
      }
      return installed;
    }
    const result = await this.query('codex', ['plugin', 'list', '--marketplace', this.paths.marketplaceName, '--json']);
    if (result === null) return null;
    try { return JSON.parse(result); } catch { return null; }
  }
  async executableVersion(name: string): Promise<string | null> {
    const executable = await this.commands.resolve(name); if (!executable) return null;
    if (name === 'npm' && process.platform === 'win32' && path.basename(executable).toLowerCase() === 'npm.cmd') {
      // Node's bundled npm.cmd is a bespoke batch launcher. The sibling npm
      // package supplies its version without invoking or interpreting the wrapper.
      try {
        const root = path.join(path.dirname(executable), 'node_modules/npm');
        const manifest = await json(path.join(root, 'package.json'));
        if (manifest.name === 'npm' && manifest.bin?.npm === 'bin/npm-cli.js' &&
            typeof manifest.version === 'string' && /^\d+\.\d+\.\d+(?:[-+][\w.-]+)?$/.test(manifest.version) &&
            (await lstat(path.join(root, 'bin/npm-cli.js'))).isFile())
          return manifest.version + ' (package metadata; command not run)';
      } catch { /* An npm-generated Node shim can still be queried below. */ }
    }
    if (this.commandQueries) return this.query(name, ['--version']);
    try {
      const target = await commandTarget(executable); let directory = path.dirname(target.prefix[0] || executable);
      for (let depth = 0; depth < 7; depth++) {
        try { const manifest = await json(path.join(directory, 'package.json'));
          if ((manifest.name === '@openai/codex' || manifest.name?.startsWith('@openai/codex-')) && name === 'codex' &&
              typeof manifest.version === 'string' && /^\d+\.\d+\.\d+(?:[-+][\w.-]+)?$/.test(manifest.version))
            return manifest.version + ' (package metadata; command not run)';
        } catch { /* not a package directory */ }
        const parent = path.dirname(directory); if (parent === directory) break; directory = parent;
      }
    } catch { /* non-Node executables need an explicit runtime probe */ }
    return 'PRESENT (version not probed)';
  }
  async version(name: 'gestalt' | 'context-mode', inventory?: unknown): Promise<string> {
    const inherited = this.invocation.environment[name === 'gestalt' ? 'GESTALT_AGENTS_VERSION' : 'GESTALT_CONTEXT_MODE_VERSION'];
    const value = inherited ? pluginVersion([{ name, version: inherited }], name) : pluginVersion(inventory ?? await this.inventory(), name);
    if (!value) throw new Error(`cannot resolve the installed ${name} plugin version; codex or a session version is required`);
    return value;
  }
  async runtime(version?: string): Promise<string> {
    const environment = this.invocation.environment;
    if (environment.GESTALT_CONTEXT_MODE_RUNTIME) return environment.GESTALT_CONTEXT_MODE_RUNTIME;
    const root = path.join(this.paths.gestaltHome, 'runtime/context-mode');
    if (!await optionalStat(path.join(root, '.context-mode-prepared.json'))) {
      const legacy = path.join(root, version ?? await this.version('context-mode'), [process.platform, process.arch, 'node-' + process.versions.modules].join('-'));
      if (await optionalStat(path.join(legacy, '.context-mode-prepared.json'))) return legacy;
    }
    return root;
  }
  mobile(): string { return this.invocation.environment.GESTALT_MOBILE_BIN || path.join(this.paths.gestaltHome, 'mobile/node_modules/.bin', process.platform === 'win32' ? 'gestalt-mobile.cmd' : 'gestalt-mobile'); }
  async managedPath(name: string): Promise<string> {
    const p = this.paths, env = this.invocation.environment;
    const fixed: Record<string, string> = { bin: path.join(p.gestaltHome, 'bin'), 'org-plan': path.join(p.gestaltHome, 'bin/org-plan'),
      'context-mode-cli': path.join(p.gestaltHome, 'bin/context-mode'), mobile: this.mobile(), 'codex-home': p.codexHome, 'gestalt-home': p.gestaltHome };
    if (Object.hasOwn(fixed, name)) return fixed[name]!;
    if (name === 'context-mode' || name === 'context-mode-runtime') return this.runtime();
    if (name === 'context-mode-plugin' || name === 'gestalt-plugin') {
      const plugin = name === 'gestalt-plugin' ? 'gestalt' : 'context-mode', key = plugin === 'gestalt' ? 'GESTALT_AGENTS_PLUGIN_ROOT' : 'GESTALT_CONTEXT_MODE_PLUGIN_ROOT';
      return env[key] || path.join(p.codexHome, 'plugins/cache', p.marketplaceName, plugin, await this.version(plugin));
    }
    throw new Error(`unknown managed path: ${name} (expected bin, org-plan, context-mode-cli, mobile, context-mode, context-mode-plugin, gestalt-plugin, codex-home, or gestalt-home)`);
  }
}

export async function verifyContextRuntime(context: DiagnosticContext): Promise<string> {
  const inventory = await context.inventory(), gestalt = await context.version('gestalt', inventory), version = await context.version('context-mode', inventory);
  if (gestalt !== version) throw new Error(`plugin version skew after marketplace setup: gestalt=${gestalt} context-mode=${version}`);
  const runtime = await context.runtime(version), marker = await json(path.join(runtime, '.context-mode-prepared.json'));
  if (marker.packageVersion !== version || marker.platform !== process.platform || marker.arch !== process.arch || marker.nodeModulesAbi !== process.versions.modules)
    throw new Error(`context-mode ${version} runtime is not prepared for this target/Node ABI: ${runtime}`);
  const installed = (Array.isArray(inventory) ? inventory : (inventory as any)?.installed) ?? [];
  if (!installed.some((entry: any) => entry.name === 'context-mode' && entry.installed !== false && entry.enabled === false))
    throw new Error('context-mode plugin source must remain disabled; run gestalt install or gestalt update to restore the native MCP bridge');
  let value;
  try { value = context.commandQueries ? JSON.parse(await context.query('codex', ['mcp', 'get', 'context-mode', '--json']) ?? 'null') :
    (parse(await readFile(path.join(context.paths.codexHome, 'config.toml'), 'utf8')).mcp_servers as any)?.['context-mode']; } catch { /* failure below */ }
  const command = value?.transport?.command ?? value?.command, args = value?.transport?.args ?? value?.args;
  if ((context.commandQueries ? value?.enabled !== true : !value || value.enabled === false) || !['node', 'node.exe', process.execPath].includes(command) ||
      !Array.isArray(args) || args.length !== 1 || args[0] !== path.join(context.paths.codexHome, 'bin/context-mode-mcp.mjs'))
    throw new Error('context-mode native MCP bridge is missing or disabled; run gestalt install or gestalt update');
  return runtime;
}

export async function version(context: DiagnosticContext, metadata: Pick<CliMetadata, 'version'>): Promise<number> {
  await context.streams.stdout(Buffer.from(`Gestalt Manager ${metadata.version}\n`));
  if (process.platform !== 'win32') { const mobile = await context.query(context.mobile(), ['--version']); if (mobile !== null) await context.print('Gestalt Mobile', mobile); }
  const inventory = await context.inventory();
  for (const [name, label] of [['gestalt', 'Gestalt Agents'], ['context-mode', 'Context Mode']] as const) {
    const value = pluginVersion(inventory, name);
    if (value) await context.print(label, value);
  }
  const skillsContext = new SetupContext(context.invocation, context.streams, context.paths, metadata.version);
  await manageSkills(skillsContext, 'status', 'version');
  for (const [name, label] of [['codex', 'Codex CLI'], ['kimi', 'Kimi CLI']] as const) {
    const value = await context.executableVersion(name); if (value !== null) await context.print(label, value);
  }
  if (process.platform === 'win32') {
    for (const label of ['Serena', 'Playwright', 'Playwright readiness', 'Gestalt Mobile']) await context.print(label, 'DEFERRED (native optional feature)');
    for (const label of ['uv (Serena)', 'Python (Serena)']) await context.print(label, 'NOT REQUIRED (native Serena deferred)');
    return context.commands.interrupted || 0;
  }
  // The component owners provide executable/readiness bodies in L5/L6. Reading
  // a descriptor here is installation metadata, never a connection claim.
  let serena: { serena: string; python: string; uv: string } | undefined;
  try {
    const descriptor = await json(path.join(context.paths.gestaltHome, 'serena/active.json'));
    if (descriptor.schemaVersion !== 1 || (descriptor.contractVersion !== 1 && descriptor.version !== '1.7.0') ||
        !/^\d+\.\d+\.\d+$/.test(descriptor.version) || !Array.isArray(descriptor.tools) ||
        !['find_symbol', 'get_symbols_overview', 'replace_symbol_body'].every(name => descriptor.tools.some((tool: any) => tool.name === name && tool.inputSchema))) throw new Error('invalid Serena installation');
    const py = await context.query(descriptor.python, ['-I', '-B', '-c', 'import importlib.metadata,json,sys;print(json.dumps({"serena":importlib.metadata.version("serena-agent"),"python":".".join(map(str,sys.version_info[:3]))}))']);
    const uv = await context.query(descriptor.uv, ['--version']), value = JSON.parse(py ?? 'null'), match = uv?.match(/^uv (\d+\.\d+\.\d+)(?:[ (]|$)/);
    if (value?.serena === descriptor.version && /^3\.13\.\d+$/.test(value.python) && match) serena = { serena: value.serena, python: value.python, uv: match[1]! };
  } catch { /* no import of Serena itself and no initialization/repair */ }
  await context.print('Serena', serena?.serena ?? 'NOT INSTALLED (optional)');
  let browser = 'NOT INSTALLED (optional)', browserPresent = false;
  try { const descriptor = await json(path.join(context.paths.gestaltHome, 'playwright/active.json'));
    browserPresent = true; browser = descriptor.schemaVersion === 1 && descriptor.contractVersion === 1 && /^\d+\.\d+\.\d+$/.test(descriptor.version) ? descriptor.version : 'INVALID INSTALLATION'; } catch { /* report absence */ }
  await context.print('Playwright', browser);
  await context.print('Playwright readiness', browserPresent ? 'UNAVAILABLE (native component doctor required)' : 'NOT INSTALLED (optional)');
  await context.print('uv (Serena)', serena?.uv ?? 'NOT INSTALLED (optional)'); await context.print('Python (Serena)', serena?.python ?? 'NOT INSTALLED (optional)');
  return context.commands.interrupted || 0;
}

async function paths(context: DiagnosticContext, argv: readonly string[]): Promise<number> {
  if (argv.length === 1 && argv[0] !== '--json') { await context.streams.stdout(Buffer.from(await context.managedPath(argv[0]!) + '\n')); return 0; }
  if (argv.length > 1 || (argv.length === 1 && argv[0] !== '--json')) throw new Error('usage: gestalt path [NAME|--json]');
  const values = { mobile: await context.managedPath('mobile'), contextModeRuntime: await context.managedPath('context-mode'),
    contextModePlugin: await context.managedPath('context-mode-plugin'), gestaltPlugin: await context.managedPath('gestalt-plugin'),
    codexHome: context.paths.codexHome, gestaltHome: context.paths.gestaltHome, bin: await context.managedPath('bin'),
    contextModeCli: await context.managedPath('context-mode-cli'), orgPlan: await context.managedPath('org-plan') };
  if (argv[0] === '--json') await context.streams.stdout(Buffer.from(JSON.stringify(values) + '\n'));
  else for (const [label, value] of [['Gestalt Mobile', values.mobile], ['Gestalt commands', values.bin], ['Context-mode runtime', values.contextModeRuntime],
    ['Context-mode plugin', values.contextModePlugin], ['Gestalt plugin', values.gestaltPlugin], ['Codex home', values.codexHome], ['Gestalt home', values.gestaltHome]]) await context.print(label!, value!);
  return 0;
}

export function supervisionCompatibility(agents: any, mobile: any): { ready: boolean; message: string; model: string } {
  const required = ['supervision-start', 'wait-lease-tool', 'checkpoint-tool', 'agent-capacity-recovery', 'canonical-agent-identity', 'org-plan-contract'];
  for (const [component, manifest, capabilities] of [['agents', agents, required], ['mobile', mobile, [...required, 'controller-status', 'session-verdict', 'acknowledgement-safe-composer']]] as const) {
    if (!manifest || manifest.schemaVersion !== 1 || manifest.component !== component || manifest.supervisionContract !== 1 ||
      !Array.isArray(manifest.capabilities) || manifest.capabilities.some((value: unknown) => typeof value !== 'string' || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value)) ||
      capabilities.some(value => !manifest.capabilities.includes(value))) return { ready: false, message: `INCOMPATIBLE (${component} capability manifest; update both packages, then restart Mobile)`, model: 'NOT VERIFIED' };
  }
  const a = agents.modelPolicyContract, m = mobile.modelPolicyContract;
  if (!a && !m && !agents.capabilities.some((name: string) => name.startsWith('l1-model-policy-')) && !mobile.capabilities.some((name: string) => name.startsWith('l1-model-policy-')))
    return { ready: true, message: 'ready (v1; offline manifests)', model: 'LEGACY (base supervision only)' };
  const declared = [agents, mobile].every(value => value.capabilities.includes('l1-model-policy-v1') && value.capabilities.includes('l2-report-boundary') &&
    value.modelPolicyContract?.version === 1 && value.modelPolicyContract.runtimeEnforcement === false);
  if (declared && a.scope === 'configured-launch-and-retention' && m.scope === a.scope &&
    a.defaultAssurance === 'configured' && m.defaultAssurance === 'configured' && a.strictAttestation === false && m.strictAttestation === false)
    return { ready: true, message: 'ready (v1; offline manifests)', model: 'SUPPORTED (v1 configured compatibility; explicit strict attestation unsupported; runtimeEnforcement=false)' };
  if (declared && a.scope === 'schema-and-local-contracts' && m.scope === 'schema-and-retention')
    return { ready: true, message: 'ready (v1; offline manifests)', model: 'SUPPORTED (v1 schema/configuration diagnostics; runtimeEnforcement=false; automatic policy launches unsupported)' };
  return { ready: false, message: 'INCOMPATIBLE (update or roll back Agents and Mobile together; then restart Mobile)', model: 'NOT VERIFIED' };
}

export async function doctor(context: DiagnosticContext, argv: readonly string[], metadata: Pick<CliMetadata, 'version'>): Promise<number> {
  let probe = false, selected: string | undefined;
  for (let index = 0; index < argv.length; index++) {
    if (argv[index] === '--probe-sandbox' && !probe) probe = true;
    else if (argv[index] === '--cwd' && selected === undefined && argv[index + 1] !== undefined) selected = argv[++index];
    else throw new Error('usage: gestalt doctor [--cwd ROOT] [--probe-sandbox]');
  }
  const workspace = await realpath(selected ? path.resolve(context.invocation.cwd, selected) : context.invocation.cwd);
  if (!(await lstat(workspace)).isDirectory()) throw new Error('doctor workspace must be an existing directory');
  let failed = false;
  await context.streams.stdout(Buffer.from(`Gestalt manager ${metadata.version}\n\n`));
  await context.print('Codex home', context.paths.codexHome); await context.print('Gestalt home', context.paths.gestaltHome);
  await context.print('Node.js', process.versions.node); if (Number(process.versions.node.split('.')[0]) < 24) failed = true;
  for (const [command, label] of [['npm', 'npm'], ['codex', 'Codex CLI']] as const) {
    const value = await context.executableVersion(command); await context.print(label, value ?? 'MISSING OR UNVERIFIED'); if (value === null) failed = true;
  }
  const marketplace = path.join(context.paths.codexHome, '.tmp/marketplaces', context.paths.marketplaceName);
  const setup = await optionalStat(path.join(marketplace, 'scripts/setup-agents.mjs')) || await optionalStat(path.join(marketplace, 'gestalt-setup.mjs')) || await optionalStat(path.join(marketplace, 'gestalt-setup.sh'));
  await context.print('Agents marketplace', setup?.isFile() ? 'present' : 'MISSING'); if (!setup?.isFile()) failed = true;
  const inventory = await context.inventory();
  for (const name of ['gestalt', 'context-mode']) { const value = pluginVersion(inventory, name); await context.print(name === 'gestalt' ? 'Gestalt plugins' : 'Context-mode plugin', value ?? 'NOT VERIFIED'); if (!value) failed = true; }
  let runtime: string | undefined;
  try { runtime = await verifyContextRuntime(context); await context.print('Context-mode runtime', runtime); }
  catch (error) { failed = true; await context.print('Context-mode runtime', `NOT VERIFIED (${error instanceof Error ? error.message : 'invalid installation'})`); }
  const mobile = process.platform === 'win32' ? null : await context.query(context.mobile(), ['--version']);
  await context.print('Gestalt Mobile', process.platform === 'win32' ? 'DEFERRED (native optional feature)' : mobile ?? 'MISSING OR UNVERIFIED');
  if (mobile === null && process.platform !== 'win32') failed = true;
  let agentsManifest, mobileManifest;
  for (const [component, file] of [['agents', path.join(marketplace, 'plugins/gestalt/gestalt-supervision-capabilities.json')],
    ['mobile', path.join(context.paths.gestaltHome, 'mobile/node_modules/gestalt-mobile/gestalt-supervision-capabilities.json')]] as const) {
    try { if ((await lstat(file)).size > 16384) throw new Error('manifest exceeds supported size'); const value = await json(file);
      if (component === 'agents') agentsManifest = value; else mobileManifest = value; } catch { /* coherence reports missing/malformed */ }
  }
  const supervision = supervisionCompatibility(agentsManifest, mobileManifest);
  await context.print('Supervision contract', process.platform === 'win32' ? 'UNAVAILABLE (native Mobile deferred)' : supervision.message);
  await context.print('L1 model policy', process.platform === 'win32' ? 'NOT VERIFIED (native Mobile deferred)' : supervision.model);
  if (!supervision.ready && process.platform !== 'win32') failed = true;
  await context.streams.stdout(Buffer.from('\nOptional skill collection\n'));
  const skillsContext = new SetupContext(context.invocation, context.streams, context.paths, metadata.version);
  if (await manageSkills(skillsContext, 'status') !== 0) failed = true;
  const codex = context.commandQueries ? await context.probeTarget('codex') : null;
  if (codex) {
    const args = ['doctor', '--summary', '--ascii', '--no-color'];
    try {
      const config = parse(await readFile(path.join(context.paths.codexHome, 'config.toml'), 'utf8'));
      const removed = new Set(['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP']);
      for (const [name, server] of Object.entries(config.mcp_servers ?? {}) as [string, any][]) {
        if (!isGestaltManagedMcp(name, server, { manager: context.paths.manager, codexHome: context.paths.codexHome })) continue;
        const names: string[] = server.env_vars ?? [], retained = names.filter(key => !removed.has(key) || Object.hasOwn(context.invocation.environment, key));
        if (retained.length !== names.length) args.push('-c', 'mcp_servers.' + name + '.env_vars=' + JSON.stringify(retained));
      }
    } catch { /* native doctor reports malformed or missing config */ }
    await context.streams.stdout(Buffer.from('\nCodex doctor\n'));
    const result = await context.commands.capture(codex.executable, [...codex.prefix, ...args]); await context.streams.stdout(Buffer.from(result.stdout)); await context.streams.stderr(Buffer.from(result.stderr));
    if (result.status !== 0 || result.claim.kind !== 'released' && process.platform !== 'darwin') failed = true;
  }
  if (context.commandQueries && runtime && await optionalStat(path.join(runtime, 'cli.bundle.mjs'))) {
    await context.streams.stdout(Buffer.from('\nContext-mode doctor\n'));
    const result = await context.commands.capture(process.execPath, [path.join(runtime, 'cli.bundle.mjs'), 'doctor'], { ...diagnosticEnvironment(context.invocation.environment), CONTEXT_MODE_PLATFORM: 'codex', CONTEXT_MODE_WORKSPACE: workspace });
    await context.streams.stdout(Buffer.from(result.stdout)); await context.streams.stderr(Buffer.from(result.stderr)); if (result.status !== 0) failed = true;
  }
  const sandbox: SandboxReport = probe ? await probeSandbox(context) : { platform: process.platform, ready: false,
    reason: 'NOT PROBED (use --probe-sandbox for an isolated owned probe)', userNamespaces: {} };
  for (const [key, value] of Object.entries(sandbox.userNamespaces)) await context.streams.stdout(Buffer.from(`${key} = ${value ?? 'unavailable'}\n`));
  await context.print('Codex sandbox', sandbox.ready ? 'ready' : sandbox.reason); if (probe && !sandbox.ready) failed = true;
  if (sandbox.diagnostic) await context.streams.stdout(Buffer.from(sandbox.diagnostic + '\n'));
  if (sandbox.retained) await context.print('Retained probe state', sandbox.retained);
  for (const [name, label] of [['serena', 'Serena'], ['playwright', 'Playwright']] as const) {
    const present = await optionalStat(path.join(context.paths.gestaltHome, name, 'active.json'));
    await context.print(label, process.platform === 'win32' ? 'DEFERRED (native optional feature; retained metadata is not readiness)' :
      present ? 'INSTALLED; native component readiness requires its component doctor' : `NOT INSTALLED (optional; gestalt ${name} install)`);
    if (present && process.platform !== 'win32') failed = true; // installation metadata alone is never readiness
  }
  if (failed) await context.streams.stderr(Buffer.from("\nDiagnostics reported a problem; follow the failing check's guidance above. Use 'gestalt install' or 'gestalt update' only for missing or incompatible managed components.\n"));
  else await context.streams.stdout(Buffer.from(context.commandQueries || probe ? '\nAll requested diagnostics passed.\n' : '\nRead-only diagnostics complete; runtime capabilities were not probed.\n'));
  return context.commands.interrupted || (failed ? 1 : 0);
}

export function createDiagnosticHandlers(metadata: CliMetadata, port?: OwnedProcessPort, ownershipHelper?: string): CommandHandlers {
  const wrap = (body: (context: DiagnosticContext, argv: readonly string[]) => Promise<number>) => async (invocation: CommandInvocation, streams: CommandStreams) => {
    let context: DiagnosticContext | undefined, unlisten: (() => void) | undefined;
    try {
      const selected = await setupPaths(invocation.environment, ownershipHelper);
      context = new DiagnosticContext(selected, { ...invocation, environment: diagnosticEnvironment({ ...invocation.environment, CODEX_HOME: selected.codexHome, GESTALT_HOME: selected.gestaltHome }) }, streams, port);
      unlisten = context.commands.listen(); return await body(context, invocation.argv);
    } catch (error) { await streams.stderr(Buffer.from(`gestalt: ${error instanceof Error ? error.message : 'diagnostic failed'}\n`)); return context?.commands.interrupted || 1; }
    finally { await context?.commands.stop(); unlisten?.(); }
  };
  return { path: wrap(paths), version: wrap(context => version(context, metadata)), doctor: wrap((context, argv) => doctor(context, argv, metadata)) };
}
