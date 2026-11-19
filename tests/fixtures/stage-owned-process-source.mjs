import { copyFile, mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

export async function stageOwnedProcessSource(repository, fixture, ownedProcessSource) {
  const relative = 'src/cli/platform/native';
  const directory = path.join(fixture, relative);
  await mkdir(directory, { recursive: true });
  const source = path.join(directory, 'owned-process.c');
  await writeFile(source, ownedProcessSource);
  for (const companion of ['windows-storage-check.c', 'windows-storage-check.h']) {
    await copyFile(path.join(repository, relative, companion), path.join(directory, companion));
  }
  return source;
}
