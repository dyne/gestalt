import path from 'node:path';
import { setupPaths } from '../setup/context.ts';
import type { CommandHandler, OwnedProcessPort } from '../contracts.ts';
import { cliScratch } from './scratch.ts';
import { scratchReleaseReady, scratchContext, scratchEnvironment } from './scratch-runtime.mjs';
import { DiagnosticContext, verifyContextRuntime } from './diagnostics.ts';
import { resultStatus, RuntimeCommands, type RuntimeTerminal } from './commands.ts';
import { launchCapabilities } from './capabilities.mjs';

const administrative = ['-h', '--help', '-V', '--version', 'help', 'login', 'logout', 'plugin', 'mcp', 'doctor', 'features', 'completion'];
export function cliHandler(owned = false, port?: OwnedProcessPort, terminal?: RuntimeTerminal): CommandHandler {
  return async (invocation, streams) => {
    let commands: RuntimeCommands | undefined, unlisten: (() => void) | undefined;
    let primary: ReturnType<typeof cliScratch>['ownership'] = null;
    let prepared = false;
    try {
      const argv = invocation.argv[0] === '--' ? invocation.argv.slice(1) : invocation.argv;
      const admin = administrative.includes(argv[0] ?? '');
      const paths = await setupPaths(invocation.environment);
      const inherited = { ...invocation.environment };
      const pathKeys = Object.keys(inherited).filter(key => process.platform === 'win32' ? key.toUpperCase() === 'PATH' : key === 'PATH');
      if (pathKeys.length > 1) throw new Error('Ambiguous case aliases for PATH');
      const operatorPath = pathKeys[0] ? inherited[pathKeys[0]] : undefined;
      for (const key of pathKeys) delete inherited[key];
      let selected = { ...invocation, argv, environment: scratchEnvironment({ ...inherited, CODEX_HOME: paths.codexHome,
        GESTALT_HOME: paths.gestaltHome, GESTALT_MANAGER_BIN: paths.manager,
        PATH: [...(process.platform === 'win32' && invocation.environment.SystemRoot ? [path.join(invocation.environment.SystemRoot, 'System32/WindowsPowerShell/v1.0')] : []),
          path.join(paths.gestaltHome, 'bin'), path.join(paths.codexHome, 'bin'), operatorPath].filter(Boolean).join(path.delimiter),
        GESTALT_STARTUP_DIAGNOSTICS_DONE: '1' }) };
      if (!owned && !admin) {
        const scratch = cliScratch(selected); selected = scratch.invocation; primary = scratch.ownership;
        if (!scratchContext(selected.environment)) await streams.stderr(Buffer.from('gestalt: no selected workspace; CLI temporary ownership unavailable (select with -C)\n'));
      }
      commands = new RuntimeCommands(selected, streams, paths.manager, port, terminal); unlisten = commands.listen();
      const codex = await commands.resolve('codex'); if (!codex) throw new Error('codex is required and was not found on PATH');
      const required = new DiagnosticContext(paths, selected, streams, port, true), unlistenDiagnostic = required.commands.listen();
      try {
        await verifyContextRuntime(required);
        if (required.commands.interrupted) return required.commands.interrupted;
      } finally { await required.commands.stop(); unlistenDiagnostic(); }
      await streams.stderr(Buffer.from(`gestalt: starting Codex with CODEX_HOME=${paths.codexHome}\n`));
      if (admin) {
        prepared = true;
        const lease = await commands.launch(codex, argv);
        const result = await lease.result; await lease.stop();
        const status = commands.interrupted || resultStatus(result);
        return status || (commands.unknown.length && process.platform !== 'darwin' ? 1 : 0);
      }
      // Required runtime identities remain exact. No model, version, approval or
      // permission admission policy is added to native Codex invocation.
      prepared = true;
      const status = await launchCapabilities(commands, selected, paths.manager);
      return status || (commands.unknown.length && process.platform !== 'darwin' ? 1 : 0);
    } catch (error) {
      await streams.stderr(Buffer.from(`gestalt: ${error instanceof Error ? error.message : 'native CLI failed'}\n`)); return commands?.interrupted || 1;
    } finally {
      await commands?.stop(); unlisten?.();
      if (primary) {
        // Before any launch, ordinary validation failures can safely retire the
        // new controller. After launch only native matching release proofs do.
        if ((!prepared || commands && !commands.active.size && !commands.unknown.length) && !await scratchReleaseReady(primary))
          await streams.stderr(Buffer.from('gestalt: CLI temporary owner remains protected by an active or unknown borrower\n'));
        else if (prepared && commands?.unknown.length) await streams.stderr(Buffer.from('gestalt: CLI result observed; complete descendant release is unconfirmed; temporary owner remains protected\n'));
      }
    }
  };
}
