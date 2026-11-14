import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';
import { parse } from 'smol-toml';

const exec = promisify(execFile);
const source = await readFile(resolve('public/gestalt'), 'utf8');
async function fixture(t, bytes) {
  const root = await mkdtemp(join(tmpdir(), 'permissions preservation '));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, 'home'), codex = join(root, 'codex');
  await mkdir(home); await mkdir(codex);
  const manager = join(root, 'gestalt'), config = join(codex, 'config.toml');
  // Select the unchanged production writer, without setup, install or stubs.
  assert.match(source, /\nmain "\$@"\s*$/);
  await writeFile(manager, source.replace(/\nmain "\$@"\s*$/, '\nload_paths\nconfigure_codex_permissions\n'), { mode: 0o755 });
  await writeFile(config, bytes, { mode: 0o640 });
  const run = () => exec('bash', [manager], { env: { ...process.env, HOME: home, CODEX_HOME: codex, GESTALT_HOME: join(root, 'managed') }, timeout: 10000 });
  return { config, run };
}

const user = '[mcp_servers.xerj]\ncommand = "/user/own-server"\nargs = ["--custom"]\nenabled = false\n';
for (const [label, newline, suffix] of [
  ['LF tail', '\n', '\n'], ['CRLF tail', '\r\n', '\r\n'], ['no final newline', '\n', ''],
]) test(`actual permissions writer preserves managed-tail comments and user table: ${label}`, async t => {
  const comment = '# operator comment retained exactly';
  const block = user.replaceAll('\n', newline).replace(new RegExp(newline + '$'), suffix);
  const bytes = ['model = "kept"', '[permissions.workspace-git]', 'extends = ":workspace"',
    '[permissions.workspace-git.filesystem]', '"/tmp" = "write"', comment, block].join(newline);
  const f = await fixture(t, bytes);
  await f.run();
  const once = await readFile(f.config, 'utf8');
  assert.ok(once.includes(comment + newline + block), 'unrelated comment/table bytes changed');
  assert.deepEqual(parse(once).mcp_servers, parse(bytes).mcp_servers);
  assert.equal(parse(once).approval_policy, 'never');
  assert.equal(parse(once).default_permissions, 'workspace-git');
  assert.equal((await stat(f.config)).mode & 0o777, 0o640);
  await f.run(); await f.run();
  assert.equal(await readFile(f.config, 'utf8'), once, 'repeated updates must preserve identical bytes');
});

test('comments and mixed newline bytes survive before, between and after owned tables', async t => {
  const blocks = ['# before owned table\r\n\r\n', '# between managed tables\n\n', '# tail before user\r\n' + user.replaceAll('\n', '\r\n')];
  const bytes = 'model = "kept"\n' + blocks[0] + '[permissions.workspace-git]\nextends = ":workspace"\n' + blocks[1] +
    '[permissions.workspace-git.filesystem]\n"/tmp" = "write"\n' + blocks[2];
  const f = await fixture(t, bytes); await f.run();
  const once = await readFile(f.config, 'utf8');
  for (const block of blocks) assert.ok(once.includes(block), JSON.stringify(block));
  await f.run(); assert.equal(await readFile(f.config, 'utf8'), once);
});

test('quoted owned headers and legacy sandbox sections retain their standalone comments', async t => {
  const comments = ['# legacy annotation\n', '# quoted table annotation\r\n'];
  const bytes = '[sandbox_workspace_write]\nnetwork_access = true\n' + comments[0] +
    '[permissions."workspace-git".filesystem]\n"/tmp" = "write"\n' + comments[1] + user;
  const f = await fixture(t, bytes); await f.run();
  const once = await readFile(f.config, 'utf8');
  for (const comment of comments) assert.ok(once.includes(comment));
  assert.equal(parse(once).sandbox_workspace_write, undefined);
  assert.deepEqual(parse(once).mcp_servers, parse(bytes).mcp_servers);
  await f.run(); assert.equal(await readFile(f.config, 'utf8'), once);
});

test('unrelated multiline strings and arrays containing table-like lines remain byte identical', async t => {
  const block = 'developer_instructions = \'\'\'\r\n[permissions.workspace-git]\r\n# literal content\r\n\'\'\'\r\n' +
    'model = "kept"\n[mcp_servers.user]\ncommand = "own"\nargs = [\n  "# not a comment",\n  "[permissions.workspace-git]",\n]\n';
  const f = await fixture(t, block); await f.run();
  const once = await readFile(f.config, 'utf8');
  assert.ok(once.includes(block));
  assert.equal(parse(once).developer_instructions, parse(block).developer_instructions);
  assert.deepEqual(parse(once).mcp_servers, parse(block).mcp_servers);
  await f.run(); assert.equal(await readFile(f.config, 'utf8'), once);
});

test('a standalone comment at EOF keeps its bytes and remains separated from regenerated tables', async t => {
  const f = await fixture(t, '[permissions.workspace-git]\nextends = ":workspace"\n# tail without newline');
  await f.run();
  const once = await readFile(f.config, 'utf8');
  assert.ok(once.endsWith('\n# tail without newline'), 'preserve the original missing final newline');
  assert.equal(parse(once).permissions['workspace-git'].extends, ':workspace');
  await f.run(); assert.equal(await readFile(f.config, 'utf8'), once);
});

test('permissions replacement keeps its position before another setup writer table without changing user gaps', async t => {
  const gap = '\r\n# operator gap retained\r\n\r\n';
  const following = '[mcp_servers.context-mode]\ncommand = "node"\nargs = ["/managed/context-mode.mjs"]\n';
  const bytes = '[permissions.workspace-git]\nextends = ":workspace"\n' + gap + following;
  const f = await fixture(t, bytes); await f.run();
  const once = await readFile(f.config, 'utf8');
  assert.ok(once.indexOf('[permissions.workspace-git]') < once.indexOf('[mcp_servers.context-mode]'));
  assert.ok(once.includes(gap + following));
  await f.run(); await f.run();
  assert.equal(await readFile(f.config, 'utf8'), once);
});
