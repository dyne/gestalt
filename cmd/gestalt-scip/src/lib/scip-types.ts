export interface ScipIndex {
  metadata?: {
    version?: { version?: number };
    toolInfo?: { name?: string; version?: string };
    projectRoot?: string;
    textDocumentEncoding?: number;
  };
  documents?: ScipDocument[];
  externalSymbols?: ScipSymbolInformation[];
}

export interface ScipDocument {
  relativePath?: string;
  language?: string;
  occurrences?: ScipOccurrence[];
  symbols?: ScipSymbolInformation[];
  text?: string;
  positionEncoding?: number;
}

export interface ScipOccurrence {
  range?: number[];
  symbol?: string;
  symbolRoles?: number;
  overrideDocumentation?: string[];
  syntaxKind?: number;
  diagnostics?: ScipDiagnostic[];
  enclosingRange?: number[];
}

export interface ScipSymbolInformation {
  symbol?: string;
  documentation?: string[];
  relationships?: ScipRelationship[];
  kind?: number;
  displayName?: string;
  signatureDocumentation?: ScipDocument;
  enclosingSymbol?: string;
}

export interface ScipRelationship {
  symbol?: string;
  isReference?: boolean;
  isImplementation?: boolean;
  isTypeDefinition?: boolean;
  isDefinition?: boolean;
}

export interface ScipDiagnostic {
  severity?: number;
  code?: string;
  message?: string;
  source?: string;
  tags?: number[];
}
