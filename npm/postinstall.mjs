import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const globalInstall = (process.env.npm_config_global ?? process.env.NPM_CONFIG_GLOBAL) === 'true';
const manager = fileURLToPath(new URL('./bin/gestalt', import.meta.url));

// Delegated commands must use the selected profile and their own npm prefixes.
const environment = { ...process.env };
for (const key of Object.keys(environment)) {
  if (/^npm_config_(?:global|prefix|location|workspace|workspaces|include_workspace_root)$/i.test(key))
    delete environment[key];
}
environment.npm_config_global = 'false';

function run(...args) {
  return spawnSync(process.execPath, [manager, ...args], { env: environment, stdio: 'inherit' });
}

// Only global installs provision Agents. Local dependency installs just report.
if (globalInstall) {
  const result = run('agents', 'install');
  if (result.error || result.status !== 0) {
    console.error('gestalt: Agents setup failed. Ensure Codex and Node.js 24+ are installed (Unix also requires Bash), then retry npm install -g @jaromil/gestalt@latest --foreground-scripts. Existing installations can run: gestalt agents install');
    process.exitCode = result.status || 1;
  }
}
if (!process.exitCode) {
  const result = run('version');
  if (result.error || result.status !== 0)
    console.error('gestalt: Could not finish the installed version report. Run gestalt version to retry.');
}
