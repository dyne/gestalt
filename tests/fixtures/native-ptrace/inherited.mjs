// Invoked identically in host, old owned namespace and test-only ptrace runtime.
import { readFile, lstat, realpath, readlink } from 'node:fs/promises';
import path from 'node:path';
import { createConnection } from 'node:net';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { pathToFileURL } from 'node:url';
const [manager, agents, directory, peerProbe, expectedJSON] = process.argv.slice(2);
const expected = JSON.parse(expectedJSON), result = { pid: process.pid, uid: process.getuid(), gid: process.getgid(), expected };
for (const name of ['uid_map', 'gid_map', 'status']) result[name] = (await readFile(`/proc/self/${name}`, 'utf8')).split('\n').filter(line => name !== 'status' || /^(Uid|Gid|NSpid|NoNewPrivs|TracerPid):/.test(line)).join('\n');
result.ancestors = [];
for (let current = await realpath(directory);; current = path.dirname(current)) {
  const entry = await lstat(current); result.ancestors.push({ path: current, uid: entry.uid, gid: entry.gid, mode: (entry.mode & 0o7777).toString(8) });
  if (current === path.dirname(current)) break;
}
const library = path.join(agents, 'plugins/gestalt/skills/org-plan/scripts');
const { ensureManagedRoot } = await import(pathToFileURL(path.join(library, 'org-plan-storage.mjs')));
result.canonicalStatusDirectory = await realpath(path.join(directory, 'status'));
try { ensureManagedRoot(result.canonicalStatusDirectory, { privateMode: false, create: false }); result.guard = { ok: true }; }
catch (error) { result.guard = { ok: false, error: error.message }; }
const { publishStatus } = await import(pathToFileURL(path.join(library, 'org-plan-publication.mjs')));
const canonicalPlan = await realpath(path.join(directory, 'fixture.org'));
result.publication = publishStatus(canonicalPlan, 'private inherited-runtime diagnostic');
if (result.publication.published) {
  const published = JSON.parse(await readFile(result.publication.path, 'utf8')), info = await lstat(result.publication.path);
  result.publishedFile = { canonicalPlanMatches: published.planPath === canonicalPlan, schemaVersion: published.schemaVersion, uid: info.uid, mode: (info.mode & 0o7777).toString(8) };
}
const { NativePrivateStorage } = await import(pathToFileURL(path.join(manager, 'src/cli/platform/storage.ts')));
const { NativeLocalIpc } = await import(pathToFileURL(path.join(manager, 'src/cli/platform/ipc.ts')));
const request = Buffer.from('authenticated private echo é東京');
try {
  const storage = await NativePrivateStorage.open(directory, peerProbe), adapter = new NativeLocalIpc(storage, peerProbe);
  const streams = await adapter.connect('s', expected); await streams.stdout(request);
  const received = []; let size = 0;
  for await (const bytes of streams.stdin) { received.push(Buffer.from(bytes)); size += bytes.length; if (size >= request.length) break; }
  result.ipc = { ok: Buffer.concat(received).equals(request), request: request.toString('base64'), reply: Buffer.concat(received).toString('base64') };
  try { await adapter.connect('s', { ...expected, started: `${expected.started}-incorrect` }); result.identityMismatch = { refused: false }; }
  catch (error) { result.identityMismatch = { refused: /creation identity differs/.test(error.message), error: error.message }; }
} catch (error) { result.ipc = { ok: false, error: error.message }; }
// This independent measurement sends no application bytes. The unchanged native
// verifier receives this same connected socket and reports the real kernel peer.
const socket = createConnection({ path: 's' }); await once(socket, 'connect');
async function measure(executable, argv) {
  const measured = spawn(executable, argv, { stdio: ['ignore', 'pipe', 'pipe', socket], shell: false });
  let stdout = '', stderr = ''; measured.stdout.on('data', b => { stdout += b; }); measured.stderr.on('data', b => { stderr += b; });
  const [exit, signal] = await once(measured, 'close'); return { exit, signal, stdout, stderr };
}
result.rawCredentials = await measure('/usr/bin/python3', ['-c', 'import socket,struct,json,os; s=socket.socket(fileno=3); p,u,g=struct.unpack("3i",s.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12)); print(json.dumps({"peerPid":p,"peerUid":u,"peerGid":g,"callerPid":os.getpid(),"callerUid":os.getuid()}))']);
result.sameSocketPeer = await measure(peerProbe, ['--peer']); socket.destroy();
try { result.expectedProc = { executable: await readlink(`/proc/${expected.pid}/exe`), stat: await readFile(`/proc/${expected.pid}/stat`, 'utf8') }; }
catch (error) { result.expectedProc = { error: error.code }; }
console.log(JSON.stringify(result));
