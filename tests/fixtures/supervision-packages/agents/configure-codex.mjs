#!/usr/bin/env node
import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const MANAGED_HOOK_EVENTS = [
  ["PreToolUse", "pretooluse", "local_shell|shell|shell_command|exec_command|Bash|Shell|apply_patch|Edit|Write|grep_files|ctx_execute|ctx_execute_file|ctx_batch_execute|ctx_fetch_and_index|ctx_search|ctx_index|mcp__"],
  ["PostToolUse", "posttooluse"],
  ["SessionStart", "sessionstart"],
  ["PreCompact", "precompact"],
  ["UserPromptSubmit", "userpromptsubmit"],
  ["Stop", "stop"],
];

function requireAbsolute(path, label) {
  if (!path || !isAbsolute(path)) throw new Error(`${label} must be an absolute path: ${path ?? ""}`);
  return resolve(path);
}

function requireVersionedPluginRoot(pluginRoot) {
  const expectedVersion = pluginRoot.split(/[\\/]/).at(-1);
  if (!/^\d+\.\d+\.\d+$/.test(expectedVersion ?? "")) {
    throw new Error(`context-mode plugin root must end with a stable package version: ${pluginRoot}`);
  }
  let packageVersion;
  try {
    packageVersion = JSON.parse(readFileSync(join(pluginRoot, "package.json"), "utf8")).version;
  } catch {
    throw new Error(`context-mode plugin root is missing a readable package.json: ${pluginRoot}`);
  }
  if (packageVersion !== expectedVersion) {
    throw new Error(`context-mode plugin root version mismatch: path=${expectedVersion} package=${packageVersion ?? "<missing>"}`);
  }
}

function tomlString(value) {
  return JSON.stringify(value);
}

