import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { chmod } from 'node:fs/promises';
import { createServer } from 'node:net';
import { NativePrivateStorage } from '../../src/cli/platform/storage.ts';
import { NativeLocalIpc, localEndpoint } from '../../src/cli/platform/ipc.ts';

const storage = await NativePrivateStorage.open(process.argv[2], process.argv[3]);
const peerProbe = process.argv[3];
const endpoint = localEndpoint(storage.root);
const server = createServer(socket => { socket.on('error', () => {}); socket.on('data', bytes => { socket.end(bytes); }); });
try {
  server.listen(endpoint); await once(server, 'listening'); await chmod(endpoint, 0o600);
  const probe = spawnSync(peerProbe, ['--identity', String(process.pid)], { encoding: 'utf8' });
  assert.equal(probe.status, 0, probe.stderr);
  const expected = JSON.parse(probe.stdout);
  const ipc = new NativeLocalIpc(storage, peerProbe);
  await assert.rejects(ipc.connect(endpoint, { ...expected, started: 'stale' }), /creation identity/);
  await assert.rejects(ipc.connect(endpoint, { ...expected, executable: '/wrong' }), /creation identity/);
  const channel = await ipc.connect(endpoint, expected);
  const bytes = Buffer.from([0, 255, 13, 10]);
  await channel.stdout(bytes);
  const received = []; for await (const chunk of channel.stdin) received.push(chunk);
  assert.deepEqual(Buffer.concat(received), bytes);
  await assert.rejects(channel.stderr(Buffer.from('diagnostic')), /protocol channel/);
  await assert.rejects(ipc.connect('../escape', expected), /Unix socket/);
  await assert.rejects(new NativeLocalIpc(storage).connect(endpoint, expected), /probe is required/);
  await chmod(endpoint, 0o666);
  await assert.rejects(ipc.connect(endpoint, expected), /not private/);
  assert.throws(() => localEndpoint('/' + 'a'.repeat(100)), /too long/);
} finally { await new Promise(resolve => server.close(resolve)); }
console.log('connected-socket identity/binary/private contracts passed');
