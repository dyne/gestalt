import { QueryEngine, type Occurrence } from '../lib/index.js';
import { formatSymbols, normalizeFormat, type OutputFormat } from '../formatter.js';
import {
  buildCombinedSymbolIndex,
  buildSymbolMetadata,
  loadIndexes,
  makeDefaultMetadata,
  type IndexOptions,
} from '../symbol-data.js';

const DEFAULT_LIMIT = 20;
const MAX_LIMIT = 1000;

export interface SymbolsOptions extends IndexOptions {
  limit?: number | string;
  format?: string;
}

type SymbolMetadata = ReturnType<typeof makeDefaultMetadata>;

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
  console.log(formatSymbols(trimmedQuery, symbols, format));
}

function clampLimit(limit?: number | string): number {
  const numericLimit = typeof limit === 'string' ? Number.parseInt(limit, 10) : limit;
  if (!numericLimit || Number.isNaN(numericLimit) || numericLimit < 1) {
    return DEFAULT_LIMIT;
  }
  return Math.min(numericLimit, MAX_LIMIT);
}

function collectSymbols(
  results: Occurrence[],
  metadata: Map<string, SymbolMetadata>,
  limit: number
): SymbolMetadata[] {
  const seen = new Set<string>();
  const symbols: SymbolMetadata[] = [];

  for (const result of results) {
    if (seen.has(result.symbol)) {
      continue;
    }
    seen.add(result.symbol);
    const symbolMetadata = metadata.get(result.symbol) ?? makeDefaultMetadata(result.symbol, 'unknown', result.filePath);
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
