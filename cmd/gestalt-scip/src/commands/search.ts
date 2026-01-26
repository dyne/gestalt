import { encode } from '@toon-format/toon';
import { normalizeFormat, type OutputFormat } from '../formatter.js';
import { QueryEngine } from '../lib/index.js';
import { loadIndexes, type IndexOptions } from '../symbol-data.js';

const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 1000;
const DEFAULT_CONTEXT_LINES = 2;
const MAX_CONTEXT_LINES = 10;

export interface SearchOptions extends IndexOptions {
  limit?: number | string;
  format?: string;
  caseSensitive?: boolean;
  context?: number | string;
}

export interface SearchMatch {
  file_path: string;
  line: number;
  column: number;
  match_text: string;
  context_before: string[];
  context_after: string[];
  language?: string;
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
    indexes: Array.from(indexes.values()),
  });

  const matches = results.slice(0, limit);
  console.log(formatSearchResults(trimmedPattern, matches, format));
}

function formatSearchResults(pattern: string, results: SearchMatch[], format: OutputFormat): string {
  if (format === 'json') {
    if (results.length === 0) {
      return JSON.stringify({ pattern, matches: [] }, null, 2);
    }
    return JSON.stringify({ pattern, matches: results }, null, 2);
  }

  if (format === 'text') {
    if (results.length === 0) {
      return `No matches found for pattern: ${pattern}`;
    }
    return formatSearchText(pattern, results);
  }

  if (results.length === 0) {
    return encode({ pattern, matches: [] });
  }
  return formatSearchToon(pattern, results);
}

function formatSearchText(pattern: string, results: SearchMatch[]): string {
  const lines: string[] = [`pattern: ${pattern}`, `matches: ${results.length}`, ''];

  for (const result of results) {
    lines.push(`${result.file_path}:${result.line}:${result.column}`);
    if (result.language) {
      lines.push(`  language: ${result.language}`);
    }

    lines.push('  context:');
    if (result.context_before.length > 0) {
      const startLine = result.line - result.context_before.length;
      for (let index = 0; index < result.context_before.length; index += 1) {
        lines.push(`    ${startLine + index}: ${result.context_before[index]}`);
      }
    }

    lines.push(`    ${result.line}: ${result.match_text}`);

    if (result.context_after.length > 0) {
      for (let index = 0; index < result.context_after.length; index += 1) {
        lines.push(`    ${result.line + 1 + index}: ${result.context_after[index]}`);
      }
    }

    lines.push('');
  }

  return lines.join('\n').trimEnd();
}

function formatSearchToon(pattern: string, results: SearchMatch[]): string {
  const payload = {
    pattern,
    matches: results.map((result) => ({
      file: result.file_path,
      location: `${result.line}:${result.column}`,
      match: result.match_text,
      language: result.language,
      context: {
        before: result.context_before,
        after: result.context_after,
      },
    })),
  };

  return encode(payload);
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
