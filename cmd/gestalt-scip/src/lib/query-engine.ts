import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { ScipDocument, ScipIndex } from './scip-loader.js';
import type { Occurrence } from './symbol-indexer.js';
import { SuffixType } from './scip/SuffixType.js';

export interface QueryOptions {
  from?: string;
  folder?: string;
  suffixFilter?: SuffixType;
}

export interface QueryResult extends Occurrence {
  isDefinition: boolean;
}

export interface ContentSearchOptions {
  caseSensitive?: boolean;
  contextLines?: number;
  language?: string;
  limit?: number;
  indexes: Array<{ languageKey: string; index: ScipIndex }>;
}

export interface ContentSearchResult {
  file_path: string;
  line: number;
  column: number;
  match_text: string;
  context_before: string[];
  context_after: string[];
  language?: string;
}

const DEFINITION_ROLE = 0x1;
const MAX_PARENT_SEARCH = 5;

export class QueryEngine {
  constructor(private readonly symbolIndex: Map<string, Occurrence[]>) {}

  find(symbolName: string, options?: QueryOptions): QueryResult[] {
    if (!symbolName) {
      return [];
    }

    const folder = normalizeFolderPath(options?.folder);
    const results: QueryResult[] = [];
    const isQualifiedName = symbolName.includes('.') || symbolName.includes('#');

    for (const [key, occurrences] of this.symbolIndex) {
      const parsedKey = parseSymbolKey(key);
      if (!parsedKey) {
        continue;
      }

      const isMatch = isQualifiedName
        ? this.matchesQualifiedName(symbolName, occurrences)
        : parsedKey.name === symbolName;

      if (!isMatch) {
        continue;
      }

      if (!this.matchesFromFilter(occurrences, options?.from)) {
        continue;
      }

      let matchingOccurrences = occurrences;
      matchingOccurrences = filterByFolder(matchingOccurrences, folder);
      matchingOccurrences = filterBySuffix(matchingOccurrences, options?.suffixFilter);
      results.push(...matchingOccurrences.map(toQueryResult));
    }

    return results;
  }

  searchContent(pattern: string, options: ContentSearchOptions): ContentSearchResult[] {
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

    const results: ContentSearchResult[] = [];
    const contextLines = options.contextLines ?? 3;
    const limit = normalizeLimit(options.limit);
    const normalizedLanguage = options.language?.toLowerCase();
    const contentCache = new Map<string, string | null>();
    const pathCache = new Map<string, string | null>();

    for (const entry of options.indexes) {
      const index = entry.index;
      const fallbackLanguage = entry.languageKey;
      if (results.length >= limit) {
        return results;
      }
      const projectRoot = normalizeProjectRoot(index.metadata?.projectRoot);
      const documents = index.documents ?? [];
      for (const document of documents) {
        if (results.length >= limit) {
          return results;
        }
        const documentLanguage = document.language ?? fallbackLanguage;
        if (normalizedLanguage && documentLanguage?.toLowerCase() !== normalizedLanguage) {
          continue;
        }

        if (!document.relativePath) {
          continue;
        }

        const documentText = resolveDocumentText(document, projectRoot, contentCache, pathCache);
        if (!documentText) {
          continue;
        }

        const lines = documentText.split('\n');
        for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
          if (results.length >= limit) {
            return results;
          }
          const lineText = lines[lineIndex];
          regex.lastIndex = 0;
          const matches = Array.from(lineText.matchAll(regex));
          if (matches.length === 0) {
            continue;
          }

          const contextBefore = lines.slice(Math.max(0, lineIndex - contextLines), lineIndex);
          const contextAfter = lines.slice(lineIndex + 1, Math.min(lines.length, lineIndex + 1 + contextLines));

          for (const match of matches) {
            const column = match.index ?? 0;

            results.push({
              file_path: document.relativePath,
              line: lineIndex + 1,
              column: column + 1,
              match_text: lineText,
              context_before: contextBefore,
              context_after: contextAfter,
              language: documentLanguage,
            });
            if (results.length >= limit) {
              return results;
            }
          }
        }
      }
    }

    return results;
  }

  private matchesQualifiedName(qualifiedName: string, occurrences: Occurrence[]): boolean {
    const scipPattern = this.convertToScipPattern(qualifiedName);
    return occurrences.some((occurrence) => {
      const descriptor = this.extractDescriptorFromSymbol(occurrence.symbol);
      return descriptor === scipPattern || descriptor.endsWith(`/${scipPattern}`);
    });
  }

  private convertToScipPattern(query: string): string {
    if (query.includes('#')) {
      if (query.endsWith('()')) {
        return query.replace('()', '().');
      }
      if (!query.endsWith('.') && !query.endsWith('#') && !query.endsWith('/')) {
        return `${query}.`;
      }
      return query;
    }

    const parts = query.split('.');
    if (parts.length < 2) {
      return query;
    }

    const lastPart = parts[parts.length - 1];
    const isMethod = lastPart.endsWith('()');
    const baseLastPart = isMethod ? lastPart.slice(0, -2) : lastPart;
    const result = parts.slice(0, -1).join('#') + '#' + baseLastPart;
    return isMethod ? `${result}().` : `${result}.`;
  }

  private extractDescriptorFromSymbol(symbol: string): string {
    const lastSpaceIndex = symbol.lastIndexOf(' ');
    if (lastSpaceIndex === -1) {
      return symbol;
    }

    const descriptorPart = symbol.slice(lastSpaceIndex + 1);
    const lastSlashInPath = descriptorPart.lastIndexOf('/');
    if (lastSlashInPath === -1) {
      return descriptorPart;
    }

    return descriptorPart.slice(lastSlashInPath + 1);
  }

  private matchesFromFilter(occurrences: Occurrence[], from?: string): boolean {
    if (!from) {
      return true;
    }

    return occurrences.some(
      (occurrence) => (occurrence.roles & DEFINITION_ROLE) !== 0 && occurrence.filePath === from
    );
  }
}

