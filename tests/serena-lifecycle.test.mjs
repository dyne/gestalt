import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
const manager = resolve('public/gestalt');
const tools = ['get_symbols_overview', 'find_symbol', 'initial_instructions', 'replace_symbol_body', 'insert_after_symbol', 'write_memory'];
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'serena lifecycle '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const bin = join(root, 'bin'), home = join(root, 'home'), managed = join(root, 'managed');
  await mkdir(bin); await mkdir(home);
  const script = async (name, body) => { await writeFile(join(bin, name), body); await chmod(join(bin, name), 0o755); };
  await script('uname', '#!/bin/sh\nif [ "$1" = -s ]; then echo Linux; else echo "${FAKE_ARCH:-x86_64}"; fi\n');
  await script('uv', '#!/bin/sh\necho system-uv-invoked >&2\nexit 99\n');
  await script('mv', `#!/usr/bin/env bash
set -eu
/bin/mv "$@"
if [[ \${FAIL_STAGE:-} == promotion ]]; then kill -TERM "$PPID"; fi
`);
  const uv = `#!/usr/bin/env bash
set -eu
case "$1" in
 --version) echo "uv \${UV_BAD_VERSION:-0.12.23}" ;;
 python) mkdir -p "$UV_PYTHON_INSTALL_DIR"; if [[ \${FAIL_STAGE:-} == python ]]; then exit 2; fi ;;
 tool)
   if [[ \${FAIL_STAGE:-} == download ]]; then exit 3; fi
   if [[ \${FAIL_STAGE:-} == interrupted ]]; then kill -TERM "$PPID"; exit 4; fi
   mkdir -p "$UV_TOOL_DIR/serena-agent/bin" "$UV_TOOL_BIN_DIR"
   cp "$FAKE_PYTHON" "$UV_TOOL_DIR/serena-agent/bin/python"
   cp "$FAKE_PYTHON" "$UV_TOOL_BIN_DIR/serena"
   chmod +x "$UV_TOOL_DIR/serena-agent/bin/python" "$UV_TOOL_BIN_DIR/serena" ;;
esac
`;
  const archiveRoot = join(root, 'uv-x86_64-unknown-linux-gnu');
  await mkdir(archiveRoot); await writeFile(join(archiveRoot, 'uv'), uv);
  const archive = join(root, 'uv.tar.gz');
  assert.equal(spawnSync('tar', ['-czf', archive, '-C', root, 'uv-x86_64-unknown-linux-gnu']).status, 0);
  const checksum = spawnSync('sha256sum', [archive], { encoding: 'utf8' }).stdout.split(' ')[0];
  await script('curl', `#!/usr/bin/env bash
set -eu
out=''; url=''
while (($#)); do case "$1" in --output) out=$2; shift 2 ;; https:*) url=$1; shift ;; *) shift ;; esac; done
if [[ \${FAIL_STAGE:-} == network ]]; then exit 22; fi
if [[ $url == https://pypi.org/* ]]; then
  version=\${LATEST_VERSION:-1.7.0}
  if [[ -n \${RELEASE_METADATA:-} ]]; then printf '%s' "$RELEASE_METADATA"
  else printf '{"info":{"version":"%s"},"releases":{"%s":[{"yanked":false}]}}' "$version" "$version"; fi
elif [[ $url == *.sha256 ]]; then
  printf '%s  uv.tar.gz\\n' "\${FAKE_CHECKSUM:-${checksum}}" > "$out"
else cp "$FAKE_ARCHIVE" "$out"; fi
`);
  const python = join(root, 'fake-python');
  await writeFile(python, `#!/usr/bin/env node
const fs = require('node:fs'), path = require('node:path');
if (['metadata', 'incompatible'].includes(process.env.FAIL_STAGE)) process.exit(5);
if (process.argv.includes('-c')) { console.log(process.argv[1].match(/release-([0-9]+[.][0-9]+[.][0-9]+)/)[1]); process.exit(); }
const root = process.argv[3];
const tools = ${JSON.stringify(tools)}.map(name => ({name,inputSchema:{type:'object'}}));
fs.writeFileSync(path.join(root, 'active.json'), JSON.stringify({schemaVersion:1,contractVersion:1,version:process.argv[4],
  executable:path.join(root,'bin/serena'), python:path.join(root,'tools/serena-agent/bin/python'),
  uv:path.join(root,'bin/uv'), pythonInstallDir:path.join(root,'python'), tools}));
`); await chmod(python, 0o755);
  const env = { ...process.env, PATH: bin + ':' + process.env.PATH, HOME: home, GESTALT_HOME: managed,
    CODEX_HOME: join(root, 'codex'), FAKE_ARCHIVE: archive, FAKE_PYTHON: python };
  const run = (args, extra = {}) => spawnSync('bash', [manager, 'serena', ...args], { env: { ...env, ...extra },
    input: '', encoding: 'utf8', timeout: 20000 });
  return { root, home, managed, bin, run };
}
test('private unattended install with spaces; idempotence; staged update leaves live prior release usable', async t => {
  const f = await fixture(t);
  let r = f.run(['install']); assert.equal(r.status, 0, r.stderr);
  const active = join(f.managed, 'serena/active.json');
  const before = await readFile(active, 'utf8'); const d = JSON.parse(before);
  assert.ok(d.executable.startsWith(f.managed));
  r = f.run(['install'], {FAIL_STAGE:'network'}); assert.equal(r.status, 0, r.stderr);
  assert.equal(await readFile(active, 'utf8'), before);
  r = f.run(['update']); assert.equal(r.status, 0, r.stderr);
  assert.notEqual(await readFile(active, 'utf8'), before);
  assert.ok((await readFile(d.executable)).length);
  assert.deepEqual(await readdir(f.home), []);
  assert.ok(!(await readdir(join(f.managed, 'serena'))).includes('.install-lock'));
});
for (const [failure, extra] of Object.entries({ network:{FAIL_STAGE:'network'}, checksum:{FAKE_CHECKSUM:'0'.repeat(64)},
  version:{UV_BAD_VERSION:'9.0.0'}, python:{FAIL_STAGE:'python'}, package:{FAIL_STAGE:'download'},
  validation:{FAIL_STAGE:'metadata'}, interrupted:{FAIL_STAGE:'interrupted'} })) {
  test(`failed ${failure} update retains previous executable and workspace data`, async t => {
    const f = await fixture(t); let r = f.run(['install']); assert.equal(r.status, 0, r.stderr);
    const active = join(f.managed, 'serena/active.json'), before = await readFile(active, 'utf8');
    const data = join(f.root, 'memory.md'); await writeFile(data, 'retained');
    r = f.run(['update'], extra); assert.notEqual(r.status, 0);
    assert.equal(await readFile(active, 'utf8'), before); assert.equal(await readFile(data, 'utf8'), 'retained');
    assert.deepEqual((await readdir(join(f.managed, 'serena'))).sort(),
      ['active.json', JSON.parse(before).executable.split('/').at(-3)].sort());
  });
}
test('signal immediately after promotion cannot delete the activated release', async t => {
  const f=await fixture(t);let r=f.run(['install']);assert.equal(r.status,0,r.stderr);
  const active=join(f.managed,'serena/active.json'), before=JSON.parse(await readFile(active,'utf8'));
  r=f.run(['update'],{FAIL_STAGE:'promotion'});assert.notEqual(r.status,0);
  const after=JSON.parse(await readFile(active,'utf8'));assert.notEqual(after.executable,before.executable);
  assert.ok((await readFile(after.executable)).length);assert.ok((await readFile(before.executable)).length);
  assert.ok(!(await readdir(join(f.managed,'serena'))).includes('.install-lock'));
});
test('unsupported platform/version and concurrent install produce bounded diagnostics', async t => {
  const f = await fixture(t);
  assert.match(f.run(['install'], { FAKE_ARCH:'mips' }).stderr, /unsupported Serena CPU/);
  assert.match(f.run(['update', '../2.0.0']).stderr, /stable VERSION/);
  await mkdir(join(f.managed, 'serena/.install-lock'), { recursive:true });
  assert.match(f.run(['install']).stderr, /already running/);
});
test('latest compatible release is promoted; explicit incompatible candidate retains active', async t => {
  const f = await fixture(t); let r = f.run(['install']); assert.equal(r.status,0,r.stderr);
  r = f.run(['update'], {RELEASE_METADATA:JSON.stringify({info:{version:'2.0.0rc1'},releases:{'1.7.0':[{yanked:false}],'1.8.0':[{yanked:false}],'2.0.0rc1':[{yanked:false}],'3.0.0':[{yanked:true}]}})}); assert.equal(r.status,0,r.stderr);
  const active = join(f.managed,'serena/active.json'), before = await readFile(active,'utf8');
  assert.equal(JSON.parse(before).version,'1.8.0');
  r = f.run(['update','2.0.0'],{FAIL_STAGE:'incompatible'}); assert.notEqual(r.status,0);
  assert.equal(await readFile(active,'utf8'),before);
});
test('missing dependency is diagnosed before creating installation state', async t => {
  const f = await fixture(t);
  const minimal = join(f.root, 'minimal'); await mkdir(minimal);
  const { symlink } = await import('node:fs/promises');
  await symlink(process.execPath, join(minimal, 'node'));
  const r = spawnSync('/bin/bash', [manager, 'serena', 'install'], { env: { ...process.env,
    PATH:minimal, HOME:f.home, GESTALT_HOME:f.managed, CODEX_HOME:join(f.root,'codex') }, encoding:'utf8' });
  assert.equal(r.status, 1); assert.match(r.stderr, /curl is required/);
  await assert.rejects(readdir(f.managed), {code:'ENOENT'});
});
test('real install/update smoke uses only disposable temporary tooling', { skip: !process.env.SERENA_REAL_LIFECYCLE, timeout:240000 }, async t => {
  const root = await mkdtemp(join(tmpdir(), 'serena-install-real-')); t.after(() => rm(root,{recursive:true,force:true}));
  for (const action of ['install', 'install', 'update']) {
    const r = spawnSync('bash', [manager,'serena',action], { env:{...process.env,GESTALT_HOME:root,CODEX_HOME:join(root,'codex')},
      encoding:'utf8', timeout:75000 });
    assert.equal(r.status,0,r.stderr.slice(-1500));
  }
});
