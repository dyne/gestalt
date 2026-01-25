import path from 'node:path';
import {
  QueryEngine,
  SymbolParser,
  buildSymbolIndex,
  resolveDisplayName,
  type Occurrence,
  type ScipIndex,
} from '../lib/index.js';
import { scip as scipProto } from '../bundle/scip.js';
import { discoverScipFiles, findScipFileByLanguage, loadAllScipFiles } from '../scip-discovery.js';
import { formatSymbols, normalizeFormat, type OutputFormat, type SymbolResult } from '../formatter.js';

const DEFAULT_LIMIT = 20;
const MAX_LIMIT = 1000;
const DEFINITION_ROLE = 0x1;
const UNKNOWN_LINE = -1;

export interface SymbolsOptions {
  scip?: string;
  language?: string;
  limit?: number | string;
  format?: string;
}

interface DefinitionInfo {
  filePath: string;
  line: number;
  language: string;
}

interface SymbolMetadata extends SymbolResult {}

export async function symbolsCommand(query: string, options: SymbolsOptions): Promise<void> {
  const trimmedQuery = query.trim();
  if (!trimmedQuery) {
    throw new Error('Query must not be empty.');
  }

  const format: OutputFormat = normalizeFormat(options.format);
  const limit = clampLimit(options.limit);
  const indexes = loadIndexes(options);

  const combinedIndex = buildCombinedSymbolIndex(indexes);
  const metadata = buildSymbolMetadata(indexes);
  const engine = new QueryEngine(combinedIndex);
  const results = engine.find(trimmedQuery);

  const symbols = collectSymbols(results, metadata, limit);
  const output = formatSymbols(trimmedQuery, symbols, format);
  console.log(output);
}

function clampLimit(limit?: number | string): number {
  const numericLimit = typeof limit === 'string' ? Number.parseInt(limit, 10) : limit;
  if (!numericLimit || Number.isNaN(numericLimit) || numericLimit < 1) {
    return DEFAULT_LIMIT;
  }
  return Math.min(numericLimit, MAX_LIMIT);
}

function loadIndexes(options: SymbolsOptions): Map<string, ScipIndex> {
  const normalizedLanguage = options.language?.toLowerCase();

  if (options.scip) {
    const indexes = loadAllScipFiles(options.scip);
    const ensured = ensureIndexesPresent(indexes, options.scip);
    return filterIndexesByLanguage(ensured, normalizedLanguage, options.scip);
  }

  if (normalizedLanguage) {
    const scipFile = findScipFileByLanguage(normalizedLanguage);
    if (!scipFile) {
      const available = discoverScipFiles().map((file) => file.language).sort().join(', ');
      const suffix = available ? ` Available languages: ${available}.` : '';
      throw new Error(`No SCIP index found for language "${normalizedLanguage}".${suffix}`);
    }
    const indexes = loadAllScipFiles(scipFile);
    const ensured = ensureIndexesPresent(indexes, scipFile);
    return filterIndexesByLanguage(ensured, normalizedLanguage, scipFile);
  }

  const discoveredFiles = discoverScipFiles();
  if (discoveredFiles.length === 0) {
    throw new Error('No .scip files found. Run indexing or use --scip <path>.');
  }

  const scipDir = path.dirname(discoveredFiles[0].path);
  const indexes = loadAllScipFiles(scipDir);
  return ensureIndexesPresent(indexes, scipDir);
}

function filterIndexesByLanguage(
  indexes: Map<string, ScipIndex>,
  language: string | undefined,
  sourcePath: string
): Map<string, ScipIndex> {
  if (!language) {
    return indexes;
  }

  if (!indexes.has(language)) {
    const available = Array.from(indexes.keys()).sort().join(', ');
    const suffix = available ? ` Available languages: ${available}.` : '';
    throw new Error(`Language "${language}" not found in ${sourcePath}.${suffix}`);
  }

  return new Map([[language, indexes.get(language)!]]);
}

