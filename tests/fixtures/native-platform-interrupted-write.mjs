// Test-only original MIT source. All filesystem calls operate on the real adapter stage.
import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import path from 'node:path';

const [managed, destination, probe] = process.argv.slice(2);
const nativeOpen = fs.promises.open;
let release;
const gate = new Promise(resolve => { release = resolve; });
process.on('message', message => { if (message?.kind === 'release-stage') release(); });
process.once('disconnect', () => release());
let injected = false;
fs.promises.open = async (...args) => {
  const handle = await nativeOpen(...args);
  const filename = String(args[0]);
  if (!injected && path.dirname(filename) === managed && path.basename(filename).endsWith('.stage')) {
    injected = true;
    const writeFile = handle.writeFile.bind(handle);
    handle.writeFile = async bytes => {
      const payload = Buffer.from(bytes);
      const prefix = payload.subarray(0, 8);
      await writeFile(prefix);
      await handle.sync();
      await new Promise((resolve, reject) => process.send({ kind: 'stage-held', stage: filename,
        bytesWritten: prefix.length, totalBytes: payload.length }, error => error ? reject(error) : resolve()));
      await gate;
      await writeFile(payload.subarray(prefix.length));
    };
  }
  return handle;
};
syncBuiltinESMExports();
try {
  // Import after the child-only I/O wrapper is bound. The adapter still creates
  // its actual exclusive lock/stage, checks privacy and performs its transaction.
  const { NativePrivateStorage } = await import('../../src/cli/platform/storage.ts');
  const storage = await NativePrivateStorage.open(managed, probe || undefined);
  await storage.replaceFile(destination, Buffer.from('replacement-transaction-payload'));
  if (!injected) throw new Error('The intended staged write was not intercepted');
} finally {
  release();
  fs.promises.open = nativeOpen;
  syncBuiltinESMExports();
  if (process.connected) process.disconnect();
}
