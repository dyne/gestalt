import { type Occurrence } from '../lib/index.js';
import { formatReferences, normalizeFormat, type OutputFormat, type ReferenceResult } from '../formatter.js';
import { decodeSymbolId } from '../symbol-id-codec.js';
import {
  buildCombinedSymbolIndex,
  buildOccurrencesBySymbol,
  loadIndexes,
  type IndexOptions,
} from '../symbol-data.js';

const DEFINITION_ROLE = 0x1;

export interface ReferencesOptions extends IndexOptions {
  format?: string;
}

export async function referencesCommand(symbolId: string, options: ReferencesOptions): Promise<void> {
  const trimmedSymbolId = symbolId.trim();
  if (!trimmedSymbolId) {
    throw new Error('Symbol id must not be empty.');
  }

  const resolvedSymbolId = decodeSymbolId(trimmedSymbolId);
  const format: OutputFormat = normalizeFormat(options.format ?? 'json');
  const indexes = loadIndexes(options);
  const combinedIndex = buildCombinedSymbolIndex(indexes);
  const occurrencesBySymbol = buildOccurrencesBySymbol(combinedIndex);
  const occurrences = occurrencesBySymbol.get(resolvedSymbolId);

  if (!occurrences || occurrences.length === 0) {
    throw new Error(`Symbol not found: ${trimmedSymbolId}`);
  }

  const references = occurrences
    .filter((occurrence) => (occurrence.roles & DEFINITION_ROLE) === 0)
    .sort(sortOccurrences)
    .map((occurrence) => toReferenceResult(resolvedSymbolId, occurrence));

  console.log(formatReferences(resolvedSymbolId, references, format));
}

function toReferenceResult(symbolId: string, occurrence: Occurrence): ReferenceResult {
  return {
    symbol: symbolId,
    file_path: occurrence.filePath,
    line: occurrence.line,
    column: occurrence.column,
    role: 'reference',
  };
}

function sortOccurrences(left: Occurrence, right: Occurrence): number {
  const fileCompare = left.filePath.localeCompare(right.filePath, undefined, { sensitivity: 'base' });
  if (fileCompare !== 0) {
    return fileCompare;
  }
  if (left.line !== right.line) {
    return left.line - right.line;
  }
  return left.column - right.column;
}
