import path from 'node:path';
import type { CommandHandler, CommandInvocation } from '../contracts.ts';
import { scratchAllocate, scratchAssociate, scratchContext, scratchEnvironment, scratchRecoveryCommand,
  type ScratchOwnership } from './scratch-owner.mjs';

export const scratchHelp = 'Stop Mobile and all runtimes/tools using this owner before recovery.\n' +
  'gestalt scratch inspect --cwd ROOT --owner ID\n' +
  'gestalt scratch recover --cwd ROOT --owner ID --expect TOKEN --confirm-quiescent\n' +
  'gestalt scratch inspect-tool-leases --cwd ROOT --owner ID\n' +
  'gestalt scratch recover-tool-leases --cwd ROOT --owner ID --expect TOKEN --confirm-quiescent\n' +
  'gestalt scratch inspect-publication --cwd ROOT\n' +
  'gestalt scratch recover-publication --cwd ROOT --expect TOKEN --confirm-quiescent\n' +
  'Use the current inspection token. Recovery is an explicit offline operator assertion, never a process absence proof. Payload and provider history are retained.\n';
export const scratchHandler: CommandHandler = async (invocation, streams) => {
  try {
    scratchEnvironment(invocation.environment);
    if (invocation.argv.length === 1 && ['-h', '--help'].includes(invocation.argv[0]!)) { await streams.stdout(Buffer.from(scratchHelp)); return 0; }
    let output = '';
    scratchRecoveryCommand(invocation.argv, text => { output += text; });
    await streams.stdout(Buffer.from(output)); return 0;
  } catch (error) { await streams.stderr(Buffer.from(`gestalt: ${error instanceof Error ? error.message : 'scratch operation failed'}\n`)); return 1; }
};
export const scratchHookHandler: CommandHandler = async (invocation, streams) => {
  try {
    let input = Buffer.alloc(0);
    for await (const chunk of streams.stdin) {
      if (input.length + chunk.length > 16384) throw new Error('SESSION_SCRATCH_INVALID_CONTEXT');
      input = Buffer.concat([input, chunk]);
    }
    const value = JSON.parse(input.toString('utf8'));
    if (!['SessionStart', 'SubagentStart'].includes(value?.hook_event_name)) throw new Error('SESSION_SCRATCH_INVALID_CONTEXT');
    scratchAssociate(invocation.environment, value.hook_event_name === 'SubagentStart' ? value.agent_id : value.session_id);
    return 0;
  } catch {
    // Missing association protects the whole owner rather than manufacturing
    // a provider identity from argv, a resume selector, transcript or a PID.
    const { scratchBorrow } = await import('./scratch-owner.mjs');
    try { const context = scratchContext(invocation.environment); if (context) scratchBorrow(context, 'cli-provider-association-failed'); } catch { /* validation never grants write authority */ }
    await streams.stderr(Buffer.from('gestalt: native provider association unavailable; CLI temporary owner remains protected\n')); return 1;
  }
};

/** Match the native selector grammar without consuming stdin or interpreting
 * native thread/resume IDs. One explicit workspace creates one invocation UUID. */
export function selectedWorkspace(invocation: CommandInvocation): string | undefined {
  let selected: string | undefined;
  const args = invocation.argv;
  for (let index = 0; index < args.length && args[index] !== '--'; index++) {
    const arg = args[index]!;
    if (['-c', '--config', '-p', '--profile', '--model', '-m', '--sandbox', '-s', '--ask-for-approval', '-a', '--image', '-i', '--permissions', '--remote', '--add-dir', '--local-provider'].includes(arg)) { index++; continue; }
    if (arg === '-C' || arg === '--cd') {
      if (args[index + 1] === undefined) throw new Error('native workspace selector requires a directory');
      selected = path.resolve(selected ?? invocation.cwd, args[++index]!);
    } else if (arg.startsWith('--cd=')) selected = path.resolve(selected ?? invocation.cwd, arg.slice(5));
    else if (arg.startsWith('-C') && arg.length > 2) selected = path.resolve(selected ?? invocation.cwd, arg.slice(2));
  }
  return selected;
}
export function cliScratch(invocation: CommandInvocation): { invocation: CommandInvocation; ownership: ScratchOwnership | null } {
  const inherited = scratchEnvironment(invocation.environment);
  if (Object.hasOwn(inherited, 'GESTALT_SESSION_TMP_CONTEXT')) return { invocation: { ...invocation, environment: inherited }, ownership: null };
  const workspace = selectedWorkspace(invocation);
  if (!workspace) return { invocation, ownership: null };
  const ownership = scratchAllocate(workspace, 'cli-session', 'cli-controller');
  return { ownership, invocation: { ...invocation, environment: scratchEnvironment({ ...invocation.environment, GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(ownership.context) }) } };
}
