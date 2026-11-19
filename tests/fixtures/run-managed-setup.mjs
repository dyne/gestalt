import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { NativeOwnedProcesses } from '../../src/cli/platform/processes.ts';

const [manager, helper, ...argv] = process.argv.slice(2);
assert.ok(path.isAbsolute(manager) && path.isAbsolute(helper));
const api = await import(pathToFileURL(manager).href);
assert.equal(api.UPDATED_MANAGER_API_VERSION, 1);
const port = new NativeOwnedProcesses(helper);
const write = (stream, bytes) => new Promise((resolve, reject) => {
  stream.write(bytes, error => error ? reject(error) : resolve());
});
const outcome = await api.continueUpdatedManager({ argv, cwd: process.cwd(), environment: process.env }, {
  stdin: process.stdin, stdout: bytes => write(process.stdout, bytes), stderr: bytes => write(process.stderr, bytes),
}, { commands: { apiVersion: 1, launch: port.launch.bind(port) }, signal: new AbortController().signal });
assert.equal(outcome.apiVersion, 1);
if (process.env.FIXTURE_MANAGED_OUTCOME) await writeFile(process.env.FIXTURE_MANAGED_OUTCOME, JSON.stringify(outcome) + '\n');
process.exitCode = outcome.status;
