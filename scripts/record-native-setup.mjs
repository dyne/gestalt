import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { captureCopyTree, compareCopyTrees } from './native-copy-provenance.mjs';
const root = path.resolve(import.meta.dirname, '..');
const evidence = path.join(root, 'native-evidence/setup');
const dependency = path.join(root, '.native-dependencies/agents');
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const git = (cwd, ...args) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();
await mkdir(evidence, { recursive: true });
if (process.argv[2] === '--verify-copy') {
  const expected = JSON.parse(await readFile(path.join(evidence, 'checkout.json'), 'utf8'));
  for (const [directory, entry] of [[root, expected.manager], [dependency, expected.agents]])
    for (const file of entry.files) assert.equal(digest(await readFile(path.join(directory, file.path))), file.sha256, `copied source differs: ${file.path}`);
  assert.ok(Array.isArray(expected.toolchain?.entries), 'installed dependency inventory is required before copying');
  const actual = await captureCopyTree(path.join(root, 'node_modules'));
  const differences = compareCopyTrees(expected.toolchain.entries, actual);
  await writeFile(path.join(evidence, 'copied-toolchain-verification.json'), JSON.stringify({ observedAt: new Date().toISOString(),
    status: differences.length ? 'mismatch' : 'matched', expectedEntries: expected.toolchain.entries.length, actualEntries: actual.length, differences }, null, 2));
  assert.equal(differences.length, 0, `copied installed toolchain differs at ${differences[0]?.path}; see copied-toolchain-verification.json for every mismatch`);
  await writeFile(path.join(evidence, 'copied-source-verification.json'), JSON.stringify({ observedAt: new Date().toISOString(), status: 'matched', manager: expected.manager.head, managerTree: expected.manager.tree,
    agents: expected.agents.head, agentsTree: expected.agents.tree, files: expected.manager.files.length + expected.agents.files.length,
    installedToolchainEntries: actual.length }));
} else {
  const inspect = async directory => {
    const files = [];
    const entries = execFileSync('git', ['ls-files', '-s', '-z'], { cwd: directory, encoding: 'utf8' }).split('\0').filter(Boolean);
    for (const entry of entries) {
      const match = /^(\d+) ([a-f0-9]+) 0\t(.+)$/.exec(entry); assert.ok(match, 'invalid source index entry');
      if (match[1] === '120000') throw new Error(`source record requires regular tracked files: ${match[3]}`);
      files.push({ path: match[3], mode: match[1], gitBlob: match[2], sha256: digest(await readFile(path.join(directory, match[3]))) });
    }
    return { head: git(directory, 'rev-parse', 'HEAD'), tree: git(directory, 'rev-parse', 'HEAD^{tree}'), branch: git(directory, 'rev-parse', '--abbrev-ref', 'HEAD'), files };
  };
  const agents = await inspect(dependency);
  const acceptedAgents = JSON.parse(await readFile(path.join(root, 'tests/fixtures/native-runtime/agents-source.json'), 'utf8'));
  assert.equal(agents.head, acceptedAgents.acceptedCommit, 'CI must consume the accepted Agents commit');
  assert.equal(agents.tree, acceptedAgents.acceptedTree, 'CI must consume the accepted Agents tree');
  const toolchain = { root: 'node_modules', entries: await captureCopyTree(path.join(root, 'node_modules')) };
  await writeFile(path.join(evidence, 'checkout.json'), JSON.stringify({ observedAt: new Date().toISOString(), githubSha: process.env.GITHUB_SHA,
    acceptedAgentsCommit: acceptedAgents.acceptedCommit, equivalentAgentsSnapshot: acceptedAgents.remoteCheckoutCommit,
    manager: await inspect(root), agents, toolchain }, null, 2));
}
