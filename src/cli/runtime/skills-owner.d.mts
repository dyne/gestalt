import type { SetupContext } from '../setup/context.ts';
export function manageSkills(context: SetupContext, operation: 'install' | 'update' | 'status' | 'inspect-recovery' | 'recover',
  outputMode?: 'text' | 'json' | 'version', recovery?: { readonly confirmed: boolean; readonly token: string } | null): Promise<number>;
