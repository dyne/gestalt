export const storageReadToken: string;
export function windowsReadArguments(request: Readonly<Record<string, unknown>>): string[];
export function assertWindowsReadReply(reply: Uint8Array | string): void;
export function windowsReadFailure(error: unknown): string;
