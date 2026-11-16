// Explicit native-CI script; ordinary tests never download a Codex artifact.
import path from 'node:path';
import { mkdir, writeFile } from 'node:fs/promises';
import { buildOwnedProcess } from '../scripts/build-owned-process.mjs';
import { pinnedCodex, exerciseCodexTerminal } from './fixtures/native-ptrace/codex.mjs';
const root = path.resolve(import.meta.dirname, '..');
const evidence = path.resolve(process.env.GESTALT_NATIVE_TEST_EVIDENCE ?? 'native-evidence', 'pinned-codex');
await mkdir(evidence, { recursive: true, mode: 0o700 });
let failure;
try {
  const fixture = path.resolve(process.env.GESTALT_NATIVE_CODEX_FIXTURE ?? path.join(evidence, 'fixtures'));
  await mkdir(fixture, { recursive: true, mode: 0o700 });
  const artifact = await pinnedCodex(path.join(evidence, 'artifact'), { homeDirectory: path.join(fixture, 'home') });
  const helper = await buildOwnedProcess(root, path.join(evidence, 'native'));
  const records = await exerciseCodexTerminal({ helper, artifact, evidence, directory: path.join(fixture, 'work') });
  console.log(JSON.stringify({ kind: 'actual-owned-pinned-codex-terminal', target: `${process.platform}-${process.arch}`, passed: 2, failed: 0, records: records.map(r => ({ mode: r.mode, claim: r.claim.kind, interactive: r.interactiveSignInChoices })) }));
} catch (error) { failure = { message: error.message, stack: error.stack }; throw error; }
finally { await writeFile(path.join(evidence, 'execution.json'), JSON.stringify({ cwd: root, command: ['node', 'tests/native-codex-terminal.mjs'], observedAt: new Date().toISOString(), failure: failure ?? null }, null, 2) + '\n'); }
