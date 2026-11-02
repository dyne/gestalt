import {createHash} from 'node:crypto';
import {mkdir, writeFile, realpath} from 'node:fs/promises';
import {join} from 'node:path';

// A contract producer fixture, independent of the launcher's consumer code.
export async function sessionScratch(workspace, sessionId = 'relay-fixture') {
  workspace = await realpath(workspace);
  const ownerId = 'mobile-' + createHash('sha256').update(sessionId).digest('hex');
  for (const directory of [join(workspace, 'tmp'), join(workspace, 'tmp/sessions'), join(workspace, 'tmp/sessions', ownerId)])
    await mkdir(directory, {mode: 0o700}).catch(error => { if (error.code !== 'EEXIST') throw error; });
  const directory = join(workspace, 'tmp/sessions', ownerId);
  const markerPath = join(directory, '.gestalt-owner.json');
  const marker = {schemaVersion: 1, ownerKind: 'mobile-session', ownerId, workspacePath: workspace,
    sessionId, createdAt: new Date().toISOString(), authority: {kind: 'mobile-roster', sessionId}, providerThreads: []};
  await writeFile(markerPath, JSON.stringify(marker), {mode: 0o600});
  const context = {schemaVersion: 1, workspacePath: workspace, ownerId, directory, markerPath};
  return {context, marker, environment: {GESTALT_SESSION_TMP_CONTEXT: JSON.stringify(context),
    TMPDIR: directory, TMP: directory, TEMP: directory}};
}
