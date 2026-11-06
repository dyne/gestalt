import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, readFile, writeFile, rm, readdir, stat } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';

const source = await readFile(resolve('public/gestalt'), 'utf8');
assert.match(source, /\nmain "\$@"\s*$/);

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'gestalt chrome install '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const manager = join(root, 'gestalt'), log = join(root, 'calls');
  // The public launcher normalizes hostile TMPDIR to the managed context.
  // Invalid inherited context must fail rather than select the fallback.
  const temporaryRoot = Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT') ?
    JSON.parse(process.env.GESTALT_SESSION_TMP_CONTEXT).directory : root;
  assert.ok(typeof temporaryRoot === 'string' && temporaryRoot.length > 0, 'temporary root missing');
  // Keep production install orchestration and cleanup. All package, network and
  // privilege operations are local stubs: these tests cannot install software.
  await writeFile(manager, source.replace(/\nmain "\$@"\s*$/, `
record_chrome_call() {
  node -e 'require("node:fs").appendFileSync(process.env.CHROME_TEST_LOG,JSON.stringify(process.argv.slice(1))+"\\n")' "$@"
}
playwright_runtime() { printf '%s' 'process.exit(Number(process.env.PACKAGE_FAILURE || 0))'; }
playwright_chrome_binary() {
  [[ \${CHROME_PRESENT:-0} == 1 || -f $CHROME_TEST_ROOT/installed ]] || return 1
  printf '%s\\n' "$CHROME_TEST_ROOT/chrome"
}
playwright_debian_host() { [[ \${CHROME_DEBIAN:-1} == 1 ]]; }
uname() { printf '%s\\n' "\${CHROME_OS:-Linux}"; }
dpkg() { printf '%s\\n' "\${CHROME_ARCH:-amd64}"; }
id() { printf '%s\\n' "\${CHROME_UID:-1000}"; }
curl() {
  record_chrome_call curl "$@"
  [[ \${CHROME_FAIL:-} != download ]] || return 22
  while (($#)); do
    if [[ $1 == --output ]]; then printf 'fixture package' > "$2"; return; fi
    shift
  done
  return 1
}
sudo() { record_chrome_call sudo "$@"; "$@"; }
apt-get() {
  record_chrome_call apt-get "$@"
  [[ \${CHROME_FAIL:-} != apt ]] || return 42
  [[ -f $3 ]] || return 1
  if [[ \${CHROME_FAIL:-} != missing ]]; then touch "$CHROME_TEST_ROOT/installed"; fi
}
main "$@"
`));
  await writeFile(join(root, 'chrome'), `#!/bin/sh\n[ "\${CHROME_FAIL:-}" != version ] || exit 3\nprintf 'Google Chrome 140.0.1.2\\n'\n`, { mode: 0o755 });
  const run = (args = ['install'], extra = {}) => spawnSync('bash', [manager, 'playwright', ...args], {
    env: { ...process.env, GESTALT_HOME: join(root, 'managed'), TMPDIR: root,
      CHROME_TEST_ROOT: root, CHROME_TEST_LOG: log, ...extra },
    encoding: 'utf8', timeout: 10000,
  });
  const calls = async () => readFile(log, 'utf8').then(
    s => s.trim().split('\n').map(JSON.parse), e => { if (e.code === 'ENOENT') return []; throw e; },
  );
  const cleaned = async () => {
    assert.deepEqual((await readdir(root)).filter(n => n.startsWith('gestalt-chrome.')), []);
    for (const call of await calls()) {
      if (call[0] !== 'curl') continue;
      const output = call.indexOf('--output');
      assert.ok(output >= 0 && typeof call[output + 1] === 'string', 'fixture download output missing');
      const staging = dirname(call[output + 1]);
      assert.ok(dirname(staging) === temporaryRoot && staging.startsWith(temporaryRoot + '/gestalt-chrome.'), 'download outside selected temporary root');
      let absent = false;
      try { await stat(staging); } catch (error) { if (error.code !== 'ENOENT') throw error; absent = true; }
      assert.ok(absent, 'fixture Chrome download staging directory survived cleanup');
    }
  };
  return { root, temporaryRoot, run, calls, cleaned };
}

for (const uid of ['0', '1000']) test(`missing Debian amd64 Chrome installs and verifies with uid ${uid}`, async t => {
  const f = await fixture(t), r = f.run(['install'], { CHROME_UID: uid });
  assert.equal(r.status, 0, r.stderr);
  assert.match(r.stdout, /Google Chrome 140/);
  const calls = await f.calls();
  assert.deepEqual(calls.map(c => c[0]), uid === '0' ? ['curl', 'apt-get'] : ['curl', 'sudo', 'apt-get']);
  assert.ok(calls[0].includes('https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb'));
  assert.ok(calls[0].includes('--fail'));
  const apt = calls.at(-1);
  assert.deepEqual(apt.slice(0, 3), ['apt-get', 'install', '-y']);
  assert.ok(apt[3].startsWith(f.temporaryRoot + '/gestalt-chrome.'), 'apt package outside selected temporary root');
  await f.cleaned();
  assert.equal(f.run().status, 0);
  assert.deepEqual(await f.calls(), calls, 'repeat install must not download or use apt');
});

for (const extra of [{ CHROME_PRESENT: '1' }, { CHROME_OS: 'Darwin' }, { CHROME_DEBIAN: '0' }, { CHROME_ARCH: 'arm64' }]) {
  test(`does not install system Chrome for ${JSON.stringify(extra)}`, async t => {
    const f = await fixture(t), r = f.run(['install'], extra);
    assert.equal(r.status, 0, r.stderr);
    assert.deepEqual(await f.calls(), []);
    await f.cleaned();
  });
}

for (const failure of ['download', 'apt', 'missing', 'version']) test(`Chrome ${failure} failure is reported and temporary downloads are removed`, async t => {
  const f = await fixture(t), r = f.run(['install'], { CHROME_FAIL: failure });
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /Chrome/);
  if (failure === 'download') assert.deepEqual((await f.calls()).map(c => c[0]), ['curl']);
  await f.cleaned();
});

test('failed package installation never installs Chrome', async t => {
  const f = await fixture(t), r = f.run(['install'], { PACKAGE_FAILURE: '7' });
  assert.equal(r.status, 7);
  assert.deepEqual(await f.calls(), []);
});

for (const action of ['update', 'doctor', 'version', 'help', 'mcp']) test(`${action} never installs Chrome`, async t => {
  const f = await fixture(t), r = f.run([action]);
  assert.equal(r.status, 0, r.stderr);
  assert.deepEqual(await f.calls(), []);
});
