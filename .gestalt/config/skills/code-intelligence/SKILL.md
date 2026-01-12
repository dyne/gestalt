---
name: code-intelligence
version: 1.0.0
description: Query code symbols and structure using SCIP indexes
category: analysis
---

# Code Intelligence (SCIP)

Use SCIP (Sourcegraph Code Intelligence Protocol) endpoints to query symbols,
definitions, and references. Use these APIs before asking for code structure
that can be resolved through symbol lookup.

## Endpoints

- GET /api/scip/symbols?q=<query>&limit=<n>
  - Search symbols by name or symbol string.
- GET /api/scip/symbols/<symbol-id>
  - Fetch symbol definition and metadata.
- GET /api/scip/symbols/<symbol-id>/references
  - Find references to a symbol.
- GET /api/scip/files/<file-path>
  - List symbols defined in a file.
- POST /api/scip/index
  - Trigger on-demand indexing. Body: {"path":"/repo/path"}

## Usage pattern

1. Search for a symbol: /api/scip/symbols?q=ProcessOrder
2. Resolve definition: /api/scip/symbols/<id>
3. Find usages: /api/scip/symbols/<id>/references
4. If needed, list file symbols: /api/scip/files/internal/orders/service.go

## Notes

- Symbol and file path parameters must be URL-encoded.
- Results include line numbers (0-based from SCIP). Add +1 for editor display.
- If the index is missing, requests return 503.
- Rate limits apply; cache results when possible.