function ensureIndexesPresent(indexes: Map<string, ScipIndex>, sourcePath: string): Map<string, ScipIndex> {
  if (indexes.size === 0) {
    throw new Error(`No .scip files found in ${sourcePath}.`);
  }
  return indexes;
}

function buildCombinedSymbolIndex(indexes: Map<string, ScipIndex>): Map<string, Occurrence[]> {
  const combined = new Map<string, Occurrence[]>();
  for (const index of indexes.values()) {
    const symbolIndex = buildSymbolIndex(index);
    for (const [key, occurrences] of symbolIndex) {
      const existing = combined.get(key);
      combined.set(key, existing ? existing.concat(occurrences) : occurrences.slice());
    }
  }
  return combined;
}

function buildSymbolMetadata(indexes: Map<string, ScipIndex>): Map<string, SymbolMetadata> {
  const metadata = new Map<string, SymbolMetadata>();
  const parser = new SymbolParser();

  for (const [languageKey, index] of indexes) {
    const definitions = collectDefinitions(index, languageKey);
    collectSymbolsFromIndex(index, languageKey, parser, metadata, definitions);
    applyDefinitions(metadata, definitions);
  }

  return metadata;
}

function collectDefinitions(index: ScipIndex, fallbackLanguage: string): Map<string, DefinitionInfo> {
  const definitions = new Map<string, DefinitionInfo>();
  for (const document of index.documents ?? []) {
    const filePath = document.relativePath ?? '';
    const language = document.language || fallbackLanguage;
    for (const occurrence of document.occurrences ?? []) {
      if (!occurrence.symbol || !isDefinition(occurrence.symbolRoles)) {
        continue;
      }
      const startLine = getStartLine(occurrence.range);
      const existing = definitions.get(occurrence.symbol);
      if (!existing || existing.line === UNKNOWN_LINE) {
        definitions.set(occurrence.symbol, {
          filePath,
          line: startLine,
          language,
        });
      }
    }
  }
  return definitions;
}

function collectSymbolsFromIndex(
  index: ScipIndex,
  fallbackLanguage: string,
  parser: SymbolParser,
  metadata: Map<string, SymbolMetadata>,
  definitions: Map<string, DefinitionInfo>
): void {
  for (const document of index.documents ?? []) {
    const filePath = document.relativePath ?? '';
    const language = document.language || fallbackLanguage;
    for (const symbolInfo of document.symbols ?? []) {
      if (!symbolInfo.symbol) {
        continue;
      }
      const base = makeSymbolMetadata(symbolInfo.symbol, language, filePath, symbolInfo, parser);
      const withDefinition = mergeDefinition(base, definitions.get(symbolInfo.symbol));
      mergeMetadata(metadata, withDefinition);
    }
  }

  for (const externalSymbol of index.externalSymbols ?? []) {
    if (!externalSymbol.symbol) {
      continue;
    }
    const base = makeSymbolMetadata(externalSymbol.symbol, fallbackLanguage, '', externalSymbol, parser);
    const withDefinition = mergeDefinition(base, definitions.get(externalSymbol.symbol));
    mergeMetadata(metadata, withDefinition);
  }
}

function applyDefinitions(metadata: Map<string, SymbolMetadata>, definitions: Map<string, DefinitionInfo>): void {
  for (const [symbolId, definition] of definitions) {
    const existing = metadata.get(symbolId);
    if (!existing) {
      metadata.set(symbolId, makeDefaultMetadata(symbolId, definition.language, definition.filePath));
      continue;
    }
    metadata.set(symbolId, mergeDefinition(existing, definition));
  }
}

function makeSymbolMetadata(
  symbolId: string,
  language: string,
  filePath: string,
  symbolInfo: { documentation?: string[]; kind?: number },
  parser: SymbolParser
): SymbolMetadata {
  const parsed = parser.parse(symbolId);
  const name = resolveDisplayName(parsed.displayName, parsed.fullDescriptor, symbolId);
  const documentation = symbolInfo.documentation ?? [];
  const signature = extractSignature(documentation, name);
  const kind = kindToString(symbolInfo.kind);

  return {
    id: symbolId,
    name,
    kind,
    signature,
    documentation,
    file_path: filePath || parsed.filePath,
    line: UNKNOWN_LINE,
    language,
  };
}

