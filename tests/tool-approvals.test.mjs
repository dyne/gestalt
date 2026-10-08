import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { parse } from 'smol-toml';

const manager = new URL('../public/gestalt', import.meta.url).pathname;
const source = await readFile(manager, 'utf8');
const helper = source.slice(source.indexOf('configure_codex_tool_approvals() {'), source.indexOf('\ninstall_mobile() {'));
for (const policy of ['never', 'on-request']) {
  test(`tool defaults for ${policy} preserve explicit overrides and are idempotent`, async () => {
    const root = await mkdtemp(join(tmpdir(), 'gestalt-approvals-'));
    try {
      const configPath = join(root, 'config.toml');
      const config = `approval_policy = "${policy}"
model = "example" # preserve this comment
[apps.custom]
default_tools_approval_mode = "prompt"
[mcp_servers.fresh]
command = "example"
enabled = false
[mcp_servers.custom]
command = "example"
default_tools_approval_mode = "writes"
[mcp_servers.fresh.tools.edit]
approval_mode = "prompt"
[plugins."custom@market".mcp_servers.example]
[plugins."custom@market".mcp_servers.overridden]
default_tools_approval_mode = "auto"
`;
      await writeFile(configPath, config, { mode: 0o600 });
      const script = join(root, 'helper.sh');
      // Use the real self-contained distribution as the trusted parser source.
      await writeFile(script, helper.replace('${BASH_SOURCE[0]}', manager) + '\nconfigure_codex_tool_approvals\n');
      const run = () => {
        const result = spawnSync('bash', [script], { env: { ...process.env, codex_home: root }, encoding: 'utf8' });
        assert.equal(result.status, 0, result.stderr);
      };
      run();
      const first = await readFile(configPath, 'utf8');
      const parsed = parse(first);
      if (policy === 'never') {
        assert.equal(parsed.apps._default.default_tools_approval_mode, 'approve');
        assert.equal(parsed.mcp_servers.fresh.default_tools_approval_mode, 'approve');
        assert.equal(parsed.plugins['custom@market'].mcp_servers.example.default_tools_approval_mode, 'approve');
      } else assert.equal(first, config);
      assert.equal(parsed.model, 'example');
      assert.ok(first.includes('# preserve this comment'));
      assert.equal(parsed.apps.custom.default_tools_approval_mode, 'prompt');
      assert.equal(parsed.mcp_servers.custom.default_tools_approval_mode, 'writes');
      assert.equal(parsed.mcp_servers.fresh.enabled, false);
      assert.equal(parsed.mcp_servers.fresh.tools.edit.approval_mode, 'prompt');
      assert.equal(parsed.plugins['custom@market'].mcp_servers.overridden.default_tools_approval_mode, 'auto');
      run();
      assert.equal(await readFile(configPath, 'utf8'), first);
      assert.equal((await stat(configPath)).mode & 0o777, 0o600);
    } finally { await rm(root, { recursive: true, force: true }); }
  });
}

test('inline TOML and an explicit app default retain their meaning', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gestalt-inline-approvals-'));
  try {
    const configPath = join(root, 'config.toml');
    await writeFile(configPath, 'approval_policy="never"\napps={_default={default_tools_approval_mode="prompt"}}\nmcp_servers={example={command="example",tools={edit={approval_mode="prompt"}}}}\n');
    const script = join(root, 'helper.sh');
    await writeFile(script, helper.replace('${BASH_SOURCE[0]}', manager) + '\nconfigure_codex_tool_approvals\n');
    const result = spawnSync('bash', [script], { env: { ...process.env, codex_home: root }, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    const parsed = parse(await readFile(configPath, 'utf8'));
    assert.equal(parsed.apps._default.default_tools_approval_mode, 'prompt');
    assert.equal(parsed.mcp_servers.example.default_tools_approval_mode, 'approve');
    assert.equal(parsed.mcp_servers.example.tools.edit.approval_mode, 'prompt');
  } finally { await rm(root, { recursive: true, force: true }); }
});
