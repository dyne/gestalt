import { type Occurrence } from '../lib/index.js';
import { formatDefinition, normalizeFormat, type OutputFormat, type SymbolResult } from '../formatter.js';
import {
  buildCombinedSymbolIndex,
  buildOccurrencesBySymbol,
  buildSymbolMetadata,
  loadIndexes,
  makeDefaultMetadata,
  type IndexOptions,
} from '../symbol-data.js';

const DEFINITION_ROLE = 0x1;

export interface DefinitionOptions extends IndexOptions {
  format?: string;
}

export async function definitionCommand(symbolId: string, options: DefinitionOptions): Promise<void> {
  const trimmedSymbolId = symbolId.trim();
  if (!trimmedSymbolId) {
    throw new Error('Symbol id must not be empty.');
  }

  const format: OutputFormat = normalizeFormat(options.format ?? 'json');
  const indexes = loadIndexes(options);
  const combinedIndex = buildCombinedSymbolIndex(indexes);
  const occurrencesBySymbol = buildOccurrencesBySymbol(combinedIndex);
  const occurrences = occurrencesBySymbol.get(trimmedSymbolId);

  if (!occurrences || occurrences.length === 0) {
    throw new Error(`Symbol not found: ${trimmedSymbolId}`);
  }

  const metadata = buildSymbolMetadata(indexes);
  const definitionOccurrence = selectDefinitionOccurrence(occurrences);
  const baseSymbol = metadata.get(trimmedSymbolId) ?? makeDefaultMetadata(trimmedSymbolId, 'unknown', definitionOccurrence.filePath);
  const symbol = applyDefinitionOccurrence(baseSymbol, definitionOccurrence);

  console.log(formatDefinition(symbol, format));
}

function selectDefinitionOccurrence(occurrences: Occurrence[]): Occurrence {
  return occurrences.find((occurrence) => (occurrence.roles & DEFINITION_ROLE) !== 0) ?? occurrences[0];
}

function applyDefinitionOccurrence(symbol: SymbolResult, occurrence: Occurrence): SymbolResult {
  return {
    ...symbol,
    file_path: occurrence.filePath || symbol.file_path,
    line: occurrence.line,
  };
}
