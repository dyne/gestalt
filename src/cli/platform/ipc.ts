import { createConnection, type Socket } from 'node:net';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { spawn } from 'node:child_process';
import type { CommandStreams, IpcPort, ProcessIdentity } from '../contracts.ts';
import type { NativePrivateStorage } from './storage.ts';
import { startWindowsSystem } from './windows-system.ts';

export function localEndpoint(directory: string): string {
  if (process.platform === 'win32') return `\\\\.\\pipe\\gestalt-${randomUUID()}`;
  const name = `ipc-${randomUUID()}.sock`;
  const endpoint = path.join(directory, name);
  // macOS sockaddr_un is smaller than Linux. Never silently truncate/fall back to TCP.
  if (Buffer.byteLength(endpoint) > 103) {
    if (path.resolve(directory) === process.cwd()) return name;
    throw new Error('Private socket path is too long; use a private directory as the child working directory');
  }
  return endpoint;
}

function streamsFor(socket: Socket): CommandStreams {
  return {
    stdin: socket,
    stdout: bytes => new Promise<void>((resolve, reject) => socket.write(bytes, error => error ? reject(error) : resolve())),
    stderr: async () => { throw new Error('IPC has one protocol channel; diagnostics belong to the caller'); },
  };
}

/** The target-built probe queries credentials from this same connected socket. */
export class NativeLocalIpc implements IpcPort {
  private readonly storage: NativePrivateStorage;
  private readonly unixPeerProbe: string | undefined;
  constructor(storage: NativePrivateStorage, unixPeerProbe?: string) {
    this.storage = storage;
    this.unixPeerProbe = unixPeerProbe;
  }

  async connect(endpoint: string, expectedOwner: ProcessIdentity): Promise<CommandStreams> {
    if (process.platform === 'win32') {
      const match = /^\\\\\.\\pipe\\(gestalt-[a-f0-9-]{36})$/.exec(endpoint);
      if (!match?.[1]) throw new Error('Expected a local managed Windows named pipe');
      const child = startWindowsSystem({ operation: 'connect-pipe', name: match[1], owner: expectedOwner });
      // Read readiness from stderr, leaving protocol stdout untouched.
      let diagnostic = '';
      await new Promise<void>((resolve, reject) => {
        child.once('error', reject);
        child.once('close', code => reject(new Error(`Private pipe connection failed (${code}): ${diagnostic.trim()}`)));
        child.stderr.setEncoding('utf8');
        child.stderr.on('data', bytes => {
          diagnostic = (diagnostic + bytes).slice(-8192);
          if (diagnostic === 'GESTALT_NATIVE_READY\r\n' || diagnostic === 'GESTALT_NATIVE_READY\n') resolve();
        });
      });
      return {
        stdin: child.stdout,
        stdout: bytes => new Promise<void>((resolve, reject) => child.stdin.write(bytes, error => error ? reject(error) : resolve())),
        stderr: async () => { throw new Error('IPC has one protocol channel; diagnostics belong to the caller'); },
      };
    }
    if ((!path.isAbsolute(endpoint) && (path.basename(endpoint) !== endpoint || !endpoint || endpoint.includes('\0'))) || Buffer.byteLength(endpoint) > 103)
      throw new Error('Expected a bounded Unix socket path');
    const absolute = path.resolve(endpoint);
    const cwd = process.cwd();
    await this.storage.assertPrivate(path.dirname(absolute), 'directory');
    await this.storage.assertPrivate(absolute, 'socket');
    if (!this.unixPeerProbe || !path.isAbsolute(this.unixPeerProbe)) throw new Error('Target-built native Unix peer probe is required');
    if (cwd !== process.cwd()) throw new Error('Working directory changed during relative IPC validation');
    const socket = createConnection({ path: endpoint });
    try {
      await once(socket, 'connect');
      const probe = spawn(this.unixPeerProbe, ['--peer'], { shell: false, stdio: ['ignore', 'pipe', 'pipe', socket] });
      if (!probe.stdout) throw new Error('Native peer probe stdout is unavailable');
      let bytes = '';
      probe.stdout.setEncoding('utf8');
      probe.stdout.on('data', chunk => { bytes += chunk; if (bytes.length > 16384) probe.kill(); });
      const [code] = await once(probe, 'close');
      if (code !== 0) throw new Error('Unix peer credentials could not be established');
      const peer = JSON.parse(bytes) as ProcessIdentity & { uid: number };
      if (peer.uid !== process.getuid?.() || peer.pid !== expectedOwner.pid || peer.started !== expectedOwner.started || peer.executable !== expectedOwner.executable)
        throw new Error('Unix socket peer creation identity differs from expected owner');
      return streamsFor(socket);
    } catch (error) { socket.destroy(); throw error; }
  }
}
