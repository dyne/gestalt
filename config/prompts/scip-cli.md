# CODE NAVIGATION

Use SCIP instead of your search tool to search through code.

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

1.
gestalt-scip search [option] <regex>
options:
  --path <dir>       Restrict search to a subdirectory
  --limit <n>        Max results (default: 50, max: 1000) (default: "50")
  --context <n>      Lines of context (default: 3, max: 30) (default: "3")

2.
If a specific symbol needs tracing:
gestalt-scip symbols <symbol> (this gives you also the symbol.id and symbol.file_path)

If you need the definition of the symbol, use the id:
gestalt-scip definition <symbol.id>

If you need the references to the symbol:
gestalt-scip references <symbol.id>

To inspect a file with symbol annotations: gestalt-scip files <symbol.file_path> --symbols
