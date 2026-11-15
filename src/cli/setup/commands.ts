import path from 'node:path';
import type { CommandHandler, OwnedProcessPort } from '../contracts.ts';
import type { CliMetadata, CommandHandlers } from '../dispatch.ts';
import { setupAgents } from './agents.ts';
import { CommandFailure, optionalStat, setupPaths, SetupContext } from './context.ts';
import { installMobile } from './mobile.ts';
import { updateManager } from './manager-update.ts';
import { retireManagedConfiguration } from './retirement.mjs';
import { restartWorker, scheduleRestart } from './restart.ts';
import { scratchEnvironment } from './scratch-context.mjs';

declare const GESTALT_BUILD_SKILLS_PROGRAM: string;
export type UpdateComponent = 'all' | 'gestalt' | 'serena' | 'playwright';
export interface UpdateOptions { readonly component: UpdateComponent; readonly extra: boolean; readonly help: boolean }
const retirement = 'Xerj has been retired; run gestalt update to migrate managed configuration. User data is retained.';
export function parseUpdateOptions(argv: readonly string[]): UpdateOptions {
  let component: string = 'all', selected = false, extra = false;
  for (let index = 0; index < argv.length; index++) {
    const token = argv[index]!;
    if (token === 'xerj') throw new Error(retirement);
    if (token === '--extra-skills') { extra = true; continue; }
    if (token === '--help' || token === '-h') return { component: component as UpdateComponent, extra, help: true };
    if (token === '--component') {
      if (!argv[index + 1]) throw new Error('--component requires all, gestalt, serena, or playwright');
      if (selected) throw new Error('select only one update component');
      component = argv[++index]!; selected = true;
    } else if (['all', 'gestalt', 'serena', 'playwright'].includes(token)) {
      if (selected) throw new Error('select only one update component');
      component = token; selected = true;
    } else throw new Error(`unknown update option or component: ${token} (run 'gestalt update --help')`);
  }
  if (component === 'xerj') throw new Error(retirement);
  if (!['all', 'gestalt', 'serena', 'playwright'].includes(component)) throw new Error(`unknown update component: ${component} (choose all, gestalt, serena, or playwright)`);
  if (extra && !['all', 'gestalt'].includes(component)) throw new Error('--extra-skills requires the all or gestalt update component');
  return { component: component as UpdateComponent, extra, help: false };
}
export function parseSetupOptions(argv: readonly string[]): { extra: boolean; help: boolean } {
  let extra = false;
  for (const token of argv) {
    if (token === '--extra-skills') extra = true;
    else if (token === '--help' || token === '-h') return { extra, help: true };
    else throw new Error(`unknown setup option: ${token}`);
  }
  return { extra, help: false };
}
export interface SetupOperations {
  agents(context: SetupContext, mode: 'install' | 'upgrade', extra: boolean): Promise<void>;
  skills(context: SetupContext, mode: 'install' | 'update'): Promise<void>;
  mobile(context: SetupContext): Promise<unknown>;
  manager(context: SetupContext, argv: readonly string[], resumed: boolean): Promise<boolean>;
  component(context: SetupContext, component: 'serena' | 'playwright'): Promise<void>;
}
const defaults: SetupOperations = {
  agents: setupAgents,
  skills: async (context, mode) => {
    await context.require('git');
    const unlisten = context.listen();
    try { await context.run(process.execPath, ['--input-type=commonjs', '-', mode, context.paths.codexHome,
      'https://github.com/dyne/gestalt-skills', 'main', 'text'], { program: GESTALT_BUILD_SKILLS_PROGRAM }); }
    finally { unlisten(); }
  },
  mobile: context => installMobile(context), manager: updateManager,
  component: async (_context, component) => { throw new Error(`native ${component} update is not available in this development artifact`); },
};

