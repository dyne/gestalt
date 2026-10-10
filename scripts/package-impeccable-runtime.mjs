#!/usr/bin/env node
import { readFileSync, writeFileSync, mkdirSync, copyFileSync, mkdtempSync, rmSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const inputs = join(root, 'vendor/impeccable');
const pin = JSON.parse(readFileSync(join(inputs, 'provenance.json'), 'utf8'));
const [source, binary, output] = process.argv.slice(2).map(p => resolve(p));
if (!source || !binary || !output) throw Error('usage: package-impeccable-runtime.mjs SOURCE BINARY NEW_OUTPUT');
const hash = data => createHash('sha256').update(data).digest('hex');
function command(command, args, cwd = source) {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', timeout: 30000, maxBuffer: 2000000 });
  if (result.status !== 0 || result.error) throw Error(`${command} failed`);
  return result.stdout.trim();
}
if (command('git', ['write-tree']) !== pin.reconstructedTree || command('git', ['rev-parse', 'HEAD']) !== pin.baseCommit)
  throw Error('source does not reconstruct the accepted adaptation');
command('git', ['diff', '--exit-code']);
for (const [name, expected] of [['Cargo.lock', pin.cargoLockSha256], ['bun.lock', pin.bunLockSha256]])
  if (hash(readFileSync(join(source, name))) !== expected) throw Error(`${name} changed`);
if (hash(readFileSync(join(inputs, 'remote-public-url.patch'))) !== pin.patchSha256) throw Error('patch changed');
if (command(binary, ['engine-probe']) !== `impeccable-engine ${pin.engineVersion}` ||
    command(binary, ['--version']) !== pin.cliVersion) throw Error('binary identity mismatch');
if (!command(binary, ['live-server', '--help']).includes('IMPECCABLE_LIVE_PUBLIC_BASE_URL') ||
    !command(binary, ['live-poll', '--help']).includes('--then-poll')) throw Error('binary protocol mismatch');
if (command('readelf', ['-l', binary]).includes('INTERP') || !command('readelf', ['-h', binary]).includes('Advanced Micro Devices X86-64'))
  throw Error('expected static Linux x86_64 executable');
mkdirSync(output, { recursive: false });
const temporary = mkdtempSync(join(output, 'package-'));
try {
  const files = ['impeccable', 'LICENSE', 'NOTICE.md', 'SOURCE.md', 'source.patch', 'source.tar.gz'];
  copyFileSync(binary, join(temporary, 'impeccable'));
  for (const name of ['LICENSE', 'NOTICE.md']) copyFileSync(join(source, name), join(temporary, name));
  copyFileSync(join(inputs, 'remote-public-url.patch'), join(temporary, 'source.patch'));
  command('git', ['archive', '--format=tar', '--mtime=@0', '--output=' + join(temporary, 'source.tar'), pin.reconstructedTree]);
  command('gzip', ['-n', join(temporary, 'source.tar')]);
  const ci = process.env.GITHUB_ACTIONS === 'true' ? {
    repository: process.env.GITHUB_REPOSITORY, sha: process.env.GITHUB_SHA, ref: process.env.GITHUB_REF,
    runId: process.env.GITHUB_RUN_ID, runAttempt: process.env.GITHUB_RUN_ATTEMPT,
    workflowRef: process.env.GITHUB_WORKFLOW_REF,
  } : null;
  if (ci && (ci.repository !== 'dyne/gestalt' || !/^[a-f0-9]{40}$/.test(ci.sha) || !/^\d+$/.test(ci.runId)))
    throw Error('invalid CI provenance');
  const runtimeSmoke = JSON.parse(readFileSync(join(source, 'test-results/runtime-smoke.json'), 'utf8'));
  if (runtimeSmoke.state !== 'passed' || runtimeSmoke.binarySha256 !== hash(readFileSync(binary)) ||
      runtimeSmoke.health !== 'ok' || runtimeSmoke.shutdownExit !== 0 || !runtimeSmoke.serverRecordRemoved || !runtimeSmoke.listenerClosed || !runtimeSmoke.publicBaseUrlConfigured)
    throw Error('missing successful startup smoke for this exact binary');
  writeFileSync(join(temporary, 'SOURCE.md'), `# Gestalt Impeccable Live adaptation\n\n` +
    `Original work: ${pin.upstream}; ${pin.license}; see LICENSE and NOTICE.md.\n` +
    `AI assisted the browser-facing URL adaptation; modified files are in source.patch.\n` +
    `Public base ${pin.baseCommit}, patch SHA256 ${pin.patchSha256}.\n` +
    `Accepted local adaptation identity ${pin.acceptedAdaptationCommit} is not a public fetch target.\n` +
    `Reconstructed source tree ${pin.reconstructedTree}; source.tar.gz contains that exact tree.\n` +
    `Rust ${pin.rust}, Node ${pin.node}, target ${pin.platform}.\n` +
    `Build: cargo build --release -p impeccable --locked --target ${pin.platform}.\n` +
    `Native engine ${pin.engineVersion}, CLI ${pin.cliVersion}, npm manifest ${pin.npmVersion}.\n` +
    `CI-built: ${ci ? 'yes, run ' + ci.runId : 'NO: local packaging validation only; not publishable'}.\n` +
    `Caddy/DNS/TLS, preview authorization, exclusive relay integration and manager acceptance are separate gates.\n` +
    `Bounded runtime smoke: actual start, health OK, clean shutdown, listener closed and server record removed.\n` +
    `Full upstream Impeccable test suites are not run in Gestalt CI; full manager checks remain separate.\n`);
  const metadata = {
    schemaVersion: 1, contractVersion: 1, release: pin.release, platform: pin.platform,
    engineVersion: pin.engineVersion, cliVersion: pin.cliVersion, npmVersion: pin.npmVersion,
    upstream: pin.upstream, sourceCommit: pin.acceptedAdaptationCommit, baseCommit: pin.baseCommit,
    reconstructedTree: pin.reconstructedTree, patchSha256: pin.patchSha256,
    publicBaseUrl: true, copyAgent: 'chat', protocol: 'impeccable-live-poll-v1', license: pin.license,
    ci, runtimeSmoke, files: Object.fromEntries(files.map(name => [name, hash(readFileSync(join(temporary, name)))])),
  };
  writeFileSync(join(temporary, 'adaptation.json'), JSON.stringify(metadata, null, 2) + '\n');
  const name = `impeccable-${pin.release}-${pin.platform}.tar.gz`;
  command('tar', ['--sort=name', '--mtime=@0', '--owner=0', '--group=0', '--numeric-owner',
    '--mode=u+rwX,go+rX,go-w', '-C', temporary, '-cf', join(output, 'release.tar'), ...files, 'adaptation.json']);
  command('gzip', ['-n', join(output, 'release.tar')]);
  const { renameSync } = await import('node:fs');
  renameSync(join(output, 'release.tar.gz'), join(output, name));
  const archiveSha256 = hash(readFileSync(join(output, name)));
  writeFileSync(join(output, name + '.sha256'), `${archiveSha256}  ${name}\n`);
  const provenance = { schemaVersion: 1, origin: ci ? 'github-actions' : 'local-validation-only',
    ...pin, ci, archive: name, archiveSha256, binarySha256: metadata.files.impeccable,
    adaptationSha256: hash(readFileSync(join(temporary, 'adaptation.json'))),
    compiler: command('x86_64-linux-gnu-gcc', ['--version']).split('\n')[0],
    runnerImage: { os: process.env.ImageOS ?? null, version: process.env.ImageVersion ?? null },
    runtimeSmoke,
  };
  writeFileSync(join(output, 'build-provenance.json'), JSON.stringify(provenance, null, 2) + '\n');
  console.log(JSON.stringify({ archive: name, archiveSha256, origin: provenance.origin }));
} finally { rmSync(temporary, { recursive: true, force: true }); }
