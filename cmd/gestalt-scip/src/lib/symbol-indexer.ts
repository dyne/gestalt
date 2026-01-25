import type { ScipIndex, ScipOccurrence } from './scip-loader.js';
import { SymbolIndexKey } from './scip/SymbolIndexKey.js';
import { SymbolParser } from './scip/SymbolParser.js';
import { SuffixType } from './scip/SuffixType.js';

export interface Occurrence {
  symbol: string;
  filePath: string;
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
  roles: number;
  suffix?: SuffixType;
}

interface ParsedRange {
  startLine: number;
  startChar: number;
  endLine: number;
  endChar: number;
}

interface VariantBuckets {
  ts: Occurrence[];
  dts: Occurrence[];
}

function stripSuffix(name: string): string {
  if (name.endsWith('().')) {
    return name.slice(0, -3);
  }
  const lastChar = name[name.length - 1];
  if (lastChar === '#' || lastChar === '.' || lastChar === '/') {
    return name.slice(0, -1);
  }
  return name;
}

function resolveDisplayName(
  parsedDisplayName: string,
  fullDescriptor: string,
  symbol: string
): string {
  if (parsedDisplayName) {
    return parsedDisplayName;
  }

  const descriptorSource = fullDescriptor || symbol.slice(symbol.lastIndexOf(' ') + 1);
  const descriptor = descriptorSource.slice(descriptorSource.lastIndexOf('/') + 1);
  if (!descriptor) {
    return symbol;
  }

  const lastHashIndex = descriptor.lastIndexOf('#');
  let leaf = descriptor;
  if (lastHashIndex >= 0 && lastHashIndex < descriptor.length - 1) {
    leaf = descriptor.slice(lastHashIndex + 1);
  }
  return stripSuffix(leaf);
}

function parseRange(range: number[]): ParsedRange | null {
  if (range.length < 3) {
    return null;
  }

  const [startLine, startChar, ...rest] = range;
  const endLine = range.length === 3 ? startLine : rest[0];
  const endChar = range.length === 3 ? rest[0] : rest[1];

  return { startLine, startChar, endLine, endChar };
}

function toIndexedOccurrence(
  occurrence: ScipOccurrence,
  documentPath: string,
  range: ParsedRange,
  suffix?: SuffixType
): Occurrence {
  return {
    symbol: occurrence.symbol || '',
    filePath: documentPath,
    line: range.startLine,
    column: range.startChar,
    endLine: range.endLine,
    endColumn: range.endChar,
    roles: occurrence.symbolRoles || 0,
    suffix,
  };
}

function getOrCreateBuckets(map: Map<string, VariantBuckets>, key: string): VariantBuckets {
  if (!map.has(key)) {
    map.set(key, { ts: [], dts: [] });
  }
  return map.get(key)!;
}

function isDeclarationFile(filePath: string): boolean {
  return filePath.endsWith('.d.ts');
}

function normalizeSymbolPath(filePath: string): string {
  if (isDeclarationFile(filePath)) {
    return filePath.replace(/\.d\.ts$/, '.ts');
  }
  return filePath;
}

function parseOccurrenceRange(occurrence: ScipOccurrence): ParsedRange | null {
  const range = occurrence.range;
  if (!range || range.length < 3) {
    return null;
  }
  return parseRange(range);
}

function addToBucket(buckets: VariantBuckets, occurrence: Occurrence, documentPath: string): void {
  if (isDeclarationFile(documentPath)) {
    buckets.dts.push(occurrence);
  } else {
    buckets.ts.push(occurrence);
  }
}

function processOccurrence(
  variantMap: Map<string, VariantBuckets>,
  occurrence: ScipOccurrence,
  documentPath: string
): void {
  const symbol = occurrence.symbol;
  if (!symbol) {
    return;
  }

  const parsedRange = parseOccurrenceRange(occurrence);
  if (!parsedRange) {
    return;
  }

  const parser = new SymbolParser();
  const parsed = parser.parse(symbol);
  const occurrenceRecord = toIndexedOccurrence(occurrence, documentPath, parsedRange, parsed.suffix);
  const resolvedPath = parsed.filePath || documentPath;
  const normalizedPath = normalizeSymbolPath(resolvedPath);
  const displayName = resolveDisplayName(parsed.displayName, parsed.fullDescriptor, symbol);

  const indexKey = new SymbolIndexKey(
    parsed.packageName,
    normalizedPath,
    displayName,
    parsed.suffix,
    parsed.fullDescriptor
  );

  const buckets = getOrCreateBuckets(variantMap, indexKey.toString());
  addToBucket(buckets, occurrenceRecord, documentPath);
}

function processDocument(
  variantMap: Map<string, VariantBuckets>,
  documentPath: string,
  occurrences: ScipOccurrence[]
): void {
  for (const occurrence of occurrences) {
    processOccurrence(variantMap, occurrence, documentPath);
  }
}

function processDocumentFromIndex(
  variantMap: Map<string, VariantBuckets>,
  doc: { relativePath?: string; occurrences?: ScipOccurrence[] }
): void {
  const documentPath = doc.relativePath || '';
  const occurrences = doc.occurrences || [];
  processDocument(variantMap, documentPath, occurrences);
}

function buildVariantMap(scipIndex: ScipIndex): Map<string, VariantBuckets> {
  const variantMap = new Map<string, VariantBuckets>();
  const documents = scipIndex.documents || [];

  for (const document of documents) {
    processDocumentFromIndex(variantMap, document);
  }

  return variantMap;
}

export function buildSymbolIndex(scipIndex: ScipIndex): Map<string, Occurrence[]> {
  const index = new Map<string, Occurrence[]>();
  const variantMap = buildVariantMap(scipIndex);

  for (const [key, { ts, dts }] of variantMap) {
    const merged = mergeSymbolVariants(ts, dts);
    index.set(key, merged);
  }

  return index;
}

interface DedupeState {
  seen: Set<string>;
  merged: Occurrence[];
}

function makeDedupeKey(occurrence: Occurrence): string {
  return `${occurrence.filePath}:${occurrence.line}:${occurrence.column}`;
}

function addUniqueOccurrence(state: DedupeState, occurrence: Occurrence): void {
  const dedupeKey = makeDedupeKey(occurrence);
  if (!state.seen.has(dedupeKey)) {
    state.seen.add(dedupeKey);
    state.merged.push(occurrence);
  }
}

function addUniqueOccurrences(state: DedupeState, occurrences: Occurrence[]): void {
  for (const occurrence of occurrences) {
    addUniqueOccurrence(state, occurrence);
  }
}

export function mergeSymbolVariants(tsOccurrences: Occurrence[], dtsOccurrences: Occurrence[]): Occurrence[] {
  const state: DedupeState = { seen: new Set<string>(), merged: [] };
  addUniqueOccurrences(state, tsOccurrences);
  addUniqueOccurrences(state, dtsOccurrences);
  return state.merged;
}
