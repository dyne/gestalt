import type { CommandHandler } from '../contracts.ts';
import type { CliMetadata } from '../dispatch.ts';
import { SetupContext, setupPaths, CommandFailure } from '../setup/context.ts';
import { scratchEnvironment } from '../setup/scratch-context.mjs';
import { manageSkills } from './skills-owner.mjs';
import { exportSkills, importSkills } from './skills-transfer.ts';

export const skillsHelp = 'Usage: gestalt skills install|update|status [--json]\nInstall/update maintain only the optional collection; choose activation in skill profiles.\nStatus reads current version, revision and integrity without changing files or downloading.\nRetained transactions: gestalt skills inspect-recovery; then gestalt skills recover --expect TOKEN --confirm-quiescent after stopping every user of this owner.\n';
export function skillsHandler(metadata: CliMetadata): CommandHandler {
  return async (invocation, streams) => {
    let context: SetupContext | undefined, unlisten: (() => void) | undefined;
    try {
      const [operation = 'help', ...options] = invocation.argv;
      if (['help', '--help', '-h'].includes(operation) || (options.length === 1 && ['--help', '-h'].includes(options[0]!))) {
        await streams.stdout(Buffer.from(skillsHelp)); return 0;
      }
      let mode: 'text' | 'json' = 'text', recovery = null;
      if (operation === 'status' && options.length === 1 && options[0] === '--json') mode = 'json';
      else if (operation === 'recover') {
        if (options.length !== 3 || options[0] !== '--expect' || !/^[a-f0-9]{64}$/.test(options[1]!) || options[2] !== '--confirm-quiescent')
          throw new Error('usage: gestalt skills recover --expect TOKEN --confirm-quiescent; stop every runtime using this owner first');
        recovery = { token: options[1]!, confirmed: true };
      } else if (options.length || !['install', 'update', 'status', 'inspect-recovery'].includes(operation)) throw new Error(skillsHelp.trimEnd());
      const environment = scratchEnvironment(invocation.environment);
      context = new SetupContext({ ...invocation, environment }, streams, await setupPaths(environment), metadata.version);
      unlisten = context.listen();
      if (operation === 'install' || operation === 'update') await context.require('git');
      if (operation === 'recover') await context.storage(context.paths.codexHome);
      return await manageSkills(context, operation as 'install' | 'update' | 'status' | 'recover' | 'inspect-recovery', mode, recovery);
    } catch (error) {
      await streams.stderr(Buffer.from(`gestalt: skills: ${error instanceof Error ? error.message : 'maintenance failed'}\n`));
      return error instanceof CommandFailure ? error.status : 1;
    } finally { await context?.stopActive(); unlisten?.(); }
  };
}
export function skillsTransferHandler(metadata: CliMetadata, direction: 'import' | 'export'): CommandHandler {
  return async (invocation, streams) => {
    let context: SetupContext | undefined, unlisten: (() => void) | undefined;
    try {
      const environment = scratchEnvironment(invocation.environment);
      context = new SetupContext({ ...invocation, environment }, streams, await setupPaths(environment), metadata.version);
      unlisten = context.listen(); await (direction === 'import' ? importSkills : exportSkills)(context, invocation.argv); return 0;
    } catch (error) { await streams.stderr(Buffer.from(`gestalt: skills ${direction}: ${error instanceof Error ? error.message : 'transfer failed'}\n`)); return error instanceof CommandFailure ? error.status : 1; }
    finally { await context?.stopActive(); unlisten?.(); }
  };
}
