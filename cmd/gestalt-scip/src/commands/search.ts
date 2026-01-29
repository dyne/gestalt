import { formatSearchResults, normalizeFormat, type OutputFormat } from '../formatter.js';
import { QueryEngine } from '../lib/index.js';
import { loadIndexes, type IndexOptions } from '../symbol-data.js';

const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 1000;
const DEFAULT_CONTEXT_LINES = 3;
const MAX_CONTEXT_LINES = 30;

export interface SearchOptions extends IndexOptions {
  limit?: number | string;
  format?: string;
  caseSensitive?: boolean;
  context?: number | string;
  path?: string;
}

export async function searchCommand(pattern: string, options: SearchOptions): Promise<void> {
  const trimmedPattern = pattern.trim();
  if (!trimmedPattern) {
    throw new Error('Search pattern must not be empty.');
  }

  const format: OutputFormat = normalizeFormat(options.format);
  const limit = clampLimit(options.limit);
  const contextLines = clampContext(options.context);
  const caseSensitive = options.caseSensitive ?? false;

  const indexes = loadIndexes(options);
  const engine = new QueryEngine(new Map());
  const results = engine.searchContent(trimmedPattern, {
    caseSensitive,
    contextLines,
    language: options.language,
    limit,
    path: options.path,
    indexes: Array.from(indexes.entries()).map(([languageKey, index]) => ({
      languageKey,
      index,
    })),
  });

  const matches = results.slice(0, limit);
  console.log(formatSearchResults(trimmedPattern, matches, format));
}

function clampLimit(limit?: number | string): number {
  const numericLimit = typeof limit === 'string' ? Number.parseInt(limit, 10) : limit;
  if (!numericLimit || Number.isNaN(numericLimit) || numericLimit < 1) {
    return DEFAULT_LIMIT;
  }
  return Math.min(numericLimit, MAX_LIMIT);
}

function clampContext(context?: number | string): number {
  const numericContext = typeof context === 'string' ? Number.parseInt(context, 10) : context;
  if (numericContext === undefined || numericContext === null || Number.isNaN(numericContext) || numericContext < 0) {
    return DEFAULT_CONTEXT_LINES;
  }
  return Math.min(numericContext, MAX_CONTEXT_LINES);
}
