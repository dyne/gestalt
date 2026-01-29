# Search in code

Use `gestalt-scip` instead of your search tool.

Default output is 2-space indent, arrays show length and fields (toon).

    gestalt-scip search [option] <regex>
Search file contents with regex patterns (supports OR via |)
options:
  --path <dir>  Restrict search to a subdirectory
  --limit <n>   Max results (default: 50, max: 1000) (default: "50")
  --context <n> Lines of context (default: 3, max: 30) (default: "3")

    gestalt-scip symbols <symbol>
To trace a specific symbol. Returns symbol.id and symbol.file_path.

    gestalt-scip definition <symbol.id>
To retrieve the definition of a symbol.

    gestalt-scip references <symbol.id>
For a list of references to the symbol.

    gestalt-scip files <symbol.file_path> --symbols
To inspect a file with symbol annotations.
