// Extracted from the existing manager setup boundary; pure TOML transformations.
import { parse, stringify } from 'smol-toml';
import { join } from 'node:path';

function managedToolApproval(server) {
  return {...server, ...(!server.url ? {env_vars:[...new Set([...(server.env_vars ?? []),'GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'])]} : {}), default_tools_approval_mode:'approve', ...(server.tools ? {tools:Object.fromEntries(
    Object.entries(server.tools).map(([name, tool]) => [name, tool && typeof tool === 'object' &&
      Object.hasOwn(tool,'approval_mode') ? {...tool,approval_mode:'approve'} : tool]))} : {})};
}
function isGestaltManagedMcp(name, server, owner) {
  if (!server || typeof server !== 'object' || server.url) return false;
  const components = {'gestalt-serena':'serena','gestalt-playwright':'playwright'};
  if (Object.hasOwn(components,name)) return (!server.command && !server.args) ||
    ([owner.manager,'gestalt'].includes(server.command) && server.args?.[0] === components[name] && server.args?.[1] === 'mcp');
  return name === 'context-mode' && ['node','node.exe'].includes(String(server.command).replaceAll('\\','/').split('/').at(-1)) &&
    server.args?.includes(join(owner.codexHome, 'bin/context-mode-mcp.mjs'));
}
function managedMcpApprovals(config, owner) {
  const result = {...config};
  if (config.mcp_servers) result.mcp_servers = Object.fromEntries(Object.entries(config.mcp_servers)
    .map(([name,server]) => [name,isGestaltManagedMcp(name,server,owner) ? managedToolApproval(server) : server]));
  if (config.plugins) result.plugins = Object.fromEntries(Object.entries(config.plugins).map(([name,plugin]) =>
    [name,name === 'context-mode@dyne-gestalt-agents' && plugin?.mcp_servers ? {...plugin,mcp_servers:Object.fromEntries(
      Object.entries(plugin.mcp_servers).map(([serverName,server]) => [serverName,serverName === 'context-mode' ? managedToolApproval(server) : server]))} : plugin]));
  return result;
}

