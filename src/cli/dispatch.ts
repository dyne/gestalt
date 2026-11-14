import { commandAliases, commandNames, internalCommandNames, retiredCommandNames } from './contracts.ts';
import type { CommandHandler, CommandInvocation, CommandName, CommandStreams } from './contracts.ts';

export interface CliMetadata { readonly version: string; readonly help: string }
export type CommandHandlers = Partial<Readonly<Record<CommandName, CommandHandler>>>;

/** Development composition seam. Later milestones replace each unfinished handler here. */
export async function runCli(
  invocation: CommandInvocation,
  streams: CommandStreams,
  metadata: CliMetadata,
  handlers: CommandHandlers = {},
): Promise<number> {
  const token = invocation.argv[0] ?? 'help';
  const alias = Object.hasOwn(commandAliases, token)
    ? commandAliases[token as keyof typeof commandAliases] : token;
  const write = (text: string) => streams.stdout(new TextEncoder().encode(text));
  const fail = async (text: string) => {
    await streams.stderr(new TextEncoder().encode(`gestalt: ${text}\n`));
    return 1;
  };
  if ((retiredCommandNames as readonly string[]).includes(token))
    return fail('Xerj has been retired; run gestalt update to migrate managed configuration. User data is retained.');
  if (alias === 'help') { await write(metadata.help); return 0; }
  if (alias === 'version') { await write(`gestalt ${metadata.version}\n`); return 0; }
  if (!(commandNames as readonly string[]).includes(alias)) {
    if ((internalCommandNames as readonly string[]).includes(token))
      return fail(`development handler is not implemented: ${token}`);
    return fail(`unknown command: ${token} (run 'gestalt help')`);
  }
  const handler = handlers[alias as CommandName];
  if (!handler) return fail(`development handler is not implemented: ${alias}`);
  return handler({ ...invocation, argv: invocation.argv.slice(1) }, streams);
}
