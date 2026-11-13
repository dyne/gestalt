import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

// Local dependency installs and npx inspection do not provision a user profile.
if ((process.env.npm_config_global ?? process.env.NPM_CONFIG_GLOBAL) === 'true') {
  // A global install must not make the delegated runtime's npm installs global
  // or redirect their explicit prefixes into this package's installation.
  const environment = { ...process.env };
  for (const key of Object.keys(environment)) {
    if (/^npm_config_(?:global|prefix|location|workspace|workspaces|include_workspace_root)$/i.test(key))
      delete environment[key];
  }
  environment.npm_config_global = 'false';
  const result = spawnSync('bash', [fileURLToPath(new URL('./bin/gestalt', import.meta.url)), 'agents', 'install'], {
    env: environment,
    stdio: 'inherit',
  });
  if (result.error || result.status !== 0) {
    console.error('gestalt: Agents setup failed. Ensure Codex, Bash and Node.js 24+ are installed, then retry npm install -g @jaromil/gestalt. Existing installations can run: gestalt agents install');
    process.exitCode = result.status || 1;
  }
}
