# gestalt-scip

Offline CLI tool for querying SCIP code intelligence indexes.

## Features
- No network dependency (reads `.scip` files directly)
- Multi-language support (Go, TypeScript, Python, etc.)
- JSON output suitable for scripting
- Auto-discovers SCIP files in `.gestalt/scip/`
- Generates indexes with `gestalt-scip index`

## Usage

Generate indexes:
```bash
gestalt-scip index --path . --output .gestalt/scip/index.scip
```

Search for symbols:
```bash
gestalt-scip symbols Manager
gestalt-scip symbols Manager --language go --limit 50 --format json
gestalt-scip symbols Manager --format toon
```

Get symbol definition:
```bash
gestalt-scip definition "scip-go gomod gestalt v0 `internal/terminal`/Manager#" --format json
```

Get symbol references:
```bash
gestalt-scip references "scip-go gomod gestalt v0 `internal/terminal`/Manager#" --format text
```

Get file content:
```bash
gestalt-scip files internal/terminal/manager.go --symbols --format json
```

### Searching File Contents

Search across all indexed file contents with regex support:

```bash
# Basic search (case-insensitive)
gestalt-scip search "handleScipReindex"

# OR clauses for multiple terms
gestalt-scip search "error|warning|fail"

# Regex patterns with wildcards
gestalt-scip search "terminal.*manager"

# Case-sensitive search
gestalt-scip search "ErrorCode" --case-sensitive

# Filter by language
gestalt-scip search "async function" --language typescript

# Restrict search to a subdirectory
gestalt-scip search "handleScipReindex" --path src

# More context lines
gestalt-scip search "TODO" --context 5

# Limit results
gestalt-scip search "const" --limit 10

# JSON output for scripting
gestalt-scip search "export" --format json
```

The search command supports:
- Full regex patterns (JavaScript regex syntax)
- OR clauses via pipe `|` separator
- Case-sensitive/insensitive modes
- Configurable context lines (default 3)
- Path scoping via `--path`
- Language filtering
- Multiple output formats (json, text, toon)

## Symbol IDs

- Symbol IDs in output are base64url-encoded and safe to paste into the shell.
- `definition` and `references` accept both encoded IDs and raw SCIP IDs.

## Output formats

- `json`: machine-readable output suitable for scripting
- `text`: human-readable output with context
- `toon`: compact tabular output rendered from the JSON payload
- Default format: `toon`

## Development

Build:
```bash
npm run build
```

Test:
```bash
npm test
```
