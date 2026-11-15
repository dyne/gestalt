import type { Environment } from '../contracts.ts';
export function scratchEnvironment(environment: Environment): Environment;
export function borrowSetupScratch(environment: Environment): unknown | null;
export function releaseSetupScratch(ownership: unknown): Promise<boolean>;
