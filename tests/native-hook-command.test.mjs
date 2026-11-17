import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import path from 'node:path';
import test from 'node:test';
import { existsSync } from 'node:fs';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { inflateRawSync } from 'node:zlib';
import { nodeHookCommand, associationHookProgram } from '../src/cli/runtime/hook-command.mjs';

test('native hook shell transports literal JavaScript/data and raw streams to Node with exact command outcome', { timeout: 30000 }, async () => {
  const literals = ['', 'space ü 東京', '$(touch escaped)', '%PATH%', '!literal!', '& | <> ^', "'quoted'", '\\trailing\\'];
  const program = `const chunks=[];for await(const b of process.stdin)chunks.push(b);process.stdout.write(JSON.stringify({literals:${JSON.stringify(literals)},input:Buffer.concat(chunks).toString('hex')}));process.stderr.write(Buffer.from([255,0,10]));process.exitCode=23;`;
  const command = nodeHookCommand(program), windows = process.platform === 'win32';
  const executable = windows ? path.join(process.env.SystemRoot, 'System32/cmd.exe') : '/bin/sh';
  const argv = windows ? ['/d', '/s', '/c', '"' + command + '"'] : ['-c', command];
  const child = spawn(executable, argv, { stdio: ['pipe', 'pipe', 'pipe'], shell: false, windowsVerbatimArguments: windows });
  const output = [], errors = []; child.stdout.on('data', bytes => output.push(bytes)); child.stderr.on('data', bytes => errors.push(bytes));
  const closed = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); });
  const input = Buffer.from([0, 255, 10, 13, 128]); child.stdin.end(input);
  assert.equal(await closed, 23, Buffer.concat(errors).toString());
  assert.deepEqual(JSON.parse(Buffer.concat(output)), { literals, input: input.toString('hex') }); assert.deepEqual(Buffer.concat(errors), Buffer.from([255, 0, 10]));
});
const powershell = process.platform === 'win32' ? path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe') :
  existsSync('/usr/bin/pwsh') ? '/usr/bin/pwsh' : null;
test('cold Windows hook bridge suppresses its own forced progress while preserving child raw stderr and status', { timeout: 30000, skip: !powershell && 'Optional local PowerShell unavailable; Windows gate always executes' }, async t => {
  const cache = await mkdtemp(path.join(tmpdir(), 'hook-bridge-cache-')); t.after(() => rm(cache, { recursive: true, force: true }));
  const command = nodeHookCommand("process.stdout.write(Buffer.from([0,255,13,10]));process.stderr.write(Buffer.from([255,0,10]));process.exitCode=23;", process.execPath, 'win32');
  let bridge = Buffer.from(command.split(' ').at(-1), 'base64').toString('utf16le');
  assert.ok(bridge.includes("$ProgressPreference='SilentlyContinue'"));
  bridge = bridge.replace("$ProgressPreference='SilentlyContinue'", "$ProgressPreference='SilentlyContinue'\nWrite-Progress -Activity 'Forced cold bridge progress' -Status 'Preparing modules' -PercentComplete 10");
  const child = spawn(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(bridge, 'utf16le').toString('base64')], {
    stdio: ['pipe', 'pipe', 'pipe'], shell: false, env: { ...process.env, XDG_CACHE_HOME: path.join(cache, 'cache'), XDG_DATA_HOME: path.join(cache, 'data'), XDG_CONFIG_HOME: path.join(cache, 'config') },
  });
  const stdout = [], stderr = []; child.stdout.on('data', b => stdout.push(b)); child.stderr.on('data', b => stderr.push(b));
  const closed = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); }); child.stdin.end();
  assert.equal(await closed, 23, Buffer.concat(stderr).toString());
  assert.deepEqual(Buffer.concat(stdout), Buffer.from([0, 255, 13, 10])); assert.deepEqual(Buffer.concat(stderr), Buffer.from([255, 0, 10]));
});
test('Windows hook commands carry compressed program data and never interpolate managed paths into cmd source', () => {
  const program = associationHookProgram('file:///C:/space%20%C3%BC%20%26/gestalt.mjs', 'retained context & %PATH%', true);
  const command = nodeHookCommand(program, 'C:\\Node ü & %PATH%\\node.exe', 'win32');
  assert.match(command, /^powershell\.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand [A-Za-z0-9+/=]+$/);
  assert.doesNotMatch(command, /Node ü|%PATH%|retained context/);
  assert.ok(command.length < 30000);
  const bridge = Buffer.from(command.split(' ').at(-1), 'base64').toString('utf16le');
  assert.match(bridge, /CopyToAsync/); assert.match(bridge, /UseShellExecute=\$false/);
  // This structural test is not Windows execution evidence. The preceding test
  // executes the same generated bridge on the actual Windows native runner.
});

