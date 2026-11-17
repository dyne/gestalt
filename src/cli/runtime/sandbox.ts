import { randomUUID } from 'node:crypto';
import { readFile, rm } from 'node:fs/promises';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { NativePrivateStorage } from '../platform/storage.ts';
import { diagnosticEnvironment } from './commands.ts';
import type { DiagnosticContext } from './diagnostics.ts';

export interface SandboxReport { readonly ready: boolean; readonly reason: string; readonly diagnostic?: string;
  readonly platform: string; readonly userNamespaces: Readonly<Record<string, string | null>>; readonly retained?: string }
/** Exercise Codex's selected OS sandbox. A missing capability never causes an
 * unconfined retry; Linux's helper is not an admission rule for other OSes. */
export async function probeSandbox(context: DiagnosticContext): Promise<SandboxReport> {
  const userNamespaces: Record<string, string | null> = {};
  if (process.platform === 'linux') for (const key of ['kernel.unprivileged_userns_clone', 'kernel.apparmor_restrict_unprivileged_userns', 'user.max_user_namespaces']) {
    try { userNamespaces[key] = (await readFile('/proc/sys/' + key.replaceAll('.', '/'), 'utf8')).trim(); } catch { userNamespaces[key] = null; }
  }
  const base = { platform: process.platform, userNamespaces };
  // No mutation of an inherited owner's roster/leases is authorized by a
  // read-only diagnostic. Do not create a temporary profile inside that owner
  // and then erase it on a PID/group/deadline guess.
  if (Object.hasOwn(context.invocation.environment, 'GESTALT_SESSION_TMP_CONTEXT') || Object.hasOwn(process.env, 'GESTALT_SESSION_TMP_CONTEXT'))
    return { ...base, ready: false, reason: 'sandbox-check-unavailable', diagnostic: 'An inherited session temporary owner is active. Run gestalt doctor from a native host shell to probe an isolated Codex home without changing session leases.' };
  if (!await context.commands.resolve('codex')) return { ...base, ready: false, reason: 'codex-not-found' };
  const directory = path.join(tmpdir(), 'gestalt-sandbox-doctor-' + randomUUID());
  let created = false, released = false;
  try {
    const storage = await NativePrivateStorage.open(directory, context.paths.unixProbe); created = true;
    const environment = { ...diagnosticEnvironment(context.invocation.environment), CODEX_HOME: storage.root };
    const result = await context.commands.capture('codex', ['-c', 'default_permissions=":read-only"', 'sandbox', '--', process.execPath, '-e', 'process.exit(0)'], environment, 65536, 10000);
    released = result.claim.kind === 'released';
    const ready = result.status === 0 && (released || process.platform === 'darwin');
    return { ...base, ready, reason: ready ? 'ready' : 'sandbox-startup-failed',
      ...(!ready ? { diagnostic: (result.stderr || result.stdout || 'Codex sandbox could not complete its native probe').slice(-4096) } : {}),
      ...(!released ? { retained: directory } : {}) };
  } catch (error) {
    return { ...base, ready: false, reason: 'sandbox-probe-unavailable', diagnostic: error instanceof Error ? error.message : 'native sandbox probe failed',
      ...(created && !released ? { retained: directory } : {}) };
  } finally {
    if (created && released) await rm(directory, { recursive: true, force: true });
  }
}
