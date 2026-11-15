import { spawn } from 'node:child_process';
import { createConnection } from 'node:net';
import { fileURLToPath } from 'node:url';

const [role = 'root', behavior = 'graceful', token] = process.argv.slice(2);
const endpoint = process.env.GESTALT_MAC_FIXTURE_SOCKET;
const interval = setInterval(() => {}, 1000);
process.on('SIGTERM', () => { if (role === 'root' && behavior === 'graceful') process.exit(17); });
const channel = createConnection(endpoint);
// This independent harness channel supplies controlled teardown even when the
// leaf has detached and the production group supervisor cannot reach it.
const release = () => { clearInterval(interval); channel.destroy(); process.exit(0); };
channel.on('data', release); channel.on('end', release);
channel.on('error', () => process.exit(86));
channel.on('connect', () => {
  channel.write(JSON.stringify({ token, role, pid: process.pid }) + '\n');
  if (role === 'root') {
    const leaf = spawn(process.execPath, [fileURLToPath(import.meta.url), 'leaf', 'resistant', token], {
      env: process.env, detached: process.env.GESTALT_MAC_DETACH === '1', stdio: 'ignore',
    });
    leaf.on('error', () => process.exit(87)); leaf.unref();
  }
});