function resolveDocumentText(
  document: ScipDocument,
  projectRoot: string | undefined,
  contentCache: Map<string, string | null>,
  pathCache: Map<string, string | null>
): string | null {
  const text = document.text?.trim();
  if (!document.relativePath) {
    return null;
  }

  const cacheKey = buildCacheKey(projectRoot, document.relativePath);
  if (contentCache.has(cacheKey)) {
    return contentCache.get(cacheKey) ?? null;
  }

  if (text) {
    contentCache.set(cacheKey, document.text ?? null);
    return document.text ?? null;
  }

  const resolvedPath = resolveExistingPath(document.relativePath, projectRoot, cacheKey, pathCache);
  if (!resolvedPath) {
    contentCache.set(cacheKey, null);
    return null;
  }

  try {
    const content = fs.readFileSync(resolvedPath, 'utf-8');
    contentCache.set(cacheKey, content);
    return content;
  } catch {
    contentCache.set(cacheKey, null);
    return null;
  }
}

function normalizeProjectRoot(projectRoot?: string): string | undefined {
  if (!projectRoot) {
    return undefined;
  }
  if (projectRoot.startsWith('file://')) {
    try {
      return fileURLToPath(projectRoot);
    } catch {
      return undefined;
    }
  }
  return projectRoot;
}

function resolveExistingPath(
  filePath: string,
  projectRoot: string | undefined,
  cacheKey: string,
  pathCache: Map<string, string | null>
): string | null {
  if (pathCache.has(cacheKey)) {
    return pathCache.get(cacheKey) ?? null;
  }

  if (path.isAbsolute(filePath)) {
    const resolved = fs.existsSync(filePath) ? filePath : null;
    pathCache.set(cacheKey, resolved);
    return resolved;
  }

  if (projectRoot) {
    const projectCandidate = path.join(projectRoot, filePath);
    if (fs.existsSync(projectCandidate)) {
      pathCache.set(cacheKey, projectCandidate);
      return projectCandidate;
    }
  }

  if (fs.existsSync(filePath)) {
    pathCache.set(cacheKey, filePath);
    return filePath;
  }

  let currentDir = process.cwd();
  for (let depth = 0; depth <= MAX_PARENT_SEARCH; depth += 1) {
    const candidate = path.join(currentDir, filePath);
    if (fs.existsSync(candidate)) {
      pathCache.set(cacheKey, candidate);
      return candidate;
    }
    const parentDir = path.dirname(currentDir);
    if (parentDir === currentDir) {
      break;
    }
    currentDir = parentDir;
  }

  pathCache.set(cacheKey, null);
  return null;
}

function buildCacheKey(projectRoot: string | undefined, relativePath: string): string {
  return `${projectRoot ?? ''}::${relativePath}`;
}

function normalizeLimit(limit?: number): number {
  if (limit === undefined || limit === null || Number.isNaN(limit) || limit < 1) {
    return Number.POSITIVE_INFINITY;
  }
  return limit;
}

function normalizeFolderPath(folder?: string): string | undefined {
  if (!folder) {
    return undefined;
  }
  return folder.endsWith('/') ? folder : `${folder}/`;
}

function parseSymbolKey(key: string): { definingFile: string; name: string } | null {
  const parts = key.split(':');
  if (parts.length !== 3) {
    return null;
  }

  const [, definingFile, name] = parts;
  return { definingFile, name };
}

function filterByFolder(occurrences: Occurrence[], folder?: string): Occurrence[] {
  if (!folder) {
    return occurrences;
  }
  return occurrences.filter((occurrence) => occurrence.filePath.startsWith(folder));
}

function filterBySuffix(occurrences: Occurrence[], suffixFilter?: SuffixType): Occurrence[] {
  if (!suffixFilter) {
    return occurrences;
  }
  return occurrences.filter((occurrence) => occurrence.suffix === suffixFilter);
}

function toQueryResult(occurrence: Occurrence): QueryResult {
  return {
    symbol: occurrence.symbol,
    filePath: occurrence.filePath,
    line: occurrence.line,
    column: occurrence.column,
    endLine: occurrence.endLine,
    endColumn: occurrence.endColumn,
    roles: occurrence.roles,
    suffix: occurrence.suffix,
    isDefinition: (occurrence.roles & DEFINITION_ROLE) !== 0,
  };
}
