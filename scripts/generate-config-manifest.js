#!/usr/bin/env node
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const repoRoot = path.resolve(__dirname, "..");
const configRoot = path.join(repoRoot, "config");
const manifestPath = path.join(configRoot, "manifest.json");
const versionPath = path.join(repoRoot, "internal", "version", "build_info.json");

function gitOutput(args) {
  try {
    return execFileSync("git", args, {
      cwd: repoRoot,
      stdio: ["ignore", "pipe", "ignore"],
    })
      .toString()
      .trim();
  } catch (error) {
    return "";
  }
}

function resolveVersionString() {
  const envVersion = (process.env.VERSION || "").trim();
  if (envVersion) {
    return envVersion;
  }
  const tagVersion = gitOutput(["describe", "--tags", "--abbrev=0"]);
  if (tagVersion) {
    return tagVersion;
  }
  const described = gitOutput(["describe", "--tags", "--always", "--dirty"]);
  if (described) {
    return described;
  }
  return "dev";
}

function parseSemver(rawVersion) {
  const cleaned = rawVersion.replace(/^v/, "");
  const match = cleaned.match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!match) {
    return { version: rawVersion, major: 0, minor: 0, patch: 0 };
  }
  return {
    version: match[0],
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
  };
}

const FNV_OFFSET_BASIS_64 = 0xcbf29ce484222325n;
const FNV_PRIME_64 = 0x100000001b3n;
const FNV_MASK_64 = 0xffffffffffffffffn;

function hashFile(filePath) {
  const contents = fs.readFileSync(filePath);
  let hash = FNV_OFFSET_BASIS_64;
  for (const byte of contents) {
    hash ^= BigInt(byte);
    hash = (hash * FNV_PRIME_64) & FNV_MASK_64;
  }
  return hash.toString(16).padStart(16, "0");
}

function walkConfig(dir, baseDir, manifest) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name.startsWith(".") || entry.name.startsWith("_")) {
      continue;
    }
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkConfig(fullPath, baseDir, manifest);
      continue;
    }
    if (!entry.isFile()) {
      continue;
    }
    const relativePath = path
      .relative(baseDir, fullPath)
      .split(path.sep)
      .join("/");
    if (relativePath === "manifest.json") {
      continue;
    }
    manifest[relativePath] = hashFile(fullPath);
  }
}

function writeJson(targetPath, data) {
  const payload = JSON.stringify(data, null, 2) + "\n";
  fs.mkdirSync(path.dirname(targetPath), { recursive: true });
  fs.writeFileSync(targetPath, payload);
}

function generateManifest() {
  if (!fs.existsSync(configRoot)) {
    throw new Error(`config directory not found at ${configRoot}`);
  }
  const manifest = {};
  walkConfig(configRoot, configRoot, manifest);
  const sorted = {};
  for (const key of Object.keys(manifest).sort()) {
    sorted[key] = manifest[key];
  }
  writeJson(manifestPath, sorted);
}

function generateVersionInfo() {
  const rawVersion = resolveVersionString();
  const parsed = parseSemver(rawVersion);
  const versionInfo = {
    version: parsed.version,
    major: parsed.major,
    minor: parsed.minor,
    patch: parsed.patch,
    built: new Date().toISOString(),
  };
  const gitCommit = gitOutput(["rev-parse", "--short", "HEAD"]);
  if (gitCommit) {
    versionInfo.git_commit = gitCommit;
  }
  writeJson(versionPath, versionInfo);
}

function main() {
  generateManifest();
  generateVersionInfo();
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
