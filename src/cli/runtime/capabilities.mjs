// Existing native discovery/profile/startup policy, extracted from public/gestalt.
// The selected component bodies remain with their L5/L6 owners.
import * as gestaltProfileToml from 'smol-toml';
import { managedToolApproval, managedMcpApprovals, isGestaltManagedMcp } from './approvals.mjs';
import { managedChild } from './child.ts';
import { scratchEnvironment, scratchJson, scratchBorrow, scratchRelease } from './scratch-runtime.mjs';
import { createInterface } from 'node:readline';
import { readFile, realpath, access } from 'node:fs/promises';
import { constants } from 'node:fs';
import { resolve } from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { nodeHookCommand, associationHookProgram } from './hook-command.mjs';
import { diagnosticEnvironment } from './commands.ts';


export async function launchCapabilities(commands, invocation, manager) {
const listeners = [];
let writing = Promise.resolve();
const sink = write => ({write(bytes) { writing = writing.then(()=>write(Buffer.from(bytes))); return true; }});
const process = { env: invocation.environment, cwd:()=>invocation.cwd, execPath:globalThis.process.execPath,
  stderr:sink(commands.streams.stderr), stdout:sink(commands.streams.stdout),
  on(signal, listener) { listeners.push([signal,listener]); globalThis.process.on(signal,listener); },
};
const spawn = (executable, args, options) => managedChild(commands,executable,args,options);
const args = [...invocation.argv];
try {

const scratchNames=['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'];
const environment={...process.env};
let cliOwned=false,scratchApi;
if(Object.hasOwn(environment,scratchNames[0])) {
 scratchApi={scratchEnvironment,scratchJson,scratchBorrow,scratchRelease};
 Object.assign(environment,scratchApi.scratchEnvironment(environment));
 cliOwned=scratchApi.scratchJson(JSON.parse(environment.GESTALT_SESSION_TMP_CONTEXT).markerPath).ownerKind==='cli-session';
}
// These are required runtime queries, not default read-only diagnostics. Keep
// the validated caller namespace and publish real owned borrowers as usual.
const discoveryEnvironment=diagnosticEnvironment(environment);
let mappingUnknown=false;
const protectUnknownMapping=()=>{
 if(cliOwned&&!mappingUnknown){mappingUnknown=true;scratchApi.scratchBorrow(JSON.parse(environment.GESTALT_SESSION_TMP_CONTEXT),'cli-provider-mapping-unknown');
 process.stderr.write('gestalt: native provider association unavailable; CLI temporary owner remains protected\n');}
};
// Windows owner/ACL bookkeeping is charged to this shared startup window.
// Each snapshot still has its separate 5s cap and the deadline starts before launch.
const budget = globalThis.process.platform === 'win32' ? 10000 : 5000;
// One deadline covers native discovery, shared readiness and MCP startup.
let readinessDeadline = Date.now() + budget;
let currentChild;
let playwrightController;
let interrupted = false;
let interruptionCode = 130;
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => {
  interrupted = true;
  interruptionCode = signal === 'SIGINT' ? 130 : 143;
  currentChild?.kill(signal);
});
const diagnose = reason => process.stderr.write(`gestalt: capability discovery unavailable (${reason}); continuing with native tools\n`);
const toml = value => {
  if (typeof value === 'string') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(toml).join(',')}]`;
  if (value && typeof value === 'object') return `{${Object.entries(value).filter(([, v]) => v != null)
    .map(([k, v]) => `${JSON.stringify(k)}=${toml(v)}`).join(',')}}`;
  return String(value);
};
const appendFlags = flags => {
  const end = args.indexOf('--');
  const boundary = end < 0 ? args.length : end;
  // Codex 0.160 replaces the root repeated-config list when another config
  // flag appears at subcommand scope. Add at the user's selected parse level,
  // after its overrides, without moving or copying any original user argument.
  const index = configInsertion ?? boundary;
  // Ordinary session overrides use an embedded server. Managed Playwright
  // instead supplies its own remote frontend; --no-daemon conflicts there.
  // An explicit user --no-daemon stays untouched and selects the ordinary path.
  const mode = remote || playwrightController || args.slice(0, boundary).includes('--no-daemon') ? [] : ['--no-daemon'];
  return [...mode, ...args.slice(0, index), ...flags.flatMap(flag => ['-c', flag]), ...args.slice(index)];
};
const flags = [];
let cwd = process.cwd();
let rawSkillConfig;
let explicitSkills = {};
let remote = false;
let namedProfile;
let configInsertion;
for (let i = 0; i < args.length && args[i] !== '--'; i++) {
  const option = args[i];
  let config;
  if (option === '-c' || option === '--config') config = args[++i];
  else if (option.startsWith('--config=')) config = option.slice(9);
  else if (option.startsWith('-c') && option.length > 2) config = option.slice(2);
  if (config !== undefined) {
    configInsertion = i + 1;
    flags.push('-c', config);
    const boundary = config.indexOf('=');
    const key = config.slice(0, boundary).trim();
    if (key === 'skills' || key === 'skills.config') {
      try {
        const value = gestaltProfileToml.parse('value=' + config.slice(boundary + 1)).value;
        if (key === 'skills') explicitSkills = value; else explicitSkills.config = value;
      } catch { /* Native CLI still receives the original invalid override. */ }
    }
    if (/^skills\.config\s*=/.test(config)) rawSkillConfig = config.slice(config.indexOf('=') + 1).trim();
  } else if (option === '-C' || option === '--cd') cwd = resolve(cwd, args[++i] ?? '.');
  else if (option.startsWith('--cd=')) cwd = resolve(cwd, option.slice(5));
  else if (option.startsWith('-C') && option.length > 2) cwd = resolve(cwd, option.slice(2));
  else if (option === '-p' || option === '--profile') { namedProfile = args[++i]; }
  else if (option.startsWith('--profile=')) namedProfile = option.slice(10);
  else if (option.startsWith('-p') && option.length > 2) namedProfile = option.slice(2);
  else if (option === '--enable' || option === '--disable') flags.push('-c', `features.${args[++i]}=${option === '--enable'}`);
  else if (option.startsWith('--enable=') || option.startsWith('--disable=')) {
    const [setting, feature] = option.split('=', 2);
    flags.push('-c', `features.${feature}=${setting === '--enable'}`);
  }
  else if (option.startsWith('--remote')) remote = true;
}

async function snapshot(extra = [], hooksOnly = false, beforeFlags = false, deadline = readinessDeadline) {
  const remaining = Math.max(0, deadline - Date.now());
  if (!remaining) throw new Error('readiness-deadline');
  const child = spawn('codex', ['app-server', '--stdio', ...(beforeFlags ? extra.flatMap(value => ['-c', value]) : []), ...flags, ...(!beforeFlags ? extra.flatMap(value => ['-c', value]) : [])], {
    cwd, env: discoveryEnvironment, stdio: ['pipe', 'pipe', 'ignore'],
  });
  const previousChild = currentChild;
  currentChild = child;
  const pending = new Map();
  let id = 0;
  const fail = () => { for (const value of pending.values()) value.reject(new Error('config-unavailable')); pending.clear(); };
  child.on('error', fail);
  child.on('exit', fail);
  child.stdin.on('error', fail);
  const lines = createInterface({ input: child.stdout });
  lines.on('line', line => {
    let message;
    try { message = JSON.parse(line); } catch { return; }
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    message.error ? waiter.reject(new Error('config-unavailable')) : waiter.resolve(message.result);
  });
  const request = (method, params) => new Promise((resolve, reject) => {
    const next = ++id;
    pending.set(next, { resolve, reject });
    child.stdin.write(`${JSON.stringify({ id: next, method, params })}\n`);
  });
  const timer = setTimeout(() => { fail(); child.kill(); }, Math.min(5000, remaining));
  try {
    const initialized = await request('initialize', { clientInfo: { name: 'gestalt-capability-config', version: '1' }, capabilities: {} });
    child.stdin.write(`${JSON.stringify({ method: 'initialized', params: {} })}\n`);
    if (hooksOnly) return await request('hooks/list', { cwds: [cwd] });
    const config = await request('config/read', { cwd, includeLayers: true });
    const skills = await request('skills/list', { cwds: [cwd], forceReload: true });
    return { config: config.config, layers: config.layers ?? [], skills: skills.data?.find(entry => entry.cwd === cwd)?.skills ?? [],
      version: initialized.userAgent ?? '' };
  } finally {
    clearTimeout(timer);
    lines.close();
    child.stdin.end();
    child.kill();
    await child.stop();
    currentChild = previousChild;
  }
}

async function launch(launchArgs) {
  if (playwrightController) return playwrightController(launchArgs);
  if (commands.terminal) {
    const lease = await commands.launchTerminal('codex', launchArgs, environment, process.cwd());
    const result = await lease.result;
    await lease.stop();
    return { code: result.kind === 'exit' ? result.code : result.kind === 'signal' ?
      (Number.isInteger(Number(result.signal)) ? 128 + Number(result.signal) : result.signal === 'SIGINT' ? 130 : 143) : 1,
      startupFailed: new Set() };
  }
  const child = spawn('codex', launchArgs, { cwd: process.cwd(), env: environment,
    // Node's own stdin carries this program. Open the inherited terminal/stdin
    // separately so prompts and EOF retain normal Codex behavior.
    stdio: [3, 'inherit', 'pipe'] });
  currentChild = child;
  let tail = '';
  const startupFailed = new Set();
  child.stderr.on('data', data => {
    process.stderr.write(data);
    tail = (tail + data.toString()).slice(-8192);
    // Scope names to the required-failure diagnostic, not unrelated stderr.
    for (const diagnostic of tail.matchAll(/required MCP servers failed to initialize:([^\r\n]*)/g))
      for (const name of ['gestalt-serena'])
        if (new RegExp(`(?:^|[\\s,;])${name}:`).test(diagnostic[1])) startupFailed.add(name);
  });
  const result = await new Promise(resolve => {
    child.once('error', error => {
      const reason = String(error?.message ?? 'native launch failed').replace(/[\r\n\0]/g, ' ').slice(0, 512);
      process.stderr.write(`gestalt: native command launch failed (${reason})\n`);
      resolve({ code: 127 });
    });
    child.once('exit', (code, signal) => resolve({ code: code ?? (signal === 'SIGINT' ? 130 : 143) }));
  });
  await child.stop(); currentChild = undefined;
  return { ...result, startupFailed };
}

let state;
try { state = await snapshot(); } catch { diagnose('native-config-unavailable'); }
// Codex 0.160 profile-v2 files overlay the base user layer, then projects
// and session flags win. Only readiness-relevant settings are adapted; the
// actual CLI retains --profile and loads all other settings natively.
const profileKeys = ['approval_policy', 'mcp_servers', 'plugins', 'skills', 'hooks', 'features', 'projects', 'developer_instructions'];
const table = value => value && typeof value === 'object' && !Array.isArray(value);
const merge = (base, overlay) => {
  for (const [key, value] of Object.entries(overlay ?? {})) {
    if (table(value) && table(base[key])) merge(base[key], value);
    else base[key] = structuredClone(value);
  }
  return base;
};
async function resolveNamedProfile(state, deadline = readinessDeadline, includeSkills = true) {
  if (!state || !namedProfile) return state;
    if (!/^[a-zA-Z0-9_-]+$/.test(namedProfile)) throw new Error('invalid-profile');
    const profile = gestaltProfileToml.parse(await readFile(resolve(environment.CODEX_HOME, namedProfile + '.config.toml'), 'utf8'));
    const originalSession = state.layers.find(layer => layer.name?.type === 'sessionFlags');
    // Profile trust settings participate in native project discovery.
    if (profile.projects) {
      const discovered = await snapshot([`projects=${toml(profile.projects)}`], false, true, deadline);
      state.layers = discovered.layers.filter(layer => layer.name?.type !== 'sessionFlags');
      if (originalSession) state.layers.unshift(originalSession);
    }
    const effective = {};
    let inserted = false;
    for (const layer of [...state.layers].reverse()) {
      if (!layer.disabledReason) merge(effective, Object.fromEntries(Object.entries(layer.config).filter(([key]) => profileKeys.includes(key))));
      if (layer.name?.type === 'user' && !layer.name.profile) {
        merge(effective, Object.fromEntries(Object.entries(profile).filter(([key]) => profileKeys.includes(key))));
        inserted = true;
      }
    }
    if (!inserted) throw new Error('profile-layer-unavailable');
    state.config = { ...state.config, ...effective };
    // Plugin inclusion is discovered by the native catalog, not inferred from
    // the profile text. No credentials or unrelated settings enter argv.
    if (profile.plugins && includeSkills) {
      const discovered = await snapshot([`plugins=${toml(effective.plugins ?? {})}`], false, false, deadline);
      state.skills = discovered.skills;
    }
  return state;
}
try { state = await resolveNamedProfile(state); } catch { state = undefined; diagnose('profile-discovery-unavailable'); }
const session = state?.layers?.find(layer => layer.name?.type === 'sessionFlags')?.config ?? {};
let rules = session.skills?.config ?? explicitSkills?.config ?? [];
if (!Array.isArray(rules)) rules = [];
// Preserve unrelated selectors from effective config and user session flags.
const servers = state?.config?.mcp_servers ?? {};
const hookCommands = new Set();
let startupHook;
let expectedHookCount = 0;
const sessionHook = context => ['SessionStart', 'SubagentStart'].map(event => {
  const command = nodeHookCommand(associationHookProgram(pathToFileURL(manager).href, context, cliOwned&&!mappingUnknown),process.execPath);
  hookCommands.add(command);
  if (startupHook) hookCommands.add(startupHook);
  expectedHookCount += startupHook ? 2 : 1;
  return `hooks.${event}=${toml([...(session.hooks?.[event] ?? []),
    // Native SessionStart matches its source. Stable instructions belong to a
    // fresh thread; capability/ownership hooks still run on resume as before.
    ...(startupHook ? [{ ...(event === 'SessionStart' ? { matcher: 'startup' } : {}),
      hooks: [{ type: 'command', timeout: 2, command: startupHook }] }] : []),
    { hooks: [{ type: 'command', timeout: globalThis.process.platform === 'win32' ? 120 : 2, command }] }])}`;
});
async function trustedHooks(selected) {
  const listing = await snapshot(selected, true);
  const handlers = listing.data?.find(entry => entry.cwd === cwd)?.hooks ?? [];
  const owned = handlers.filter(hook => hook.source === 'sessionFlags' && hookCommands.has(hook.command));
  if (owned.length !== expectedHookCount) throw new Error('guidance-hooks-unavailable');
  const trust = { ...(session.hooks?.state ?? {}) };
  for (const hook of owned) trust[hook.key] = { ...trust[hook.key], enabled: true, trusted_hash: hook.currentHash };
  return [...selected, `hooks.state=${toml(trust)}`];
}
// Compose retained capabilities through the native discovery and hook path.
// The outer Serena catalog is connection evidence only. Its backend receives
// effective permissions on the first native tool call, never from this bootstrap.
const serenaName = 'gestalt-serena';
const serenaSkill = state?.skills.find(entry => entry.name === 'gestalt:serena');
const serenaReserved = servers[serenaName];
const serenaConflict = serenaReserved && (serenaReserved.env && Object.keys(serenaReserved.env).length ||
  serenaReserved.enabled === false || serenaReserved.enabled_tools || serenaReserved.disabled_tools?.length ||
  Object.values(serenaReserved.tools ?? {}).some(tool => tool.enabled !== undefined));
const serenaAliases = Object.entries(servers).filter(([name, config]) => name !== serenaName &&
  config.command === manager && config.args?.[0] === 'serena' && config.args?.[1] === 'mcp').map(([name]) => name);
const aliases = serenaAliases.length ? [`mcp_servers=${toml(Object.fromEntries(serenaAliases.map(name => [name, { enabled: false }])))}`] : [];
const capabilities = [{ name: serenaName, skillName: 'gestalt:serena', skill: serenaSkill, valid: false,
    context: '<gestalt_serena_unavailable>\nSerena is unavailable in this runtime. Earlier Serena guidance is inactive; use native code tools.\n</gestalt_serena_unavailable>',
    config: { command: manager, args: ['serena', 'mcp', '--cwd', cwd] } }];
for (const cap of capabilities) cap.inactiveContext = cap.context;
const diagnoseSerena = reason => process.stderr.write(`gestalt: serena unavailable (${reason}); continuing with native code tools\n`);
const usableGuidance = async entry => {
  const path = await realpath(entry.path);
  const manifest = JSON.parse(await readFile(resolve(path, '../../../.codex-plugin/plugin.json'), 'utf8'));
  const nativeVersion = /(?:codex[^0-9]*)?0\.(\d+)\./.exec(state.version);
  if (manifest.name !== 'gestalt' || manifest.skills !== './skills/' || !nativeVersion || Number(nativeVersion[1]) < 160)
    throw new Error('incompatible-runtime');
  return await readFile(path, 'utf8');
};
const common = state && !remote && state.config?.skills?.include_instructions !== false && state.config?.features?.hooks === true;
const startupNotice = reason => process.stderr.write(`gestalt: startup guidance unavailable (${reason}); retaining legacy instructions\n`);
// The selected native skill catalog identifies the installed package. Never
// select a cache version by directory order or consult a source checkout.
async function selectedStartupHook() {
  if (!common) return;
  const nativeVersion = /0\.(\d+)\./.exec(state.version);
  if (!nativeVersion || Number(nativeVersion[1]) < 160) { startupNotice('unsupported-runtime'); return; }
  const selected = state.skills.filter(entry => entry.name.startsWith('gestalt:') && entry.enabled !== false &&
    [...(state.config.skills?.config ?? []), ...rules].filter(rule => rule.name === entry.name || rule.path === entry.path).at(-1)?.enabled !== false);
  if (!selected.length) return;
  try {
    const roots = new Set(await Promise.all(selected.map(async entry => resolve(await realpath(entry.path), '../../..'))));
    if (roots.size !== 1) { startupNotice('ambiguous-selected-package'); return; }
    const packageRoot = [...roots][0];
    const manifest = JSON.parse(await readFile(resolve(packageRoot, '.codex-plugin/plugin.json'), 'utf8'));
    if (manifest.name !== 'gestalt' || manifest.skills !== './skills/') throw Error('invalid-package');
    let descriptor;
    try { descriptor = JSON.parse(await readFile(resolve(packageRoot, 'gestalt-startup-contract.json'), 'utf8')); }
    catch (error) { if (error.code === 'ENOENT') { startupNotice('missing-contract'); return; } throw error; }
    if (descriptor.schemaVersion !== 1 || descriptor.blockVersion !== 1) { startupNotice('unsupported-contract'); return; }
    if (descriptor.composer !== 'scripts/startup-contract.mjs') throw Error('invalid-composer');
    const { pathToFileURL } = await import('node:url');
    const { createHash } = await import('node:crypto');
    const composer = pathToFileURL(resolve(packageRoot, descriptor.composer)).href;
    const { readStartupContract, composeStartup } = await import(composer);
    const contract = await readStartupContract(packageRoot);
    if (contract.status !== 'ready') { startupNotice(contract.reason === 'unsupported-contract' ? 'unsupported-contract' : 'missing-contract'); return; }
    const block = composeStartup('', contract);
    const effective = state.config.developer_instructions ?? '';
    // A scalar owned by native config is immutable through this hook channel.
    // Reuse a current single block there; stale/duplicate blocks need an
    // explicit migration, never an additional hook copy or scalar override.
    const composed = composeStartup(effective, contract);
    const rootNeedsBlock = !effective.includes('<!-- gestalt:startup');
    if (effective.includes('<!-- gestalt:startup')) {
      if (composed !== effective) startupNotice('migration-required');
    }
    // Native standalone role names come from TOML, not filenames. Retain only
    // decisions and fingerprints; never serialize private role instructions.
    // Multiple matching definitions are left to native precedence and omitted
    // from bootstrap delivery with a bounded compatibility diagnostic.
    const { readdir } = await import('node:fs/promises');
    const rolePolicies = Object.create(null);
    const roleFolders = new Set([resolve(environment.CODEX_HOME, 'agents'), ...state.layers
      .filter(layer => layer.name?.type === 'project' && !layer.disabledReason && layer.name.dotCodexFolder)
      .map(layer => resolve(layer.name.dotCodexFolder, 'agents'))]);
    for (const folder of roleFolders) {
      let names;
      try { names = await readdir(folder); } catch (error) { if (error.code === 'ENOENT') continue; throw error; }
      for (const name of names.filter(name => name.endsWith('.toml')).sort()) {
        const path = resolve(folder, name), text = await readFile(path, 'utf8');
        const role = gestaltProfileToml.parse(text);
        if (typeof role.name !== 'string' || typeof role.developer_instructions !== 'string') continue;
        let action = 'append';
        try {
          if (role.developer_instructions.includes('<!-- gestalt:startup'))
            action = composeStartup(role.developer_instructions, contract) === role.developer_instructions ? 'reuse' : 'migration-required';
        } catch { action = 'migration-required'; }
        const policy = { path, digest: createHash('sha256').update(text).digest('hex'), action };
        rolePolicies[role.name] = rolePolicies[role.name] ? { action: 'ambiguous-role' } : policy;
      }
    }
    const digest = createHash('sha256').update(block).digest('hex');
    async function emitStartup(composer, packageRoot, digest, rootNeedsBlock, roles) {
      let reason = 'package-changed';
      try {
        let raw = '';
        for await (const chunk of process.stdin) { raw += chunk; if (raw.length > 16384) throw Error('invalid-hook-input'); }
        const input = JSON.parse(raw);
        if (input.hook_event_name === 'SessionStart' && !rootNeedsBlock) return;
        if (input.hook_event_name === 'SubagentStart') {
          const role = roles[input.agent_type ?? 'default'];
          if (role) {
            if (role.action === 'ambiguous-role') { reason = 'ambiguous-role'; throw Error('ambiguous-role'); }
            reason = 'role-config-changed';
            const { readFile } = await import('node:fs/promises');
            const { createHash } = await import('node:crypto');
            if (!role.path || createHash('sha256').update(await readFile(role.path)).digest('hex') !== role.digest) throw Error('role-config-changed');
            if (role.action === 'reuse') return;
            if (role.action !== 'append') { reason = role.action; throw Error('role-migration'); }
          } else if (!['default', 'worker', 'explorer'].includes(input.agent_type ?? 'default')) {
            reason = 'role-discovery-unavailable'; throw Error('unknown-role');
          } else if (!rootNeedsBlock) return;
        }
        reason = 'package-changed';
        const { readStartupContract, composeStartup } = await import(composer);
        const { createHash } = await import('node:crypto');
        const contract = await readStartupContract(packageRoot);
        if (contract.status !== 'ready') throw Error('package-changed');
        const block = composeStartup('', contract);
        if (createHash('sha256').update(block).digest('hex') !== digest) throw Error('package-changed');
        process.stdout.write(block);
      } catch {
        process.stderr.write(`gestalt: startup guidance unavailable (${reason}); retaining legacy instructions\n`);
      }
    }
    // Only package identity and a digest enter argv. Canonical stable text and
    // effective user instructions are read in-process, never pasted into flags.
    return nodeHookCommand(`await (${emitStartup.toString()})(${JSON.stringify(composer)},${JSON.stringify(packageRoot)},${JSON.stringify(digest)},${rootNeedsBlock},${JSON.stringify(rolePolicies)})`,process.execPath);
  } catch { startupNotice('invalid-contract-or-instructions'); }
}
startupHook = await selectedStartupHook();
// Managed browser consent is native named-profile state, independent of
// package installation and of the supervisor's later activation request.
const playwrightName = 'gestalt-playwright';
const playwrightSkill = state?.skills.find(entry => entry.name === 'gestalt:playwright');
const playwrightConfig = servers[playwrightName];
const playwrightConsent = Boolean(namedProfile && playwrightConfig?.enabled === true);
const playwrightRequested = Number(environment.GESTALT_PLAYWRIGHT_READY_TIMEOUT_MS ?? 5000);
const playwrightBudget = Number.isInteger(playwrightRequested) && playwrightRequested >= 100 && playwrightRequested <= 60000 ? playwrightRequested : 5000;
const playwrightRules = [...(state?.config?.skills?.config ?? []), ...rules].filter(rule =>
  rule.name === 'gestalt:playwright' || rule.path === playwrightSkill?.path);
const playwrightNotice = reason => process.stderr.write(`gestalt: playwright unavailable (${reason}); continuing without browser access\n`);
const refreshPlaywrightConsent = async () => {
  try {
    const deadline = Date.now() + playwrightBudget;
    const current = await resolveNamedProfile(await snapshot([], false, false, deadline), deadline, false);
    return Boolean(namedProfile && current?.config?.mcp_servers?.[playwrightName]?.enabled === true);
  } catch { return false; }
};
const playwrightAliases = Object.entries(servers).filter(([name, config]) => name !== playwrightName &&
  config.command === manager && config.args?.[0] === 'playwright' && config.args?.[1] === 'mcp').map(([name]) => name);
if (playwrightAliases.length) aliases.push(`mcp_servers=${toml(Object.fromEntries(playwrightAliases.map(name => [name, { enabled: false }])))}`);

// The browser's host controller belongs to its native component milestone.
// Consent does not manufacture a working controller in this core artifact.
if (playwrightConsent) playwrightNotice('native-browser-component-required');
// Serena honors native skill exclusions; absence from the selected plugin catalog
// also prevents connection. Do not manufacture a globally installed skill path.
const serenaRules = [...(state?.config?.skills?.config ?? []), ...rules].filter(rule =>
  rule.name === 'gestalt:serena' || rule.path === serenaSkill?.path);
// Component installation metadata is not connection proof. L5 supplies the
// native Serena handler; until then keep guidance/servers explicitly inactive.
if (common && serenaSkill && serenaSkill.enabled !== false && serenaRules.at(-1)?.enabled !== false)
  diagnoseSerena('native-serena-component-required');
if (interrupted) return interruptionCode;
if (capabilities[0].valid) readinessDeadline = Math.max(readinessDeadline, Date.now() + budget);
const compose = (includeGeneratedHooks = true) => {
  hookCommands.clear();
  expectedHookCount = 0;
  const approved = managedMcpApprovals(state?.config ?? {},{manager,codexHome:environment.CODEX_HOME});
  const approvalFlags = [];
  for (const [name,server] of Object.entries(approved.mcp_servers ?? {})) if (isGestaltManagedMcp(name,server,{manager,codexHome:environment.CODEX_HOME})) {
    // Native config flags replace this server table. A policy-only dotted
    // override can discard its transport and prevent the hook-trust query.
    approvalFlags.push(`mcp_servers.${name}=${toml(server)}`);
  }
  for (const [name,plugin] of Object.entries(approved.plugins ?? {})) if (name === 'context-mode@dyne-gestalt-agents')
    for (const [server,options] of Object.entries(plugin.mcp_servers ?? {})) if (server === 'context-mode') {
      approvalFlags.push(`plugins.${name}.mcp_servers.${server}.default_tools_approval_mode="approve"`);
      for (const [tool,setting] of Object.entries(options.tools ?? {})) if (setting?.approval_mode === 'approve')
        approvalFlags.push(`plugins.${name}.mcp_servers.${server}.tools.${tool}.approval_mode="approve"`);
    }
  const selectors = [...capabilities.flatMap(cap => cap.valid ? [{ name: cap.skillName, enabled: true }] :
    [...(cap.skill ? [{ path: cap.skill.path, enabled: false }] : []), { name: cap.skillName, enabled: false }]),
    { name: 'gestalt:playwright', enabled: false }];
  const deniedSelectors = state || rules.length ? toml([...rules, ...selectors]) :
    rawSkillConfig?.startsWith('[') && rawSkillConfig.endsWith(']')
      ? `${rawSkillConfig.slice(0, -1).replace(/,\s*$/, '')}${rawSkillConfig.slice(1, -1).trim() ? ',' : ''}${toml(selectors).slice(1, -1)}]`
      : toml(selectors);
  const browserOff = { ...playwrightConfig, command: playwrightConfig?.command ?? manager,
    args: playwrightConfig?.args ?? ['playwright', 'mcp', '--cwd', cwd], enabled:false, required:false };
  const browserPolicy = isGestaltManagedMcp(playwrightName,browserOff,{manager,codexHome:environment.CODEX_HOME})
    ? managedToolApproval(browserOff) : browserOff;
  return [...approvalFlags, ...aliases, ...capabilities.map(cap => `mcp_servers.${cap.name}=${toml(cap.valid ? managedToolApproval(cap.config) :
    { command: manager, args: cap.config.args, enabled: false, required: false })}`), `skills.config=${deniedSelectors}`,
    `mcp_servers.${playwrightName}=${toml(browserPolicy)}`,
    ...(state && includeGeneratedHooks ? sessionHook(capabilities.map(cap => cap.valid ? cap.context : cap.inactiveContext).join('\n')) : [])];
};
async function selection() {
  if(cliOwned&&state?.config?.features?.hooks!==true)protectUnknownMapping();
  let selected = compose();
  try { if (state?.config?.features?.hooks === true) selected = await trustedHooks(selected); }
  catch {
    diagnose('guidance-hooks-unavailable');
    if (startupHook) { startupNotice('guidance-hooks-unavailable'); startupHook = undefined; }
    protectUnknownMapping();
    for (const cap of capabilities) cap.valid = false;
    // An unsuccessful query supplies no trust authority. Preserve the user's
    // original hooks/flags and retain unknown mapping, without creating fresh
    // untrusted manager hooks in the fallback launch.
    selected = compose(false);
  }
  return selected;
}
let result = await launch(appendFlags(await selection()));
// Drop only the failed optional connection, preserving the independent service.
// Retry only the selected retained service after a connection failure.
for (let attempt = 0; result.code !== 0 && !interrupted && attempt < capabilities.length; attempt++) {
  const failed = capabilities.filter(cap => cap.valid && result.startupFailed.has(cap.name));
  if (!failed.length) break;
  for (const cap of failed) {
    cap.valid = false;
    diagnoseSerena('runtime-connection-failed');
  }
  readinessDeadline = Date.now() + budget;
  result = await launch(appendFlags(await selection()));
}
return interrupted ? interruptionCode : result.code;
} finally {
  await commands.stop(); await writing;
  for (const [signal,listener] of listeners) globalThis.process.removeListener(signal,listener);
}
}
