import * as fs from 'fs';
import * as path from 'path';
import type { ScipDocument, ScipIndex, ScipOccurrence, ScipRelationship, ScipSymbolInformation } from './scip-types.js';
import { parseJsonIndex } from './scip-json-normalizer.js';
import { scip } from '../bundle/scip.js';

export type {
  ScipIndex,
  ScipDocument,
  ScipOccurrence,
  ScipSymbolInformation,
  ScipRelationship,
  ScipDiagnostic,
} from './scip-types.js';

const MAX_PARENT_SEARCH = 10;

function normalizeRelationship(relationship: any): ScipRelationship {
  return {
    symbol: relationship.symbol,
    isReference: relationship.is_reference ?? relationship.isReference,
    isImplementation: relationship.is_implementation ?? relationship.isImplementation,
    isTypeDefinition: relationship.is_type_definition ?? relationship.isTypeDefinition,
    isDefinition: relationship.is_definition ?? relationship.isDefinition,
  };
}

function normalizeSymbolInformation(symbol: any): ScipSymbolInformation {
  const relationships = symbol.relationships ?? symbol.relationshipsList ?? [];
  return {
    symbol: symbol.symbol,
    documentation: symbol.documentation,
    relationships: relationships.map(normalizeRelationship),
    kind: symbol.kind,
    displayName: symbol.display_name ?? symbol.displayName,
    signatureDocumentation: symbol.signature_documentation ?? symbol.signatureDocumentation,
    enclosingSymbol: symbol.enclosing_symbol ?? symbol.enclosingSymbol,
  };
}

function normalizeOccurrence(occurrence: any): ScipOccurrence {
  return {
    range: occurrence.range,
    symbol: occurrence.symbol,
    symbolRoles: occurrence.symbol_roles ?? occurrence.symbolRoles,
    overrideDocumentation: occurrence.override_documentation ?? occurrence.overrideDocumentation,
    syntaxKind: occurrence.syntax_kind ?? occurrence.syntaxKind,
    diagnostics: occurrence.diagnostics,
    enclosingRange: occurrence.enclosing_range ?? occurrence.enclosingRange,
  };
}

function normalizeDocument(document: any): ScipDocument {
  const occurrences = document.occurrences ?? document.occurrencesList ?? [];
  const symbols = document.symbols ?? document.symbolsList ?? [];
  return {
    relativePath: document.relative_path ?? document.relativePath,
    language: document.language,
    occurrences: occurrences.map(normalizeOccurrence),
    symbols: symbols.map(normalizeSymbolInformation),
    text: document.text,
    positionEncoding: document.position_encoding ?? document.positionEncoding,
  };
}

function normalizeMetadata(metadata: any): ScipIndex['metadata'] {
  if (!metadata) {
    return undefined;
  }
  return {
    version: metadata.version,
    toolInfo: metadata.tool_info ?? metadata.toolInfo,
    projectRoot: metadata.project_root ?? metadata.projectRoot,
    textDocumentEncoding: metadata.text_document_encoding ?? metadata.textDocumentEncoding,
  };
}

function normalizeScipIndex(index: any): ScipIndex {
  const documents = index.documents ?? index.documentsList ?? [];
  const externalSymbols = index.external_symbols ?? index.externalSymbols ?? [];
  return {
    metadata: normalizeMetadata(index.metadata),
    documents: documents.map(normalizeDocument),
    externalSymbols: externalSymbols.map(normalizeSymbolInformation),
  };
}

function parseProtobufIndex(buffer: Buffer): ScipIndex {
  const index = scip.Index.deserializeBinary(buffer);
  const rawObject = index.toObject();
  return normalizeScipIndex(rawObject);
}

export function findScipFile(scipPath?: string): string | null {
  if (scipPath) {
    return fs.existsSync(scipPath) ? scipPath : null;
  }

  let currentDir = process.cwd();
  for (let index = 0; index < MAX_PARENT_SEARCH; index += 1) {
    const scipFilePath = path.join(currentDir, 'index.scip');
    if (fs.existsSync(scipFilePath)) {
      return scipFilePath;
    }
    const parentDir = path.dirname(currentDir);
    if (parentDir === currentDir) {
      break;
    }
    currentDir = parentDir;
  }

  return null;
}

export function loadScipIndex(scipPath: string): ScipIndex {
  if (!fs.existsSync(scipPath)) {
    throw new Error(`SCIP file not found: ${scipPath}`);
  }

  const buffer = fs.readFileSync(scipPath);
  if (buffer.length === 0) {
    return { documents: [] };
  }

  const content = buffer.toString('utf-8');
  const jsonResult = parseJsonIndex(content);
  if (jsonResult) {
    return jsonResult;
  }

  try {
    return parseProtobufIndex(buffer);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown error';
    throw new Error(`Failed to parse SCIP file: ${message}`);
  }
}