test('PowerShell parses compact hook filename and arguments as exact literal values', { timeout: 30000, skip: !powershell && 'Optional local PowerShell unavailable; Windows gate always executes' }, async t => {
  const cache = await mkdtemp(path.join(tmpdir(), 'hook-literal-cache-')); t.after(() => rm(cache, { recursive: true, force: true }));
  const node = "C:\\Node ' ü 東京 $(exit 99) %PATH% &\\node.exe";
  const program = "process.stdout.write(\"' ü 東京 $(exit 99) %PATH% &\");";
  const command = nodeHookCommand(program, node, 'win32');
  const bridge = Buffer.from(command.split(' ').at(-1), 'base64').toString('utf16le');
  const start = bridge.indexOf('$p=[Diagnostics.Process]::Start($i)'); assert.ok(start > 0);
  // Execute the real parser and ProcessStartInfo assignments, but never launch
  // the deliberately nonexistent filename or execute its JavaScript.
  const inspection = bridge.slice(0, start) + `
$json=ConvertTo-Json -Compress -InputObject @{filename=$i.FileName;arguments=$i.Arguments}
$bytes=[Text.Encoding]::UTF8.GetBytes($json)
[Console]::OpenStandardOutput().Write($bytes,0,$bytes.Length)
`;
  const child = spawn(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(inspection, 'utf16le').toString('base64')], {
    stdio: ['pipe', 'pipe', 'pipe'], shell: false, env: { ...process.env, XDG_CACHE_HOME: path.join(cache, 'cache'), XDG_DATA_HOME: path.join(cache, 'data'), XDG_CONFIG_HOME: path.join(cache, 'config') },
  });
  const output = [], errors = []; child.stdout.on('data', b => output.push(b)); child.stderr.on('data', b => errors.push(b));
  const closed = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', resolve); }); child.stdin.end();
  assert.equal(await closed, 0, Buffer.concat(errors).toString()); assert.equal(Buffer.concat(errors).length, 0);
  const values = JSON.parse(Buffer.concat(output)); assert.equal(values.filename, node);
  assert.match(values.arguments, /^--input-type=module -e "/); assert.ok(values.arguments.endsWith('"'));
  const compressed = /Buffer\.from\('([A-Za-z0-9+/=]+)','base64'\)/.exec(values.arguments); assert.ok(compressed);
  assert.equal(inflateRawSync(Buffer.from(compressed[1], 'base64')).toString(), program);
});

