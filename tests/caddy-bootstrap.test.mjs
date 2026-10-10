import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const snippet = readFileSync(new URL('../examples/caddy/gestalt-live-admin.caddy', import.meta.url), 'utf8');
const available = spawnSync('caddy', ['version'], { encoding: 'utf8', timeout: 5000 });
const skip = available.error?.code === 'ENOENT' && process.env.GESTALT_CADDY_TEST_REQUIRED !== '1'
  ? 'Caddy unavailable; run with GESTALT_CADDY_TEST_REQUIRED=1 on a Caddy host'
  : false;

function fixture(run) {
  const root = mkdtempSync(join(tmpdir(), 'gestalt caddy bootstrap '));
  // Every CLI invocation uses private HOME/storage and temporary config. No
  // daemon, admin API, /etc, certificate issuance or service mutation is used.
  const env = { ...process.env, HOME: root, XDG_DATA_HOME: root, XDG_CONFIG_HOME: root };
  delete env.CADDY_ADMIN;
  const socket = `unix/${root}/admin.sock|0600`;
  writeFileSync(join(root, 'admin.caddy'), snippet.replace('unix//run/caddy/gestalt-admin.sock|0600', `"${socket}"`));
  const invoke = (verb, content) => {
    writeFileSync(join(root, 'Caddyfile'), content);
    return spawnSync('caddy', [verb, '--config', join(root, 'Caddyfile'), '--adapter', 'caddyfile'], {
      cwd: root, env, encoding: 'utf8', timeout: 10000,
    });
  };
  const adapt = content => {
    const result = invoke('adapt', content);
    assert.equal(result.status, 0, `adapt failed (${result.error?.code ?? result.status}): ${result.stderr?.slice(-600)}`);
    return JSON.parse(result.stdout);
  };
  const validate = content => {
    const result = invoke('validate', content);
    assert.equal(result.status, 0, `validate failed (${result.error?.code ?? result.status}): ${result.stderr?.slice(-600)}`);
  };
  try { run({ invoke, adapt, validate, socket }); }
  finally { rmSync(root, { recursive: true, force: true }); }
}

test('admin include adapts and validates in its documented standalone global context', { skip }, () => {
  fixture(({ adapt, validate, socket }) => {
    const config = '{\n import admin.caddy\n}\n';
    const json = adapt(config);
    assert.equal(json.admin.listen, socket);
    assert.equal(json.apps?.http, undefined, 'bootstrap must not create a public proxy/listener');
    validate(config);
  });
});

test('manual import preserves unrelated sites and existing global options', { skip }, () => {
  fixture(({ adapt, validate, socket }) => {
    // HTTP-only fixtures avoid network/certificate prerequisites at validation.
    const sites = 'http://unrelated.example.test:18080 {\n respond "unrelated site" 200\n}\n';
    const before = `{\n admin off\n persist_config off\n}\n${sites}`;
    const after = `{\n import admin.caddy\n persist_config off\n}\n${sites}`;
    const original = adapt(before);
    const updated = adapt(after);
    assert.deepEqual(updated.apps, original.apps);
    assert.equal(updated.admin.listen, socket);
    assert.equal(updated.admin.config.persist, false);
    assert.equal(updated.apps.http.servers.srv0.routes[0].handle[0].routes[0].handle[0].body, 'unrelated site');
    validate(before);
    validate(after);
  });
});

test('documented count diagnostic catches repeated import even when Caddy accepts it', { skip }, () => {
  fixture(({ adapt, validate }) => {
    const once = '{\n import admin.caddy\n}\n';
    const twice = '{\n import admin.caddy\n import admin.caddy\n}\n';
    assert.deepEqual(adapt(twice), adapt(once));
    validate(twice);
    const diagnostic = '$1 == "import" && $2 == "admin.caddy" { n++ } END { if (n != 1) { print "Expected exactly one Gestalt admin import"; exit 1 } }';
    for (const [content, status] of [[once, 0], [twice, 1], ['{\n admin off\n}\n', 1]]) {
      const result = spawnSync('awk', [diagnostic], { input: content, encoding: 'utf8', timeout: 5000 });
      assert.equal(result.status, status);
      if (status) assert.match(result.stdout, /Expected exactly one/);
    }
  });
});

test('site enclosing context rejects the global admin include', { skip }, () => {
  fixture(({ invoke }) => {
    const result = invoke('adapt', 'http://unrelated.example.test:18080 {\n import admin.caddy\n}\n');
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /admin|unrecognized/i);
  });
});
