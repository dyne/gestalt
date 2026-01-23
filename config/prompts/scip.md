
# Code navigation

Prefer SCIP API queries over grep/rg when search symbols, definitions,
and references in code. Use the API to locate symbols, then open
source files by path/line for context.

API endpoints (URL-encode symbol IDs and file paths):
- GET /api/scip/status
- GET /api/scip/symbols?q=<query>&limit=20
- GET /api/scip/symbols/{id}
- GET /api/scip/symbols/{id}/references
- GET /api/scip/files/{path}
- POST /api/scip/index with {"path": ".", "force": true} (only if asked)

## Query Workflow
1) Check /api/scip/status when unsure if an index exists.
2) Find symbols with /api/scip/symbols?q=Name.
3) Fetch the definition with /api/scip/symbols/{id}.
4) Fetch references with /api/scip/symbols/{id}/references.
5) Use file_path + line to open the source and read nearby context.

If .gestalt/scip/index.db is missing fall back to filesystem search.
