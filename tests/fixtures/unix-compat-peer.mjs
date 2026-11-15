// Test-only host observer. Passing a Socket as child stdio pauses its Node reader.
// Resume it after the read-only credential probe closes, before protocol traffic.
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { readFile } from 'node:fs/promises';

export async function observeHostSocket(peerProbe, socket, progress = () => {}) {
  progress('host-credential-probe-start');
  const probe = spawn(peerProbe, ['--peer'], { shell: false, stdio: ['ignore', 'pipe', 'pipe', socket] });
  let out = '', err = '', timedOut = false;
  probe.stdout.on('data', bytes => { out += bytes; });
  probe.stderr.on('data', bytes => { err += bytes; });
  const deadline = setTimeout(() => { timedOut = true; probe.kill('SIGKILL'); }, 5000);
  try {
    const [exit, signal] = await once(probe, 'close');
    const record = { exit, signal, timedOut, stderr: err, peer: exit === 0 ? JSON.parse(out) : null };
    progress('host-credential-probe-close', { ...record, readerPaused: socket.isPaused() });
    socket.resume();
    progress('host-protocol-reader-resumed', { readerPaused: socket.isPaused() });
    return record;
  } finally { clearTimeout(deadline); }
}

export async function readAuthorityReport(file, primaryError) {
  try { return { observed: JSON.parse(await readFile(file, 'utf8')), primaryError }; }
  catch (error) {
    return { observed: null, primaryError: primaryError ?? { message: 'Authority report unavailable', cause: error.message }, readError: error.message };
  }
}
