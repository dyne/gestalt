// Explicit producer replacement fixture, never a consumer runtime fallback.
import { writeFileSync, readFileSync, readlinkSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
const { selectInteractiveRuntime } = await import(process.env.GESTALT_TEST_INTERACTIVE_MODULE ?? new URL('../../src/cli/platform/interactive.ts', import.meta.url));
const [manager, peer, before, cwd, ...argv] = process.argv.slice(2);
const selected = selectInteractiveRuntime({ legacyManager: manager, ownedHelper: 'must-not-select-namespace-helper' });
if (selected.kind !== 'unix-compatibility' || 'owned' in selected) throw Error('Expected explicit Unix compatibility selection');
const identity = JSON.parse(execFileSync(peer, ['--identity', String(process.pid)], { encoding: 'utf8' }));
writeFileSync(before, JSON.stringify({ identity, pid: process.pid, ppid: process.ppid, uid: process.getuid(), gid: process.getgid(),
  environment: Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'].map(key=>[key,process.env[key]??null])),
  namespaces: process.platform === 'linux' ? {user:readlinkSync('/proc/self/ns/user'),pid:readlinkSync('/proc/self/ns/pid'),uidMap:readFileSync('/proc/self/uid_map','utf8'),gidMap:readFileSync('/proc/self/gid_map','utf8')} : null }));
process.on('exit',()=>writeFileSync(`${before}.js-exit-cleanup`,'must not run across exec'));
selected.exec({ argv, cwd, environment: process.env });