function shellQuote(value) {
  if (/\0|\r|\n/.test(value)) throw new Error("hook launcher path contains an unsupported character");
  return `'${value.replaceAll("'", `'\\''`)}'`;
}

function sectionName(line) {
  return line.match(/^\s*\[([^\]]+)]\s*(?:#.*)?$/)?.[1] ?? null;
}

function removeSection(lines, wanted) {
  const result = [];
  for (let index = 0; index < lines.length; ) {
    if (sectionName(lines[index]) !== wanted) {
      result.push(lines[index++]);
      continue;
    }
    index += 1;
    while (index < lines.length && sectionName(lines[index]) === null) index += 1;
  }
  return result;
}

function setFeatureHooks(lines) {
  const start = lines.findIndex((line) => sectionName(line) === "features");
  if (start === -1) return [...lines, ...(lines.at(-1) === "" ? [] : [""]), "[features]", "hooks = true"];
  let end = start + 1;
  while (end < lines.length && sectionName(lines[end]) === null) end += 1;
  let replaced = false;
  const body = [];
  for (const line of lines.slice(start + 1, end)) {
    if (/^\s*hooks\s*=/.test(line)) {
      if (!replaced) body.push("hooks = true");
      replaced = true;
    } else {
      body.push(line);
    }
  }
  if (!replaced) body.push("hooks = true");
  return [...lines.slice(0, start + 1), ...body, ...lines.slice(end)];
}

function setPluginDisabled(lines, pluginId) {
  const wanted = `plugins.${tomlString(pluginId)}`;
  const start = lines.findIndex((line) => sectionName(line) === wanted);
  if (start === -1) return appendSection(lines, wanted, ["enabled = false"]);
  let end = start + 1;
  while (end < lines.length && sectionName(lines[end]) === null) end += 1;
  let replaced = false;
  const body = [];
  for (const line of lines.slice(start + 1, end)) {
    if (/^\s*enabled\s*=/.test(line)) {
      if (!replaced) body.push("enabled = false");
      replaced = true;
    } else {
      body.push(line);
    }
  }
  if (!replaced) body.push("enabled = false");
  return [...lines.slice(0, start + 1), ...body, ...lines.slice(end)];
}

function removeInlineContextModeServer(lines) {
  const start = lines.findIndex((line) => sectionName(line) === "mcp_servers");
  if (start === -1) return lines;
  let end = start + 1;
  while (end < lines.length && sectionName(lines[end]) === null) end += 1;
  const body = lines
    .slice(start + 1, end)
    .filter((line) => !/^\s*(?:context-mode|"context-mode"|'context-mode')\s*=/.test(line));
  return [...lines.slice(0, start + 1), ...body, ...lines.slice(end)];
}

function appendSection(lines, header, values) {
  while (lines.length > 0 && lines.at(-1) === "") lines.pop();
  if (lines.length > 0) lines.push("");
  lines.push(`[${header}]`, ...values);
  return lines;
}

export function reconcileCodexToml(source, { launcherPath, pluginId }) {
  let lines = source ? source.replaceAll("\r\n", "\n").split("\n") : [];
  lines = setFeatureHooks(lines);
  lines = removeInlineContextModeServer(lines);
  for (const section of [
    "mcp_servers.context-mode",
    'mcp_servers."context-mode"',
    "mcp_servers.'context-mode'",
  ]) {
    lines = removeSection(lines, section);
  }
  lines = removeSection(lines, `plugins.${tomlString(pluginId)}.mcp_servers.context-mode`);
  lines = setPluginDisabled(lines, pluginId);
  lines = appendSection(lines, "mcp_servers.context-mode", [
    'command = "node"',
    `args = [${tomlString(launcherPath)}]`,
    "enabled = true",
    "required = true",
    'default_tools_approval_mode = "approve"',
  ]);
  return `${lines.join("\n")}\n`;
}

function hookCommands(value, commands = []) {
  if (!value || typeof value !== "object") return commands;
  if (typeof value.command === "string") commands.push(value.command);
  if (Array.isArray(value)) for (const item of value) hookCommands(item, commands);
  else for (const item of Object.values(value)) hookCommands(item, commands);
  return commands;
}

function isManagedHook(value) {
  return hookCommands(value).some((command) =>
    /context-mode-hook\.mjs|context-mode\s+hook\s+codex|context-mode[/\\].*runtime-hook\.mjs/.test(command),
  );
}

export function reconcileCodexHooks(source, hookLauncherPath) {
  const parsed = source.trim() ? JSON.parse(source) : {};
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("hooks.json must contain a JSON object");
  }
  if (parsed.hooks === undefined) parsed.hooks = {};
  if (!parsed.hooks || typeof parsed.hooks !== "object" || Array.isArray(parsed.hooks)) {
    throw new Error("hooks.json hooks must contain a JSON object");
  }
  for (const [event, argument, matcher] of MANAGED_HOOK_EVENTS) {
    const existing = parsed.hooks[event];
    if (existing !== undefined && !Array.isArray(existing)) {
      throw new Error(`hooks.json ${event} must contain an array`);
    }
    const entry = {
      ...(matcher ? { matcher } : {}),
      hooks: [
        {
          type: "command",
          command: `: ${shellQuote(`context-mode hook codex ${argument}`)}; node ${shellQuote(hookLauncherPath)} ${argument}`,
        },
      ],
    };
    parsed.hooks[event] = [...(existing ?? []).filter((item) => !isManagedHook(item)), entry];
  }
  return `${JSON.stringify(parsed, null, 2)}\n`;
}

function writeIfChanged(path, contents, mode = 0o644) {
  const current = existsSync(path) ? readFileSync(path, "utf8") : null;
  if (current === contents) return false;
  mkdirSync(dirname(path), { recursive: true });
  const temporary = `${path}.tmp-${process.pid}`;
  writeFileSync(temporary, contents, { encoding: "utf8", mode });
  if (existsSync(path)) chmodSync(temporary, statSync(path).mode & 0o777);
  renameSync(temporary, path);
  return true;
}

function mcpLauncher(pluginRoot, gestaltHome) {
  return `#!/usr/bin/env node
import { pathToFileURL } from "node:url";

process.env.GESTALT_HOME ||= ${JSON.stringify(gestaltHome)};
process.env.CONTEXT_MODE_PLATFORM = "codex";
process.env.CONTEXT_MODE_WORKSPACE ||= process.cwd();
await import(pathToFileURL(${JSON.stringify(join(pluginRoot, "start.mjs"))}).href);
`;
}

function hookLauncher(pluginRoot, gestaltHome) {
  return `#!/usr/bin/env node
import { pathToFileURL } from "node:url";

process.env.GESTALT_HOME ||= ${JSON.stringify(gestaltHome)};
process.env.CONTEXT_MODE_PLATFORM = "codex";
await import(pathToFileURL(${JSON.stringify(join(pluginRoot, "hooks", "runtime-hook.mjs"))}).href);
`;
}

export function configureCodexIntegration({ codexHome, gestaltHome, pluginRoot, pluginId }) {
  codexHome = requireAbsolute(codexHome, "CODEX_HOME");
  gestaltHome = requireAbsolute(gestaltHome, "GESTALT_HOME");
  pluginRoot = requireAbsolute(pluginRoot, "context-mode plugin root");
  requireVersionedPluginRoot(pluginRoot);
  if (!pluginId || !/^[A-Za-z0-9._+-]+@[A-Za-z0-9._+-]+$/.test(pluginId)) {
    throw new Error(`invalid context-mode plugin id: ${pluginId ?? ""}`);
  }
  for (const relative of ["start.mjs", join("hooks", "runtime-hook.mjs")]) {
    if (!existsSync(join(pluginRoot, relative))) throw new Error(`context-mode script is missing: ${join(pluginRoot, relative)}`);
  }

  const bin = join(codexHome, "bin");
  const launcherPath = join(bin, "context-mode-mcp.mjs");
  const hookLauncherPath = join(bin, "context-mode-hook.mjs");
  const configPath = join(codexHome, "config.toml");
  const hooksPath = join(codexHome, "hooks.json");
  const changed = [];
  if (writeIfChanged(launcherPath, mcpLauncher(pluginRoot, gestaltHome), 0o755)) changed.push(launcherPath);
  if (writeIfChanged(hookLauncherPath, hookLauncher(pluginRoot, gestaltHome), 0o755)) changed.push(hookLauncherPath);
  const config = reconcileCodexToml(existsSync(configPath) ? readFileSync(configPath, "utf8") : "", {
    launcherPath,
    pluginId,
  });
  if (writeIfChanged(configPath, config)) changed.push(configPath);
  const hooks = reconcileCodexHooks(existsSync(hooksPath) ? readFileSync(hooksPath, "utf8") : "", hookLauncherPath);
  if (writeIfChanged(hooksPath, hooks)) changed.push(hooksPath);
  return { changed, configPath, hooksPath, launcherPath, hookLauncherPath };
}

function parseArgs(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index += 2) {
    const key = argv[index];
    const value = argv[index + 1];
    if (!key?.startsWith("--") || value === undefined) throw new Error(`invalid argument: ${key ?? ""}`);
    options[key.slice(2)] = value;
  }
  return options;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const options = parseArgs(process.argv.slice(2));
  const result = configureCodexIntegration({
    codexHome: options["codex-home"],
    gestaltHome: options["gestalt-home"],
    pluginRoot: options["plugin-root"],
    pluginId: options["plugin-id"],
  });
  process.stdout.write(
    result.changed.length > 0
      ? `context-mode Codex integration repaired: ${result.changed.length} file(s) updated\n`
      : "context-mode Codex integration already current\n",
  );
}
