import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const [mode, behavior = 'resistant'] = process.argv.slice(2);
const fixture = fileURLToPath(import.meta.url);
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
  console.log(JSON.stringify({ kind: mode, pid: process.pid }));
  if (mode === 'root' || mode === 'exit-parent') {
    const worker = spawn(process.execPath, [fixture, 'worker', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], { stdio: ['ignore', 'inherit', 'inherit', 'ipc'], env: process.env });
    worker.on('message', message => {
      if (message === 'leaf-ready') {
        console.log(JSON.stringify({ kind: 'tree-ready', pid: process.pid }));
        treeReady = true; maybeExit();
      }
    });
  } else if (mode === 'worker') {
    if (process.env.GESTALT_NESTED_HELPER) {
      const { NativeOwnedProcesses } = await import('../../src/cli/platform/processes.ts');
      await new NativeOwnedProcesses(process.env.GESTALT_NESTED_HELPER).launch(process.execPath, {
        argv: [fixture, 'leaf', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], cwd: process.cwd(), environment: process.env,
      }, { stdin: (async function* () {})(), stdout: async bytes => {
        process.stdout.write(bytes);
        if (Buffer.from(bytes).toString().includes('"kind":"leaf"')) process.send?.('leaf-ready');
      }, stderr: async bytes => { process.stderr.write(bytes); } });
    } else {
      const leaf = spawn(process.execPath, [fixture, 'leaf', behavior, process.env.GESTALT_OWNED_FIXTURE ?? ''], { detached: true, stdio: ['ignore', 'inherit', 'inherit', 'ipc'], env: process.env });
      leaf.on('message', message => { if (message === 'leaf-ready') process.send?.('leaf-ready'); });
      leaf.unref();
    }
  } else if (mode === 'leaf') process.send?.('leaf-ready');
  setInterval(() => {}, 1000);
}
