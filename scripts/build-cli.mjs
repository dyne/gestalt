#!/usr/bin/env node
import { chmod, readFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { build } from 'esbuild';
import { managerHelp, managerVersion } from './manager-metadata.mjs';

export async function buildCli(root, output = join(root, 'dist/cli/gestalt.mjs')) {
  const source = await readFile(join(root, 'public/gestalt'), 'utf8');
  await build({
    entryPoints: [join(root, 'src/cli/main.ts')], outfile: output,
    bundle: true, platform: 'node', target: 'node24', format: 'esm',
    define: { GESTALT_BUILD_VERSION: JSON.stringify(managerVersion(source)), GESTALT_BUILD_HELP: JSON.stringify(managerHelp(source)) },
  });
  await chmod(output, 0o755);
  return output;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  buildCli(dirname(dirname(fileURLToPath(import.meta.url)))).then(output => console.log(output)).catch(error => {
    console.error(`build-cli: ${error.message}`); process.exitCode = 1;
  });
}
