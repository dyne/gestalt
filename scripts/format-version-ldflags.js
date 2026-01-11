#!/usr/bin/env node
"use strict";

const fs = require("node:fs");
const path = require("node:path");

const repoRoot = path.resolve(__dirname, "..");
const infoPath = path.join(repoRoot, "internal", "version", "build_info.json");

function shellEscape(value) {
  return String(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

function main() {
  const payload = fs.readFileSync(infoPath, "utf8");
  const info = JSON.parse(payload);
  const flags = [
    `-X gestalt/internal/version.Version=${shellEscape(info.version || "")}`,
    `-X gestalt/internal/version.Major=${shellEscape(info.major ?? "")}`,
    `-X gestalt/internal/version.Minor=${shellEscape(info.minor ?? "")}`,
    `-X gestalt/internal/version.Patch=${shellEscape(info.patch ?? "")}`,
    `-X gestalt/internal/version.Built=${shellEscape(info.built || "")}`,
    `-X gestalt/internal/version.GitCommit=${shellEscape(info.git_commit || "")}`,
  ];
  process.stdout.write(flags.join(" "));
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
