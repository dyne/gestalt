const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { mkdtempSync, writeFileSync } = require('node:fs');
const { tmpdir } = require('node:os');
const path = require('node:path');
const test = require('node:test');

const { getNextVersion } = require('./get-next-version');

function runGit(cwd, args) {
  execFileSync('git', args, { cwd, stdio: 'ignore' });
}

function runScript(cwd) {
  const originalCwd = process.cwd();
  try {
    process.chdir(cwd);
    return getNextVersion();
  } finally {
    process.chdir(originalCwd);
  }
}

function initRepo() {
  const repoDir = mkdtempSync(path.join(tmpdir(), 'gestalt-version-'));
  runGit(repoDir, ['init']);
  writeFileSync(path.join(repoDir, 'README.md'), 'test');
  runGit(repoDir, ['add', '.']);
  runGit(repoDir, [
    '-c',
    'user.name=Gestalt',
    '-c',
    'user.email=gestalt@example.com',
    'commit',
    '-m',
    'chore: initial',
  ]);
  return repoDir;
}

test('returns 1.0.0 when no tags exist', () => {
  const repoDir = initRepo();
  const version = runScript(repoDir);
  assert.equal(version, '1.0.0');
});

test('bumps minor for feat commits', () => {
  const repoDir = initRepo();
  runGit(repoDir, ['tag', 'v1.0.0']);
  writeFileSync(path.join(repoDir, 'feature.txt'), 'feat');
  runGit(repoDir, ['add', '.']);
  runGit(repoDir, [
    '-c',
    'user.name=Gestalt',
    '-c',
    'user.email=gestalt@example.com',
    'commit',
    '-m',
    'feat: add feature',
  ]);
  const version = runScript(repoDir);
  assert.equal(version, '1.1.0');
});
