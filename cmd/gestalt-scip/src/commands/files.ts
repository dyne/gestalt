import fs from 'node:fs';
import {
  SymbolParser,
  type Occurrence,
  type ScipDocument,
  type ScipIndex,
} from '../lib/index.js';
import {
  formatFile,
  normalizeFormat,
  resolveExistingPath,
  type FileOccurrenceResult,
  type FileResult,
  type OutputFormat,
  type SymbolResult,
} from '../formatter.js';
import {
  buildCombinedSymbolIndex,
  buildOccurrencesBySymbol,
  buildSymbolMetadata,
  loadIndexes,
  makeDefaultMetadata,
  type IndexOptions,
} from '../symbol-data.js';

const DEFINITION_ROLE = 0x1;

type DocumentMatch = {
  document: ScipDocument;
  languageKey: string;
};

export interface FilesOptions extends IndexOptions {
  format?: string;
  symbols?: boolean;
}

export async function filesCommand(filePath: string, options: FilesOptions): Promise<void> {
  const trimmedPath = filePath.trim();
  if (!trimmedPath) {
    throw new Error('File path must not be empty.');
  }

  const format: OutputFormat = normalizeFormat(options.format);
  const indexes = loadIndexes(options);
  const combinedIndex = buildCombinedSymbolIndex(indexes);
  const occurrencesBySymbol = buildOccurrencesBySymbol(combinedIndex);
  const metadata = buildSymbolMetadata(indexes);

  const matches = findMatchingDocuments(indexes, trimmedPath);
  if (matches.length === 0) {
    throw new Error(`File not found in SCIP indexes: ${trimmedPath}`);
  }

  const content = resolveContent(trimmedPath, matches);
  const symbols = collectFileSymbols(matches, metadata, occurrencesBySymbol);
  const occurrences = options.symbols ? collectFileOccurrences(matches, metadata) : undefined;

  const result: FileResult = {
    path: trimmedPath,
    content,
    symbols,
    occurrences,
  };

  console.log(formatFile(result, format));
}

function findMatchingDocuments(indexes: Map<string, ScipIndex>, filePath: string): DocumentMatch[] {
  const matches: DocumentMatch[] = [];
  for (const [languageKey, index] of indexes) {
    for (const document of index.documents ?? []) {
      if (document.relativePath === filePath) {
        matches.push({ document, languageKey });
      }
    }
  }
  return matches;
}

function resolveContent(filePath: string, matches: DocumentMatch[]): string {
  for (const match of matches) {
    if (match.document.text && match.document.text.trim() !== '') {
      return match.document.text;
    }
  }

  const resolvedPath = resolveExistingPath(filePath);
  if (!resolvedPath) {
    return '';
  }
  try {
    return fs.readFileSync(resolvedPath, 'utf-8');
  } catch {
    return '';
  }
}

function collectFileSymbols(
  matches: DocumentMatch[],
  metadata: Map<string, SymbolResult>,
  occurrencesBySymbol: Map<string, Occurrence[]>
): SymbolResult[] {
  const symbols: SymbolResult[] = [];
  const seen = new Set<string>();

  for (const match of matches) {
    const documentPath = match.document.relativePath ?? '';
    const language = match.document.language || match.languageKey;

    for (const symbolInfo of match.document.symbols ?? []) {
      if (!symbolInfo.symbol || seen.has(symbolInfo.symbol)) {
        continue;
      }
      seen.add(symbolInfo.symbol);
      const baseSymbol = metadata.get(symbolInfo.symbol) ?? makeDefaultMetadata(symbolInfo.symbol, language, documentPath);
      const symbol = applyDefinitionFromOccurrences(baseSymbol, occurrencesBySymbol.get(symbolInfo.symbol), documentPath);
      symbols.push(symbol);
    }
  }

  symbols.sort(sortSymbolsByLine);
  return symbols;
}

function applyDefinitionFromOccurrences(
  symbol: SymbolResult,
  occurrences: Occurrence[] | undefined,
  documentPath: string
): SymbolResult {
  if (!occurrences || occurrences.length === 0) {
    return symbol;
  }
  const definition = occurrences.find(
    (occurrence) => (occurrence.roles & DEFINITION_ROLE) !== 0 && occurrence.filePath === documentPath
  ) ?? occurrences.find((occurrence) => (occurrence.roles & DEFINITION_ROLE) !== 0);

  if (!definition) {
    return symbol;
  }

  return {
    ...symbol,
    file_path: definition.filePath || symbol.file_path,
    line: definition.line,
  };
}

function collectFileOccurrences(matches: DocumentMatch[], metadata: Map<string, SymbolResult>): FileOccurrenceResult[] {
  const occurrences: FileOccurrenceResult[] = [];
  const seen = new Set<string>();
  const parser = new SymbolParser();

  for (const match of matches) {
    const language = match.document.language || match.languageKey;
    for (const occurrence of match.document.occurrences ?? []) {
      if (!occurrence.symbol || !occurrence.range || occurrence.range.length < 2) {
        continue;
      }
      const line = occurrence.range[0];
      const character = occurrence.range[1];
      const dedupeKey = `${occurrence.symbol}:${line}:${character}`;
      if (seen.has(dedupeKey)) {
        continue;
      }
      seen.add(dedupeKey);

      const symbol = metadata.get(occurrence.symbol) ?? makeDefaultMetadata(occurrence.symbol, language, '', parser);
      occurrences.push({
        line,
        character,
        symbol: symbol.name,
        kind: (occurrence.symbolRoles ?? 0) & DEFINITION_ROLE ? 'definition' : 'reference',
      });
    }
  }

  occurrences.sort(sortOccurrences);
  return occurrences;
}

function sortSymbolsByLine(left: SymbolResult, right: SymbolResult): number {
  if (left.line !== right.line) {
    return left.line - right.line;
  }
  return left.name.localeCompare(right.name, undefined, { sensitivity: 'base' });
}

function sortOccurrences(left: FileOccurrenceResult, right: FileOccurrenceResult): number {
  if (left.line !== right.line) {
    return left.line - right.line;
  }
  return left.character - right.character;
}
