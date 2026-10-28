import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, symlink, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';

const require = createRequire(import.meta.url);
const parser = pathToFileURL(require.resolve('smol-toml')).href;
const source = await readFile(resolve('public/gestalt'), 'utf8');
const program = source.split('launch_cli_with_xerj() {')[1].split("<<'NODE'\n")[1].split('\nNODE\n')[0];

const cases = [
  { name: 'no named profile remains off despite installed package and base enabled', base: true },
  { name: 'legacy profile is off', profile: '[profiles.browser]\nmodel="fixture"\n' },
  { name: 'empty profile is off', profile: '' },
  { name: 'explicit profile off wins base on', base: true, profile: '[mcp_servers.gestalt-playwright]\nenabled=false\n' },
  { name: 'profile on is idle without supervisor request', on: true },
  { name: 'session explicit off overrides profile on', on: true, flags: ['-c', 'mcp_servers.gestalt-playwright.enabled=false'], off: true },
  { name: 'project explicit off overrides profile on', on: true, projectOff: true, off: true },
  { name: 'missing selected profile stays off', missing: true },
  { name: 'malformed selected profile stays off', profile: 'bad=[', malformed: true },
  { name: 'missing conditional skill stays usable', on: true, missingSkill: true, off: true },
  { name: 'unsupported native runtime falls back without browser', on: true, nativeVersion: '0.159.0', off: true, fallback: true },
  { name: 'absent package falls back without installation', on: true, missingPackage: true, off: true, fallback: true },
  { name: 'managed alias remains disabled without profile consent', alias: true },
  { name: 'remote user endpoint remains intact', on: true, flags: ['--remote', 'unix:///user/selected.sock'], off: true, remote: true },
  { name: 'explicit no-daemon stays on the native ordinary path', on: true, flags: ['--no-daemon'], off: true },
  { name: 'separate profile and cd arguments', on: true, long: true },
  { name: 'equals profile and cd arguments', on: true, equals: true },
  { name: 'resume retains native selection and has no browser owner', on: true, command: ['resume', '--last'] },
  { name: 'fork retains native selection and has no browser owner', on: true, command: ['fork', '--last'] },
];
for (const scenario of cases) test(`Playwright CLI bootstrap: ${scenario.name}`, { timeout: 20000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'playwright-cli-profile-'));
  const home = join(root, 'codex home'), workspace = join(root, 'workspace with spaces'), bin = join(root, 'bin');
  const managed = join(root, 'managed'), release = join(managed, 'playwright/release-0.0.1-fixture');
  const skill = join(root, 'plugin/skills/playwright/SKILL.md'), owner = join(bin, 'manager');
  let child;
  try {
    for (const path of [home, workspace, bin, join(release, 'node_modules'), join(root, 'plugin/.codex-plugin'), join(root, 'plugin/skills/playwright')])
      await mkdir(path, { recursive: true });
    await symlink(resolve('node_modules/playwright-core'), join(release, 'node_modules/playwright-core'), 'dir');
    await writeFile(join(release, 'package.json'), '{"private":true}');
    if (!scenario.missingPackage) await writeFile(join(managed, 'playwright/active.json'), JSON.stringify({ schemaVersion: 1, contractVersion: 1, version: '0.0.1', release }));
    await writeFile(skill, '---\nname: playwright\ndescription: Managed fixture browser.\n---\nFixture browser guidance.\n');
    await writeFile(join(root, 'plugin/.codex-plugin/plugin.json'), '{"name":"gestalt","skills":"./skills/"}');
    const selected = scenario.on || scenario.profile !== undefined || scenario.missing;
    if (selected && !scenario.missing) await writeFile(join(home, 'browser.config.toml'),
      scenario.profile ?? '[mcp_servers.gestalt-playwright]\nenabled=true\n');
    await writeFile(owner, `#!/usr/bin/env node
require('node:fs').appendFileSync(${JSON.stringify(join(root, 'unexpected-manager'))},JSON.stringify(process.argv.slice(2)));process.exit(1);
`, { mode: 0o755 });
    await writeFile(join(bin, 'codex.mjs'), `#!/usr/bin/env node
import fs from 'node:fs';import{createInterface}from'node:readline';import{parse}from${JSON.stringify(parser)};
const args=process.argv.slice(2),root=${JSON.stringify(root)},workspace=${JSON.stringify(workspace)};
fs.appendFileSync(root+'/invocations',JSON.stringify(args)+'\\n');
if(args[0]==='app-server'){
 createInterface({input:process.stdin}).on('line',line=>{
  const m=JSON.parse(line);if(m.id===undefined)return;let result={};
  const base={approval_policy:'on-request',features:{hooks:false},mcp_servers:{'gestalt-playwright':{enabled:${Boolean(scenario.base)}},unrelated:{command:'kept',enabled:false,env:{TOKEN:'private-unrelated-value'}},...${scenario.alias ? `{'legacy-browser':{command:${JSON.stringify(owner)},args:['playwright','mcp'],enabled:true}}` : '{}'}}};
  const project=${scenario.projectOff ? "{mcp_servers:{'gestalt-playwright':{enabled:false}}}" : '{}'};
  const session={};const merge=(a,b)=>{for(const[k,v]of Object.entries(b)){if(v&&typeof v==='object'&&!Array.isArray(v))merge(a[k]??={},v);else a[k]=v;}return a;};
  for(let i=0;i<args.length;i++)if(args[i]==='-c'){try{merge(session,parse(args[++i]));}catch{}}
  if(m.method==='initialize')result={userAgent:${JSON.stringify('codex_cli_rs/' + (scenario.nativeVersion ?? '0.160.0'))}};
  if(m.method==='config/read'){fs.writeFileSync(root+'/discovery.json',JSON.stringify(m.params));result={config:merge(merge(structuredClone(base),project),session),layers:[{name:{type:'sessionFlags'},config:session},...(Object.keys(project).length?[{name:{type:'project'},config:project}]:[]),{name:{type:'user'},config:base}]};}
  if(m.method==='skills/list')result={data:[{cwd:workspace,skills:${scenario.missingSkill ? '[]' : `[{name:'gestalt:playwright',path:${JSON.stringify(skill)},enabled:true}]`}}]};
  console.log(JSON.stringify({id:m.id,result}));
 });
}else fs.writeFileSync(root+'/launched.json',JSON.stringify(args));
`, { mode: 0o755 });
    await symlink('codex.mjs', join(bin, 'codex'));
    const selection = !selected ? [] : scenario.long ? ['--profile', 'browser'] : scenario.equals ? ['--profile=browser'] : ['-pbrowser'];
    const location = scenario.long ? ['--cd', workspace] : scenario.equals ? ['--cd=' + workspace] : ['-C' + workspace];
    const args = [...selection, ...location, '-s', 'read-only', '-a', 'on-request', ...(scenario.flags ?? []), ...(scenario.command ?? ['PROMPT_FIXTURE'])];
    const env = { ...process.env, CODEX_HOME: home, GESTALT_HOME: managed, GESTALT_MANAGER_BIN: owner,
      PATH: bin + ':' + process.env.PATH };
    child = spawn(process.execPath, ['--input-type=module', '-', ...args], { cwd: workspace, env, stdio: ['pipe', 'pipe', 'pipe', 'pipe'] });
    let stderr = ''; child.stderr.on('data', data => stderr += data); child.stdout.resume();
    child.stdin.end(program); child.stdio[3].end();
    const code = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => { child.kill('SIGTERM'); reject(new Error('profile fixture deadline')); }, 10000);
      child.once('error', reject); child.once('exit', code => { clearTimeout(timer); resolve(code); });
    });
    assert.equal(code, 0, stderr);
    const launched = JSON.parse(await readFile(join(root, 'launched.json'), 'utf8'));
    const shouldOwnRuntime = Boolean(scenario.on && !scenario.off);
    assert.equal(launched[0] === '--remote' && launched[1]?.includes('gestalt-cli-playwright-'), shouldOwnRuntime, stderr);
    assert.equal(launched.includes('--no-daemon'), !shouldOwnRuntime && !scenario.remote,
      'manager mode flag conflicts with its owned remote frontend or changed ordinary native behavior');
    for (const original of args) assert.ok(launched.includes(original), 'native argument lost: ' + original);
    if (scenario.remote) assert.ok(launched.includes('unix:///user/selected.sock'));
    assert.ok(!JSON.stringify(launched).includes('private-unrelated-value'), 'unrelated secret copied into argv');
    const discovery = JSON.parse(await readFile(join(root, 'discovery.json'), 'utf8'));
    assert.equal(discovery.cwd, workspace);
    await assert.rejects(readFile(join(root, 'unexpected-manager')), { code: 'ENOENT' }, 'idle bootstrap invoked package/browser manager');
    assert.equal(stderr.includes('managed-runtime-unavailable'), Boolean(scenario.fallback),
      'unexpected native fallback or missing explicit unavailable diagnostic');
    if (scenario.alias) assert.ok(launched.some(arg => arg.startsWith('mcp_servers=') && arg.includes('legacy-browser') && arg.includes('false')),
      'managed alias could eagerly start outside profile consent');
  } finally {
    if (child && child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
    await rm(root, { recursive: true, force: true });
  }
});
