export type OutputFormat = 'json' | 'text';

export interface SymbolResult {
  id: string;
  name: string;
  kind: string;
  signature: string;
  documentation: string[];
  file_path: string;
  line: number;
  language: string;
}

export function normalizeFormat(format?: string): OutputFormat {
  return format === 'json' ? 'json' : 'text';
}

export function formatSymbols(query: string, symbols: SymbolResult[], format: OutputFormat): string {
  return format === 'json' ? formatSymbolsJson(query, symbols) : formatSymbolsText(query, symbols);
}

function formatSymbolsJson(query: string, symbols: SymbolResult[]): string {
  return JSON.stringify({ query, symbols }, null, 2);
}

function formatSymbolsText(query: string, symbols: SymbolResult[]): string {
  if (symbols.length === 0) {
    return `No symbols found for "${query}".`;
  }

  const lines: string[] = [];
  lines.push(`Found ${symbols.length} symbol(s) for "${query}":`);
  lines.push('');

  for (const symbol of symbols) {
    const displayLine = symbol.line >= 0 ? symbol.line + 1 : symbol.line;
    lines.push(`${symbol.file_path}:${displayLine}  ${symbol.name} (${symbol.kind})`);
    if (symbol.signature) {
      lines.push(`  ${symbol.signature}`);
    }
    const docLine = symbol.documentation.find((line) => line.trim() !== '' && !line.trim().startsWith('```'));
    if (docLine && docLine !== symbol.signature) {
      lines.push(`  ${docLine.trim()}`);
    }
    lines.push('');
  }

  return lines.join('\n').trimEnd();
}