export function permissionConfiguration(source, homeReadRoots, temporaryRoot = '/tmp') {
parse(source); // Reject invalid input before any replacement is staged.
const newline = source.match(/\r\n|\n|\r/)?.[0] ?? '\n';
const retained = [];
let rootSection = true;
let removingSection = false;
let permissionsPosition;
let statement = '';
// Complete TOML statements keep multiline strings/arrays opaque. Retain raw
// comment, whitespace and unrelated statement bytes, including mixed newlines.
for (const line of source.match(/[^\r\n]*(?:\r\n|\n|\r|$)/g) ?? []) {
  if (!line) continue;
  statement += line;
  let parsed;
  try { parsed = parse(statement); } catch { continue; }
  const current = statement;
  statement = '';
  if (/^[ \t]*\[/.test(current)) {
    rootSection = false;
    removingSection = Boolean(parsed.sandbox_workspace_write || parsed.permissions?.['workspace-git']);
    if (removingSection) {
      permissionsPosition ??= retained.length;
      continue;
    }
  }
  if (removingSection && Object.keys(parsed).length) continue;
  if (rootSection && ['approval_policy', 'default_permissions', 'sandbox_mode'].some(key => Object.hasOwn(parsed, key))) continue;
  retained.push(current);
}
if (statement) throw new Error('Could not safely localize permissions configuration');

const root = [
  'approval_policy = "never"',
  'default_permissions = "workspace-git"',
];
const profile = [
  '[permissions.workspace-git]',
  'description = "Workspace and Git writable with local test support"',
  'extends = ":workspace"',
  '',
  '[permissions.workspace-git.network]',
  'enabled = true',
  'mode = "full"',
  'allow_local_binding = true',
  '',
  '[permissions.workspace-git.filesystem]',
  `${JSON.stringify(temporaryRoot)} = "write"`,
  ...homeReadRoots.map((path) => `${JSON.stringify(path)} = "read"`),
  '',
  '[permissions.workspace-git.filesystem.":workspace_roots"]',
  '"." = "write"',
  '".git" = "write"',
  '".agents" = "read"',
  '".codex" = "read"',
];
const position = permissionsPosition ?? retained.length;
let before = retained.slice(0, position).join('');
if (before && !/[\r\n]$/.test(before)) before += newline;
// Keep the replacement at the original owned-table boundary. Moving it past
// another setup writer's table would strand that writer's separator on repeats.
// Existing comments/blank lines stay intact; generated tables add no new gaps.
const output = root.join(newline) + newline + before +
  profile.filter(line => line !== '').join(newline) + newline + retained.slice(position).join('');
parse(output);
return output;

}

export function approvalConfiguration(source, managerPath, codexHome) {
const config = parse(source);
const expected = managedMcpApprovals(config, { manager: managerPath, codexHome });
const changes = [];
const changed = (before, after, path = []) => {
  for (const [key, value] of Object.entries(after ?? {})) {
    if (value && typeof value === 'object' && !Array.isArray(value)) changed(before?.[key], value, [...path,key]);
    else if (JSON.stringify(before?.[key]) !== JSON.stringify(value) && ['default_tools_approval_mode','approval_mode','env_vars'].includes(key)) changes.push([...path,key]);
  }
};
changed(config, expected);
if (!changes.length) return source;
// Insert into ordinary tables without discarding comments or formatting.
// Inline and dotted table definitions fall back to semantic serialization.
let output = source;
for (const fullPath of changes) {
  const path = fullPath.slice(0,-1), key = fullPath.at(-1);
  const value = fullPath.reduce((item, key) => item[key], expected);
  const rendered = stringify({[key]: value}).trimEnd();
  const header = '[' + path.map(key => JSON.stringify(key)).join('.') + ']';
  let inserted = false;
  output = output.split(/(?=^[ \t]*\[)/m).map(section => {
    const line = section.split('\n')[0];
    if (inserted || line.trimStart().startsWith('[[')) return section;
    try {
      let table = parse(line + '\n__gestalt_marker = true');
      for (const key of path) table = table?.[key];
      if (table?.__gestalt_marker !== true) return section;
      inserted = true;
      const assignment = new RegExp('^[ \\t]*'+key+'[ \\t]*=[^\\n]*','m');
      const existing = assignment.exec(section);
      if (key === 'env_vars' && existing) {
        // Extend the actual TOML array in place, retaining multiline layout,
        // comments and the operator's existing environment names.
        let end = existing.index + existing[0].length, parsed;
        while (true) {
          try { parsed = parse(section.slice(existing.index, end))[key]; break; }
          catch { const next = section.indexOf('\n', end + 1); if (next < 0) throw new Error('Incomplete environment array'); end = next; }
        }
        const original = section.slice(existing.index, end);
        let close = original.indexOf(']');
        while (close >= 0) {
          try { if (JSON.stringify(parse(original.slice(0, close + 1))[key]) === JSON.stringify(parsed)) break; } catch {}
          close = original.indexOf(']', close + 1);
        }
        if (close < 0 || !Array.isArray(parsed)) throw new Error('Invalid environment array');
        const missing = value.filter(name => !parsed.includes(name));
        if (!missing.length) return section;
        let separator = '';
        if (parsed.length) try { parse(original.slice(0, close) + ',]'); separator = ','; } catch { /* Already has a trailing comma. */ }
        const additions = stringify({env_vars: missing}).trim().replace(/^env_vars\s*=\s*\[/, '').replace(/\]$/, '');
        const updated = original.slice(0, close) + separator + ' ' + additions + original.slice(close);
        return section.slice(0, existing.index) + updated + section.slice(end);
      }
      return existing ? section.replace(assignment, rendered) : line+'\n'+rendered+section.slice(line.length);
    } catch { return section; }
  }).join('');
  if (!inserted) output += '\n' + header + '\n'+rendered+'\n';
}
const canonical = value => JSON.stringify(value, (_key, item) =>
  item && typeof item === 'object' && !Array.isArray(item)
    ? Object.fromEntries(Object.keys(item).sort().map(key => [key, item[key]])) : item);
try {
  if (canonical(parse(output)) !== canonical(expected)) output = stringify(expected);
} catch { output = stringify(expected); }
return output;
}
