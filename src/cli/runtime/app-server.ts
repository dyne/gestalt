import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { PassThrough, Readable } from 'node:stream';
import { createInterface } from 'node:readline';
import path from 'node:path';
import type { CommandInvocation, CommandStreams, OwnedProcessPort } from '../contracts.ts';
import { managedMcpApprovals, managedMcpOverrides } from './approvals.mjs';
import { resultStatus, RuntimeCommands } from './commands.ts';
import { scratchEnvironment } from './scratch-owner.mjs';
import { setupPaths } from '../setup/context.ts';

/** Existing JSONL policy boundary with native ownership in place of a Bash
 * guardian/group leader. Native byte streams use their own pipes throughout. */
export async function proxyAppServer(invocation: CommandInvocation, streams: CommandStreams,
  manager: string, native: string, port?: OwnedProcessPort): Promise<number> {
  if (!path.isAbsolute(native)) throw new Error('native Codex executable must have an absolute path');
  const commands = new RuntimeCommands(invocation, streams, manager, port), unlisten = commands.listen();
  const incoming = new PassThrough(), outgoing = new PassThrough();
  const inputBytes = Readable.from(streams.stdin), input = createInterface({ input: inputBytes }), output = createInterface({ input: outgoing });
  // Clients may send initialize before the ownership helper is ready. readline
  // otherwise consumes and drops those lines before its async iterator starts.
  input.pause();
  const pending = new Map<string, { resolve(value: any): void; reject(error: Error): void }>(), reads = new Set<unknown>();
  const prefix = 'gestalt-approvals-' + randomUUID() + '-'; let sequence = 0, failed = false, ending = false, shutdownStatus: number | undefined;
  let lease: Awaited<ReturnType<RuntimeCommands['launch']>> | undefined;
  let writing = Promise.resolve();
  const send = (message: unknown) => writing = writing.then(() => streams.stdout(Buffer.from(JSON.stringify(message) + '\n')));
  const write = async (message: unknown) => { if (!incoming.write(JSON.stringify(message) + '\n')) await once(incoming, 'drain'); };
  const request = (params: unknown) => new Promise<any>((resolve, reject) => {
    const id = prefix + ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('config-timeout')); }, 5000);
    pending.set(id, { resolve: value => { clearTimeout(timer); resolve(value); }, reject: error => { clearTimeout(timer); reject(error); } });
    void write({ id, method: 'config/read', params }).catch(error => { pending.get(id)?.reject(error); pending.delete(id); });
  });
  const owner = { manager: invocation.environment.GESTALT_MANAGER_BIN || manager, codexHome: invocation.environment.CODEX_HOME! };
  const finish = async (status?: number) => {
    if (ending) return; ending = true;
    shutdownStatus = status;
    for (const waiter of pending.values()) waiter.reject(new Error('runtime-exited')); pending.clear();
    input.close(); inputBytes.destroy(); incoming.end();
    await lease?.stop(); outgoing.end();
  };
  const outputTask = (async () => {
    for await (const line of output) {
      let message; try { message = JSON.parse(line); } catch { failed = true; void finish(1); continue; }
      const waiter = pending.get(message.id);
      if (waiter) { pending.delete(message.id); message.error ? waiter.reject(new Error('config-unavailable')) : waiter.resolve(message.result); continue; }
      if ((message.result || message.error) && reads.delete(message.id) && message.result?.config)
        message.result = { ...message.result, config: managedMcpApprovals(message.result.config, owner) };
      await send(message);
    }
  })();
  try {
    lease = await commands.launch(native, ['app-server', ...invocation.argv], { stdin: incoming,
      stdout: async bytes => { if (!outgoing.write(bytes)) await once(outgoing, 'drain'); }, stderr: bytes => streams.stderr(bytes) });
    if (ending) await lease.stop();
    const inputTask = (async () => {
      for await (const line of input) {
        if (ending) break;
        let message; try { message = JSON.parse(line); } catch { failed = true; await finish(1); return; }
        try {
          if (['thread/start', 'thread/resume', 'thread/fork'].includes(message.method)) {
            const baseline = await request({ includeLayers: false, ...(message.params?.cwd ? { cwd: message.params.cwd } : {}) });
            message.params = { ...message.params, config: managedMcpOverrides(message.params?.config ?? {}, baseline.config ?? {}, owner) };
          }
          if (message.method === 'config/read' && message.id !== undefined) reads.add(message.id);
          await write(message);
        } catch {
          if (message.id !== undefined) await send({ id: message.id, error: { code: -32603, message: 'Could not prepare managed MCP approval policy' } });
        }
      }
      await finish(0);
    })().catch(async () => { failed = true; await finish(1); });
    input.resume();
    const result = await lease.result;
    await finish(); await inputTask; await outputTask; await writing;
    if (commands.unknown.length) await streams.stderr(Buffer.from('gestalt: native AppServer result observed; complete descendant release is unconfirmed; temporary owner remains protected\n'));
    const status = commands.interrupted || (failed ? 1 : shutdownStatus ?? resultStatus(result));
    return status || (commands.unknown.length && process.platform !== 'darwin' ? 1 : 0);
  } finally {
    await finish(); await commands.stop(); unlisten();
    input.close(); inputBytes.destroy(); incoming.destroy(); outgoing.destroy();
    await outputTask.catch(() => {});
  }
}

export function appServerHandler(port?: OwnedProcessPort) {
  return async (invocation: CommandInvocation, streams: CommandStreams): Promise<number> => {
    try {
      const [native, ...argv] = invocation.argv;
      if (!native) throw new Error('native Codex executable is missing');
      const environment = scratchEnvironment(invocation.environment), paths = await setupPaths(environment);
      return await proxyAppServer({ ...invocation, argv, environment: { ...environment, CODEX_HOME: paths.codexHome, GESTALT_HOME: paths.gestaltHome } }, streams, paths.manager, native, port);
    } catch (error) { await streams.stderr(Buffer.from(`gestalt: ${error instanceof Error ? error.message : 'AppServer proxy failed'}\n`)); return 1; }
  };
}
