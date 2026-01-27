# CODE NAVIGATION

Use SCIP to search symbols, definitions, references, or file context.

CLI binary: gestalt-scip

Default output is 2-space indent, arrays show length and fields (toon).

## Commands

    gestalt-scip index --path <repo> --output .gestalt/scip/index.scip

Generate or refresh SCIP indexes (writes `.gestalt/scip/index.scip` and `.meta.json` by default).

    gestalt-scip search <regex>

Search file contents with regex patterns (supports OR via |)

    gestalt-scip symbols <symbol>

gestalt-scip files internal/terminal/manager.go --symbols

    gestalt-scip definition symbol.id

    gestalt-scip references symbol.id

    gestalt-scip files <path> --symbols

## Recommended workflow
1. Generate indexes: gestalt-scip index --path .
2. Start with search: gestalt-scip search <regex>
3. If a specific symbol needs tracing: gestalt-scip symbols <symbol>
4. Copy the symbol id and fetch the definition: gestalt-scip definition <symbol.id>
5. If needed, fetch references: gestalt-scip references <symbol.id>
6. Inspect a file with symbol annotations: gestalt-scip files <symbol.file_path> --symbols
