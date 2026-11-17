import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createScratchOwner } from './scratch-owner.mjs';

// Runtime only. Setup and omitted-argument factory callers keep their existing
// fixed PowerShell checker. No PATH/environment probe selection or fallback.
export const runtimeReadProbe = process.platform === 'win32'
  ? path.join(path.dirname(fileURLToPath(import.meta.url)), 'native/gestalt-owned-process.exe') : undefined;
export const { scratchContext, scratchEnvironment, scratchDiscoveryEnvironment, scratchAllocate, scratchBorrow,
  scratchRelease, scratchReleaseReady, scratchRecoveryCommand, scratchAssociate, scratchJson, scratchSocketPath } =
  createScratchOwner(undefined, runtimeReadProbe);
