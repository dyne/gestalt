const BASE64URL_PATTERN = /^[A-Za-z0-9_-]+$/;

export function encodeSymbolId(rawSymbolId: string): string {
  return Buffer.from(rawSymbolId, 'utf-8').toString('base64url');
}

export function decodeSymbolId(inputSymbolId: string): string {
  const trimmed = inputSymbolId.trim();
  if (!trimmed) {
    return trimmed;
  }
  if (!BASE64URL_PATTERN.test(trimmed)) {
    return trimmed;
  }

  try {
    const decoded = Buffer.from(trimmed, 'base64url').toString('utf-8');
    if (!decoded.startsWith('scip-')) {
      return trimmed;
    }
    if (encodeSymbolId(decoded) !== trimmed) {
      return trimmed;
    }
    return decoded;
  } catch {
    return trimmed;
  }
}

export function encodeSymbolIdForOutput(symbolId: string): string {
  return encodeSymbolId(decodeSymbolId(symbolId));
}

