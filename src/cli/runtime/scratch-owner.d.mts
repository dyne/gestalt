import type { Environment } from '../contracts.ts';
export interface ScratchContext {
  readonly schemaVersion: 1 | 2;
  readonly workspacePath: string;
  readonly ownerId: string;
  readonly directory: string;
  readonly markerPath: string;
  readonly runtime?: { readonly directory: string; readonly token: string; readonly identity: string };
}
export interface ScratchOwnership { readonly context: ScratchContext; readonly lease: { readonly leaseId: string }; readonly borrowed?: boolean }
export function scratchContext(environment?: Environment, allowReleased?: boolean): ScratchContext | null;
export function scratchEnvironment(environment?: Environment): Environment;
export function scratchDiscoveryEnvironment(environment?: Environment): Environment;
export function scratchAllocate(workspace: string, kind: 'cli-session' | 'workspace-service', consumer: string): ScratchOwnership;
export function scratchBorrow(context: ScratchContext, consumer: string): ScratchOwnership;
export function scratchRelease(ownership: ScratchOwnership): boolean;
export function scratchReleaseReady(ownership: ScratchOwnership): Promise<boolean>;
export function scratchRecoveryCommand(args: readonly string[], write?: (text: string) => void): void;
export function scratchAssociate(environment: Environment, threadId: string): void;
export function scratchJson(file: string): any;
export function scratchSocketPath(endpoint: string, capacity?: number): string;
export function createScratchOwner(privateProbe?: string): {
  scratchContext: typeof scratchContext; scratchEnvironment: typeof scratchEnvironment; scratchDiscoveryEnvironment: typeof scratchDiscoveryEnvironment;
  scratchAllocate: typeof scratchAllocate; scratchBorrow: typeof scratchBorrow; scratchRelease: typeof scratchRelease; scratchReleaseReady: typeof scratchReleaseReady;
  scratchRecoveryCommand: typeof scratchRecoveryCommand; scratchAssociate: typeof scratchAssociate; scratchJson: typeof scratchJson; scratchSocketPath: typeof scratchSocketPath;
};
