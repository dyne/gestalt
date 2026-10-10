import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmod, mkdtemp, mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import http from 'node:http';
import net from 'node:net';
import { join, resolve } from 'node:path';
import test from 'node:test';

const manager = resolve('public/gestalt');

async function fixture(t, { mode = 0o600, response, hang = false, version = 'v2.11.7' } = {}) {
  const root = await mkdtemp('/tmp/caddy doctor ');
  await chmod(root, 0o700);
  const bin = join(root, 'bin');
  await mkdir(bin);
  const calls = join(root, 'calls');
  const caddy = join(bin, 'caddy');
  // Controlled version command is the only allowed CLI invocation.
  await writeFile(caddy, `#!/bin/sh\n[ "$1" = version ] && [ "$#" = 1 ] || exit 99\nprintf '%s\\n' '${version}'\n`, { mode: 0o755 });
  const socket = join(root, 'admin.sock');
  const admin = `unix/${socket}`;
  const requests = [];
  const config = { admin: { listen: `${admin}|0600` }, apps: { http: { servers: {
    existing: { listen: [':18080'], routes: [{ handle: [{ handler: 'static_response', body: 'private unrelated token' }] }] },
  } } } };
  const server = http.createServer(async (req, res) => {
    requests.push([req.method, req.url]);
    await writeFile(calls, JSON.stringify(requests));
    if (hang) return;
    res.end(response === undefined ? JSON.stringify(config) : response);
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(socket, resolve); });
  await chmod(socket, mode);
  t.after(async () => {
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    await rm(root, { recursive: true, force: true });
  });
  const env = { ...process.env, HOME: root, GESTALT_HOME: join(root, 'gestalt'), CODEX_HOME: join(root, 'codex'), PATH: bin + ':' + process.env.PATH };
  const args = ['caddy', 'doctor', '--json', '--mobile-origin', 'https://mobile.example.test',
    '--preview-host', 'preview.example.test', '--ports', '59441-59443', '--admin', admin];
  async function run(replace = {}, changes = {}) {
    const child = spawn('/bin/bash', [manager, ...args.map(arg => replace[arg] ?? arg)], { env: { ...env, ...changes }, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.on('data', part => stdout += part);
    child.stderr.on('data', part => stderr += part);
    const timer = setTimeout(() => child.kill('SIGKILL'), 8000);
    try {
      const code = await new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); });
      assert.equal(code, 1, stderr);
      const report = JSON.parse(stdout);
      assert.equal(report.ready, false);
      assert.equal(report.integrationReady, false);
      assert.equal(report.dns.ready, null);
      assert.equal(report.tls.ready, null);
      assert.ok(stdout.length < 1500);
      assert.doesNotMatch(stdout + stderr, /private unrelated token/);
      assert.ok(requests.every(([method, url]) => method === 'GET' && url === '/config/'));
      return report;
    } finally { clearTimeout(timer); }
  }
  return { root, bin, caddy, socket, admin, config, requests, run };
}

test('secure isolated admin fixture passes bootstrap checks but cannot assert project isolation', async t => {
  const f = await fixture(t);
  const report = await f.run();
  assert.equal(report.reason, 'admin-unisolated');
  assert.equal(report.adminReady, true);
  assert.equal(report.configReady, true);
  assert.equal(report.portsReady, true);
  assert.equal(report.isolation.ready, false);
  assert.deepEqual(f.requests, [['GET', '/config/']]);
  assert.equal(await readFile(join(f.root, 'calls'), 'utf8'), '[["GET","/config/"]]');
  await assert.rejects(readFile(join(f.root, 'gestalt', 'active.json')), { code: 'ENOENT' });
});

test('absent or incompatible Caddy does not access admin', async t => {
  const f = await fixture(t, { version: 'v2.11.6' });
  assert.equal((await f.run()).reason, 'unsupported-version');
  await writeFile(f.caddy, '#!/bin/sh\nexit 127\n');
  assert.equal((await f.run()).reason, 'version-unavailable');
  // An executable symlink to a nonexistent target is an actual ENOENT probe.
  await rm(f.caddy);
  await symlink(process.execPath, join(f.bin, 'node'));
  assert.equal((await f.run({}, { PATH: f.bin })).reason, 'caddy-absent');
  assert.equal(f.requests.length, 0);
});

