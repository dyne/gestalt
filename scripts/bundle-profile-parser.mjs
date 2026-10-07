import { build } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';

// Keep the distributed manager self-contained; no runtime npm dependency.
const bundled = await build({ stdin: { contents: "export { parse } from 'smol-toml';", resolveDir: process.cwd() },
  bundle: true, write: false, minify: true, format: 'iife', globalName: 'gestaltProfileToml', legalComments: 'none' });
const license = await readFile('node_modules/smol-toml/LICENSE', 'utf8');
const code = `// BEGIN BUNDLED PROFILE TOML PARSER\n/* smol-toml 1.9.0 (BSD-3-Clause)\n${license}*/\n${bundled.outputFiles[0].text}// END BUNDLED PROFILE TOML PARSER`;
const path = 'public/gestalt';
const manager = await readFile(path, 'utf8');
await writeFile(path, manager.replace(/\/\/ BEGIN BUNDLED PROFILE TOML PARSER[\s\S]*?\/\/ END BUNDLED PROFILE TOML PARSER/, code));
