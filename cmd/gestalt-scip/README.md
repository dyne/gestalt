# gestalt-scip

Offline CLI tool for querying SCIP code intelligence indexes.

## Features
- No network dependency (reads `.scip` files directly)
- Multi-language support (Go, TypeScript, Python, etc.)
- JSON output compatible with gestalt SCIP API responses
- Auto-discovers SCIP files in `.gestalt/scip/`

## Usage

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

## Symbol IDs

- Symbol IDs in output are base64url-encoded and safe to paste into the shell.
- `definition` and `references` accept both encoded IDs and raw SCIP IDs.

## Output formats

- `json`: machine-readable output that matches the gestalt SCIP API schema
- `text`: human-readable output with context
- `toon`: compact tabular output rendered from the JSON payload

## Development

Build:
```bash
npm run build
```

Test:
```bash
npm test
```
