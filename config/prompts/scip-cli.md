# CODE NAVIGATION

Use SCIP to search symbols, definitions, references, or file context.

CLI binary: gestalt-scip

Default output is 2-space indent, arrays show length and fields (toon).

## Commands

    gestalt-scip symbols <query>

gestalt-scip files internal/terminal/manager.go --symbols

    gestalt-scip definition symbol.id

    gestalt-scip references symbol.id

    gestalt-scip files <path> --symbols

## Recommended workflow
1. Start with symbol search: gestalt-scip symbols Manager
2) Copy the id and fetch the definition: gestalt-scip definition <symbol.id>
3) If needed, fetch references: gestalt-scip references <symbol.id>
4) Inspect a file with symbol annotations: gestalt-scip files <symbol.file_path> --symbols
