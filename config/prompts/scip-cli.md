# Offline code navigation (SCIP CLI)

Prefer the offline CLI when you need symbols, definitions, references, or
file context without depending on the HTTP API.

CLI binary: `gestalt-scip`

## Commands
- `gestalt-scip symbols <query>`
- `gestalt-scip definition <symbol-id>`
- `gestalt-scip references <symbol-id>`
- `gestalt-scip files <path> --symbols`

## Recommended workflow
1) Start with symbol search:
   - `gestalt-scip symbols Manager --format json`
2) Copy the returned `id` and fetch the definition:
   - `gestalt-scip definition "<symbol-id>" --format json`
3) If needed, fetch references:
   - `gestalt-scip references "<symbol-id>" --format json`
4) Inspect a file with symbol annotations:
   - `gestalt-scip files internal/terminal/manager.go --symbols --format json`

## Discovery and filtering
- Auto-discovery looks under `.gestalt/scip/` from the current directory
  and up the parent directories.
- By default, the CLI merges all discovered `.scip` files across languages.
- Use `--language <lang>` to filter (for example: `go`, `typescript`, `python`).
- Use `--scip <path>` to point to a specific `.scip` file or directory.

## Output formats
- Use `--format json` for machine-readable output you can parse and reuse.
- Use `--format text` for quick human inspection.
- Use `--format toon` for compact tabular output derived from the JSON payload.
- Symbol IDs in output are base64url-encoded and safe to paste into `definition` and `references`.

## Index freshness and warnings
- The server indexes asynchronously on startup unless `--noindex` (or
  `GESTALT_SCIP_NO_INDEX=true`) is set.
- Missing indexer binaries are warnings, not fatal. Use whatever `.scip`
  files already exist.
