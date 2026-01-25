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

const DEFINITION_ROLE = 0x1;

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
