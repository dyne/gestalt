import path from 'node:path';

export const storageReadToken = 'GESTALT_STORAGE_CHECK_V1_OK\n';
export function windowsReadArguments(request) {
  const operation = request.operation;
  const paths = operation === 'assert-private' ? [request.path] : operation === 'assert-namespace' ? request.paths : null;
  if (!Array.isArray(paths) || paths.length < 1 || paths.length > 128 ||
      paths.some(value => typeof value !== 'string' || !path.win32.isAbsolute(value) || /[\x00-\x1f\x7f]/.test(value)))
    throw Error('Windows native storage read request is invalid');
  // Re-converting an extended drive root removes its final separator in Node.
  // Forward extended bytes literally; the native path validator still applies.
  return ['--storage-check-v1', operation === 'assert-private' ? 'private' : 'namespace', '--',
    ...paths.map(value => value.startsWith('\\\\?\\') ? value : path.win32.toNamespacedPath(value))];
}
export function assertWindowsReadReply(reply) {
  if (Buffer.from(reply).toString('utf8') !== storageReadToken) throw Error('Windows native storage read reply is unconfirmed; install the matching target-built helper');
}
export function windowsReadFailure(error) {
  const diagnostic = error?.stderr && Buffer.from(error.stderr).toString('utf8').match(/^GESTALT_STORAGE_CHECK_V1 ([a-z]+) error=(\d+) index=(-?\d+)\n$/);
  if (diagnostic?.[2] === '50') return 'Windows storage ACL representation is unsupported; use standard filesystem allow/deny ACEs in a private namespace (no automatic ACL repair)';
  if (diagnostic) return `Windows private namespace/ACL validation failed (${diagnostic[1]}, error ${diagnostic[2]}, path index ${diagnostic[3]})`;
  return 'Windows native storage read is unconfirmed; install the matching target-built helper and verify the private namespace/ACL prerequisite';
}
