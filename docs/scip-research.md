# SCIP Research Notes

This captures findings for the L2 task "Research SCIP format and understand
SQLite schema" under the SCIP integration plan.

## Commands used

```
PATH=/usr/local/go/bin:$PATH GOMODCACHE=/tmp/gomodcache GOPATH=/tmp/gopath \
GOCACHE=/tmp/gocache /tmp/gopath/bin/scip-go --output index.scip \
  --project-root . --module-root . --repository-root . --skip-tests

scip stats index.scip
scip lint index.scip
scip expt-convert --output index.db index.scip
```

Notes:
- `scip lint` reported missing external symbols (stdlib and deps). This is
  expected when external symbols are not provided by the indexer.

## SCIP proto essentials (scip.proto)

- Index:
  - `metadata` (protocol version, tool info, project_root, text encoding).
  - `documents` list of per-file entries.
  - `external_symbols` for deps (optional; empty for scip-go by default).
- Document:
  - `language`, `relative_path`, `occurrences`, `symbols`, `text`,
    `position_encoding`.
- Occurrence:
  - `range` (3 or 4 int32s; 0-based; character encoding per Document).
  - `symbol` (string id), `symbol_roles` (bitset), optional docs and
    `enclosing_range`.
- SymbolInformation:
  - `symbol`, `documentation`, `relationships`, `kind`, `signature`,
    `enclosing_symbol`.
- Relationship:
  - `is_reference`, `is_implementation`, `is_type_definition`, `is_definition`.
- SymbolRole bitset: Definition=1, Import=2, WriteAccess=4, ReadAccess=8.

## SQLite schema from scip expt-convert

Tables observed in `index.db`:

- `documents`
  - `id` (INTEGER PK), `language` (TEXT), `relative_path` (TEXT, required),
    `position_encoding` (TEXT), `text` (TEXT).
  - Observed `text` and `position_encoding` were NULL for scip-go output.
- `global_symbols`
  - `id` (INTEGER PK), `symbol` (TEXT, required), `display_name` (TEXT),
    `kind` (INTEGER), `documentation` (TEXT), `signature` (BLOB),
    `enclosing_symbol` (TEXT), `relationships` (BLOB).
  - `kind` maps to `SymbolInformation.Kind` enum.
  - `signature` and `relationships` are protobuf blobs.
- `chunks`
  - `id` (INTEGER PK), `document_id` (INTEGER), `chunk_index` (INTEGER),
    `start_line` (INTEGER), `end_line` (INTEGER), `occurrences` (BLOB).
  - `start_line`/`end_line` define the line range covered by the chunk.
- `mentions`
  - `chunk_id` (INTEGER), `symbol_id` (INTEGER), `role` (INTEGER bitset).
  - Quick mapping from symbols to chunks and roles.
- `defn_enclosing_ranges`
  - `id` (INTEGER PK), `document_id` (INTEGER), `symbol_id` (INTEGER),
    `start_line`, `start_char`, `end_line`, `end_char`.
  - Useful for definition ranges and outline/hover logic.

Indexes observed:
- `idx_chunks_doc_id`, `idx_chunks_line_range`
- `idx_defn_enclosing_ranges_document`, `idx_defn_enclosing_ranges_symbol_id`
- `idx_global_symbols_symbol`
- `idx_mentions_symbol_id_role`

## Occurrence blob decoding notes

`chunks.occurrences` is stored as an opaque protobuf blob by design. A quick
hex dump shows plain ASCII symbol strings, suggesting the blob is raw protobuf
bytes (not compressed). The exact layout should be treated as an internal
encoding detail of `scip expt-convert`.

Suggested decode approach when building the query layer:
- Use the SCIP protobuf bindings (`scip.proto` -> language-specific bindings).
- Treat the blob as a sequence of `Occurrence` messages for the chunk.
- Parse the blob with a small wrapper message or a streaming protobuf parser.
- Use `mentions` for fast symbol -> chunk lookup, then decode only needed blobs.

If needed, `protoc --decode_raw` can be used to explore the blob structure.
