#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildCli } from './build-cli.mjs';

try {
  const entry = await buildCli(dirname(dirname(fileURLToPath(import.meta.url))));
  const child = spawn(process.execPath, [entry, ...process.argv.slice(2)], { stdio: 'inherit' });
  for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
  child.once('error', error => { console.error(`gestalt: ${error.message}`); process.exitCode = 1; });
  child.once('close', (code, signal) => { process.exitCode = code ?? (signal === 'SIGINT' ? 130 : signal === 'SIGTERM' ? 143 : 1); });
} catch (error) {
  console.error(`gestalt: ${error.message}`); process.exitCode = 1;
}
