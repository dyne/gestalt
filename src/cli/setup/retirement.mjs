// Exact managed-retirement ownership rules, extracted from the existing manager.
import { readFileSync, readdirSync, lstatSync, realpathSync, openSync, writeFileSync, fchmodSync, fsyncSync, closeSync, renameSync, unlinkSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { randomBytes } from 'node:crypto';
import { parse } from 'smol-toml';
import { isAbsolute } from 'node:path';
export function retireManagedConfiguration(selectedCodex, selectedHome, manager) {
const notices = new Set();
const notice = kind => notices.add(kind);
const exists = path => { try { return lstatSync(path); } catch (error) { if (error.code === 'ENOENT') return null; throw error; } };
const canonical = path => { try { return realpathSync(path); } catch { return null; } };
const codex = canonical(selectedCodex), home = canonical(selectedHome), executable = canonical(manager);
// Home aliases are allowed. Descendant links/hardlinks are never write targets.
const plain = path => {
  const info = exists(path);
  return info?.isFile() && info.nlink === 1 && (!process.getuid || info.uid === process.getuid()) ? info : null;
};
const owned = server => server && !server.url && typeof server.command === 'string' &&
  isAbsolute(server.command) && canonical(server.command) === executable &&
  Array.isArray(server.args) && JSON.stringify(server.args) === '["xerj","mcp"]';
const related = (name, server) => /xerj/i.test(name) || server?.args?.[0] === 'xerj';
const stable = value => JSON.stringify(value, (_key, item) => item && typeof item === 'object' && !Array.isArray(item)
  ? Object.fromEntries(Object.keys(item).sort().map(key => [key, item[key]])) : item);
function migrate(path) {
  if (!exists(path)) return;
  const info = plain(path);
  if (!info) { notice('configuration'); return; }
  const bytes = readFileSync(path, 'utf8');
  let config;
  try { config = parse(bytes); } catch { notice('configuration'); return; }
  const names = Object.entries(config.mcp_servers ?? {}).filter(([name, server]) => {
    if (owned(server)) return true;
    if (related(name, server)) notice('configuration');
    return false;
  }).map(([name]) => name);
  if (!names.length) return;
  const expected = structuredClone(config);
  for (const name of names) delete expected.mcp_servers[name];
  // Preserve physical bytes of every unrelated section and every standalone
  // comment. Validate the entire semantic result before committing any edit;
  // inline/dotted or multiline constructs that cannot be localized stay intact.
  let removing = false;
  const output = bytes.split(/(?<=\n)/).filter(line => {
    if (/^[ \t]*\[/.test(line)) {
      removing = false;
      try {
        const table = parse(line + '\n__gestalt_retirement_marker = true\n');
        removing = names.some(name => table.mcp_servers?.[name]);
      } catch { /* Whole-result validation below rejects uncertain edits. */ }
    }
    return !removing || line.trim() === '' || /^[ \t]*#/.test(line);
  }).join('');
  // TOML does not retain empty implicit parent tables after removing children.
  if (!Object.keys(expected.mcp_servers).length) delete expected.mcp_servers;
  try { if (stable(parse(output)) !== stable(expected)) { notice('configuration'); return; } }
  catch { notice('configuration'); return; }
  if (bytes === output) return;
  const temporary = join(dirname(path), '.gestalt-retirement-' + randomBytes(12).toString('hex'));
  let handle;
  try {
    handle = openSync(temporary, 'wx', 0o600);
    writeFileSync(handle, output); fchmodSync(handle, info.mode & 0o777); fsyncSync(handle);
    closeSync(handle); handle = undefined;
    const current = plain(path);
    if (!current || current.dev !== info.dev || current.ino !== info.ino || readFileSync(path, 'utf8') !== bytes)
      throw new Error('configuration changed during Xerj retirement; retry update');
    renameSync(temporary, path);
  } finally {
    if (handle !== undefined) closeSync(handle);
    try { unlinkSync(temporary); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
}
if (codex) {
  for (const name of readdirSync(codex).sort())
    if (name === 'config.toml' || /^[a-zA-Z0-9_-]+\.config\.toml$/.test(name)) migrate(join(codex, name));
  // No PID, socket, endpoint or record alone proves a live process belongs to
  // this updater. Do not probe it, send signals, or delete persistent data.
  if (exists(join(codex, 'xerj-data', '.lifecycle'))) notice('process records');
}
if (home && exists(join(home, 'run'))) {
  const run = join(home, 'run');
  if (!exists(run).isDirectory()) notice('restart descriptors');
  else for (const name of readdirSync(run)) if (/^mobile-[1-9][0-9]*\.restart$/.test(name)) {
    const file = join(run, name);
    if (!plain(file)) { notice('restart descriptors'); continue; }
    if (readFileSync(file).includes(Buffer.from('--xerj'))) notice('restart descriptors');
  }
}
return [...notices];
}
