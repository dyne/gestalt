#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export async function buildOwnedProcess(root, outputDirectory = path.join(root, 'dist/cli/native')) {
  if (!['linux', 'win32', 'darwin'].includes(process.platform)) throw new Error('Unsupported ownership target');
  await mkdir(outputDirectory, { recursive: true });
  const source = path.join(root, 'src/cli/platform/native/owned-process.c');
  const output = path.join(outputDirectory, process.platform === 'win32' ? 'gestalt-owned-process.exe' : 'gestalt-owned-process');
  let environment = process.env, compiler = process.env.CC || 'cc', argv;
  if (process.platform === 'win32') {
    const programFiles = process.env['ProgramFiles(x86)'];
    const systemRoot = process.env.SystemRoot;
    if (!programFiles || !systemRoot) throw new Error('Windows compiler discovery prerequisites missing');
    const discovery = path.join(programFiles, 'Microsoft Visual Studio/Installer/vswhere.exe');
    const found = spawnSync(discovery, ['-latest', '-products', '*', '-requires', 'Microsoft.VisualStudio.Component.VC.Tools.x86.x64', '-property', 'installationPath', '-utf8'], { encoding: 'utf8', shell: false });
    const installation = found.stdout?.trim();
    if (found.status !== 0 || !installation || !path.isAbsolute(installation) || /["\r\n&|<>^%!]/.test(installation))
      throw new Error('A trusted target MSVC installation is required; no compiler download');
    const architecture = process.arch === 'arm64' ? 'arm64' : process.arch === 'x64' ? 'amd64' : null;
    if (!architecture) throw new Error('Unsupported Windows compiler architecture');
    // Only the validated installed Microsoft script enters cmd. Candidate paths
    // and target/user arguments go directly to cl/CreateProcess as token arrays.
    const script = path.join(installation, 'Common7/Tools/VsDevCmd.bat');
    const prepared = spawnSync(path.join(systemRoot, 'System32/cmd.exe'), ['/d', '/u', '/s', '/c', `""${script}" -no_logo -arch=${architecture} >nul && set"`], { encoding: 'utf16le', shell: false, windowsVerbatimArguments: true });
    if (prepared.status !== 0) throw new Error('Target MSVC environment preparation failed');
    environment = Object.fromEntries(prepared.stdout.split(/\r?\n/).filter(line => /^[^=]+=/.test(line)).map(line => {
      const equals = line.indexOf('='); return [line.slice(0, equals), line.slice(equals + 1)];
    }));
    compiler = 'cl.exe';
    argv = ['/nologo', '/W3', '/WX', '/std:c11', '/O2', '/D_CRT_SECURE_NO_WARNINGS', source, `/Fe:${output}`, `/Fo:${path.join(outputDirectory, 'owned-process.obj')}`, 'user32.lib'];
  } else argv = ['-std=c11', '-O2', '-Wall', '-Wextra', '-Werror', '-D_FORTIFY_SOURCE=2', source, '-o', output];
  const compiled = spawnSync(compiler, argv, { env: environment, encoding: 'utf8', shell: false });
  if (compiled.status !== 0) throw new Error(`Native ownership build failed: ${compiled.error?.message ?? ''}\n${compiled.stdout}\n${compiled.stderr}`);
  const digest = data => createHash('sha256').update(data).digest('hex');
  const version = spawnSync(compiler, process.platform === 'win32' ? [] : ['--version'], { env: environment, encoding: 'utf8', shell: false });
  const provenance = {
    source: 'src/cli/platform/native/owned-process.c', license: 'MIT (full license in source)',
    sourceSha256: digest(await readFile(source)), artifactSha256: digest(await readFile(output)),
    platform: process.platform, architecture: process.arch, node: process.version,
    compiler, compilerVersion: `${version.stdout ?? ''}${version.stderr ?? ''}`.trim(), argv,
    observedAt: new Date().toISOString(),
  };
  await writeFile(`${output}.provenance.json`, `${JSON.stringify(provenance, null, 2)}\n`);
  return output;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  buildOwnedProcess(path.dirname(path.dirname(fileURLToPath(import.meta.url)))).then(output => console.log(output)).catch(error => { console.error(error.message); process.exitCode = 1; });
