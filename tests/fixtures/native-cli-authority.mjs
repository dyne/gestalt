// Test-only Codex SessionStart handler. It always blocks before provider I/O.
// The actual installed Org owner and whole NativeLocalIpc perform the work;
// this file never synthesizes a publication or substitutes a raw socket test.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { lstat, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { NativePrivateStorage } from '../../src/cli/platform/storage.ts';
import { NativeLocalIpc } from '../../src/cli/platform/ipc.ts';

let configuration;
const result = { observedAt: new Date().toISOString(), pass: false, pid: process.pid, platform: process.platform,
  uid: process.getuid?.() ?? null, gid: process.getgid?.() ?? null, ancestorMetadata: [], installedBindings: [] };
try {
  configuration = JSON.parse(await readFile(process.env.GESTALT_SLICE_CONFIGURATION, 'utf8'));
  let input = '';
  for await (const bytes of process.stdin) { input += Buffer.from(bytes).toString(); assert.ok(input.length < 1024 * 1024); }
  result.hookInput = JSON.parse(input);
  assert.equal(result.hookInput.hook_event_name, 'SessionStart');
  result.inherited = Object.fromEntries(['GESTALT_SESSION_TMP_CONTEXT', 'TMPDIR', 'TMP', 'TEMP'].map(key => [key, process.env[key] ?? null]));
  assert.deepEqual(result.inherited, configuration.inherited, 'Actual Codex hook changed the inherited owner/TMP context');
  if (process.platform !== 'win32') result.identity = JSON.parse(execFileSync(configuration.peerProbe, ['--identity', String(process.pid)], { encoding: 'utf8' }));
  else {
    // Fixed read-only diagnostic; PID lookup conveys no signalling authority.
    const script = "$ErrorActionPreference='Stop';$p=[Diagnostics.Process]::GetProcessById([int]$env:GESTALT_SLICE_OBSERVE_PID);@{pid=$p.Id;started=$p.StartTime.ToUniversalTime().Ticks.ToString();executable=$p.MainModule.FileName;observerSid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value}|ConvertTo-Json -Compress";
    result.identity = JSON.parse(execFileSync(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'),
      ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')],
      { env: { ...process.env, GESTALT_SLICE_OBSERVE_PID: String(process.pid) }, encoding: 'utf8', timeout: 10000 }));
  }
  for (let directory = configuration.fixture; ; directory = path.dirname(directory)) {
    const info = await lstat(directory);
    result.ancestorMetadata.push({ path: directory, uid: info.uid, gid: info.gid, mode: (info.mode & 0o7777).toString(8) });
    if (directory === path.dirname(directory)) break;
  }
  for (const expected of configuration.orgFiles) {
    const file = path.join(configuration.orgBundle, path.basename(expected.path)), bytes = await readFile(file), info = await lstat(file);
    const actual = { name: path.basename(file), bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'), mode: (info.mode & 0o7777).toString(8) };
    assert.ok(info.isFile() && !info.isSymbolicLink());
    assert.equal(actual.bytes, expected.bytes); assert.equal(actual.sha256, expected.sha256);
    if (process.platform !== 'win32') assert.equal(info.mode & 0o7777, Number.parseInt(expected.gitMode.slice(-3), 8));
    result.installedBindings.push(actual);
  }
  const { publishStatus } = await import(pathToFileURL(path.join(configuration.orgBundle, 'org-plan-publication.mjs')));
  result.publication = publishStatus(configuration.plan, 'actual compiled CLI / Codex terminal / installed Org slice');
  assert.equal(result.publication.published, true, JSON.stringify(result.publication));
  const publicationInfo = await lstat(result.publication.path), publication = JSON.parse(await readFile(result.publication.path, 'utf8'));
  assert.equal(publication.planPath, configuration.plan); assert.equal(publication.schemaVersion, 1);
  if (process.platform !== 'win32') { assert.equal(publicationInfo.uid, process.getuid()); assert.equal(publicationInfo.mode & 0o7777, 0o600); }
  const storage = await NativePrivateStorage.open(configuration.fixture, configuration.peerProbe);
  await storage.assertPrivate(result.publication.path, 'file');
  result.publishedFile = { path: result.publication.path, uid: publicationInfo.uid, mode: (publicationInfo.mode & 0o7777).toString(8), planMatches: true, schemaVersion: 1,
    value: publication, sha256: createHash('sha256').update(await readFile(result.publication.path)).digest('hex') };
  const ipc = new NativeLocalIpc(storage, configuration.peerProbe);
  try {
    const wrong = await ipc.connect(configuration.endpoint, { ...configuration.host, started: configuration.host.started + '-wrong' });
    wrong.stdin.destroy?.(); throw Error('Whole IPC accepted a mismatched creation identity');
  } catch (error) { assert.match(error.message, /creation identity differs/); result.identityMismatch = { refused: true, message: error.message }; }
  const channel = await ipc.connect(configuration.endpoint, configuration.host), request = Buffer.from([0, 255, 13, 10, ...Buffer.from('authenticated host é東京')]);
  await channel.stdout(request);
  const chunks = []; let received = 0;
  for await (const bytes of channel.stdin) { chunks.push(Buffer.from(bytes)); received += bytes.length; if (received >= request.length) break; }
  assert.deepEqual(Buffer.concat(chunks), request);
  channel.stdin.destroy?.();
  result.ipc = { ok: true, host: configuration.host, requestHex: request.toString('hex'), replyHex: Buffer.concat(chunks).toString('hex') };
  result.pass = true;
} catch (error) { result.error = { message: error.message, stack: error.stack }; }
finally {
  if (configuration) await writeFile(configuration.report, JSON.stringify(result, null, 2) + '\n');
  // Even a refused fixture must not send a model request. The parent asserts
  // pass, this stop reason, the exact installed files and a zero-request sentinel.
  console.log(JSON.stringify({ continue: false, stopReason: 'Native core slice no-model barrier' }));
}
