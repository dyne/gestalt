import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';

// Local release data shared by shell integration fixtures; never a network source.
const root = process.argv[2];
if (!root?.startsWith('/')) throw new Error('fixture requires an absolute temporary directory');
await mkdir(`${root}/skills/nuklear-gui`, { recursive: true });
const content = '---\nname: nuklear-gui\ndescription: Local optional skill for lifecycle integration tests.\n---\n# Fixture\n';
await writeFile(`${root}/skills/nuklear-gui/SKILL.md`, content);
await writeFile(`${root}/LICENSE`, 'MIT test fixture\n');
await writeFile(`${root}/manifest.json`, JSON.stringify({ schemaVersion: 1, repository: 'dyne/gestalt-skills',
  source: 'https://github.com/dyne/gestalt-skills', version: '0.1.0', skills: [{ name: 'nuklear-gui',
    entry: 'skills/nuklear-gui/SKILL.md', files: [{ path: 'skills/nuklear-gui/SKILL.md',
      sha256: createHash('sha256').update(content).digest('hex') }] }] }));
for (const args of [['init', '--initial-branch=main', '--template='], ['add', '--all'], ['commit', '-m', 'fixture release']]) {
  execFileSync('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test',
    '-c', 'commit.gpgSign=false', ...args], { cwd: root, stdio: 'ignore' });
}
