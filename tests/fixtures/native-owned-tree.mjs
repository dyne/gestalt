import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const [mode, behavior = 'resistant'] = process.argv.slice(2);
const fixture = fileURLToPath(import.meta.url);
const token = process.env.GESTALT_OWNED_FIXTURE ?? '';
if (mode === 'echo') {
  const input = []; for await (const chunk of process.stdin) input.push(chunk);
  process.stdout.write(JSON.stringify({ argv: process.argv.slice(3), input: Buffer.concat(input).toString('hex'), cwd: process.cwd(), retained: process.env.RETAINED }));
  process.stderr.write(Buffer.from([255, 0, 13])); process.exitCode = 23;
} else {
  let treeReady = false, exitRequested = false;
  const maybeExit = () => { if (mode === 'exit-parent' && treeReady && exitRequested) process.exit(41); };
  if (mode === 'exit-parent') process.stdin.on('data', () => { exitRequested = true; maybeExit(); });
  const signal = () => { if (behavior === 'graceful') process.exit(17); };
  process.on('SIGTERM', signal);
  if (process.platform === 'win32') process.on('SIGBREAK', signal);
  // A detached Windows child does not share the parent's console. Its held
  // identity travels over the existing child IPC channel, never console text.
  if (mode !== 'leaf' || !process.send) console.log(JSON.stringify({ kind: mode === 'exit-parent' ? 'root' : mode, pid: process.pid }));
  if (mode === 'root' || mode === 'exit-parent') {
    const worker = spawn(process.execPath, [fixture, 'worker', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], { stdio: ['ignore', 'inherit', 'inherit', 'ipc'], env: process.env });
    worker.on('message', message => {
      if (message?.kind === 'leaf-ready') {
        if (message.token !== token || message.worker !== worker.pid || !Number.isSafeInteger(message.pid) || message.pid <= 0) throw Error('Invalid bound leaf identity');
        console.log(JSON.stringify({ kind: 'leaf', pid: message.pid, worker: worker.pid }));
        console.log(JSON.stringify({ kind: 'tree-ready', pid: process.pid }));
        treeReady = true; maybeExit();
      }
    });
  } else if (mode === 'worker') {
    if (process.env.GESTALT_NESTED_HELPER) {
      const { NativeOwnedProcesses } = await import('../../src/cli/platform/processes.ts');
      let partial = '';
      await new NativeOwnedProcesses(process.env.GESTALT_NESTED_HELPER).launch(process.execPath, {
        argv: [fixture, 'leaf', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], cwd: process.cwd(), environment: process.env,
      }, { stdin: (async function* () {})(), stdout: async bytes => {
        partial += Buffer.from(bytes).toString();
        for (;;) {
          const newline = partial.indexOf('\n'); if (newline < 0) break;
          const line = partial.slice(0, newline); partial = partial.slice(newline + 1);
          const identity = JSON.parse(line);
          if (identity.kind !== 'leaf' || !Number.isSafeInteger(identity.pid) || identity.pid <= 0) throw Error('Invalid nested leaf identity');
          process.send?.({ kind: 'leaf-ready', token, worker: process.pid, pid: identity.pid });
        }
      }, stderr: async bytes => { process.stderr.write(bytes); } });
    } else {
      const leaf = spawn(process.execPath, [fixture, 'leaf', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], { detached: true, stdio: ['ignore', 'inherit', 'inherit', 'ipc'], env: process.env });
      leaf.on('message', message => {
        if (message?.kind !== 'leaf-ready' || message.token !== token || message.pid !== leaf.pid) throw Error('Invalid detached leaf identity');
        process.send?.({ kind: 'leaf-ready', token, worker: process.pid, pid: leaf.pid });
      });
      leaf.unref();
    }
  } else if (mode === 'leaf') process.send?.({ kind: 'leaf-ready', token, pid: process.pid });
  setInterval(() => {}, 1000);
}