test('inaccessible socket, permissive mode and TCP admin fail closed', async t => {
  const f = await fixture(t, { mode: 0o666 });
  assert.equal((await f.run()).reason, 'admin-permissions');
  assert.equal((await f.run({ [f.admin]: 'localhost:2019' })).reason, 'admin-insecure');
  assert.equal((await f.run({ [f.admin]: 'unix//tmp/nonexistent-gestalt-caddy.sock' })).reason, 'admin-inaccessible');
  assert.equal(f.requests.length, 0);
});

test('Caddy version timeout and output limit are bounded and never reach admin', async t => {
  const f = await fixture(t);
  await writeFile(f.caddy, `#!${process.execPath}\nsetInterval(() => {}, 1000);\n`);
  const started = Date.now();
  assert.equal((await f.run()).reason, 'version-unavailable');
  assert.ok(Date.now() - started < 6000);
  await writeFile(f.caddy, `#!${process.execPath}\nconsole.log('private unrelated token'.repeat(1000));\n`);
  assert.equal((await f.run()).reason, 'version-unavailable');
  assert.equal(f.requests.length, 0);
});

test('live insecure config and wrong preview hostname are rejected', async t => {
  const f = await fixture(t);
  f.config.admin.listen = 'localhost:2019';
  assert.equal((await f.run()).reason, 'admin-insecure');
  assert.equal((await f.run({ 'preview.example.test': 'mobile.example.test' })).reason, 'preview-host-conflict');
  assert.equal((await f.run({ '59441-59443': '1-65535' })).reason, 'configuration-required');
  assert.equal(f.requests.length, 1);
});

test('configured Caddy and unrelated process port conflicts are read-only', async t => {
  const f = await fixture(t);
  f.config.apps.http.servers.existing.listen = [':59441'];
  assert.equal((await f.run()).reason, 'port-conflict');
  f.config.apps.http.servers.existing.listen = [':18080'];
  const listener = net.createServer();
  await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => listener.close(resolve)));
  const port = listener.address().port;
  assert.equal((await f.run({ '59441-59443': `${port}-${port}` })).reason, 'port-conflict');
});

test('admin response size and elapsed time are bounded without leaking config', async t => {
  const big = await fixture(t, { response: 'x'.repeat(262145) });
  assert.equal((await big.run()).reason, 'admin-response-invalid');
  const slow = await fixture(t, { hang: true });
  const started = Date.now();
  assert.equal((await slow.run()).reason, 'admin-response-invalid');
  assert.ok(Date.now() - started < 6000);
});

test('actual selected Caddy serves its private admin config without doctor mutation', async t => {
  const f = await fixture(t);
  const probe = spawn('caddy', ['version'], { stdio: 'ignore' });
  const available = await new Promise(resolve => { probe.on('error', () => resolve(false)); probe.on('close', code => resolve(code === 0)); });
  if (!available) {
    assert.notEqual(process.env.GESTALT_CADDY_TEST_REQUIRED, '1', 'real Caddy is required for focused evidence');
    t.skip('Caddy unavailable on the test host');
    return;
  }
  const root = await mkdtemp('/tmp/gestalt real caddy ');
  await chmod(root, 0o700);
  const socket = join(root, 'admin.sock');
  const endpoint = `unix/${socket}`;
  const config = join(root, 'Caddyfile');
  await writeFile(config, `{\n admin "${endpoint}|0600"\n persist_config off\n}\n`);
  const child = spawn('caddy', ['run', '--config', config, '--adapter', 'caddyfile'], {
    env: { ...process.env, HOME: root, XDG_DATA_HOME: root, XDG_CONFIG_HOME: root }, stdio: 'ignore',
  });
  const closed = new Promise(resolve => { child.once('close', resolve); child.once('error', resolve); });
  t.after(async () => {
    child.kill('SIGTERM');
    const kill = setTimeout(() => child.kill('SIGKILL'), 3000);
    await closed;
    clearTimeout(kill);
    await rm(root, { recursive: true, force: true });
  });
  let report;
  for (let attempt = 0; attempt < 20; attempt++) {
    report = await f.run({ [f.admin]: endpoint });
    if (report.adminReady) break;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  assert.equal(report.reason, 'admin-unisolated');
  assert.equal(report.adminReady, true);
  assert.equal(report.configReady, true);
  assert.equal(report.portsReady, true);
  assert.equal(await readFile(config, 'utf8'), `{\n admin "${endpoint}|0600"\n persist_config off\n}\n`);
  assert.equal(child.exitCode, null, 'doctor must not stop the selected daemon');
});
