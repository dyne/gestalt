import type { CliMetadata, CommandHandlers } from '../dispatch.ts';
import { createDiagnosticHandlers } from './diagnostics.ts';
import { skillsHandler, skillsTransferHandler } from './skills.ts';
import { scratchHandler, scratchHookHandler } from './scratch.ts';
import { appServerHandler } from './app-server.ts';
import { cliHandler } from './cli.ts';
import { mobileHostSupport } from '../setup/mobile.ts';
import type { RuntimeTerminal } from './commands.ts';
import type { CommandHandler } from '../contracts.ts';

function deferredTool(name: string): CommandHandler {
  return async (_invocation, streams) => {
    await streams.stderr(Buffer.from(`gestalt: native ${name} support is deferred in this core release; use an existing supported Unix Gestalt installation for ${name}. Gestalt CLI and Agents remain available. No profile or installation was changed.\n`));
    return 1;
  };
}

export function createRuntimeHandlers(metadata: CliMetadata, terminal?: RuntimeTerminal, ownershipHelper?: string): CommandHandlers {
  return { ...createDiagnosticHandlers(metadata, undefined, ownershipHelper), skills: skillsHandler(metadata, ownershipHelper),
    'skills-export': skillsTransferHandler(metadata, 'export', ownershipHelper),
    'skills-import': skillsTransferHandler(metadata, 'import', ownershipHelper),
    scratch: scratchHandler, '__scratch-hook': scratchHookHandler, '__codex-app-server': appServerHandler(),
    cli: cliHandler(false, undefined, terminal), '__cli-owned': cliHandler(true, undefined, terminal),
    serena: deferredTool('serena'), playwright: deferredTool('playwright'),
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
