// Test-only pinned artifact acquisition and actual TUI precondition exercise.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createHash } from 'node:crypto';
import { createReadStream, createWriteStream } from 'node:fs';
import { chmod, lstat, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { PassThrough, Readable, Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { fileURLToPath } from 'node:url';
import { NativeOwnedProcesses } from '../../../src/cli/platform/processes.ts';

const tag = 'rust-v0.164.0', commit = '6aaac694803004a63c6f393d153c96d22521a6b9';
const assets = {
  'linux-x64': ['codex-x86_64-unknown-linux-musl.tar.gz', '90dd0b03b28afcb7fa733a4f73b413db2916a47347efd43fb669cc1c774ee8e4'],
  'linux-arm64': ['codex-aarch64-unknown-linux-musl.tar.gz', '29a507a2a3fd8f6cd94b96b762d281d299d15d7902dee6132d314b78cf9108eb'],
  'darwin-arm64': ['codex-aarch64-apple-darwin.tar.gz', 'aa8299c169d80111aac05543ad2621c9bdbedb9bc252c905fe3f5a5baf9a4142'],
  'darwin-x64': ['codex-x86_64-apple-darwin.tar.gz', 'de6bbc013c56337f6ddd3f26dfbfd62aad5924cb68cd5ef6430538922bfa0e75'],
  'win32-x64': ['codex-x86_64-pc-windows-msvc.exe', '6f267830b029ee25d9339864aa867cd45de9f8fd4c5ea007d2e63c13a0c66d36'],
};
export async function digest(file) {
  const hash = createHash('sha256'); for await (const chunk of createReadStream(file)) hash.update(chunk);
  const info = await lstat(file);
  return { path: file, bytes: info.size, mode: (info.mode & 0o7777).toString(8), sha256: hash.digest('hex') };
}
export async function bounded(promise, label, milliseconds = 20000) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(Error(`${label}: deadline reached`)), milliseconds); })]); }
  finally { clearTimeout(timer); }
}
export function isolatedEnvironment(home) {
  return { ...Object.fromEntries(Object.entries(process.env).filter(([key]) =>
    !/^(CODEX_|OPENAI_|AZURE_|ANTHROPIC_|AWS_|GOOGLE_|GEMINI_)/.test(key) &&
    !['GH_TOKEN', 'GITHUB_TOKEN', 'GESTALT_MOBILE_ORG_PLAN_STATUS_DIRECTORY', 'GESTALT_MOBILE_ORG_PLAN_STATUS_FILE'].includes(key))),
  CODEX_HOME: home, TERM: 'xterm-256color' };
}
export async function command(executable, argv, options = {}) {
  const child = spawn(executable, argv, { shell: false, stdio: ['pipe', 'pipe', 'pipe'], ...options });
  const closed = once(child, 'close'); const out = [], err = [];
  child.stdout.on('data', b => out.push(Buffer.from(b))); child.stderr.on('data', b => err.push(Buffer.from(b)));
  try {
    const [exit, signal] = await bounded(closed, executable, 60000);
    return { command: [executable, ...argv], cwd: options.cwd ?? process.cwd(), exit, signal,
      stdout: Buffer.concat(out).toString(), stderr: Buffer.concat(err).toString(), observedAt: new Date().toISOString() };
  } finally {
    if (child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await bounded(closed, 'trusted command cleanup'); }
    child.stdin.destroy();
  }
}
export function verifyFrozenMetadata(metadata, name, sha256) {
  assert.equal(metadata.mode, 'frozen-public-api-capture'); assert.equal(metadata.tag, tag); assert.equal(metadata.commit, commit);
  assert.equal(metadata.reference.ref, `refs/tags/${tag}`); assert.equal(metadata.reference.object.type, 'tag');
  assert.equal(metadata.reference.object.sha, 'fe3b29455488c2e189e43ad5526a107900c5e1b7');
  assert.equal(metadata.resolved.type, 'commit'); assert.equal(metadata.resolved.sha, commit);
  const asset = metadata.assets.find(a => a.name === name); assert.ok(asset, 'Pinned target absent from verified public metadata');
  assert.equal(asset.publishedDigest, `sha256:${sha256}`, 'Frozen official artifact digest differs');
  assert.equal(asset.url, `https://github.com/openai/codex/releases/download/${tag}/${name}`);
  return asset;
}
export async function saveHttpFailure(response, directory, requestedUrl) {
  const headers = {};
  for (const key of ['date', 'server', 'content-type', 'retry-after', 'x-github-request-id', 'x-ratelimit-limit', 'x-ratelimit-remaining', 'x-ratelimit-reset', 'x-ratelimit-resource']) {
    const value = response.headers.get(key); if (value !== null) headers[key] = value;
  }
  const chunks = []; let size = 0, truncated = false;
  if (response.body) {
    const reader = response.body.getReader();
    for (;;) {
      const { done, value } = await reader.read(); if (done) break;
      const remaining = 65536 - size; chunks.push(Buffer.from(value.subarray(0, remaining))); size += Math.min(value.length, remaining);
      if (value.length >= remaining) { truncated = true; await reader.cancel(); break; }
    }
  }
  const safeUrl = value => { const u = new URL(value); return `${u.origin}${u.pathname}`; };
  const record = { requestedUrl: safeUrl(requestedUrl), responseUrl: safeUrl(response.url || requestedUrl), status: response.status,
    statusText: response.statusText, headers, body: Buffer.concat(chunks).toString(), bodyBytesCaptured: size, bodyTruncated: truncated,
    cause: 'HTTP failure observed; rate-limit or other cause is not inferred without supporting headers/body', observedAt: new Date().toISOString() };
  const file = path.join(directory, 'download-http-failure.json'); await writeFile(file, JSON.stringify(record, null, 2) + '\n'); return file;
}
export async function pinnedCodex(directory, { homeDirectory } = {}) {
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const target = assets[`${process.platform}-${process.arch}`]; assert.ok(target, 'Unverified Codex test target');
  const [name, sha256] = target;
  const metadataPath = fileURLToPath(new URL('./codex-release.json', import.meta.url));
  const metadata = JSON.parse(await readFile(metadataPath, 'utf8')), asset = verifyFrozenMetadata(metadata, name, sha256);
  const archive = path.join(directory, name);
  try { assert.equal((await digest(archive)).sha256, sha256); }
  catch (error) {
    if (error.code !== 'ENOENT') throw error;
    // No metadata API call or CI credential crosses the standard-user boundary.
    // Artifact redirects are anonymous and every response is bound to fixed SHA.
    const response = await fetch(asset.url, { signal: AbortSignal.timeout(180000) });
    if (!response.ok) { const failure = await saveHttpFailure(response, directory, asset.url); throw Error(`Official artifact download ${response.status}; ${failure}`); }
    const hash = createHash('sha256');
    await pipeline(Readable.fromWeb(response.body), new Transform({ transform(chunk, _, done) { hash.update(chunk); done(null, chunk); } }), createWriteStream(archive, { flags: 'wx', mode: 0o600 }));
    assert.equal(hash.digest('hex'), sha256);
  }
  assert.equal((await lstat(archive)).size, asset.bytes);
  let binary = archive;
  if (process.platform !== 'win32') {
    const expected = name.slice(0, -'.tar.gz'.length), bin = path.join(directory, 'bin'); await mkdir(bin, { recursive: true, mode: 0o700 });
    const listing = await command('/usr/bin/tar', ['-tf', archive]); assert.equal(listing.exit, 0, listing.stderr);
    assert.deepEqual(listing.stdout.trim().split(/\r?\n/), [expected]);
    const extracted = await command('/usr/bin/tar', ['-xf', archive, '-C', bin, expected]); assert.equal(extracted.exit, 0, extracted.stderr);
    binary = path.join(bin, expected); const info = await lstat(binary); assert.ok(info.isFile()); assert.equal(info.nlink, 1); await chmod(binary, 0o700);
  }
  const home = homeDirectory ?? path.join(directory, 'home'); await mkdir(home, { recursive: true, mode: 0o700 });
  const version = await command(binary, ['--version'], { env: isolatedEnvironment(home), cwd: directory });
  assert.equal(version.exit, 0, version.stderr); assert.match(version.stdout, /^codex-cli 0\.164\.0\s*$/);
  const record = { tag, commit, reference: metadata.reference, resolved: metadata.resolved, release: metadata.release, platform: process.platform, arch: process.arch,
    metadataVerification: { mode: metadata.mode, upstreamRechecked: false, capture: metadata.capture, sourceSnapshot: await digest(metadataPath) },
    source: `https://github.com/openai/codex/tree/${commit}`, license: `https://github.com/openai/codex/blob/${commit}/LICENSE`,
    asset: { name, url: asset.url, publishedDigest: asset.publishedDigest, ...await digest(archive) },
    binary: await digest(binary), version, acquisition: 'isolated test artifact only; no host/global installation', observedAt: new Date().toISOString() };
  await writeFile(path.join(directory, 'artifact.json'), JSON.stringify(record, null, 2) + '\n');
  return { binary, home, record };
}
export async function exerciseCodexTerminal({ helper, artifact, directory, evidence }) {
  await mkdir(directory, { recursive: true, mode: 0o700 }); await mkdir(evidence, { recursive: true });
  const environment = isolatedEnvironment(artifact.home);
  const invocation = { argv: ['--no-alt-screen', '--sandbox', 'read-only', '--ask-for-approval', 'never', '-c', 'check_for_update_on_startup=false'], cwd: directory, environment };
  const adapter = new NativeOwnedProcesses(helper), records = []; let lease, input;
  try {
    input = new PassThrough(); const pipeOut = [], pipeErr = [];
    lease = await adapter.launch(artifact.binary, invocation, { stdin: input, stdout: async b => pipeOut.push(Buffer.from(b)), stderr: async b => pipeErr.push(Buffer.from(b)) });
    await bounded(lease.completed, 'pinned Codex pipe rejection');
    const pipe = { mode: 'owned-byte-pipes', result: await lease.result, claim: lease.claim(), stdout: Buffer.concat(pipeOut).toString(), stderr: Buffer.concat(pipeErr).toString() };
    records.push(pipe); input.destroy(); assert.match(pipe.stdout + pipe.stderr, /stdin is not a terminal/); assert.deepEqual(pipe.result, { kind: 'exit', code: 1 });
    input = new PassThrough(); const output = []; let tail = '', ready, cursorReplies = 0, raw = '';
    const initialized = new Promise(resolve => { ready = resolve; });
    lease = await adapter.launchTerminal(artifact.binary, invocation, { input, output: async b => {
      output.push(Buffer.from(b)); raw = Buffer.concat(output).toString();
      const query = tail + Buffer.from(b).toString('latin1');
      for (const _ of query.matchAll(/\x1b\[6n/g)) { assert.ok(cursorReplies++ < 8, 'Repeated cursor query exceeded fixture bound'); input.write('\x1b[1;1R'); }
      tail = query.slice(-3);
      const visible = raw.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '').replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '');
      if (visible.includes('Sign in with ChatGPT') && /Device Code|OpenAI API key/.test(visible)) ready();
    } }, { columns: 110, rows: 40 }, { graceMs: 100, deadlineMs: 5000 });
    let interactive = false;
    try {
      await bounded(Promise.race([initialized.then(() => { interactive = true; }), lease.completed.then(claim => { throw Error(`Pinned TUI ended before actual sign-in choices: ${JSON.stringify(claim)}`); })]), 'actual pinned interactive sign-in screen');
      input.write(Buffer.from([3]));
    } finally {
      await bounded(lease.stop(), 'pinned TUI bounded stop'); await bounded(lease.completed, 'pinned TUI helper settlement'); input.destroy();
      records.push({ mode: 'owned-real-terminal', interactiveSignInChoices: interactive, cursorReplies, result: await lease.result, claim: lease.claim(), output: raw });
    }
    assert.equal(interactive, true); assert.doesNotMatch(raw, /(?:stdin|stdout) is not a terminal/);
  } finally {
    if (lease) { await lease.stop(); await bounded(lease.completed, 'final pinned TUI cleanup'); }
    input?.destroy();
    const record = { artifact: artifact.record, helper: await digest(helper), helperProvenance: JSON.parse(await readFile(`${helper}.provenance.json`, 'utf8')),
      invocation: { argv: invocation.argv, cwd: invocation.cwd, environmentKeys: Object.keys(environment).sort(), CODEX_HOME: environment.CODEX_HOME, TERM: environment.TERM }, requests: 'terminal reports and Ctrl-C only; no prompt, provider credentials, authentication selection or model request',
      scratchContext: { GESTALT_SESSION_TMP_CONTEXT: process.env.GESTALT_SESSION_TMP_CONTEXT ?? null, TMPDIR: process.env.TMPDIR ?? null, TMP: process.env.TMP ?? null, TEMP: process.env.TEMP ?? null },
      retainedDirectory: directory, records, observedAt: new Date().toISOString() };
    await writeFile(path.join(evidence, 'codex-terminal.json'), JSON.stringify(record, null, 2) + '\n');
  }
  return records;
}