function makeDefaultMetadata(symbolId: string, language: string, filePath: string): SymbolMetadata {
  const parser = new SymbolParser();
  const parsed = parser.parse(symbolId);
  const name = resolveDisplayName(parsed.displayName, parsed.fullDescriptor, symbolId);
  return {
    id: symbolId,
    name,
    kind: kindToString(undefined),
    signature: name,
    documentation: [],
    file_path: filePath || parsed.filePath,
    line: UNKNOWN_LINE,
    language,
  };
}

function mergeMetadata(metadata: Map<string, SymbolMetadata>, incoming: SymbolMetadata): void {
  const existing = metadata.get(incoming.id);
  if (!existing) {
    metadata.set(incoming.id, incoming);
    return;
  }

  const existingHasSignature = existing.signature && existing.signature !== existing.name;
  const signature = existingHasSignature ? existing.signature : incoming.signature || existing.signature;

  metadata.set(incoming.id, {
    id: incoming.id,
    name: existing.name || incoming.name,
    kind: isUnknownKind(existing.kind) ? incoming.kind : existing.kind,
    signature,
    documentation: existing.documentation.length > 0 ? existing.documentation : incoming.documentation,
    file_path: existing.file_path || incoming.file_path,
    line: existing.line !== UNKNOWN_LINE ? existing.line : incoming.line,
    language: existing.language || incoming.language,
  });
}

function mergeDefinition(symbol: SymbolMetadata, definition?: DefinitionInfo): SymbolMetadata {
  if (!definition) {
    return symbol;
  }

  return {
    ...symbol,
    file_path: symbol.file_path || definition.filePath,
    line: symbol.line === UNKNOWN_LINE ? definition.line : symbol.line,
    language: symbol.language || definition.language,
  };
}

function collectSymbols(results: Occurrence[], metadata: Map<string, SymbolMetadata>, limit: number): SymbolMetadata[] {
  const seen = new Set<string>();
  const symbols: SymbolMetadata[] = [];

  for (const result of results) {
    if (seen.has(result.symbol)) {
      continue;
    }
    seen.add(result.symbol);
    const symbolMetadata = metadata.get(result.symbol) ?? makeDefaultMetadata(result.symbol, '', result.filePath);
    symbols.push(symbolMetadata);
  }

  symbols.sort(sortSymbols);
  return symbols.slice(0, limit);
}

function sortSymbols(left: SymbolMetadata, right: SymbolMetadata): number {
  const nameCompare = left.name.localeCompare(right.name, undefined, { sensitivity: 'base' });
  if (nameCompare !== 0) {
    return nameCompare;
  }
  const fileCompare = left.file_path.localeCompare(right.file_path, undefined, { sensitivity: 'base' });
  if (fileCompare !== 0) {
    return fileCompare;
  }
  return left.line - right.line;
}

function kindToString(kind?: number): string {
  if (kind === undefined || kind === null) {
    return 'UnspecifiedKind';
  }
  return scipProto.SymbolInformation.Kind[kind] ?? 'UnspecifiedKind';
}

function isUnknownKind(kind: string): boolean {
  return !kind || kind === 'UnspecifiedKind' || kind === 'unknown';
}

function extractSignature(documentation: string[], fallback: string): string {
  for (const doc of documentation) {
    const lines = doc.split('\n');
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('```')) {
        continue;
      }
      return trimmed;
    }
  }
  return fallback;
}

function isDefinition(symbolRoles?: number): boolean {
  return (symbolRoles ?? 0) & DEFINITION_ROLE ? true : false;
}

function getStartLine(range?: number[]): number {
  if (!range || range.length === 0) {
    return UNKNOWN_LINE;
  }
  return range[0];
}
