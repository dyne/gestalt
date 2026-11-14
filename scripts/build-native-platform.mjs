#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export async function buildNativePlatform(root, outputDirectory = path.join(root, 'dist/cli/native')) {
  if (process.platform === 'win32') return null; // Built-in PowerShell/.NET APIs; fixed source is bundled with JS.
  if (!['linux', 'darwin'].includes(process.platform)) throw new Error('Unsupported native peer target');
  const source = path.join(root, 'src/cli/platform/native/unix-peer.c');
  const output = path.join(outputDirectory, 'gestalt-unix-peer');
  await mkdir(outputDirectory, { recursive: true });
  const compiler = process.env.CC || 'cc';
  const argv = ['-std=c11', '-O2', '-Wall', '-Wextra', '-Werror', '-D_FORTIFY_SOURCE=2', source, '-o', output];
  const result = spawnSync(compiler, argv, { encoding: 'utf8', shell: false });
  if (result.status !== 0) throw new Error(`Native peer build failed: ${result.error?.message ?? result.stderr}`);
  await chmod(output, 0o755);
  const digest = bytes => createHash('sha256').update(bytes).digest('hex');
  const provenance = {
    source: 'src/cli/platform/native/unix-peer.c', license: 'MIT (full license in source)',
    sourceSha256: digest(await readFile(source)), artifactSha256: digest(await readFile(output)),
    platform: process.platform, architecture: process.arch, node: process.version,
    compiler, compilerVersion: spawnSync(compiler, ['--version'], { encoding: 'utf8' }).stdout.trim(),
    argv, observedAt: new Date().toISOString(),
  };
  await writeFile(`${output}.provenance.json`, `${JSON.stringify(provenance, null, 2)}\n`);
  return output;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  buildNativePlatform(path.dirname(path.dirname(fileURLToPath(import.meta.url)))).then(output => console.log(output ?? 'Windows: fixed OS API source')).catch(error => { console.error(error.message); process.exitCode = 1; });
