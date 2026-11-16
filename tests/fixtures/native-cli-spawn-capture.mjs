// Passive, test-only argv observation. No stream listener, shell, ownership
// replacement or process result synthesis. execve discards the initial import;
// the unchanged Unix bootstrap loads it again through this fixture's NODE_OPTIONS.
import childProcess from 'node:child_process';
import { appendFileSync } from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
const original = childProcess.spawn;
childProcess.spawn = function (executable, argv, ...options) {
  const child = Reflect.apply(original, this, [executable, argv, ...options]);
  const target = process.env.GESTALT_SLICE_CODEX_BINARY, file = process.env.GESTALT_SLICE_SPAWN_CAPTURE;
  if (target && file && Array.isArray(argv)) {
    const boundary = argv.indexOf('--'), supervised = /gestalt-owned-process(?:\.exe)?$/i.test(String(executable)) && boundary >= 0;
    const selected = supervised ? argv[boundary + 1] : executable, selectedArgv = supervised ? argv.slice(boundary + 2) : argv;
    if (selected === 'codex' || String(selected).toLowerCase() === target.toLowerCase()) {
      try { appendFileSync(file, JSON.stringify({ observedAt: new Date().toISOString(), observer: process.pid, spawnedChildPid: child.pid,
        executable, target: selected, argv: selectedArgv, cwd: options[0]?.cwd ?? process.cwd(),
        transport: supervised ? 'accepted-owned-helper' : 'unchanged-Unix-spawn' }) + '\n'); }
      catch { /* Missing capture fails the test; never lose the real child handle. */ }
    }
  }
  return child;
};
syncBuiltinESMExports();