export async function runUpdate(context: SetupContext, options: UpdateOptions, originalArgv: readonly string[],
  resumed: boolean, operations: SetupOperations = defaults): Promise<boolean> {
  if (options.component === 'all' || options.component === 'gestalt') {
    if (await operations.manager(context, originalArgv, resumed)) return true;
    await context.prepareHomes();
    // Retire only proven managed configuration; user data and binaries survive.
    for (const notice of retireManagedConfiguration(context.paths.codexHome, context.paths.gestaltHome, context.paths.manager))
      await context.log(`Xerj retirement: ${notice} preserved; review legacy Xerj entries/processes manually, then retry update. User data and binaries are retained.`);
    await operations.skills(context, 'update');
    await operations.agents(context, 'upgrade', options.extra);
    const mobile = await operations.mobile(context);
    const summary = mobile === false ? 'Agents, optional skills, and context-mode' : 'Agents, optional skills, Mobile, and context-mode';
    await context.log(context.environment.GESTALT_MANAGER_DISTRIBUTION === 'npm'
      ? `${summary} are up to date; update the manager with npm install -g @jaromil/gestalt@latest`
      : `manager, ${summary} are up to date`);
  }
  for (const component of ['serena', 'playwright'] as const) {
    if (options.component === component || (options.component === 'all' && await optionalStat(path.join(context.paths.gestaltHome, component, 'active.json'))))
      await operations.component(context, component);
    else if (options.component === 'all') await context.log(component === 'serena'
      ? 'skipping Serena (not installed; use gestalt update serena to install)' : 'skipping Playwright (not installed; use gestalt playwright install)');
  }
  await context.log(`update complete (${options.component})`);
  return false;
}

export interface ManagedSetup {
  readonly commands: OwnedProcessPort;
  readonly signal: AbortSignal;
  unknown(): void;
}
export function createSetupHandlers(metadata: CliMetadata, operations: SetupOperations = defaults, resumed = false,
  managed?: ManagedSetup): CommandHandlers {
  const wrap = (command: string, body: (context: SetupContext, argv: readonly string[]) => Promise<number>): CommandHandler => async (invocation, streams) => {
    let context: SetupContext | undefined;
    try {
      const environment = scratchEnvironment(invocation.environment);
      context = new SetupContext({ ...invocation, environment }, streams, await setupPaths(environment), metadata.version,
        managed?.commands, managed?.signal);
      context.check();
      return await body(context, invocation.argv);
    } catch (error) {
      await streams.stderr(Buffer.from(`gestalt: ${error instanceof Error ? error.message : `${command} failed`}\n`));
      return error instanceof CommandFailure ? error.status : 1;
    } finally {
      if (context) {
        await context.stopActive();
        if (!context.canRollback()) managed?.unknown();
      }
    }
  };
  return {
    agents: wrap('agents', async (context, argv) => {
      if (!argv.length || (argv.length === 1 && ['help', '-h', '--help'].includes(argv[0]!))) {
        await context.streams.stdout(Buffer.from('Usage: gestalt agents install|update\nInstall or reconcile Gestalt Agents and their context-mode runtime.\n')); return 0;
      }
      if (argv.length !== 1 || !['install', 'update'].includes(argv[0]!)) throw new Error('usage: gestalt agents install|update');
      await operations.agents(context, argv[0] === 'install' ? 'install' : 'upgrade', false); return 0;
    }),
    install: wrap('install', async (context, argv) => {
      const options = parseSetupOptions(argv);
      if (options.help) { await context.streams.stdout(Buffer.from(metadata.help)); return 0; }
      await context.prepareHomes();
      await operations.skills(context, 'install');
      await operations.agents(context, 'install', options.extra);
      const mobile = await operations.mobile(context);
      await context.log(mobile === false ? "installation complete; run 'gestalt doctor', then 'gestalt cli'"
        : "installation complete; run 'gestalt doctor', then 'gestalt cli' or 'gestalt mobile'"); return 0;
    }),
    update: wrap('update', async (context, argv) => {
      const options = parseUpdateOptions(argv);
      if (options.help) { await context.streams.stdout(Buffer.from(metadata.help)); return 0; }
      await runUpdate(context, options, ['update', ...argv], resumed, operations); return 0;
    }),
    'update-restart': wrap('update-restart', async (context, argv) => {
      const options = parseUpdateOptions(argv);
      if (options.help) { await context.streams.stdout(Buffer.from(metadata.help)); return 0; }
      await scheduleRestart(context, options.component, options.extra); return 0;
    }),
    '__updated-manager': async (invocation, streams) => {
      const [command, ...argv] = invocation.argv;
      const handlers = createSetupHandlers(metadata, operations, true, managed);
      const handler = command && Object.hasOwn(handlers, command) ? handlers[command as keyof CommandHandlers] : undefined;
      if (!handler || command === '__updated-manager') { await streams.stderr(Buffer.from('gestalt: invalid updated-manager continuation\n')); return 1; }
      return handler({ ...invocation, argv }, streams);
    },
    '__update-restart-worker': wrap('__update-restart-worker', async (context, argv) =>
      restartWorker(context, argv, (worker, component, extra) => runUpdate(worker,
        parseUpdateOptions([component, ...(extra ? ['--extra-skills'] : [])]), ['__update-restart-worker', ...argv], resumed, operations))),
  };
}
