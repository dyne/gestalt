#!/usr/bin/env node
'use strict';

const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const semver = require('semver');

function gitOutput(args, options = {}) {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gestalt-git-'));
  const outputPath = path.join(tempDir, 'out.txt');
  const outputFile = fs.openSync(outputPath, 'w');
  try {
    execFileSync('git', args, {
      ...options,
      stdio: ['ignore', outputFile, 'ignore'],
    });
    return fs.readFileSync(outputPath, 'utf8').trim();
  } finally {
    fs.closeSync(outputFile);
    fs.unlinkSync(outputPath);
    fs.rmdirSync(tempDir);
  }
}

function getCurrentTag() {
  try {
    return gitOutput(['describe', '--tags', '--abbrev=0']);
  } catch (error) {
    return '';
  }
}

function cleanVersion(rawVersion) {
  const cleaned = semver.clean(rawVersion);
  if (!cleaned || !semver.valid(cleaned)) {
    throw new Error(`Invalid version tag: ${rawVersion}`);
  }
  return cleaned;
}

function getCommitsSince(tag) {
  if (!tag) {
    return [];
  }
  const log = gitOutput(['log', `${tag}..HEAD`, '--pretty=format:%s%n%b%x00']);
  if (!log) {
    return [];
  }
  return log
    .split('\0')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function detectBump(commits) {
  let hasBreaking = false;
  let hasFeature = false;
  let hasFix = false;

  for (const commit of commits) {
    const lines = commit.split('\n');
    const subject = lines[0] || '';
    const body = lines.slice(1).join('\n');
    const match = subject.match(/^(\w+)(\([^)]+\))?(!)?:/);

    if (match) {
      const type = match[1];
      const hasBang = Boolean(match[3]);

      if (hasBang) {
        hasBreaking = true;
      }
      if (type === 'feat') {
        hasFeature = true;
      }
      if (type === 'fix') {
        hasFix = true;
      }
    }

    if (/BREAKING CHANGE|BREAKING-CHANGE/.test(body)) {
      hasBreaking = true;
    }
  }

  if (hasBreaking) {
    return 'major';
  }
  if (hasFeature) {
    return 'minor';
  }
  if (hasFix) {
    return 'patch';
  }
  return '';
}

function getNextVersion() {
  const currentTag = getCurrentTag();
  if (!currentTag) {
    return '1.0.0';
  }

  const currentVersion = cleanVersion(currentTag);
  const commits = getCommitsSince(currentTag);
  const bump = detectBump(commits);
  const nextVersion = bump ? semver.inc(currentVersion, bump) : currentVersion;

  if (!nextVersion) {
    throw new Error(`Unable to calculate next version from ${currentVersion}`);
  }

  return nextVersion;
}

function main() {
  const nextVersion = getNextVersion();
  console.log(nextVersion);
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

module.exports = { getNextVersion };
