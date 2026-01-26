import { encode } from '@toon-format/toon';
import { normalizeFormat, type OutputFormat } from '../formatter.js';
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

interface ContentSearchOptions {
  caseSensitive: boolean;
  contextLines: number;
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
  const results = searchContent(trimmedPattern, indexes, {
    caseSensitive,
    contextLines,
    language: options.language,
  });

  const matches = results.slice(0, limit);
  console.log(formatSearchResults(trimmedPattern, matches, format));
}

function searchContent(
  pattern: string,
  indexes: Map<string, { documents?: Array<{ relativePath?: string; language?: string; text?: string }> }>,
  options: ContentSearchOptions
): SearchMatch[] {
  if (!pattern) {
    return [];
  }

  const flags = options.caseSensitive ? 'g' : 'gi';
  let regex: RegExp;
  try {
    regex = new RegExp(pattern, flags);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`Invalid regex pattern: ${message}`);
  }

  const results: SearchMatch[] = [];
  const normalizedLanguage = options.language?.toLowerCase();

  for (const [fallbackLanguage, index] of indexes) {
    const documents = index.documents ?? [];
    for (const document of documents) {
      const documentLanguage = document.language ?? fallbackLanguage;
      if (normalizedLanguage && documentLanguage?.toLowerCase() !== normalizedLanguage) {
        continue;
      }

      if (!document.text || !document.relativePath) {
        continue;
      }

      const lines = document.text.split('\n');
      for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
        const lineText = lines[lineIndex];
        regex.lastIndex = 0;
        const matches = Array.from(lineText.matchAll(regex));
        if (matches.length === 0) {
          continue;
        }

        for (const match of matches) {
          const column = match.index ?? 0;
          const contextBefore = lines.slice(Math.max(0, lineIndex - options.contextLines), lineIndex);
          const contextAfter = lines.slice(
            lineIndex + 1,
            Math.min(lines.length, lineIndex + 1 + options.contextLines)
          );

          results.push({
            file_path: document.relativePath,
            line: lineIndex + 1,
            column: column + 1,
            match_text: lineText,
            context_before: contextBefore,
            context_after: contextAfter,
            language: documentLanguage,
          });
        }
      }
    }
  }

  return results;
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
