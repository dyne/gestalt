import fs from 'node:fs';
import path from 'node:path';
import { encode } from '@toon-format/toon';

export type OutputFormat = 'json' | 'text' | 'toon';

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

export interface ReferenceResult {
  symbol: string;
  file_path: string;
  line: number;
  column: number;
  role: string;
}

export interface FileOccurrenceResult {
  line: number;
  character: number;
  symbol: string;
  kind: string;
}

export interface FileResult {
  path: string;
  content: string;
  symbols: SymbolResult[];
  occurrences?: FileOccurrenceResult[];
}

export function normalizeFormat(format?: string): OutputFormat {
  if (!format) {
    return 'text';
  }
  const normalized = format.toLowerCase();
  if (normalized === 'json' || normalized === 'text' || normalized === 'toon') {
    return normalized;
  }
  throw new Error(`Unsupported format "${format}". Expected one of: json, text, toon.`);
}

export function formatSymbols(query: string, symbols: SymbolResult[], format: OutputFormat): string {
  const payload = { query, symbols };
  return renderStructured(payload, format, () => formatSymbolsText(query, symbols));
}

export function formatDefinition(symbol: SymbolResult, format: OutputFormat): string {
  return renderStructured(symbol, format, () => formatDefinitionText(symbol));
}

export function formatReferences(symbolId: string, references: ReferenceResult[], format: OutputFormat): string {
  const payload = { symbol: symbolId, references };
  return renderStructured(payload, format, () => formatReferencesText(symbolId, references));
}

export function formatFile(file: FileResult, format: OutputFormat): string {
  return renderStructured(file, format, () => formatFileText(file));
}

function formatSymbolsText(query: string, symbols: SymbolResult[]): string {
  if (symbols.length === 0) {
    return `No symbols found for "${query}".`;
  }

  const lines: string[] = [];
  lines.push(`Found ${symbols.length} symbol(s) for "${query}":`);
  lines.push('');

  for (const symbol of symbols) {
    lines.push(`${symbol.file_path}:${displayLine(symbol.line)}  ${symbol.name} (${symbol.kind})`);
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

function formatDefinitionText(symbol: SymbolResult): string {
  const lines: string[] = [];
  lines.push(`Definition: ${symbol.name} (${symbol.kind})`);
  lines.push(`${symbol.file_path}:${displayLine(symbol.line)}`);
  if (symbol.signature) {
    lines.push(`  ${symbol.signature}`);
  }
  const context = readContextLine(symbol.file_path, symbol.line);
  if (context && context !== symbol.signature) {
    lines.push(`  ${context}`);
  }
  return lines.join('\n');
}

function formatReferencesText(symbolId: string, references: ReferenceResult[]): string {
  if (references.length === 0) {
    return `No references found for "${symbolId}".`;
  }

  const lines: string[] = [];
  lines.push(`Found ${references.length} reference(s) for "${symbolId}":`);
  lines.push('');

  for (const reference of references) {
    lines.push(
      `${reference.file_path}:${displayLine(reference.line)}:${displayColumn(reference.column)}  ${reference.role}`
    );
    const context = readContextLine(reference.file_path, reference.line);
    if (context) {
      lines.push(`  ${context}`);
    }
    lines.push('');
  }

  return lines.join('\n').trimEnd();
}

function formatFileText(file: FileResult): string {
  const lines: string[] = [];
  lines.push(`File: ${file.path}`);
  lines.push('');

  if (!file.content) {
    lines.push('No file content available.');
    return lines.join('\n');
  }

  const contentLines = file.content.split('\n');
  const width = String(contentLines.length).length;
  for (let index = 0; index < contentLines.length; index += 1) {
    const lineNumber = String(index + 1).padStart(width, ' ');
    lines.push(`${lineNumber} | ${contentLines[index]}`);
  }

  if (file.occurrences && file.occurrences.length > 0) {
    lines.push('');
    lines.push('Occurrences:');
    for (const occurrence of file.occurrences) {
      lines.push(
        `${displayLine(occurrence.line)}:${displayColumn(occurrence.character)} ${occurrence.symbol} (${occurrence.kind})`
      );
    }
  }

  return lines.join('\n');
}

function renderStructured(payload: unknown, format: OutputFormat, renderText: () => string): string {
  if (format === 'text') {
    return renderText();
  }
  if (format === 'json') {
    return JSON.stringify(payload, null, 2);
  }
  return encode(payload);
}

function displayLine(line: number): number {
  return line >= 0 ? line + 1 : line;
}

function displayColumn(column: number): number {
  return column >= 0 ? column + 1 : column;
}

function readContextLine(filePath: string, line: number): string | undefined {
  if (line < 0) {
    return undefined;
  }
  const resolved = resolveExistingPath(filePath);
  if (!resolved) {
    return undefined;
  }
  try {
    const content = fs.readFileSync(resolved, 'utf-8');
    const lines = content.split('\n');
    const context = lines[line]?.trim();
    return context || undefined;
  } catch {
    return undefined;
  }
}

export function resolveExistingPath(filePath: string): string | null {
  if (path.isAbsolute(filePath)) {
    return fs.existsSync(filePath) ? filePath : null;
  }
  if (fs.existsSync(filePath)) {
    return filePath;
  }
  let currentDir = process.cwd();
  for (let depth = 0; depth <= 5; depth += 1) {
    const candidate = path.join(currentDir, filePath);
    if (fs.existsSync(candidate)) {
      return candidate;
    }
    const parentDir = path.dirname(currentDir);
    if (parentDir === currentDir) {
      break;
    }
    currentDir = parentDir;
  }
  return null;
}
