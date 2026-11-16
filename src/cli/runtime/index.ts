import type { CliMetadata, CommandHandlers } from '../dispatch.ts';
import { createDiagnosticHandlers } from './diagnostics.ts';
import { skillsHandler, skillsTransferHandler } from './skills.ts';
import { scratchHandler, scratchHookHandler } from './scratch.ts';
import { appServerHandler } from './app-server.ts';
import { cliHandler } from './cli.ts';
import { mobileHostSupport } from '../setup/mobile.ts';
import type { RuntimeTerminal } from './commands.ts';

export function createRuntimeHandlers(metadata: CliMetadata, terminal?: RuntimeTerminal): CommandHandlers {
  return { ...createDiagnosticHandlers(metadata), skills: skillsHandler(metadata),
    'skills-export': skillsTransferHandler(metadata, 'export'), 'skills-import': skillsTransferHandler(metadata, 'import'),
    scratch: scratchHandler, '__scratch-hook': scratchHookHandler, '__codex-app-server': appServerHandler(),
    cli: cliHandler(metadata, false, undefined, terminal), '__cli-owned': cliHandler(metadata, true, undefined, terminal),
    mobile: async (invocation, streams) => {
      // The selected Mobile owner must supply its host-side/native lifecycle
      // boundary in L7. Never run the whole filesystem owner in a mapped Linux
      // namespace or substitute raw host spawn/PID authority in this artifact.
      const obsolete = invocation.argv.some(token => token === '--xerj' || token.startsWith('--xerj='));
      const support = mobileHostSupport();
      await streams.stderr(Buffer.from(obsolete ? 'gestalt: Xerj has been retired; omit --xerj when starting Mobile. User data is retained.\n' :
        `gestalt: Mobile runtime unavailable: ${support.prerequisite || 'a native Mobile host with host-side filesystem orchestration, owned command lifetimes and authenticated restart authority is required'}; Gestalt CLI and Agents remain available\n`));
      return 1;
    } };
}
