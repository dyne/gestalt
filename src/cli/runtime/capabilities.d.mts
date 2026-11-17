import type { CommandInvocation } from '../contracts.ts';
import type { RuntimeCommands } from './commands.ts';
export function launchCapabilities(commands: RuntimeCommands, invocation: CommandInvocation, manager: string): Promise<number>;