test('captured complete Windows hook composition fits helper and payload limits without dropping argv or trust', async () => {
  const fixture = JSON.parse(await readFile(new URL('./fixtures/native-runtime/windows-hook-budget.json', import.meta.url)));
  assert.equal(fixture.schemaVersion, 1); assert.equal(fixture.observedRun, 44035381057);
  assert.match(fixture.scope, /NOT claimed as observed fresh final argv/);
  assert.equal(fixture.captureProvenance.length, 4); for (const ref of fixture.captureProvenance) assert.match(ref.sha256, /^[a-f0-9]{64}$/);
  const { query, originalUserArguments: original, representativeFinalTrustState: trust } = fixture;
  assert.deepEqual(query.argv.slice(0, 2), ['app-server', '--stdio']); assert.equal(query.argv.length, 14);
  assert.equal((trust.match(/sha256:[a-f0-9]{64}/g) ?? []).length, 4);
  assert.equal((trust.match(/trusted_hash/g) ?? []).length, 4); assert.equal((trust.match(/enabled\\?"?=true/g) ?? []).length, 4);
  const selected = query.argv.slice(2), changes = [];
  const compact = selected.map(arg => arg.replace(/powershell\.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand [A-Za-z0-9+/=]+/g, old => {
    const bridge = Buffer.from(old.split(' ').at(-1), 'base64').toString('utf16le');
    const data = /\$r=\[Text.Encoding\]::UTF8.GetString\(\[Convert\]::FromBase64String\('([^']+)'\)\)\|ConvertFrom-Json/.exec(bridge); assert.ok(data);
    const values = JSON.parse(Buffer.from(data[1], 'base64')); assert.deepEqual(Object.keys(values), ['node', 'arguments']);
    const compressed = /Buffer\.from\('([A-Za-z0-9+/=]+)','base64'\)/.exec(values.arguments); assert.ok(compressed);
    const program = inflateRawSync(Buffer.from(compressed[1], 'base64')).toString();
    const next = nodeHookCommand(program, values.node, 'win32');
    const nextBridge = Buffer.from(next.split(' ').at(-1), 'base64').toString('utf16le');
    // Only the two transport assignments may change; program bytes and all
    // pumps/wait/result/progress code remain the actual captured bridge.
    const literal = value => "'" + value.replaceAll("'", "''") + "'";
    const expected = bridge.replace(/^\$r=.*\r?\n/m, '').replace('$i.FileName=$r.node;$i.Arguments=$r.arguments;', `$i.FileName=${literal(values.node)};$i.Arguments=${literal(values.arguments)};`);
    assert.equal(nextBridge, expected); assert.ok(next.length < 30000);
    changes.push([old.length, next.length]); return next;
  }));
  assert.deepEqual(changes, [[9210, 7178], [5806, 4630], [9210, 7178], [5806, 4630]]);
  assert.equal(compact.length, selected.length);
  // CRT writer/count policy is quoted for every payload argument. libuv's
  // helper command quotes only empty/space/tab/quote arguments. JS length is
  // UTF16 code units, including surrogate pairs, exactly as both Windows APIs.
  const crt = value => '"' + value.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, '$1$1') + '"';
  const uv = value => !value || /[ \t"]/.test(value) ? crt(value) : value;
  const count = (argv, quote) => argv.map(quote).join(' ').length;
  const helper = argv => [query.executable, '--controller', String(query.observer), '--grace', '1000', '--', query.target, ...argv];
  const final = flags => ['--no-daemon', ...original, ...flags, '-c', trust];
  assert.deepEqual(final(compact).slice(1, 1 + original.length), original);
  assert.equal(final(compact).at(-1), trust);
  const measured = {
    lastObservedSuccessfulHookQueryHelper: count(helper(query.argv), uv),
    derivedFreshFinalHelper: count(helper(final(selected)), uv),
    derivedFreshPayloadIfItCouldBeReached: count([query.target, ...final(selected)], crt),
    candidateCompactFinalHelper: count(helper(final(compact)), uv),
    candidateCompactFinalPayload: count([query.target, ...final(compact)], crt),
  };
  assert.deepEqual(measured, fixture.expectedCounts); assert.equal(fixture.maximumExcludingNul, 32766);
  assert.ok(measured.derivedFreshFinalHelper > 32766); assert.ok(measured.derivedFreshPayloadIfItCouldBeReached > 32766);
  assert.ok(measured.candidateCompactFinalHelper <= 32766); assert.ok(measured.candidateCompactFinalPayload <= 32766);
});
