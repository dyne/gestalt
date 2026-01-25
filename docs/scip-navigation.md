# SCIP Navigation

Gestalt supports code navigation through both the HTTP API and an offline CLI.

## Offline CLI: `gestalt-scip`

`gestalt-scip` reads raw `.scip` indexes directly from `.gestalt/scip/` and does not require the server to be running.

Examples:
```bash
gestalt-scip symbols Manager
gestalt-scip symbols Manager --language go --format json
gestalt-scip definition "scip-go gomod gestalt v0 `internal/terminal`/Manager#"
gestalt-scip references "scip-go gomod gestalt v0 `internal/terminal`/Manager#"
gestalt-scip files internal/terminal/manager.go --symbols --format json
```

Key options:
- `--scip <path>`: path to a `.scip` file or directory
- `--language <lang>`: limit results to a single language index
- `--format <fmt>`: output format (`text` or `json`)

## Index location and refresh

- Index files live under `.gestalt/scip/`.
- Server startup performs background indexing by default.
- You can trigger reindexing from the dashboard or via `POST /api/scip/reindex`.
