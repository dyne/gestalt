import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import path from 'node:path';
import test from 'node:test';
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
