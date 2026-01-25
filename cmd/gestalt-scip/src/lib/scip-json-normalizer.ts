import type { ScipDocument, ScipIndex, ScipOccurrence } from './scip-types.js';

interface ObjectRange {
  start?: { line?: number; character?: number };
  end?: { line?: number; character?: number };
}

const EMPTY_OCCURRENCE: Omit<ScipOccurrence, 'range'> = {
  symbol: '',
  symbolRoles: 0,
  overrideDocumentation: [],
  syntaxKind: 0,
  diagnostics: [],
  enclosingRange: [],
};

const EMPTY_DOCUMENT: Omit<ScipDocument, 'occurrences'> = {
  relativePath: '',
  language: '',
  symbols: [],
  text: '',
};

function isObjectRange(range: unknown): range is ObjectRange {
  return range !== null && typeof range === 'object' && !Array.isArray(range);
}

function extractPosition(
  position?: { line?: number; character?: number },
  fallbackLine?: number,
  fallbackCharacter?: number
): { line: number; character: number } {
  return {
    line: position?.line ?? fallbackLine ?? 0,
    character: position?.character ?? fallbackCharacter ?? 0,
  };
}

function convertRangeToArray(range: ObjectRange): number[] {
  const start = extractPosition(range.start);
  const end = extractPosition(range.end, start.line, start.character);
  return [start.line, start.character, end.line, end.character];
}

function getSymbolRoles(occurrence: { role?: number; symbolRoles?: number }): number {
  return occurrence.role ?? occurrence.symbolRoles ?? 0;
}

function normalizeOccurrence(occurrence: any): ScipOccurrence {
  if (!isObjectRange(occurrence.range)) {
    return occurrence;
  }

  return {
    ...EMPTY_OCCURRENCE,
    symbol: occurrence.symbol,
    symbolRoles: getSymbolRoles(occurrence),
    range: convertRangeToArray(occurrence.range),
  };
}

function getDocumentPath(document: { relativePath?: string; uri?: string }): string {
  if (document.relativePath) {
    return document.relativePath;
  }
  if (document.uri) {
    return document.uri.replace('file:///', '');
  }
  return '';
}

function normalizeDocument(document: any): ScipDocument {
  return {
    ...EMPTY_DOCUMENT,
    relativePath: getDocumentPath(document),
    language: document.language,
    occurrences: (document.occurrences ?? []).map(normalizeOccurrence),
    symbols: document.symbols,
    text: document.text,
  };
}

export function parseJsonIndex(content: string): ScipIndex | null {
  const trimmedContent = content.trim();
  if (!trimmedContent.startsWith('{')) {
    return null;
  }

  try {
    const jsonIndex = JSON.parse(trimmedContent) as ScipIndex & { documents?: any[] };
    jsonIndex.documents = jsonIndex.documents?.map(normalizeDocument) ?? [];
    return jsonIndex;
  } catch {
    return null;
  }
}
